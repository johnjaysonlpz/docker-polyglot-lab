import pytest


@pytest.fixture(autouse=True)
def _default_env(monkeypatch: pytest.MonkeyPatch, settings) -> None:
    monkeypatch.setenv("SERVICE_NAME", "python-django-app")
    monkeypatch.setenv("VERSION", "test")
    monkeypatch.setenv("BUILD_TIME", "test-time")
    monkeypatch.setenv("DJANGO_SECRET_KEY", "test-secret")
    monkeypatch.setenv("DEBUG", "false")
    monkeypatch.setenv("LOG_LEVEL", "INFO")
    monkeypatch.setenv("MAX_BODY_BYTES", "1048576")
    monkeypatch.delenv("TRUSTED_PROXIES", raising=False)
    monkeypatch.delenv("PORT", raising=False)

    settings.SERVICE_NAME = "python-django-app"
    settings.VERSION = "test"
    settings.BUILD_TIME = "test-time"
    settings.MAX_BODY_BYTES = 1048576
    settings.TRUSTED_PROXY_NETWORKS = []


@pytest.fixture
def request_id_header_name() -> str:
    return "X-Request-ID"
