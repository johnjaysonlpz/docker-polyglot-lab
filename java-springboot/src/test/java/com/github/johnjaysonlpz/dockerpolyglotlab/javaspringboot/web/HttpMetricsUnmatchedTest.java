package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.web;

import io.micrometer.core.instrument.Counter;
import io.micrometer.core.instrument.MeterRegistry;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.AutoConfigureMockMvc;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.test.web.servlet.MockMvc;

import static org.assertj.core.api.Assertions.assertThat;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

@SpringBootTest
@AutoConfigureMockMvc
class HttpMetricsUnmatchedTest {

    @Autowired
    MockMvc mockMvc;

    @Autowired
    MeterRegistry meterRegistry;

    @Test
    void unmatchedRouteUsesStableMetricsLabel() throws Exception {
        mockMvc.perform(get("/does-not-exist"))
            .andExpect(status().isNotFound());

        Counter counter = meterRegistry.find("http_requests_total").tags(
            "service", "java-springboot-app",
            "version", "0.0.0-dev",
            "method", "GET",
            "path", "__unmatched__",
            "status", "404"
        ).counter();

        assertThat(counter).isNotNull();
        assertThat(counter.count()).isGreaterThanOrEqualTo(1.0);
    }
}
