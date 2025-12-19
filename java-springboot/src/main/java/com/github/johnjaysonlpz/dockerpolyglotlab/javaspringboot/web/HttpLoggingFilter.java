package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.web;

import static net.logstash.logback.argument.StructuredArguments.kv;

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
import org.slf4j.MDC;
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
    private static final String UNMATCHED = "__unmatched__";

    private final ServiceProperties props;
    private final HttpServerMetrics metrics;

    public HttpLoggingFilter(ServiceProperties props, HttpServerMetrics metrics) {
        this.props = props;
        this.metrics = metrics;
    }

    private boolean isInfraPath(String path) {
        if (path == null) return false;
        return SKIP_PATHS.contains(path) || path.startsWith(ACTUATOR_PREFIX);
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
            Duration duration = Duration.between(start, Instant.now());
            int status = response.getStatus();

            String pattern = (String) request.getAttribute(HandlerMapping.BEST_MATCHING_PATTERN_ATTRIBUTE);

            String metricsPath = (pattern != null) ? pattern : UNMATCHED;
            if (status == 404 && (pattern == null || "/**".equals(pattern))) {
                metricsPath = UNMATCHED;
            }

            String logPath = (pattern != null) ? pattern : UNMATCHED;
            if (status == 404 && (pattern == null || "/**".equals(pattern))) {
                logPath = UNMATCHED;
            }

            metrics.record(request.getMethod(), metricsPath, status, duration);

            if (skip) {
                return;
            }

            String requestId = MDC.get(RequestIdFilter.MDC_KEY);
            String clientIp = resolveClientIp(request);
            String userAgent = request.getHeader("User-Agent");
            String query = request.getQueryString();

            if (status >= 500) {
                log.error("http_request",
                    kv("service", props.getServiceName()),
                    kv("version", props.getVersion()),
                    kv("request_id", requestId),
                    kv("method", request.getMethod()),
                    kv("path", logPath),
                    kv("rawPath", rawPath),
                    kv("query", query == null ? "" : query),
                    kv("status", status),
                    kv("ip", clientIp),
                    kv("latencyMs", duration.toMillis()),
                    kv("userAgent", userAgent == null ? "" : userAgent)
                );
            } else if (status >= 400) {
                log.warn("http_request",
                    kv("service", props.getServiceName()),
                    kv("version", props.getVersion()),
                    kv("request_id", requestId),
                    kv("method", request.getMethod()),
                    kv("path", logPath),
                    kv("rawPath", rawPath),
                    kv("query", query == null ? "" : query),
                    kv("status", status),
                    kv("ip", clientIp),
                    kv("latencyMs", duration.toMillis()),
                    kv("userAgent", userAgent == null ? "" : userAgent)
                );
            } else {
                log.info("http_request",
                    kv("service", props.getServiceName()),
                    kv("version", props.getVersion()),
                    kv("request_id", requestId),
                    kv("method", request.getMethod()),
                    kv("path", logPath),
                    kv("rawPath", rawPath),
                    kv("query", query == null ? "" : query),
                    kv("status", status),
                    kv("ip", clientIp),
                    kv("latencyMs", duration.toMillis()),
                    kv("userAgent", userAgent == null ? "" : userAgent)
                );
            }
        }
    }

    private String resolveClientIp(HttpServletRequest request) {
        // With server.forward-headers-strategy=framework, Spring will process forwarded headers,
        // but getRemoteAddr() still returns the TCP peer. For logging, we prefer XFF if present.
        String xff = request.getHeader("X-Forwarded-For");
        if (xff != null && !xff.isBlank()) {
            String first = xff.split(",")[0].trim();
            if (!first.isEmpty()) return first;
        }
        String xri = request.getHeader("X-Real-IP");
        if (xri != null && !xri.isBlank()) return xri.trim();
        return request.getRemoteAddr();
    }
}
