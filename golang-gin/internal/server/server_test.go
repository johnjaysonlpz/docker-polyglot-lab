package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

/*
NOTE ABOUT PARALLEL TESTS

These tests intentionally DO NOT call t.Parallel().

Reason:
- Some tests swap global OpenTelemetry state (otel.SetTracerProvider).
- Gin has global mode as well (gin.SetMode).

Running them in parallel can lead to races / flaky tests.
*/

const (
	testServiceName = "svc"
	testVersion     = "v"
	testBuildTime   = "bt"
)

//
// ==========================
// Test helpers / fixtures
// ==========================
//

// mustJSON decodes a JSON response body into a map for easy assertions.
func mustJSON(t *testing.T, b []byte) map[string]any {
	t.Helper()

	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("failed to decode json: %v\nbody=%s", err, string(b))
	}
	return m
}

// captureHandler collects slog records for assertions.
// We copy records because slog may reuse record objects internally.
type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	c := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		c.AddAttrs(a)
		return true
	})

	h.records = append(h.records, c)
	return nil
}

func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *captureHandler) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.records)
}

func (h *captureHandler) Last() (slog.Record, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.records) == 0 {
		return slog.Record{}, false
	}
	return h.records[len(h.records)-1], true
}

func recordAttrsToMap(r slog.Record) map[string]any {
	out := make(map[string]any)
	r.Attrs(func(a slog.Attr) bool {
		out[a.Key] = a.Value.Any()
		return true
	})
	return out
}

func newCapturedLogger() (*slog.Logger, *captureHandler) {
	h := &captureHandler{}
	return slog.New(h), h
}

func baseTestConfig() Config {
	return Config{
		GinMode:           gin.TestMode,
		Host:              "127.0.0.1",
		Port:              8080,
		LogLevel:          slog.LevelInfo,
		ReadTimeout:       time.Second,
		WriteTimeout:      time.Second,
		ReadHeaderTimeout: time.Second,
		IdleTimeout:       time.Second,
		ShutdownTimeout:   time.Second,
		MaxBodyBytes:      1024,
		ServiceName:       testServiceName,
		Version:           testVersion,
		BuildTime:         testBuildTime,
	}
}

// sizeNegWriter forces ResponseWriter.Size() to return a negative value.
// This is used to cover the "clamp to zero" logic in logging middleware.
type sizeNegWriter struct{ gin.ResponseWriter }

func (w *sizeNegWriter) Size() int { return -1 }

// traceProviderSwap installs tp globally and returns the old provider.
// Always restore the old provider using t.Cleanup().
func traceProviderSwap(tp trace.TracerProvider) trace.TracerProvider {
	old := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	return old
}

//
// ==========================
// Config: LoadConfig + parsing helpers + Validate
// ==========================
//

func TestLoadConfig(t *testing.T) {
	t.Run("empty env uses defaults; valid; no warnings", testLoadConfigEmptyEnvDefaults)
	t.Run("invalid env produces warnings and validation error", testLoadConfigInvalidEnv)
	t.Run("valid explicit env; no warnings", testLoadConfigValidExplicitEnv)
	t.Run("warnings but still valid", testLoadConfigWarnsButValid)
	t.Run("negative MAX_BODY_BYTES causes validation error without warnings", testLoadConfigNegMaxBodyBytes)
}

func testLoadConfigEmptyEnvDefaults(t *testing.T) {
	t.Setenv("GIN_MODE", "")
	t.Setenv("HOST", "")
	t.Setenv("PORT", "")

	t.Setenv("LOG_LEVEL", "")
	t.Setenv("READ_TIMEOUT", "")
	t.Setenv("WRITE_TIMEOUT", "")
	t.Setenv("READ_HEADER_TIMEOUT", "")
	t.Setenv("IDLE_TIMEOUT", "")
	t.Setenv("SHUTDOWN_TIMEOUT", "")
	t.Setenv("MAX_BODY_BYTES", "")
	t.Setenv("TRUSTED_PROXIES", "")

	cfg, warns, err := LoadConfig(testServiceName, testVersion, testBuildTime)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if len(warns) != 0 {
		t.Fatalf("expected no warnings, got %v", warns)
	}

	if cfg.GinMode != gin.ReleaseMode {
		t.Fatalf("GinMode: got %q want %q", cfg.GinMode, gin.ReleaseMode)
	}
	if cfg.Host != "0.0.0.0" {
		t.Fatalf("Host: got %q want %q", cfg.Host, "0.0.0.0")
	}
	if cfg.Port != 8080 {
		t.Fatalf("Port: got %d want %d", cfg.Port, 8080)
	}
	if cfg.MaxBodyBytes != 1*1024*1024 {
		t.Fatalf("MaxBodyBytes: got %d want %d", cfg.MaxBodyBytes, 1*1024*1024)
	}
}

func testLoadConfigInvalidEnv(t *testing.T) {
	t.Setenv("GIN_MODE", "")
	t.Setenv("HOST", "")
	t.Setenv("PORT", "nope")
	t.Setenv("LOG_LEVEL", "what")
	t.Setenv("READ_TIMEOUT", "bad")
	t.Setenv("WRITE_TIMEOUT", "0")

	t.Setenv("READ_HEADER_TIMEOUT", "")
	t.Setenv("IDLE_TIMEOUT", "-1")
	t.Setenv("SHUTDOWN_TIMEOUT", "1s")
	t.Setenv("MAX_BODY_BYTES", "x")

	t.Setenv("TRUSTED_PROXIES", "10.0.0.0/8,  ,172.16.0.0/12")

	cfg, warns, err := LoadConfig(testServiceName, testVersion, testBuildTime)
	if err == nil {
		t.Fatalf("expected validation error, got nil")
	}

	// Even when invalid inputs are present, we expect defaults for some fields.
	if cfg.GinMode != gin.ReleaseMode {
		t.Fatalf("expected default gin mode %q, got %q", gin.ReleaseMode, cfg.GinMode)
	}
	if cfg.Host != "0.0.0.0" {
		t.Fatalf("expected default host 0.0.0.0, got %q", cfg.Host)
	}

	// Service metadata should always be set from parameters.
	if cfg.ServiceName != testServiceName || cfg.Version != testVersion || cfg.BuildTime != testBuildTime {
		t.Fatalf("service metadata mismatch: %#v", cfg)
	}

	// Trusted proxies parsing should trim empties.
	if len(cfg.TrustedProxies) != 2 {
		t.Fatalf("expected 2 trusted proxies, got %v", cfg.TrustedProxies)
	}

	if len(warns) == 0 {
		t.Fatalf("expected warnings, got none")
	}
}

