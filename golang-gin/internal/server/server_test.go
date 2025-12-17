package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func newTestConfig() Config {
	return Config{
		GinMode:           gin.TestMode,
		Host:              "0.0.0.0",
		Port:              "8080",
		LogLevel:          slog.LevelInfo,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
		IdleTimeout:       120 * time.Second,
		ShutdownTimeout:   5 * time.Second,
		ServiceName:       "golang-gin-app",
		Version:           "test-version",
		BuildTime:         "test-build-time",
	}
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"unknown", slog.LevelInfo},
	}

	for _, tt := range tests {
		if got := parseLogLevel(tt.input); got != tt.want {
			t.Errorf("parseLogLevel(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParseDurationEnv(t *testing.T) {
	t.Setenv("TEST_DURATION", "150ms")
	d := parseDurationEnv("TEST_DURATION", 1*time.Second)
	if d != 150*time.Millisecond {
		t.Fatalf("expected 150ms, got %s", d)
	}

	t.Setenv("TEST_DURATION_EMPTY", "")
	d = parseDurationEnv("TEST_DURATION_EMPTY", 2*time.Second)
	if d != 2*time.Second {
		t.Fatalf("expected 2s default, got %s", d)
	}

	t.Setenv("TEST_DURATION_BAD", "not-a-duration")
	d = parseDurationEnv("TEST_DURATION_BAD", 3*time.Second)
	if d != 3*time.Second {
		t.Fatalf("expected 3s default on invalid, got %s", d)
	}
}

func TestConfigValidateOK(t *testing.T) {
	cfg := newTestConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}
}

func TestConfigValidateHostRequired(t *testing.T) {
	cfg := newTestConfig()
	cfg.Host = "  "
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for empty HOST, got nil")
	}
}

func TestConfigValidateBadPort(t *testing.T) {
	cfg := newTestConfig()
	cfg.Port = "not-a-port"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for invalid port, got nil")
	}
}

func TestConfigValidateBadGinMode(t *testing.T) {
	cfg := newTestConfig()
	cfg.GinMode = "invalid-mode"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for invalid gin mode, got nil")
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	t.Setenv("GIN_MODE", "")
	t.Setenv("HOST", "")
	t.Setenv("PORT", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("READ_TIMEOUT", "")
	t.Setenv("READ_HEADER_TIMEOUT", "")
	t.Setenv("IDLE_TIMEOUT", "")
	t.Setenv("SHUTDOWN_TIMEOUT", "")

	const (
		serviceName = "golang-gin-app"
		version     = "1.2.3"
		buildTime   = "2025-01-01T00:00:00Z"
	)

	cfg := LoadConfig(serviceName, version, buildTime)

	if cfg.GinMode != gin.ReleaseMode {
		t.Fatalf("expected GinMode %q, got %q", gin.ReleaseMode, cfg.GinMode)
	}
	if cfg.Host != "0.0.0.0" {
		t.Fatalf("expected Host 0.0.0.0, got %q", cfg.Host)
	}
	if cfg.Port != "8080" {
		t.Fatalf("expected Port 8080, got %q", cfg.Port)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Fatalf("expected LogLevel info, got %v", cfg.LogLevel)
	}
	if cfg.ReadTimeout != 5*time.Second {
		t.Fatalf("expected ReadTimeout 5s, got %s", cfg.ReadTimeout)
	}
	if cfg.WriteTimeout != 10*time.Second {
		t.Fatalf("expected WriteTimeout 10s, got %s", cfg.WriteTimeout)
	}
	if cfg.ReadHeaderTimeout != 2*time.Second {
		t.Fatalf("expected ReadHeaderTimeout 2s, got %s", cfg.ReadHeaderTimeout)
	}
	if cfg.IdleTimeout != 120*time.Second {
		t.Fatalf("expected IdleTimeout 120s, got %s", cfg.IdleTimeout)
	}
	if cfg.ShutdownTimeout != 5*time.Second {
		t.Fatalf("expected ShutdownTimeout 5s, got %s", cfg.ShutdownTimeout)
	}
	if cfg.ServiceName != serviceName {
		t.Fatalf("expected ServiceName %q, got %q", serviceName, cfg.ServiceName)
	}
	if cfg.Version != version {
		t.Fatalf("expected Version %q, got %q", version, cfg.Version)
	}
	if cfg.BuildTime != buildTime {
		t.Fatalf("expected BuildTime %q, got %q", buildTime, cfg.BuildTime)
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	t.Setenv("GIN_MODE", gin.DebugMode)
	t.Setenv("HOST", "127.0.0.1")
	t.Setenv("PORT", "9000")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("READ_TIMEOUT", "1s")
	t.Setenv("WRITE_TIMEOUT", "2s")
	t.Setenv("READ_HEADER_TIMEOUT", "500ms")
	t.Setenv("IDLE_TIMEOUT", "10s")
	t.Setenv("SHUTDOWN_TIMEOUT", "7s")

	cfg := LoadConfig("svc-name", "vX", "buildX")

	if cfg.GinMode != gin.DebugMode {
		t.Fatalf("expected GinMode %q, got %q", gin.DebugMode, cfg.GinMode)
	}
	if cfg.Host != "127.0.0.1" {
		t.Fatalf("expected Host 127.0.0.1, got %q", cfg.Host)
	}
	if cfg.Port != "9000" {
		t.Fatalf("expected Port 9000, got %q", cfg.Port)
	}
	if cfg.LogLevel != slog.LevelDebug {
		t.Fatalf("expected LogLevel debug, got %v", cfg.LogLevel)
	}
	if cfg.ReadTimeout != 1*time.Second {
		t.Fatalf("expected ReadTimeout 1s, got %s", cfg.ReadTimeout)
	}
	if cfg.WriteTimeout != 2*time.Second {
		t.Fatalf("expected WriteTimeout 2s, got %s", cfg.WriteTimeout)
	}
	if cfg.ReadHeaderTimeout != 500*time.Millisecond {
		t.Fatalf("expected ReadHeaderTimeout 500ms, got %s", cfg.ReadHeaderTimeout)
	}
	if cfg.IdleTimeout != 10*time.Second {
		t.Fatalf("expected IdleTimeout 10s, got %s", cfg.IdleTimeout)
	}
	if cfg.ShutdownTimeout != 7*time.Second {
		t.Fatalf("expected ShutdownTimeout 7s, got %s", cfg.ShutdownTimeout)
	}
	if cfg.ServiceName != "svc-name" || cfg.Version != "vX" || cfg.BuildTime != "buildX" {
		t.Fatalf("unexpected service metadata: %+v", cfg)
	}
}

func TestRoutes(t *testing.T) {
	cfg := newTestConfig()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	metrics := NewMetrics(cfg)

	r := SetupRouter(cfg, logger, metrics)

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantBody   string
	}{
		{"liveness", "GET", LivenessPath, http.StatusOK, ""},
		{"readiness", "GET", ReadinessPath, http.StatusOK, ""},
		{"root", "GET", RootPath, http.StatusOK, "golang-gin-app is running (Go + Gin)\n"},
		{"notFound", "GET", "/does-not-exist", http.StatusNotFound, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(tt.method, tt.path, nil)
			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d", tt.wantStatus, w.Code)
			}
			if tt.wantBody != "" && w.Body.String() != tt.wantBody {
				t.Fatalf("expected body %q, got %q", tt.wantBody, w.Body.String())
			}
		})
	}
}

