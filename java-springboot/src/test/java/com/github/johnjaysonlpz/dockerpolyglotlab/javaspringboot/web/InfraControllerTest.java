package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.web;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.availability.AvailabilityChangeEvent;
import org.springframework.boot.availability.ReadinessState;
import org.springframework.boot.test.autoconfigure.web.servlet.AutoConfigureMockMvc;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.context.ApplicationContext;
import org.springframework.http.MediaType;
import org.springframework.test.web.servlet.MockMvc;

import static org.hamcrest.Matchers.is;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.content;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

@SpringBootTest
@AutoConfigureMockMvc
class InfraControllerTest {

    @Autowired
    MockMvc mockMvc;

    @Autowired
    ApplicationContext context;

    @BeforeEach
    void resetReadiness() {
        AvailabilityChangeEvent.publish(context, ReadinessState.ACCEPTING_TRAFFIC);
    }

    @Test
    void rootReturnsBanner() throws Exception {
        mockMvc.perform(get("/"))
            .andExpect(status().isOk())
            .andExpect(content().string("java-springboot-app is running (Java + Spring Boot)\n"));
    }

    @Test
    void healthAndReadyEndpointsRespondOk() throws Exception {
        mockMvc.perform(get("/health"))
            .andExpect(status().isOk());

        mockMvc.perform(get("/ready"))
            .andExpect(status().isOk());
    }

    @Test
    void infoReturnsMetadata() throws Exception {
        mockMvc.perform(get("/info").accept(MediaType.APPLICATION_JSON))
            .andExpect(status().isOk())
            .andExpect(jsonPath("$.service", is("java-springboot-app")))
            .andExpect(jsonPath("$.version", is("0.0.0-dev")))
            .andExpect(jsonPath("$.buildTime", is("unknown")));
    }

    @Test
    void readyReturns503WhenRefusingTraffic() throws Exception {
        AvailabilityChangeEvent.publish(context, ReadinessState.REFUSING_TRAFFIC);

        mockMvc.perform(get("/ready"))
            .andExpect(status().isServiceUnavailable());
    }
}
