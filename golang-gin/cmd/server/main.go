package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/gin-gonic/gin"

	"github.com/johnjaysonlpz/docker-polyglot-lab/golang-gin/internal/server"
	"github.com/johnjaysonlpz/docker-polyglot-lab/golang-gin/internal/telemetry"
)

var (
	ServiceName = "golang-gin-app"
	Version     = "0.0.0-dev"
	BuildTime   = "unknown"

	exitFn       = os.Exit
	depsProvider = defaultDeps
)

func main() {
	exitFn(run())
}

type httpServer interface {
	ListenAndServe() error
	Shutdown(context.Context) error
}

type appDeps struct {
	loadConfig     func(serviceName, version, buildTime string) (server.Config, []string, error)
	newLogger      func(level slog.Level) (*slog.Logger, *log.Logger)
	initTracing    func(ctx context.Context, logger *slog.Logger, serviceName, version, buildTime string) (func(context.Context) error, error)
	newMetrics     func(cfg server.Config) *server.Metrics
	setupRouter    func(cfg server.Config, logger *slog.Logger, metrics *server.Metrics) *gin.Engine
	notifyContext  func(parent context.Context, signals ...os.Signal) (context.Context, context.CancelFunc)
	listenAndServe func(srv httpServer) error
	shutdown       func(srv httpServer, ctx context.Context) error
}

func defaultDeps() appDeps {
	return appDeps{
		loadConfig:    server.LoadConfig,
		newLogger:     server.NewLogger,
		initTracing:   telemetry.InitTracing,
		newMetrics:    server.NewMetrics,
		setupRouter:   server.SetupRouter,
		notifyContext: signal.NotifyContext,
		listenAndServe: func(srv httpServer) error {
			return srv.ListenAndServe()
		},
		shutdown: func(srv httpServer, ctx context.Context) error {
			return srv.Shutdown(ctx)
		},
	}
}

func run() int {
	return runWithDeps(depsProvider())
}

func sendErrNonBlocking(ch chan<- error, err error) {
	select {
	case ch <- err:
	default:
	}
}

func runWithDeps(deps appDeps) int {
	cfg, warns, cfgErr := deps.loadConfig(ServiceName, Version, BuildTime)

	baseLogger, httpErrLogger := deps.newLogger(cfg.LogLevel)
	logger := baseLogger.With(
		"service", cfg.ServiceName,
		"version", cfg.Version,
		"build_time", cfg.BuildTime,
	)

	for _, w := range warns {
		logger.Warn("config_warning", "warning", w)
	}

	if cfgErr != nil {
		logger.Error("invalid_config", "error", cfgErr)
		return 1
	}

	otelShutdown, err := deps.initTracing(context.Background(), logger, cfg.ServiceName, cfg.Version, cfg.BuildTime)
	if err != nil {
		logger.Error("otel_init_failed", "error", err)
		return 1
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		_ = otelShutdown(ctx)
	}()

	metrics := deps.newMetrics(cfg)
	r := deps.setupRouter(cfg, logger, metrics)

	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))

	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ErrorLog:          httpErrLogger,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		MaxHeaderBytes:    1 << 20,
	}

	sigCtx, stop := deps.notifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	srvErrCh := make(chan error, 1)

	go func() {
		logger.Info("starting_server", "addr", addr, "gin_mode", gin.Mode())

		if err := deps.listenAndServe(srv); err != nil && !errors.Is(err, http.ErrServerClosed) {
			sendErrNonBlocking(srvErrCh, err)
		}
	}()

	select {
	case <-sigCtx.Done():
		logger.Info("shutdown_signal_received")
	case err := <-srvErrCh:
		logger.Error("listen_error", "error", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := deps.shutdown(srv, ctx); err != nil {
		logger.Error("server_forced_shutdown", "error", err)
		return 1
	}

	logger.Info("server_shutdown_complete")
	return 0
}
