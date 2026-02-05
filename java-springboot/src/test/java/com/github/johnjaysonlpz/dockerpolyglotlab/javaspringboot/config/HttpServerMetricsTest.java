package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.config;

import static org.assertj.core.api.Assertions.assertThat;

import io.micrometer.core.instrument.Counter;
import io.micrometer.core.instrument.Timer;
import io.micrometer.core.instrument.simple.SimpleMeterRegistry;
import java.time.Duration;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;
import org.junit.jupiter.api.Test;

class HttpServerMetricsTest {

  @Test
  void record_createsMetersOnFirstUse_andIncrementsCounter_andRecordsTimer() {
    SimpleMeterRegistry registry = new SimpleMeterRegistry();

    AtomicInteger supplierCalls = new AtomicInteger(0);
    HttpServerMetrics metrics =
        new HttpServerMetrics(
            registry,
            () -> {
              supplierCalls.incrementAndGet();
              return Timer.builder("http_server_latency_ms");
            },
            "http_server_requests_total");

    metrics.record("GET", "/info", 200, Duration.ofMillis(12));

    assertThat(supplierCalls.get()).isEqualTo(1);

    Counter c =
        registry
            .find("http_server_requests_total")
            .tags("method", "GET", "path", "/info", "status", "200")
            .counter();
    assertThat(c).isNotNull();
    assertThat(c.count()).isEqualTo(1.0);

    Timer t =
        registry
            .find("http_server_latency_ms")
            .tags("method", "GET", "path", "/info", "status", "200")
            .timer();
    assertThat(t).isNotNull();
    assertThat(t.count()).isEqualTo(1);
    assertThat(t.totalTime(TimeUnit.MILLISECONDS)).isGreaterThanOrEqualTo(12.0);
  }

  @Test
  void record_sameKey_reusesCachedMeters_soSupplierRunsOnce_butCountsIncrease() {
    SimpleMeterRegistry registry = new SimpleMeterRegistry();

    AtomicInteger supplierCalls = new AtomicInteger(0);
    HttpServerMetrics metrics =
        new HttpServerMetrics(
            registry,
            () -> {
              supplierCalls.incrementAndGet();
              return Timer.builder("http_server_latency_ms");
            },
            "http_server_requests_total");

    metrics.record("POST", "/x", 201, Duration.ofMillis(5));
    metrics.record("POST", "/x", 201, Duration.ofMillis(7));

    assertThat(supplierCalls.get()).isEqualTo(1);

    Counter c =
        registry
            .find("http_server_requests_total")
            .tags("method", "POST", "path", "/x", "status", "201")
            .counter();
    assertThat(c).isNotNull();
    assertThat(c.count()).isEqualTo(2.0);

    Timer t =
        registry
            .find("http_server_latency_ms")
            .tags("method", "POST", "path", "/x", "status", "201")
            .timer();
    assertThat(t).isNotNull();
    assertThat(t.count()).isEqualTo(2);
    assertThat(t.totalTime(TimeUnit.MILLISECONDS)).isGreaterThanOrEqualTo(12.0);
  }

  @Test
  void record_differentKeys_createSeparateMeters_andSupplierRunsPerKey() {
    SimpleMeterRegistry registry = new SimpleMeterRegistry();

    AtomicInteger supplierCalls = new AtomicInteger(0);
    HttpServerMetrics metrics =
        new HttpServerMetrics(
            registry,
            () -> {
              supplierCalls.incrementAndGet();
              return Timer.builder("http_server_latency_ms");
            },
            "http_server_requests_total");

    metrics.record("GET", "/a", 200, Duration.ofMillis(1));
    metrics.record("GET", "/a", 500, Duration.ofMillis(2));
    metrics.record("GET", "/b", 200, Duration.ofMillis(3));

    assertThat(supplierCalls.get()).isEqualTo(3);

    Counter cA200 =
        registry
            .find("http_server_requests_total")
            .tags("method", "GET", "path", "/a", "status", "200")
            .counter();
    Counter cA500 =
        registry
            .find("http_server_requests_total")
            .tags("method", "GET", "path", "/a", "status", "500")
            .counter();
    Counter cB200 =
        registry
            .find("http_server_requests_total")
            .tags("method", "GET", "path", "/b", "status", "200")
            .counter();

    assertThat(cA200.count()).isEqualTo(1.0);
    assertThat(cA500.count()).isEqualTo(1.0);
    assertThat(cB200.count()).isEqualTo(1.0);

    Timer tA200 =
        registry
            .find("http_server_latency_ms")
            .tags("method", "GET", "path", "/a", "status", "200")
            .timer();
    Timer tA500 =
        registry
            .find("http_server_latency_ms")
            .tags("method", "GET", "path", "/a", "status", "500")
            .timer();
    Timer tB200 =
        registry
            .find("http_server_latency_ms")
            .tags("method", "GET", "path", "/b", "status", "200")
            .timer();

    assertThat(tA200.count()).isEqualTo(1);
    assertThat(tA500.count()).isEqualTo(1);
    assertThat(tB200.count()).isEqualTo(1);
  }
}
