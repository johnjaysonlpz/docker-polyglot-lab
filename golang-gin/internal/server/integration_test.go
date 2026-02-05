package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.opentelemetry.io/otel"

	"github.com/johnjaysonlpz/docker-polyglot-lab/golang-gin/internal/telemetry"
)

/*
INTEGRATION TESTS (real sockets)

These tests bind real TCP listeners and exercise the full net/http server lifecycle:
accept loop, keep-alives, connection reuse, and graceful shutdown.

Why not httptest?
- httptest exercises handler logic, but it does not cover socket behavior.
- Real listeners catch issues around shutdown races, timeouts, and leaked connections.

Parallelism:
These tests intentionally DO NOT call t.Parallel().
Gin mode and OpenTelemetry globals are process-wide; running these in parallel can create
order-dependent failures and data races.
*/

const (
	itServiceName = "svc"
	itVersion     = "v"
	itBuildTime   = "bt"
)

const (
	itClientTimeout   = 5 * time.Second
	itReqTimeout      = 2 * time.Second
	itReadyTimeout    = 10 * time.Second
	itShutdownTimeout = 10 * time.Second
)

// drainAndClose drains the response body so the underlying TCP connection can be reused.
// Skipping the drain commonly leads to socket churn and goroutine buildup under repetition.
func drainAndClose(rc io.ReadCloser) {
	if rc == nil {
		return
	}
	_, _ = io.Copy(io.Discard, rc)
	_ = rc.Close()
}

func newTestHTTPClient(t *testing.T) *http.Client {
	t.Helper()

	// A dedicated Transport per test prevents cross-test coupling and allows safe cleanup.
	tr := &http.Transport{
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 200,
		IdleConnTimeout:     30 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   2 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}

	t.Cleanup(func() {
		tr.CloseIdleConnections()
	})

	return &http.Client{
		Timeout:   itClientTimeout,
		Transport: tr,
	}
}

func mustJSONMap(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("expected json body, got err=%v body=%q", err, string(b))
	}
	return m
}

// startHTTPServer starts a real http.Server on 127.0.0.1:0 and blocks until it is reachable.
// The returned stop() shuts down the server and waits for Serve() to exit.
func startHTTPServer(t *testing.T) (baseURL string, client *http.Client, stop func()) {
	t.Helper()
	baseURL, client, stop, _ = startHTTPServerWithMetrics(t)
	return baseURL, client, stop
}

