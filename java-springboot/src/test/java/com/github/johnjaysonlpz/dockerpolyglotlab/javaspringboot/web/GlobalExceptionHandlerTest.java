package com.github.johnjaysonlpz.dockerpolyglotlab.javaspringboot.web;

import static org.assertj.core.api.Assertions.assertThat;

import jakarta.servlet.http.HttpServletRequest;
import java.lang.reflect.Method;
import java.util.Locale;
import org.junit.jupiter.api.Test;
import org.springframework.http.HttpHeaders;
import org.springframework.http.HttpStatus;
import org.springframework.http.HttpStatusCode;
import org.springframework.http.MediaType;
import org.springframework.http.ResponseEntity;
import org.springframework.mock.web.MockHttpServletRequest;
import org.springframework.web.ErrorResponseException;
import org.springframework.web.HttpRequestMethodNotSupportedException;
import org.springframework.web.method.annotation.MethodArgumentTypeMismatchException;
import org.springframework.web.server.ResponseStatusException;
import org.springframework.web.servlet.NoHandlerFoundException;

class GlobalExceptionHandlerTest {

  @Test
  void handleNoHandler_returnsNotFoundJson_andUsesRequestIdAttributeIfPresent() throws Exception {
    GlobalExceptionHandler geh = new GlobalExceptionHandler();

    MockHttpServletRequest req = new MockHttpServletRequest("GET", "/missing");
    req.setAttribute(RequestIdFilter.REQ_ID_ATTR, "rid-1");

    NoHandlerFoundException ex = new NoHandlerFoundException("GET", "/missing", new HttpHeaders());

    ResponseEntity<ErrorResponse> out = geh.handleNoHandler(ex, req);

    assertThat(out.getStatusCode()).isEqualTo(HttpStatus.NOT_FOUND);
    assertThat(out.getHeaders().getContentType()).isEqualTo(MediaType.APPLICATION_JSON);

    ErrorResponse body = out.getBody();
    assertThat(body).isNotNull();
    assertThat(body.error()).isEqualTo("not found");
    assertThat(body.code()).isEqualTo("not_found");
    assertThat(body.requestId()).isEqualTo("rid-1");

    assertThat(req.getAttribute(GlobalExceptionHandler.HANDLED_EXCEPTION_ATTR)).isNull();
  }

  @Test
  void handleMethodNotSupported_returnsMethodNotAllowedJson() throws Exception {
    GlobalExceptionHandler geh = new GlobalExceptionHandler();

    MockHttpServletRequest req = new MockHttpServletRequest("POST", "/x");
    req.setAttribute(RequestIdFilter.REQ_ID_ATTR, "rid-2");

    HttpRequestMethodNotSupportedException ex = new HttpRequestMethodNotSupportedException("POST");

    ResponseEntity<ErrorResponse> out = geh.handleMethodNotSupported(ex, req);

    assertThat(out.getStatusCode()).isEqualTo(HttpStatus.METHOD_NOT_ALLOWED);
    assertThat(out.getHeaders().getContentType()).isEqualTo(MediaType.APPLICATION_JSON);

    ErrorResponse body = out.getBody();
    assertThat(body).isNotNull();
    assertThat(body.error()).isEqualTo("method not allowed");
    assertThat(body.code()).isEqualTo("method_not_allowed");
    assertThat(body.requestId()).isEqualTo("rid-2");
  }

  @Test
  void handleBadRequest_returnsBadRequestJson() {
    GlobalExceptionHandler geh = new GlobalExceptionHandler();

    MockHttpServletRequest req = new MockHttpServletRequest("GET", "/x");
    req.setAttribute(RequestIdFilter.REQ_ID_ATTR, "rid-3");

    MethodArgumentTypeMismatchException ex =
        new MethodArgumentTypeMismatchException(
            "x", Integer.class, "id", null, new IllegalArgumentException("bad"));

    ResponseEntity<ErrorResponse> out = geh.handleBadRequest(ex, req);

    assertThat(out.getStatusCode()).isEqualTo(HttpStatus.BAD_REQUEST);
    assertThat(out.getHeaders().getContentType()).isEqualTo(MediaType.APPLICATION_JSON);

    ErrorResponse body = out.getBody();
    assertThat(body).isNotNull();
    assertThat(body.error()).isEqualTo("bad request");
    assertThat(body.code()).isEqualTo("bad_request");
    assertThat(body.requestId()).isEqualTo("rid-3");
  }

