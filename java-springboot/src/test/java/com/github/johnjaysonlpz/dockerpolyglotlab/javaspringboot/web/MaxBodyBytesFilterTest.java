package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.web;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.Mockito.*;

import com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.config.ServiceProperties;
import jakarta.servlet.ReadListener;
import jakarta.servlet.ServletException;
import jakarta.servlet.ServletInputStream;
import java.io.BufferedReader;
import java.io.IOException;
import java.lang.reflect.Constructor;
import java.lang.reflect.Method;
import java.nio.charset.Charset;
import java.nio.charset.StandardCharsets;
import org.junit.jupiter.api.Test;
import org.springframework.mock.web.MockHttpServletRequest;
import org.springframework.mock.web.MockHttpServletResponse;
import tools.jackson.databind.ObjectMapper;

class MaxBodyBytesFilterTest {

  @Test
  void limitDisabled_passesThrough_withoutWriting413() throws Exception {
    ServiceProperties props = mock(ServiceProperties.class);
    ObjectMapper mapper = mock(ObjectMapper.class);
    when(props.getMaxBodyBytes()).thenReturn(0L);

    MaxBodyBytesFilter filter = new MaxBodyBytesFilter(props, mapper);

    MockHttpServletRequest req = new MockHttpServletRequest("POST", "/x");
    MockHttpServletResponse res = new MockHttpServletResponse();

    filter.doFilter(
        req,
        res,
        (r, s) -> {
          res.setStatus(200);
          res.getWriter().write("ok");
        });

    assertThat(res.getStatus()).isEqualTo(200);
    verifyNoInteractions(mapper);
  }

  @Test
  void contentLengthExceedsLimit_writes413_andSkipsChain() throws Exception {
    ServiceProperties props = mock(ServiceProperties.class);
    ObjectMapper mapper = mock(ObjectMapper.class);
    when(props.getMaxBodyBytes()).thenReturn(5L);

    MaxBodyBytesFilter filter = new MaxBodyBytesFilter(props, mapper);

    MockHttpServletRequest req =
        new MockHttpServletRequest("POST", "/x") {
          @Override
          public int getContentLength() {
            return 10;
          }
        };
    MockHttpServletResponse res = new MockHttpServletResponse();

    filter.doFilter(req, res, (r, s) -> fail("chain should not run"));

    assertThat(res.getStatus()).isEqualTo(413);
    assertThat(res.getContentType()).isEqualTo("application/json;charset=UTF-8");
    assertThat(res.getCharacterEncoding()).isEqualTo("UTF-8");
    assertThat(res.getHeader(RequestIdFilter.HEADER_NAME)).isNotBlank();
    verify(mapper).writeValue(any(java.io.Writer.class), any());
  }

  @Test
  void withinLimit_wrapsRequest_andRunsChainSuccessfully() throws Exception {
    ServiceProperties props = mock(ServiceProperties.class);
    ObjectMapper mapper = mock(ObjectMapper.class);
    when(props.getMaxBodyBytes()).thenReturn(100L);

    MaxBodyBytesFilter filter = new MaxBodyBytesFilter(props, mapper);

    MockHttpServletRequest req = new MockHttpServletRequest("POST", "/x");
    req.setContent("hello".getBytes(StandardCharsets.UTF_8));
    MockHttpServletResponse res = new MockHttpServletResponse();

    final boolean[] sawWrapped = {false};

    filter.doFilter(
        req,
        res,
        (r, s) -> {
          sawWrapped[0] = (r != req);

          ServletInputStream in1 = r.getInputStream();
          ServletInputStream in2 = r.getInputStream();
          assertThat(in2).isSameAs(in1);

          BufferedReader br = r.getReader();
          assertThat(br.readLine()).isEqualTo("hello");

          res.setStatus(204);
        });

    assertThat(sawWrapped[0]).isTrue();
    assertThat(res.getStatus()).isEqualTo(204);
    verifyNoInteractions(mapper);
  }

