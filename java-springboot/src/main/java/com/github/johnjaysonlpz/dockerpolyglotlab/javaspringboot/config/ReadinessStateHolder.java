package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.config;

import org.springframework.boot.availability.AvailabilityChangeEvent;
import org.springframework.boot.availability.ReadinessState;
import org.springframework.context.ApplicationListener;
import org.springframework.stereotype.Component;

@Component
public class ReadinessStateHolder implements ApplicationListener<AvailabilityChangeEvent<ReadinessState>> {

    private volatile ReadinessState state = ReadinessState.ACCEPTING_TRAFFIC;

    @Override
    public void onApplicationEvent(AvailabilityChangeEvent<ReadinessState> event) {
        this.state = event.getState();
    }

    public ReadinessState getState() {
        return state;
    }
}
