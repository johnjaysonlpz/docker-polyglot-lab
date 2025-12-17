package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.config;

import org.junit.jupiter.api.Test;
import org.springframework.boot.autoconfigure.context.ConfigurationPropertiesAutoConfiguration;
import org.springframework.boot.autoconfigure.context.PropertyPlaceholderAutoConfiguration;
import org.springframework.boot.autoconfigure.validation.ValidationAutoConfiguration;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.boot.context.properties.bind.validation.BindValidationException;
import org.springframework.boot.test.context.runner.ApplicationContextRunner;
import org.springframework.context.annotation.Configuration;

import static org.assertj.core.api.Assertions.assertThat;

class ServicePropertiesTest {

    private final ApplicationContextRunner contextRunner = new ApplicationContextRunner()
        .withConfiguration(org.springframework.boot.autoconfigure.AutoConfigurations.of(
            PropertyPlaceholderAutoConfiguration.class,
            ConfigurationPropertiesAutoConfiguration.class,
            ValidationAutoConfiguration.class
        ))
        .withUserConfiguration(TestConfig.class);

    @Configuration
    @EnableConfigurationProperties(ServiceProperties.class)
    static class TestConfig { }

    @Test
    void bindsDefaultsWhenUnset() {
        contextRunner.run(context -> {
            assertThat(context).hasNotFailed();
            ServiceProperties props = context.getBean(ServiceProperties.class);
            assertThat(props.getServiceName()).isEqualTo("java-springboot-app");
            assertThat(props.getVersion()).isEqualTo("0.0.0-dev");
            assertThat(props.getBuildTime()).isEqualTo("unknown");
            assertThat(props.getHost()).isEqualTo("0.0.0.0");
            assertThat(props.getPort()).isEqualTo(8080);
            assertThat(props.getReadTimeout()).hasSeconds(5);
            assertThat(props.getIdleTimeout()).hasSeconds(120);
            assertThat(props.getShutdownTimeout()).hasSeconds(5);
        });
    }

    @Test
    void bindsValidValues() {
        contextRunner
            .withPropertyValues(
                "app.service-name=test-svc",
                "app.version=1.2.3",
                "app.build-time=2024-01-01T00:00:00Z",
                "app.host=127.0.0.1",
                "app.port=9000",
                "app.read-timeout=2s",
                "app.idle-timeout=15s",
                "app.shutdown-timeout=4s"
            )
            .run(context -> {
                assertThat(context).hasNotFailed();
                ServiceProperties props = context.getBean(ServiceProperties.class);
                assertThat(props.getServiceName()).isEqualTo("test-svc");
                assertThat(props.getVersion()).isEqualTo("1.2.3");
                assertThat(props.getBuildTime()).isEqualTo("2024-01-01T00:00:00Z");
                assertThat(props.getHost()).isEqualTo("127.0.0.1");
                assertThat(props.getPort()).isEqualTo(9000);
                assertThat(props.getReadTimeout()).hasSeconds(2);
                assertThat(props.getIdleTimeout()).hasSeconds(15);
                assertThat(props.getShutdownTimeout()).hasSeconds(4);
            });
    }

    @Test
    void failsValidationOnBadValues() {
        contextRunner
            .withPropertyValues(
                "app.port=70000",
                "app.host=   ",
                "app.read-timeout=0s"
            )
            .run(context -> {
                assertThat(context).hasFailed();
                assertThat(context.getStartupFailure().getCause())
                    .isInstanceOfSatisfying(org.springframework.boot.context.properties.bind.BindException.class, ex -> {
                        assertThat(ex.getCause()).isInstanceOf(BindValidationException.class);
                    });
            });
    }
}