// startHTTPServerWithMetrics is startHTTPServer plus access to the Metrics instance for assertions.
func startHTTPServerWithMetrics(t *testing.T) (baseURL string, client *http.Client, stop func(), metrics *Metrics) {
	t.Helper()

	gin.SetMode(gin.TestMode)

	cfg := Config{
		GinMode:           gin.TestMode,
		Host:              "127.0.0.1",
		Port:              0,
		LogLevel:          slog.LevelInfo,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
		ShutdownTimeout:   10 * time.Second,
		MaxBodyBytes:      1024 * 1024,
		ServiceName:       itServiceName,
		Version:           itVersion,
		BuildTime:         itBuildTime,
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
	metrics = NewMetrics(cfg)
	router := SetupRouter(cfg, logger, metrics)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}

	srv := &http.Server{
		Handler:           router,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	baseURL = "http://" + ln.Addr().String()
	client = newTestHTTPClient(t)

	// Readiness probe: do not return until the server is actually accepting requests.
	readyCtx, cancel := context.WithTimeout(context.Background(), itReadyTimeout)
	defer cancel()

	for {
		if readyCtx.Err() != nil {
			_ = ln.Close()
			_ = srv.Close()
			t.Fatalf("server did not become ready in time: %s", baseURL)
		}

		req, _ := http.NewRequestWithContext(readyCtx, http.MethodGet, baseURL+LivenessPath, nil)
		resp, pingErr := client.Do(req)
		if pingErr == nil {
			drainAndClose(resp.Body)
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	stop = func() {
		t.Helper()

		ctx, cancel := context.WithTimeout(context.Background(), itShutdownTimeout)
		defer cancel()

		// Prefer graceful shutdown; if it can't complete, force Close() so Serve() unblocks.
		if err := srv.Shutdown(ctx); err != nil {
			_ = srv.Close()
		}

		// Ensure the accept loop goroutine exits (no goroutine leaks across -count=N).
		select {
		case err := <-errCh:
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				t.Fatalf("server Serve returned unexpected error: %v", err)
			}
		case <-ctx.Done():
			t.Fatalf("server did not stop in time: %v", ctx.Err())
		}

		_ = ln.Close()

		// Keep-alives can hold sockets/goroutines; closing idles keeps long stress runs stable.
		if tr, ok := client.Transport.(*http.Transport); ok {
			tr.CloseIdleConnections()
		}
	}

	return baseURL, client, stop, metrics
}

// startHTTPServerWithEngine is startHTTPServer but uses a caller-provided gin.Engine.
func startHTTPServerWithEngine(t *testing.T, engine *gin.Engine) (baseURL string, client *http.Client, stop func()) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}

	srv := &http.Server{Handler: engine}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	baseURL = "http://" + ln.Addr().String()
	client = newTestHTTPClient(t)

	readyCtx, cancel := context.WithTimeout(context.Background(), itReadyTimeout)
	defer cancel()

	for {
		if readyCtx.Err() != nil {
			_ = ln.Close()
			_ = srv.Close()
			t.Fatalf("server did not become ready in time: %s", baseURL)
		}

		req, _ := http.NewRequestWithContext(readyCtx, http.MethodGet, baseURL+"/__ready", nil)
		resp, pingErr := client.Do(req)
		if pingErr == nil {
			drainAndClose(resp.Body)
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	stop = func() {
		t.Helper()

		ctx, cancel := context.WithTimeout(context.Background(), itShutdownTimeout)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			_ = srv.Close()
		}

		select {
		case err := <-errCh:
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				t.Fatalf("server Serve returned unexpected error: %v", err)
			}
		case <-ctx.Done():
			t.Fatalf("server did not stop in time: %v", ctx.Err())
		}

		_ = ln.Close()

		if tr, ok := client.Transport.(*http.Transport); ok {
			tr.CloseIdleConnections()
		}
	}

	return baseURL, client, stop
}

func TestIntegration_ServerLifecycle_HealthAndShutdown(t *testing.T) {
	baseURL, client, stop := startHTTPServer(t)
	defer stop()

	reqCtx, cancel := context.WithTimeout(context.Background(), itReqTimeout)
	defer cancel()

	req, _ := http.NewRequestWithContext(reqCtx, http.MethodGet, baseURL+LivenessPath, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", LivenessPath, err)
	}
	drainAndClose(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, resp.StatusCode)
	}
}

func TestIntegration_RequestID_Propagation_E2E(t *testing.T) {
	baseURL, client, stop := startHTTPServer(t)
	defer stop()

	const rid = "it-rid-12345"

	reqCtx, cancel := context.WithTimeout(context.Background(), itReqTimeout)
	defer cancel()

	// Use a missing route to force the default error JSON path.
	req, _ := http.NewRequestWithContext(reqCtx, http.MethodGet, baseURL+"/definitely-not-a-route", nil)
	req.Header.Set(requestIDHeader, rid)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	drainAndClose(resp.Body)

	if got := resp.Header.Get(requestIDHeader); got != rid {
		t.Fatalf("expected %s header %q, got %q", requestIDHeader, rid, got)
	}

	m := mustJSONMap(t, body)
	if got, _ := m["request_id"].(string); got != rid {
		t.Fatalf("expected json request_id=%q, got %v", rid, m["request_id"])
	}
}

func TestIntegration_RequestID_GeneratesWhenMissing(t *testing.T) {
	baseURL, client, stop := startHTTPServer(t)
	defer stop()

	reqCtx, cancel := context.WithTimeout(context.Background(), itReqTimeout)
	defer cancel()

	// Hit a missing route so we definitely get JSON back (and request_id included).
	req, _ := http.NewRequestWithContext(reqCtx, http.MethodGet, baseURL+"/missing-route-for-rid", nil)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	drainAndClose(resp.Body)

	gotRID := resp.Header.Get(requestIDHeader)
	if gotRID == "" {
		t.Fatalf("expected %s header to be set", requestIDHeader)
	}
	if strings.Contains(gotRID, "/") {
		t.Fatalf("expected generated RID to not contain '/': %q", gotRID)
	}

	m := mustJSONMap(t, body)
	jrid, _ := m["request_id"].(string)
	if jrid == "" {
		t.Fatalf("expected json request_id to be set, got %v", m["request_id"])
	}
	if jrid != gotRID {
		t.Fatalf("expected header request_id == json request_id; header=%q json=%q", gotRID, jrid)
	}
}

