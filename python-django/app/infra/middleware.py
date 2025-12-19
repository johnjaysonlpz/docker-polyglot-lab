import logging
import time

from django.utils.deprecation import MiddlewareMixin
from django.urls import resolve

from django_app import settings
from . import metrics

log = logging.getLogger("http")

INFRA_PATHS = {"/health", "/ready", "/metrics"}


class HttpLoggingAndMetricsMiddleware(MiddlewareMixin):
    def process_request(self, request):
        request._start_time = time.monotonic()
        return None

    def process_response(self, request, response):
        start = getattr(request, "_start_time", None)
        if start is None:
            return response

        duration = time.monotonic() - start

        raw_path = request.path

        try:
            match = resolve(raw_path)
            path_label = f"/{match.route}" if match.route else raw_path
        except Exception:
            path_label = raw_path

        method = request.method
        status = response.status_code

        metrics.record_http_request(
            method=method,
            path=path_label,
            status_code=status,
            duration_seconds=duration,
        )

        if raw_path in INFRA_PATHS:
            return response

        client_ip = request.META.get("REMOTE_ADDR", "")
        ua = request.META.get("HTTP_USER_AGENT", "")

        log.info(
            "http_request service=%s version=%s method=%s path=%s rawPath=%s status=%s ip=%s latencyMs=%.3f userAgent=%r",
            settings.SERVICE_NAME,
            settings.VERSION,
            method,
            path_label,
            raw_path,
            status,
            client_ip,
            duration * 1000.0,
            ua,
        )

        return response
