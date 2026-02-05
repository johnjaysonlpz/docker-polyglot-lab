from __future__ import annotations

import ipaddress
import logging
import re
import time
import traceback
import uuid
from collections.abc import Callable
from typing import Any, cast

from django.conf import settings as dj_settings
from django.core.exceptions import RequestDataTooBig
from django.http import HttpRequest, HttpResponse, HttpResponseBase
from django.utils.deprecation import MiddlewareMixin

from . import errors, metrics
from .request_id import reset_request_id, set_request_id

log = logging.getLogger("http")

INFRA_PATHS = {"/health", "/ready", "/metrics"}
REQUEST_ID_HEADER = "X-Request-ID"

REQ_ID_RE = re.compile(r"^[A-Za-z0-9._\-:]+$")
REQ_ID_MAX_LEN = 128

IPAddress = ipaddress.IPv4Address | ipaddress.IPv6Address
IPNetwork = ipaddress.IPv4Network | ipaddress.IPv6Network


def _is_valid_request_id(v: str | None) -> bool:
    if v is None:
        return False

    s = v.strip()
    if not s or len(s) > REQ_ID_MAX_LEN:
        return False

    if any(c in ("\r", "\n", "\t") for c in s):
        return False

    return REQ_ID_RE.fullmatch(s) is not None


def _stable_path_label(request: HttpRequest, raw_path: str, status: int) -> str:
    if status == 404:
        return "__unmatched__"

    match = getattr(request, "resolver_match", None)
    route = getattr(match, "route", None) if match else None

    if match is not None and route == "":
        return "/"

    if route:
        return route if route.startswith("/") else f"/{route}"

    if raw_path in INFRA_PATHS:
        return raw_path

    return "__unmatched__"


def _trusted_proxy_networks() -> list[IPNetwork]:
    networks = getattr(dj_settings, "TRUSTED_PROXY_NETWORKS", None) or []
    return [n for n in networks if isinstance(n, ipaddress.IPv4Network | ipaddress.IPv6Network)]


def _is_trusted_ip(ip: IPAddress) -> bool:
    for n in _trusted_proxy_networks():
        try:
            if ip in n:
                return True
        except Exception:
            continue
    return False


def _parse_ip(s: str) -> IPAddress | None:
    try:
        ip = ipaddress.ip_address(s)
    except Exception:
        return None

    if isinstance(ip, ipaddress.IPv4Address | ipaddress.IPv6Address):
        return ip
    return None


def _client_ip(request: HttpRequest) -> str:
    remote_s = (request.META.get("REMOTE_ADDR", "") or "").strip()
    if not remote_s:
        return ""

    remote_ip = _parse_ip(remote_s)
    if remote_ip is None:
        return remote_s

    if not _is_trusted_ip(remote_ip):
        return remote_s

    xff = (request.META.get("HTTP_X_FORWARDED_FOR", "") or "").strip()
    if not xff:
        return remote_s

    chain: list[IPAddress] = []
    for part in [p.strip() for p in xff.split(",") if p.strip()]:
        ip = _parse_ip(part)
        if ip is not None:
            chain.append(ip)
    chain.append(remote_ip)

    for ip in reversed(chain):
        if not _is_trusted_ip(ip):
            return str(ip)

    return str(chain[0]) if chain else remote_s


def _bytes_written(response: HttpResponseBase) -> int:
    try:
        cl = response.get("Content-Length")
        if cl is not None:
            s = str(cl).strip()
            if s.isdigit():
                return int(s)
    except Exception:
        pass

    if getattr(response, "streaming", False):
        return 0

    try:
        content = getattr(response, "content", b"")
        return len(content)
    except Exception:
        return 0


def _truncate(s: str, max_len: int) -> str:
    v = (s or "").strip()
    return v if len(v) <= max_len else v[:max_len]


def _set_span_request_id(rid: str) -> None:
    if not rid:
        return
    try:
        from opentelemetry import trace

        span = trace.get_current_span()
        ctx = span.get_span_context() if span is not None else None
        if ctx is not None and getattr(ctx, "is_valid", False):
            span.set_attribute("request_id", rid)
    except Exception:
        return