func TestIntegration_Metrics_HTTPRequestsTotal_Increments(t *testing.T) {
	baseURL, client, stop, metrics := startHTTPServerWithMetrics(t)
	defer stop()

	before := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("GET", InfoPath, "200"))

	reqCtx, cancel := context.WithTimeout(context.Background(), itReqTimeout)
	defer cancel()

	req, _ := http.NewRequestWithContext(reqCtx, http.MethodGet, baseURL+InfoPath, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s failed: %v", InfoPath, err)
	}
	drainAndClose(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	after := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("GET", InfoPath, "200"))
	if after-before != 1 {
		t.Fatalf("expected HTTPRequestsTotal delta=1, got before=%v after=%v delta=%v", before, after, after-before)
	}
}

func TestIntegration_Metrics_Endpoint_FormatLooksPrometheus(t *testing.T) {
	baseURL, client, stop := startHTTPServer(t)
	defer stop()

	reqCtx, cancel := context.WithTimeout(context.Background(), itReqTimeout)
	defer cancel()

	req, _ := http.NewRequestWithContext(reqCtx, http.MethodGet, baseURL+MetricsPath, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s failed: %v", MetricsPath, err)
	}
	body, _ := io.ReadAll(resp.Body)
	drainAndClose(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", resp.StatusCode, string(body))
	}

	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		t.Fatalf("expected Content-Type to be set")
	}

	s := string(body)

	// Very lightweight “format” sanity:
	// - Prometheus exposition usually contains HELP/TYPE lines and metric names.
	// - We assert on our known metric families to avoid brittle full parsing.
	if !strings.Contains(s, "\n# HELP ") && !strings.HasPrefix(s, "# HELP ") {
		t.Fatalf("expected prometheus HELP lines, got body=%q", s)
	}
	if !strings.Contains(s, "http_requests_total") {
		t.Fatalf("expected http_requests_total to exist, got body=%q", s)
	}
	if !strings.Contains(s, "http_request_duration_seconds") {
		t.Fatalf("expected http_request_duration_seconds to exist, got body=%q", s)
	}
	if !strings.Contains(s, "build_info") {
		t.Fatalf("expected build_info to exist, got body=%q", s)
	}
}

func TestIntegration_ErrorFinalizer_HandlerErrorToJSON_IncludesRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))

	cfg := Config{
		GinMode:           gin.TestMode,
		Host:              "127.0.0.1",
		Port:              0,
		LogLevel:          slog.LevelInfo,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
		ShutdownTimeout:   10 * time.Second,
		MaxBodyBytes:      1024 * 1024,
		ServiceName:       itServiceName,
		Version:           itVersion,
		BuildTime:         itBuildTime,
	}

	m := NewMetrics(cfg)

	r := gin.New()
	r.GET("/__ready", func(c *gin.Context) { c.Status(http.StatusOK) })

	applyMiddlewares(r, cfg, logger, m)

	r.GET("/finalize", func(c *gin.Context) {
		_ = c.Error(NewHTTPError(http.StatusBadRequest, "bad_request", "bad request", nil))
		c.Status(http.StatusOK) // the finalizer must override this to 400
	})

	baseURL, client, stop := startHTTPServerWithEngine(t, r)
	defer stop()

	const rid = "it-finalizer-rid-1"

	reqCtx, cancel := context.WithTimeout(context.Background(), itReqTimeout)
	defer cancel()

	req, _ := http.NewRequestWithContext(reqCtx, http.MethodGet, baseURL+"/finalize", nil)
	req.Header.Set(requestIDHeader, rid)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /finalize failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	drainAndClose(resp.Body)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d body=%q", http.StatusBadRequest, resp.StatusCode, string(body))
	}

	if got := resp.Header.Get(requestIDHeader); got != rid {
		t.Fatalf("expected %s header %q, got %q", requestIDHeader, rid, got)
	}

	j := mustJSONMap(t, body)
	if j["code"] != "bad_request" || j["error"] != "bad request" {
		t.Fatalf("unexpected json: %v", j)
	}
	if got, _ := j["request_id"].(string); got != rid {
		t.Fatalf("expected json request_id=%q, got %v", rid, j["request_id"])
	}
}

