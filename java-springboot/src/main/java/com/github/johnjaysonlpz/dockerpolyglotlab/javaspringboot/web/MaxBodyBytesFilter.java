package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.web;

import com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.config.ServiceProperties;
import jakarta.servlet.FilterChain;
import jakarta.servlet.ServletException;
import jakarta.servlet.ServletInputStream;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletRequestWrapper;
import jakarta.servlet.http.HttpServletResponse;
import java.io.BufferedReader;
import java.io.IOException;
import java.io.InputStreamReader;
import java.nio.charset.Charset;
import java.nio.charset.StandardCharsets;
import java.util.UUID;
import org.springframework.core.Ordered;
import org.springframework.core.annotation.Order;
import org.springframework.stereotype.Component;
import org.springframework.web.filter.OncePerRequestFilter;
import tools.jackson.databind.ObjectMapper;

@Component
@Order(Ordered.HIGHEST_PRECEDENCE + 20)
public class MaxBodyBytesFilter extends OncePerRequestFilter {

  private static final String MSG = "payload too large";
  private static final String CODE = "payload_too_large";

  private final ServiceProperties props;
  private final ObjectMapper objectMapper;

  public MaxBodyBytesFilter(ServiceProperties props, ObjectMapper objectMapper) {
    this.props = props;
    this.objectMapper = objectMapper;
  }

  @Override
  protected void doFilterInternal(
      HttpServletRequest request, HttpServletResponse response, FilterChain filterChain)
      throws ServletException, IOException {

    long limit = props.getMaxBodyBytes();
    if (limit <= 0) {
      filterChain.doFilter(request, response);
      return;
    }

    int contentLength = request.getContentLength();
    if (contentLength > 0 && contentLength > limit) {
      write413(request, response);
      return;
    }

    HttpServletRequest wrapped = new LimitedBodyRequestWrapper(request, limit);

    try {
      filterChain.doFilter(wrapped, response);
    } catch (PayloadTooLargeException e) {
      if (!response.isCommitted()) {
        write413(request, response);
        return;
      }
      throw e;
    } catch (ServletException se) {
      PayloadTooLargeException e = findCause(se, PayloadTooLargeException.class);
      if (e != null && !response.isCommitted()) {
        write413(request, response);
        return;
      }
      throw se;
    }
  }

  private void write413(HttpServletRequest request, HttpServletResponse response)
      throws IOException {
    if (response.isCommitted()) return;

    String rid = requestId(request);
    response.resetBuffer();
    response.setStatus(HttpServletResponse.SC_REQUEST_ENTITY_TOO_LARGE);
    response.setContentType("application/json");
    response.setCharacterEncoding(StandardCharsets.UTF_8.name());

    response.setHeader(RequestIdFilter.HEADER_NAME, rid);

    ErrorResponse body = new ErrorResponse(MSG, CODE, rid);
    objectMapper.writeValue(response.getWriter(), body);
    response.flushBuffer();
  }

  private static String requestId(HttpServletRequest request) {
    Object v = request.getAttribute(RequestIdFilter.REQ_ID_ATTR);
    if (v instanceof String s && !s.isBlank()) return s;
    String hdr = request.getHeader(RequestIdFilter.HEADER_NAME);
    if (hdr != null && !hdr.isBlank()) return hdr.trim();
    return UUID.randomUUID().toString();
  }

  private static <T extends Throwable> T findCause(Throwable t, Class<T> want) {
    Throwable cur = t;
    int guard = 0;
    while (cur != null && guard++ < 20) {
      if (want.isInstance(cur)) return want.cast(cur);
      cur = cur.getCause();
    }
    return null;
  }

  private static final class LimitedBodyRequestWrapper extends HttpServletRequestWrapper {
    private final long limit;
    private ServletInputStream limitedStream;

    LimitedBodyRequestWrapper(HttpServletRequest request, long limit) {
      super(request);
      this.limit = limit;
    }

    @Override
    public ServletInputStream getInputStream() throws IOException {
      if (limitedStream == null) {
        limitedStream = new LimitedServletInputStream(super.getInputStream(), limit);
      }
      return limitedStream;
    }

    @Override
    public BufferedReader getReader() throws IOException {
      Charset cs = resolveCharset(getCharacterEncoding());
      return new BufferedReader(new InputStreamReader(getInputStream(), cs));
    }

    private static Charset resolveCharset(String enc) {
      if (enc == null || enc.isBlank()) return StandardCharsets.UTF_8;
      try {
        return Charset.forName(enc);
      } catch (Exception ignored) {
        return StandardCharsets.UTF_8;
      }
    }
  }

  private static final class LimitedServletInputStream extends ServletInputStream {
    private final ServletInputStream delegate;
    private final long limit;
    private long count = 0L;

    LimitedServletInputStream(ServletInputStream delegate, long limit) {
      this.delegate = delegate;
      this.limit = limit;
    }

    @Override
    public int read() throws IOException {
      int b = delegate.read();
      if (b == -1) return -1;
      count++;
      if (count > limit) throw new PayloadTooLargeException(limit);
      return b;
    }

    @Override
    public int read(byte[] b, int off, int len) throws IOException {
      int n = delegate.read(b, off, len);
      if (n <= 0) return n;
      count += n;
      if (count > limit) throw new PayloadTooLargeException(limit);
      return n;
    }

    @Override
    public boolean isReady() {
      return delegate.isReady();
    }

    @Override
    public void setReadListener(jakarta.servlet.ReadListener readListener) {
      delegate.setReadListener(readListener);
    }

    @Override
    public boolean isFinished() {
      return delegate.isFinished();
    }

    @Override
    public void close() throws IOException {
      delegate.close();
    }
  }
}
