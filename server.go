package socket

import (
	"context"
	"errors"
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
	listener *net.TCPListener

	mu       sync.Mutex
	shutdown bool
}

// New creates a new TCP server bound to the specified address.
// Returns an error if the address cannot be bound.
func New(addr *net.TCPAddr) (*Server, error) {
	listener, err := net.ListenTCP(addr.Network(), addr)
	if err != nil {
		return nil, err
	}

	return &Server{
		listener: listener,
	}, nil
}

// Serve starts accepting connections and dispatching them to the handler.
// It blocks until the context is canceled or an unrecoverable error occurs.
// When the context is canceled, it stops accepting new connections gracefully.
func (s *Server) Serve(ctx context.Context, handler Handler) error {
	// Start a goroutine to handle context cancellation
	go func() {
		<-ctx.Done()
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
				return ctx.Err()
			}

			// Check if it's a temporary error
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				continue
			}
			return err
		}

		_ = conn.SetNoDelay(true)
		go handler.Handle(conn)
	}
}

// Close stops the server by closing the underlying listener.
// Any blocked Accept calls will return with an error.
func (s *Server) Close() error {
	s.mu.Lock()
	s.shutdown = true
	s.mu.Unlock()
	return s.listener.Close()
}

// Addr returns the listener's network address.
func (s *Server) Addr() net.Addr {
	return s.listener.Addr()
}
