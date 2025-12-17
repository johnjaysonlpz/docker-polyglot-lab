package server

import (
	"log"
	"log/slog"
	"os"
)

func NewLogger(level slog.Level) (*slog.Logger, *log.Logger) {
	handlerOpts := &slog.HandlerOptions{
		Level: level,
	}

	handler := slog.NewJSONHandler(os.Stdout, handlerOpts)

	slogger := slog.New(handler)

	stdLogger := slog.NewLogLogger(handler, slog.LevelError)

	return slogger, stdLogger
}
