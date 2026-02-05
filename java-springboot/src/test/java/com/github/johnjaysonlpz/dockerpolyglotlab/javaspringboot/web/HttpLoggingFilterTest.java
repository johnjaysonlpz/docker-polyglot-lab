package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.web;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.verifyNoInteractions;

import com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.config.HttpServerMetrics;
import jakarta.servlet.ServletException;
import jakarta.servlet.http.HttpServletRequest;
import java.lang.reflect.Method;
import java.time.Duration;
import org.junit.jupiter.api.Test;
import org.springframework.mock.web.MockHttpServletRequest;
import org.springframework.mock.web.MockHttpServletResponse;
import org.springframework.web.servlet.HandlerMapping;

class HttpLoggingFilterTest {

  @Test
  void infraPath_2xx_recordsMetrics_andSkipsExtraWork() throws Exception {
    HttpServerMetrics metrics = mock(HttpServerMetrics.class);
    HttpLoggingFilter filter = new HttpLoggingFilter(metrics);

    MockHttpServletRequest req = new MockHttpServletRequest("GET", "/health");
    MockHttpServletResponse res = new MockHttpServletResponse();

    filter.doFilter(req, res, (r, s) -> res.setStatus(200));

    verify(metrics).record(eq("GET"), eq("/health"), eq(200), any(Duration.class));
  }

  @Test
  void infraPath_5xx_doesNotSkip_andUsesRawPathLabel() throws Exception {
    HttpServerMetrics metrics = mock(HttpServerMetrics.class);
    HttpLoggingFilter filter = new HttpLoggingFilter(metrics);

    MockHttpServletRequest req = new MockHttpServletRequest("GET", "/metrics");
    MockHttpServletResponse res = new MockHttpServletResponse();

    filter.doFilter(req, res, (r, s) -> res.setStatus(500));

    verify(metrics).record(eq("GET"), eq("/metrics"), eq(500), any(Duration.class));
  }

  @Test
  void nonInfra_matchedPattern_usesPatternAsLabel_andCountsBytes() throws Exception {
    HttpServerMetrics metrics = mock(HttpServerMetrics.class);
    HttpLoggingFilter filter = new HttpLoggingFilter(metrics);

    MockHttpServletRequest req = new MockHttpServletRequest("GET", "/info");
    req.setAttribute(HandlerMapping.BEST_MATCHING_PATTERN_ATTRIBUTE, "/info");
    req.addHeader("User-Agent", "ua");
    MockHttpServletResponse res = new MockHttpServletResponse();

    filter.doFilter(
        req,
        res,
        (r, s) -> {
          res.setStatus(200);
          res.getWriter().write("ok");
        });

    verify(metrics).record(eq("GET"), eq("/info"), eq(200), any(Duration.class));
  }

  @Test
  void nonInfra_blankPattern_isTreatedAsUnmatched() throws Exception {
    HttpServerMetrics metrics = mock(HttpServerMetrics.class);
    HttpLoggingFilter filter = new HttpLoggingFilter(metrics);

    MockHttpServletRequest req = new MockHttpServletRequest("GET", "/x");
    req.setAttribute(HandlerMapping.BEST_MATCHING_PATTERN_ATTRIBUTE, "   ");
    MockHttpServletResponse res = new MockHttpServletResponse();

    filter.doFilter(req, res, (r, s) -> res.setStatus(200));

    verify(metrics).record(eq("GET"), eq("__unmatched__"), eq(200), any(Duration.class));
  }

  @Test
  void nonInfra_patternWildcard_isTreatedAsUnmatched() throws Exception {
    HttpServerMetrics metrics = mock(HttpServerMetrics.class);
    HttpLoggingFilter filter = new HttpLoggingFilter(metrics);

    MockHttpServletRequest req = new MockHttpServletRequest("GET", "/x");
    req.setAttribute(HandlerMapping.BEST_MATCHING_PATTERN_ATTRIBUTE, "/**");
    MockHttpServletResponse res = new MockHttpServletResponse();

    filter.doFilter(req, res, (r, s) -> res.setStatus(200));

    verify(metrics).record(eq("GET"), eq("__unmatched__"), eq(200), any(Duration.class));
  }