  @Test
  void handlePayloadTooLarge_returnsContentTooLargeJson_andExceptionCarriesMaxBytes() {
    GlobalExceptionHandler geh = new GlobalExceptionHandler();

    MockHttpServletRequest req = new MockHttpServletRequest("POST", "/upload");
    req.setAttribute(RequestIdFilter.REQ_ID_ATTR, "rid-4");

    PayloadTooLargeException ex = new PayloadTooLargeException(123L);
    assertThat(ex.getMaxBodyBytes()).isEqualTo(123L);
    assertThat(ex.getMessage()).contains("payload exceeds 123 bytes");

    ResponseEntity<ErrorResponse> out = geh.handlePayloadTooLarge(ex, req);

    assertThat(out.getStatusCode()).isEqualTo(HttpStatus.CONTENT_TOO_LARGE);
    assertThat(out.getHeaders().getContentType()).isEqualTo(MediaType.APPLICATION_JSON);

    ErrorResponse body = out.getBody();
    assertThat(body).isNotNull();
    assertThat(body.error()).isEqualTo("payload too large");
    assertThat(body.code()).isEqualTo("payload_too_large");
    assertThat(body.requestId()).isEqualTo("rid-4");
  }

  @Test
  void handleFallback_returnsInternal_andMarksHandledExceptionAttribute() {
    GlobalExceptionHandler geh = new GlobalExceptionHandler();

    MockHttpServletRequest req = new MockHttpServletRequest("GET", "/boom");
    req.setAttribute(RequestIdFilter.REQ_ID_ATTR, "rid-5");

    Exception ex = new RuntimeException("boom");

    ResponseEntity<ErrorResponse> out = geh.handleFallback(ex, req);

    assertThat(out.getStatusCode()).isEqualTo(HttpStatus.INTERNAL_SERVER_ERROR);
    assertThat(out.getHeaders().getContentType()).isEqualTo(MediaType.APPLICATION_JSON);

    ErrorResponse body = out.getBody();
    assertThat(body).isNotNull();
    assertThat(body.error()).isEqualTo("internal server error");
    assertThat(body.code()).isEqualTo("internal_server_error");
    assertThat(body.requestId()).isEqualTo("rid-5");

    assertThat(req.getAttribute(GlobalExceptionHandler.HANDLED_EXCEPTION_ATTR)).isSameAs(ex);
  }

  @Test
  void handleResponseStatus_unresolvableStatus_goesInternal_andMarksHandled() {
    GlobalExceptionHandler geh = new GlobalExceptionHandler();

    MockHttpServletRequest req = new MockHttpServletRequest("GET", "/x");
    req.setAttribute(RequestIdFilter.REQ_ID_ATTR, "rid-rse-1");

    ResponseStatusException ex =
        new ResponseStatusException(HttpStatusCode.valueOf(599), "weird", null);

    ResponseEntity<ErrorResponse> out = geh.handleResponseStatus(ex, req);

    assertThat(out.getStatusCode()).isEqualTo(HttpStatus.INTERNAL_SERVER_ERROR);
    assertThat(out.getBody()).isNotNull();
    assertThat(out.getBody().code()).isEqualTo("internal_server_error");

    assertThat(req.getAttribute(GlobalExceptionHandler.HANDLED_EXCEPTION_ATTR)).isSameAs(ex);
  }

