package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.web;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

import com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.config.ServiceProperties;
import java.util.Map;
import org.junit.jupiter.api.Test;
import org.springframework.boot.availability.ApplicationAvailability;
import org.springframework.boot.availability.LivenessState;
import org.springframework.boot.availability.ReadinessState;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;

class InfraControllerTest {

  @Test
  void root_returnsOkMessage() {
    ServiceProperties props = mock(ServiceProperties.class);
    ApplicationAvailability availability = mock(ApplicationAvailability.class);
    InfraController c = new InfraController(props, availability);

    ResponseEntity<String> res = c.root();

    assertThat(res.getStatusCode()).isEqualTo(HttpStatus.OK);
    assertThat(res.getBody()).isEqualTo("java-springboot-app is running (Java + Spring Boot)\n");
  }

  @Test
  void info_returnsServiceVersionBuildTimeFromProps() {
    ServiceProperties props = mock(ServiceProperties.class);
    ApplicationAvailability availability = mock(ApplicationAvailability.class);

    when(props.getServiceName()).thenReturn("svc");
    when(props.getVersion()).thenReturn("1.2.3");
    when(props.getBuildTime()).thenReturn("2026-01-30T00:00:00Z");

    InfraController c = new InfraController(props, availability);

    Map<String, String> out = c.info();

    assertThat(out)
        .containsEntry("service", "svc")
        .containsEntry("version", "1.2.3")
        .containsEntry("build_time", "2026-01-30T00:00:00Z");
  }

  @Test
  void health_whenLivenessCorrect_returns200() {
    ServiceProperties props = mock(ServiceProperties.class);
    ApplicationAvailability availability = mock(ApplicationAvailability.class);
    when(availability.getLivenessState()).thenReturn(LivenessState.CORRECT);

    InfraController c = new InfraController(props, availability);

    ResponseEntity<Void> res = c.health();

    assertThat(res.getStatusCode()).isEqualTo(HttpStatus.OK);
  }

  @Test
  void health_whenLivenessNotCorrect_returns500() {
    ServiceProperties props = mock(ServiceProperties.class);
    ApplicationAvailability availability = mock(ApplicationAvailability.class);
    when(availability.getLivenessState()).thenReturn(LivenessState.BROKEN);

    InfraController c = new InfraController(props, availability);

    ResponseEntity<Void> res = c.health();

    assertThat(res.getStatusCode()).isEqualTo(HttpStatus.INTERNAL_SERVER_ERROR);
  }

  @Test
  void ready_whenAcceptingTraffic_returns200() {
    ServiceProperties props = mock(ServiceProperties.class);
    ApplicationAvailability availability = mock(ApplicationAvailability.class);
    when(availability.getReadinessState()).thenReturn(ReadinessState.ACCEPTING_TRAFFIC);

    InfraController c = new InfraController(props, availability);

    ResponseEntity<Void> res = c.ready();

    assertThat(res.getStatusCode()).isEqualTo(HttpStatus.OK);
  }

  @Test
  void ready_whenNotAcceptingTraffic_returns503() {
    ServiceProperties props = mock(ServiceProperties.class);
    ApplicationAvailability availability = mock(ApplicationAvailability.class);
    when(availability.getReadinessState()).thenReturn(ReadinessState.REFUSING_TRAFFIC);

    InfraController c = new InfraController(props, availability);

    ResponseEntity<Void> res = c.ready();

    assertThat(res.getStatusCode()).isEqualTo(HttpStatus.SERVICE_UNAVAILABLE);
  }
}