func testLoadConfigValidExplicitEnv(t *testing.T) {
	t.Setenv("GIN_MODE", gin.DebugMode)
	t.Setenv("HOST", "127.0.0.1")
	t.Setenv("PORT", "9090")

	t.Setenv("LOG_LEVEL", "warn")
	t.Setenv("READ_TIMEOUT", "1s")
	t.Setenv("WRITE_TIMEOUT", "2s")
	t.Setenv("READ_HEADER_TIMEOUT", "3s")
	t.Setenv("IDLE_TIMEOUT", "4s")
	t.Setenv("SHUTDOWN_TIMEOUT", "5s")
	t.Setenv("MAX_BODY_BYTES", "123")
	t.Setenv("TRUSTED_PROXIES", "10.0.0.0/8,172.16.0.0/12")

	cfg, warns, err := LoadConfig(testServiceName, testVersion, testBuildTime)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if len(warns) != 0 {
		t.Fatalf("expected no warnings, got %v", warns)
	}

	if cfg.GinMode != gin.DebugMode {
		t.Fatalf("GinMode: got %q want %q", cfg.GinMode, gin.DebugMode)
	}
	if cfg.Host != "127.0.0.1" || cfg.Port != 9090 {
		t.Fatalf("host/port: got %q:%d", cfg.Host, cfg.Port)
	}
	if cfg.LogLevel != slog.LevelWarn {
		t.Fatalf("LogLevel: got %v want %v", cfg.LogLevel, slog.LevelWarn)
	}
	if cfg.MaxBodyBytes != 123 {
		t.Fatalf("MaxBodyBytes: got %d want %d", cfg.MaxBodyBytes, 123)
	}
	if len(cfg.TrustedProxies) != 2 {
		t.Fatalf("TrustedProxies: got %v", cfg.TrustedProxies)
	}
}

func testLoadConfigWarnsButValid(t *testing.T) {
	t.Setenv("GIN_MODE", gin.ReleaseMode)
	t.Setenv("HOST", "127.0.0.1")
	t.Setenv("PORT", "8080")

	// Invalid values that should warn but fall back to defaults.
	t.Setenv("READ_HEADER_TIMEOUT", "bad")
	t.Setenv("SHUTDOWN_TIMEOUT", "0")

	t.Setenv("LOG_LEVEL", "")
	t.Setenv("READ_TIMEOUT", "")
	t.Setenv("WRITE_TIMEOUT", "")
	t.Setenv("IDLE_TIMEOUT", "")
	t.Setenv("MAX_BODY_BYTES", "")
	t.Setenv("TRUSTED_PROXIES", "")

	_, warns, err := LoadConfig(testServiceName, testVersion, testBuildTime)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if len(warns) == 0 {
		t.Fatalf("expected warnings, got none")
	}
}

func testLoadConfigNegMaxBodyBytes(t *testing.T) {
	t.Setenv("GIN_MODE", gin.ReleaseMode)
	t.Setenv("HOST", "127.0.0.1")
	t.Setenv("PORT", "8080")

	t.Setenv("LOG_LEVEL", "info")
	t.Setenv("READ_TIMEOUT", "1s")
	t.Setenv("WRITE_TIMEOUT", "1s")
	t.Setenv("READ_HEADER_TIMEOUT", "1s")
	t.Setenv("IDLE_TIMEOUT", "1s")
	t.Setenv("SHUTDOWN_TIMEOUT", "1s")

	t.Setenv("MAX_BODY_BYTES", "-1")
	t.Setenv("TRUSTED_PROXIES", "")

	_, warns, err := LoadConfig(testServiceName, testVersion, testBuildTime)
	if err == nil {
		t.Fatalf("expected validation error, got nil")
	}
	if len(warns) != 0 {
		t.Fatalf("expected no warnings, got %v", warns)
	}
	if !strings.Contains(err.Error(), "MAX_BODY_BYTES must be >= 0") {
		t.Fatalf("expected MAX_BODY_BYTES validation error, got %v", err)
	}
}

func TestParseHelpers(t *testing.T) {
	t.Run("parseLogLevel", testParseHelpersParseLogLevel)
	t.Run("parseDurationEnv", testParseHelpersParseDurationEnv)
	t.Run("parseCSVEnv", testParseHelpersParseCSVEnv)
	t.Run("parseInt64Env", testParseHelpersParseInt64Env)
}

func testParseHelpersParseLogLevel(t *testing.T) {
	cases := []struct {
		in       string
		want     slog.Level
		wantWarn string
	}{
		{"debug", slog.LevelDebug, ""},
		{"warn", slog.LevelWarn, ""},
		{"warning", slog.LevelWarn, ""},
		{"error", slog.LevelError, ""},
		{"info", slog.LevelInfo, ""},
		{"", slog.LevelInfo, ""},
		{"NOPE", slog.LevelInfo, `invalid LOG_LEVEL="NOPE"; defaulting to info`},
	}

	for _, tc := range cases {
		got, w := parseLogLevel(tc.in)
		if got != tc.want {
			t.Fatalf("parseLogLevel(%q): got %v want %v", tc.in, got, tc.want)
		}
		if w != tc.wantWarn {
			t.Fatalf("parseLogLevel(%q) warn: got %q want %q", tc.in, w, tc.wantWarn)
		}
	}
}

func testParseHelpersParseDurationEnv(t *testing.T) {
	t.Setenv("DUR", "")
	if got, w := parseDurationEnv("DUR", 123*time.Millisecond); got != 123*time.Millisecond || w != "" {
		t.Fatalf("empty -> default: got=%s warn=%q", got, w)
	}

	t.Setenv("DUR", "bad")
	if got, w := parseDurationEnv("DUR", time.Second); got != time.Second || !strings.Contains(w, "invalid DUR=") {
		t.Fatalf("bad -> default + warn: got=%s warn=%q", got, w)
	}

	t.Setenv("DUR", "-1s")
	if got, w := parseDurationEnv("DUR", time.Second); got != time.Second || !strings.Contains(w, "invalid DUR=") {
		t.Fatalf("negative -> default + warn: got=%s warn=%q", got, w)
	}

	t.Setenv("DUR", "250ms")
	if got, w := parseDurationEnv("DUR", time.Second); got != 250*time.Millisecond || w != "" {
		t.Fatalf("valid: got=%s warn=%q", got, w)
	}
}