func TestInfoRouteReturnsMetadata(t *testing.T) {
	cfg := newTestConfig()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	metrics := NewMetrics(cfg)
	r := SetupRouter(cfg, logger, metrics)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", InfoPath, nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if body["service"] != cfg.ServiceName {
		t.Fatalf("expected service %q, got %q", cfg.ServiceName, body["service"])
	}
	if body["version"] != cfg.Version {
		t.Fatalf("expected version %q, got %q", cfg.Version, body["version"])
	}
	if body["buildTime"] != cfg.BuildTime {
		t.Fatalf("expected buildTime %q, got %q", cfg.BuildTime, body["buildTime"])
	}
}

func TestMetricsRouteExists(t *testing.T) {
	cfg := newTestConfig()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	metrics := NewMetrics(cfg)
	r := SetupRouter(cfg, logger, metrics)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", MetricsPath, nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestGinSlogMiddlewareSkipsInfraEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	cfg := newTestConfig()
	metrics := NewMetrics(cfg)

	r := gin.New()
	r.Use(GinSlogMiddleware(logger, cfg, metrics))

	r.GET(LivenessPath, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	r.GET(ReadinessPath, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	r.GET(MetricsPath, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	for _, path := range []string{LivenessPath, ReadinessPath, MetricsPath} {
		buf.Reset()

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, path, nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status %d for %s, got %d", http.StatusOK, path, w.Code)
		}

		count := testutil.ToFloat64(
			metrics.HTTPRequestsTotal.WithLabelValues(cfg.ServiceName, http.MethodGet, path, "200"),
		)
		if count != 1 {
			t.Fatalf("expected metrics count 1 for %s, got %v", path, count)
		}

		if buf.Len() != 0 {
			t.Fatalf("expected no logs for %s, got %q", path, buf.String())
		}
	}
}

func TestGinSlogMiddlewareLogsNormalRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	cfg := newTestConfig()
	metrics := NewMetrics(cfg)

	r := gin.New()
	r.Use(GinSlogMiddleware(logger, cfg, metrics))
	r.GET(RootPath, func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", RootPath, nil)
	r.ServeHTTP(w, req)

	out := buf.String()
	if out == "" {
		t.Fatalf("expected some logs for %s, got empty output", RootPath)
	}
	if !bytes.Contains([]byte(out), []byte("http_request")) {
		t.Fatalf("expected \"http_request\" in logs, got %q", out)
	}
}

func TestGinRecoveryWithSlogRecoversPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	r := gin.New()
	r.Use(GinRecoveryWithSlog(logger))
	r.GET("/panic", func(c *gin.Context) {
		panic("boom")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/panic", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}

	out := buf.String()
	if out == "" {
		t.Fatalf("expected panic logs, got empty output")
	}
	if !bytes.Contains([]byte(out), []byte("panic_recovered")) {
		t.Fatalf("expected \"panic_recovered\" in logs, got %q", out)
	}
}
