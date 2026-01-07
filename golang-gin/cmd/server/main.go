package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/johnjaysonlpz/docker-polyglot-lab/golang-gin/internal/server"
)

var (
	ServiceName = "golang-gin-app"
	Version     = "0.0.0-dev"
	BuildTime   = "unknown"
)

func main() {
	cfg := server.LoadConfig(ServiceName, Version, BuildTime)
	baseLogger, httpErrLogger := server.NewLogger(cfg.LogLevel)

	logger := baseLogger.With(
		"service", cfg.ServiceName,
		"version", cfg.Version,
		"buildTime", cfg.BuildTime,
	)

	if err := cfg.Validate(); err != nil {
		logger.Error("invalid_config", "error", err)
		os.Exit(1)
	}

	metrics := server.NewMetrics(cfg)
	r := server.SetupRouter(cfg, logger, metrics)

	addr := net.JoinHostPort(cfg.Host, cfg.Port)

	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ErrorLog:          httpErrLogger,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		MaxHeaderBytes:    1 << 20, // 1 MiB
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	srvErrCh := make(chan error, 1)

	go func() {
		logger.Info("starting_server",
			"addr", addr,
			"gin_mode", gin.Mode(),
		)

		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			srvErrCh <- err
		}
	}()

	select {
	case sig := <-quit:
		signal.Stop(quit)
		logger.Info("shutdown_signal_received", "signal", sig.String())

	case err := <-srvErrCh:
		logger.Error("listen_error", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("server_forced_shutdown", "error", err)
		os.Exit(1)
	}

	logger.Info("server_shutdown_complete")
}
