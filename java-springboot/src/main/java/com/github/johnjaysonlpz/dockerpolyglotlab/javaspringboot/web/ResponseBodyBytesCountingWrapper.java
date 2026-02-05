package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.web;

import jakarta.servlet.ServletOutputStream;
import jakarta.servlet.WriteListener;
import jakarta.servlet.http.HttpServletResponse;
import jakarta.servlet.http.HttpServletResponseWrapper;
import java.io.IOException;
import java.io.OutputStreamWriter;
import java.io.PrintWriter;
import java.nio.charset.Charset;
import java.nio.charset.StandardCharsets;

public class ResponseBodyBytesCountingWrapper extends HttpServletResponseWrapper {

  private CountingServletOutputStream countingOutputStream;
  private PrintWriter countingWriter;

  public ResponseBodyBytesCountingWrapper(HttpServletResponse response) {
    super(response);
  }

  @Override
  public ServletOutputStream getOutputStream() throws IOException {
    if (countingWriter != null) {
      throw new IllegalStateException("getWriter() has already been called for this response");
    }
    if (countingOutputStream == null) {
      countingOutputStream = new CountingServletOutputStream(super.getOutputStream());
    }
    return countingOutputStream;
  }

  @Override
  public PrintWriter getWriter() throws IOException {
    if (countingWriter == null) {
      ServletOutputStream os = getOutputStream();
      Charset cs = resolveCharset();
      countingWriter = new PrintWriter(new OutputStreamWriter(os, cs), true);
    }
    return countingWriter;
  }

  public long getBytesWritten() {
    return (countingOutputStream == null) ? 0L : countingOutputStream.getBytesWritten();
  }

  public void flushCountingBuffers() throws IOException {
    if (countingWriter != null) {
      countingWriter.flush();
    }
    if (countingOutputStream != null) {
      countingOutputStream.flush();
    }
    super.flushBuffer();
  }

  @Override
  public void flushBuffer() throws IOException {
    flushCountingBuffers();
  }

  @Override
  public void resetBuffer() {
    super.resetBuffer();
    if (countingOutputStream != null) {
      countingOutputStream.resetCount();
    }
  }

  @Override
  public void reset() {
    super.reset();
    countingWriter = null;
    countingOutputStream = null;
  }

  private Charset resolveCharset() {
    try {
      String enc = getCharacterEncoding();
      if (enc == null || enc.isBlank()) return StandardCharsets.UTF_8;
      return Charset.forName(enc);
    } catch (Exception ignored) {
      return StandardCharsets.UTF_8;
    }
  }

  private static final class CountingServletOutputStream extends ServletOutputStream {

    private final ServletOutputStream delegate;
    private long bytesWritten = 0L;

    CountingServletOutputStream(ServletOutputStream delegate) {
      this.delegate = delegate;
    }

    long getBytesWritten() {
      return bytesWritten;
    }

    void resetCount() {
      this.bytesWritten = 0L;
    }

    @Override
    public void write(int b) throws IOException {
      delegate.write(b);
      bytesWritten += 1;
    }

    @Override
    public void write(byte[] b) throws IOException {
      delegate.write(b);
      bytesWritten += b.length;
    }

    @Override
    public void write(byte[] b, int off, int len) throws IOException {
      delegate.write(b, off, len);
      if (len > 0) bytesWritten += len;
    }

    @Override
    public void flush() throws IOException {
      delegate.flush();
    }

    @Override
    public void close() throws IOException {
      delegate.close();
    }

    @Override
    public boolean isReady() {
      return delegate.isReady();
    }

    @Override
    public void setWriteListener(WriteListener writeListener) {
      delegate.setWriteListener(writeListener);
    }
  }
}
