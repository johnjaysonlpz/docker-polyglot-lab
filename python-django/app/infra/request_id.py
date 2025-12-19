from __future__ import annotations

from contextvars import ContextVar

_request_id: ContextVar[str] = ContextVar("request_id", default="-")


def get_request_id() -> str:
    rid = _request_id.get()
    return rid if rid else "-"


def set_request_id(rid: str):
    return _request_id.set(rid if rid else "-")


def reset_request_id(token) -> None:
    _request_id.reset(token)
