package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.config;

import java.net.InetAddress;
import java.net.UnknownHostException;

import org.apache.catalina.connector.Connector;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.boot.web.embedded.tomcat.TomcatServletWebServerFactory;
import org.springframework.boot.web.server.WebServerFactoryCustomizer;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

@Configuration
public class ServerConfiguration {

    private static final Logger log = LoggerFactory.getLogger(ServerConfiguration.class);

    @Bean
    WebServerFactoryCustomizer<TomcatServletWebServerFactory> tomcatCustomizer(ServiceProperties props) {
        return factory -> {
            InetAddress address = resolveAddress(props.getHost());
            if (address != null) {
                factory.setAddress(address);
            } else {
                log.warn("Failed to resolve host '{}', binding to all interfaces instead", props.getHost());
            }
            factory.setPort(props.getPort());
            factory.addConnectorCustomizers(connector -> configureConnectorTimeouts(connector, props));
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

    private void configureConnectorTimeouts(Connector connector, ServiceProperties props) {
        connector.setProperty("connectionTimeout", String.valueOf(props.getReadTimeout().toMillis()));
        connector.setProperty("keepAliveTimeout", String.valueOf(props.getIdleTimeout().toMillis()));
    }
}
