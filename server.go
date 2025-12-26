package socket

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"sync"
	"time"
)

// Handler is the interface for handling incoming TCP connections.
// Implementations should handle the connection lifecycle and message processing.
type Handler interface {
	// Handle is called for each new connection.
	// The implementation is responsible for managing the connection.
	Handle(conn *net.TCPConn)
}

// Server represents a TCP server that listens for incoming connections.
type Server struct {
	listener        *net.TCPListener
	logger          Logger
	shutdownTimeout time.Duration

	mu          sync.Mutex
	shutdown    bool
	shutdownNow chan struct{} // signals immediate shutdown, bypassing timeout
}

// ServerOption configures a Server.
type ServerOption func(*Server)

// ServerLoggerOption sets the logger for the server.
func ServerLoggerOption(logger Logger) ServerOption {
	return func(s *Server) {
		s.logger = logger
	}
}

// ServerShutdownTimeoutOption sets the graceful shutdown timeout.
// When the context is canceled, the server will wait up to this duration
// before closing the listener. This gives existing connections time to complete.
// Default is 0 (immediate shutdown).
//
// Note: This only delays listener closure. For full graceful shutdown with
// connection draining, track connections at the application level and cancel
// them with the context passed to Conn.Run().
func ServerShutdownTimeoutOption(timeout time.Duration) ServerOption {
	return func(s *Server) {
		s.shutdownTimeout = timeout
	}
}

// New creates a new TCP server bound to the specified address.
// Returns an error if the address cannot be bound.
func New(addr *net.TCPAddr, opts ...ServerOption) (*Server, error) {
	listener, err := net.ListenTCP(addr.Network(), addr)
	if err != nil {
		return nil, err
	}

	s := &Server{
		listener:    listener,
		logger:      slog.Default(),
		shutdownNow: make(chan struct{}),
	}

	for _, opt := range opts {
		opt(s)
	}

	return s, nil
}

// Serve starts accepting connections and dispatching them to the handler.
// It blocks until the context is canceled or an unrecoverable error occurs.
// When the context is canceled, it stops accepting new connections gracefully.
// If ServerShutdownTimeoutOption is set, the server waits up to the specified
// duration before stopping, allowing existing handlers to complete. Call Close()
// to bypass the timeout and shut down immediately.
func (s *Server) Serve(ctx context.Context, handler Handler) error {
	s.logger.Info("server started", "addr", s.listener.Addr())

	// Start a goroutine to handle context cancellation
	go func() {
		<-ctx.Done()

		// Wait for shutdown timeout if configured, but allow early exit via Close()
		if s.shutdownTimeout > 0 {
			s.logger.Info("graceful shutdown initiated", "timeout", s.shutdownTimeout)
			select {
			case <-time.After(s.shutdownTimeout):
				// Timeout expired, proceed with shutdown
			case <-s.shutdownNow:
				// Close() was called, skip remaining timeout
				s.logger.Debug("shutdown timeout bypassed via Close()")
			}
		}

		s.mu.Lock()
		s.shutdown = true
		s.mu.Unlock()
		// Set a deadline to unblock Accept
		_ = s.listener.SetDeadline(time.Now())
	}()

	for {
		conn, err := s.listener.AcceptTCP()
		if err != nil {
			s.mu.Lock()
			isShutdown := s.shutdown
			s.mu.Unlock()

			if isShutdown {
				s.logger.Info("server stopped", "addr", s.listener.Addr())
				return ctx.Err()
			}

			// Check if it's a temporary error
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				continue
			}
			s.logger.Error("accept error", "error", err)
			return err
		}

		s.logger.Debug("accepted connection", "remote_addr", conn.RemoteAddr())
		_ = conn.SetNoDelay(true)
		go handler.Handle(conn)
	}
}

// Close stops the server by closing the underlying listener.
// If a shutdown timeout is configured, Close() bypasses the remaining timeout.
// Any blocked Accept calls will return with an error.
func (s *Server) Close() error {
	s.mu.Lock()
	s.shutdown = true
	s.mu.Unlock()

	// Signal to bypass any pending shutdown timeout
	select {
	case s.shutdownNow <- struct{}{}:
	default:
		// Channel already has a signal or no one is listening
	}

	return s.listener.Close()
}

// Addr returns the listener's network address.
func (s *Server) Addr() net.Addr {
	return s.listener.Addr()
}