  @Test
  void streamExceedsLimit_unknownContentLength_caughtPayloadTooLarge_writes413() throws Exception {
    ServiceProperties props = mock(ServiceProperties.class);
    ObjectMapper mapper = mock(ObjectMapper.class);
    when(props.getMaxBodyBytes()).thenReturn(5L);

    MaxBodyBytesFilter filter = new MaxBodyBytesFilter(props, mapper);

    MockHttpServletRequest req =
        new MockHttpServletRequest("POST", "/x") {
          @Override
          public int getContentLength() {
            return -1;
          }
        };
    req.setContent(new byte[50]);
    req.addHeader(RequestIdFilter.HEADER_NAME, "hdr-id");
    MockHttpServletResponse res = new MockHttpServletResponse();

    filter.doFilter(
        req,
        res,
        (r, s) -> {
          byte[] buf = new byte[64];
          while (r.getInputStream().read(buf) != -1) {}
        });

    assertThat(res.getStatus()).isEqualTo(413);
    assertThat(res.getHeader(RequestIdFilter.HEADER_NAME)).isEqualTo("hdr-id");
    verify(mapper).writeValue(any(java.io.Writer.class), any());
  }

  @Test
  void streamExceedsLimit_byteByByte_hitsReadMethod_andThrowsBranch() throws Exception {
    ServiceProperties props = mock(ServiceProperties.class);
    ObjectMapper mapper = mock(ObjectMapper.class);
    when(props.getMaxBodyBytes()).thenReturn(1L);

    MaxBodyBytesFilter filter = new MaxBodyBytesFilter(props, mapper);

    MockHttpServletRequest req =
        new MockHttpServletRequest("POST", "/x") {
          @Override
          public int getContentLength() {
            return -1;
          }
        };
    req.setContent(new byte[] {1, 2});
    MockHttpServletResponse res = new MockHttpServletResponse();

    filter.doFilter(
        req,
        res,
        (r, s) -> {
          assertThat(r.getInputStream().read()).isNotEqualTo(-1);
          r.getInputStream().read();
        });

    assertThat(res.getStatus()).isEqualTo(413);
    verify(mapper).writeValue(any(java.io.Writer.class), any());
  }

  @Test
  void payloadTooLarge_committedResponse_isRethrown() throws Exception {
    ServiceProperties props = mock(ServiceProperties.class);
    ObjectMapper mapper = mock(ObjectMapper.class);
    when(props.getMaxBodyBytes()).thenReturn(5L);

    MaxBodyBytesFilter filter = new MaxBodyBytesFilter(props, mapper);

    MockHttpServletRequest req = new MockHttpServletRequest("POST", "/x");
    MockHttpServletResponse res = new MockHttpServletResponse();

    assertThatThrownBy(
            () ->
                filter.doFilter(
                    req,
                    res,
                    (r, s) -> {
                      res.setStatus(200);
                      res.getWriter().write("committed");
                      res.flushBuffer();
                      throw new PayloadTooLargeException(5L);
                    }))
        .isInstanceOf(PayloadTooLargeException.class);

    verifyNoInteractions(mapper);
  }

  @Test
  void servletExceptionWrappingPayloadTooLarge_uncommitted_writes413() throws Exception {
    ServiceProperties props = mock(ServiceProperties.class);
    ObjectMapper mapper = mock(ObjectMapper.class);
    when(props.getMaxBodyBytes()).thenReturn(5L);

    MaxBodyBytesFilter filter = new MaxBodyBytesFilter(props, mapper);

    MockHttpServletRequest req = new MockHttpServletRequest("POST", "/x");
    MockHttpServletResponse res = new MockHttpServletResponse();

    filter.doFilter(
        req,
        res,
        (r, s) -> {
          throw new ServletException(new PayloadTooLargeException(5L));
        });

    assertThat(res.getStatus()).isEqualTo(413);
    verify(mapper).writeValue(any(java.io.Writer.class), any());
  }

  @Test
  void servletExceptionWrappingPayloadTooLarge_committed_isRethrown() throws Exception {
    ServiceProperties props = mock(ServiceProperties.class);
    ObjectMapper mapper = mock(ObjectMapper.class);
    when(props.getMaxBodyBytes()).thenReturn(5L);

    MaxBodyBytesFilter filter = new MaxBodyBytesFilter(props, mapper);

    MockHttpServletRequest req = new MockHttpServletRequest("POST", "/x");
    MockHttpServletResponse res = new MockHttpServletResponse();

    assertThatThrownBy(
            () ->
                filter.doFilter(
                    req,
                    res,
                    (r, s) -> {
                      res.getWriter().write("x");
                      res.flushBuffer();
                      throw new ServletException(new PayloadTooLargeException(5L));
                    }))
        .isInstanceOf(ServletException.class);

    verifyNoInteractions(mapper);
  }

