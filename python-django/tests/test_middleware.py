import builtins
import ipaddress
import json
import logging
import uuid
from typing import Any, cast

import pytest
from django.core.exceptions import RequestDataTooBig
from django.http import HttpRequest, HttpResponse, StreamingHttpResponse
from django.test import Client, RequestFactory
from django.test.utils import override_settings

from infra.middleware import (
    HttpLoggingAndMetricsMiddleware,
    MaxBodyBytesMiddleware,
    _bytes_written,
    _client_ip,
    _is_trusted_ip,
    _is_valid_request_id,
    _parse_ip,
    _set_span_request_id,
    _stable_path_label,
    _truncate,
)


def test_is_valid_request_id_rules() -> None:
    assert _is_valid_request_id("abc-123._:") is True
    assert _is_valid_request_id("") is False
    assert _is_valid_request_id(None) is False
    assert _is_valid_request_id("a" * 129) is False
    assert _is_valid_request_id("abc\n123") is False
    assert _is_valid_request_id("has space") is False
    assert _is_valid_request_id("abc\t123") is False
    assert _is_valid_request_id("abc\r123") is False


def test_request_id_header_roundtrip(client: Client, request_id_header_name: str) -> None:
    header_key = f"HTTP_{request_id_header_name.upper().replace('-', '_')}"

    headers_ok = cast(dict[str, Any], {header_key: "rid-1"})
    r = client.get("/health", **headers_ok)
    assert r.status_code == 200
    assert r[request_id_header_name] == "rid-1"

    headers_bad = cast(dict[str, Any], {header_key: "bad\nid"})
    r2 = client.get("/health", **headers_bad)
    rid = r2[request_id_header_name]
    uuid.UUID(rid)


def test_max_body_bytes_middleware_blocks_large_payload_and_edge_cases() -> None:
    mw = MaxBodyBytesMiddleware(lambda r: HttpResponse("ok"))
    rf = RequestFactory()

    with override_settings(MAX_BODY_BYTES=10):
        req = rf.get("/health")
        req.META["CONTENT_LENGTH"] = "11"
        resp = mw.process_view(req, None, [], {})
        assert resp is not None
        assert resp.status_code == 413
        payload = json.loads(resp.content)
        assert payload["code"] == "payload_too_large"

        req2 = rf.get("/health")
        req2.META["CONTENT_LENGTH"] = "9"
        assert mw.process_view(req2, None, [], {}) is None

        req3 = rf.get("/health")
        req3.META["CONTENT_LENGTH"] = "nope"
        assert mw.process_view(req3, None, [], {}) is None

        req4 = rf.get("/health")
        assert mw.process_view(req4, None, [], {}) is None

    with override_settings(MAX_BODY_BYTES=0):
        req5 = rf.get("/health")
        assert mw.process_view(req5, None, [], {}) is None


def test_stable_path_label_routes_and_special_cases() -> None:
    rf = RequestFactory()
    req = rf.get("/x")

    assert _stable_path_label(req, "/x", 404) == "__unmatched__"

    req2 = rf.get("/x")
    req2.resolver_match = cast(Any, type("M", (), {"route": ""})())
    assert _stable_path_label(req2, "/x", 200) == "/"

    req3 = rf.get("/x")
    req3.resolver_match = cast(Any, type("M", (), {"route": "info"})())
    assert _stable_path_label(req3, "/x", 200) == "/info"

    assert _stable_path_label(req, "/health", 200) == "/health"
    assert _stable_path_label(req, "/nope", 200) == "__unmatched__"


def test_client_ip_branches_and_trusted_proxy_chain() -> None:
    rf = RequestFactory()

    req0 = rf.get("/x")
    req0.META["REMOTE_ADDR"] = ""
    assert _client_ip(req0) == ""

    req1 = rf.get("/x")
    req1.META["REMOTE_ADDR"] = "not-an-ip"
    assert _client_ip(req1) == "not-an-ip"

    req2 = rf.get("/x")
    req2.META["REMOTE_ADDR"] = "10.0.0.1"
    with override_settings(TRUSTED_PROXY_NETWORKS=[ipaddress.ip_network("10.0.0.0/8")]):
        assert _client_ip(req2) == "10.0.0.1"

    req3 = rf.get("/x")
    req3.META["REMOTE_ADDR"] = "10.0.0.1"
    req3.META["HTTP_X_FORWARDED_FOR"] = "10.0.0.2, 10.0.0.3"
    with override_settings(TRUSTED_PROXY_NETWORKS=[ipaddress.ip_network("10.0.0.0/8")]):
        assert _client_ip(req3) == "10.0.0.2"

    req4 = rf.get("/x")
    req4.META["REMOTE_ADDR"] = "10.0.0.1"
    req4.META["HTTP_X_FORWARDED_FOR"] = "junk, 1.2.3.4"
    with override_settings(TRUSTED_PROXY_NETWORKS=[ipaddress.ip_network("10.0.0.0/8")]):
        assert _client_ip(req4) == "1.2.3.4"

    req5 = rf.get("/x")
    req5.META["REMOTE_ADDR"] = "10.0.0.1"
    req5.META["HTTP_X_FORWARDED_FOR"] = "junk, also-junk"
    with override_settings(TRUSTED_PROXY_NETWORKS=[ipaddress.ip_network("10.0.0.0/8")]):
        assert _client_ip(req5) == "10.0.0.1"


