package telemetry

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

/*
NOTE ABOUT PARALLEL TESTS

These tests intentionally DO NOT call t.Parallel().

Reason:
- We mutate package-level variables (newOTLPHTTPExporter, newOTLPGRPCExporter, grpcEndpointResolver).
- We also mutate global OpenTelemetry state (otel.SetTracerProvider, otel.SetTextMapPropagator).

Running these tests in parallel would cause data races and flaky failures.
*/

const (
	testServiceName = "svc"
	testVersion     = "v"
	testBuildTime   = "bt"
)

// Sentinel errors used with errors.Is() for robust assertions.
var (
	errEndpointResolution = errors.New("endpoint resolution failed")
	errHTTPExporter       = errors.New("http exporter failed")
	errGRPCExporter       = errors.New("grpc exporter failed")
)

//
// ==========================
// Test helpers / fixtures
// ==========================
//

// captureHandler collects slog records for assertions.
// We copy records because slog may reuse its internal record object.
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

func (h *captureHandler) LastMessage() (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.records) == 0 {
		return "", false
	}
	return h.records[len(h.records)-1].Message, true
}

func newCapturedLogger() (*slog.Logger, *captureHandler) {
	h := &captureHandler{}
	return slog.New(h), h
}

// newBufLogger is useful when you only care that "something was logged".
func newBufLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(h), &buf
}

// fakeExporter is a no-op SpanExporter used to prevent network calls during wiring tests.
type fakeExporter struct{}

func (fakeExporter) ExportSpans(context.Context, []sdktrace.ReadOnlySpan) error { return nil }
func (fakeExporter) Shutdown(context.Context) error                             { return nil }

// nopExporter tracks shutdown calls for assertions.
type nopExporter struct {
	shutdownCalls int
}

func (*nopExporter) ExportSpans(context.Context, []sdktrace.ReadOnlySpan) error { return nil }
func (n *nopExporter) Shutdown(context.Context) error {
	n.shutdownCalls++
	return nil
}

// savedOTelState snapshots global OTel state so tests don't leak configuration.
type savedOTelState struct {
	tp   trace.TracerProvider
	prop propagation.TextMapPropagator
}

func saveOTelState() savedOTelState {
	return savedOTelState{
		tp:   otel.GetTracerProvider(),
		prop: otel.GetTextMapPropagator(),
	}
}

func restoreOTelState(s savedOTelState) {
	otel.SetTracerProvider(s.tp)
	otel.SetTextMapPropagator(s.prop)
}

// withOTelIsolation ensures global OTel state is restored after the test.
func withOTelIsolation(t *testing.T) {
	t.Helper()
	old := saveOTelState()
	t.Cleanup(func() { restoreOTelState(old) })
}

// withFactories restores package-level factories/resolvers after the test.
func withFactories(t *testing.T) {
	t.Helper()

	oldHTTP := newOTLPHTTPExporter
	oldGRPC := newOTLPGRPCExporter
	oldResolver := grpcEndpointResolver

	t.Cleanup(func() {
		newOTLPHTTPExporter = oldHTTP
		newOTLPGRPCExporter = oldGRPC
		grpcEndpointResolver = oldResolver
	})
}

// mustInit calls InitTracing and fails the test on error.
// This keeps "happy path" tests short and focused.
func mustInit(ctx context.Context, t *testing.T, logger *slog.Logger) func(context.Context) error {
	t.Helper()

	shutdown, err := InitTracing(ctx, logger, testServiceName, testVersion, testBuildTime)
	if err != nil {
		t.Fatalf("InitTracing() unexpected error: %v", err)
	}
	if shutdown == nil {
		t.Fatalf("InitTracing() returned nil shutdown")
	}
	return shutdown
}

//
// ==========================
// Environment helpers
// ==========================
//

func setSDKEnabled(t *testing.T) {
	t.Helper()
	t.Setenv("OTEL_SDK_DISABLED", "")
}

func setExporterOTLP(t *testing.T) {
	t.Helper()
	t.Setenv("OTEL_TRACES_EXPORTER", "otlp")
}

func setProtocol(t *testing.T, proto string) {
	t.Helper()
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", proto)
}

func clearOTLPEndpoints(t *testing.T) {
	t.Helper()
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
}