  @Test
  void handleResponseStatus_notFound_usesNotFound_andDoesNotMarkHandled() {
    GlobalExceptionHandler geh = new GlobalExceptionHandler();

    MockHttpServletRequest req = new MockHttpServletRequest("GET", "/x");
    req.setAttribute(RequestIdFilter.REQ_ID_ATTR, "rid-rse-2");

    ResponseStatusException ex = new ResponseStatusException(HttpStatus.NOT_FOUND, "nf");

    ResponseEntity<ErrorResponse> out = geh.handleResponseStatus(ex, req);

    assertThat(out.getStatusCode()).isEqualTo(HttpStatus.NOT_FOUND);
    assertThat(out.getBody()).isNotNull();
    assertThat(out.getBody().code()).isEqualTo("not_found");
    assertThat(req.getAttribute(GlobalExceptionHandler.HANDLED_EXCEPTION_ATTR)).isNull();
  }

  @Test
  void handleResponseStatus_methodNotAllowed_usesMethodNotAllowed() {
    GlobalExceptionHandler geh = new GlobalExceptionHandler();

    MockHttpServletRequest req = new MockHttpServletRequest("GET", "/x");
    req.setAttribute(RequestIdFilter.REQ_ID_ATTR, "rid-rse-3");

    ResponseStatusException ex = new ResponseStatusException(HttpStatus.METHOD_NOT_ALLOWED, "mna");

    ResponseEntity<ErrorResponse> out = geh.handleResponseStatus(ex, req);

    assertThat(out.getStatusCode()).isEqualTo(HttpStatus.METHOD_NOT_ALLOWED);
    assertThat(out.getBody()).isNotNull();
    assertThat(out.getBody().code()).isEqualTo("method_not_allowed");
  }

  @Test
  void handleResponseStatus_5xx_marksHandled_andUsesFromStatusBody() {
    GlobalExceptionHandler geh = new GlobalExceptionHandler();

    MockHttpServletRequest req = new MockHttpServletRequest("GET", "/x");
    req.setAttribute(RequestIdFilter.REQ_ID_ATTR, "rid-rse-4");

    ResponseStatusException ex =
        new ResponseStatusException(HttpStatus.SERVICE_UNAVAILABLE, "down");

    ResponseEntity<ErrorResponse> out = geh.handleResponseStatus(ex, req);

    assertThat(out.getStatusCode()).isEqualTo(HttpStatus.SERVICE_UNAVAILABLE);
    assertThat(out.getBody()).isNotNull();

    assertThat(out.getBody().error())
        .isEqualTo(HttpStatus.SERVICE_UNAVAILABLE.getReasonPhrase().toLowerCase(Locale.ROOT));
    assertThat(out.getBody().code())
        .isEqualTo(HttpStatus.SERVICE_UNAVAILABLE.name().toLowerCase(Locale.ROOT));

    assertThat(req.getAttribute(GlobalExceptionHandler.HANDLED_EXCEPTION_ATTR)).isSameAs(ex);
  }

  @Test
  void handleResponseStatus_4xx_other_goesFromStatus_andDoesNotMarkHandled() {
    GlobalExceptionHandler geh = new GlobalExceptionHandler();

    MockHttpServletRequest req = new MockHttpServletRequest("GET", "/x");
    req.setAttribute(RequestIdFilter.REQ_ID_ATTR, "rid-rse-5");

    ResponseStatusException ex = new ResponseStatusException(HttpStatus.CONFLICT, "conflict");

    ResponseEntity<ErrorResponse> out = geh.handleResponseStatus(ex, req);

    assertThat(out.getStatusCode()).isEqualTo(HttpStatus.CONFLICT);
    assertThat(out.getBody()).isNotNull();
    assertThat(out.getBody().error())
        .isEqualTo(HttpStatus.CONFLICT.getReasonPhrase().toLowerCase(Locale.ROOT));
    assertThat(out.getBody().code()).isEqualTo(HttpStatus.CONFLICT.name().toLowerCase(Locale.ROOT));

    assertThat(req.getAttribute(GlobalExceptionHandler.HANDLED_EXCEPTION_ATTR)).isNull();
  }

