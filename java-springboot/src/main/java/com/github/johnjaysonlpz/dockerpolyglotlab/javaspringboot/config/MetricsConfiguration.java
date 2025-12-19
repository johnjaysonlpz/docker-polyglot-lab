package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.config;

import java.time.Duration;

import io.micrometer.core.instrument.Clock;
import io.micrometer.core.instrument.Gauge;
import io.micrometer.core.instrument.MeterRegistry;
import io.micrometer.core.instrument.Timer;
import io.micrometer.core.instrument.binder.MeterBinder;
import io.micrometer.prometheusmetrics.PrometheusConfig;
import io.micrometer.prometheusmetrics.PrometheusMeterRegistry;
import io.prometheus.metrics.model.registry.PrometheusRegistry;
import org.springframework.boot.actuate.autoconfigure.metrics.MeterRegistryCustomizer;
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
        return registry -> Gauge
            .builder("build_info", () -> 1)
            .description("Build information for the service.")
            .tag("build_time", props.getBuildTime())
            .register(registry);
    }

    @Bean
    HttpServerMetrics httpServerMetrics(MeterRegistry registry) {
        Timer.Builder timerBuilder = Timer.builder("http_request_duration_seconds")
            .description("HTTP request latency.")
            .publishPercentileHistogram(true)
            .minimumExpectedValue(Duration.ofMillis(1))
            .maximumExpectedValue(Duration.ofMinutes(1));

        return new HttpServerMetrics(registry, timerBuilder, "http_requests_total");
    }

    @Bean
    MeterRegistryCustomizer<MeterRegistry> meterRegistryCustomizer(ServiceProperties props) {
        return registry -> registry.config()
            .commonTags("service", props.getServiceName(), "version", props.getVersion());
    }
}
