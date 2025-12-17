package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot;

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
        log.info("bootstrapping_application");
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

            log.info(
                "starting_server service={} version={} buildTime={} addr=0.0.0.0:{} profiles={}",
                props.getServiceName(),
                props.getVersion(),
                props.getBuildTime(),
                port,
                activeProfiles
            );
        };
    }

    @Bean
    ApplicationListener<ContextClosedEvent> applicationShutdownLogger(
        ServiceProperties props
    ) {
        return event -> {
            log.info(
                "server_shutdown_complete service={} version={} buildTime={}",
                props.getServiceName(),
                props.getVersion(),
                props.getBuildTime()
            );
        };
    }
}
