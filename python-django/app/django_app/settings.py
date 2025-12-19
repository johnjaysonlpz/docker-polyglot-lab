import os
import ipaddress
import logging
from pathlib import Path
from django.core.exceptions import ImproperlyConfigured
from infra.request_id import get_request_id

BASE_DIR = Path(__file__).resolve().parent.parent


def env_str(key: str, default: str | None = None, required: bool = False) -> str:
    val = os.getenv(key, default)
    if required and (val is None or str(val).strip() == ""):
        raise ImproperlyConfigured(f"Missing required environment variable {key}")
    return val


def env_int(key: str, default: int | None = None, min_val: int | None = None, max_val: int | None = None) -> int:
    raw = os.getenv(key, None)
    if raw is None:
        if default is None:
            raise ImproperlyConfigured(f"Missing required env var {key}")
        return default
    try:
        value = int(raw)
    except ValueError:
        raise ImproperlyConfigured(f"{key} must be an integer, got {raw!r}")
    if min_val is not None and value < min_val:
        raise ImproperlyConfigured(f"{key} must be >= {min_val}, got {value}")
    if max_val is not None and value > max_val:
        raise ImproperlyConfigured(f"{key} must be <= {max_val}, got {value}")
    return value


def env_duration_seconds(key: str, default_seconds: int) -> float:
    """
    Parse simple duration like '5s', '120s' or raw seconds as int.
    Keep it simple for now to avoid extra deps.
    """
    raw = os.getenv(key, None)
    if raw is None or raw.strip() == "":
        return float(default_seconds)

    s = raw.strip().lower()
    if s.endswith("s"):
        s = s[:-1]
    try:
        secs = float(s)
    except ValueError:
        raise ImproperlyConfigured(f"{key} must be seconds like '5s' or '5', got {raw!r}")
    if secs <= 0:
        raise ImproperlyConfigured(f"{key} must be > 0, got {secs}")
    return secs


def parse_trusted_proxies(raw: str) -> list[ipaddress._BaseNetwork]:
    """
    TRUSTED_PROXIES="10.0.0.0/8,172.16.0.0/12,192.168.1.10/32"
    """
    out: list[ipaddress._BaseNetwork] = []
    s = (raw or "").strip()
    if not s:
        return out

    parts = [p.strip() for p in s.split(",") if p.strip()]
    for p in parts:
        try:
            out.append(ipaddress.ip_network(p, strict=False))
        except ValueError as e:
            raise ImproperlyConfigured(f"TRUSTED_PROXIES contains invalid CIDR/IP {p!r}: {e}") from e
    return out


SERVICE_NAME = env_str("SERVICE_NAME", "python-django-app")
VERSION = env_str("VERSION", "0.0.0-dev")
BUILD_TIME = env_str("BUILD_TIME", "unknown")

HOST = env_str("HOST", "0.0.0.0")
PORT = env_int("PORT", 8080, min_val=1, max_val=65535)
READ_TIMEOUT_SECONDS = env_duration_seconds("READ_TIMEOUT", 5)
IDLE_TIMEOUT_SECONDS = env_duration_seconds("IDLE_TIMEOUT", 120)
SHUTDOWN_TIMEOUT_SECONDS = env_duration_seconds("SHUTDOWN_TIMEOUT", 5)

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

class RequestIdLogFilter(logging.Filter):
    def filter(self, record: logging.LogRecord) -> bool:
        rid = getattr(record, "request_id", None)
        if not rid or str(rid).strip() == "":
            record.request_id = get_request_id()
        return True

LOGGING = {
    "version": 1,
    "disable_existing_loggers": False,

    "filters": {
        "request_id": {"()": "django_app.settings.RequestIdLogFilter"},
    },

    "formatters": {
        "json": {
            "()": "pythonjsonlogger.jsonlogger.JsonFormatter",
            "fmt": "%(asctime)s %(levelname)s %(name)s %(message)s",
        },
    },

    "handlers": {
        "console": {
            "class": "logging.StreamHandler",
            "formatter": "json",
            "filters": ["request_id"],
        },
    },

    "root": {"handlers": ["console"], "level": LOG_LEVEL},

    "loggers": {
        "django": {"handlers": ["console"], "level": LOG_LEVEL, "propagate": False},
        "infra": {"handlers": ["console"], "level": LOG_LEVEL, "propagate": False},
        "http": {"handlers": ["console"], "level": LOG_LEVEL, "propagate": False},
        "django.request": {"handlers": ["console"], "level": "ERROR", "propagate": False},
    },
}