// setOTLPHTTP configures exporter=otlp and protocol=http/protobuf.
// If endpoint is empty, the code will fall back to base/default endpoint logic.
func setOTLPHTTP(t *testing.T, endpoint string) {
	t.Helper()
	setExporterOTLP(t)
	setProtocol(t, "http/protobuf")
	if endpoint != "" {
		t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", endpoint)
	}
}

// setOTLPGRPC configures exporter=otlp and protocol=grpc.
// If endpoint is empty, the code will fall back to base/default endpoint logic.
func setOTLPGRPC(t *testing.T, endpoint string) {
	t.Helper()
	setExporterOTLP(t)
	setProtocol(t, "grpc")
	if endpoint != "" {
		t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", endpoint)
	}
}

//
// ==========================
// InitTracing behavior
// ==========================
//

func TestInitTracing_SDKDisabled_UsesNoopProviderAndLogs(t *testing.T) {
	t.Setenv("OTEL_SDK_DISABLED", "true")
	withOTelIsolation(t)

	logger, h := newCapturedLogger()
	shutdown := mustInit(context.Background(), t, logger)

	// noop.NewTracerProvider() yields a concrete noop provider.
	switch otel.GetTracerProvider().(type) {
	case noop.TracerProvider, *noop.TracerProvider:
		// ok
	default:
		t.Fatalf("expected noop tracer provider, got %T", otel.GetTracerProvider())
	}

	// Even disabled mode sets propagators, so instrumentation still works.
	if otel.GetTextMapPropagator() == nil {
		t.Fatalf("expected propagator to be set")
	}

	if h.Count() == 0 {
		t.Fatalf("expected a log record")
	}
	if msg, _ := h.LastMessage(); msg != "otel_sdk_disabled" {
		t.Fatalf("expected log message %q, got %q", "otel_sdk_disabled", msg)
	}

	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown unexpected error: %v", err)
	}
}

func TestInitTracing_UnsupportedExporterEnv_ReturnsError(t *testing.T) {
	setSDKEnabled(t)
	t.Setenv("OTEL_TRACES_EXPORTER", "zipkin") // unsupported by this implementation
	withOTelIsolation(t)

	if _, err := InitTracing(context.Background(), nil, testServiceName, testVersion, testBuildTime); err == nil {
		t.Fatalf("expected error")
	}
}

func TestInitTracing_NoExporter_Path_LogsAndReturnsShutdown(t *testing.T) {
	setSDKEnabled(t)
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	t.Setenv("OTEL_TRACES_SAMPLER", "") // exporter=none defaults sampler to always_off
	withOTelIsolation(t)

	logger, h := newCapturedLogger()
	shutdown := mustInit(context.Background(), t, logger)

	// exporter=none should still install a real SDK provider (not noop).
	if _, ok := otel.GetTracerProvider().(*noop.TracerProvider); ok {
		t.Fatalf("expected non-noop provider when exporter=none")
	}

	if msg, _ := h.LastMessage(); msg != "otel_traces_exporter_disabled" {
		t.Fatalf("expected log message %q, got %q", "otel_traces_exporter_disabled", msg)
	}

	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown unexpected error: %v", err)
	}
}

func TestInitTracing_OTLP_HTTP_Success_LogsWhenLoggerProvided(t *testing.T) {
	setSDKEnabled(t)
	setOTLPHTTP(t, "http://example.invalid:4318/v1/traces")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_TIMEOUT", "150ms")

	withOTelIsolation(t)
	withFactories(t)

	newOTLPHTTPExporter = func(context.Context, string, time.Duration) (sdktrace.SpanExporter, error) {
		return fakeExporter{}, nil
	}

	logger, h := newCapturedLogger()
	shutdown := mustInit(context.Background(), t, logger)

	if msg, _ := h.LastMessage(); msg != "otel_tracing_enabled" {
		t.Fatalf("expected log message %q, got %q", "otel_tracing_enabled", msg)
	}

	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown unexpected error: %v", err)
	}
}

func TestInitTracing_OTLP_HTTP_Success_NoLogger_NoPanic_ShutdownWorks(t *testing.T) {
	setSDKEnabled(t)
	setOTLPHTTP(t, "http://example.invalid:4318/v1/traces")

	withOTelIsolation(t)
	withFactories(t)

	exp := &nopExporter{}
	newOTLPHTTPExporter = func(context.Context, string, time.Duration) (sdktrace.SpanExporter, error) {
		return exp, nil
	}

	shutdown := mustInit(context.Background(), t, nil)

	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown unexpected error: %v", err)
	}
	if exp.shutdownCalls == 0 {
		t.Fatalf("expected exporter shutdown to be called")
	}
}

