package socket

import (
	"time"
)

// ErrorAction defines the action to take when an error occurs.
type ErrorAction int

const (
	// Disconnect closes the connection when an error occurs.
	Disconnect ErrorAction = iota
	// Continue suppresses the error and continues processing.
	Continue
)

// options holds the configuration for a connection.
type options struct {
	codec  Codec
	logger Logger

	onMessage func(message Message) error
	// onError is called when an error occurs.
	// Returns Disconnect to close the connection, Continue to suppress the error.
	onError func(error) ErrorAction

	bufferSize    int           // size of buffered channel
	maxReadLength int           // maximum size of a single message
	heartbeat     time.Duration // heartbeat interval for read/write deadlines
}

// Option is a function that configures connection options.
type Option func(*options)

// CustomCodecOption returns an Option that sets the message codec.
// The codec is required and must be provided before creating a connection.
func CustomCodecOption(codec Codec) Option {
	return func(o *options) {
		o.codec = codec
	}
}

// BufferSizeOption returns an Option that sets the size of the send channel buffer.
// A larger buffer allows more messages to be queued before blocking.
func BufferSizeOption(size int) Option {
	return func(o *options) {
		o.bufferSize = size
	}
}

// HeartbeatOption returns an Option that sets the heartbeat interval.
// This determines the read/write deadline timeout (heartbeat * 2).
func HeartbeatOption(heartbeat time.Duration) Option {
	return func(o *options) {
		o.heartbeat = heartbeat
	}
}

// MessageMaxSize returns an Option that sets the maximum message buffer size.
// Messages larger than this size cannot be received.
func MessageMaxSize(size int) Option {
	return func(o *options) {
		o.maxReadLength = size
	}
}

// OnErrorOption returns an Option that sets the error callback.
// The callback is invoked when a read/write error occurs.
// Return Disconnect to close the connection, or Continue to suppress the error.
func OnErrorOption(cb func(error) ErrorAction) Option {
	return func(o *options) {
		o.onError = cb
	}
}

// OnMessageOption returns an Option that sets the message handler callback.
// This callback is required and is invoked for each received message.
func OnMessageOption(cb func(Message) error) Option {
	return func(o *options) {
		o.onMessage = cb
	}
}

// LoggerOption returns an Option that sets the logger.
// If not set, the default slog logger will be used.
func LoggerOption(logger Logger) Option {
	return func(o *options) {
		o.logger = logger
	}
}
