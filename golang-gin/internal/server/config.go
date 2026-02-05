package server

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type Config struct {
	GinMode           string
	Host              string
	Port              int
	LogLevel          slog.Level
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	ReadHeaderTimeout time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	MaxBodyBytes      int64
	TrustedProxies    []string
	ServiceName       string
	Version           string
	BuildTime         string
}

func LoadConfig(serviceName, version, buildTime string) (Config, []string, error) {
	var warns []string

	mode := strings.TrimSpace(os.Getenv("GIN_MODE"))
	if mode == "" {
		mode = gin.ReleaseMode
	}

	host := strings.TrimSpace(os.Getenv("HOST"))
	if host == "" {
		host = "0.0.0.0"
	}

	portStr := strings.TrimSpace(os.Getenv("PORT"))
	if portStr == "" {
		portStr = "8080"
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		warns = append(warns, fmt.Sprintf("invalid PORT=%q; must be an integer", portStr))
		port = 0
	}

	logLevel, w := parseLogLevel(os.Getenv("LOG_LEVEL"))
	if w != "" {
		warns = append(warns, w)
	}

	readTimeout, w := parseDurationEnv("READ_TIMEOUT", 5*time.Second)
	if w != "" {
		warns = append(warns, w)
	}

	writeTimeout, w := parseDurationEnv("WRITE_TIMEOUT", 10*time.Second)
	if w != "" {
		warns = append(warns, w)
	}

	readHeaderTimeout, w := parseDurationEnv("READ_HEADER_TIMEOUT", 2*time.Second)
	if w != "" {
		warns = append(warns, w)
	}

	idleTimeout, w := parseDurationEnv("IDLE_TIMEOUT", 120*time.Second)
	if w != "" {
		warns = append(warns, w)
	}

	shutdownTimeout, w := parseDurationEnv("SHUTDOWN_TIMEOUT", 5*time.Second)
	if w != "" {
		warns = append(warns, w)
	}

	maxBody, w := parseInt64Env("MAX_BODY_BYTES", 1*1024*1024)
	if w != "" {
		warns = append(warns, w)
	}

	cfg := Config{
		GinMode:           mode,
		Host:              host,
		Port:              port,
		LogLevel:          logLevel,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
		ShutdownTimeout:   shutdownTimeout,
		MaxBodyBytes:      maxBody,
		TrustedProxies:    parseCSVEnv("TRUSTED_PROXIES"),
		ServiceName:       serviceName,
		Version:           version,
		BuildTime:         buildTime,
	}

	if err := cfg.Validate(); err != nil {
		return cfg, warns, err
	}
	return cfg, warns, nil
}

func parseLogLevel(s string) (slog.Level, string) {
	trimmed := strings.ToLower(strings.TrimSpace(s))
	switch trimmed {
	case "debug":
		return slog.LevelDebug, ""
	case "warn", "warning":
		return slog.LevelWarn, ""
	case "error":
		return slog.LevelError, ""
	case "info", "":
		return slog.LevelInfo, ""
	default:
		return slog.LevelInfo, fmt.Sprintf("invalid LOG_LEVEL=%q; defaulting to info", s)
	}
}

func parseDurationEnv(key string, def time.Duration) (time.Duration, string) {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return def, ""
	}

	d, err := time.ParseDuration(val)
	if err != nil || d <= 0 {
		return def, fmt.Sprintf("invalid %s=%q; using default %s", key, val, def)
	}

	return d, ""
}

func parseCSVEnv(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

func parseInt64Env(key string, def int64) (int64, string) {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return def, ""
	}

	n, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return def, fmt.Sprintf("invalid %s=%q; using default %d", key, val, def)
	}
	return n, ""
}

func (c Config) Validate() error {
	var errs []string

	switch c.GinMode {
	case gin.ReleaseMode, gin.DebugMode, gin.TestMode:
	default:
		errs = append(errs, fmt.Sprintf(
			"GIN_MODE must be one of %q, %q, %q, got %q",
			gin.ReleaseMode, gin.DebugMode, gin.TestMode, c.GinMode,
		))
	}

	if strings.TrimSpace(c.Host) == "" {
		errs = append(errs, "HOST must not be empty")
	}

	if c.Port <= 0 || c.Port > 65535 {
		errs = append(errs, fmt.Sprintf("PORT must be a valid TCP port (1-65535), got %d", c.Port))
	}

	if c.ReadTimeout <= 0 {
		errs = append(errs, fmt.Sprintf("READ_TIMEOUT must be > 0, got %s", c.ReadTimeout))
	}

	if c.WriteTimeout <= 0 {
		errs = append(errs, fmt.Sprintf("WRITE_TIMEOUT must be > 0, got %s", c.WriteTimeout))
	}

	if c.ReadHeaderTimeout <= 0 {
		errs = append(errs, fmt.Sprintf("READ_HEADER_TIMEOUT must be > 0, got %s", c.ReadHeaderTimeout))
	}

	if c.MaxBodyBytes < 0 {
		errs = append(errs, fmt.Sprintf("MAX_BODY_BYTES must be >= 0, got %d", c.MaxBodyBytes))
	}

	if c.IdleTimeout <= 0 {
		errs = append(errs, fmt.Sprintf("IDLE_TIMEOUT must be > 0, got %s", c.IdleTimeout))
	}

	if c.ShutdownTimeout <= 0 {
		errs = append(errs, fmt.Sprintf("SHUTDOWN_TIMEOUT must be > 0, got %s", c.ShutdownTimeout))
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed: %s", strings.Join(errs, "; "))
	}

	return nil
}