class RequestIdMiddleware:
    def __init__(self, get_response: Callable[[HttpRequest], HttpResponse]) -> None:
        self.get_response = get_response

    def __call__(self, request: HttpRequest) -> HttpResponse:
        incoming = (request.headers.get(REQUEST_ID_HEADER, "") or "").strip()
        rid = incoming if _is_valid_request_id(incoming) else str(uuid.uuid4())

        # django-stubs doesn't model custom attributes on HttpRequest; use Any for the assignment.
        cast(Any, request).request_id = rid
        token = set_request_id(rid)

        _set_span_request_id(rid)

        try:
            response = self.get_response(request)
        finally:
            reset_request_id(token)

        response[REQUEST_ID_HEADER] = rid
        return response


class MaxBodyBytesMiddleware(MiddlewareMixin):
    def process_view(
        self,
        request: HttpRequest,
        view_func: Any,
        view_args: list[Any],
        view_kwargs: dict[str, Any],
    ) -> HttpResponse | None:
        limit = int(getattr(dj_settings, "MAX_BODY_BYTES", 0) or 0)
        if limit <= 0:
            return None

        raw = (request.META.get("CONTENT_LENGTH") or "").strip()
        if not raw:
            return None

        try:
            n = int(raw)
        except ValueError:
            return None

        if n > limit:
            return errors.payload_too_large(request)

        return None


class HttpLoggingAndMetricsMiddleware(MiddlewareMixin):
    def process_request(self, request: HttpRequest) -> None:
        # django-stubs doesn't include these attrs; store them via Any to satisfy mypy --strict.
        req = cast(Any, request)
        req._start_time = time.monotonic()
        req._caught_exc = None
        req._caught_exc_tb = None

    def process_exception(
        self, request: HttpRequest, exception: BaseException
    ) -> HttpResponse | None:
        req = cast(Any, request)
        req._caught_exc = exception

        max_chars = int(getattr(dj_settings, "STACK_TRACE_MAX_CHARS", 8192))
        tb = "".join(
            traceback.format_exception(type(exception), exception, exception.__traceback__)
        )
        req._caught_exc_tb = _truncate(tb.strip(), max_chars)

        if isinstance(exception, RequestDataTooBig):
            return errors.payload_too_large(request)

        return None

    def process_response(self, request: HttpRequest, response: HttpResponse) -> HttpResponse:
        start = getattr(request, "_start_time", None)
        if not isinstance(start, int | float):
            return response

        duration = time.monotonic() - float(start)

        raw_path = request.path
        method = request.method or ""
        status = int(getattr(response, "status_code", 0) or 0)

        path_label = _stable_path_label(request, raw_path, status)

        metrics.record_http_request(
            method=method,
            path=path_label,
            status_code=status,
            duration_seconds=duration,
        )

        rid = getattr(request, "request_id", "") or ""
        _set_span_request_id(rid)

        if raw_path in INFRA_PATHS and status < 400:
            return response

        event: dict[str, Any] = {
            "method": method,
            "path": path_label,
            "raw_path": raw_path,
            "status": status,
            "ip": _client_ip(request),
            "latency_ms": round(duration * 1000.0, 3),
            "bytes_written": _bytes_written(response),
            "user_agent": _truncate(request.META.get("HTTP_USER_AGENT", "") or "", 512),
        }

        exc = getattr(request, "_caught_exc", None)
        tb2 = getattr(request, "_caught_exc_tb", None)

        if exc is not None:
            event["error"] = exc.__class__.__name__
            event["error_message"] = _truncate(str(exc), 300)
            if isinstance(tb2, str) and tb2:
                event["stack_trace"] = tb2
        elif status >= 500:
            event["error"] = "HTTP_5XX"

        if status >= 500:
            log.error("http_request", extra=event)
        elif status >= 400:
            log.warning("http_request", extra=event)
        else:
            log.info("http_request", extra=event)

        return response
