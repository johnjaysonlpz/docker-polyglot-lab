import ipaddress

import pytest
from django.core.exceptions import ImproperlyConfigured

from django_app import settings as s


def test_env_str_required_missing(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("REQ", raising=False)
    with pytest.raises(ImproperlyConfigured):
        s.env_str("REQ", required=True)


def test_env_str_required_rejects_blank(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("REQ", "   ")
    with pytest.raises(ImproperlyConfigured):
        s.env_str("REQ", required=True)


def test_env_str_default(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("X", raising=False)
    assert s.env_str("X", "abc") == "abc"


def test_env_str_returns_empty_string_when_missing_and_no_default(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.delenv("MISSING", raising=False)
    assert s.env_str("MISSING") == ""


def test_env_int_required_missing(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("REQINT", raising=False)
    with pytest.raises(ImproperlyConfigured):
        s.env_int("REQINT")


def test_env_int_default_bounds_and_bad_value(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("PORTX", raising=False)
    assert s.env_int("PORTX", 5, min_val=1, max_val=10) == 5

    monkeypatch.setenv("PORTX", "0")
    with pytest.raises(ImproperlyConfigured):
        s.env_int("PORTX", 5, min_val=1)

    monkeypatch.setenv("PORTX", "11")
    with pytest.raises(ImproperlyConfigured):
        s.env_int("PORTX", 5, max_val=10)

    monkeypatch.setenv("PORTX", "nope")
    with pytest.raises(ImproperlyConfigured):
        s.env_int("PORTX", 5)


def test_env_int_success_path_returns_value(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("PORTX", "7")
    assert s.env_int("PORTX", default=5, min_val=1, max_val=10) == 7


def test_env_duration_seconds(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("DUR", raising=False)
    assert s.env_duration_seconds("DUR", 5) == 5.0

    monkeypatch.setenv("DUR", "2s")
    assert s.env_duration_seconds("DUR", 5) == 2.0

    monkeypatch.setenv("DUR", "2")
    assert s.env_duration_seconds("DUR", 5) == 2.0

    monkeypatch.setenv("DUR", "0")
    with pytest.raises(ImproperlyConfigured):
        s.env_duration_seconds("DUR", 5)

    monkeypatch.setenv("DUR", "nope")
    with pytest.raises(ImproperlyConfigured):
        s.env_duration_seconds("DUR", 5)


def test_parse_trusted_proxies(monkeypatch: pytest.MonkeyPatch) -> None:
    assert s.parse_trusted_proxies("") == []
    nets = s.parse_trusted_proxies("10.0.0.0/8, 192.168.1.1")
    assert len(nets) == 2

    with pytest.raises(ImproperlyConfigured):
        s.parse_trusted_proxies("not-a-cidr")


def test_parse_trusted_proxies_unsupported_network_type(monkeypatch: pytest.MonkeyPatch) -> None:
    class _WeirdNet:
        pass

    monkeypatch.setattr(ipaddress, "ip_network", lambda *_a, **_k: _WeirdNet())
    with pytest.raises(ImproperlyConfigured):
        s.parse_trusted_proxies("10.0.0.0/8")
