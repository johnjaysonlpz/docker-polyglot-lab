package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.web;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.Mockito.mock;

import jakarta.servlet.ServletOutputStream;
import jakarta.servlet.WriteListener;
import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.PrintWriter;
import java.lang.reflect.Constructor;
import java.lang.reflect.Method;
import java.nio.charset.StandardCharsets;
import org.junit.jupiter.api.Test;
import org.springframework.mock.web.MockHttpServletResponse;

class ResponseBodyBytesCountingWrapperTest {

  @Test
  void getWriter_thenGetOutputStream_throwsIllegalState() throws Exception {
    MockHttpServletResponse res = new MockHttpServletResponse();
    ResponseBodyBytesCountingWrapper w = new ResponseBodyBytesCountingWrapper(res);

    w.getWriter();
    assertThatThrownBy(w::getOutputStream).isInstanceOf(IllegalStateException.class);
  }

  @Test
  void getWriter_isIdempotent_secondCallDoesNotRecreateWriter() throws Exception {
    MockHttpServletResponse res = new MockHttpServletResponse();
    ResponseBodyBytesCountingWrapper w = new ResponseBodyBytesCountingWrapper(res);

    PrintWriter w1 = w.getWriter();
    PrintWriter w2 = w.getWriter();

    assertThat(w2).isSameAs(w1);
  }

  @Test
  void getOutputStream_isIdempotent_secondCallDoesNotRecreateStream() throws Exception {
    MockHttpServletResponse res = new MockHttpServletResponse();
    ResponseBodyBytesCountingWrapper w = new ResponseBodyBytesCountingWrapper(res);

    ServletOutputStream s1 = w.getOutputStream();
    ServletOutputStream s2 = w.getOutputStream();

    assertThat(s2).isSameAs(s1);
  }

  @Test
  void flushCountingBuffers_withNoWriterAndNoStream_doesNotThrow() throws Exception {
    MockHttpServletResponse res = new MockHttpServletResponse();
    ResponseBodyBytesCountingWrapper w = new ResponseBodyBytesCountingWrapper(res);

    w.flushCountingBuffers();

    assertThat(w.getBytesWritten()).isEqualTo(0L);
  }

  @Test
  void flushCountingBuffers_flushesStreamWhenPresent() throws Exception {
    MockHttpServletResponse res = new MockHttpServletResponse();
    ResponseBodyBytesCountingWrapper w = new ResponseBodyBytesCountingWrapper(res);

    w.getOutputStream().write('x');

    w.flushCountingBuffers();

    assertThat(w.getBytesWritten()).isEqualTo(1L);
  }

  @Test
  void resetBuffer_withNoStream_doesNotThrow() {
    MockHttpServletResponse res = new MockHttpServletResponse();
    ResponseBodyBytesCountingWrapper w = new ResponseBodyBytesCountingWrapper(res);

    w.resetBuffer();

    assertThat(w.getBytesWritten()).isEqualTo(0L);
  }

  @Test
  void resetBuffer_resetsCount_whenStreamPresent() throws Exception {
    MockHttpServletResponse res = new MockHttpServletResponse();
    ResponseBodyBytesCountingWrapper w = new ResponseBodyBytesCountingWrapper(res);

    w.getOutputStream().write(new byte[] {1, 2, 3});
    assertThat(w.getBytesWritten()).isEqualTo(3L);

    w.resetBuffer();

    assertThat(w.getBytesWritten()).isEqualTo(0L);
  }

  @Test
  void getWriter_writesBytes_andGetBytesWrittenReflectsIt() throws Exception {
    MockHttpServletResponse res = new MockHttpServletResponse();
    ResponseBodyBytesCountingWrapper w = new ResponseBodyBytesCountingWrapper(res);

    PrintWriter pw = w.getWriter();
    pw.write("ok");
    w.flushBuffer();

    assertThat(w.getBytesWritten()).isGreaterThanOrEqualTo(2L);
  }

  @Test
  void getOutputStream_countsAllWriteOverloads_andResetBufferResetsCount() throws Exception {
    MockHttpServletResponse res = new MockHttpServletResponse();
    ResponseBodyBytesCountingWrapper w = new ResponseBodyBytesCountingWrapper(res);

    ServletOutputStream os = w.getOutputStream();

    os.write('a');
    os.write(new byte[] {1, 2, 3});
    os.write(new byte[] {9, 8, 7, 6}, 1, 2);
    os.write(new byte[] {9, 9, 9}, 0, 0);

    assertThat(w.getBytesWritten()).isEqualTo(1L + 3L + 2L);

    w.resetBuffer();
    assertThat(w.getBytesWritten()).isEqualTo(0L);

    os.write('z');
    assertThat(w.getBytesWritten()).isEqualTo(1L);

    w.flushCountingBuffers();
  }

