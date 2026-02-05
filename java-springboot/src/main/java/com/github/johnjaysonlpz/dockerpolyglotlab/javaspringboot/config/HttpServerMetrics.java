package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.config;

import io.micrometer.core.instrument.Counter;
import io.micrometer.core.instrument.MeterRegistry;
import io.micrometer.core.instrument.Tags;
import io.micrometer.core.instrument.Timer;
import java.time.Duration;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.ConcurrentMap;
import java.util.function.Supplier;

public class HttpServerMetrics {

  private final MeterRegistry registry;
  private final Supplier<Timer.Builder> timerBuilderSupplier;
  private final String counterName;

  private final ConcurrentMap<Key, HttpMeters> metersCache = new ConcurrentHashMap<>();

  public HttpServerMetrics(
      MeterRegistry registry, Supplier<Timer.Builder> timerBuilderSupplier, String counterName) {
    this.registry = registry;
    this.timerBuilderSupplier = timerBuilderSupplier;
    this.counterName = counterName;
  }

  public void record(String method, String path, int status, Duration duration) {
    Key key = new Key(method, path, status);

    HttpMeters meters =
        metersCache.computeIfAbsent(
            key,
            k -> {
              Tags tags =
                  Tags.of(
                      "method", k.method(),
                      "path", k.path(),
                      "status", String.valueOf(k.status()));

              Counter counter =
                  Counter.builder(counterName)
                      .tags(tags)
                      .description("Total number of HTTP requests processed.")
                      .register(registry);

              Timer timer = timerBuilderSupplier.get().tags(tags).register(registry);

              return new HttpMeters(counter, timer);
            });

    meters.counter().increment();
    meters.timer().record(duration);
  }

  private record HttpMeters(Counter counter, Timer timer) {}

  private record Key(String method, String path, int status) {}
}
