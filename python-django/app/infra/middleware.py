import ipaddress
import logging
import time
import uuid

from django.utils.deprecation import MiddlewareMixin

from django_app import settings
from . import metrics
from .request_id import get_request_id, set_request_id, reset_request_id

log = logging.getLogger("http")

INFRA_PATHS = {"/health", "/ready", "/metrics"}
REQUEST_ID_HEADER = "X-Request-ID"


def _stable_path_label(request) -> str:
    """
    Stable label for Prometheus:
      - matched routes: "/", "/info", "/health", "/ready", "/metrics"
      - unmatched routes (404): "__unmatched__"
    """
    match = getattr(request, "resolver_match", None)
    route = getattr(match, "route", None) if match else None

    if match is not None and route == "":
        return "/"
    if not route:
        return "__unmatched__"

    return route if route.startswith("/") else f"/{route}"


def _is_trusted_proxy(remote_addr: str) -> bool:
    trusted = (getattr(settings, "TRUSTED_PROXIES", "") or "").strip()
    if not trusted:
        return False

    try:
        ip = ipaddress.ip_address(remote_addr)
    except ValueError:
        return False

    for item in [x.strip() for x in trusted.split(",") if x.strip()]:
        try:
            if "/" in item:
                if ip in ipaddress.ip_network(item, strict=False):
                    return True
            else:
                if ip == ipaddress.ip_address(item):
                    return True
        except ValueError:
            continue

    return False


def _client_ip(request) -> str:
    """
    If TRUSTED_PROXIES is empty -> use REMOTE_ADDR.
    If REMOTE_ADDR is trusted -> prefer first X-Forwarded-For entry.
    """
    remote = (request.META.get("REMOTE_ADDR", "") or "").strip()
    if not remote:
        return ""

    if not _is_trusted_proxy(remote):
        return remote

    xff = (request.META.get("HTTP_X_FORWARDED_FOR", "") or "").strip()
    if not xff:
        return remote

    first = xff.split(",")[0].strip()
    return first or remote


class RequestIdMiddleware(MiddlewareMixin):
    """
    Request correlation:
      - Reuse incoming X-Request-ID if present, else generate one.
      - Store on request as request.request_id
      - Store in ContextVar so ALL logs get request_id automatically (MDC-like)
      - Always echo it back in response header
    """

    def process_request(self, request):
        incoming = (request.headers.get(REQUEST_ID_HEADER, "") or "").strip()
        rid = incoming if incoming else uuid.uuid4().hex

        request.request_id = rid
        request._request_id_token = set_request_id(rid)
        return None

    def process_response(self, request, response):
        if response is None:
            return response

        rid = getattr(request, "request_id", None) or get_request_id()
        if rid and rid != "-":
            response[REQUEST_ID_HEADER] = rid

        token = getattr(request, "_request_id_token", None)
        if token is not None:
            reset_request_id(token)

        return response


class HttpLoggingAndMetricsMiddleware(MiddlewareMixin):
    def process_request(self, request):
        request._start_time = time.monotonic()
        return None

    def process_response(self, request, response):
        if response is None:
            return response

        start = getattr(request, "_start_time", None)
        if start is None:
            return response

        duration = time.monotonic() - start

        raw_path = request.path
        query = request.META.get("QUERY_STRING", "") or ""

        method = request.method
        status = response.status_code

        stable_path = _stable_path_label(request)

        metrics.record_http_request(
            method=method,
            path=stable_path,
            status_code=status,
            duration_seconds=duration,
        )

        if raw_path in INFRA_PATHS:
            return response

        rid = getattr(request, "request_id", None) or get_request_id()
        client_ip = _client_ip(request)
        ua = request.META.get("HTTP_USER_AGENT", "") or ""

        event = {
            "event": "http_request",
            "request_id": rid,
            "service": settings.SERVICE_NAME,
            "version": settings.VERSION,
            "buildTime": settings.BUILD_TIME,
            "status": int(status),
            "method": method,
            "path": stable_path,
            "rawPath": raw_path,
            "query": query,
            "ip": client_ip,
            "latencyMs": round(duration * 1000.0, 3),
            "userAgent": ua,
        }

        if status >= 500:
            log.error("http_request", extra=event)
        elif status >= 400:
            log.warning("http_request", extra=event)
        else:
            log.info("http_request", extra=event)

        return response
