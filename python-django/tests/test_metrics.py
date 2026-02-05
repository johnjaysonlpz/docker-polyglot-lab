from infra import metrics


def test_record_http_request_and_scrape_metrics() -> None:
    metrics.record_http_request("GET", "/x", 200, 0.01)
    body, content_type = metrics.scrape_metrics()
    assert "text/plain" in content_type
    assert b"http_requests_total" in body
