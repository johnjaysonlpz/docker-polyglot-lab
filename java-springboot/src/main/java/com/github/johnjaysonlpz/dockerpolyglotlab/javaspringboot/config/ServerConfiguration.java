package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.config;

import java.net.InetAddress;
import java.net.UnknownHostException;
import java.time.Duration;
import org.apache.catalina.connector.Connector;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.boot.tomcat.servlet.TomcatServletWebServerFactory;
import org.springframework.boot.web.server.WebServerFactoryCustomizer;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

@Configuration
public class ServerConfiguration {

  private static final Logger log = LoggerFactory.getLogger(ServerConfiguration.class);

  @Bean
  WebServerFactoryCustomizer<TomcatServletWebServerFactory> tomcatCustomizer(
      ServiceProperties props) {
    return factory -> {
      InetAddress address = resolveAddress(props.getHost());
      if (address != null) {
        factory.setAddress(address);
      } else {
        log.warn("Failed to resolve host '{}', binding to all interfaces instead", props.getHost());
      }

      factory.setPort(props.getPort());
      factory.addConnectorCustomizers(connector -> configureConnector(connector, props));
    };
  }

  private InetAddress resolveAddress(String host) {
    try {
      return InetAddress.getByName(host);
    } catch (UnknownHostException ex) {
      log.warn("Unknown host '{}': {}", host, ex.getMessage());
      return null;
    }
  }

  private void configureConnector(Connector connector, ServiceProperties props) {
    long connectionTimeoutMs = safeMillis(props.getTomcatConnectionTimeout());
    long keepAliveTimeoutMs = safeMillis(props.getTomcatKeepAliveTimeout());

    connector.setProperty("connectionTimeout", String.valueOf(connectionTimeoutMs));
    connector.setProperty("keepAliveTimeout", String.valueOf(keepAliveTimeoutMs));
    connector.setProperty("maxConnections", String.valueOf(props.getTomcatMaxConnections()));
    connector.setProperty("acceptCount", String.valueOf(props.getTomcatAcceptCount()));
    connector.setProperty(
        "maxKeepAliveRequests", String.valueOf(props.getTomcatMaxKeepAliveRequests()));
    connector.setProperty("maxThreads", String.valueOf(props.getTomcatMaxThreads()));
    connector.setProperty("minSpareThreads", String.valueOf(props.getTomcatMinSpareThreads()));
  }

  private long safeMillis(Duration d) {
    long ms = d.toMillis();
    return Math.min(ms, Integer.MAX_VALUE);
  }
}
