package main

import (
	"context"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Zereker/socket"
)

type message struct {
	body []byte
}

func (m message) Length() int {
	return len(m.body)
}

func (m message) Body() []byte {
	return m.body
}

// codec implements a simple line-based protocol.
// In production, you should implement proper message framing
// (e.g., length-prefixed messages) to handle TCP packet fragmentation.
type codec struct{}

func (c *codec) Decode(r io.Reader) (socket.Message, error) {
	// Read up to 1024 bytes as a simple example.
	// In production, implement proper message framing.
	buf := make([]byte, 1024)
	n, err := r.Read(buf)
	if err != nil {
		return nil, err
	}
	return message{body: buf[:n]}, nil
}

func (c *codec) Encode(msg socket.Message) ([]byte, error) {
	return msg.Body(), nil
}

type Server struct {
	connID int64

	sync.RWMutex
	connections map[int64]*socket.Conn
}

func newHandler(connID int64) *Server {
	return &Server{connID: connID, connections: make(map[int64]*socket.Conn)}
}

func (s *Server) Handle(conn *net.TCPConn) {
	connID := atomic.AddInt64(&s.connID, 1)

	codecOption := socket.CustomCodecOption(new(codec))
	errorOption := socket.OnErrorOption(func(err error) socket.ErrorAction {
		slog.Error("connection error", "error", err)
		return socket.Disconnect
	})

	// Echo
	onMessageOption := socket.OnMessageOption(func(m socket.Message) error {
		conn := s.getConn(connID)
		return conn.Write(m)
	})

	newConn, err := socket.NewConn(conn, codecOption, errorOption, onMessageOption)
	if err != nil {
		panic(err)
	}

	s.addConn(connID, newConn)

	if err = newConn.Run(context.Background()); err != nil {
		s.deleteConn(connID)
	}
}

func (s *Server) addConn(connID int64, conn *socket.Conn) {
	s.Lock()
	defer s.Unlock()

	slog.Info("add new conn", "connID", connID, "addr", conn.Addr())
	s.connections[connID] = conn
}

func (s *Server) deleteConn(connID int64) {
	s.Lock()
	defer s.Unlock()

	delete(s.connections, connID)
}

func (s *Server) getConn(connID int64) *socket.Conn {
	s.RLock()
	defer s.RUnlock()

	if conn, ok := s.connections[connID]; ok {
		return conn
	}

	return nil
}

func main() {
	addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:12345")
	if err != nil {
		panic(err)
	}

	server, err := socket.New(addr)
	if err != nil {
		slog.Error("failed to create server", "error", err)
		return
	}

	// Handle graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		slog.Info("shutting down server...")
		cancel()
	}()

	slog.Info("server start", "addr", addr.String())
	if err := server.Serve(ctx, newHandler(time.Now().Unix())); err != nil {
		slog.Error("server error", "error", err)
	}
}
