from __future__ import annotations

import logging
import logging.config
import os
from contextlib import suppress
from datetime import UTC, datetime
from typing import Any, ClassVar

from pythonjsonlogger.jsonlogger import JsonFormatter  # type: ignore[attr-defined]

from infra.request_id import get_request_id

SERVICE_NAME = os.getenv("SERVICE_NAME", "python-django-app")
VERSION = os.getenv("VERSION", "0.0.0-dev")
BUILD_TIME = os.getenv("BUILD_TIME", "unknown")
LOG_LEVEL = os.getenv("LOG_LEVEL", "INFO").upper()


class RequestIdLogFilter(logging.Filter):
    def filter(self, record: logging.LogRecord) -> bool:
        rid = getattr(record, "request_id", None)

        if rid in ("", "-", None) or str(rid).strip() == "":
            ctx = get_request_id()
            if ctx not in ("", "-", None) and str(ctx).strip() != "":
                record.request_id = ctx
            else:
                if hasattr(record, "request_id"):
                    with suppress(Exception):
                        delattr(record, "request_id")

        return True


class ServiceMetaLogFilter(logging.Filter):
    def filter(self, record: logging.LogRecord) -> bool:
        if not getattr(record, "service", None):
            record.service = SERVICE_NAME
        if not getattr(record, "version", None):
            record.version = VERSION
        if not getattr(record, "build_time", None):
            record.build_time = BUILD_TIME
        return True


class TraceContextLogFilter(logging.Filter):
    def filter(self, record: logging.LogRecord) -> bool:
        try:
            from opentelemetry import trace

            span = trace.get_current_span()
            ctx = span.get_span_context() if span is not None else None
            if ctx is not None and getattr(ctx, "is_valid", False):
                record.trace_id = format(ctx.trace_id, "032x")
                record.span_id = format(ctx.span_id, "016x")
            else:
                for k in ("trace_id", "span_id"):
                    if hasattr(record, k):
                        delattr(record, k)
        except Exception:
            for k in ("trace_id", "span_id"):
                if hasattr(record, k):
                    with suppress(Exception):
                        delattr(record, k)
        return True


class CanonicalJsonFormatter(JsonFormatter):
    CANONICAL_ORDER: ClassVar[tuple[str, ...]] = (
        "time",
        "level",
        "msg",
        "service",
        "version",
        "build_time",
        "request_id",
        "trace_id",
        "span_id",
        "method",
        "path",
        "raw_path",
        "status",
        "ip",
        "latency_ms",
        "bytes_written",
        "user_agent",
        "error",
        "error_message",
        "errors",
        "stack_trace",
    )

    def add_fields(
        self,
        log_record: dict[str, Any],
        record: logging.LogRecord,
        message_dict: dict[str, Any],
    ) -> None:
        super().add_fields(log_record, record, message_dict)

        log_record["time"] = (
            datetime.fromtimestamp(record.created, tz=UTC)
            .isoformat(timespec="microseconds")
            .replace("+00:00", "Z")
        )

        lvl = record.levelname
        log_record["level"] = "WARN" if lvl == "WARNING" else lvl
        log_record["msg"] = record.getMessage()

        for k in ("asctime", "levelname", "message", "name"):
            log_record.pop(k, None)

    def process_log_record(self, log_record: dict[str, Any]) -> dict[str, Any]:
        for k in ("request_id", "trace_id", "span_id"):
            v = log_record.get(k)
            if v in ("", "-", None) or (isinstance(v, str) and v.strip() == ""):
                log_record.pop(k, None)

        out: dict[str, Any] = {}
        for k in self.CANONICAL_ORDER:
            if k in log_record:
                out[k] = log_record[k]
        for k, v in log_record.items():
            if k not in out:
                out[k] = v
        return out


LOGGING = {
    "version": 1,
    "disable_existing_loggers": False,
    "filters": {
        "service_meta": {"()": ServiceMetaLogFilter},
        "request_id": {"()": RequestIdLogFilter},
        "trace_ctx": {"()": TraceContextLogFilter},
    },
    "formatters": {
        "json": {"()": CanonicalJsonFormatter, "fmt": "%(message)s"},
    },
    "handlers": {
        "console": {
            "class": "logging.StreamHandler",
            "formatter": "json",
            "filters": ["service_meta", "request_id", "trace_ctx"],
            "stream": "ext://sys.stdout",
        },
    },
    "root": {"handlers": ["console"], "level": LOG_LEVEL},
    "loggers": {
        "gunicorn.error": {
            "handlers": ["console"],
            "level": LOG_LEVEL,
            "propagate": False,
        },
        "gunicorn.access": {"handlers": [], "level": "CRITICAL", "propagate": False},
        "django": {"handlers": ["console"], "level": LOG_LEVEL, "propagate": False},
        "infra": {"handlers": ["console"], "level": LOG_LEVEL, "propagate": False},
        "http": {"handlers": ["console"], "level": LOG_LEVEL, "propagate": False},
        "django.request": {
            "handlers": ["console"],
            "level": "ERROR",
            "propagate": False,
        },
    },
}


def configure_logging_if_needed() -> None:
    if logging.getLogger().handlers:
        return
    logging.config.dictConfig(LOGGING)
