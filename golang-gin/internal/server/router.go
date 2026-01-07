package server

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	LivenessPath  = "/health"
	ReadinessPath = "/ready"
	InfoPath      = "/info"
	MetricsPath   = "/metrics"
	RootPath      = "/"
)

func SetupRouter(cfg Config, logger *slog.Logger, metrics *Metrics) *gin.Engine {
	gin.SetMode(cfg.GinMode)

	r := gin.New()

	if len(cfg.TrustedProxies) == 0 {
		_ = r.SetTrustedProxies(nil)
	} else {
		if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
			logger.Error("invalid_trusted_proxies", "error", err, "trusted_proxies", cfg.TrustedProxies)
			_ = r.SetTrustedProxies(nil)
		}
	}

	r.Use(
		RequestIDMiddleware(),
		GinRecoveryWithSlog(logger),
		GinSlogMiddleware(logger, cfg, metrics),
	)

	r.GET(LivenessPath, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	r.GET(ReadinessPath, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	r.GET(MetricsPath, gin.WrapH(promhttp.HandlerFor(metrics.Registry, promhttp.HandlerOpts{})))

	r.GET(InfoPath, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"service":   cfg.ServiceName,
			"version":   cfg.Version,
			"buildTime": cfg.BuildTime,
		})
	})

	r.GET(RootPath, func(c *gin.Context) {
		c.String(http.StatusOK, "golang-gin-app is running (Go + Gin)\n")
	})

	return r
}
