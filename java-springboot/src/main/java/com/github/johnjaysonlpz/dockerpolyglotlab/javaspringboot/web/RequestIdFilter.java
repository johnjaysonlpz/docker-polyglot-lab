package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.web;

import java.io.IOException;
import java.util.UUID;

import jakarta.servlet.FilterChain;
import jakarta.servlet.ServletException;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import org.slf4j.MDC;
import org.springframework.core.Ordered;
import org.springframework.core.annotation.Order;
import org.springframework.stereotype.Component;
import org.springframework.web.filter.OncePerRequestFilter;

@Component
@Order(Ordered.HIGHEST_PRECEDENCE)
public class RequestIdFilter extends OncePerRequestFilter {

    public static final String HEADER_NAME = "X-Request-ID";
    public static final String MDC_KEY = "request_id";

    @Override
    protected void doFilterInternal(
        HttpServletRequest request,
        HttpServletResponse response,
        FilterChain filterChain
    ) throws ServletException, IOException {

        String incoming = request.getHeader(HEADER_NAME);
        String requestId = isValidRequestId(incoming) ? incoming : UUID.randomUUID().toString();

        MDC.put(MDC_KEY, requestId);
        request.setAttribute(MDC_KEY, requestId);
        response.setHeader(HEADER_NAME, requestId);

        try {
            filterChain.doFilter(request, response);
        } finally {
            MDC.remove(MDC_KEY);
        }
    }

    private boolean isValidRequestId(String v) {
        if (v == null) return false;
        String s = v.trim();
        if (s.isEmpty() || s.length() > 128) return false;
        // conservative allowlist
        return s.matches("^[A-Za-z0-9._\\-]+$");
    }
}
