package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.web;

import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.AutoConfigureMockMvc;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.test.web.servlet.MockMvc;
import org.springframework.test.web.servlet.MvcResult;

import static org.assertj.core.api.Assertions.assertThat;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

@SpringBootTest
@AutoConfigureMockMvc
class MetricsControllerTest {

    @Autowired
    MockMvc mockMvc;

    @Test
    void metricsEndpointContainsHttpRequestsTotalAfterTraffic() throws Exception {
        mockMvc.perform(get("/")).andExpect(status().isOk());

        MvcResult result = mockMvc.perform(get("/metrics"))
            .andExpect(status().isOk())
            .andReturn();

        String body = result.getResponse().getContentAsString();

        assertThat(body).contains("http_requests_total");
    }
}