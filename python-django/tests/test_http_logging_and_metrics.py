from __future__ import annotations

from typing import Any

import pytest
from django.test import Client

from infra import metrics as metrics_module


def test_metrics_recorded_for_404(monkeypatch: pytest.MonkeyPatch, client: Client) -> None:
    calls: list[dict[str, Any]] = []

    def _spy(*, method: str, path: str, status_code: int, duration_seconds: float) -> None:
        calls.append(
            {
                "method": method,
                "path": path,
                "status_code": status_code,
                "duration_seconds": duration_seconds,
            }
        )

    monkeypatch.setattr(metrics_module, "record_http_request", _spy)

    r = client.get("/does-not-exist")
    assert r.status_code == 404
    assert calls
    assert calls[-1]["path"] == "__unmatched__"
