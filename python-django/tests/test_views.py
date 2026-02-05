import json

from django.test import Client

from infra import readiness


def test_root_info_health_ready_metrics(client: Client) -> None:
    r = client.get("/")
    assert r.status_code == 200
    assert r["Content-Type"].startswith("text/plain")
    assert b"python-django-app is running" in r.content

    r = client.get("/info")
    assert r.status_code == 200
    payload = json.loads(r.content)
    assert payload["service"] == "python-django-app"
    assert payload["version"] == "test"
    assert payload["build_time"] == "test-time"

    assert client.get("/health").status_code == 200

    readiness.state.set_accepting(True)
    assert client.get("/ready").status_code == 200
    readiness.state.set_accepting(False)
    assert client.get("/ready").status_code == 503
    readiness.state.set_accepting(True)

    r = client.get("/metrics")
    assert r.status_code == 200
    assert "text/plain" in r["Content-Type"]
    assert b"http_requests_total" in r.content


def test_require_get_returns_405(client: Client) -> None:
    r = client.post("/health")
    assert r.status_code == 405
    assert r["Allow"] == "GET"
    payload = json.loads(r.content)
    assert payload["code"] == "method_not_allowed"
