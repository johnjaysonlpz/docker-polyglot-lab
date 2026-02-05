package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.web;

import static net.logstash.logback.argument.StructuredArguments.kv;

import com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.config.HttpServerMetrics;
import jakarta.servlet.FilterChain;
import jakarta.servlet.ServletException;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import java.io.IOException;
import java.time.Duration;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Set;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.core.Ordered;
import org.springframework.core.annotation.Order;
import org.springframework.stereotype.Component;
import org.springframework.web.filter.OncePerRequestFilter;
import org.springframework.web.servlet.HandlerMapping;

@Component
@Order(Ordered.HIGHEST_PRECEDENCE + 10)
public class HttpLoggingFilter extends OncePerRequestFilter {

  private static final Logger log = LoggerFactory.getLogger("http");

  private static final Set<String> SKIP_LOG_PATHS = Set.of("/health", "/ready", "/metrics");
  private static final String ACTUATOR_PREFIX = "/actuator";
  private static final String UNMATCHED = "__unmatched__";

  private final HttpServerMetrics metrics;

  public HttpLoggingFilter(HttpServerMetrics metrics) {
    this.metrics = metrics;
  }

  @Override
  protected void doFilterInternal(
      HttpServletRequest request, HttpServletResponse response, FilterChain filterChain)
      throws ServletException, IOException {

    String rawPath = request.getRequestURI();
    boolean infraPath = isInfraPath(rawPath);

    ResponseBodyBytesCountingWrapper wrapped = new ResponseBodyBytesCountingWrapper(response);

    long startNanos = System.nanoTime();
    Throwable thrown = null;

    try {
      filterChain.doFilter(request, wrapped);
    } catch (IOException ioe) {
      thrown = ioe;
      throw ioe;
    } catch (ServletException se) {
      thrown = se;
      throw se;
    } catch (RuntimeException re) {
      thrown = re;
      throw re;
    } catch (Error err) {
      thrown = err;
      throw err;
    } finally {

      if (!(thrown instanceof Error)) {
        onComplete(request, wrapped, rawPath, infraPath, startNanos, thrown);
      }
    }
  }

  private void onComplete(
      HttpServletRequest request,
      ResponseBodyBytesCountingWrapper wrapped,
      String rawPath,
      boolean infraPath,
      long startNanos,
      Throwable thrown) {

    long elapsedNanos = System.nanoTime() - startNanos;
    Duration duration = Duration.ofNanos(elapsedNanos);
    double latencyMs = latencyMs(elapsedNanos);

    int status = effectiveStatus(wrapped, thrown);
    Throwable effectiveThrown = resolveThrown(thrown, request, status);

    String pattern = bestMatchingPattern(request);
    String pathLabel = resolvePathLabel(rawPath, pattern, status);

    metrics.record(request.getMethod(), pathLabel, status, duration);

    if (infraPath && status < 400) {
      return;
    }

    Object[] args =
        buildLogArgs(request, wrapped, rawPath, pathLabel, status, latencyMs, effectiveThrown);

    if (effectiveThrown != null || status >= 500) {
      log.error("http_request", appendThrowable(args, effectiveThrown));
      return;
    }
    if (status >= 400) {
      log.warn("http_request", args);
      return;
    }
    log.info("http_request", args);
  }

  private static boolean isInfraPath(String path) {
    if (path == null) return false;
    return SKIP_LOG_PATHS.contains(path) || path.startsWith(ACTUATOR_PREFIX);
  }

  private static String bestMatchingPattern(HttpServletRequest request) {
    Object v = request.getAttribute(HandlerMapping.BEST_MATCHING_PATTERN_ATTRIBUTE);
    return (v != null) ? v.toString() : null;
  }

  private static String resolvePathLabel(String rawPath, String pattern, int status) {
    if (status == 404) return UNMATCHED;

    if (pattern != null && !pattern.isBlank() && !"/**".equals(pattern)) return pattern;

    if (isInfraPath(rawPath)) return rawPath;

    return UNMATCHED;
  }

  private static int effectiveStatus(ResponseBodyBytesCountingWrapper wrapped, Throwable thrown) {
    int status = wrapped.getStatus();
    if (thrown != null && status < 500) {
      return HttpServletResponse.SC_INTERNAL_SERVER_ERROR;
    }
    return status;
  }

  private static Throwable resolveThrown(Throwable thrown, HttpServletRequest request, int status) {
    if (thrown != null) return thrown;
    if (status < 500) return null;

    Object handled = request.getAttribute(GlobalExceptionHandler.HANDLED_EXCEPTION_ATTR);
    return (handled instanceof Throwable t) ? t : null;
  }

  private static Object[] buildLogArgs(
      HttpServletRequest request,
      ResponseBodyBytesCountingWrapper wrapped,
      String rawPath,
      String pathLabel,
      int status,
      double latencyMs,
      Throwable thrown) {

    String clientIp = resolveClientIp(request);
    String userAgent = truncate(request.getHeader("User-Agent"), 512);

    ArrayList<Object> fields = new ArrayList<>(18);
    fields.add(kv("method", request.getMethod()));
    fields.add(kv("path", pathLabel));
    fields.add(kv("raw_path", rawPath));
    fields.add(kv("status", status));
    fields.add(kv("ip", clientIp));
    fields.add(kv("latency_ms", latencyMs));
    fields.add(kv("bytes_written", wrapped.getBytesWritten()));
    fields.add(kv("user_agent", userAgent));

    if (thrown != null) {
      Throwable root = rootCause(thrown);
      fields.add(kv("error", root.getClass().getSimpleName()));
      fields.add(kv("error_message", truncate(root.getMessage(), 300)));
    } else if (status >= 500) {
      fields.add(kv("error", "HTTP_5XX"));
    }

    return fields.toArray();
  }

  private static double latencyMs(long elapsedNanos) {
    double raw = elapsedNanos / 1_000_000.0;
    double ms = Math.round(raw * 1000.0) / 1000.0;
    if (ms == 0.0 && raw > 0.0) {
      ms = Math.round(raw * 1_000_000.0) / 1_000_000.0;
    }
    return ms;
  }

  private static Throwable rootCause(Throwable t) {
    Throwable cur = t;
    int guard = 0;
    while (cur.getCause() != null && cur.getCause() != cur && guard < 20) {
      cur = cur.getCause();
      guard++;
    }
    return cur;
  }

  private static Object[] appendThrowable(Object[] args, Throwable t) {
    if (t == null) return args;
    Object[] out = Arrays.copyOf(args, args.length + 1);
    out[out.length - 1] = t;
    return out;
  }

  private static String truncate(String s, int max) {
    if (s == null) return "";
    String v = s.trim();
    if (v.length() <= max) return v;
    return v.substring(0, max);
  }

  private static String resolveClientIp(HttpServletRequest request) {
    return request.getRemoteAddr();
  }
}
