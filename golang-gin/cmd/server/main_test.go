package main

import (
	"context"
	"errors"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/johnjaysonlpz/docker-polyglot-lab/golang-gin/internal/server"
)

/*
NOTE ABOUT PARALLEL TESTS

These tests intentionally DO NOT call t.Parallel().

Reason:
- We override package-level globals (depsProvider, exitFn), which are shared state.
- Parallel tests would race and cause flakiness.
*/

// Sentinel errors used with errors.Is() (more robust than string compares).
var (
	errListen   = errors.New("listen error")
	errShutdown = errors.New("shutdown error")
	errConfig   = errors.New("config error")
	errOtel     = errors.New("otel init error")
)

// newDiscardLoggers creates loggers that write nowhere.
// Useful when we just want to exercise code paths without noisy test output.
func newDiscardLoggers(level slog.Level) (*slog.Logger, *log.Logger) {
	h := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: level})
	return slog.New(h), log.New(io.Discard, "", 0)
}

// baseTestConfig returns a small, fast config suitable for tests.
// The short timeouts ensure tests fail quickly if something unexpectedly blocks.
func baseTestConfig() server.Config {
	return server.Config{
		GinMode:           gin.TestMode,
		Host:              "127.0.0.1",
		Port:              8080,
		LogLevel:          slog.LevelInfo,
		ReadTimeout:       50 * time.Millisecond,
		WriteTimeout:      50 * time.Millisecond,
		ReadHeaderTimeout: 50 * time.Millisecond,
		IdleTimeout:       50 * time.Millisecond,
		ShutdownTimeout:   50 * time.Millisecond,
		MaxBodyBytes:      1024,
		ServiceName:       "svc",
		Version:           "v",
		BuildTime:         "bt",
	}
}

// noopRouter provides a tiny router for app wiring tests.
// We expose a /health route so the engine is not "empty".
func noopRouter(_ server.Config, _ *slog.Logger, _ *server.Metrics) *gin.Engine {
	r := gin.New()
	r.GET("/health", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func noopMetrics(_ server.Config) *server.Metrics { return &server.Metrics{} }

// baseDeps returns a dependency set that makes runWithDeps deterministic and fast.
// Key behavior:
// - notifyContext returns an already-canceled context, so runWithDeps immediately enters shutdown flow.
// - listenAndServe returns http.ErrServerClosed (the "normal" shutdown error).
func baseDeps() appDeps {
	return appDeps{
		loadConfig: func(_, _, _ string) (server.Config, []string, error) {
			return baseTestConfig(), nil, nil
		},
		newLogger:   newDiscardLoggers,
		newMetrics:  noopMetrics,
		setupRouter: noopRouter,
		notifyContext: func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
			ctx, cancel := context.WithCancel(parent)
			// Cancel immediately to simulate "we were told to shut down".
			cancel()
			return ctx, func() {}
		},
		listenAndServe: func(httpServer) error { return http.ErrServerClosed },
		shutdown:       func(httpServer, context.Context) error { return nil },
		initTracing: func(context.Context, *slog.Logger, string, string, string) (func(context.Context) error, error) {
			// Tracing shutdown should be safe to call; here it's a no-op.
			return func(context.Context) error { return nil }, nil
		},
	}
}

// fakeHTTPServer lets us verify that wrapper functions call interface methods
// and properly return their errors.
type fakeHTTPServer struct {
	lsErr       error
	shutdownErr error

	lsCalls int
	sdCalls int
}

func (f *fakeHTTPServer) ListenAndServe() error {
	f.lsCalls++
	return f.lsErr
}

func (f *fakeHTTPServer) Shutdown(context.Context) error {
	f.sdCalls++
	return f.shutdownErr
}

// assertExitCode is a tiny helper for readability in runWithDeps tests.
func assertExitCode(t *testing.T, got, want int) {
	t.Helper()
	if got != want {
		t.Fatalf("expected exit code %d, got %d", want, got)
	}
}

//
// ==========================
// defaultDeps wrappers
// ==========================
//

func TestDefaultDeps_Wrappers_CallInterfaceMethods(t *testing.T) {
	d := defaultDeps()

	fs := &fakeHTTPServer{
		lsErr:       errListen,
		shutdownErr: errShutdown,
	}

	// listenAndServe should call fs.ListenAndServe and return its error.
	if err := d.listenAndServe(fs); !errors.Is(err, errListen) {
		t.Fatalf("expected listen error %q, got %v", errListen, err)
	}

	// shutdown should call fs.Shutdown and return its error.
	if err := d.shutdown(fs, context.Background()); !errors.Is(err, errShutdown) {
		t.Fatalf("expected shutdown error %q, got %v", errShutdown, err)
	}

	if fs.lsCalls != 1 || fs.sdCalls != 1 {
		t.Fatalf("expected lsCalls=1 sdCalls=1, got lsCalls=%d sdCalls=%d", fs.lsCalls, fs.sdCalls)
	}
}

//
// ==========================
// sendErrNonBlocking
// ==========================
//

func TestSendErrNonBlocking_SendsWhenEmpty(t *testing.T) {
	ch := make(chan error, 1)

	sendErrNonBlocking(ch, errors.New("x"))

	select {
	case <-ch:
		// ok
	default:
		t.Fatalf("expected error to be sent")
	}
}

func TestSendErrNonBlocking_DropsWhenFull(t *testing.T) {
	ch := make(chan error, 1)

	// Fill the channel so the non-blocking send should be dropped.
	ch <- errors.New("full")
	sendErrNonBlocking(ch, errors.New("drop"))

	// Ensure the original error wasn't replaced.
	got := <-ch
	if got == nil || got.Error() != "full" {
		t.Fatalf("expected original 'full', got %v", got)
	}
}

//
// ==========================
// runWithDeps
// ==========================
//

func TestRun_ConfigError_Returns1(t *testing.T) {
	deps := baseDeps()
	deps.loadConfig = func(_, _, _ string) (server.Config, []string, error) {
		return baseTestConfig(), []string{"warn-1"}, errConfig
	}

	assertExitCode(t, runWithDeps(deps), 1)
}

func TestRun_OtelInitError_Returns1(t *testing.T) {
	deps := baseDeps()
	deps.initTracing = func(context.Context, *slog.Logger, string, string, string) (func(context.Context) error, error) {
		return nil, errOtel
	}

	assertExitCode(t, runWithDeps(deps), 1)
}

func TestRun_ListenError_Returns1(t *testing.T) {
	deps := baseDeps()

	// Make notifyContext NOT canceled; run should fail because listenAndServe fails.
	deps.notifyContext = func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
		return context.WithCancel(parent)
	}
	deps.listenAndServe = func(httpServer) error { return errListen }

	assertExitCode(t, runWithDeps(deps), 1)
}

