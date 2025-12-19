import logging
from django.test import TestCase, Client
from . import readiness


def _get_header(resp, name: str) -> str | None:
    if hasattr(resp, "headers"):
        return resp.headers.get(name)
    try:
        return resp.get(name)
    except Exception:
        return None


class InfraEndpointsTest(TestCase):
    def setUp(self):
        self.client = Client()
        readiness.state.set_accepting(True)

    def test_root_returns_banner(self):
        resp = self.client.get("/")
        self.assertEqual(resp.status_code, 200)
        self.assertEqual(resp.content.decode(), "python-django-app is running (Python + Django)\n")
        self.assertTrue(_get_header(resp, "X-Request-ID"))

    def test_info_returns_metadata(self):
        resp = self.client.get("/info")
        self.assertEqual(resp.status_code, 200)
        body = resp.json()
        self.assertIn("service", body)
        self.assertIn("version", body)
        self.assertIn("buildTime", body)
        self.assertTrue(_get_header(resp, "X-Request-ID"))

    def test_health_and_ready_ok_by_default(self):
        r1 = self.client.get("/health")
        self.assertEqual(r1.status_code, 200)
        self.assertTrue(_get_header(r1, "X-Request-ID"))

        r2 = self.client.get("/ready")
        self.assertEqual(r2.status_code, 200)
        self.assertTrue(_get_header(r2, "X-Request-ID"))

    def test_ready_returns_503_when_not_accepting(self):
        readiness.state.set_accepting(False)
        resp = self.client.get("/ready")
        self.assertEqual(resp.status_code, 503)
        self.assertTrue(_get_header(resp, "X-Request-ID"))

    def test_request_id_echoes_incoming(self):
        resp = self.client.get("/info", **{"HTTP_X_REQUEST_ID": "demo-123"})
        self.assertEqual(resp.status_code, 200)
        self.assertEqual(_get_header(resp, "X-Request-ID"), "demo-123")

    def test_metrics_contains_http_requests_total_after_traffic(self):
        self.client.get("/")
        resp = self.client.get("/metrics")
        self.assertEqual(resp.status_code, 200)
        body = resp.content.decode()
        self.assertIn("http_requests_total", body)


class LoggingAndMetricsMiddlewareTest(TestCase):
    def setUp(self):
        self.client = Client()
        readiness.state.set_accepting(True)

    def test_infra_endpoints_still_record_metrics(self):
        resp = self.client.get("/health")
        self.assertEqual(resp.status_code, 200)
        resp = self.client.get("/metrics")
        self.assertEqual(resp.status_code, 200)
        self.assertIn("http_requests_total", resp.content.decode())

    def test_application_endpoints_are_logged_with_structured_fields(self):
        logger = logging.getLogger("http")

        class CapturingHandler(logging.Handler):
            def __init__(self):
                super().__init__()
                self.records = []

            def emit(self, record):
                self.records.append(record)

        handler = CapturingHandler()
        logger.addHandler(handler)
        try:
            resp = self.client.get("/", **{"HTTP_X_REQUEST_ID": "rid-test"})
            self.assertEqual(resp.status_code, 200)
            self.assertEqual(_get_header(resp, "X-Request-ID"), "rid-test")

            records = handler.records
            self.assertTrue(records, "Expected at least one log record")

            http_recs = [r for r in records if r.getMessage() == "http_request"]
            self.assertTrue(http_recs, f"No 'http_request' log record found. Got: {[r.getMessage() for r in records]}")

            r = http_recs[-1]
            self.assertEqual(getattr(r, "request_id", None), "rid-test")
            self.assertEqual(getattr(r, "method", None), "GET")
            self.assertEqual(getattr(r, "status", None), 200)
            self.assertEqual(getattr(r, "path", None), "/")
        finally:
            logger.removeHandler(handler)

    def test_404_metrics_use_unmatched_label(self):
        self.client.get("/does-not-exist", **{"HTTP_X_REQUEST_ID": "rid-404"})
        resp = self.client.get("/metrics")
        body = resp.content.decode()

        self.assertIn('path="__unmatched__"', body)