func TestInitTracing_OTLP_HTTP_EndpointURLParseError_BubblesUp(t *testing.T) {
	clearOTLPEndpoints(t)
	setSDKEnabled(t)
	setOTLPHTTP(t, "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://[::1") // invalid URL

	withOTelIsolation(t)
	withFactories(t)

	newOTLPHTTPExporter = func(context.Context, string, time.Duration) (sdktrace.SpanExporter, error) {
		t.Fatalf("exporter factory should not be called when endpoint URL parsing fails")
		return nil, nil
	}

	if _, err := InitTracing(context.Background(), nil, testServiceName, testVersion, testBuildTime); err == nil {
		t.Fatalf("expected error")
	}
}

func TestInitTracing_OTLP_HTTP_ExporterFactoryError_BubblesUp(t *testing.T) {
	setSDKEnabled(t)
	setOTLPHTTP(t, "http://example.invalid:4318/v1/traces")

	withOTelIsolation(t)
	withFactories(t)

	newOTLPHTTPExporter = func(context.Context, string, time.Duration) (sdktrace.SpanExporter, error) {
		return nil, errHTTPExporter
	}

	if _, err := InitTracing(context.Background(), nil, testServiceName, testVersion, testBuildTime); err == nil {
		t.Fatalf("expected error")
	}
}

func TestInitTracing_OTLP_GRPC_Success_ParsesInsecureFromURL_Logs(t *testing.T) {
	setSDKEnabled(t)
	setOTLPGRPC(t, "http://collector.invalid:4317") // http => insecure
	t.Setenv("OTEL_EXPORTER_OTLP_TIMEOUT", "200")   // integer => milliseconds

	withOTelIsolation(t)
	withFactories(t)

	newOTLPGRPCExporter = func(context.Context, string, time.Duration, bool) (sdktrace.SpanExporter, error) {
		return fakeExporter{}, nil
	}

	logger, h := newCapturedLogger()
	shutdown := mustInit(context.Background(), t, logger)

	if msg, _ := h.LastMessage(); msg != "otel_tracing_enabled" {
		t.Fatalf("expected log message %q, got %q", "otel_tracing_enabled", msg)
	}

	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown unexpected error: %v", err)
	}
}

func TestInitTracing_OTLP_GRPC_Success_NoLogger_CoversNoLogBranch(t *testing.T) {
	setSDKEnabled(t)
	setOTLPGRPC(t, "collector.invalid:4317") // raw host branch

	withOTelIsolation(t)
	withFactories(t)

	newOTLPGRPCExporter = func(context.Context, string, time.Duration, bool) (sdktrace.SpanExporter, error) {
		return fakeExporter{}, nil
	}

	shutdown := mustInit(context.Background(), t, nil)
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown unexpected error: %v", err)
	}
}

func TestInitTracing_OTLP_GRPC_ProtoVariant_UsesGRPCPath_AndLogs(t *testing.T) {
	clearOTLPEndpoints(t)
	setSDKEnabled(t)
	setExporterOTLP(t)
	setProtocol(t, "grpc/protobuf")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "https://collector.example:4317") // https => insecure=false

	withOTelIsolation(t)
	withFactories(t)

	exp := &nopExporter{}
	var gotInsecure bool
	newOTLPGRPCExporter = func(_ context.Context, _ string, _ time.Duration, insecure bool) (sdktrace.SpanExporter, error) {
		gotInsecure = insecure
		return exp, nil
	}

	logger, buf := newBufLogger()
	shutdown := mustInit(context.Background(), t, logger)

	if gotInsecure {
		t.Fatalf("expected insecure=false for https endpoint")
	}
	if buf.Len() == 0 {
		t.Fatalf("expected logs, got empty buffer")
	}

	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown unexpected error: %v", err)
	}
}

