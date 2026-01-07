package server

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	requestIDHeader     = "X-Request-ID"
	requestIDContextKey = "request_id"
	unmatchedRouteLabel = "__unmatched__"
)

var skipLogPaths = map[string]struct{}{
	LivenessPath:  {}, // "/health"
	ReadinessPath: {}, // "/ready"
	MetricsPath:   {}, // "/metrics"
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b[:])
}

func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader(requestIDHeader)
		if rid == "" {
			rid = newRequestID()
		}
		c.Set(requestIDContextKey, rid)
		c.Writer.Header().Set(requestIDHeader, rid)
		c.Next()
	}
}

func GinSlogMiddleware(l *slog.Logger, cfg Config, m *Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		rawPath := c.Request.URL.Path

		c.Next()

		latency := time.Since(start)
		statusCode := c.Writer.Status()
		routePath := c.FullPath()

		pathLabel := routePath
		if pathLabel == "" {
			pathLabel = unmatchedRouteLabel
		}

		method := c.Request.Method
		status := strconv.Itoa(statusCode)
		latencySeconds := float64(latency) / float64(time.Second)

		m.HTTPRequestsTotal.WithLabelValues(cfg.ServiceName, method, pathLabel, status).Inc()
		m.HTTPRequestDurationSeconds.WithLabelValues(cfg.ServiceName, method, pathLabel, status).Observe(latencySeconds)

		if _, ok := skipLogPaths[rawPath]; ok {
			return
		}

		lvl := slog.LevelInfo
		if statusCode >= 500 {
			lvl = slog.LevelError
		} else if statusCode >= 400 {
			lvl = slog.LevelWarn
		}

		attrs := []any{
			"status", statusCode,
			"method", method,
			"path", pathLabel,
			"rawPath", rawPath,
			"query", c.Request.URL.RawQuery,
			"ip", c.ClientIP(),
			"latency", latency.String(),
			"userAgent", c.Request.UserAgent(),
			"request_id", c.GetString(requestIDContextKey),
		}

		if len(c.Errors) > 0 {
			errs := make([]string, len(c.Errors))
			for i, e := range c.Errors {
				errs[i] = e.Error()
			}
			attrs = append(attrs, "errors", errs)
			if lvl < slog.LevelWarn {
				lvl = slog.LevelWarn
			}
		}

		l.Log(c.Request.Context(), lvl, "http_request", attrs...)
	}
}
