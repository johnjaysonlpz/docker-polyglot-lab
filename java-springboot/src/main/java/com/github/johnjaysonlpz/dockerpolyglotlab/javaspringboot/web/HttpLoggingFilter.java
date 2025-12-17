package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.web;

import java.io.IOException;
import java.time.Duration;
import java.time.Instant;
import java.util.Set;

import com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.config.HttpServerMetrics;
import com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.config.ServiceProperties;
import jakarta.servlet.FilterChain;
import jakarta.servlet.ServletException;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.stereotype.Component;
import org.springframework.web.filter.OncePerRequestFilter;
import org.springframework.web.servlet.HandlerMapping;

@Component
public class HttpLoggingFilter extends OncePerRequestFilter {

    private static final Logger log = LoggerFactory.getLogger("http");

    private static final Set<String> SKIP_PATHS = Set.of(
        "/health",
        "/ready",
        "/metrics"
    );

    private static final String ACTUATOR_PREFIX = "/actuator";

    private final ServiceProperties props;
    private final HttpServerMetrics metrics;

    public HttpLoggingFilter(ServiceProperties props, HttpServerMetrics metrics) {
        this.props = props;
        this.metrics = metrics;
    }

    private boolean isInfraPath(String path) {
        if (path == null) {
            return false;
        }

        if (SKIP_PATHS.contains(path)) {
            return true;
        }

        if (path.startsWith(ACTUATOR_PREFIX)) {
            return true;
        }

        return false;
    }

    @Override
    protected void doFilterInternal(
        HttpServletRequest request,
        HttpServletResponse response,
        FilterChain filterChain
    ) throws ServletException, IOException {

        String rawPath = request.getRequestURI();
        boolean skip = isInfraPath(rawPath);

        Instant start = Instant.now();
        try {
            filterChain.doFilter(request, response);
        } finally {
            Instant end = Instant.now();
            Duration duration = Duration.between(start, end);

            String pattern = (String) request.getAttribute(HandlerMapping.BEST_MATCHING_PATTERN_ATTRIBUTE);
            String pathLabel = (pattern != null) ? pattern : rawPath;

            metrics.record(request.getMethod(), pathLabel, response.getStatus(), duration);

            if (skip) {
                return;
            }

            log.info(
                "http_request service={} version={} method={} path={} rawPath={} status={} ip={} latencyMs={} userAgent=\"{}\"",
                props.getServiceName(),
                props.getVersion(),
                request.getMethod(),
                pathLabel,
                rawPath,
                response.getStatus(),
                request.getRemoteAddr(),
                duration.toMillis(),
                request.getHeader("User-Agent")
            );
        }
    }
}