  @Test
  void reset_clearsWriterAndStream_andBytesWrittenBecomesZero() throws Exception {
    MockHttpServletResponse res = new MockHttpServletResponse();
    ResponseBodyBytesCountingWrapper w = new ResponseBodyBytesCountingWrapper(res);

    w.getOutputStream().write('x');
    assertThat(w.getBytesWritten()).isEqualTo(1L);

    w.reset();
    assertThat(w.getBytesWritten()).isEqualTo(0L);

    w.getWriter().write("hi");
    w.flushBuffer();
    assertThat(w.getBytesWritten()).isGreaterThanOrEqualTo(2L);
  }

  @Test
  void getBytesWritten_returnsZero_whenOutputStreamNeverCreated() {
    MockHttpServletResponse res = new MockHttpServletResponse();
    ResponseBodyBytesCountingWrapper w = new ResponseBodyBytesCountingWrapper(res);

    assertThat(w.getBytesWritten()).isEqualTo(0L);
  }

  @Test
  void resolveCharset_branches_areCoveredViaReflection() throws Exception {
    class EncResponse extends MockHttpServletResponse {
      private final String enc;

      EncResponse(String enc) {
        this.enc = enc;
      }

      @Override
      public String getCharacterEncoding() {
        return enc;
      }
    }

    Method resolveCharset =
        ResponseBodyBytesCountingWrapper.class.getDeclaredMethod("resolveCharset");
    resolveCharset.setAccessible(true);

    ResponseBodyBytesCountingWrapper wNull =
        new ResponseBodyBytesCountingWrapper(new EncResponse(null));
    assertThat(resolveCharset.invoke(wNull)).isEqualTo(StandardCharsets.UTF_8);

    ResponseBodyBytesCountingWrapper wBlank =
        new ResponseBodyBytesCountingWrapper(new EncResponse("   "));
    assertThat(resolveCharset.invoke(wBlank)).isEqualTo(StandardCharsets.UTF_8);

    ResponseBodyBytesCountingWrapper wBad =
        new ResponseBodyBytesCountingWrapper(new EncResponse("NO_SUCH_CHARSET_123"));
    assertThat(resolveCharset.invoke(wBad)).isEqualTo(StandardCharsets.UTF_8);

    ResponseBodyBytesCountingWrapper wUtf16 =
        new ResponseBodyBytesCountingWrapper(new EncResponse("UTF-16"));
    assertThat(resolveCharset.invoke(wUtf16).toString()).isEqualTo("UTF-16");
  }

  @Test
  void countingServletOutputStream_delegates_areCoveredViaReflection() throws Exception {
    Class<?> inner =
        Class.forName(
            "com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.web.ResponseBodyBytesCountingWrapper$CountingServletOutputStream");

    Constructor<?> ctor = inner.getDeclaredConstructor(ServletOutputStream.class);
    ctor.setAccessible(true);

    ByteArrayOutputStream sink = new ByteArrayOutputStream();

    ServletOutputStream delegate =
        new ServletOutputStream() {
          @Override
          public void write(int b) throws IOException {
            sink.write(b);
          }

          @Override
          public void write(byte[] b) throws IOException {
            sink.write(b);
          }

          @Override
          public void write(byte[] b, int off, int len) throws IOException {
            sink.write(b, off, len);
          }

          @Override
          public void flush() throws IOException {
            sink.flush();
          }

          @Override
          public void close() throws IOException {
            sink.close();
          }

          @Override
          public boolean isReady() {
            return true;
          }

          @Override
          public void setWriteListener(WriteListener writeListener) {
            // no-op
          }
        };

    Object counting = ctor.newInstance(delegate);

    Method getBytesWritten = inner.getDeclaredMethod("getBytesWritten");
    getBytesWritten.setAccessible(true);
    Method resetCount = inner.getDeclaredMethod("resetCount");
    resetCount.setAccessible(true);

    Method writeInt = inner.getDeclaredMethod("write", int.class);
    writeInt.setAccessible(true);
    Method writeArr = inner.getDeclaredMethod("write", byte[].class);
    writeArr.setAccessible(true);
    Method writeArrOffLen = inner.getDeclaredMethod("write", byte[].class, int.class, int.class);
    writeArrOffLen.setAccessible(true);

    Method flush = inner.getDeclaredMethod("flush");
    flush.setAccessible(true);
    Method close = inner.getDeclaredMethod("close");
    close.setAccessible(true);
    Method isReady = inner.getDeclaredMethod("isReady");
    isReady.setAccessible(true);
    Method setWriteListener = inner.getDeclaredMethod("setWriteListener", WriteListener.class);
    setWriteListener.setAccessible(true);

    writeInt.invoke(counting, (int) 'a');
    writeArr.invoke(counting, (Object) new byte[] {1, 2, 3});
    writeArrOffLen.invoke(counting, new byte[] {9, 8, 7}, 0, 2);
    writeArrOffLen.invoke(counting, new byte[] {9, 8, 7}, 0, 0);

    assertThat((long) getBytesWritten.invoke(counting)).isEqualTo(1L + 3L + 2L);

    resetCount.invoke(counting);
    assertThat((long) getBytesWritten.invoke(counting)).isEqualTo(0L);

    flush.invoke(counting);
    assertThat((boolean) isReady.invoke(counting)).isTrue();
    setWriteListener.invoke(counting, mock(WriteListener.class));
    close.invoke(counting);
  }
}
