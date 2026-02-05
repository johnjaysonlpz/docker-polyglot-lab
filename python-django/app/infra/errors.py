from __future__ import annotations

import uuid
from typing import Any

from django.http import HttpRequest, HttpResponse, JsonResponse

from .request_id import get_request_id

MSG_BAD_REQUEST = "bad request"
MSG_FORBIDDEN = "forbidden"
MSG_NOT_FOUND = "not found"
MSG_METHOD_NOT_ALLOWED = "method not allowed"
MSG_PAYLOAD_TOO_LARGE = "payload too large"
MSG_INTERNAL = "internal server error"

CODE_BAD_REQUEST = "bad_request"
CODE_FORBIDDEN = "forbidden"
CODE_NOT_FOUND = "not_found"
CODE_METHOD_NOT_ALLOWED = "method_not_allowed"
CODE_PAYLOAD_TOO_LARGE = "payload_too_large"
CODE_INTERNAL = "internal_server_error"


def _request_id(request: HttpRequest | None) -> str:
    if request is not None:
        rid = getattr(request, "request_id", None)
        if isinstance(rid, str) and rid.strip():
            return rid.strip()

    rid = get_request_id()
    if rid not in ("", "-", None) and str(rid).strip():
        return str(rid).strip()

    return str(uuid.uuid4())


def json_error(
    request: HttpRequest | None,
    status: int,
    *,
    msg: str,
    code: str,
) -> HttpResponse:
    payload = {
        "error": msg,
        "code": code,
        "request_id": _request_id(request),
    }
    resp = JsonResponse(payload, status=status)
    resp["Cache-Control"] = "no-store"
    return resp


def bad_request(request: HttpRequest | None) -> HttpResponse:
    return json_error(request, 400, msg=MSG_BAD_REQUEST, code=CODE_BAD_REQUEST)


def forbidden(request: HttpRequest | None) -> HttpResponse:
    return json_error(request, 403, msg=MSG_FORBIDDEN, code=CODE_FORBIDDEN)


def not_found(request: HttpRequest | None) -> HttpResponse:
    return json_error(request, 404, msg=MSG_NOT_FOUND, code=CODE_NOT_FOUND)


def method_not_allowed(request: HttpRequest | None, *, allow: str | None = None) -> HttpResponse:
    resp = json_error(request, 405, msg=MSG_METHOD_NOT_ALLOWED, code=CODE_METHOD_NOT_ALLOWED)
    if allow:
        resp["Allow"] = allow
    return resp


def payload_too_large(request: HttpRequest | None) -> HttpResponse:
    return json_error(request, 413, msg=MSG_PAYLOAD_TOO_LARGE, code=CODE_PAYLOAD_TOO_LARGE)


def internal(request: HttpRequest | None) -> HttpResponse:
    return json_error(request, 500, msg=MSG_INTERNAL, code=CODE_INTERNAL)


def handler400(request: HttpRequest, exception: Any = None) -> HttpResponse:
    return bad_request(request)


def handler403(request: HttpRequest, exception: Any = None) -> HttpResponse:
    return forbidden(request)


def handler404(request: HttpRequest, exception: Any = None) -> HttpResponse:
    return not_found(request)


def handler500(request: HttpRequest) -> HttpResponse:
    return internal(request)
