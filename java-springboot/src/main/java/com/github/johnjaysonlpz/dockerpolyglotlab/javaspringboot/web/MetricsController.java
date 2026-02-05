package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.web;

import io.micrometer.prometheusmetrics.PrometheusMeterRegistry;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;

@RestController
public class MetricsController {

  private final PrometheusMeterRegistry prometheusMeterRegistry;

  MetricsController(PrometheusMeterRegistry prometheusMeterRegistry) {
    this.prometheusMeterRegistry = prometheusMeterRegistry;
  }

  @GetMapping(value = "/metrics", produces = "text/plain; version=0.0.4; charset=utf-8")
  ResponseEntity<String> metrics() {
    return ResponseEntity.ok(prometheusMeterRegistry.scrape());
  }
}
