package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.config;

import jakarta.validation.constraints.Max;
import jakarta.validation.constraints.Min;
import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.NotNull;
import java.time.Duration;
import org.hibernate.validator.constraints.time.DurationMin;
import org.springframework.boot.context.properties.ConfigurationProperties;
import org.springframework.validation.annotation.Validated;

@ConfigurationProperties(prefix = "app")
@Validated
public class ServiceProperties {

  private String serviceName = "java-springboot-app";
  private String version = "0.0.0-dev";
  private String buildTime = "unknown";

  @NotBlank private String host = "0.0.0.0";

  @Min(1)
  @Max(65535)
  private int port = 8080;

  @NotNull
  @DurationMin(seconds = 1)
  private Duration shutdownTimeout = Duration.ofSeconds(5);

  @NotNull
  @DurationMin(seconds = 1)
  private Duration tomcatConnectionTimeout = Duration.ofSeconds(5);

  @NotNull
  @DurationMin(seconds = 1)
  private Duration tomcatKeepAliveTimeout = Duration.ofSeconds(120);

  @Min(1)
  private int tomcatMaxConnections = 8192;

  @Min(1)
  private int tomcatAcceptCount = 100;

  @Min(1)
  private int tomcatMaxKeepAliveRequests = 100;

  @Min(1)
  private int tomcatMaxThreads = 200;

  @Min(1)
  private int tomcatMinSpareThreads = 10;

  @Min(0)
  private long maxBodyBytes = 1_048_576L;

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

  public Duration getShutdownTimeout() {
    return shutdownTimeout;
  }

  public void setShutdownTimeout(Duration shutdownTimeout) {
    this.shutdownTimeout = shutdownTimeout;
  }

  public Duration getTomcatConnectionTimeout() {
    return tomcatConnectionTimeout;
  }

  public void setTomcatConnectionTimeout(Duration tomcatConnectionTimeout) {
    this.tomcatConnectionTimeout = tomcatConnectionTimeout;
  }

  public Duration getTomcatKeepAliveTimeout() {
    return tomcatKeepAliveTimeout;
  }

  public void setTomcatKeepAliveTimeout(Duration tomcatKeepAliveTimeout) {
    this.tomcatKeepAliveTimeout = tomcatKeepAliveTimeout;
  }

  public int getTomcatMaxConnections() {
    return tomcatMaxConnections;
  }

  public void setTomcatMaxConnections(int tomcatMaxConnections) {
    this.tomcatMaxConnections = tomcatMaxConnections;
  }

  public int getTomcatAcceptCount() {
    return tomcatAcceptCount;
  }

  public void setTomcatAcceptCount(int tomcatAcceptCount) {
    this.tomcatAcceptCount = tomcatAcceptCount;
  }

  public int getTomcatMaxKeepAliveRequests() {
    return tomcatMaxKeepAliveRequests;
  }

  public void setTomcatMaxKeepAliveRequests(int tomcatMaxKeepAliveRequests) {
    this.tomcatMaxKeepAliveRequests = tomcatMaxKeepAliveRequests;
  }

  public int getTomcatMaxThreads() {
    return tomcatMaxThreads;
  }

  public void setTomcatMaxThreads(int tomcatMaxThreads) {
    this.tomcatMaxThreads = tomcatMaxThreads;
  }

  public int getTomcatMinSpareThreads() {
    return tomcatMinSpareThreads;
  }

  public void setTomcatMinSpareThreads(int tomcatMinSpareThreads) {
    this.tomcatMinSpareThreads = tomcatMinSpareThreads;
  }

  public long getMaxBodyBytes() {
    return maxBodyBytes;
  }

  public void setMaxBodyBytes(long maxBodyBytes) {
    this.maxBodyBytes = maxBodyBytes;
  }
}
