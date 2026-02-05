package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.web;

import jakarta.servlet.http.HttpServletRequest;
import java.util.Locale;
import java.util.UUID;
import org.springframework.core.Ordered;
import org.springframework.core.annotation.Order;
import org.springframework.http.HttpStatus;
import org.springframework.http.MediaType;
import org.springframework.http.ResponseEntity;
import org.springframework.web.ErrorResponseException;
import org.springframework.web.HttpRequestMethodNotSupportedException;
import org.springframework.web.bind.MethodArgumentNotValidException;
import org.springframework.web.bind.annotation.ExceptionHandler;
import org.springframework.web.bind.annotation.RestControllerAdvice;
import org.springframework.web.method.annotation.MethodArgumentTypeMismatchException;
import org.springframework.web.server.ResponseStatusException;
import org.springframework.web.servlet.NoHandlerFoundException;

@RestControllerAdvice
@Order(Ordered.HIGHEST_PRECEDENCE)
public class GlobalExceptionHandler {

  public static final String HANDLED_EXCEPTION_ATTR =
      GlobalExceptionHandler.class.getName() + ".handledException";

  private static final String MSG_INTERNAL = "internal server error";
  private static final String MSG_NOT_FOUND = "not found";
  private static final String MSG_BAD_REQUEST = "bad request";
  private static final String MSG_METHOD_NOT_ALLOWED = "method not allowed";

  private static final String CODE_INTERNAL = "internal_server_error";
  private static final String CODE_NOT_FOUND = "not_found";
  private static final String CODE_BAD_REQUEST = "bad_request";
  private static final String CODE_METHOD_NOT_ALLOWED = "method_not_allowed";

  private static final String MSG_PAYLOAD_TOO_LARGE = "payload too large";
  private static final String CODE_PAYLOAD_TOO_LARGE = "payload_too_large";

  private static String requestId(HttpServletRequest request) {
    if (request == null) return UUID.randomUUID().toString();
    Object v = request.getAttribute(RequestIdFilter.REQ_ID_ATTR);
    if (v instanceof String s && !s.isBlank()) return s;
    return UUID.randomUUID().toString();
  }

  private static ResponseEntity<ErrorResponse> json(HttpStatus status, ErrorResponse body) {
    return ResponseEntity.status(status).contentType(MediaType.APPLICATION_JSON).body(body);
  }

  private static ResponseEntity<ErrorResponse> internal(
      HttpServletRequest request, Throwable handled) {
    if (request != null && handled != null) {
      request.setAttribute(HANDLED_EXCEPTION_ATTR, handled);
    }
    return json(
        HttpStatus.INTERNAL_SERVER_ERROR,
        new ErrorResponse(MSG_INTERNAL, CODE_INTERNAL, requestId(request)));
  }

  private static ResponseEntity<ErrorResponse> notFound(HttpServletRequest request) {
    return json(
        HttpStatus.NOT_FOUND, new ErrorResponse(MSG_NOT_FOUND, CODE_NOT_FOUND, requestId(request)));
  }

  private static ResponseEntity<ErrorResponse> badRequest(HttpServletRequest request) {
    return json(
        HttpStatus.BAD_REQUEST,
        new ErrorResponse(MSG_BAD_REQUEST, CODE_BAD_REQUEST, requestId(request)));
  }

  private static ResponseEntity<ErrorResponse> methodNotAllowed(HttpServletRequest request) {
    return json(
        HttpStatus.METHOD_NOT_ALLOWED,
        new ErrorResponse(MSG_METHOD_NOT_ALLOWED, CODE_METHOD_NOT_ALLOWED, requestId(request)));
  }

  private static ResponseEntity<ErrorResponse> fromStatus(
      HttpStatus status, HttpServletRequest request) {

    String msg = status.getReasonPhrase().toLowerCase(Locale.ROOT);
    String code = status.name().toLowerCase(Locale.ROOT);

    return json(status, new ErrorResponse(msg, code, requestId(request)));
  }

  private static ResponseEntity<ErrorResponse> serverStatus(
      HttpStatus status, HttpServletRequest request, Throwable handled) {

    if (request != null && handled != null) {
      request.setAttribute(HANDLED_EXCEPTION_ATTR, handled);
    }
    return fromStatus(status, request);
  }

  @ExceptionHandler(NoHandlerFoundException.class)
  public ResponseEntity<ErrorResponse> handleNoHandler(
      NoHandlerFoundException ex, HttpServletRequest request) {
    return notFound(request);
  }

  @ExceptionHandler(HttpRequestMethodNotSupportedException.class)
  public ResponseEntity<ErrorResponse> handleMethodNotSupported(
      HttpRequestMethodNotSupportedException ex, HttpServletRequest request) {
    return methodNotAllowed(request);
  }

  @ExceptionHandler({
    MethodArgumentNotValidException.class,
    MethodArgumentTypeMismatchException.class
  })
  public ResponseEntity<ErrorResponse> handleBadRequest(Exception ex, HttpServletRequest request) {
    return badRequest(request);
  }

  @ExceptionHandler(ResponseStatusException.class)
  public ResponseEntity<ErrorResponse> handleResponseStatus(
      ResponseStatusException ex, HttpServletRequest request) {

    HttpStatus status = HttpStatus.resolve(ex.getStatusCode().value());
    if (status == null) return internal(request, ex);

    if (status == HttpStatus.NOT_FOUND) return notFound(request);
    if (status == HttpStatus.METHOD_NOT_ALLOWED) return methodNotAllowed(request);

    if (status.is5xxServerError()) return serverStatus(status, request, ex);

    return fromStatus(status, request);
  }

  @ExceptionHandler(ErrorResponseException.class)
  public ResponseEntity<ErrorResponse> handleErrorResponseException(
      ErrorResponseException ex, HttpServletRequest request) {

    HttpStatus status = HttpStatus.resolve(ex.getStatusCode().value());
    if (status == null) return internal(request, ex);

    if (status == HttpStatus.NOT_FOUND) return notFound(request);
    if (status == HttpStatus.METHOD_NOT_ALLOWED) return methodNotAllowed(request);

    if (status.is5xxServerError()) return serverStatus(status, request, ex);

    return fromStatus(status, request);
  }

  @ExceptionHandler(PayloadTooLargeException.class)
  public ResponseEntity<ErrorResponse> handlePayloadTooLarge(
      PayloadTooLargeException ex, HttpServletRequest request) {
    return json(
        HttpStatus.CONTENT_TOO_LARGE,
        new ErrorResponse(MSG_PAYLOAD_TOO_LARGE, CODE_PAYLOAD_TOO_LARGE, requestId(request)));
  }

  @ExceptionHandler(Exception.class)
  public ResponseEntity<ErrorResponse> handleFallback(Exception ex, HttpServletRequest request) {
    return internal(request, ex);
  }
}
