package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.config;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

import io.micrometer.core.instrument.Gauge;
import io.micrometer.core.instrument.Timer;
import io.micrometer.core.instrument.binder.MeterBinder;
import io.micrometer.core.instrument.simple.SimpleMeterRegistry;
import io.micrometer.prometheusmetrics.PrometheusMeterRegistry;
import io.prometheus.metrics.model.registry.PrometheusRegistry;
import java.time.Duration;
import java.util.concurrent.TimeUnit;
import org.junit.jupiter.api.Test;

class MetricsConfigurationTest {

  @Test
  void buildInfoMeter_bindsGauge_withTags_andGaugeSupplierIsExecuted() {
    ServiceProperties props = mock(ServiceProperties.class);
    when(props.getVersion()).thenReturn("1.2.3");
    when(props.getBuildTime()).thenReturn("2026-01-30T00:00:00Z");

    MetricsConfiguration cfg = new MetricsConfiguration();

    MeterBinder binder = cfg.buildInfoMeter(props);
    SimpleMeterRegistry registry = new SimpleMeterRegistry();

    binder.bindTo(registry);

    Gauge g =
        registry
            .find("build.info")
            .tag("version", "1.2.3")
            .tag("build_time", "2026-01-30T00:00:00Z")
            .gauge();

    assertThat(g).isNotNull();

    assertThat(g.value()).isEqualTo(1.0d);
  }

  @Test
  void httpServerMetrics_timerBuilderSupplier_lambdaIsExecuted_onRecord() {
    MetricsConfiguration cfg = new MetricsConfiguration();
    SimpleMeterRegistry registry = new SimpleMeterRegistry();

    HttpServerMetrics metrics = cfg.httpServerMetrics(registry);

    metrics.record("GET", "/x", 200, Duration.ofMillis(12));

    assertThat(
            registry
                .find("http_requests_total")
                .tags("method", "GET", "path", "/x", "status", "200")
                .counter())
        .isNotNull();

    Timer t =
        registry
            .find("http_request_duration")
            .tags("method", "GET", "path", "/x", "status", "200")
            .timer();

    assertThat(t).isNotNull();
    assertThat(t.count()).isEqualTo(1);
    assertThat(t.totalTime(TimeUnit.MILLISECONDS)).isGreaterThanOrEqualTo(12.0);
  }

  @Test
  void prometheusBeans_createRegistryAndMeterRegistry() {
    MetricsConfiguration cfg = new MetricsConfiguration();

    PrometheusRegistry pr = cfg.prometheusRegistry();
    assertThat(pr).isNotNull();

    PrometheusMeterRegistry pmr = cfg.prometheusMeterRegistry(pr);
    assertThat(pmr).isNotNull();
  }
}
