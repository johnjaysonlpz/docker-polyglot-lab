package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.config;

import java.time.Duration;
import java.util.Objects;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.ConcurrentMap;

import io.micrometer.core.instrument.Counter;
import io.micrometer.core.instrument.MeterRegistry;
import io.micrometer.core.instrument.Tags;
import io.micrometer.core.instrument.Timer;

public class HttpServerMetrics {

    private final MeterRegistry registry;
    private final Timer.Builder timerBuilder;
    private final String counterName;

    private final ConcurrentMap<Key, HttpMeters> metersCache = new ConcurrentHashMap<>();

    public HttpServerMetrics(
        MeterRegistry registry,
        Timer.Builder timerBuilder,
        String counterName
    ) {
        this.registry = registry;
        this.timerBuilder = timerBuilder;
        this.counterName = counterName;
    }

    public void record(String method, String path, int status, Duration duration) {
        Key key = new Key(method, path, status);

        HttpMeters meters = metersCache.computeIfAbsent(key, k -> {
            Tags tags = Tags.of(
                "method", k.method,
                "path", k.path,
                "status", String.valueOf(k.status)
            );

            Counter counter = Counter
                .builder(counterName)
                .tags(tags)
                .description("Total number of HTTP requests processed.")
                .register(registry);

            Timer timer = timerBuilder
                .tags(tags)
                .register(registry);

            return new HttpMeters(counter, timer);
        });

        meters.counter.increment();
        meters.timer.record(duration);
    }

    private static final class HttpMeters {
        final Counter counter;
        final Timer timer;

        HttpMeters(Counter counter, Timer timer) {
            this.counter = counter;
            this.timer = timer;
        }
    }

    private static final class Key {
        final String method;
        final String path;
        final int status;

        Key(String method, String path, int status) {
            this.method = method;
            this.path = path;
            this.status = status;
        }

        @Override
        public boolean equals(Object o) {
            if (this == o) return true;
            if (!(o instanceof Key key)) return false;
            return status == key.status
                && Objects.equals(method, key.method)
                && Objects.equals(path, key.path);
        }

        @Override
        public int hashCode() {
            return Objects.hash(method, path, status);
        }
    }
}
