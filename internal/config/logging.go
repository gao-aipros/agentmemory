package config

import (
	"log/slog"
	"os"
	"strings"
)

// LogLevelEnv is the environment variable that controls the log level.
const LogLevelEnv = "LOG_LEVEL"

// InitLogging configures the global slog logger based on the LOG_LEVEL
// environment variable. Defaults to "info". Output goes to stdout.
// Uses JSON handler when LOG_FORMAT=json, otherwise text handler.
func InitLogging() {
	level := parseLogLevel(os.Getenv(LogLevelEnv))
	format := strings.ToLower(os.Getenv("LOG_FORMAT"))

	var handler slog.Handler

	opts := &slog.HandlerOptions{
		Level: level,
	}

	if format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	slog.SetDefault(slog.New(handler))
}

// parseLogLevel converts a log level string to slog.Level.
// Defaults to slog.LevelInfo for unknown values.
func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Info logs at info level.
func Info(msg string, args ...any) {
	slog.Info(msg, args...)
}

// Debug logs at debug level.
func Debug(msg string, args ...any) {
	slog.Debug(msg, args...)
}

// Warn logs at warn level.
func Warn(msg string, args ...any) {
	slog.Warn(msg, args...)
}

// Error logs at error level.
func Error(msg string, args ...any) {
	slog.Error(msg, args...)
}