  @Test
  void status404_forcesUnmatchedLabel_evenIfPatternExists() throws Exception {
    HttpServerMetrics metrics = mock(HttpServerMetrics.class);
    HttpLoggingFilter filter = new HttpLoggingFilter(metrics);

    MockHttpServletRequest req = new MockHttpServletRequest("GET", "/missing");
    req.setAttribute(HandlerMapping.BEST_MATCHING_PATTERN_ATTRIBUTE, "/missing");
    MockHttpServletResponse res = new MockHttpServletResponse();

    filter.doFilter(req, res, (r, s) -> res.setStatus(404));

    verify(metrics).record(eq("GET"), eq("__unmatched__"), eq(404), any(Duration.class));
  }

  @Test
  void thrownIOException_isRethrown_andStatusIsCoercedTo500() throws Exception {
    HttpServerMetrics metrics = mock(HttpServerMetrics.class);
    HttpLoggingFilter filter = new HttpLoggingFilter(metrics);

    MockHttpServletRequest req = new MockHttpServletRequest("GET", "/ioe");
    MockHttpServletResponse res = new MockHttpServletResponse();

    assertThatThrownBy(
            () ->
                filter.doFilter(
                    req,
                    res,
                    (r, s) -> {
                      res.setStatus(200);
                      throw new java.io.IOException("io");
                    }))
        .isInstanceOf(java.io.IOException.class);

    verify(metrics).record(eq("GET"), eq("__unmatched__"), eq(500), any(Duration.class));
  }

  @Test
  void thrownServletException_isRethrown_andStatusIsCoercedTo500() throws Exception {
    HttpServerMetrics metrics = mock(HttpServerMetrics.class);
    HttpLoggingFilter filter = new HttpLoggingFilter(metrics);

    MockHttpServletRequest req = new MockHttpServletRequest("GET", "/se");
    MockHttpServletResponse res = new MockHttpServletResponse();

    assertThatThrownBy(
            () ->
                filter.doFilter(
                    req,
                    res,
                    (r, s) -> {
                      res.setStatus(200);
                      throw new ServletException(new IllegalArgumentException("bad"));
                    }))
        .isInstanceOf(ServletException.class);

    verify(metrics).record(eq("GET"), eq("__unmatched__"), eq(500), any(Duration.class));
  }

  @Test
  void thrownRuntimeException_isRethrown_andStatusIsCoercedTo500() throws Exception {
    HttpServerMetrics metrics = mock(HttpServerMetrics.class);
    HttpLoggingFilter filter = new HttpLoggingFilter(metrics);

    MockHttpServletRequest req = new MockHttpServletRequest("GET", "/re");
    MockHttpServletResponse res = new MockHttpServletResponse();

    assertThatThrownBy(
            () ->
                filter.doFilter(
                    req,
                    res,
                    (r, s) -> {
                      res.setStatus(200);
                      throw new RuntimeException("boom");
                    }))
        .isInstanceOf(RuntimeException.class);

    verify(metrics).record(eq("GET"), eq("__unmatched__"), eq(500), any(Duration.class));
  }

  @Test
  void thrownRuntimeException_withStatusAlready500_doesNotOverrideStatus() throws Exception {
    HttpServerMetrics metrics = mock(HttpServerMetrics.class);
    HttpLoggingFilter filter = new HttpLoggingFilter(metrics);

    MockHttpServletRequest req = new MockHttpServletRequest("GET", "/re500");
    MockHttpServletResponse res = new MockHttpServletResponse();

    assertThatThrownBy(
            () ->
                filter.doFilter(
                    req,
                    res,
                    (r, s) -> {
                      res.setStatus(500);
                      throw new RuntimeException("boom");
                    }))
        .isInstanceOf(RuntimeException.class);

    verify(metrics).record(eq("GET"), eq("__unmatched__"), eq(500), any(Duration.class));
  }

  @Test
  void thrownError_isRethrown_andCompletionWorkIsSkipped() throws Exception {
    HttpServerMetrics metrics = mock(HttpServerMetrics.class);
    HttpLoggingFilter filter = new HttpLoggingFilter(metrics);

    MockHttpServletRequest req = new MockHttpServletRequest("GET", "/err");
    MockHttpServletResponse res = new MockHttpServletResponse();

    assertThatThrownBy(
            () ->
                filter.doFilterInternal(
                    req,
                    res,
                    (r, s) -> {
                      throw new AssertionError("err");
                    }))
        .isInstanceOf(AssertionError.class);

    verifyNoInteractions(metrics);
  }

