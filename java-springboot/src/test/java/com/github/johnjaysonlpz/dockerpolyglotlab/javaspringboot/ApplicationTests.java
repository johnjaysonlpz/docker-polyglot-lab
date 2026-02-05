package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot;

import static org.assertj.core.api.Assertions.assertThat;

import com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.config.ServiceProperties;
import java.time.Duration;
import org.junit.jupiter.api.Test;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.context.event.ApplicationReadyEvent;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.context.ConfigurableApplicationContext;
import org.springframework.context.event.ContextClosedEvent;
import org.springframework.context.support.GenericApplicationContext;
import org.springframework.mock.env.MockEnvironment;

@SpringBootTest
class ApplicationTests {

  @Test
  void contextLoads() {}

  @Test
  void applicationListeners_doNotThrow() {
    Application app = new Application();

    ServiceProperties props = new ServiceProperties();
    props.setHost("0.0.0.0");

    MockEnvironment env = new MockEnvironment();
    env.setActiveProfiles("test");

    var ctx = new GenericApplicationContext();

    var ready = app.applicationReadyLogger(props, env);
    ready.onApplicationEvent(
        new ApplicationReadyEvent(
            new SpringApplication(Application.class), new String[0], ctx, Duration.ZERO));

    var shutdown = app.applicationShutdownLogger();
    shutdown.onApplicationEvent(new ContextClosedEvent(ctx));
  }

  @Test
  void applicationReadyLogger_defaultsToDefault_whenNoActiveProfiles() {
    Application app = new Application();

    ServiceProperties props = new ServiceProperties();
    props.setHost("0.0.0.0");

    MockEnvironment env = new MockEnvironment();

    var ctx = new GenericApplicationContext();

    var ready = app.applicationReadyLogger(props, env);
    ready.onApplicationEvent(
        new ApplicationReadyEvent(
            new SpringApplication(Application.class), new String[0], ctx, Duration.ZERO));
  }

  @Test
  void run_autoCloseTrue_closesContext() {
    ConfigurableApplicationContext ctx =
        Application.run(
            new String[] {
              "--spring.main.web-application-type=none",
              "--spring.main.banner-mode=off",
              "--logging.level.root=OFF"
            },
            true);

    assertThat(ctx.isActive()).isFalse();
  }

  @Test
  void run_autoCloseFalse_leavesContextActive_thenTestClosesIt() {
    ConfigurableApplicationContext ctx =
        Application.run(
            new String[] {
              "--spring.main.web-application-type=none",
              "--spring.main.banner-mode=off",
              "--logging.level.root=OFF"
            },
            false);

    try {
      assertThat(ctx.isActive()).isTrue();
    } finally {
      ctx.close();
    }

    assertThat(ctx.isActive()).isFalse();
  }

  @Test
  void main_executesPropertyReadLine_andReturns() {
    System.setProperty("app.test.autoClose", "true");
    try {
      Application.main(
          new String[] {
            "--spring.main.web-application-type=none",
            "--spring.main.banner-mode=off",
            "--logging.level.root=OFF"
          });
    } finally {
      System.clearProperty("app.test.autoClose");
    }
  }
}