  @Test
  void servletExceptionWithoutPayloadCause_isRethrown() throws Exception {
    ServiceProperties props = mock(ServiceProperties.class);
    ObjectMapper mapper = mock(ObjectMapper.class);
    when(props.getMaxBodyBytes()).thenReturn(5L);

    MaxBodyBytesFilter filter = new MaxBodyBytesFilter(props, mapper);

    MockHttpServletRequest req = new MockHttpServletRequest("POST", "/x");
    MockHttpServletResponse res = new MockHttpServletResponse();

    assertThatThrownBy(
            () ->
                filter.doFilter(
                    req,
                    res,
                    (r, s) -> {
                      throw new ServletException("nope");
                    }))
        .isInstanceOf(ServletException.class);

    verifyNoInteractions(mapper);
  }

  @Test
  void privateHelpers_andDelegates_areCoveredViaReflection() throws Exception {
    ServiceProperties props = mock(ServiceProperties.class);
    ObjectMapper mapper = mock(ObjectMapper.class);
    when(props.getMaxBodyBytes()).thenReturn(5L);
    MaxBodyBytesFilter filter = new MaxBodyBytesFilter(props, mapper);

    Method requestId =
        MaxBodyBytesFilter.class.getDeclaredMethod(
            "requestId", jakarta.servlet.http.HttpServletRequest.class);
    requestId.setAccessible(true);

    MockHttpServletRequest r1 = new MockHttpServletRequest();
    r1.setAttribute(RequestIdFilter.REQ_ID_ATTR, "attr-id");
    assertThat((String) requestId.invoke(null, r1)).isEqualTo("attr-id");

    MockHttpServletRequest rBlankAttr = new MockHttpServletRequest();
    rBlankAttr.setAttribute(RequestIdFilter.REQ_ID_ATTR, "   ");
    rBlankAttr.addHeader(RequestIdFilter.HEADER_NAME, " hdr-id ");
    assertThat((String) requestId.invoke(null, rBlankAttr)).isEqualTo("hdr-id");

    MockHttpServletRequest rBlankHdr = new MockHttpServletRequest();
    rBlankHdr.addHeader(RequestIdFilter.HEADER_NAME, "   ");
    String rid = (String) requestId.invoke(null, rBlankHdr);
    assertThat(rid).isNotBlank();

    Method findCause =
        MaxBodyBytesFilter.class.getDeclaredMethod("findCause", Throwable.class, Class.class);
    findCause.setAccessible(true);

    PayloadTooLargeException p = new PayloadTooLargeException(1L);
    ServletException seDirect = new ServletException(p);
    assertThat(findCause.invoke(null, seDirect, PayloadTooLargeException.class)).isSameAs(p);

    PayloadTooLargeException deep = new PayloadTooLargeException(2L);
    RuntimeException mid = new RuntimeException("mid", deep);
    ServletException seDeep = new ServletException(mid);
    assertThat(findCause.invoke(null, seDeep, PayloadTooLargeException.class)).isSameAs(deep);

    ServletException none = new ServletException(new IllegalArgumentException("x"));
    assertThat(findCause.invoke(null, none, PayloadTooLargeException.class)).isNull();

    Throwable chain = new RuntimeException("0");
    Throwable cur = chain;
    PayloadTooLargeException at25 = null;
    for (int i = 1; i <= 30; i++) {
      Throwable next =
          (i == 25)
              ? (at25 = new PayloadTooLargeException(25L))
              : new RuntimeException(String.valueOf(i));
      cur.initCause(next);
      cur = next;
    }
    assertThat(at25).isNotNull();
    assertThat(findCause.invoke(null, chain, PayloadTooLargeException.class)).isNull();

    Method write413 =
        MaxBodyBytesFilter.class.getDeclaredMethod(
            "write413",
            jakarta.servlet.http.HttpServletRequest.class,
            jakarta.servlet.http.HttpServletResponse.class);
    write413.setAccessible(true);

    MockHttpServletRequest wrReq = new MockHttpServletRequest();
    MockHttpServletResponse wrRes = new MockHttpServletResponse();
    wrRes.setStatus(200);
    wrRes.getWriter().write("committed");
    wrRes.flushBuffer();
    write413.invoke(filter, wrReq, wrRes);
    assertThat(wrRes.getStatus()).isEqualTo(200);
    verifyNoInteractions(mapper);

    Class<?> wrapperClass =
        Class.forName(
            "com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.web.MaxBodyBytesFilter$LimitedBodyRequestWrapper");
    Constructor<?> ctor =
        wrapperClass.getDeclaredConstructor(
            jakarta.servlet.http.HttpServletRequest.class, long.class);
    ctor.setAccessible(true);

    Method getReader = wrapperClass.getDeclaredMethod("getReader");
    getReader.setAccessible(true);

    MockHttpServletRequest wReqBlankEnc = new MockHttpServletRequest();
    wReqBlankEnc.setCharacterEncoding("   ");
    wReqBlankEnc.setContent("hello".getBytes(StandardCharsets.UTF_8));
    Object wBlankEnc = ctor.newInstance(wReqBlankEnc, 100L);
    BufferedReader br = (BufferedReader) getReader.invoke(wBlankEnc);
    assertThat(br.readLine()).isEqualTo("hello");

    MockHttpServletRequest wReqBadEnc = new MockHttpServletRequest();
    wReqBadEnc.setCharacterEncoding("NO_SUCH_CHARSET_123");
    wReqBadEnc.setContent("hello".getBytes(StandardCharsets.UTF_8));
    Object wBadEnc = ctor.newInstance(wReqBadEnc, 100L);
    BufferedReader brBad = (BufferedReader) getReader.invoke(wBadEnc);
    assertThat(brBad.readLine()).isEqualTo("hello");

    Method resolveCharset = wrapperClass.getDeclaredMethod("resolveCharset", String.class);
    resolveCharset.setAccessible(true);
    Charset cs = (Charset) resolveCharset.invoke(null, "UTF-16");
    assertThat(cs.toString()).isEqualTo("UTF-16");

    MockHttpServletRequest wReqGoodEnc = new MockHttpServletRequest();
    wReqGoodEnc.setCharacterEncoding("UTF-16");
    wReqGoodEnc.setContent("hello".getBytes(Charset.forName("UTF-16")));
    Object wGoodEnc = ctor.newInstance(wReqGoodEnc, 100L);
    BufferedReader brGood = (BufferedReader) getReader.invoke(wGoodEnc);
    assertThat(brGood.readLine()).isEqualTo("hello");

    Class<?> limitedClass =
        Class.forName(
            "com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.web.MaxBodyBytesFilter$LimitedServletInputStream");
    Constructor<?> limitedCtor =
        limitedClass.getDeclaredConstructor(ServletInputStream.class, long.class);
    limitedCtor.setAccessible(true);

    ServletInputStream delegate = new TestServletInputStream(new byte[0]);
    ServletInputStream limited = (ServletInputStream) limitedCtor.newInstance(delegate, 100L);

    limited.isReady();
    limited.isFinished();
    limited.setReadListener(mock(ReadListener.class));
    limited.close();

    assertThat(limited.read()).isEqualTo(-1);
  }

  private static void fail(String msg) {
    throw new AssertionError(msg);
  }

  private static final class TestServletInputStream extends ServletInputStream {
    private final byte[] data;
    private int idx = 0;

    @SuppressWarnings("unused")
    private ReadListener readListener;

    private TestServletInputStream(byte[] data) {
      this.data = data;
    }

    @Override
    public int read() throws IOException {
      if (idx >= data.length) return -1;
      return data[idx++] & 0xff;
    }

    @Override
    public int read(byte[] b, int off, int len) throws IOException {
      if (idx >= data.length) return -1;
      int n = Math.min(len, data.length - idx);
      System.arraycopy(data, idx, b, off, n);
      idx += n;
      return n;
    }

    @Override
    public boolean isFinished() {
      return idx >= data.length;
    }

    @Override
    public boolean isReady() {
      return true;
    }

    @Override
    public void setReadListener(ReadListener readListener) {
      this.readListener = readListener;
    }

    @Override
    public void close() throws IOException {
      // no-op
    }
  }
}
