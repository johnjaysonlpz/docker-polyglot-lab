import logging
from django.test import TestCase, Client
from . import readiness
from . import metrics as infra_metrics


class InfraEndpointsTest(TestCase):
    def setUp(self):
        self.client = Client()
        readiness.state.set_accepting(True)

    def test_root_returns_banner(self):
        resp = self.client.get("/")
        self.assertEqual(resp.status_code, 200)
        self.assertEqual(
            resp.content.decode(),
            "python-django-app is running (Python + Django)\n",
        )

    def test_info_returns_metadata(self):
        resp = self.client.get("/info")
        self.assertEqual(resp.status_code, 200)
        body = resp.json()
        self.assertIn("service", body)
        self.assertIn("version", body)
        self.assertIn("buildTime", body)

    def test_health_and_ready_ok_by_default(self):
        self.assertEqual(self.client.get("/health").status_code, 200)
        self.assertEqual(self.client.get("/ready").status_code, 200)

    def test_ready_returns_503_when_not_accepting(self):
        readiness.state.set_accepting(False)
        resp = self.client.get("/ready")
        self.assertEqual(resp.status_code, 503)

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

    def test_application_endpoints_are_logged(self):
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
            resp = self.client.get("/")
            self.assertEqual(resp.status_code, 200)

            messages = [r.getMessage() for r in handler.records]
            self.assertTrue(
                any("http_request" in msg for msg in messages),
                msg=f"No http_request log found, got: {messages}",
            )
        finally:
            logger.removeHandler(handler)
