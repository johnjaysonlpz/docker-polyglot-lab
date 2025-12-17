package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.config;

import com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.JavaSpringbootApplication;
import org.junit.jupiter.api.Test;
import org.springframework.boot.SpringApplication;

import static org.assertj.core.api.Assertions.assertThatThrownBy;

class InvalidConfigStartupTest {

    @Test
    void invalidPortCausesStartupFailure() {
        assertThatThrownBy(() ->
            SpringApplication.from(JavaSpringbootApplication::main)
                .run("--app.port=70000", "--app.host=   ")
        ).isInstanceOf(Exception.class);
    }
}
