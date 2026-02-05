from django.conf import settings
from prometheus_client import (
    CONTENT_TYPE_LATEST,
    CollectorRegistry,
    Counter,
    Gauge,
    Histogram,
    generate_latest,
)
from prometheus_client.gc_collector import GCCollector
from prometheus_client.platform_collector import PlatformCollector
from prometheus_client.process_collector import ProcessCollector

_registry = CollectorRegistry()

ProcessCollector(registry=_registry)
PlatformCollector(registry=_registry)
GCCollector(registry=_registry)

HTTP_LATENCY_BUCKETS_SECONDS = (
    0.005,
    0.01,
    0.025,
    0.05,
    0.1,
    0.25,
    0.5,
    1.0,
    2.5,
    5.0,
    10.0,
)

HTTP_REQUESTS_TOTAL = Counter(
    "http_requests_total",
    "Total number of HTTP requests processed.",
    ["method", "path", "status"],
    registry=_registry,
)

HTTP_REQUEST_DURATION_SECONDS = Histogram(
    "http_request_duration_seconds",
    "HTTP request latencies in seconds.",
    ["method", "path", "status"],
    buckets=HTTP_LATENCY_BUCKETS_SECONDS,
    registry=_registry,
)

BUILD_INFO = Gauge(
    "build_info",
    "Build information for the service (value always 1).",
    ["version", "build_time"],
    registry=_registry,
)

BUILD_INFO.labels(
    version=settings.VERSION,
    build_time=settings.BUILD_TIME,
).set(1)


def record_http_request(method: str, path: str, status_code: int, duration_seconds: float) -> None:
    status = str(status_code)
    labels = {"method": method, "path": path, "status": status}
    HTTP_REQUESTS_TOTAL.labels(**labels).inc()
    HTTP_REQUEST_DURATION_SECONDS.labels(**labels).observe(duration_seconds)


def scrape_metrics() -> tuple[bytes, str]:
    output = generate_latest(_registry)
    return output, CONTENT_TYPE_LATEST
