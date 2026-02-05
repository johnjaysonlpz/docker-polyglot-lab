from __future__ import annotations

import ipaddress
import os
from pathlib import Path

from django.core.exceptions import ImproperlyConfigured

BASE_DIR = Path(__file__).resolve().parent.parent

type IPAddress = ipaddress.IPv4Address | ipaddress.IPv6Address
type IPNetwork = ipaddress.IPv4Network | ipaddress.IPv6Network


def env_str(key: str, default: str | None = None, required: bool = False) -> str:
    val = os.getenv(key, default)
    if required and (val is None or str(val).strip() == ""):
        raise ImproperlyConfigured(f"Missing required environment variable {key}")
    return "" if val is None else str(val)


def env_int(
    key: str,
    default: int | None = None,
    min_val: int | None = None,
    max_val: int | None = None,
) -> int:
    raw = os.getenv(key, None)
    if raw is None:
        if default is None:
            raise ImproperlyConfigured(f"Missing required env var {key}")
        return default
    try:
        value = int(raw)
    except ValueError as e:
        raise ImproperlyConfigured(f"{key} must be an integer, got {raw!r}") from e
    if min_val is not None and value < min_val:
        raise ImproperlyConfigured(f"{key} must be >= {min_val}, got {value}")
    if max_val is not None and value > max_val:
        raise ImproperlyConfigured(f"{key} must be <= {max_val}, got {value}")
    return value


def env_duration_seconds(key: str, default_seconds: int) -> float:
    raw = os.getenv(key, None)
    if raw is None or raw.strip() == "":
        return float(default_seconds)

    s = raw.strip().lower()
    if s.endswith("s"):
        s = s[:-1]
    try:
        secs = float(s)
    except ValueError as e:
        raise ImproperlyConfigured(f"{key} must be seconds like '5s' or '5', got {raw!r}") from e
    if secs <= 0:
        raise ImproperlyConfigured(f"{key} must be > 0, got {secs}")
    return secs


def parse_trusted_proxies(raw: str) -> list[IPNetwork]:
    out: list[IPNetwork] = []
    s = (raw or "").strip()
    if not s:
        return out

    parts = [p.strip() for p in s.split(",") if p.strip()]
    for p in parts:
        try:
            net = ipaddress.ip_network(p, strict=False)
        except ValueError as e:
            raise ImproperlyConfigured(
                f"TRUSTED_PROXIES contains invalid CIDR/IP {p!r}: {e}"
            ) from e

        if isinstance(net, ipaddress.IPv4Network | ipaddress.IPv6Network):
            out.append(net)
        else:
            raise ImproperlyConfigured(f"Unsupported network type for {p!r}: {type(net)!r}")

    return out


SERVICE_NAME = env_str("SERVICE_NAME", "python-django-app")
VERSION = env_str("VERSION", "0.0.0-dev")
BUILD_TIME = env_str("BUILD_TIME", "unknown")

HOST = env_str("HOST", "0.0.0.0")
PORT = env_int("PORT", 8080, min_val=1, max_val=65535)
READ_TIMEOUT_SECONDS = env_duration_seconds("READ_TIMEOUT", 5)
IDLE_TIMEOUT_SECONDS = env_duration_seconds("IDLE_TIMEOUT", 120)
SHUTDOWN_TIMEOUT_SECONDS = env_duration_seconds("SHUTDOWN_TIMEOUT", 5)

MAX_BODY_BYTES = env_int("MAX_BODY_BYTES", 1024 * 1024, min_val=0, max_val=50 * 1024 * 1024)

DATA_UPLOAD_MAX_MEMORY_SIZE = MAX_BODY_BYTES
FILE_UPLOAD_MAX_MEMORY_SIZE = MAX_BODY_BYTES

TRUSTED_PROXIES = env_str("TRUSTED_PROXIES", "")
TRUSTED_PROXY_NETWORKS = parse_trusted_proxies(TRUSTED_PROXIES)

SECRET_KEY = env_str("DJANGO_SECRET_KEY", "insecure-dev-secret")
DEBUG = os.getenv("DEBUG", "false").lower() == "true"

ALLOWED_HOSTS = ["*"]

INSTALLED_APPS = [
    "django.contrib.contenttypes",
    "django.contrib.staticfiles",
    "infra",
]

MIDDLEWARE = [
    "infra.middleware.RequestIdMiddleware",
    "django.middleware.security.SecurityMiddleware",
    "django.middleware.common.CommonMiddleware",
    "infra.middleware.HttpLoggingAndMetricsMiddleware",
    "infra.middleware.MaxBodyBytesMiddleware",
]

ROOT_URLCONF = "django_app.urls"

TEMPLATES = [
    {
        "BACKEND": "django.template.backends.django.DjangoTemplates",
        "DIRS": [],
        "APP_DIRS": True,
        "OPTIONS": {"context_processors": []},
    }
]

WSGI_APPLICATION = "django_app.wsgi.application"

DATABASES = {
    "default": {
        "ENGINE": "django.db.backends.sqlite3",
        "NAME": BASE_DIR / "db.sqlite3",
    }
}

LANGUAGE_CODE = "en-us"
TIME_ZONE = "UTC"
USE_I18N = False
USE_TZ = True
STATIC_URL = "static/"

LOG_LEVEL = os.getenv("LOG_LEVEL", "INFO").upper()
LOGGING_CONFIG = None
