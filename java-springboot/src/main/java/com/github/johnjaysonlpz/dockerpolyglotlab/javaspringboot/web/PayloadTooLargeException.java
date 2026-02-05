package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.web;

import java.io.IOException;

public final class PayloadTooLargeException extends IOException {
  private final long maxBodyBytes;

  public PayloadTooLargeException(long maxBodyBytes) {
    super("payload exceeds " + maxBodyBytes + " bytes");
    this.maxBodyBytes = maxBodyBytes;
  }

  public long getMaxBodyBytes() {
    return maxBodyBytes;
  }
}
