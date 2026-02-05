package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.config;

import io.micrometer.core.instrument.Clock;
import io.micrometer.core.instrument.Gauge;
import io.micrometer.core.instrument.MeterRegistry;
import io.micrometer.core.instrument.Timer;
import io.micrometer.core.instrument.binder.MeterBinder;
import io.micrometer.prometheusmetrics.PrometheusConfig;
import io.micrometer.prometheusmetrics.PrometheusMeterRegistry;
import io.prometheus.metrics.model.registry.PrometheusRegistry;
import java.time.Duration;
import java.util.function.Supplier;
import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

@Configuration
public class MetricsConfiguration {

  @Bean
  @ConditionalOnMissingBean
  PrometheusRegistry prometheusRegistry() {
    return new PrometheusRegistry();
  }

  @Bean
  @ConditionalOnMissingBean
  PrometheusMeterRegistry prometheusMeterRegistry(PrometheusRegistry registry) {
    return new PrometheusMeterRegistry(PrometheusConfig.DEFAULT, registry, Clock.SYSTEM);
  }

  @Bean
  MeterBinder buildInfoMeter(ServiceProperties props) {
    return registry ->
        Gauge.builder("build.info", () -> 1.0d)
            .description("Build information for the service (value always 1).")
            .tag("version", props.getVersion())
            .tag("build_time", props.getBuildTime())
            .register(registry);
  }

  @Bean
  HttpServerMetrics httpServerMetrics(MeterRegistry registry) {
    Supplier<Timer.Builder> timerBuilderSupplier =
        () ->
            Timer.builder("http_request_duration")
                .description("HTTP request latencies in seconds.")
                .publishPercentileHistogram(false)
                .serviceLevelObjectives(
                    Duration.ofMillis(5),
                    Duration.ofMillis(10),
                    Duration.ofMillis(25),
                    Duration.ofMillis(50),
                    Duration.ofMillis(100),
                    Duration.ofMillis(250),
                    Duration.ofMillis(500),
                    Duration.ofSeconds(1),
                    Duration.ofMillis(2500),
                    Duration.ofSeconds(5),
                    Duration.ofSeconds(10))
                .minimumExpectedValue(Duration.ofMillis(1))
                .maximumExpectedValue(Duration.ofMinutes(1));

    return new HttpServerMetrics(registry, timerBuilderSupplier, "http_requests_total");
  }
}
