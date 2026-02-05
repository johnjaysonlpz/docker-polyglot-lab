package server

import (
	"log/slog"

	"github.com/gin-gonic/gin"
)

func SetupRouter(cfg Config, logger *slog.Logger, metrics *Metrics) *gin.Engine {
	gin.SetMode(cfg.GinMode)

	r := gin.New()
	r.HandleMethodNotAllowed = true

	applyTrustedProxies(r, cfg, logger)
	applyMiddlewares(r, cfg, logger, metrics)
	registerRoutes(r, cfg, metrics)

	r.NoRoute(NotFoundHandler)
	r.NoMethod(MethodNotAllowedHandler)

	return r
}

func applyTrustedProxies(r *gin.Engine, cfg Config, logger *slog.Logger) {
	if len(cfg.TrustedProxies) == 0 {
		_ = r.SetTrustedProxies(nil)
		return
	}

	if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		if logger == nil {
			logger = slog.Default()
		}
		logger.Error("invalid_trusted_proxies", "error", err, "trusted_proxies", cfg.TrustedProxies)
		_ = r.SetTrustedProxies(nil)
	}
}