func TestInitTracing_OTLP_GRPC_EndpointResolverError_BubblesUp(t *testing.T) {
	setSDKEnabled(t)
	setOTLPGRPC(t, "")

	withOTelIsolation(t)
	withFactories(t)

	newOTLPGRPCExporter = func(context.Context, string, time.Duration, bool) (sdktrace.SpanExporter, error) {
		t.Fatalf("newOTLPGRPCExporter should not be called when endpoint resolver fails")
		return nil, nil
	}

	grpcEndpointResolver = func() (string, bool, error) {
		return "", false, errEndpointResolution
	}

	_, err := InitTracing(context.Background(), nil, testServiceName, testVersion, testBuildTime)
	if !errors.Is(err, errEndpointResolution) {
		t.Fatalf("expected resolver error %q, got %v", errEndpointResolution, err)
	}
}

func TestInitTracing_OTLP_GRPC_ExporterFactoryError_BubblesUp(t *testing.T) {
	setSDKEnabled(t)
	setOTLPGRPC(t, "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "collector.invalid:4317")

	withOTelIsolation(t)
	withFactories(t)

	newOTLPGRPCExporter = func(context.Context, string, time.Duration, bool) (sdktrace.SpanExporter, error) {
		return nil, errGRPCExporter
	}

	if _, err := InitTracing(context.Background(), nil, testServiceName, testVersion, testBuildTime); err == nil {
		t.Fatalf("expected error")
	}
}

func TestInitTracing_UnsupportedProtocol_ReturnsError(t *testing.T) {
	setSDKEnabled(t)
	setExporterOTLP(t)
	setProtocol(t, "something-weird")

	withOTelIsolation(t)

	if _, err := InitTracing(context.Background(), nil, testServiceName, testVersion, testBuildTime); err == nil {
		t.Fatalf("expected error")
	}
}

func TestInitTracing_UnsupportedSampler_ReturnsError(t *testing.T) {
	setSDKEnabled(t)
	setExporterOTLP(t)
	t.Setenv("OTEL_TRACES_SAMPLER", "definitely_not_supported")
	t.Setenv("OTEL_TRACES_SAMPLER_ARG", "")

	withOTelIsolation(t)

	if _, err := InitTracing(context.Background(), nil, testServiceName, testVersion, testBuildTime); err == nil {
		t.Fatalf("expected error")
	}
}

func TestInitTracing_BuildResourceError_BubblesUp(t *testing.T) {
	// OTEL_RESOURCE_ATTRIBUTES expects "k=v" pairs; malformed values should fail.
	setSDKEnabled(t)
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "definitely-not-kv")

	withOTelIsolation(t)

	if _, err := InitTracing(context.Background(), nil, testServiceName, testVersion, testBuildTime); err == nil {
		t.Fatalf("expected error")
	}
}

func TestInitTracing_ExporterFactoryNotCalled_WhenSamplerFails(t *testing.T) {
	// Regression test: sampler validation should happen before exporter construction.
	setSDKEnabled(t)
	setOTLPGRPC(t, "")
	t.Setenv("OTEL_TRACES_SAMPLER", "nope")

	withOTelIsolation(t)
	withFactories(t)

	called := false
	newOTLPGRPCExporter = func(context.Context, string, time.Duration, bool) (sdktrace.SpanExporter, error) {
		called = true
		return nil, errors.New("should not be called")
	}

	if _, err := InitTracing(context.Background(), nil, testServiceName, testVersion, testBuildTime); err == nil {
		t.Fatalf("expected error")
	}
	if called {
		t.Fatalf("expected exporter factory not to be called when sampler is invalid")
	}
}

//
// ==========================
// Unit tests for small helpers
// ==========================
//

func TestResolveServiceName_EnvOverridesDefault(t *testing.T) {
	t.Setenv("OTEL_SERVICE_NAME", "  my-service  ")
	if got := resolveServiceName("default"); got != "my-service" {
		t.Fatalf("got %q", got)
	}

	t.Setenv("OTEL_SERVICE_NAME", "   ")
	if got := resolveServiceName("default"); got != "default" {
		t.Fatalf("got %q", got)
	}
}

func TestBuildResource_ReturnsResource(t *testing.T) {
	res, err := buildResource(context.Background(), testServiceName, testVersion, testBuildTime)
	if err != nil || res == nil {
		t.Fatalf("expected resource, got res=%v err=%v", res, err)
	}
}