func testParseHelpersParseCSVEnv(t *testing.T) {
	t.Setenv("CSV", "")
	if got := parseCSVEnv("CSV"); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}

	t.Setenv("CSV", " a, ,b , c ")
	got := parseCSVEnv("CSV")
	if strings.Join(got, "|") != "a|b|c" {
		t.Fatalf("unexpected parse: %v", got)
	}
}

func testParseHelpersParseInt64Env(t *testing.T) {
	t.Setenv("I64", "")
	if got, w := parseInt64Env("I64", 7); got != 7 || w != "" {
		t.Fatalf("empty -> default: got=%d warn=%q", got, w)
	}

	t.Setenv("I64", "nope")
	if got, w := parseInt64Env("I64", 7); got != 7 || !strings.Contains(w, "invalid I64=") {
		t.Fatalf("bad -> default + warn: got=%d warn=%q", got, w)
	}

	t.Setenv("I64", "99")
	if got, w := parseInt64Env("I64", 7); got != 99 || w != "" {
		t.Fatalf("valid: got=%d warn=%q", got, w)
	}
}

func TestConfigValidate(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		cfg := Config{
			GinMode:           gin.TestMode,
			Host:              "127.0.0.1",
			Port:              8080,
			ReadTimeout:       time.Second,
			WriteTimeout:      time.Second,
			ReadHeaderTimeout: time.Second,
			IdleTimeout:       time.Second,
			ShutdownTimeout:   time.Second,
			MaxBodyBytes:      0,
		}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("all error paths included", func(t *testing.T) {
		cfg := Config{
			GinMode:           "nope",
			Host:              " ",
			Port:              70000,
			ReadTimeout:       0,
			WriteTimeout:      -1,
			ReadHeaderTimeout: 0,
			MaxBodyBytes:      -1,
			IdleTimeout:       0,
			ShutdownTimeout:   0,
		}

		err := cfg.Validate()
		if err == nil {
			t.Fatalf("expected validation error")
		}

		msg := err.Error()
		for _, sub := range []string{
			"GIN_MODE must be one of",
			"HOST must not be empty",
			"PORT must be a valid TCP port",
			"READ_TIMEOUT must be > 0",
			"WRITE_TIMEOUT must be > 0",
			"READ_HEADER_TIMEOUT must be > 0",
			"MAX_BODY_BYTES must be >= 0",
			"IDLE_TIMEOUT must be > 0",
			"SHUTDOWN_TIMEOUT must be > 0",
		} {
			if !strings.Contains(msg, sub) {
				t.Fatalf("expected validation msg to contain %q; got %q", sub, msg)
			}
		}
	})
}

//
// ==========================
// Error model + response resolution
// ==========================
//

