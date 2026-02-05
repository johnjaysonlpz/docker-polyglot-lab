package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.config;

import static org.assertj.core.api.Assertions.assertThat;

import java.time.Duration;
import org.junit.jupiter.api.Test;

class ServicePropertiesTest {

  @Test
  void defaultValues_andMissingGetters_areCovered() {
    ServiceProperties p = new ServiceProperties();

    assertThat(p.getServiceName()).isEqualTo("java-springboot-app");
    assertThat(p.getShutdownTimeout()).isEqualTo(Duration.ofSeconds(5));
    assertThat(p.getTomcatConnectionTimeout()).isEqualTo(Duration.ofSeconds(5));
    assertThat(p.getTomcatKeepAliveTimeout()).isEqualTo(Duration.ofSeconds(120));
    assertThat(p.getTomcatMaxConnections()).isEqualTo(8192);
    assertThat(p.getTomcatAcceptCount()).isEqualTo(100);
    assertThat(p.getTomcatMaxKeepAliveRequests()).isEqualTo(100);
    assertThat(p.getTomcatMaxThreads()).isEqualTo(200);
    assertThat(p.getTomcatMinSpareThreads()).isEqualTo(10);
    assertThat(p.getMaxBodyBytes()).isEqualTo(1_048_576L);

    assertThat(p.getVersion()).isEqualTo("0.0.0-dev");
    assertThat(p.getBuildTime()).isEqualTo("unknown");
    assertThat(p.getHost()).isEqualTo("0.0.0.0");
    assertThat(p.getPort()).isEqualTo(8080);
  }
}
