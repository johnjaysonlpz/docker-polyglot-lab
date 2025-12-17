package server

import (
	"log/slog"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

var skipLogRoutes = map[string]struct{}{
	LivenessPath:  {}, // "/health"
	ReadinessPath: {}, // "/ready"
	MetricsPath:   {}, // "/metrics"
}

func GinSlogMiddleware(l *slog.Logger, cfg Config, m *Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		rawPath := c.Request.URL.Path

		c.Next()

		latency := time.Since(start)
		statusCode := c.Writer.Status()

		routePath := c.FullPath()
		if routePath == "" {
			routePath = rawPath
		}

		method := c.Request.Method
		status := strconv.Itoa(statusCode)

		latencySeconds := float64(latency) / float64(time.Second)

		m.HTTPRequestsTotal.WithLabelValues(cfg.ServiceName, method, routePath, status).Inc()
		m.HTTPRequestDurationSeconds.WithLabelValues(cfg.ServiceName, method, routePath, status).Observe(latencySeconds)

		if _, ok := skipLogRoutes[routePath]; ok {
			return
		}

		attrs := []any{
			"status", statusCode,
			"method", method,
			"path", routePath,
			"rawPath", rawPath,
			"query", c.Request.URL.RawQuery,
			"ip", c.ClientIP(),
			"latency", latency.String(),
			"userAgent", c.Request.UserAgent(),
		}

		if len(c.Errors) > 0 {
			errs := make([]string, len(c.Errors))
			for i, e := range c.Errors {
				errs[i] = e.Error()
			}
			attrs = append(attrs, "errors", errs)
			l.Error("http_request", attrs...)
		} else {
			l.Info("http_request", attrs...)
		}
	}
}