func TestHTTPError_ErrorAndAccessors(t *testing.T) {
	cases := []struct {
		name string
		in   HTTPError
		want string
	}{
		{"message+err", HTTPError{Message: "m", Err: errors.New("x")}, "m: x"},
		{"message only", HTTPError{Message: "m"}, "m"},
		{"err only", HTTPError{Err: errors.New("x")}, "x"},
		{"neither", HTTPError{}, "http error"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.in.Error(); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}

	e := HTTPError{Message: "m", Err: errors.New("x")}
	if e.Unwrap() == nil {
		t.Fatalf("expected unwrap to return err")
	}
	if e.StatusCode() != 0 {
		t.Fatalf("expected status code 0, got %d", e.StatusCode())
	}
}

type scErr struct{ code int }

func (s scErr) Error() string   { return "sc" }
func (s scErr) StatusCode() int { return s.code }

func TestResolveErrorResponse(t *testing.T) {
	t.Run("nil err + invalid fallback -> default 500", testResolveErrorResponseNilErrInvalidFallback)
	t.Run("non-HTTP error + invalid fallback -> default 500", testResolveErrorResponseNonHTTPErrInvalidFallback)
	t.Run("HTTPError valid status + invalid fallback -> uses HTTP status", testResolveErrorResponseHTTPErrorValidStatusInvalidFallback)
	t.Run("HTTPError invalid status -> fallback; preserve code/message", testResolveErrorResponseHTTPErrorInvalidStatusUsesFallback)
	t.Run("HTTPError with blank code/message -> defaults from status", testResolveErrorResponseHTTPErrorBlankCodeMessageDefaults)
	t.Run("HTTPError mixed defaults (code blank vs message blank)", testResolveErrorResponseHTTPErrorMixedDefaults)
	t.Run("statusCoder valid + invalid", testResolveErrorResponseStatusCoderValidInvalid)
	t.Run("wrapped HTTPError via %w is detected by errors.As", testResolveErrorResponseWrappedHTTPErrorDetected)
	t.Run("normalizeStatus helpers", testResolveErrorResponseNormalizeStatusHelpers)
	t.Run("default code/message fallbacks", testResolveErrorResponseDefaultCodeMessageFallbacks)
}

func testResolveErrorResponseNilErrInvalidFallback(t *testing.T) {
	st, code, msg := ResolveErrorResponse(nil, 123)
	if st != http.StatusInternalServerError || code != "internal_server_error" || msg != "internal server error" {
		t.Fatalf("got %d %q %q", st, code, msg)
	}
}

func testResolveErrorResponseNonHTTPErrInvalidFallback(t *testing.T) {
	st, code, msg := ResolveErrorResponse(errors.New("boom"), 123)
	if st != http.StatusInternalServerError || code != "internal_server_error" || msg != "internal server error" {
		t.Fatalf("got %d %q %q", st, code, msg)
	}
}

func testResolveErrorResponseHTTPErrorValidStatusInvalidFallback(t *testing.T) {
	he := NewHTTPError(http.StatusNotFound, "   ", "   ", errors.New("x"))
	st, code, msg := ResolveErrorResponse(he, 123)
	if st != http.StatusNotFound || code != "not_found" || msg != "not found" {
		t.Fatalf("got %d %q %q", st, code, msg)
	}
}

func testResolveErrorResponseHTTPErrorInvalidStatusUsesFallback(t *testing.T) {
	he := NewHTTPError(123, "c", "m", errors.New("x"))
	st, code, msg := ResolveErrorResponse(he, http.StatusBadRequest)
	if st != http.StatusBadRequest || code != "c" || msg != "m" {
		t.Fatalf("got %d %q %q", st, code, msg)
	}
}

func testResolveErrorResponseHTTPErrorBlankCodeMessageDefaults(t *testing.T) {
	he := NewHTTPError(http.StatusNotFound, "   ", "  ", nil)
	st, code, msg := ResolveErrorResponse(he, http.StatusBadGateway)
	if st != http.StatusNotFound || code != "not_found" || msg != "not found" {
		t.Fatalf("got %d %q %q", st, code, msg)
	}
}

func testResolveErrorResponseHTTPErrorMixedDefaults(t *testing.T) {
	he1 := NewHTTPError(http.StatusForbidden, "   ", "custom msg", errors.New("x"))
	st, code, msg := ResolveErrorResponse(he1, http.StatusBadRequest)
	if st != http.StatusForbidden || code != "forbidden" || msg != "custom msg" {
		t.Fatalf("he1: got %d %q %q", st, code, msg)
	}

	he2 := NewHTTPError(http.StatusConflict, "custom_code", "   ", errors.New("x"))
	st, code, msg = ResolveErrorResponse(he2, http.StatusBadRequest)
	if st != http.StatusConflict || code != "custom_code" || msg != "conflict" {
		t.Fatalf("he2: got %d %q %q", st, code, msg)
	}
}

func testResolveErrorResponseStatusCoderValidInvalid(t *testing.T) {
	st, code, msg := ResolveErrorResponse(scErr{code: http.StatusTooManyRequests}, http.StatusBadRequest)
	if st != http.StatusTooManyRequests || code != "too_many_requests" || msg != "too many requests" {
		t.Fatalf("valid: got %d %q %q", st, code, msg)
	}

	st, code, msg = ResolveErrorResponse(scErr{code: 1}, http.StatusBadRequest)
	if st != http.StatusBadRequest || code != "bad_request" || msg != "bad request" {
		t.Fatalf("invalid: got %d %q %q", st, code, msg)
	}
}

func testResolveErrorResponseWrappedHTTPErrorDetected(t *testing.T) {
	realWrapped := fmt.Errorf("wrap: %w", NewHTTPError(http.StatusUnauthorized, "", "", errors.New("inner")))
	st, code, msg := ResolveErrorResponse(realWrapped, http.StatusBadRequest)
	if st != http.StatusUnauthorized || code != "unauthorized" || msg != "unauthorized" {
		t.Fatalf("got %d %q %q", st, code, msg)
	}
}

func testResolveErrorResponseNormalizeStatusHelpers(t *testing.T) {
	if _, ok := normalizeStatus(399); ok {
		t.Fatalf("expected false")
	}
	if s, ok := normalizeStatus(400); !ok || s != 400 {
		t.Fatalf("expected 400 true")
	}
	if got := normalizeStatusOrDefault(399, 500); got != 500 {
		t.Fatalf("expected default 500, got %d", got)
	}
}

func testResolveErrorResponseDefaultCodeMessageFallbacks(t *testing.T) {
	if got := defaultErrorCode(418); got != "error" {
		t.Fatalf("defaultErrorCode(418) got %q", got)
	}
	if got := defaultErrorCode(599); got != "internal_server_error" {
		t.Fatalf("defaultErrorCode(599) got %q", got)
	}
	if got := defaultErrorMessage(599); got != "internal server error" {
		t.Fatalf("defaultErrorMessage(599) got %q", got)
	}
	if got := defaultErrorMessage(499); got != "error" {
		t.Fatalf("defaultErrorMessage(499) got %q", got)
	}
	if got := defaultErrorMessage(http.StatusTeapot); got != "i'm a teapot" {
		t.Fatalf("defaultErrorMessage(418) got %q", got)
	}
}

//
// ==========================
// Request ID helpers + middleware
// ==========================
//

func TestRequestIDHelpers(t *testing.T) {
	t.Run("isAllowedRequestIDRune", func(t *testing.T) {
		for _, r := range []rune{'-', '_', '.', ':', 'a', 'Z', '0', '9'} {
			if !isAllowedRequestIDRune(r) {
				t.Fatalf("expected allowed rune %q", string(r))
			}
		}
		for _, r := range []rune{'@', '#', '/', '\\', '\n'} {
			if isAllowedRequestIDRune(r) {
				t.Fatalf("expected disallowed rune %q", string(r))
			}
		}
	})

	t.Run("sanitizeIncomingRequestID", func(t *testing.T) {
		cases := []struct {
			in   string
			ok   bool
			want string
		}{
			{"   ", false, ""},
			{strings.Repeat("a", 129), false, ""},
			{"a\tb", false, ""},
			{"a/b", false, ""},
			{"  Abc-_:.:09  ", true, "Abc-_:.:09"},
		}

		for _, tc := range cases {
			got, ok := sanitizeIncomingRequestID(tc.in)
			if ok != tc.ok {
				t.Fatalf("sanitizeIncomingRequestID(%q): ok=%v want %v", tc.in, ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Fatalf("sanitizeIncomingRequestID(%q): got=%q want %q", tc.in, got, tc.want)
			}
		}
	})
}

func TestRequestIDMiddleware_UsesIncomingOrGenerates(t *testing.T) {
	gin.SetMode(gin.TestMode)

	route := func() *gin.Engine {
		r := gin.New()
		r.Use(RequestIDMiddleware())
		r.GET("/x", func(c *gin.Context) { c.String(http.StatusOK, c.GetString(requestIDContextKey)) })
		return r
	}

	t.Run("incoming valid", func(t *testing.T) {
		r := route()

		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Header.Set(requestIDHeader, "abc-123")
		rr := httptest.NewRecorder()

		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("code=%d", rr.Code)
		}
		if got := rr.Header().Get(requestIDHeader); got != "abc-123" {
			t.Fatalf("header rid=%q", got)
		}
		if rr.Body.String() != "abc-123" {
			t.Fatalf("body=%q", rr.Body.String())
		}
	})

	t.Run("incoming invalid -> generates", func(t *testing.T) {
		r := route()

		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Header.Set(requestIDHeader, "bad/id")
		rr := httptest.NewRecorder()

		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("code=%d", rr.Code)
		}

		rid := rr.Header().Get(requestIDHeader)
		if rid == "" || strings.Contains(rid, "/") {
			t.Fatalf("expected generated rid, got %q", rid)
		}
		if rr.Body.String() != rid {
			t.Fatalf("expected body rid == header rid; body=%q header=%q", rr.Body.String(), rid)
		}
	})
}