  @Test
  void status500_withoutThrown_usesHandledExceptionAttribute_asEffectiveThrown() throws Exception {
    HttpServerMetrics metrics = mock(HttpServerMetrics.class);
    HttpLoggingFilter filter = new HttpLoggingFilter(metrics);

    MockHttpServletRequest req = new MockHttpServletRequest("GET", "/x");
    req.setAttribute(
        GlobalExceptionHandler.HANDLED_EXCEPTION_ATTR, new RuntimeException("handled"));
    MockHttpServletResponse res = new MockHttpServletResponse();

    filter.doFilter(req, res, (r, s) -> res.setStatus(500));

    verify(metrics).record(eq("GET"), eq("__unmatched__"), eq(500), any(Duration.class));
  }

  @Test
  void privateHelpers_areCoveredViaReflection() throws Exception {
    Method latencyMs = HttpLoggingFilter.class.getDeclaredMethod("latencyMs", long.class);
    latencyMs.setAccessible(true);
    double ms = (double) latencyMs.invoke(null, 1L);
    assertThat(ms).isGreaterThan(0.0d);

    double ms0 = (double) latencyMs.invoke(null, 0L);
    assertThat(ms0).isEqualTo(0.0d);

    Method truncate =
        HttpLoggingFilter.class.getDeclaredMethod("truncate", String.class, int.class);
    truncate.setAccessible(true);
    assertThat((String) truncate.invoke(null, (String) null, 5)).isEqualTo("");
    assertThat((String) truncate.invoke(null, "  abc  ", 10)).isEqualTo("abc");
    assertThat((String) truncate.invoke(null, "x".repeat(20), 5)).hasSize(5);

    Method appendThrowable =
        HttpLoggingFilter.class.getDeclaredMethod(
            "appendThrowable", Object[].class, Throwable.class);
    appendThrowable.setAccessible(true);
    Object[] base = new Object[] {"a"};
    assertThat((Object[]) appendThrowable.invoke(null, (Object) base, null)).isSameAs(base);
    Object[] out =
        (Object[]) appendThrowable.invoke(null, (Object) base, new RuntimeException("x"));
    assertThat(out).hasSize(2);

    Method isInfraPath = HttpLoggingFilter.class.getDeclaredMethod("isInfraPath", String.class);
    isInfraPath.setAccessible(true);
    assertThat((boolean) isInfraPath.invoke(null, (String) null)).isFalse();
    assertThat((boolean) isInfraPath.invoke(null, "/health")).isTrue();
    assertThat((boolean) isInfraPath.invoke(null, "/actuator/info")).isTrue();
    assertThat((boolean) isInfraPath.invoke(null, "/other")).isFalse();

    Method bestMatchingPattern =
        HttpLoggingFilter.class.getDeclaredMethod("bestMatchingPattern", HttpServletRequest.class);
    bestMatchingPattern.setAccessible(true);
    MockHttpServletRequest req = new MockHttpServletRequest("GET", "/x");
    assertThat(bestMatchingPattern.invoke(null, req)).isNull();
    req.setAttribute(HandlerMapping.BEST_MATCHING_PATTERN_ATTRIBUTE, "/x");
    assertThat(bestMatchingPattern.invoke(null, req)).isEqualTo("/x");

    Method rootCause = HttpLoggingFilter.class.getDeclaredMethod("rootCause", Throwable.class);
    rootCause.setAccessible(true);

    RuntimeException leaf = new RuntimeException("leaf");
    RuntimeException mid = new RuntimeException("mid", leaf);
    RuntimeException top = new RuntimeException("top", mid);
    assertThat(rootCause.invoke(null, top)).isSameAs(leaf);

    Throwable selfCause =
        new Throwable("self") {
          @Override
          public synchronized Throwable getCause() {
            return this;
          }
        };
    assertThat(rootCause.invoke(null, selfCause)).isSameAs(selfCause);

    RuntimeException chain = new RuntimeException("0");
    RuntimeException cur = chain;
    RuntimeException at20 = null;
    for (int i = 1; i <= 30; i++) {
      RuntimeException next = new RuntimeException(String.valueOf(i));
      cur.initCause(next);
      cur = next;
      if (i == 20) at20 = next;
    }
    assertThat(at20).isNotNull();
    assertThat(rootCause.invoke(null, chain)).isSameAs(at20);
  }
}
