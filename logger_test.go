package socket

import (
	"log/slog"
	"testing"
)

func TestLogger_Interface(t *testing.T) {
	// Verify that *slog.Logger implements our Logger interface
	var _ Logger = slog.Default()
}

func TestDefaultLogger(t *testing.T) {
	logger := defaultLogger()

	if logger == nil {
		t.Fatal("defaultLogger returned nil")
	}

	// Verify it's the slog default
	if logger != slog.Default() {
		t.Error("defaultLogger did not return slog.Default()")
	}
}

func TestDefaultLogger_Methods(t *testing.T) {
	logger := defaultLogger()

	// These should not panic - just verify they can be called
	logger.Debug("debug message", "key", "value")
	logger.Info("info message", "key", "value")
	logger.Warn("warn message", "key", "value")
	logger.Error("error message", "key", "value")
}

// mockLogger for testing Logger interface
type mockLogger struct {
	debugCalled bool
	infoCalled  bool
	warnCalled  bool
	errorCalled bool
	lastMsg     string
	lastArgs    []any
}

func (l *mockLogger) Debug(msg string, args ...any) {
	l.debugCalled = true
	l.lastMsg = msg
	l.lastArgs = args
}

func (l *mockLogger) Info(msg string, args ...any) {
	l.infoCalled = true
	l.lastMsg = msg
	l.lastArgs = args
}

func (l *mockLogger) Warn(msg string, args ...any) {
	l.warnCalled = true
	l.lastMsg = msg
	l.lastArgs = args
}

func (l *mockLogger) Error(msg string, args ...any) {
	l.errorCalled = true
	l.lastMsg = msg
	l.lastArgs = args
}

func TestLogger_CustomImplementation(t *testing.T) {
	var logger Logger = &mockLogger{}

	mock := logger.(*mockLogger)

	logger.Debug("test debug", "key1", "value1")
	if !mock.debugCalled {
		t.Error("Debug not called")
	}
	if mock.lastMsg != "test debug" {
		t.Errorf("lastMsg = %s, want 'test debug'", mock.lastMsg)
	}

	logger.Info("test info", "key2", "value2")
	if !mock.infoCalled {
		t.Error("Info not called")
	}

	logger.Warn("test warn", "key3", "value3")
	if !mock.warnCalled {
		t.Error("Warn not called")
	}

	logger.Error("test error", "key4", "value4")
	if !mock.errorCalled {
		t.Error("Error not called")
	}
}