  @Test
  void handleErrorResponseException_coversAllMajorBranches() throws Exception {
    GlobalExceptionHandler geh = new GlobalExceptionHandler();

    {
      MockHttpServletRequest req = new MockHttpServletRequest("GET", "/x");
      req.setAttribute(RequestIdFilter.REQ_ID_ATTR, "rid-ere-1");

      ErrorResponseException ex = newErrorResponseException(599);

      ResponseEntity<ErrorResponse> out = geh.handleErrorResponseException(ex, req);

      assertThat(out.getStatusCode()).isEqualTo(HttpStatus.INTERNAL_SERVER_ERROR);
      assertThat(req.getAttribute(GlobalExceptionHandler.HANDLED_EXCEPTION_ATTR)).isSameAs(ex);
    }

    {
      MockHttpServletRequest req = new MockHttpServletRequest("GET", "/x");
      req.setAttribute(RequestIdFilter.REQ_ID_ATTR, "rid-ere-2");

      ErrorResponseException ex = newErrorResponseException(404);

      ResponseEntity<ErrorResponse> out = geh.handleErrorResponseException(ex, req);

      assertThat(out.getStatusCode()).isEqualTo(HttpStatus.NOT_FOUND);
      assertThat(out.getBody()).isNotNull();
      assertThat(out.getBody().code()).isEqualTo("not_found");
      assertThat(req.getAttribute(GlobalExceptionHandler.HANDLED_EXCEPTION_ATTR)).isNull();
    }

    {
      MockHttpServletRequest req = new MockHttpServletRequest("GET", "/x");
      req.setAttribute(RequestIdFilter.REQ_ID_ATTR, "rid-ere-3");

      ErrorResponseException ex = newErrorResponseException(405);

      ResponseEntity<ErrorResponse> out = geh.handleErrorResponseException(ex, req);

      assertThat(out.getStatusCode()).isEqualTo(HttpStatus.METHOD_NOT_ALLOWED);
      assertThat(out.getBody()).isNotNull();
      assertThat(out.getBody().code()).isEqualTo("method_not_allowed");
    }

    {
      MockHttpServletRequest req = new MockHttpServletRequest("GET", "/x");
      req.setAttribute(RequestIdFilter.REQ_ID_ATTR, "rid-ere-4");

      ErrorResponseException ex = newErrorResponseException(502);

      ResponseEntity<ErrorResponse> out = geh.handleErrorResponseException(ex, req);

      assertThat(out.getStatusCode()).isEqualTo(HttpStatus.BAD_GATEWAY);
      assertThat(req.getAttribute(GlobalExceptionHandler.HANDLED_EXCEPTION_ATTR)).isSameAs(ex);
    }

    {
      MockHttpServletRequest req = new MockHttpServletRequest("GET", "/x");
      req.setAttribute(RequestIdFilter.REQ_ID_ATTR, "rid-ere-5");

      ErrorResponseException ex = newErrorResponseException(400);

      ResponseEntity<ErrorResponse> out = geh.handleErrorResponseException(ex, req);

      assertThat(out.getStatusCode()).isEqualTo(HttpStatus.BAD_REQUEST);
      assertThat(out.getBody()).isNotNull();
      assertThat(out.getBody().error())
          .isEqualTo(HttpStatus.BAD_REQUEST.getReasonPhrase().toLowerCase(Locale.ROOT));
      assertThat(out.getBody().code())
          .isEqualTo(HttpStatus.BAD_REQUEST.name().toLowerCase(Locale.ROOT));
    }
  }

