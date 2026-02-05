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
import org.springframework.context.ConfigurableApplicationContext;
import org.springframework.context.annotation.Bean;
import org.springframework.context.event.ContextClosedEvent;
import org.springframework.core.env.Environment;

@SpringBootApplication
@EnableConfigurationProperties(ServiceProperties.class)
public class Application {

  private static final Logger log = LoggerFactory.getLogger(Application.class);

  public static void main(String[] args) {
    run(args, Boolean.getBoolean("app.test.autoClose"));
  }

  static ConfigurableApplicationContext run(String[] args, boolean autoClose) {
    ConfigurableApplicationContext ctx = new SpringApplication(Application.class).run(args);
    if (autoClose) {
      ctx.close();
    }
    return ctx;
  }

  @Bean
  ApplicationListener<ApplicationReadyEvent> applicationReadyLogger(
      ServiceProperties props, Environment env) {
    return event -> {
      int port = env.getProperty("local.server.port", Integer.class, 8080);
      String[] profiles = env.getActiveProfiles();
      String activeProfiles = (profiles.length == 0) ? "default" : String.join(",", profiles);

      log.info(
          "starting_server",
          kv("addr", props.getHost() + ":" + port),
          kv("profiles", activeProfiles));
    };
  }

  @Bean
  ApplicationListener<ContextClosedEvent> applicationShutdownLogger() {
    return event -> log.info("server_shutdown_start");
  }
}
