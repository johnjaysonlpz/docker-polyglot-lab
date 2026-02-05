package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.web;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

import io.micrometer.prometheusmetrics.PrometheusMeterRegistry;
import org.junit.jupiter.api.Test;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;

class MetricsControllerTest {

  @Test
  void metrics_returnsScrapeBody_with200() {
    PrometheusMeterRegistry reg = mock(PrometheusMeterRegistry.class);
    when(reg.scrape()).thenReturn("# HELP a b\n# TYPE a counter\na 1\n");

    MetricsController c = new MetricsController(reg);

    ResponseEntity<String> res = c.metrics();

    assertThat(res.getStatusCode()).isEqualTo(HttpStatus.OK);
    assertThat(res.getBody()).isEqualTo("# HELP a b\n# TYPE a counter\na 1\n");
  }
}