func TestRun_ShutdownError_Returns1_AndCallsOtelShutdown(t *testing.T) {
	deps := baseDeps()

	otelShutdownCalls := 0
	deps.initTracing = func(context.Context, *slog.Logger, string, string, string) (func(context.Context) error, error) {
		return func(context.Context) error {
			otelShutdownCalls++
			return nil
		}, nil
	}
	deps.shutdown = func(httpServer, context.Context) error { return errShutdown }

	assertExitCode(t, runWithDeps(deps), 1)

	if otelShutdownCalls != 1 {
		t.Fatalf("expected otel shutdown called once, got %d", otelShutdownCalls)
	}
}

func TestRun_GracefulShutdown_Returns0_AndCallsOtelShutdown(t *testing.T) {
	deps := baseDeps()

	otelShutdownCalls := 0
	deps.initTracing = func(context.Context, *slog.Logger, string, string, string) (func(context.Context) error, error) {
		return func(context.Context) error {
			otelShutdownCalls++
			return nil
		}, nil
	}

	assertExitCode(t, runWithDeps(deps), 0)

	if otelShutdownCalls != 1 {
		t.Fatalf("expected otel shutdown called once, got %d", otelShutdownCalls)
	}
}

//
// ==========================
// run() wrapper & main()
// ==========================
//

func TestRun_Wrapper_UsesDepsProvider(t *testing.T) {
	old := depsProvider
	t.Cleanup(func() { depsProvider = old })

	depsProvider = func() appDeps {
		d := baseDeps()
		d.loadConfig = func(_, _, _ string) (server.Config, []string, error) {
			return baseTestConfig(), nil, errConfig
		}
		return d
	}

	assertExitCode(t, run(), 1)
}

func TestMain_UsesExitFn(t *testing.T) {
	oldExit := exitFn
	oldDeps := depsProvider
	t.Cleanup(func() {
		exitFn = oldExit
		depsProvider = oldDeps
	})

	depsProvider = func() appDeps {
		d := baseDeps()
		d.loadConfig = func(_, _, _ string) (server.Config, []string, error) {
			return baseTestConfig(), nil, errConfig
		}
		return d
	}

	gotCode := -1
	exitFn = func(code int) { gotCode = code }

	main()

	if gotCode != 1 {
		t.Fatalf("expected exit code 1, got %d", gotCode)
	}
}
