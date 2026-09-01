// Package logger provides structured logging for glaw-code.
// It uses Go's log/slog package with configurable levels and formats.
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync/atomic"
	"time"
)

// LogLevel represents the severity of a log message.
// Values align with slog.Level for compatibility.
type LogLevel int

const (
	LevelUnset LogLevel = -99 // Special value to indicate "use default"
	LevelDebug LogLevel = -4
	LevelInfo  LogLevel = 0
	LevelWarn  LogLevel = 4
	LevelError LogLevel = 8
)

// String returns the string representation of the log level.
func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// ParseLogLevel parses a log level from a string.
func ParseLevel(s string) (LogLevel, error) {
	switch s {
	case "DEBUG", "debug":
		return LevelDebug, nil
	case "INFO", "info":
		return LevelInfo, nil
	case "WARN", "warn", "WARNING":
		return LevelWarn, nil
	case "ERROR", "error":
		return LevelError, nil
	default:
		return LevelInfo, fmt.Errorf("unknown log level: %q", s)
	}
}

// LogFormat represents the output format of logs.
type LogFormat string

const (
	FormatText LogFormat = "text"
	FormatJSON LogFormat = "json"
)

// ParseFormat parses a log format from a string.
func ParseFormat(s string) (LogFormat, error) {
	switch s {
	case "text", "TEXT":
		return FormatText, nil
	case "json", "JSON":
		return FormatJSON, nil
	default:
		return FormatText, fmt.Errorf("unknown log format: %q", s)
	}
}

// global logger instance (set by Init)
var logger *slog.Logger
var currentLevel atomic.Int64 // stores LogLevel as int64

// Init initializes the global logger with the given level and format.
// If level is LevelUnset (or negative), LevelInfo is used as default.
// If format is empty, FormatText is used as default.
// If w is nil, os.Stderr is used.
func Init(level LogLevel, format LogFormat, w io.Writer) {
	if level < 0 {
		level = LevelInfo
	}
	if format == "" {
		format = FormatText
	}
	if w == nil {
		w = os.Stderr
	}

	currentLevel.Store(int64(level))

	var opts *slog.HandlerOptions
	if level != 0 {
		opts = &slog.HandlerOptions{
			Level: slog.Level(level),
		}
	}

	var handler slog.Handler
	switch format {
	case FormatJSON:
		handler = slog.NewJSONHandler(w, opts)
	default:
		handler = slog.NewTextHandler(w, opts)
	}

	logger = slog.New(handler)
	slog.SetDefault(logger)
}

// Default returns the global logger, or a default stderr logger if not initialized.
func Default() *slog.Logger {
	if logger != nil {
		return logger
	}
	return slog.Default()
}

// Debug logs a debug message.
func Debug(msg string, args ...any) {
	Default().Debug(msg, args...)
}

// Info logs an info message.
func Info(msg string, args ...any) {
	Default().Info(msg, args...)
}

// Warn logs a warning message.
func Warn(msg string, args ...any) {
	Default().Warn(msg, args...)
}

// Error logs an error message.
func Error(msg string, args ...any) {
	Default().Error(msg, args...)
}

// DebugCtx logs a debug message with context.
func DebugCtx(ctx context.Context, msg string, args ...any) {
	Default().DebugContext(ctx, msg, args...)
}

// InfoCtx logs an info message with context.
func InfoCtx(ctx context.Context, msg string, args ...any) {
	Default().InfoContext(ctx, msg, args...)
}

// WarnCtx logs a warning message with context.
func WarnCtx(ctx context.Context, msg string, args ...any) {
	Default().WarnContext(ctx, msg, args...)
}

// ErrorCtx logs an error message with context.
func ErrorCtx(ctx context.Context, msg string, args ...any) {
	Default().ErrorContext(ctx, msg, args...)
}

// CurrentLevel returns the current log level.
func CurrentLevel() LogLevel {
	return LogLevel(currentLevel.Load())
}

// IsEnabled returns whether logging is enabled for the given level.
func IsEnabled(level LogLevel) bool {
	return level >= CurrentLevel()
}

// String is a helper for slog string attributes.
func String(key, value string) slog.Attr {
	return slog.String(key, value)
}

// Int is a helper for slog int attributes.
func Int(key string, value int) slog.Attr {
	return slog.Int(key, value)
}

// Int64 is a helper for slog int64 attributes.
func Int64(key string, value int64) slog.Attr {
	return slog.Int64(key, value)
}

// Float64 is a helper for slog float64 attributes.
func Float64(key string, value float64) slog.Attr {
	return slog.Float64(key, value)
}

// Duration is a helper for slog duration attributes.
func Duration(key string, value interface{ Duration() int64 }) slog.Attr {
	return slog.Duration(key, time.Duration(value.Duration()))
}

// Err is a helper for slog error attributes.
func Err(err error) slog.Attr {
	return slog.Any("error", err)
}

// Any is a helper for slog any attributes.
func Any(key string, value any) slog.Attr {
	return slog.Any(key, value)
}

// Redact returns a redacted string value for logging sensitive data.
// For short values (8 chars or less), returns "***".
// For longer values, shows first 4 and last 4 chars with "***" in between.
func Redact(value string) string {
	if len(value) <= 8 {
		return "***"
	}
	return value[:4] + "***" + value[len(value)-4:]
}
