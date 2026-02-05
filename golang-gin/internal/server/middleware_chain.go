package server

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

func applyMiddlewares(r *gin.Engine, cfg Config, logger *slog.Logger, metrics *Metrics) {
	r.Use(
		GinRecoveryWithSlog(),

		otelgin.Middleware(cfg.ServiceName),

		RequestIDMiddleware(),
		RequestIDSpanAttrMiddleware(),

		InjectRequestLogger(logger),

		GinSlogMiddleware(logger, cfg, metrics),

		ErrorFinalizer(),

		MaxBodyBytesMiddleware(cfg.MaxBodyBytes),
		MaxBytesErrorMiddleware(),
	)
}
