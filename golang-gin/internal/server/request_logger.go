package server

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace"
)

func InjectRequestLogger(base *slog.Logger) gin.HandlerFunc {
	if base == nil {
		base = slog.Default()
	}

	return func(c *gin.Context) {
		l := base

		if rid := c.GetString(requestIDContextKey); rid != "" {
			l = l.With("request_id", rid)
		}

		sc := trace.SpanFromContext(c.Request.Context()).SpanContext()
		if sc.IsValid() {
			l = l.With(
				"trace_id", sc.TraceID().String(),
				"span_id", sc.SpanID().String(),
			)
		}

		c.Set(loggerContextKey, l)
		c.Next()
	}
}

func LoggerFrom(c *gin.Context) *slog.Logger {
	if c == nil {
		return slog.Default()
	}
	if v, ok := c.Get(loggerContextKey); ok {
		if l, ok := v.(*slog.Logger); ok && l != nil {
			return l
		}
	}
	return slog.Default()
}
