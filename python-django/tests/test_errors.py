import json
import uuid

from django.http import HttpRequest
from django.test import RequestFactory

from infra import errors
from infra.request_id import reset_request_id, set_request_id


def test_json_error_includes_request_id_from_request_attr() -> None:
    req: HttpRequest = RequestFactory().get("/nope")
    req.request_id = "rid-123"

    resp = errors.not_found(req)
    assert resp.status_code == 404
    assert resp["Cache-Control"] == "no-store"

    data = json.loads(resp.content)
    assert data["code"] == errors.CODE_NOT_FOUND
    assert data["request_id"] == "rid-123"


def test_json_error_generates_request_id_if_missing() -> None:
    req: HttpRequest = RequestFactory().get("/nope")
    resp = errors.internal(req)
    rid = json.loads(resp.content)["request_id"]
    uuid.UUID(rid)


def test_errors_request_id_when_request_is_none_generates_uuid() -> None:
    resp = errors.internal(None)
    payload = json.loads(resp.content)
    uuid.UUID(payload["request_id"])


def test_request_id_prefers_context_when_request_attr_missing_or_invalid() -> None:
    req: HttpRequest = RequestFactory().get("/x")
    req.request_id = 123

    tok = set_request_id("ctx-123")
    try:
        resp = errors.not_found(req)
        data = json.loads(resp.content)
        assert data["request_id"] == "ctx-123"
    finally:
        reset_request_id(tok)


def test_errors_request_id_blank_request_attr_falls_back_to_ctx() -> None:
    req: HttpRequest = RequestFactory().get("/x")
    req.request_id = "   "

    tok = set_request_id("ctx-abc")
    try:
        resp = errors.not_found(req)
        data = json.loads(resp.content)
        assert data["request_id"] == "ctx-abc"
    finally:
        reset_request_id(tok)


def test_method_not_allowed_without_allow_header() -> None:
    req: HttpRequest = RequestFactory().get("/x")
    resp = errors.method_not_allowed(req)
    assert resp.status_code == 405
    assert "Allow" not in resp


def test_error_helpers_and_handlers_cover_all_paths() -> None:
    rf = RequestFactory()
    req = rf.get("/x")

    assert errors.bad_request(req).status_code == 400
    assert errors.forbidden(req).status_code == 403
    assert errors.not_found(req).status_code == 404

    resp = errors.method_not_allowed(req, allow="GET")
    assert resp.status_code == 405
    assert resp["Allow"] == "GET"

    assert errors.payload_too_large(req).status_code == 413
    assert errors.internal(req).status_code == 500

    assert errors.handler400(req, Exception("x")).status_code == 400
    assert errors.handler403(req, Exception("x")).status_code == 403
    assert errors.handler404(req, Exception("x")).status_code == 404
    assert errors.handler500(req).status_code == 500
