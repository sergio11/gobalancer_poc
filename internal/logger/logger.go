package logger

import (
	"log/slog"
	"os"
	"strings"
)

var Log *slog.Logger

func Init(levelStr string) *slog.Logger {
	var level slog.Level
	switch strings.ToLower(levelStr) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	handler := slog.NewJSONHandler(os.Stdout, opts)
	Log = slog.New(handler)
	slog.SetDefault(Log)
	return Log
}

func Get() *slog.Logger {
	if Log == nil {
		return Init("info")
	}
	return Log
}
