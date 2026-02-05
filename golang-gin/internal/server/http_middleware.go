package server

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var (
	timeNow   = time.Now
	timeSince = time.Since
)

var skipLogPaths = map[string]struct{}{
	LivenessPath:  {},
	ReadinessPath: {},
	MetricsPath:   {},
}

func isAllowedRequestIDRune(r rune) bool {
	switch {
	case r == '-', r == '_', r == '.', r == ':':
		return true
	case r >= '0' && r <= '9':
		return true
	case r >= 'a' && r <= 'z':
		return true
	case r >= 'A' && r <= 'Z':
		return true
	default:
		return false
	}
}

func newRequestID() string {
	return uuid.NewString()
}

func sanitizeIncomingRequestID(in string) (string, bool) {
	in = strings.TrimSpace(in)
	if in == "" {
		return "", false
	}
	if len(in) > 128 {
		return "", false
	}
	if strings.ContainsAny(in, "\r\n\t") {
		return "", false
	}
	for _, r := range in {
		if !isAllowedRequestIDRune(r) {
			return "", false
		}
	}
	return in, true
}

func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid, ok := sanitizeIncomingRequestID(c.GetHeader(requestIDHeader))
		if !ok {
			rid = newRequestID()
		}

		c.Set(requestIDContextKey, rid)
		c.Writer.Header().Set(requestIDHeader, rid)

		c.Next()
	}
}

func MaxBodyBytesMiddleware(limit int64) gin.HandlerFunc {
	if limit <= 0 {
		return func(c *gin.Context) { c.Next() }
	}

	return func(c *gin.Context) {
		if c.Request.ContentLength > limit {
			WriteError(c, http.StatusRequestEntityTooLarge, "payload_too_large", "payload too large")
			return
		}

		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		c.Next()
	}
}

func MaxBytesErrorMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		for _, e := range c.Errors {
			var mbe *http.MaxBytesError
			if !errors.As(e.Err, &mbe) {
				continue
			}

			if c.Writer.Written() {
				c.Status(http.StatusRequestEntityTooLarge)
				return
			}

			WriteError(c, http.StatusRequestEntityTooLarge, "payload_too_large", "payload too large")
			return
		}
	}
}

func levelForStatus(code int) slog.Level {
	switch {
	case code >= 500:
		return slog.LevelError
	case code >= 400:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}

func shouldSkipLog(rawPath string, statusCode int) bool {
	_, ok := skipLogPaths[rawPath]
	return ok && statusCode < 400
}

func requestLoggerFromContext(c *gin.Context, base *slog.Logger) *slog.Logger {
	if base == nil {
		base = slog.Default()
	}
	if c == nil {
		return base
	}
	if v, ok := c.Get(loggerContextKey); ok {
		if rl, ok := v.(*slog.Logger); ok && rl != nil {
			return rl
		}
	}
	return base
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "…"
}

func appendErrorAttrs(attrs []any, c *gin.Context) ([]any, slog.Level) {
	if c == nil || len(c.Errors) == 0 {
		return attrs, slog.LevelInfo
	}

	filtered := make([]*gin.Error, 0, len(c.Errors))
	for _, e := range c.Errors {
		var pe PanicError
		if errors.As(e.Err, &pe) {
			continue
		}
		filtered = append(filtered, e)
	}

	if len(filtered) == 0 {
		return attrs, slog.LevelInfo
	}

	errs := make([]string, len(filtered))
	for i, e := range filtered {
		errs[i] = e.Error()
	}
	attrs = append(attrs, "errors", errs)

	if first := filtered[0].Err; first != nil {
		attrs = append(attrs,
			"error", fmt.Sprintf("%T", first),
			"error_message", truncate(first.Error(), 300),
		)
	}

	return attrs, slog.LevelWarn
}

func GinSlogMiddleware(l *slog.Logger, _ Config, m *Metrics) gin.HandlerFunc {
	if l == nil {
		l = slog.Default()
	}

	return func(c *gin.Context) {
		start := timeNow()
		rawPath := c.Request.URL.Path

		c.Next()

		latency := timeSince(start)

		latencyMsRaw := float64(latency.Nanoseconds()) / 1e6
		latencyMs := math.Round(latencyMsRaw*1e3) / 1e3
		if latencyMs == 0 && latencyMsRaw > 0 {
			latencyMs = math.Round(latencyMsRaw*1e6) / 1e6
		}

		statusCode := c.Writer.Status()
		routePath := c.FullPath()

		pathLabel := routePath
		if pathLabel == "" {
			pathLabel = unmatchedRouteLabel
		}

		method := c.Request.Method
		status := strconv.Itoa(statusCode)

		m.HTTPRequestsTotal.WithLabelValues(method, pathLabel, status).Inc()
		m.HTTPRequestDurationSeconds.WithLabelValues(method, pathLabel, status).Observe(latency.Seconds())

		if shouldSkipLog(rawPath, statusCode) {
			return
		}

		lvl := levelForStatus(statusCode)

		bytesWritten := c.Writer.Size()
		if bytesWritten < 0 {
			bytesWritten = 0
		}

		reqLogger := requestLoggerFromContext(c, l)

		attrs := []any{
			"method", method,
			"path", pathLabel,
			"raw_path", rawPath,
			"status", statusCode,
			"ip", c.ClientIP(),
			"latency_ms", latencyMs,
			"bytes_written", bytesWritten,
			"user_agent", c.Request.UserAgent(),
		}

		var errLvl slog.Level
		attrs, errLvl = appendErrorAttrs(attrs, c)
		if errLvl > lvl {
			lvl = errLvl
		}

		if errLvl == slog.LevelInfo && statusCode >= 500 {
			attrs = append(attrs, "error", "HTTP_5XX")
		}

		reqLogger.Log(c.Request.Context(), lvl, "http_request", attrs...)
	}
}