func TestTruncate(t *testing.T) {
	if got := truncate("abc", 3); got != "abc" {
		t.Fatalf("got %q", got)
	}
	if got := truncate("abcd", 3); got != "abc…" {
		t.Fatalf("got %q", got)
	}
}

//
// ==========================
// Logging helpers + logger injection middleware
// ==========================
//

func TestLoggerFromAndRequestLoggerFromContext(t *testing.T) {
	if LoggerFrom(nil) == nil {
		t.Fatalf("expected non-nil")
	}
	if requestLoggerFromContext(nil, nil) == nil {
		t.Fatalf("expected non-nil")
	}

	l, _ := newCapturedLogger()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Set(loggerContextKey, l)
	if got := requestLoggerFromContext(c, nil); got == nil {
		t.Fatalf("expected non-nil")
	}

	// Wrong type stored in context should not break.
	c.Set(loggerContextKey, "not-a-logger")
	if got := requestLoggerFromContext(c, l); got == nil {
		t.Fatalf("expected non-nil")
	}
}

func TestInjectRequestLogger(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("base nil falls back; no panic", func(t *testing.T) {
		r := gin.New()
		r.Use(InjectRequestLogger(nil))
		r.GET("/x", func(c *gin.Context) {
			if LoggerFrom(c) == nil {
				t.Fatalf("expected non-nil logger")
			}
			c.Status(http.StatusNoContent)
		})

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))

		if rr.Code != http.StatusNoContent {
			t.Fatalf("code=%d", rr.Code)
		}
	})

	t.Run("adds RID and trace/span IDs when present", func(t *testing.T) {
		rec := tracetest.NewSpanRecorder()
		tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
		oldTP := traceProviderSwap(tp)
		t.Cleanup(func() { traceProviderSwap(oldTP) })

		baseLogger, _ := newCapturedLogger()

		r := gin.New()
		r.Use(RequestIDMiddleware())
		r.Use(InjectRequestLogger(baseLogger))
		r.GET("/x", func(c *gin.Context) { c.Status(http.StatusNoContent) })

		ctx, span := tp.Tracer("t").Start(context.Background(), "span")
		defer span.End()

		req := httptest.NewRequest(http.MethodGet, "/x", nil).WithContext(ctx)
		req.Header.Set(requestIDHeader, "rid-1")

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusNoContent {
			t.Fatalf("code=%d", rr.Code)
		}
	})
}

func TestRequestIDSpanAttrMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("sets request_id attribute on active span", func(t *testing.T) {
		rec := tracetest.NewSpanRecorder()
		tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
		oldTP := traceProviderSwap(tp)
		t.Cleanup(func() { traceProviderSwap(oldTP) })

		r := gin.New()
		r.Use(RequestIDMiddleware())
		r.Use(RequestIDSpanAttrMiddleware())
		r.GET("/x", func(c *gin.Context) { c.Status(http.StatusNoContent) })

		ctx, span := tp.Tracer("t").Start(context.Background(), "span")

		req := httptest.NewRequest(http.MethodGet, "/x", nil).WithContext(ctx)
		req.Header.Set(requestIDHeader, "rid-xyz")

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		span.End()

		ended := rec.Ended()
		if len(ended) == 0 {
			t.Fatalf("expected ended span")
		}

		found := false
		for _, s := range ended {
			for _, a := range s.Attributes() {
				if a.Key == "request_id" && a.Value.AsString() == "rid-xyz" {
					found = true
					break
				}
			}
		}
		if !found {
			t.Fatalf("expected request_id attribute rid-xyz")
		}
	})

	t.Run("missing RID / invalid span: no panic", func(t *testing.T) {
		r := gin.New()
		r.Use(RequestIDSpanAttrMiddleware())
		r.GET("/x", func(c *gin.Context) { c.Status(http.StatusNoContent) })

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))
		if rr.Code != http.StatusNoContent {
			t.Fatalf("code=%d", rr.Code)
		}
	})
}

//
// ==========================
// Error writing + finalizer middleware
// ==========================
//

func TestWriteError_IncludesRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(RequestIDMiddleware())
	r.GET("/x", func(c *gin.Context) {
		WriteError(c, http.StatusTeapot, "teapot", "i am a teapot")
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(requestIDHeader, "rid-1")

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusTeapot {
		t.Fatalf("code=%d", rr.Code)
	}

	m := mustJSON(t, rr.Body.Bytes())
	if m["error"] != "i am a teapot" || m["code"] != "teapot" || m["request_id"] != "rid-1" {
		t.Fatalf("unexpected json: %v", m)
	}
}

func TestErrorFinalizer(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("writes resolved error when handler didn't write", func(t *testing.T) {
		r := gin.New()
		r.Use(RequestIDMiddleware())
		r.Use(ErrorFinalizer())
		r.GET("/x", func(c *gin.Context) {
			_ = c.Error(NewHTTPError(http.StatusBadRequest, "bad_request", "bad request", errors.New("x")))
			// Handler sets 200 but does not write a response body, so finalizer should replace it.
			c.Status(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Header.Set(requestIDHeader, "rid-1")

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
		}

		j := mustJSON(t, rr.Body.Bytes())
		if j["code"] != "bad_request" || j["error"] != "bad request" {
			t.Fatalf("unexpected json: %v", j)
		}
	})

	t.Run("skips when response already written", func(t *testing.T) {
		r := gin.New()
		r.Use(ErrorFinalizer())
		r.GET("/x", func(c *gin.Context) {
			c.String(http.StatusOK, "ok")
			_ = c.Error(errors.New("x"))
		})

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))
		if rr.Code != http.StatusOK || rr.Body.String() != "ok" {
			t.Fatalf("unexpected response: %d %q", rr.Code, rr.Body.String())
		}
	})

	t.Run("skips when no errors", func(t *testing.T) {
		r := gin.New()
		r.Use(ErrorFinalizer())
		r.GET("/x", func(c *gin.Context) { c.Status(http.StatusNoContent) })

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))
		if rr.Code != http.StatusNoContent {
			t.Fatalf("code=%d", rr.Code)
		}
	})

	t.Run("handles nil *gin.Error and nil Err safely", func(t *testing.T) {
		t.Run("last is nil pointer", func(t *testing.T) {
			r := gin.New()
			r.Use(ErrorFinalizer())
			r.GET("/x", func(c *gin.Context) {
				c.Errors = append(c.Errors, nil)
			})

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))
			if rr.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", rr.Code)
			}
		})

		t.Run("last.Err is nil", func(t *testing.T) {
			r := gin.New()
			r.Use(ErrorFinalizer())
			r.GET("/x", func(c *gin.Context) {
				c.Errors = append(c.Errors, &gin.Error{})
			})

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))
			if rr.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", rr.Code)
			}
		})
	})
}

func TestMaxBodyBytesMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("disabled when limit <= 0", func(t *testing.T) {
		r := gin.New()
		r.Use(MaxBodyBytesMiddleware(0))
		r.POST("/x", func(c *gin.Context) { c.Status(http.StatusNoContent) })

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(strings.Repeat("a", 1024))))
		if rr.Code != http.StatusNoContent {
			t.Fatalf("code=%d", rr.Code)
		}
	})

	t.Run("Content-Length pre-check returns 413", func(t *testing.T) {
		r := gin.New()
		r.Use(RequestIDMiddleware())
		r.Use(MaxBodyBytesMiddleware(3))
		r.POST("/x", func(c *gin.Context) { c.Status(http.StatusNoContent) })

		req := httptest.NewRequest(http.MethodPost, "/x", io.NopCloser(strings.NewReader("abcd")))
		req.ContentLength = 4
		req.Header.Set(requestIDHeader, "rid-1")

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("expected 413, got %d body=%s", rr.Code, rr.Body.String())
		}

		j := mustJSON(t, rr.Body.Bytes())
		if j["code"] != "payload_too_large" {
			t.Fatalf("unexpected json: %v", j)
		}
	})
}

func TestMaxBytesErrorMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("ignores non-MaxBytesError", func(t *testing.T) {
		r := gin.New()
		r.Use(MaxBytesErrorMiddleware())
		r.GET("/x", func(c *gin.Context) { _ = c.Error(errors.New("not max bytes")) })

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		if rr.Body.Len() != 0 {
			t.Fatalf("expected empty body, got %q", rr.Body.String())
		}
	})

	t.Run("writes JSON when MaxBytesError and nothing was written yet", func(t *testing.T) {
		limit := int64(3)

		r := gin.New()
		r.Use(RequestIDMiddleware())
		r.Use(MaxBodyBytesMiddleware(limit))
		r.Use(MaxBytesErrorMiddleware())
		r.POST("/x", func(c *gin.Context) {
			_, err := io.ReadAll(c.Request.Body)
			if err != nil {
				_ = c.Error(err)
				return
			}
			c.Status(http.StatusNoContent)
		})

		req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("abcd"))
		req.ContentLength = 3
		req.Header.Set(requestIDHeader, "rid-1")

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("expected 413, got %d body=%s", rr.Code, rr.Body.String())
		}

		j := mustJSON(t, rr.Body.Bytes())
		if j["code"] != "payload_too_large" {
			t.Fatalf("unexpected json: %v", j)
		}
	})

	t.Run("response already written: sets status (no panic)", func(t *testing.T) {
		limit := int64(3)

		r := gin.New()
		r.Use(MaxBodyBytesMiddleware(limit))
		r.Use(MaxBytesErrorMiddleware())
		r.POST("/x", func(c *gin.Context) {
			c.String(http.StatusOK, "already")
			_, err := io.ReadAll(c.Request.Body)
			if err != nil {
				_ = c.Error(err)
			}
		})

		req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("abcd"))
		req.ContentLength = 3

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Body.String() == "" {
			t.Fatalf("expected body written")
		}
	})
}

func TestAppendErrorAttrs(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	_ = c.Error(PanicError{Value: "boom"}).SetType(gin.ErrorTypePrivate)
	attrs, lvl := appendErrorAttrs([]any{"a", 1}, c)
	if lvl != slog.LevelInfo {
		t.Fatalf("expected info, got %v attrs=%v", lvl, attrs)
	}

	_ = c.Error(errors.New("x")).SetType(gin.ErrorTypePrivate)
	attrs, lvl = appendErrorAttrs([]any{}, c)
	if lvl != slog.LevelWarn {
		t.Fatalf("expected warn, got %v attrs=%v", lvl, attrs)
	}

	foundErrors := false
	for i := 0; i < len(attrs)-1; i += 2 {
		if k, ok := attrs[i].(string); ok && k == "errors" {
			foundErrors = true
			break
		}
	}
	if !foundErrors {
		t.Fatalf("expected errors attr, got %v", attrs)
	}
}

//
// ==========================
// Gin access logging middleware (slog + metrics)
// ==========================
//

func TestGinSlogMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := baseTestConfig()

	t.Run("skips access logs for health/ready/metrics when <400; still records metrics", func(t *testing.T) {
		testGinSlogMiddlewareSkipsAccessLogsForHealthStillRecordsMetrics(t, cfg)
	})
	t.Run("skip path but status >= 400 still logs", func(t *testing.T) {
		testGinSlogMiddlewareSkipPathButErrorStillLogs(t, cfg)
	})
	t.Run("gin errors elevate level and include errors attr", func(t *testing.T) {
		testGinSlogMiddlewareGinErrorsElevateLevel(t, cfg)
	})
	t.Run("unmatched route label + HTTP_5XX marker", func(t *testing.T) {
		testGinSlogMiddlewareUnmatchedRouteLabelAndHTTP5XXMarker(t, cfg)
	})
	t.Run("bytesWritten negative is clamped to zero", func(t *testing.T) {
		testGinSlogMiddlewareBytesWrittenNegativeClamped(t, cfg)
	})
	t.Run("tiny latency rounding stays >0 and deterministic", func(t *testing.T) {
		testGinSlogMiddlewareTinyLatencyRoundingDeterministic(t, cfg)
	})
	t.Run("matched route uses FullPath for metrics label and log path", func(t *testing.T) {
		testGinSlogMiddlewareMatchedRouteUsesFullPath(t, cfg)
	})
	t.Run("nil base logger does not panic and still records metrics", func(t *testing.T) {
		testGinSlogMiddlewareNilLoggerNoPanicStillRecordsMetrics(t, cfg)
	})
}