def test_is_trusted_ip_exception_path(monkeypatch: pytest.MonkeyPatch) -> None:
    class _BadNet:
        def __contains__(self, _ip: object) -> bool:
            raise RuntimeError("boom")

    monkeypatch.setattr("infra.middleware._trusted_proxy_networks", lambda: [_BadNet()])
    assert _is_trusted_ip(ipaddress.ip_address("1.2.3.4")) is False


def test_parse_ip_returns_none_when_ipaddress_returns_non_ip(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(ipaddress, "ip_address", lambda _s: object())
    assert _parse_ip("1.2.3.4") is None


def test_bytes_written_variants_and_edge_cases() -> None:
    resp = HttpResponse(b"hello")
    assert _bytes_written(resp) == 5

    resp2 = HttpResponse(b"hello")
    resp2["Content-Length"] = "123"
    assert _bytes_written(resp2) == 123

    resp3 = HttpResponse(b"hi")
    resp3["Content-Length"] = "nope"
    assert _bytes_written(resp3) == 2

    sresp = StreamingHttpResponse(iter([b"a", b"b"]))
    assert _bytes_written(sresp) == 0

    class _Resp:
        streaming = False
        content = b"hi"

        def get(self, _k: str) -> str:
            raise RuntimeError("boom")

    assert _bytes_written(cast(Any, _Resp())) == 2

    class _Resp2:
        streaming = False

        def get(self, _k: str) -> None:
            return None

        @property
        def content(self) -> bytes:
            raise RuntimeError("boom")

    assert _bytes_written(cast(Any, _Resp2())) == 0


def test_truncate() -> None:
    assert _truncate("abc", 2) == "ab"
    assert _truncate("abc", 3) == "abc"


def test_process_response_returns_early_when_start_time_missing() -> None:
    rf = RequestFactory()
    req = rf.get("/x")
    resp = HttpResponse("ok", status=200)

    mw = HttpLoggingAndMetricsMiddleware(lambda r: resp)
    out = mw.process_response(req, resp)
    assert out is resp


def test_process_exception_request_data_too_big_returns_413() -> None:
    rf = RequestFactory()
    req = rf.get("/x")
    mw = HttpLoggingAndMetricsMiddleware(lambda r: HttpResponse("ok"))

    mw.process_request(req)
    resp = mw.process_exception(req, RequestDataTooBig("boom"))
    assert resp is not None
    assert resp.status_code == 413

    data = json.loads(resp.content)
    assert data["code"] == "payload_too_large"


def test_infra_paths_skip_http_request_logging(caplog: pytest.LogCaptureFixture) -> None:
    rf = RequestFactory()
    req = rf.get("/health")
    resp = HttpResponse("ok", status=200)

    mw = HttpLoggingAndMetricsMiddleware(lambda r: resp)
    mw.process_request(req)

    http_logger = logging.getLogger("http")
    http_logger.addHandler(caplog.handler)
    try:
        caplog.set_level(logging.INFO, logger="http")
        mw.process_response(req, resp)
        assert not any(rec.getMessage() == "http_request" for rec in caplog.records)
    finally:
        http_logger.removeHandler(caplog.handler)


def test_infra_path_with_error_does_not_skip_http_logging(caplog: pytest.LogCaptureFixture) -> None:
    rf = RequestFactory()
    req = rf.get("/health")
    resp = HttpResponse("err", status=500)

    mw = HttpLoggingAndMetricsMiddleware(lambda r: resp)
    mw.process_request(req)

    http_logger = logging.getLogger("http")
    http_logger.addHandler(caplog.handler)
    try:
        caplog.set_level(logging.ERROR, logger="http")
        mw.process_response(req, resp)
        assert any(
            rec.name == "http" and rec.getMessage() == "http_request" for rec in caplog.records
        )
    finally:
        http_logger.removeHandler(caplog.handler)


def test_4xx_logs_warning(caplog: pytest.LogCaptureFixture) -> None:
    rf = RequestFactory()
    req = rf.get("/does-not-exist")
    resp = HttpResponse("nope", status=404)

    mw = HttpLoggingAndMetricsMiddleware(lambda r: resp)
    mw.process_request(req)

    http_logger = logging.getLogger("http")
    http_logger.addHandler(caplog.handler)
    try:
        caplog.set_level(logging.WARNING, logger="http")
        mw.process_response(req, resp)
        assert any(
            rec.name == "http"
            and rec.levelno == logging.WARNING
            and rec.getMessage() == "http_request"
            for rec in caplog.records
        )
    finally:
        http_logger.removeHandler(caplog.handler)


def test_http_logging_middleware_exception_and_5xx_logging(
    caplog: pytest.LogCaptureFixture,
) -> None:
    rf = RequestFactory()
    req: HttpRequest = rf.get("/x")

    mw = HttpLoggingAndMetricsMiddleware(lambda r: HttpResponse("ok"))

    mw.process_request(req)
    resp = mw.process_exception(req, ValueError("boom"))
    assert resp is None

    http_logger = logging.getLogger("http")
    http_logger.addHandler(caplog.handler)
    try:
        caplog.set_level(logging.ERROR, logger="http")
        r500 = HttpResponse("err", status=500)
        mw.process_response(req, r500)

        assert any(
            rec.name == "http" and rec.getMessage() == "http_request" for rec in caplog.records
        )
    finally:
        http_logger.removeHandler(caplog.handler)


def test_http_logging_5xx_without_exception_sets_http_5xx_error(
    caplog: pytest.LogCaptureFixture,
) -> None:
    rf = RequestFactory()
    req = rf.get("/x")
    mw = HttpLoggingAndMetricsMiddleware(lambda r: HttpResponse("ok"))

    mw.process_request(req)

    http_logger = logging.getLogger("http")
    http_logger.addHandler(caplog.handler)
    try:
        caplog.set_level(logging.ERROR, logger="http")
        r500 = HttpResponse("err", status=500)
        mw.process_response(req, r500)

        recs = [r for r in caplog.records if r.name == "http" and r.getMessage() == "http_request"]
        assert recs
        assert recs[-1].error == "HTTP_5XX"
    finally:
        http_logger.removeHandler(caplog.handler)


def test_http_logging_non_infra_2xx_logs_info_and_covers_short_circuit(
    caplog: pytest.LogCaptureFixture,
) -> None:
    rf = RequestFactory()
    req = rf.get("/not-infra")
    resp = HttpResponse("ok", status=200)

    mw = HttpLoggingAndMetricsMiddleware(lambda r: resp)
    mw.process_request(req)

    http_logger = logging.getLogger("http")
    http_logger.addHandler(caplog.handler)
    try:
        caplog.set_level(logging.INFO, logger="http")
        mw.process_response(req, resp)
        assert any(
            rec.name == "http"
            and rec.levelno == logging.INFO
            and rec.getMessage() == "http_request"
            for rec in caplog.records
        )
    finally:
        http_logger.removeHandler(caplog.handler)


def test_exception_path_with_empty_tb_skips_stack_trace_branch(
    caplog: pytest.LogCaptureFixture,
) -> None:
    rf = RequestFactory()
    req = rf.get("/x")

    mw = HttpLoggingAndMetricsMiddleware(lambda r: HttpResponse("ok"))
    mw.process_request(req)

    with override_settings(STACK_TRACE_MAX_CHARS=0):
        mw.process_exception(req, ValueError("boom"))

    http_logger = logging.getLogger("http")
    http_logger.addHandler(caplog.handler)
    try:
        caplog.set_level(logging.ERROR, logger="http")
        resp = HttpResponse("err", status=500)
        mw.process_response(req, resp)

        recs = [r for r in caplog.records if r.name == "http" and r.getMessage() == "http_request"]
        assert recs
        assert not hasattr(recs[-1], "stack_trace")
    finally:
        http_logger.removeHandler(caplog.handler)


def test_set_span_request_id_success_path(monkeypatch: pytest.MonkeyPatch) -> None:
    seen: list[tuple[str, str]] = []

    class _Ctx:
        is_valid = True
        trace_id = 1
        span_id = 2

    class _Span:
        def get_span_context(self) -> _Ctx:
            return _Ctx()

        def set_attribute(self, k: str, v: str) -> None:
            seen.append((k, v))

    import opentelemetry.trace

    monkeypatch.setattr(opentelemetry.trace, "get_current_span", lambda: _Span())
    _set_span_request_id("rid-123")
    assert ("request_id", "rid-123") in seen


def test_set_span_request_id_exception_path_is_swallowed(monkeypatch: pytest.MonkeyPatch) -> None:
    orig_import = builtins.__import__

    def _bad_import(name: str, *args: Any, **kwargs: Any) -> Any:
        if name.startswith("opentelemetry"):
            raise RuntimeError("boom")
        return orig_import(name, *args, **kwargs)

    monkeypatch.setattr(builtins, "__import__", _bad_import)
    _set_span_request_id("rid-123")
