from prometheus_client import (
    CollectorRegistry,
    Counter,
    Histogram,
    Gauge,
    generate_latest,
    CONTENT_TYPE_LATEST,
)
from django_app import settings

_registry = CollectorRegistry()

HTTP_REQUESTS_TOTAL = Counter(
    "http_requests_total",
    "Total number of HTTP requests processed.",
    ["service", "method", "path", "status"],
    registry=_registry,
)

HTTP_REQUEST_DURATION_SECONDS = Histogram(
    "http_request_duration_seconds",
    "HTTP request latencies in seconds.",
    ["service", "method", "path", "status"],
    registry=_registry,
)

BUILD_INFO = Gauge(
    "build_info",
    "Build information for the service.",
    ["service", "version", "build_time"],
    registry=_registry,
)

BUILD_INFO.labels(
    service=settings.APP_SERVICE_NAME,
    version=settings.APP_VERSION,
    build_time=settings.APP_BUILD_TIME,
).set(1)


def record_http_request(method: str, path: str, status_code: int, duration_seconds: float) -> None:
    status = str(status_code)
    labels = {
        "service": settings.APP_SERVICE_NAME,
        "method": method,
        "path": path,
        "status": status,
    }
    HTTP_REQUESTS_TOTAL.labels(**labels).inc()
    HTTP_REQUEST_DURATION_SECONDS.labels(**labels).observe(duration_seconds)


def scrape_metrics() -> tuple[bytes, str]:
    output = generate_latest(_registry)
    return output, CONTENT_TYPE_LATEST
