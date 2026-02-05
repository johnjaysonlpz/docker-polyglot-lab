package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.web;

import com.fasterxml.jackson.annotation.JsonProperty;

public record ErrorResponse(
    @JsonProperty("error") String error,
    @JsonProperty("code") String code,
    @JsonProperty("request_id") String requestId) {}
