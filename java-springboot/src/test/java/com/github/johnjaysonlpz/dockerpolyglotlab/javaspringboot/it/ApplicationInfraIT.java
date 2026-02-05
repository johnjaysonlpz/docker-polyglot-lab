package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.it;

import static org.assertj.core.api.Assertions.assertThat;

import com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.web.RequestIdFilter;
import java.time.Duration;
import java.util.Map;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.web.server.LocalServerPort;
import org.springframework.http.*;
import org.springframework.http.client.SimpleClientHttpRequestFactory;
import org.springframework.web.client.HttpStatusCodeException;
import org.springframework.web.client.RestTemplate;
import tools.jackson.databind.ObjectMapper;

@SpringBootTest(
    webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT,
    properties = {"spring.main.banner-mode=off", "logging.level.root=OFF"})
class ApplicationInfraIT {

  @LocalServerPort private int port;

  @Autowired private ObjectMapper objectMapper;

  private RestTemplate restTemplate() {
    SimpleClientHttpRequestFactory f = new SimpleClientHttpRequestFactory();
    f.setConnectTimeout((int) Duration.ofSeconds(2).toMillis());
    f.setReadTimeout((int) Duration.ofSeconds(5).toMillis());
    return new RestTemplate(f);
  }

  private String url(String path) {
    return "http://127.0.0.1:" + port + path;
  }

  @Test
  void infraEndpoints_work_and_requestId_isSet() throws Exception {
    RestTemplate rt = restTemplate();

    ResponseEntity<Void> health = rt.getForEntity(url("/health"), Void.class);
    assertThat(health.getStatusCode()).isEqualTo(HttpStatus.OK);

    ResponseEntity<Void> ready = rt.getForEntity(url("/ready"), Void.class);
    assertThat(ready.getStatusCode()).isEqualTo(HttpStatus.OK);

    ResponseEntity<String> info = rt.getForEntity(url("/info"), String.class);
    assertThat(info.getStatusCode()).isEqualTo(HttpStatus.OK);
    assertThat(info.getHeaders().getFirst(RequestIdFilter.HEADER_NAME)).isNotBlank();

    @SuppressWarnings("unchecked")
    Map<String, Object> m = objectMapper.readValue(info.getBody(), Map.class);
    assertThat(m).containsKeys("service", "version", "build_time");
  }

  @Test
  void requestId_is_set_on_all_infra_endpoints() {
    RestTemplate rt = restTemplate();

    ResponseEntity<Void> health = rt.getForEntity(url("/health"), Void.class);
    assertThat(health.getStatusCode()).isEqualTo(HttpStatus.OK);
    assertThat(health.getHeaders().getFirst(RequestIdFilter.HEADER_NAME)).isNotBlank();

    ResponseEntity<Void> ready = rt.getForEntity(url("/ready"), Void.class);
    assertThat(ready.getStatusCode()).isEqualTo(HttpStatus.OK);
    assertThat(ready.getHeaders().getFirst(RequestIdFilter.HEADER_NAME)).isNotBlank();

    ResponseEntity<String> metrics = rt.getForEntity(url("/metrics"), String.class);
    assertThat(metrics.getStatusCode()).isEqualTo(HttpStatus.OK);
    assertThat(metrics.getHeaders().getFirst(RequestIdFilter.HEADER_NAME)).isNotBlank();
  }

  @Test
  void missingRoute_returnsJsonError_and_propagates_requestId() throws Exception {
    RestTemplate rt = restTemplate();

    HttpHeaders headers = new HttpHeaders();
    headers.set(RequestIdFilter.HEADER_NAME, "it-rid-123");
    HttpEntity<Void> req = new HttpEntity<>(headers);

    try {
      rt.exchange(url("/definitely-not-a-route"), HttpMethod.GET, req, String.class);
      throw new AssertionError("expected 404");
    } catch (HttpStatusCodeException ex) {
      assertThat(ex.getStatusCode()).isEqualTo(HttpStatus.NOT_FOUND);

      HttpHeaders resHeaders = ex.getResponseHeaders();
      assertThat(resHeaders).isNotNull();
      assertThat(resHeaders.getFirst(RequestIdFilter.HEADER_NAME)).isEqualTo("it-rid-123");
      assertThat(resHeaders.getContentType()).isEqualTo(MediaType.APPLICATION_JSON);

      String json = ex.getResponseBodyAsString();
      assertThat(json).isNotBlank();

      @SuppressWarnings("unchecked")
      Map<String, Object> body = objectMapper.readValue(json, Map.class);

      assertThat(body.get("code")).isEqualTo("not_found");
      assertThat(body.get("error")).isEqualTo("not found");
      assertThat(body.get("request_id")).isEqualTo("it-rid-123");
    }
  }

  @Test
  void metrics_endpoint_is_prometheusish_and_contains_expected_metric_families() {
    RestTemplate rt = restTemplate();

    ResponseEntity<String> warmup = rt.getForEntity(url("/info"), String.class);
    assertThat(warmup.getStatusCode()).isEqualTo(HttpStatus.OK);

    ResponseEntity<String> metrics = rt.getForEntity(url("/metrics"), String.class);
    assertThat(metrics.getStatusCode()).isEqualTo(HttpStatus.OK);

    String ct = metrics.getHeaders().getFirst(HttpHeaders.CONTENT_TYPE);
    assertThat(ct).isNotBlank();
    assertThat(ct).contains("text/plain");

    String body = metrics.getBody();
    assertThat(body).isNotBlank();

    assertThat(body).contains("# HELP");
    assertThat(body).contains("http_requests_total");
    assertThat(body).contains("http_request_duration_seconds");
    assertThat(body).contains("build_info");
  }
}