  @Test
  void privateHelpers_areCoveredViaReflection() throws Exception {
    Method requestId =
        GlobalExceptionHandler.class.getDeclaredMethod("requestId", HttpServletRequest.class);
    requestId.setAccessible(true);

    String idNullReq = (String) requestId.invoke(null, (HttpServletRequest) null);
    assertThat(idNullReq).isNotBlank();

    MockHttpServletRequest reqNoAttr = new MockHttpServletRequest("GET", "/x");
    String idNoAttr = (String) requestId.invoke(null, reqNoAttr);
    assertThat(idNoAttr).isNotBlank();

    MockHttpServletRequest reqBlankAttr = new MockHttpServletRequest("GET", "/x");
    reqBlankAttr.setAttribute(RequestIdFilter.REQ_ID_ATTR, "   ");
    String idBlankAttr = (String) requestId.invoke(null, reqBlankAttr);
    assertThat(idBlankAttr).isNotBlank();

    MockHttpServletRequest reqGoodAttr = new MockHttpServletRequest("GET", "/x");
    reqGoodAttr.setAttribute(RequestIdFilter.REQ_ID_ATTR, "rid-ok");
    String idGood = (String) requestId.invoke(null, reqGoodAttr);
    assertThat(idGood).isEqualTo("rid-ok");

    Method json =
        GlobalExceptionHandler.class.getDeclaredMethod(
            "json", HttpStatus.class, ErrorResponse.class);
    json.setAccessible(true);

    @SuppressWarnings("unchecked")
    ResponseEntity<ErrorResponse> j =
        (ResponseEntity<ErrorResponse>)
            json.invoke(null, HttpStatus.BAD_REQUEST, new ErrorResponse("m", "c", "r"));

    assertThat(j.getStatusCode()).isEqualTo(HttpStatus.BAD_REQUEST);
    assertThat(j.getHeaders().getContentType()).isEqualTo(MediaType.APPLICATION_JSON);
    assertThat(j.getBody()).isNotNull();

    Method internal =
        GlobalExceptionHandler.class.getDeclaredMethod(
            "internal", HttpServletRequest.class, Throwable.class);
    internal.setAccessible(true);

    MockHttpServletRequest reqInternal = new MockHttpServletRequest("GET", "/x");
    reqInternal.setAttribute(RequestIdFilter.REQ_ID_ATTR, "rid-internal");
    RuntimeException handled = new RuntimeException("handled");

    @SuppressWarnings("unchecked")
    ResponseEntity<ErrorResponse> internalOut =
        (ResponseEntity<ErrorResponse>) internal.invoke(null, reqInternal, handled);

    assertThat(internalOut.getStatusCode()).isEqualTo(HttpStatus.INTERNAL_SERVER_ERROR);
    assertThat(reqInternal.getAttribute(GlobalExceptionHandler.HANDLED_EXCEPTION_ATTR))
        .isSameAs(handled);

    MockHttpServletRequest reqInternalNullHandled = new MockHttpServletRequest("GET", "/x");
    reqInternalNullHandled.setAttribute(RequestIdFilter.REQ_ID_ATTR, "rid-internal-nullhandled");

    @SuppressWarnings("unchecked")
    ResponseEntity<ErrorResponse> internalNullHandledOut =
        (ResponseEntity<ErrorResponse>) internal.invoke(null, reqInternalNullHandled, null);

    assertThat(internalNullHandledOut.getStatusCode()).isEqualTo(HttpStatus.INTERNAL_SERVER_ERROR);
    assertThat(reqInternalNullHandled.getAttribute(GlobalExceptionHandler.HANDLED_EXCEPTION_ATTR))
        .isNull();

    @SuppressWarnings("unchecked")
    ResponseEntity<ErrorResponse> internalNullReq =
        (ResponseEntity<ErrorResponse>) internal.invoke(null, null, handled);

    assertThat(internalNullReq.getStatusCode()).isEqualTo(HttpStatus.INTERNAL_SERVER_ERROR);
    assertThat(internalNullReq.getBody()).isNotNull();
    assertThat(internalNullReq.getBody().requestId()).isNotBlank();

    Method fromStatus =
        GlobalExceptionHandler.class.getDeclaredMethod(
            "fromStatus", HttpStatus.class, HttpServletRequest.class);
    fromStatus.setAccessible(true);

    MockHttpServletRequest reqFrom = new MockHttpServletRequest("GET", "/x");
    reqFrom.setAttribute(RequestIdFilter.REQ_ID_ATTR, "rid-from");

    @SuppressWarnings("unchecked")
    ResponseEntity<ErrorResponse> fromOut =
        (ResponseEntity<ErrorResponse>) fromStatus.invoke(null, HttpStatus.IM_USED, reqFrom);

    assertThat(fromOut.getStatusCode()).isEqualTo(HttpStatus.IM_USED);
    assertThat(fromOut.getBody()).isNotNull();
    assertThat(fromOut.getBody().error())
        .isEqualTo(HttpStatus.IM_USED.getReasonPhrase().toLowerCase(Locale.ROOT));
    assertThat(fromOut.getBody().code())
        .isEqualTo(HttpStatus.IM_USED.name().toLowerCase(Locale.ROOT));
    assertThat(fromOut.getBody().requestId()).isEqualTo("rid-from");

    Method serverStatus =
        GlobalExceptionHandler.class.getDeclaredMethod(
            "serverStatus", HttpStatus.class, HttpServletRequest.class, Throwable.class);
    serverStatus.setAccessible(true);

    MockHttpServletRequest reqServer = new MockHttpServletRequest("GET", "/x");
    reqServer.setAttribute(RequestIdFilter.REQ_ID_ATTR, "rid-server");
    RuntimeException h2 = new RuntimeException("h2");

    @SuppressWarnings("unchecked")
    ResponseEntity<ErrorResponse> serverOut =
        (ResponseEntity<ErrorResponse>)
            serverStatus.invoke(null, HttpStatus.BAD_GATEWAY, reqServer, h2);

    assertThat(serverOut.getStatusCode()).isEqualTo(HttpStatus.BAD_GATEWAY);
    assertThat(reqServer.getAttribute(GlobalExceptionHandler.HANDLED_EXCEPTION_ATTR)).isSameAs(h2);
    assertThat(serverOut.getBody()).isNotNull();

    MockHttpServletRequest reqServerNullHandled = new MockHttpServletRequest("GET", "/x");
    reqServerNullHandled.setAttribute(RequestIdFilter.REQ_ID_ATTR, "rid-server-nullhandled");

    @SuppressWarnings("unchecked")
    ResponseEntity<ErrorResponse> serverNullHandledOut =
        (ResponseEntity<ErrorResponse>)
            serverStatus.invoke(null, HttpStatus.BAD_GATEWAY, reqServerNullHandled, null);

    assertThat(serverNullHandledOut.getStatusCode()).isEqualTo(HttpStatus.BAD_GATEWAY);
    assertThat(reqServerNullHandled.getAttribute(GlobalExceptionHandler.HANDLED_EXCEPTION_ATTR))
        .isNull();
    assertThat(serverNullHandledOut.getBody()).isNotNull();

    @SuppressWarnings("unchecked")
    ResponseEntity<ErrorResponse> serverNullRequestOut =
        (ResponseEntity<ErrorResponse>)
            serverStatus.invoke(null, HttpStatus.BAD_GATEWAY, null, new RuntimeException("x"));

    assertThat(serverNullRequestOut.getStatusCode()).isEqualTo(HttpStatus.BAD_GATEWAY);
    assertThat(serverNullRequestOut.getBody()).isNotNull();
    assertThat(serverNullRequestOut.getBody().requestId()).isNotBlank();
  }

  private static ErrorResponseException newErrorResponseException(int code) throws Exception {
    HttpStatusCode sc = HttpStatusCode.valueOf(code);

    for (var ctor : ErrorResponseException.class.getDeclaredConstructors()) {
      Class<?>[] p = ctor.getParameterTypes();
      if (p.length == 1 && HttpStatusCode.class.isAssignableFrom(p[0])) {
        ctor.setAccessible(true);
        return (ErrorResponseException) ctor.newInstance(sc);
      }
    }

    for (var ctor : ErrorResponseException.class.getDeclaredConstructors()) {
      Class<?>[] p = ctor.getParameterTypes();
      if (p.length >= 1 && HttpStatusCode.class.isAssignableFrom(p[0])) {
        ctor.setAccessible(true);
        Object[] args = new Object[p.length];
        args[0] = sc;
        return (ErrorResponseException) ctor.newInstance(args);
      }
    }

    throw new IllegalStateException("No compatible ErrorResponseException constructor found");
  }
}