func testGinSlogMiddlewareSkipsAccessLogsForHealthStillRecordsMetrics(t *testing.T, cfg Config) {
	t.Helper()

	logger, h := newCapturedLogger()
	m := NewMetrics(cfg)

	r := gin.New()
	r.Use(GinSlogMiddleware(logger, cfg, m))
	r.GET(LivenessPath, func(c *gin.Context) { c.Status(http.StatusOK) })

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("code=%d", rr.Code)
	}
	if h.Count() != 0 {
		t.Fatalf("expected no logs, got %d", h.Count())
	}
	if got := testutil.ToFloat64(m.HTTPRequestsTotal.WithLabelValues("GET", LivenessPath, "200")); got < 1 {
		t.Fatalf("expected counter increment, got %v", got)
	}
}

func testGinSlogMiddlewareSkipPathButErrorStillLogs(t *testing.T, cfg Config) {
	t.Helper()

	logger, h := newCapturedLogger()
	m := NewMetrics(cfg)

	r := gin.New()
	r.Use(GinSlogMiddleware(logger, cfg, m))
	r.GET(LivenessPath, func(c *gin.Context) { c.Status(http.StatusInternalServerError) })

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("code=%d", rr.Code)
	}
	if h.Count() == 0 {
		t.Fatalf("expected log record")
	}
}

func testGinSlogMiddlewareGinErrorsElevateLevel(t *testing.T, cfg Config) {
	t.Helper()

	logger, h := newCapturedLogger()
	m := NewMetrics(cfg)

	r := gin.New()
	r.Use(RequestIDMiddleware())
	r.Use(GinSlogMiddleware(logger, cfg, m))
	r.GET("/x", func(c *gin.Context) {
		_ = c.Error(errors.New("boom"))
		c.Status(http.StatusOK)
	})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("code=%d", rr.Code)
	}

	rec, ok := h.Last()
	if !ok {
		t.Fatalf("missing log")
	}
	if rec.Level != slog.LevelWarn {
		t.Fatalf("expected warn, got %v", rec.Level)
	}
	attrs := recordAttrsToMap(rec)
	if _, ok := attrs["errors"]; !ok {
		t.Fatalf("expected errors attr, got %v", attrs)
	}
}

func testGinSlogMiddlewareUnmatchedRouteLabelAndHTTP5XXMarker(t *testing.T, cfg Config) {
	t.Helper()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	m := NewMetrics(cfg)

	r := gin.New()
	r.Use(RequestIDMiddleware(), InjectRequestLogger(logger), GinSlogMiddleware(logger, cfg, m))

	r.NoRoute(func(c *gin.Context) {
		c.Status(http.StatusInternalServerError)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", w.Code)
	}

	line := buf.String()
	if !strings.Contains(line, `"path":"`+unmatchedRouteLabel+`"`) {
		t.Fatalf("expected path=%s, got log: %s", unmatchedRouteLabel, line)
	}
	if !strings.Contains(line, `"error":"HTTP_5XX"`) {
		t.Fatalf("expected HTTP_5XX marker, got log: %s", line)
	}
}

func testGinSlogMiddlewareBytesWrittenNegativeClamped(t *testing.T, cfg Config) {
	t.Helper()

	logger, h := newCapturedLogger()
	m := NewMetrics(cfg)

	r := gin.New()
	r.Use(GinSlogMiddleware(logger, cfg, m))
	r.GET("/x", func(c *gin.Context) {
		c.Writer = &sizeNegWriter{ResponseWriter: c.Writer}
		c.Status(http.StatusOK)
	})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	rec, ok := h.Last()
	if !ok {
		t.Fatalf("expected log record")
	}
	attrs := recordAttrsToMap(rec)

	v, ok := attrs["bytes_written"]
	if !ok {
		t.Fatalf("expected bytes_written attr; got %v", attrs)
	}

	switch n := v.(type) {
	case int:
		if n != 0 {
			t.Fatalf("expected bytes_written=0, got %v", v)
		}
	case int64:
		if n != 0 {
			t.Fatalf("expected bytes_written=0, got %v", v)
		}
	default:
		t.Fatalf("unexpected bytes_written type %T val=%v", v, v)
	}
}

func testGinSlogMiddlewareTinyLatencyRoundingDeterministic(t *testing.T, cfg Config) {
	t.Helper()

	oldNow, oldSince := timeNow, timeSince
	t.Cleanup(func() {
		timeNow = oldNow
		timeSince = oldSince
	})

	fixed := time.Unix(0, 0)
	timeNow = func() time.Time { return fixed }
	timeSince = func(time.Time) time.Duration { return time.Nanosecond }

	logger, h := newCapturedLogger()
	m := NewMetrics(cfg)

	r := gin.New()
	r.Use(GinSlogMiddleware(logger, cfg, m))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("code=%d", rr.Code)
	}

	rec, ok := h.Last()
	if !ok {
		t.Fatalf("expected log record")
	}
	attrs := recordAttrsToMap(rec)

	v, ok := attrs["latency_ms"]
	if !ok {
		t.Fatalf("expected latency_ms attr; attrs=%v", attrs)
	}

	x, ok := v.(float64)
	if !ok {
		t.Fatalf("unexpected latency_ms type %T value=%v", v, v)
	}
	if x <= 0 || x >= 0.001 {
		t.Fatalf("expected tiny positive latency_ms, got %v", x)
	}
}

func testGinSlogMiddlewareMatchedRouteUsesFullPath(t *testing.T, cfg Config) {
	t.Helper()

	logger, h := newCapturedLogger()
	m := NewMetrics(cfg)

	r := gin.New()
	r.Use(GinSlogMiddleware(logger, cfg, m))
	r.GET("/users/:id", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/users/123", nil))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("code=%d", rr.Code)
	}

	rec, ok := h.Last()
	if !ok {
		t.Fatalf("expected log record")
	}

	attrs := recordAttrsToMap(rec)
	if got := attrs["path"]; got != "/users/:id" {
		t.Fatalf("expected path=/users/:id, got %v", got)
	}
	if got := attrs["raw_path"]; got != "/users/123" {
		t.Fatalf("expected raw_path=/users/123, got %v", got)
	}
}

func testGinSlogMiddlewareNilLoggerNoPanicStillRecordsMetrics(t *testing.T, cfg Config) {
	t.Helper()

	m := NewMetrics(cfg)
	mw := GinSlogMiddleware(nil, cfg, m)

	r := gin.New()
	r.Use(mw)
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("code=%d", rr.Code)
	}

	if got := testutil.ToFloat64(m.HTTPRequestsTotal.WithLabelValues("GET", "/x", "204")); got != 1 {
		t.Fatalf("expected counter=1, got %v", got)
	}
}