func TestIntegration_Metrics_SeesFinalizedStatus_ForFinalizeEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))

	cfg := Config{
		GinMode:           gin.TestMode,
		Host:              "127.0.0.1",
		Port:              0,
		LogLevel:          slog.LevelInfo,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
		ShutdownTimeout:   10 * time.Second,
		MaxBodyBytes:      1024 * 1024,
		ServiceName:       itServiceName,
		Version:           itVersion,
		BuildTime:         itBuildTime,
	}

	metrics := NewMetrics(cfg)

	r := gin.New()
	r.GET("/__ready", func(c *gin.Context) { c.Status(http.StatusOK) })
	applyMiddlewares(r, cfg, logger, metrics)

	r.GET("/finalize", func(c *gin.Context) {
		_ = c.Error(NewHTTPError(http.StatusBadRequest, "bad_request", "bad request", nil))
		c.Status(http.StatusOK) // should be finalized to 400
	})

	baseURL, client, stop := startHTTPServerWithEngine(t, r)
	defer stop()

	// Metrics must observe the finalized status code (400), not the handler's temporary 200.
	before400 := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("GET", "/finalize", "400"))
	before200 := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("GET", "/finalize", "200"))

	reqCtx, cancel := context.WithTimeout(context.Background(), itReqTimeout)
	defer cancel()

	req, _ := http.NewRequestWithContext(reqCtx, http.MethodGet, baseURL+"/finalize", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /finalize failed: %v", err)
	}
	drainAndClose(resp.Body)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, resp.StatusCode)
	}

	after400 := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("GET", "/finalize", "400"))
	after200 := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("GET", "/finalize", "200"))

	if after400-before400 != 1 {
		t.Fatalf("expected /finalize 400 counter delta=1, got before=%v after=%v delta=%v", before400, after400, after400-before400)
	}
	if after200-before200 != 0 {
		t.Fatalf("expected /finalize 200 counter delta=0, got before=%v after=%v delta=%v", before200, after200, after200-before200)
	}
}

func TestIntegration_MaxBytes_PayloadTooLarge_Returns413_JSON(t *testing.T) {
	gin.SetMode(gin.TestMode)

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))

	cfg := Config{
		GinMode:           gin.TestMode,
		Host:              "127.0.0.1",
		Port:              0,
		LogLevel:          slog.LevelInfo,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
		ShutdownTimeout:   10 * time.Second,
		MaxBodyBytes:      3,
		ServiceName:       itServiceName,
		Version:           itVersion,
		BuildTime:         itBuildTime,
	}

	m := NewMetrics(cfg)

	r := gin.New()
	r.GET("/__ready", func(c *gin.Context) { c.Status(http.StatusOK) })

	applyMiddlewares(r, cfg, logger, m)

	r.POST("/echo", func(c *gin.Context) {
		_, _ = io.ReadAll(c.Request.Body)
		c.Status(http.StatusNoContent)
	})

	baseURL, client, stop := startHTTPServerWithEngine(t, r)
	defer stop()

	const rid = "it-maxbytes-rid-1"

	reqCtx, cancel := context.WithTimeout(context.Background(), itReqTimeout)
	defer cancel()

	req, _ := http.NewRequestWithContext(reqCtx, http.MethodPost, baseURL+"/echo", strings.NewReader("abcd"))
	req.Header.Set(requestIDHeader, rid)
	req.Header.Set("Content-Type", "text/plain")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /echo failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	drainAndClose(resp.Body)

	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected %d, got %d body=%q", http.StatusRequestEntityTooLarge, resp.StatusCode, string(body))
	}

	j := mustJSONMap(t, body)
	if j["code"] != "payload_too_large" {
		t.Fatalf("expected code=payload_too_large, got %v", j)
	}
	if got, _ := j["request_id"].(string); got != rid {
		t.Fatalf("expected json request_id=%q, got %v", rid, j["request_id"])
	}
}

