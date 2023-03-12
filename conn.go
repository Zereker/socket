package socket

import (
	"context"
	"net"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

var (
	ErrInvalidCodec     = errors.New("invalid codec callback")
	ErrInvalidOnMessage = errors.New("invalid on message callback")
)

// Conn represents a client connection to a TCP server.
type Conn struct {
	rawConn *net.TCPConn

	opts options

	heartbeat time.Duration
	sendMsg   chan []byte
}

const (
	defaultBufferSize32     = 32
	defaultMaxPackageLength = 1024 * 1024
)

// NewConn returns a new client connection which has not started to
// serve requests yet.
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

func checkOptions(opts *options) error {
	if opts.bufferSize <= 0 {
		opts.bufferSize = defaultBufferSize32
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

	return nil
}

func newClientConnWithOptions(c *net.TCPConn, opts options) *Conn {
	cc := &Conn{
		rawConn:   c,
		opts:      opts,
		heartbeat: opts.heartbeat,
		sendMsg:   make(chan []byte, opts.bufferSize),
	}

	return cc
}

// Run run the connection, creating two go-routines
// for reading and writing.
func (c *Conn) Run(ctx context.Context) error {
	group, child := errgroup.WithContext(ctx)

	// Read Loop
	group.Go(func() error {
		return c.readLoop(child)
	})

	// Write Loop
	group.Go(func() error {
		return c.writeLoop(child)
	})

	// Wait err and break
	if err := group.Wait(); err != nil {
		c.close()
		return err
	}

	return nil
}

// Write write []byte to the Conn.
func (c *Conn) Write(message Message) error {
	bytes, err := c.opts.codec.Encode(message)
	if err != nil {
		return err
	}

	select {
	case c.sendMsg <- bytes:
		return nil
	default:
		return errors.New("chan blocked")
	}
}

func (c *Conn) Addr() net.Addr {
	return c.rawConn.RemoteAddr()
}

func (c *Conn) read(data []byte) (int, error) {
	c.rawConn.SetReadDeadline(time.Now().Add(c.heartbeat * 2))

	n, err := c.rawConn.Read(data)

	// 如果错误处理方, 觉得不需要接着运行, 返回 error
	if err != nil && c.opts.onError(err) {
		return 0, err
	}

	return n, nil
}

// readLoop() blocking read from connection
// if blocked, will throw error
func (c *Conn) readLoop(ctx context.Context) error {
	data := make([]byte, c.opts.maxReadLength)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			_, err := c.read(data)
			if err != nil {
				return err
			}

			message, err := c.opts.codec.Decode(data)
			if err != nil {
				return err
			}

			err = c.opts.onMessage(message)
			if err != nil {
				return err
			}

			data = data[:c.opts.maxReadLength]
		}
	}
}

// writeLoop() receive message from channel
// serialize it into bytes, then blocking write into connection
func (c *Conn) writeLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case data := <-c.sendMsg:
			err := c.write(data)
			if err != nil {
				return err
			}
		}
	}
}

func (c *Conn) write(data []byte) error {
	c.rawConn.SetWriteDeadline(time.Now().Add(c.heartbeat * 2))

	_, err := c.rawConn.Write(data)

	// 如果错误处理方, 觉得不需要接着运行, 返回 error
	if err != nil && c.opts.onError(err) {
		return err
	}

	return nil
}

func (c *Conn) close() {
	c.rawConn.Close()
}
