// Package socket provides a simple TCP server framework for Go.
// It supports custom message encoding/decoding, asynchronous I/O operations,
// and connection management with heartbeat monitoring.
package socket

import (
	"bufio"
	"context"
	"errors"
	"net"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
)

// Errors returned by connection operations.
var (
	// ErrInvalidCodec is returned when no codec is provided.
	ErrInvalidCodec = errors.New("invalid codec callback")
	// ErrInvalidOnMessage is returned when no message handler is provided.
	ErrInvalidOnMessage = errors.New("invalid on message callback")
)

// ErrConnectionClosed is returned when operating on a closed connection.
var ErrConnectionClosed = errors.New("connection closed")

// Conn represents a client connection to a TCP server.
// It manages the underlying TCP connection, message encoding/decoding,
// and provides read/write loops for asynchronous communication.
type Conn struct {
	rawConn *net.TCPConn
	reader  *bufio.Reader
	logger  Logger

	opts options

	sendMsg chan []byte
	closed  atomic.Bool
	cancel  context.CancelFunc
}

// Default configuration values.
const (
	// defaultBufferSize is the default size of the message channel buffer.
	defaultBufferSize = 1
	// defaultMaxPackageLength is the default maximum size of a single message (1MB).
	defaultMaxPackageLength = 1024 * 1024
)

// NewConn creates a new connection wrapper around the given TCP connection.
// It applies the provided options and validates them before returning.
// Returns an error if required options (codec, onMessage) are missing.
func NewConn(conn *net.TCPConn, opt ...Option) (*Conn, error) {
	var opts options
	for _, o := range opt {
		o(&opts)
	}

	err := checkOptions(&opts)
	if err != nil {
		return nil, err
	}

	return newClientConnWithOptions(conn, opts), nil
}

// checkOptions validates and sets default values for connection options.
func checkOptions(opts *options) error {
	if opts.bufferSize <= 0 {
		opts.bufferSize = defaultBufferSize
	}

	if opts.maxReadLength <= 0 {
		opts.maxReadLength = defaultMaxPackageLength
	}

	if opts.onMessage == nil {
		return ErrInvalidOnMessage
	}

	if opts.heartbeat <= 0 {
		opts.heartbeat = time.Second * 30
	}

	if opts.codec == nil {
		return ErrInvalidCodec
	}

	if opts.onError == nil {
		opts.onError = func(err error) ErrorAction { return Disconnect }
	}

	if opts.logger == nil {
		opts.logger = defaultLogger()
	}

	return nil
}

// newClientConnWithOptions creates a new Conn with the given options.
func newClientConnWithOptions(c *net.TCPConn, opts options) *Conn {
	cc := &Conn{
		rawConn: c,
		reader:  bufio.NewReaderSize(c, opts.maxReadLength),
		logger:  opts.logger,
		opts:    opts,
		sendMsg: make(chan []byte, opts.bufferSize),
	}

	return cc
}

// Run starts the connection's read and write loops.
// It creates two goroutines for concurrent reading and writing,
// and blocks until an error occurs or the context is canceled.
// The connection is automatically closed when Run returns.
func (c *Conn) Run(ctx context.Context) error {
	c.logger.Debug("connection started", "addr", c.Addr())

	ctx, c.cancel = context.WithCancel(ctx)
	group, child := errgroup.WithContext(ctx)

	group.Go(func() error {
		return c.readLoop(child)
	})

	group.Go(func() error {
		return c.writeLoop(child)
	})

	err := group.Wait()
	c.closeConn()

	if err != nil {
		c.logger.Debug("connection closed with error", "addr", c.Addr(), "error", err)
	} else {
		c.logger.Debug("connection closed", "addr", c.Addr())
	}

	return err
}

// Close gracefully closes the connection.
// It cancels the context and closes the underlying TCP connection.
// Safe to call multiple times.
func (c *Conn) Close() error {
	if c.closed.Swap(true) {
		return nil // already closed
	}
	if c.cancel != nil {
		c.cancel()
	}
	return c.rawConn.Close()
}

// IsClosed returns true if the connection has been closed.
func (c *Conn) IsClosed() bool {
	return c.closed.Load()
}

// ErrBufferFull is returned when the send buffer is full and cannot accept more messages.
var ErrBufferFull = errors.New("send buffer full")

// Write sends a message through the connection without blocking.
// The message is encoded using the configured codec and queued for sending.
// Returns ErrBufferFull if the send buffer is full, or ErrConnectionClosed if closed.
func (c *Conn) Write(message Message) error {
	if c.closed.Load() {
		return ErrConnectionClosed
	}

	bytes, err := c.opts.codec.Encode(message)
	if err != nil {
		return err
	}

	select {
	case c.sendMsg <- bytes:
		return nil
	default:
		return ErrBufferFull
	}
}

// WriteBlocking sends a message through the connection, blocking until the message
// is queued or the context is canceled.
// Returns ErrConnectionClosed if the connection is closed.
func (c *Conn) WriteBlocking(ctx context.Context, message Message) error {
	if c.closed.Load() {
		return ErrConnectionClosed
	}

	bytes, err := c.opts.codec.Encode(message)
	if err != nil {
		return err
	}

	select {
	case c.sendMsg <- bytes:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// WriteTimeout sends a message through the connection with a timeout.
// Returns ErrBufferFull if the message cannot be queued within the timeout,
// or ErrConnectionClosed if the connection is closed.
func (c *Conn) WriteTimeout(message Message, timeout time.Duration) error {
	if c.closed.Load() {
		return ErrConnectionClosed
	}

	bytes, err := c.opts.codec.Encode(message)
	if err != nil {
		return err
	}

	select {
	case c.sendMsg <- bytes:
		return nil
	case <-time.After(timeout):
		return ErrBufferFull
	}
}

// Addr returns the remote address of the connection.
func (c *Conn) Addr() net.Addr {
	return c.rawConn.RemoteAddr()
}

// readLoop continuously reads from the connection and processes messages.
// It decodes incoming data using the configured codec and calls the message handler.
// Returns when the context is canceled or an unrecoverable error occurs.
func (c *Conn) readLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			_ = c.rawConn.SetReadDeadline(time.Now().Add(c.opts.heartbeat * 2))

			message, err := c.opts.codec.Decode(c.reader)
			if err != nil {
				c.logger.Debug("read error", "addr", c.Addr(), "error", err)
				if c.opts.onError(err) == Disconnect {
					return err
				}
				continue
			}

			if err = c.opts.onMessage(message); err != nil {
				return err
			}
		}
	}
}

// writeLoop continuously sends messages from the send channel to the connection.
// Returns when the context is canceled or an unrecoverable error occurs.
func (c *Conn) writeLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case data := <-c.sendMsg:
			if err := c.write(data); err != nil {
				return err
			}
		}
	}
}

// write sends data to the connection with a deadline.
// If an error occurs and onError returns true, the error is propagated.
// Otherwise, the error is suppressed and writing continues.
func (c *Conn) write(data []byte) error {
	_ = c.rawConn.SetWriteDeadline(time.Now().Add(c.opts.heartbeat * 2))

	_, err := c.rawConn.Write(data)

	if err != nil {
		c.logger.Debug("write error", "addr", c.Addr(), "error", err)
		if c.opts.onError(err) == Disconnect {
			return err
		}
	}

	return nil
}

// closeConn marks the connection as closed and closes the underlying TCP connection.
func (c *Conn) closeConn() {
	c.closed.Store(true)
	c.rawConn.Close()
}