func TestReadSamplerEnv_DefaultsToAlwaysOffWhenExporterNone(t *testing.T) {
	t.Setenv("OTEL_TRACES_SAMPLER", "")
	t.Setenv("OTEL_TRACES_SAMPLER_ARG", "")

	name, arg := readSamplerEnv(exporterNone)
	if name != "parentbased_always_off" || arg != "" {
		t.Fatalf("got name=%q arg=%q", name, arg)
	}
}

func TestSamplerNameOrDefault(t *testing.T) {
	if got := samplerNameOrDefault(" "); got != "parentbased_always_on" {
		t.Fatalf("got %q", got)
	}
	if got := samplerNameOrDefault("always_off"); got != "always_off" {
		t.Fatalf("got %q", got)
	}
}

func TestSamplerFromEnv_Cases(t *testing.T) {
	cases := []struct {
		name string
		arg  string
		ok   bool
	}{
		{"", "", true},
		{"parentbased_always_on", "", true},
		{"always_on", "", true},
		{"parentbased_always_off", "", true},
		{"always_off", "", true},
		{"traceidratio", "0.5", true},
		{"parentbased_traceidratio", "0.25", true},
		{"parentbased_traceidratio", "", false}, // parseRatio error branch
		{"traceidratio", "", false},
		{"traceidratio", "2", false},
		{"traceidratio", "nope", false},
		{"unknown", "", false},
		{"  TRACEIDRATIO  ", "0.5", true}, // normalization coverage
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name+"("+tc.arg+")", func(t *testing.T) {
			s, err := samplerFromEnv(tc.name, tc.arg)
			if tc.ok {
				if err != nil || s == nil {
					t.Fatalf("expected ok, got sampler=%v err=%v", s, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}

func TestParseRatio(t *testing.T) {
	if _, err := parseRatio(""); err == nil {
		t.Fatalf("expected error for empty")
	}
	if _, err := parseRatio("nope"); err == nil {
		t.Fatalf("expected error for invalid float")
	}
	if _, err := parseRatio("-0.1"); err == nil {
		t.Fatalf("expected error for <0")
	}
	if _, err := parseRatio("1.1"); err == nil {
		t.Fatalf("expected error for >1")
	}
	if got, err := parseRatio("0.75"); err != nil || got != 0.75 {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

func TestResolveOTLPProtocol_DefaultAndPrecedence(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", "")
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "")
	if got := resolveOTLPProtocol(); got != "http/protobuf" {
		t.Fatalf("got %q", got)
	}

	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "grpc")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", "")
	if got := resolveOTLPProtocol(); got != "grpc" {
		t.Fatalf("got %q", got)
	}

	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", "http/protobuf")
	if got := resolveOTLPProtocol(); got != "http/protobuf" {
		t.Fatalf("got %q", got)
	}
}

func TestSetDefaultPropagators_SetsNonNilPropagator(t *testing.T) {
	withOTelIsolation(t)

	setDefaultPropagators()
	if otel.GetTextMapPropagator() == nil {
		t.Fatalf("expected propagator set")
	}
}

func TestTracesExporterFromEnv(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "")
	if got, err := tracesExporterFromEnv(); err != nil || got != exporterOTLP {
		t.Fatalf("got=%q err=%v", got, err)
	}

	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	if got, err := tracesExporterFromEnv(); err != nil || got != exporterNone {
		t.Fatalf("got=%q err=%v", got, err)
	}

	t.Setenv("OTEL_TRACES_EXPORTER", "none, otlp")
	if got, err := tracesExporterFromEnv(); err != nil || got != exporterOTLP {
		t.Fatalf("got=%q err=%v", got, err)
	}

	t.Setenv("OTEL_TRACES_EXPORTER", "zipkin, jaeger")
	if _, err := tracesExporterFromEnv(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestParseOTLPTimeout_AllPaths(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_TIMEOUT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TIMEOUT", "")
	if got := parseOTLPTimeout(); got != 10*time.Second {
		t.Fatalf("got %s", got)
	}

	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_TIMEOUT", "250ms")
	if got := parseOTLPTimeout(); got != 250*time.Millisecond {
		t.Fatalf("got %s", got)
	}

	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_TIMEOUT", "0")
	if got := parseOTLPTimeout(); got != 10*time.Second {
		t.Fatalf("got %s", got)
	}

	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_TIMEOUT", "0s")
	if got := parseOTLPTimeout(); got != 10*time.Second {
		t.Fatalf("got %s", got)
	}

	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_TIMEOUT", "150")
	if got := parseOTLPTimeout(); got != 150*time.Millisecond {
		t.Fatalf("got %s", got)
	}

	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_TIMEOUT", "nope")
	if got := parseOTLPTimeout(); got != 10*time.Second {
		t.Fatalf("got %s", got)
	}
}

func TestHTTPTracesEndpointURL_AllPaths(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "http://x/v1/traces")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://base/")
	if got, err := httpTracesEndpointURL(); err != nil || got != "http://x/v1/traces" {
		t.Fatalf("got=%q err=%v", got, err)
	}

	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:4318")
	got, err := httpTracesEndpointURL()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "http://127.0.0.1:4318/v1/traces" {
		t.Fatalf("got %q", got)
	}

	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:4318/otel")
	got, err = httpTracesEndpointURL()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "http://127.0.0.1:4318/otel/v1/traces" {
		t.Fatalf("got %q", got)
	}

	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://[::1") // invalid
	if _, err := httpTracesEndpointURL(); err == nil {
		t.Fatalf("expected error")
	}

	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	if got, err := httpTracesEndpointURL(); err != nil || got != "http://127.0.0.1:4318/v1/traces" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestGRPCTracesEndpointAndInsecure_AllPaths(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	ep, insecure, err := grpcTracesEndpointAndInsecure()
	if err != nil || ep != "127.0.0.1:4317" || insecure != true {
		t.Fatalf("got ep=%q insecure=%v err=%v", ep, insecure, err)
	}

	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "https://collector.example:4317")
	ep, insecure, err = grpcTracesEndpointAndInsecure()
	if err != nil || ep != "collector.example:4317" || insecure != false {
		t.Fatalf("got ep=%q insecure=%v err=%v", ep, insecure, err)
	}

	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "http://collector.example:4317")
	ep, insecure, err = grpcTracesEndpointAndInsecure()
	if err != nil || ep != "collector.example:4317" || insecure != true {
		t.Fatalf("got ep=%q insecure=%v err=%v", ep, insecure, err)
	}

	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "collector.example:4317")
	ep, insecure, err = grpcTracesEndpointAndInsecure()
	if err != nil || ep != "collector.example:4317" || insecure != true {
		t.Fatalf("got ep=%q insecure=%v err=%v", ep, insecure, err)
	}
}

func TestEnvTrue(t *testing.T) {
	t.Setenv("X", "true")
	if !envTrue("X") {
		t.Fatalf("expected true")
	}

	t.Setenv("X", " TRUE ")
	if !envTrue("X") {
		t.Fatalf("expected true")
	}

	t.Setenv("X", "false")
	if envTrue("X") {
		t.Fatalf("expected false")
	}

	t.Setenv("X", "")
	if envTrue("X") {
		t.Fatalf("expected false")
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty(" ", "\t", "a", "b"); got != "a" {
		t.Fatalf("got %q", got)
	}
	if got := firstNonEmpty(" ", ""); got != "" {
		t.Fatalf("got %q", got)
	}
}

//
// ==========================
// Coverage-only: default exporter factory literals
// ==========================
//
// These anonymous function literals are assigned at package init time in tracing.go.
// They count toward statement coverage but don't show up in `go tool cover -func`.
//

var (
	defaultHTTPExporterFactory = newOTLPHTTPExporter
	defaultGRPCExporterFactory = newOTLPGRPCExporter
)

func TestDefaultExporterFactories_AreCovered(_ *testing.T) {
	// Use Background() for stability; keep timeout small; ignore errors.
	{
		exp, _ := defaultHTTPExporterFactory(
			context.Background(),
			"http://127.0.0.1:4318/v1/traces",
			time.Millisecond,
		)
		if exp != nil {
			_ = exp.Shutdown(context.Background())
		}
	}

	{
		exp, _ := defaultGRPCExporterFactory(
			context.Background(),
			"127.0.0.1:4317",
			time.Millisecond,
			false,
		)
		if exp != nil {
			_ = exp.Shutdown(context.Background())
		}
	}

	{
		exp, _ := defaultGRPCExporterFactory(
			context.Background(),
			"127.0.0.1:4317",
			time.Millisecond,
			true,
		)
		if exp != nil {
			_ = exp.Shutdown(context.Background())
		}
	}
}
