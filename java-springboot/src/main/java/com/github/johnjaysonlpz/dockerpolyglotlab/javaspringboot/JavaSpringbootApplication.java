package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot;

import static net.logstash.logback.argument.StructuredArguments.kv;

import com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.config.ServiceProperties;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.boot.context.event.ApplicationReadyEvent;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.ApplicationListener;
import org.springframework.context.annotation.Bean;
import org.springframework.context.event.ContextClosedEvent;
import org.springframework.core.env.Environment;

@SpringBootApplication
@EnableConfigurationProperties(ServiceProperties.class)
public class JavaSpringbootApplication {

    private static final Logger log = LoggerFactory.getLogger(JavaSpringbootApplication.class);

    public static void main(String[] args) {
        SpringApplication.run(JavaSpringbootApplication.class, args);
    }

    @Bean
    ApplicationListener<ApplicationReadyEvent> applicationReadyLogger(
        ServiceProperties props,
        Environment env
    ) {
        return event -> {
            int port = env.getProperty("local.server.port", Integer.class, 8080);
            String[] profiles = env.getActiveProfiles();
            String activeProfiles = (profiles.length == 0)
                ? "default"
                : String.join(",", profiles);

            log.info("starting_server",
                kv("service", props.getServiceName()),
                kv("version", props.getVersion()),
                kv("buildTime", props.getBuildTime()),
                kv("addr", "0.0.0.0:" + port),
                kv("profiles", activeProfiles)
            );
        };
    }

    @Bean
    ApplicationListener<ContextClosedEvent> applicationShutdownLogger(ServiceProperties props) {
        return event -> log.info("server_shutdown_complete",
            kv("service", props.getServiceName()),
            kv("version", props.getVersion()),
            kv("buildTime", props.getBuildTime())
        );
    }
}
