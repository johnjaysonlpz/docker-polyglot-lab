package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.it;

import static org.assertj.core.api.Assertions.assertThat;

import com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.web.RequestIdFilter;
import java.time.Duration;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicReference;
import org.junit.jupiter.api.Test;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.boot.test.context.TestConfiguration;
import org.springframework.boot.test.web.server.LocalServerPort;
import org.springframework.context.ConfigurableApplicationContext;
import org.springframework.context.annotation.Bean;
import org.springframework.http.*;
import org.springframework.test.context.DynamicPropertyRegistry;
import org.springframework.test.context.DynamicPropertySource;
import org.springframework.web.bind.annotation.*;
import org.springframework.web.client.ResourceAccessException;
import org.springframework.web.client.RestTemplate;

class ServerIntegrationIT {

  @SpringBootTest(
      webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT,
      classes = {
        com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.Application.class,
        SlowShutdownTestConfig.class
      },
      properties = {
        "server.shutdown=graceful",
        "spring.lifecycle.timeout-per-shutdown-phase=10s",
        "logging.level.root=OFF",
        "app.max-body-bytes=1048576"
      })
  static class GracefulShutdownWaitsForInflightIT {

    @DynamicPropertySource
    static void forceRandomPort(DynamicPropertyRegistry r) {
      r.add("server.port", () -> "0");
    }

    @LocalServerPort int port;

    @org.springframework.beans.factory.annotation.Autowired ConfigurableApplicationContext ctx;

    @org.springframework.beans.factory.annotation.Autowired SlowShutdownTestConfig.Gates gates;

    @Test
    void shutdown_waits_for_inflight_request_then_completes_after_release() throws Exception {
      String base = "http://127.0.0.1:" + port;

      AtomicReference<ResponseEntity<String>> slowResp = new AtomicReference<>();
      AtomicReference<Throwable> slowErr = new AtomicReference<>();

      Thread reqThread =
          new Thread(
              () -> {
                try {
                  var f = new org.springframework.http.client.SimpleClientHttpRequestFactory();
                  f.setConnectTimeout((int) Duration.ofSeconds(2).toMillis());
                  f.setReadTimeout((int) Duration.ofSeconds(5).toMillis());
                  RestTemplate local = new RestTemplate(f);

                  ResponseEntity<String> r = local.getForEntity(base + "/__it/slow", String.class);
                  slowResp.set(r);
                } catch (Throwable t) {
                  slowErr.set(t);
                }
              });
      reqThread.setDaemon(true);
      reqThread.start();

      boolean started = gates.started.await(5, TimeUnit.SECONDS);
      assertThat(started).as("slow handler should start").isTrue();

      CountDownLatch closeFinished = new CountDownLatch(1);
      AtomicReference<Throwable> closeErr = new AtomicReference<>();
      Thread closeThread =
          new Thread(
              () -> {
                try {
                  ctx.close();
                } catch (Throwable t) {
                  closeErr.set(t);
                } finally {
                  closeFinished.countDown();
                }
              });
      closeThread.setDaemon(true);
      closeThread.start();

      boolean finishedTooEarly = closeFinished.await(1, TimeUnit.SECONDS);
      assertThat(finishedTooEarly).as("shutdown should wait while request is in-flight").isFalse();

      gates.release.countDown();

      reqThread.join(5_000);
      assertThat(slowErr.get()).as("slow request should not error").isNull();

      assertThat(slowResp.get()).isNotNull();
      assertThat(slowResp.get().getStatusCode()).isEqualTo(HttpStatus.OK);
      assertThat(slowResp.get().getBody()).isEqualTo("ok");

      boolean finished = closeFinished.await(5, TimeUnit.SECONDS);
      assertThat(finished).as("shutdown should finish after inflight completes").isTrue();
      assertThat(closeErr.get()).as("shutdown should not throw").isNull();
    }
  }

