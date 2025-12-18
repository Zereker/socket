package socket

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"
)

// mockHandler implements Handler interface for testing
type mockHandler struct {
	mu       sync.Mutex
	conns    []*net.TCPConn
	handleCh chan *net.TCPConn
}

func newMockHandler() *mockHandler {
	return &mockHandler{
		conns:    make([]*net.TCPConn, 0),
		handleCh: make(chan *net.TCPConn, 10),
	}
}

func (h *mockHandler) Handle(conn *net.TCPConn) {
	h.mu.Lock()
	h.conns = append(h.conns, conn)
	h.mu.Unlock()

	select {
	case h.handleCh <- conn:
	default:
	}
}

func (h *mockHandler) getConns() []*net.TCPConn {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.conns
}

func TestNew(t *testing.T) {
	addr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}
	server, err := New(addr)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer server.Close()

	if server.listener == nil {
		t.Error("listener is nil")
	}
}

func TestNew_InvalidAddr(t *testing.T) {
	// First create a listener to occupy a port
	addr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}
	server1, err := New(addr)
	if err != nil {
		t.Fatalf("first New failed: %v", err)
	}
	defer server1.Close()

	// Try to listen on the same port - should fail
	occupiedAddr := server1.listener.Addr().(*net.TCPAddr)
	_, err = New(occupiedAddr)
	if err == nil {
		t.Error("expected error for occupied port")
	}
}

func TestServer_Close(t *testing.T) {
	addr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}
	server, err := New(addr)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	err = server.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Verify listener is closed by trying to accept
	_, err = server.listener.AcceptTCP()
	if err == nil {
		t.Error("expected error after close")
	}
}

func TestServer_Addr(t *testing.T) {
	addr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}
	server, err := New(addr)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer server.Close()

	serverAddr := server.Addr()
	if serverAddr == nil {
		t.Error("Addr returned nil")
	}
}

func TestServer_Serve(t *testing.T) {
	addr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}
	server, err := New(addr)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	handler := newMockHandler()
	ctx, cancel := context.WithCancel(context.Background())

	// Start serving in goroutine
	done := make(chan error, 1)
	go func() {
		done <- server.Serve(ctx, handler)
	}()

	// Give server time to start
	time.Sleep(time.Millisecond * 50)

	// Connect a client
	clientConn, err := net.DialTCP("tcp", nil, server.listener.Addr().(*net.TCPAddr))
	if err != nil {
		t.Fatalf("client dial failed: %v", err)
	}
	defer clientConn.Close()

	// Wait for handler to receive the connection
	select {
	case conn := <-handler.handleCh:
		if conn != nil {
			conn.Close()
		} else {
			t.Error("handler received nil connection")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for handler")
	}

	// Cancel context to stop server
	cancel()

	// Wait for Serve to return
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for Serve to return")
	}
}

func TestServer_Serve_MultipleConnections(t *testing.T) {
	addr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}
	server, err := New(addr)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	handler := newMockHandler()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start serving in goroutine
	go server.Serve(ctx, handler)

	// Give server time to start
	time.Sleep(time.Millisecond * 50)

	// Connect multiple clients
	numClients := 5
	clients := make([]*net.TCPConn, numClients)
	for i := 0; i < numClients; i++ {
		clientConn, err := net.DialTCP("tcp", nil, server.listener.Addr().(*net.TCPAddr))
		if err != nil {
			t.Fatalf("client %d dial failed: %v", i, err)
		}
		clients[i] = clientConn
	}

	// Wait for all handlers to receive connections
	for i := 0; i < numClients; i++ {
		select {
		case conn := <-handler.handleCh:
			if conn == nil {
				t.Errorf("handler %d received nil connection", i)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timeout waiting for handler %d", i)
		}
	}

	// Close all client connections
	for _, conn := range clients {
		conn.Close()
	}

	// Verify handler received all connections
	conns := handler.getConns()
	if len(conns) != numClients {
		t.Errorf("handler received %d connections, want %d", len(conns), numClients)
	}

	// Close handler connections
	for _, conn := range conns {
		conn.Close()
	}
}

func TestServer_Serve_ContextCanceled(t *testing.T) {
	addr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}
	server, err := New(addr)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	handler := newMockHandler()
	ctx, cancel := context.WithCancel(context.Background())

	// Start serving in goroutine
	done := make(chan error, 1)
	go func() {
		done <- server.Serve(ctx, handler)
	}()

	// Give server time to start
	time.Sleep(time.Millisecond * 50)

	// Cancel context
	cancel()

	// Wait for Serve to return
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for Serve to return")
	}
}
