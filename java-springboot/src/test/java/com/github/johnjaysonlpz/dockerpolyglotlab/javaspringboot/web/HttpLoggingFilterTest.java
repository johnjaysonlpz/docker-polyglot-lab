package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.web;

import java.util.List;

import ch.qos.logback.classic.Logger;
import ch.qos.logback.classic.spi.ILoggingEvent;
import ch.qos.logback.core.read.ListAppender;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.availability.AvailabilityChangeEvent;
import org.springframework.boot.availability.ReadinessState;
import org.springframework.boot.test.autoconfigure.web.servlet.AutoConfigureMockMvc;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.context.ApplicationContext;
import org.springframework.test.web.servlet.MockMvc;

import static org.assertj.core.api.Assertions.assertThat;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

@SpringBootTest
@AutoConfigureMockMvc
class HttpLoggingFilterTest {

    @Autowired
    MockMvc mockMvc;

    @Autowired
    ApplicationContext context;

    private ListAppender<ILoggingEvent> appender;
    private Logger httpLogger;

    @BeforeEach
    void resetReadiness() {
        AvailabilityChangeEvent.publish(context, ReadinessState.ACCEPTING_TRAFFIC);
    }

    @BeforeEach
    void setUp() {
        httpLogger = (Logger) LoggerFactory.getLogger("http");
        appender = new ListAppender<>();
        appender.start();
        httpLogger.addAppender(appender);
    }

    @AfterEach
    void tearDown() {
        if (httpLogger != null && appender != null) {
            httpLogger.detachAppender(appender);
            appender.stop();
        }
    }

    @Test
    void skipsInfraEndpointsInLogs() throws Exception {
        for (String path : List.of("/health", "/ready", "/metrics")) {
            appender.list.clear();
            mockMvc.perform(get(path)).andExpect(status().isOk());
            assertThat(appender.list).as("logs for %s", path).isEmpty();
        }
    }

    @Test
    void logsApplicationEndpoints() throws Exception {
        mockMvc.perform(get("/")).andExpect(status().isOk());

        assertThat(appender.list)
            .isNotEmpty();

        String message = appender.list.get(0).getFormattedMessage();
        assertThat(message).contains("http_request");
    }
}
