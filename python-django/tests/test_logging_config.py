from __future__ import annotations

import builtins
import logging
from typing import Any

import pytest

from infra.logging_config import (
    CanonicalJsonFormatter,
    RequestIdLogFilter,
    ServiceMetaLogFilter,
    TraceContextLogFilter,
    configure_logging_if_needed,
)
from infra.request_id import reset_request_id, set_request_id


def test_configure_logging_if_needed_idempotent() -> None:
    root = logging.getLogger()
    root.handlers.clear()
    configure_logging_if_needed()
    assert root.handlers
    before = list(root.handlers)
    configure_logging_if_needed()
    assert root.handlers == before


def test_configure_logging_if_needed_noop_when_handlers_exist() -> None:
    root = logging.getLogger()
    root.handlers.clear()
    root.addHandler(logging.NullHandler())
    before = list(root.handlers)
    configure_logging_if_needed()
    assert root.handlers == before


def test_request_id_filter_injects_from_contextvar() -> None:
    f = RequestIdLogFilter()
    record = logging.LogRecord("x", logging.INFO, __file__, 1, "m", (), None)

    tok = set_request_id("rid-from-ctx")
    try:
        record.request_id = "-"
        assert f.filter(record) is True
        assert record.request_id == "rid-from-ctx"
    finally:
        reset_request_id(tok)


def test_request_id_filter_deletes_empty_request_id_when_no_context() -> None:
    f = RequestIdLogFilter()
    record = logging.LogRecord("x", logging.INFO, __file__, 1, "m", (), None)
    record.request_id = ""
    assert f.filter(record) is True
    assert not hasattr(record, "request_id")


def test_request_id_filter_no_request_id_attr_is_noop() -> None:
    f = RequestIdLogFilter()
    record = logging.LogRecord("x", logging.INFO, __file__, 1, "m", (), None)

    tok = set_request_id("")
    try:
        assert f.filter(record) is True
        assert not hasattr(record, "request_id")
    finally:
        reset_request_id(tok)


def test_request_id_log_filter_keeps_existing_non_blank_request_id() -> None:
    f = RequestIdLogFilter()
    record = logging.LogRecord("x", logging.INFO, __file__, 1, "m", (), None)
    record.request_id = "keep-me"

    tok = set_request_id("ctx-should-not-overwrite")
    try:
        assert f.filter(record) is True
        assert record.request_id == "keep-me"
    finally:
        reset_request_id(tok)


def test_request_id_log_filter_suppresses_delattr_exception() -> None:
    f = RequestIdLogFilter()

    class _Rec:
        request_id = ""

        def __delattr__(self, name: str) -> None:
            raise RuntimeError("boom")

    tok = set_request_id("")
    try:
        r = _Rec()
        assert f.filter(r) is True
        assert hasattr(r, "request_id")
    finally:
        reset_request_id(tok)


def test_service_meta_log_filter_sets_defaults() -> None:
    f = ServiceMetaLogFilter()
    record: Any = logging.LogRecord("x", logging.INFO, __file__, 1, "m", (), None)
    assert f.filter(record) is True
    assert record.service
    assert record.version
    assert record.build_time


def test_service_meta_filter_does_not_overwrite_existing() -> None:
    f = ServiceMetaLogFilter()
    record = logging.LogRecord("x", logging.INFO, __file__, 1, "m", (), None)
    record.service = "custom"
    record.version = "v1"
    record.build_time = "bt"
    assert f.filter(record) is True
    assert record.service == "custom"
    assert record.version == "v1"
    assert record.build_time == "bt"


def test_trace_context_filter_success_and_exception_paths(monkeypatch: pytest.MonkeyPatch) -> None:
    f = TraceContextLogFilter()
    record: Any = logging.LogRecord("x", logging.INFO, __file__, 1, "m", (), None)

    class _Ctx:
        is_valid = True
        trace_id = 1
        span_id = 2

    class _Span:
        def get_span_context(self) -> _Ctx:
            return _Ctx()

        def set_attribute(self, k: str, v: str) -> None:
            return None

    import opentelemetry.trace

    monkeypatch.setattr(opentelemetry.trace, "get_current_span", lambda: _Span())
    assert f.filter(record) is True
    assert record.trace_id == "0" * 31 + "1"
    assert record.span_id == "0" * 15 + "2"

    orig_import = builtins.__import__

    def _bad_import(name: str, *args: Any, **kwargs: Any) -> Any:
        if name.startswith("opentelemetry"):
            raise RuntimeError("boom")
        return orig_import(name, *args, **kwargs)

    monkeypatch.setattr(builtins, "__import__", _bad_import)
    record2: Any = logging.LogRecord("x", logging.INFO, __file__, 1, "m", (), None)
    assert f.filter(record2) is True


def test_trace_context_filter_invalid_context_clears_existing(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    f = TraceContextLogFilter()
    record = logging.LogRecord("x", logging.INFO, __file__, 1, "m", (), None)
    record.trace_id = "abc"
    record.span_id = "def"

    class _Ctx:
        is_valid = False
        trace_id = 1
        span_id = 2

    class _Span:
        def get_span_context(self) -> _Ctx:
            return _Ctx()

    import opentelemetry.trace

    monkeypatch.setattr(opentelemetry.trace, "get_current_span", lambda: _Span())
    assert f.filter(record) is True
    assert not hasattr(record, "trace_id")
    assert not hasattr(record, "span_id")


def test_trace_context_filter_exception_path_clears_existing(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    f = TraceContextLogFilter()
    record = logging.LogRecord("x", logging.INFO, __file__, 1, "m", (), None)
    record.trace_id = "abc"
    record.span_id = "def"

    orig_import = builtins.__import__

    def _bad_import(name: str, *args: Any, **kwargs: Any) -> Any:
        if name.startswith("opentelemetry"):
            raise RuntimeError("boom")
        return orig_import(name, *args, **kwargs)

    monkeypatch.setattr(builtins, "__import__", _bad_import)
    assert f.filter(record) is True
    assert not hasattr(record, "trace_id")
    assert not hasattr(record, "span_id")


def test_canonical_json_formatter_formats_and_orders() -> None:
    formatter = CanonicalJsonFormatter(fmt="%(message)s")
    record = logging.LogRecord("x", logging.INFO, __file__, 1, "hello", (), None)
    out = formatter.format(record)
    assert '"msg"' in out
    assert '"level"' in out
    assert '"time"' in out


def test_canonical_formatter_process_log_record_removes_blank_ids_and_keeps_extras() -> None:
    fmt = CanonicalJsonFormatter(fmt="%(message)s")
    record = logging.LogRecord("x", logging.INFO, __file__, 1, "hello", (), None)

    record.__dict__["request_id"] = "-"
    record.__dict__["trace_id"] = ""
    record.__dict__["span_id"] = None
    record.__dict__["zzz"] = 123

    out = fmt.format(record)
    assert '"request_id"' not in out
    assert '"trace_id"' not in out
    assert '"span_id"' not in out
    assert '"zzz"' in out
