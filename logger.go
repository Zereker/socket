package socket

import "log/slog"

// Logger is the interface for structured logging.
// It is designed to be compatible with *slog.Logger from the standard library.
// Applications can provide their own implementation or use the default slog logger.
type Logger interface {
	// Debug logs a debug-level message with optional key-value pairs.
	Debug(msg string, args ...any)
	// Info logs an info-level message with optional key-value pairs.
	Info(msg string, args ...any)
	// Warn logs a warning-level message with optional key-value pairs.
	Warn(msg string, args ...any)
	// Error logs an error-level message with optional key-value pairs.
	Error(msg string, args ...any)
}

// defaultLogger returns the default slog logger from the standard library.
func defaultLogger() Logger {
	return slog.Default()
}