  @SpringBootTest(
      webEnvironment = SpringBootTest.WebEnvironment.RANDOM_PORT,
      classes = {
        com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.Application.class,
        EchoTooLargeTestConfig.class
      },
      properties = {
        "server.shutdown=graceful",
        "spring.lifecycle.timeout-per-shutdown-phase=10s",
        "logging.level.root=OFF",
        "app.max-body-bytes=3"
      })
  static class PayloadTooLargeE2EIT {

    @DynamicPropertySource
    static void forceRandomPort(DynamicPropertyRegistry r) {
      r.add("server.port", () -> "0");
    }

    @LocalServerPort int port;

    private final RestTemplate rt = new RestTemplate();

    @Test
    void payloadWithinLimit_succeeds() {
      String base = "http://127.0.0.1:" + port;

      HttpHeaders h = new HttpHeaders();
      h.setContentType(MediaType.TEXT_PLAIN);
      h.set(RequestIdFilter.HEADER_NAME, "it-ok-rid-1");

      HttpEntity<String> req = new HttpEntity<>("abc", h);

      ResponseEntity<Void> res = rt.exchange(base + "/__it/echo", HttpMethod.POST, req, Void.class);
      assertThat(res.getStatusCode()).isEqualTo(HttpStatus.NO_CONTENT);
      assertThat(res.getHeaders().getFirst(RequestIdFilter.HEADER_NAME)).isEqualTo("it-ok-rid-1");
    }

    @Test
    void payloadTooLarge_returns413_json_and_requestId() {
      String base = "http://127.0.0.1:" + port;

      String rid = "it-maxbytes-rid-1";

      HttpHeaders h = new HttpHeaders();
      h.setContentType(MediaType.TEXT_PLAIN);
      h.set(RequestIdFilter.HEADER_NAME, rid);

      HttpEntity<String> req = new HttpEntity<>("abcd", h);

      try {
        ResponseEntity<String> res =
            rt.exchange(base + "/__it/echo", HttpMethod.POST, req, String.class);
        throw new AssertionError("expected 413, got " + res.getStatusCode());
      } catch (org.springframework.web.client.HttpStatusCodeException ex) {
        assertThat(ex.getStatusCode()).isEqualTo(HttpStatus.CONTENT_TOO_LARGE);

        String hdrRid =
            ex.getResponseHeaders() != null
                ? ex.getResponseHeaders().getFirst(RequestIdFilter.HEADER_NAME)
                : null;
        assertThat(hdrRid).isEqualTo(rid);

        String body = ex.getResponseBodyAsString();
        assertThat(body).isNotBlank();
        assertThat(body).contains("\"code\":\"payload_too_large\"");
        assertThat(body).contains("\"request_id\":\"" + rid + "\"");
      } catch (ResourceAccessException rae) {
        throw new AssertionError("request failed (transport): " + rae.getMessage(), rae);
      }
    }
  }

  @TestConfiguration
  static class SlowShutdownTestConfig {

    @Bean
    Gates gates() {
      return new Gates();
    }

    static class Gates {
      final CountDownLatch started = new CountDownLatch(1);
      final CountDownLatch release = new CountDownLatch(1);
    }

    @RestController
    static class SlowController {
      private final Gates gates;

      SlowController(Gates gates) {
        this.gates = gates;
      }

      @GetMapping("/__it/slow")
      public ResponseEntity<String> slow() throws Exception {
        gates.started.countDown();
        boolean ok = gates.release.await(5, TimeUnit.SECONDS);
        if (!ok) {
          return ResponseEntity.status(HttpStatus.INTERNAL_SERVER_ERROR).body("timeout");
        }
        return ResponseEntity.ok("ok");
      }
    }
  }

  @TestConfiguration
  static class EchoTooLargeTestConfig {

    @RestController
    static class EchoController {

      @PostMapping(path = "/__it/echo", consumes = MediaType.ALL_VALUE)
      public ResponseEntity<Void> echo(@RequestBody byte[] body) {
        assertThat(body).isNotNull();
        return ResponseEntity.noContent().build();
      }
    }
  }
}
