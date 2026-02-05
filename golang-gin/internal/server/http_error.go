package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

type HTTPError struct {
	Status  int
	Code    string
	Message string
	Err     error
}

func (e HTTPError) Error() string {
	switch {
	case strings.TrimSpace(e.Message) != "" && e.Err != nil:
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	case strings.TrimSpace(e.Message) != "":
		return e.Message
	case e.Err != nil:
		return e.Err.Error()
	default:
		return "http error"
	}
}

func (e HTTPError) Unwrap() error { return e.Err }

func (e HTTPError) StatusCode() int { return e.Status }

type statusCoder interface {
	StatusCode() int
}

func NewHTTPError(status int, code, message string, err error) HTTPError {
	return HTTPError{
		Status:  status,
		Code:    code,
		Message: message,
		Err:     err,
	}
}

func ResolveErrorResponse(err error, fallbackStatus int) (status int, code string, message string) {
	status = normalizeStatusOrDefault(fallbackStatus, http.StatusInternalServerError)
	code = defaultErrorCode(status)
	message = defaultErrorMessage(status)

	if err == nil {
		return status, code, message
	}

	var he HTTPError
	if errors.As(err, &he) {
		if s, ok := normalizeStatus(he.Status); ok {
			status = s
		}
		if strings.TrimSpace(he.Code) != "" {
			code = he.Code
		} else {
			code = defaultErrorCode(status)
		}
		if strings.TrimSpace(he.Message) != "" {
			message = he.Message
		} else {
			message = defaultErrorMessage(status)
		}
		return status, code, message
	}

	var sc statusCoder
	if errors.As(err, &sc) {
		if s, ok := normalizeStatus(sc.StatusCode()); ok {
			status = s
			code = defaultErrorCode(status)
			message = defaultErrorMessage(status)
		}
		return status, code, message
	}

	return status, code, message
}

func normalizeStatus(code int) (int, bool) {
	if code >= 400 && code <= 599 {
		return code, true
	}
	return 0, false
}

func normalizeStatusOrDefault(code int, def int) int {
	if s, ok := normalizeStatus(code); ok {
		return s
	}
	return def
}

var defaultCodeByStatus = map[int]string{
	http.StatusBadRequest:            "bad_request",
	http.StatusUnauthorized:          "unauthorized",
	http.StatusForbidden:             "forbidden",
	http.StatusNotFound:              "not_found",
	http.StatusMethodNotAllowed:      "method_not_allowed",
	http.StatusRequestTimeout:        "request_timeout",
	http.StatusConflict:              "conflict",
	http.StatusRequestEntityTooLarge: "payload_too_large",
	http.StatusUnsupportedMediaType:  "unsupported_media_type",
	http.StatusUnprocessableEntity:   "unprocessable_entity",
	http.StatusTooManyRequests:       "too_many_requests",
	http.StatusBadGateway:            "bad_gateway",
	http.StatusServiceUnavailable:    "service_unavailable",
	http.StatusGatewayTimeout:        "gateway_timeout",
}

func defaultErrorCode(status int) string {
	if v, ok := defaultCodeByStatus[status]; ok {
		return v
	}
	if status >= 500 {
		return "internal_server_error"
	}
	return "error"
}

var defaultMessageByStatus = map[int]string{
	http.StatusBadRequest:            "bad request",
	http.StatusUnauthorized:          "unauthorized",
	http.StatusForbidden:             "forbidden",
	http.StatusNotFound:              "not found",
	http.StatusMethodNotAllowed:      "method not allowed",
	http.StatusRequestTimeout:        "request timeout",
	http.StatusConflict:              "conflict",
	http.StatusRequestEntityTooLarge: "payload too large",
	http.StatusUnsupportedMediaType:  "unsupported media type",
	http.StatusUnprocessableEntity:   "unprocessable entity",
	http.StatusTooManyRequests:       "too many requests",
	http.StatusBadGateway:            "bad gateway",
	http.StatusServiceUnavailable:    "service unavailable",
	http.StatusGatewayTimeout:        "gateway timeout",
}

func defaultErrorMessage(status int) string {
	if v, ok := defaultMessageByStatus[status]; ok {
		return v
	}
	if status >= 500 {
		return "internal server error"
	}

	txt := strings.TrimSpace(http.StatusText(status))
	if txt == "" {
		return "error"
	}
	return strings.ToLower(txt)
}