func TestIntegration_Recovery_Panic_Returns500_JSON(t *testing.T) {
	gin.SetMode(gin.TestMode)

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))

	cfg := Config{
		GinMode:           gin.TestMode,
		Host:              "127.0.0.1",
		Port:              0,
		LogLevel:          slog.LevelInfo,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
		ShutdownTimeout:   10 * time.Second,
		MaxBodyBytes:      1024 * 1024,
		ServiceName:       itServiceName,
		Version:           itVersion,
		BuildTime:         itBuildTime,
	}

	m := NewMetrics(cfg)

	r := gin.New()
	r.GET("/__ready", func(c *gin.Context) { c.Status(http.StatusOK) })

	applyMiddlewares(r, cfg, logger, m)

	r.GET("/panic", func(_ *gin.Context) {
		panic("boom")
	})

	baseURL, client, stop := startHTTPServerWithEngine(t, r)
	defer stop()

	reqCtx, cancel := context.WithTimeout(context.Background(), itReqTimeout)
	defer cancel()

	req, _ := http.NewRequestWithContext(reqCtx, http.MethodGet, baseURL+"/panic", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /panic failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	drainAndClose(resp.Body)

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected %d, got %d body=%q", http.StatusInternalServerError, resp.StatusCode, string(body))
	}

	j := mustJSONMap(t, body)
	if j["code"] != "internal_server_error" {
		t.Fatalf("expected code=internal_server_error, got %v", j)
	}
}

func TestIntegration_ConcurrencyAndGracefulShutdown(t *testing.T) {
	baseURL, client, stop := startHTTPServer(t)

	var (
		okCount  int64
		errCount int64
	)

	loadCtx, cancelLoad := context.WithCancel(context.Background())
	defer cancelLoad()

	const (
		workers      = 20
		testDuration = 250 * time.Millisecond
	)

	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()

			for {
				select {
				case <-loadCtx.Done():
					return
				default:
				}

				reqCtx, cancel := context.WithTimeout(loadCtx, itReqTimeout)
				req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, baseURL+InfoPath, nil)
				if err != nil {
					cancel()
					atomic.AddInt64(&errCount, 1)
					continue
				}

				resp, err := client.Do(req)
				if err != nil {
					cancel()
					atomic.AddInt64(&errCount, 1)
					continue
				}

				drainAndClose(resp.Body)
				cancel()

				if resp.StatusCode == http.StatusOK {
					atomic.AddInt64(&okCount, 1)
				} else {
					atomic.AddInt64(&errCount, 1)
				}
			}
		}()
	}

	time.Sleep(testDuration)

	// Stop clients first; then shutdown. This avoids in-flight requests keeping Shutdown() blocked.
	cancelLoad()
	wg.Wait()
	stop()

	if atomic.LoadInt64(&okCount) == 0 {
		t.Fatalf("expected some successful requests before shutdown; ok=%d err=%d", okCount, errCount)
	}
}

func TestIntegration_ShutdownWaitsForInflightRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	release := make(chan struct{})
	started := make(chan struct{})
	var startedOnce sync.Once

	r := gin.New()
	r.GET("/__ready", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/slow", func(c *gin.Context) {
		startedOnce.Do(func() { close(started) })
		<-release
		c.Status(http.StatusOK)
	})

	baseURL, client, stop := startHTTPServerWithEngine(t, r)

	reqCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, baseURL+"/slow", nil)
		if err != nil {
			errCh <- err
			return
		}

		resp, err := client.Do(req)
		if err != nil {
			errCh <- err
			return
		}
		drainAndClose(resp.Body)

		if resp.StatusCode != http.StatusOK {
			errCh <- context.Canceled
			return
		}
		errCh <- nil
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		stop()
		t.Fatalf("slow handler did not start in time")
	}

	shutdownDone := make(chan struct{})
	go func() {
		stop()
		close(shutdownDone)
	}()

	// Shutdown should not complete until the handler unblocks.
	select {
	case <-shutdownDone:
		t.Fatalf("shutdown finished while a request was still in-flight (expected it to wait)")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)

	if err := <-errCh; err != nil {
		t.Fatalf("slow request failed: %v", err)
	}

	select {
	case <-shutdownDone:
	case <-time.After(1 * time.Second):
		t.Fatalf("shutdown did not finish after in-flight request completed")
	}
}

// withOTelIsolation snapshots global OpenTelemetry state and restores it at test end.
func withOTelIsolation(t *testing.T) {
	t.Helper()

	oldTP := otel.GetTracerProvider()
	oldProp := otel.GetTextMapPropagator()

	t.Cleanup(func() {
		otel.SetTracerProvider(oldTP)
		otel.SetTextMapPropagator(oldProp)
	})
}

