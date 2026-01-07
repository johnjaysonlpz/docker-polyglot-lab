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
	Port              string
	LogLevel          slog.Level
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	ReadHeaderTimeout time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration

	TrustedProxies []string

	ServiceName string
	Version     string
	BuildTime   string
}

func parseLogLevel(s string) slog.Level {
	trimmed := strings.ToLower(strings.TrimSpace(s))
	switch trimmed {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	case "info", "":
		return slog.LevelInfo
	default:
		fmt.Fprintf(os.Stderr, "invalid LOG_LEVEL=%q, defaulting to info\n", s)
		return slog.LevelInfo
	}
}

func parseDurationEnv(key string, def time.Duration) time.Duration {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return def
	}

	d, err := time.ParseDuration(val)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid %s=%q: %v, using default %s\n", key, val, err, def)
		return def
	}

	return d
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

func LoadConfig(serviceName, version, buildTime string) Config {
	mode := os.Getenv("GIN_MODE")
	if mode == "" {
		mode = gin.ReleaseMode
	}

	host := strings.TrimSpace(os.Getenv("HOST"))
	if host == "" {
		host = "0.0.0.0"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	logLevel := parseLogLevel(os.Getenv("LOG_LEVEL"))

	return Config{
		GinMode:           mode,
		Host:              host,
		Port:              port,
		LogLevel:          logLevel,
		ReadTimeout:       parseDurationEnv("READ_TIMEOUT", 5*time.Second),
		WriteTimeout:      parseDurationEnv("WRITE_TIMEOUT", 10*time.Second),
		ReadHeaderTimeout: parseDurationEnv("READ_HEADER_TIMEOUT", 2*time.Second),
		IdleTimeout:       parseDurationEnv("IDLE_TIMEOUT", 120*time.Second),
		ShutdownTimeout:   parseDurationEnv("SHUTDOWN_TIMEOUT", 5*time.Second),

		TrustedProxies: parseCSVEnv("TRUSTED_PROXIES"),

		ServiceName: serviceName,
		Version:     version,
		BuildTime:   buildTime,
	}
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

	portStr := strings.TrimSpace(c.Port)
	if portStr == "" {
		errs = append(errs, "PORT must not be empty")
	} else if p, err := strconv.Atoi(portStr); err != nil || p <= 0 || p > 65535 {
		errs = append(errs, fmt.Sprintf("PORT must be a valid TCP port (1-65535), got %q", c.Port))
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
