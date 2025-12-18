package socket

import (
	"testing"
	"time"
)

func TestCustomCodecOption(t *testing.T) {
	codec := &mockCodec{}
	opt := CustomCodecOption(codec)

	var opts options
	opt(&opts)

	if opts.codec != codec {
		t.Error("codec not set correctly")
	}
}

func TestBufferSizeOption(t *testing.T) {
	opt := BufferSizeOption(100)

	var opts options
	opt(&opts)

	if opts.bufferSize != 100 {
		t.Errorf("bufferSize = %d, want 100", opts.bufferSize)
	}
}

func TestHeartbeatOption(t *testing.T) {
	heartbeat := time.Minute * 5
	opt := HeartbeatOption(heartbeat)

	var opts options
	opt(&opts)

	if opts.heartbeat != heartbeat {
		t.Errorf("heartbeat = %v, want %v", opts.heartbeat, heartbeat)
	}
}

func TestMessageMaxSize(t *testing.T) {
	opt := MessageMaxSize(4096)

	var opts options
	opt(&opts)

	if opts.maxReadLength != 4096 {
		t.Errorf("maxReadLength = %d, want 4096", opts.maxReadLength)
	}
}

func TestOnErrorOption(t *testing.T) {
	called := false
	onError := func(err error) ErrorAction {
		called = true
		return Disconnect
	}
	opt := OnErrorOption(onError)

	var opts options
	opt(&opts)

	if opts.onError == nil {
		t.Fatal("onError is nil")
	}

	// Call to verify it's the right function
	opts.onError(nil)
	if !called {
		t.Error("onError callback not called")
	}
}

func TestOnMessageOption(t *testing.T) {
	called := false
	onMessage := func(msg Message) error {
		called = true
		return nil
	}
	opt := OnMessageOption(onMessage)

	var opts options
	opt(&opts)

	if opts.onMessage == nil {
		t.Fatal("onMessage is nil")
	}

	// Call to verify it's the right function
	opts.onMessage(nil)
	if !called {
		t.Error("onMessage callback not called")
	}
}

func TestLoggerOption(t *testing.T) {
	logger := &mockLogger{}
	opt := LoggerOption(logger)

	var opts options
	opt(&opts)

	if opts.logger != logger {
		t.Error("logger not set correctly")
	}
}

func TestOptions_MultipleOptions(t *testing.T) {
	codec := &mockCodec{}
	logger := &mockLogger{}
	onMessage := func(msg Message) error { return nil }
	onError := func(err error) ErrorAction { return Continue }
	heartbeat := time.Second * 45
	bufferSize := 50
	maxSize := 8192

	var opts options
	options := []Option{
		CustomCodecOption(codec),
		OnMessageOption(onMessage),
		OnErrorOption(onError),
		HeartbeatOption(heartbeat),
		BufferSizeOption(bufferSize),
		MessageMaxSize(maxSize),
		LoggerOption(logger),
	}

	for _, opt := range options {
		opt(&opts)
	}

	if opts.codec != codec {
		t.Error("codec not set")
	}
	if opts.onMessage == nil {
		t.Error("onMessage not set")
	}
	if opts.onError == nil {
		t.Error("onError not set")
	}
	if opts.heartbeat != heartbeat {
		t.Errorf("heartbeat = %v, want %v", opts.heartbeat, heartbeat)
	}
	if opts.bufferSize != bufferSize {
		t.Errorf("bufferSize = %d, want %d", opts.bufferSize, bufferSize)
	}
	if opts.maxReadLength != maxSize {
		t.Errorf("maxReadLength = %d, want %d", opts.maxReadLength, maxSize)
	}
	if opts.logger != logger {
		t.Error("logger not set")
	}
}

func TestErrorAction(t *testing.T) {
	// Test Disconnect constant
	if Disconnect != 0 {
		t.Errorf("Disconnect = %d, want 0", Disconnect)
	}

	// Test Continue constant
	if Continue != 1 {
		t.Errorf("Continue = %d, want 1", Continue)
	}
}
