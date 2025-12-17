package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.web;

import java.util.Map;

import com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.config.ReadinessStateHolder;
import com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.config.ServiceProperties;
import org.springframework.boot.availability.ApplicationAvailability;
import org.springframework.boot.availability.LivenessState;
import org.springframework.boot.availability.ReadinessState;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;

@RestController
public class InfraController {

    private final ServiceProperties props;
    private final ApplicationAvailability availability;
    private final ReadinessStateHolder readinessStateHolder;

    public InfraController(
        ServiceProperties props,
        ApplicationAvailability availability,
        ReadinessStateHolder readinessStateHolder
    ) {
        this.props = props;
        this.availability = availability;
        this.readinessStateHolder = readinessStateHolder;
    }

    @GetMapping("/")
    public ResponseEntity<String> root() {
        return ResponseEntity.ok("java-springboot-app is running (Java + Spring Boot)\n");
    }

    @GetMapping("/info")
    public Map<String, String> info() {
        return Map.of(
            "service", props.getServiceName(),
            "version", props.getVersion(),
            "buildTime", props.getBuildTime()
        );
    }

    @GetMapping("/health")
    public ResponseEntity<Void> health() {
        LivenessState state = availability.getLivenessState();
        HttpStatus status = (state == LivenessState.CORRECT)
            ? HttpStatus.OK
            : HttpStatus.INTERNAL_SERVER_ERROR;
        return ResponseEntity.status(status).build();
    }

    @GetMapping("/ready")
    public ResponseEntity<Void> ready() {
        ReadinessState state = readinessStateHolder.getState();
        HttpStatus status = (state == ReadinessState.ACCEPTING_TRAFFIC)
            ? HttpStatus.OK
            : HttpStatus.SERVICE_UNAVAILABLE;
        return ResponseEntity.status(status).build();
    }
}