//
// ==========================
// Router setup / routing behavior
// ==========================
//

func TestNotFoundAndMethodNotAllowedHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := baseTestConfig()
	metrics := NewMetrics(cfg)
	logger, _ := newCapturedLogger()

	r := SetupRouter(cfg, logger, metrics)

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/nope", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	j := mustJSON(t, rr.Body.Bytes())
	if j["code"] != "not_found" {
		t.Fatalf("unexpected json: %v", j)
	}

	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/health", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
	j = mustJSON(t, rr.Body.Bytes())
	if j["code"] != "method_not_allowed" {
		t.Fatalf("unexpected json: %v", j)
	}
}

func TestSetupRouter_Routes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := baseTestConfig()
	cfg.Version = "v1"
	cfg.BuildTime = "bt1"

	metrics := NewMetrics(cfg)
	logger, _ := newCapturedLogger()

	r := SetupRouter(cfg, logger, metrics)

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/info", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("info code=%d body=%s", rr.Code, rr.Body.String())
	}
	j := mustJSON(t, rr.Body.Bytes())
	if j["service"] != testServiceName || j["version"] != "v1" || j["build_time"] != "bt1" {
		t.Fatalf("unexpected /info: %v", j)
	}

	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("/health expected 200, got %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("/ready expected 200, got %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "golang-gin-app is running") {
		t.Fatalf("root: %d %q", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("metrics code=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Content-Type") == "" {
		t.Fatalf("expected Content-Type")
	}
}

//
// ==========================
// Trusted proxies
// ==========================
//

func TestApplyTrustedProxies(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("empty list -> sets nil", func(_ *testing.T) {
		cfg := Config{GinMode: gin.TestMode, TrustedProxies: nil}
		r := gin.New()
		applyTrustedProxies(r, cfg, nil)
	})

	t.Run("invalid CIDR -> logs and resets", func(t *testing.T) {
		cfg := Config{GinMode: gin.TestMode, TrustedProxies: []string{"not-a-cidr"}}
		logger, h := newCapturedLogger()

		r := gin.New()
		applyTrustedProxies(r, cfg, logger)

		if h.Count() != 1 {
			t.Fatalf("expected 1 log record, got %d", h.Count())
		}
		rec, _ := h.Last()
		if rec.Message != "invalid_trusted_proxies" {
			t.Fatalf("expected invalid_trusted_proxies, got %q", rec.Message)
		}
	})

	t.Run("invalid CIDR with nil logger -> no panic", func(_ *testing.T) {
		cfg := Config{GinMode: gin.TestMode, TrustedProxies: []string{"not-a-cidr"}}
		r := gin.New()
		applyTrustedProxies(r, cfg, nil)
	})

	t.Run("valid CIDRs -> no error log", func(t *testing.T) {
		cfg := Config{GinMode: gin.TestMode, TrustedProxies: []string{"10.0.0.0/8", "172.16.0.0/12"}}
		logger, h := newCapturedLogger()

		r := gin.New()
		applyTrustedProxies(r, cfg, logger)

		if h.Count() != 0 {
			last, _ := h.Last()
			t.Fatalf("expected no logs, got last=%q", last.Message)
		}
	})
}

//
// ==========================
// Recovery middleware (panic -> 500 + logs)
// ==========================
//

func TestGinRecoveryWithSlog(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("recovers panic, writes 500, logs via request logger", func(t *testing.T) {
		logger, h := newCapturedLogger()

		r := gin.New()
		r.Use(InjectRequestLogger(logger))
		r.Use(GinRecoveryWithSlog())
		r.GET("/panic", func(_ *gin.Context) { panic("boom") })

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/panic", nil))

		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d body=%s", rr.Code, rr.Body.String())
		}
		j := mustJSON(t, rr.Body.Bytes())
		if j["code"] != "internal_server_error" || j["error"] != "internal server error" {
			t.Fatalf("unexpected json: %v", j)
		}
		if h.Count() == 0 {
			t.Fatalf("expected logs")
		}
	})

	t.Run("no FullPath uses raw path fallback; no injected logger", func(t *testing.T) {
		r := gin.New()
		r.Use(GinRecoveryWithSlog())
		r.NoRoute(func(_ *gin.Context) { panic("boom") })

		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/no-route", nil))

		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d body=%s", rr.Code, rr.Body.String())
		}
		j := mustJSON(t, rr.Body.Bytes())
		if j["code"] != "internal_server_error" || j["error"] != "internal server error" {
			t.Fatalf("unexpected json: %v", j)
		}
	})
}

//
// ==========================
// Misc: types + constructors
// ==========================
//

func TestPanicError_Error(t *testing.T) {
	if got := (PanicError{Value: "x"}).Error(); got != "x" {
		t.Fatalf("expected 'x', got %q", got)
	}
}

func TestNewLogger_ReturnsBothLoggers(t *testing.T) {
	l, std := NewLogger(slog.LevelInfo)
	if l == nil || std == nil {
		t.Fatalf("expected non-nil loggers")
	}
}

func TestNewMetrics_RegistersAndSetsBuildInfo(t *testing.T) {
	cfg := Config{Version: "v", BuildTime: "bt"}

	m := NewMetrics(cfg)
	if m == nil || m.Registry == nil || m.HTTPRequestsTotal == nil || m.HTTPRequestDurationSeconds == nil || m.BuildInfo == nil {
		t.Fatalf("expected non-nil metrics and fields")
	}

	if got := testutil.ToFloat64(m.BuildInfo.WithLabelValues(cfg.Version, cfg.BuildTime)); got != 1 {
		t.Fatalf("expected build_info=1, got %v", got)
	}

	assertAlreadyRegistered := func(c prometheus.Collector) {
		t.Helper()

		err := m.Registry.Register(c)
		if err == nil {
			t.Fatalf("expected AlreadyRegisteredError, got nil")
		}
		var are prometheus.AlreadyRegisteredError
		if !errors.As(err, &are) {
			t.Fatalf("expected AlreadyRegisteredError, got %T: %v", err, err)
		}
	}

	assertAlreadyRegistered(m.HTTPRequestsTotal)
	assertAlreadyRegistered(m.HTTPRequestDurationSeconds)
	assertAlreadyRegistered(m.BuildInfo)
}
