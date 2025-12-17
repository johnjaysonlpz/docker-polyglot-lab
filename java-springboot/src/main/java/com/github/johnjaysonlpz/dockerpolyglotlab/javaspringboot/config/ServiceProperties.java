package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.config;

import java.time.Duration;

import org.hibernate.validator.constraints.time.DurationMin;
import org.springframework.boot.context.properties.ConfigurationProperties;
import org.springframework.validation.annotation.Validated;

import jakarta.validation.constraints.Max;
import jakarta.validation.constraints.Min;
import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.NotNull;

@ConfigurationProperties(prefix = "app")
@Validated
public class ServiceProperties {

    private String serviceName = "java-springboot-app";
    private String version = "0.0.0-dev";
    private String buildTime = "unknown";

    @NotBlank
    private String host = "0.0.0.0";

    @Min(1)
    @Max(65535)
    private int port = 8080;

    @NotNull
    @DurationMin(seconds = 1)
    private Duration readTimeout = Duration.ofSeconds(5);

    @NotNull
    @DurationMin(seconds = 1)
    private Duration idleTimeout = Duration.ofSeconds(120);

    @NotNull
    @DurationMin(seconds = 1)
    private Duration shutdownTimeout = Duration.ofSeconds(5);

    public String getServiceName() {
        return serviceName;
    }

    public void setServiceName(String serviceName) {
        this.serviceName = serviceName;
    }

    public String getVersion() {
        return version;
    }

    public void setVersion(String version) {
        this.version = version;
    }

    public String getBuildTime() {
        return buildTime;
    }

    public void setBuildTime(String buildTime) {
        this.buildTime = buildTime;
    }

    public String getHost() {
        return host;
    }

    public void setHost(String host) {
        this.host = host;
    }

    public int getPort() {
        return port;
    }

    public void setPort(int port) {
        this.port = port;
    }

    public Duration getReadTimeout() {
        return readTimeout;
    }

    public void setReadTimeout(Duration readTimeout) {
        this.readTimeout = readTimeout;
    }

    public Duration getIdleTimeout() {
        return idleTimeout;
    }

    public void setIdleTimeout(Duration idleTimeout) {
        this.idleTimeout = idleTimeout;
    }

    public Duration getShutdownTimeout() {
        return shutdownTimeout;
    }

    public void setShutdownTimeout(Duration shutdownTimeout) {
        this.shutdownTimeout = shutdownTimeout;
    }
}