// startFakeOTLPHTTPCollector starts a minimal OTLP/HTTP endpoint and counts POSTs to /v1/traces.
// We do not decode protobuf; the test only needs proof that an export was attempted.
func startFakeOTLPHTTPCollector(t *testing.T) (endpoint string, gotRequests *int64, stop func()) {
	t.Helper()

	var count int64

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/traces", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()

		_ = bytes.TrimSpace(body)
		atomic.AddInt64(&count, 1)

		w.WriteHeader(http.StatusOK)
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("collector net.Listen: %v", err)
	}

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	stop = func() {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), itShutdownTimeout)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			_ = srv.Close()
		}
		_ = ln.Close()

		select {
		case err := <-errCh:
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				t.Fatalf("server Serve returned unexpected error: %v", err)
			}
		case <-ctx.Done():
			t.Fatalf("collector did not stop in time: %v", ctx.Err())
		}
	}

	return "http://" + ln.Addr().String() + "/v1/traces", &count, stop
}

func TestIntegration_Telemetry_HTTPRequest_ProducesOTelExport(t *testing.T) {
	withOTelIsolation(t)

	endpoint, reqCount, stopCollector := startFakeOTLPHTTPCollector(t)
	defer stopCollector()

	t.Setenv("OTEL_SDK_DISABLED", "")
	t.Setenv("OTEL_TRACES_EXPORTER", "otlp")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", "http/protobuf")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", endpoint)
	t.Setenv("OTEL_TRACES_SAMPLER", "always_on")
	t.Setenv("OTEL_TRACES_SAMPLER_ARG", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_TIMEOUT", "250")

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))

	shutdown, err := telemetry.InitTracing(context.Background(), logger, itServiceName, itVersion, itBuildTime)
	if err != nil {
		t.Fatalf("InitTracing: %v", err)
	}

	// Build an engine that uses your real middleware chain (includes otelgin).
	gin.SetMode(gin.TestMode)

	cfg := Config{
		GinMode:           gin.TestMode,
		Host:              "127.0.0.1",
		Port:              0,
		LogLevel:          slog.LevelInfo,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
		ShutdownTimeout:   10 * time.Second,
		MaxBodyBytes:      1024 * 1024,
		ServiceName:       itServiceName,
		Version:           itVersion,
		BuildTime:         itBuildTime,
	}
	m := NewMetrics(cfg)

	r := gin.New()
	r.GET("/__ready", func(c *gin.Context) { c.Status(http.StatusOK) })
	applyMiddlewares(r, cfg, logger, m)
	r.GET("/spanme", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	baseURL, client, stop := startHTTPServerWithEngine(t, r)

	reqCtx, cancel := context.WithTimeout(context.Background(), itReqTimeout)
	defer cancel()

	req, _ := http.NewRequestWithContext(reqCtx, http.MethodGet, baseURL+"/spanme", nil)
	resp, err := client.Do(req)
	if err != nil {
		stop()
		_ = shutdown(context.Background())
		t.Fatalf("GET /spanme failed: %v", err)
	}
	drainAndClose(resp.Body)

	stop()

	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("telemetry shutdown: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(reqCount) > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("expected at least one OTLP HTTP export request after an HTTP request, got %d", atomic.LoadInt64(reqCount))
}

func TestIntegration_Telemetry_ExportsToFakeOTLPHTTPCollector(t *testing.T) {
	withOTelIsolation(t)

	endpoint, reqCount, stopCollector := startFakeOTLPHTTPCollector(t)
	defer stopCollector()

	t.Setenv("OTEL_SDK_DISABLED", "")
	t.Setenv("OTEL_TRACES_EXPORTER", "otlp")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", "http/protobuf")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", endpoint)

	t.Setenv("OTEL_TRACES_SAMPLER", "always_on")
	t.Setenv("OTEL_TRACES_SAMPLER_ARG", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_TIMEOUT", "250")

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))

	shutdown, err := telemetry.InitTracing(context.Background(), logger, itServiceName, itVersion, itBuildTime)
	if err != nil {
		t.Fatalf("InitTracing: %v", err)
	}

	_, span := otel.Tracer("integration").Start(context.Background(), "test-span")
	span.End()

	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("telemetry shutdown: %v", err)
	}

	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(reqCount) > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("expected at least one OTLP HTTP export request to /v1/traces, got %d", atomic.LoadInt64(reqCount))
}
