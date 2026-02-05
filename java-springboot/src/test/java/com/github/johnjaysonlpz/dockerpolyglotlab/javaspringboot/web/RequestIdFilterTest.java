package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.web;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import jakarta.servlet.FilterChain;
import jakarta.servlet.ServletException;
import java.io.IOException;
import java.lang.reflect.Method;
import java.util.UUID;
import org.junit.jupiter.api.Test;
import org.slf4j.MDC;
import org.springframework.mock.web.MockHttpServletRequest;
import org.springframework.mock.web.MockHttpServletResponse;

class RequestIdFilterTest {

  @Test
  void doFilterInternal_validHeader_trims_setsHeadersAndAttr_setsMdc_andCleansUp()
      throws Exception {
    RequestIdFilter filter = new RequestIdFilter();

    MockHttpServletRequest req = new MockHttpServletRequest("GET", "/x");
    MockHttpServletResponse res = new MockHttpServletResponse();
    req.addHeader(
        RequestIdFilter.HEADER_NAME, "  abc.DEF-123_:\t".replace("\t", "  ")); // valid after trim

    String expected = "abc.DEF-123_:";

    filter.doFilter(
        req,
        res,
        (r, s) -> {
          assertThat(MDC.get(RequestIdFilter.MDC_KEY)).isEqualTo(expected);
          assertThat(req.getAttribute(RequestIdFilter.REQ_ID_ATTR)).isEqualTo(expected);
          assertThat(res.getHeader(RequestIdFilter.HEADER_NAME)).isEqualTo(expected);
        });

    assertThat(MDC.get(RequestIdFilter.MDC_KEY)).isNull();
  }

  @Test
  void doFilterInternal_invalidHeader_generatesUuid_andCleansUp_evenWhenChainThrows() {
    RequestIdFilter filter = new RequestIdFilter();

    MockHttpServletRequest req = new MockHttpServletRequest("GET", "/x");
    MockHttpServletResponse res = new MockHttpServletResponse();

    req.addHeader(RequestIdFilter.HEADER_NAME, "bad id");

    final String[] seen = new String[1];

    assertThatThrownBy(
            () ->
                filter.doFilter(
                    req,
                    res,
                    (FilterChain)
                        (r, s) -> {
                          seen[0] = MDC.get(RequestIdFilter.MDC_KEY);
                          assertThat(seen[0]).isNotBlank();
                          throw new RuntimeException("boom");
                        }))
        .isInstanceOf(RuntimeException.class);

    assertThat(seen[0]).isNotNull();
    assertThat(UUID.fromString(seen[0])).isNotNull();

    assertThat(req.getAttribute(RequestIdFilter.REQ_ID_ATTR)).isEqualTo(seen[0]);
    assertThat(res.getHeader(RequestIdFilter.HEADER_NAME)).isEqualTo(seen[0]);

    assertThat(MDC.get(RequestIdFilter.MDC_KEY)).isNull();
  }

  @Test
  void private_isValidRequestId_branches_areCoveredViaReflection() throws Exception {
    Method isValid = RequestIdFilter.class.getDeclaredMethod("isValidRequestId", String.class);
    isValid.setAccessible(true);

    assertThat((boolean) isValid.invoke(null, (String) null)).isFalse();

    assertThat((boolean) isValid.invoke(null, "   ")).isFalse();

    assertThat((boolean) isValid.invoke(null, "a".repeat(129))).isFalse();

    assertThat((boolean) isValid.invoke(null, "abc\rdef")).isFalse();
    assertThat((boolean) isValid.invoke(null, "abc\ndef")).isFalse();
    assertThat((boolean) isValid.invoke(null, "abc\tdef")).isFalse();

    assertThat((boolean) isValid.invoke(null, "abc def")).isFalse();

    assertThat((boolean) isValid.invoke(null, "  a-b.c_d:1  ")).isTrue();
  }

  @Test
  void doFilterInternal_nullHeader_generatesUuid() throws ServletException, IOException {
    RequestIdFilter filter = new RequestIdFilter();

    MockHttpServletRequest req = new MockHttpServletRequest("GET", "/x");
    MockHttpServletResponse res = new MockHttpServletResponse();

    filter.doFilter(
        req,
        res,
        (r, s) -> {
          String id = (String) req.getAttribute(RequestIdFilter.REQ_ID_ATTR);
          assertThat(id).isNotBlank();
          assertThat(UUID.fromString(id)).isNotNull();
          assertThat(res.getHeader(RequestIdFilter.HEADER_NAME)).isEqualTo(id);
          assertThat(MDC.get(RequestIdFilter.MDC_KEY)).isEqualTo(id);
        });

    assertThat(MDC.get(RequestIdFilter.MDC_KEY)).isNull();
  }
}
