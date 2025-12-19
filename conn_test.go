package socket

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

// mockMessage implements Message interface for testing
type mockMessage struct {
	body []byte
}

func (m mockMessage) Length() int {
	return len(m.body)
}

func (m mockMessage) Body() []byte {
	return m.body
}

// mockCodec implements Codec interface for testing
type mockCodec struct {
	decodeFunc func(io.Reader) (Message, error)
	encodeFunc func(Message) ([]byte, error)
}

func (c *mockCodec) Decode(r io.Reader) (Message, error) {
	if c.decodeFunc != nil {
		return c.decodeFunc(r)
	}
	buf := make([]byte, 1024)
	n, err := r.Read(buf)
	if err != nil {
		return nil, err
	}
	return mockMessage{body: buf[:n]}, nil
}

func (c *mockCodec) Encode(msg Message) ([]byte, error) {
	if c.encodeFunc != nil {
		return c.encodeFunc(msg)
	}
	return msg.Body(), nil
}

// createTestTCPPair creates a connected pair of TCP connections for testing
func createTestTCPPair(t *testing.T) (*net.TCPConn, *net.TCPConn) {
	t.Helper()

	listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	defer listener.Close()

	// Connect client in goroutine
	clientChan := make(chan *net.TCPConn, 1)
	errChan := make(chan error, 1)
	go func() {
		conn, err := net.DialTCP("tcp", nil, listener.Addr().(*net.TCPAddr))
		if err != nil {
			errChan <- err
			return
		}
		clientChan <- conn
	}()

	// Accept server side
	serverConn, err := listener.AcceptTCP()
	if err != nil {
		t.Fatalf("failed to accept: %v", err)
	}

	select {
	case clientConn := <-clientChan:
		return serverConn, clientConn
	case err := <-errChan:
		serverConn.Close()
		t.Fatalf("client dial failed: %v", err)
		return nil, nil
	case <-time.After(5 * time.Second):
		serverConn.Close()
		t.Fatal("timeout waiting for client connection")
		return nil, nil
	}
}

func TestNewConn(t *testing.T) {
	serverConn, clientConn := createTestTCPPair(t)
	defer serverConn.Close()
	defer clientConn.Close()

	codec := &mockCodec{}
	onMessage := func(msg Message) error { return nil }

	conn, err := NewConn(serverConn,
		CustomCodecOption(codec),
		OnMessageOption(onMessage),
	)

	if err != nil {
		t.Fatalf("NewConn failed: %v", err)
	}

	if conn == nil {
		t.Fatal("NewConn returned nil")
	}

	if conn.rawConn != serverConn {
		t.Error("rawConn not set correctly")
	}
}

func TestNewConn_MissingCodec(t *testing.T) {
	serverConn, clientConn := createTestTCPPair(t)
	defer serverConn.Close()
	defer clientConn.Close()

	onMessage := func(msg Message) error { return nil }

	_, err := NewConn(serverConn,
		OnMessageOption(onMessage),
	)

	if err != ErrInvalidCodec {
		t.Errorf("expected ErrInvalidCodec, got %v", err)
	}
}

func TestNewConn_MissingOnMessage(t *testing.T) {
	serverConn, clientConn := createTestTCPPair(t)
	defer serverConn.Close()
	defer clientConn.Close()

	codec := &mockCodec{}

	_, err := NewConn(serverConn,
		CustomCodecOption(codec),
	)

	if err != ErrInvalidOnMessage {
		t.Errorf("expected ErrInvalidOnMessage, got %v", err)
	}
}

func TestNewConn_WithAllOptions(t *testing.T) {
	serverConn, clientConn := createTestTCPPair(t)
	defer serverConn.Close()
	defer clientConn.Close()

	codec := &mockCodec{}
	onMessage := func(msg Message) error { return nil }
	onError := func(err error) ErrorAction { return Continue }

	conn, err := NewConn(serverConn,
		CustomCodecOption(codec),
		OnMessageOption(onMessage),
		OnErrorOption(onError),
		BufferSizeOption(10),
		HeartbeatOption(time.Minute),
		MessageMaxSize(2048),
	)

	if err != nil {
		t.Fatalf("NewConn failed: %v", err)
	}

	if conn.opts.bufferSize != 10 {
		t.Errorf("bufferSize = %d, want 10", conn.opts.bufferSize)
	}

	if conn.opts.heartbeat != time.Minute {
		t.Errorf("heartbeat = %v, want %v", conn.opts.heartbeat, time.Minute)
	}

	if conn.opts.maxReadLength != 2048 {
		t.Errorf("maxReadLength = %d, want 2048", conn.opts.maxReadLength)
	}
}

func TestCheckOptions_DefaultValues(t *testing.T) {
	codec := &mockCodec{}
	onMessage := func(msg Message) error { return nil }

	opts := &options{
		codec:     codec,
		onMessage: onMessage,
	}

	err := checkOptions(opts)
	if err != nil {
		t.Fatalf("checkOptions failed: %v", err)
	}

	if opts.bufferSize != defaultBufferSize {
		t.Errorf("bufferSize = %d, want %d", opts.bufferSize, defaultBufferSize)
	}

	if opts.maxReadLength != defaultMaxPackageLength {
		t.Errorf("maxReadLength = %d, want %d", opts.maxReadLength, defaultMaxPackageLength)
	}

	if opts.heartbeat != time.Second*30 {
		t.Errorf("heartbeat = %v, want %v", opts.heartbeat, time.Second*30)
	}

	if opts.onError == nil {
		t.Error("onError should have default value")
	}
}

func TestCheckOptions_DefaultOnError(t *testing.T) {
	codec := &mockCodec{}
	onMessage := func(msg Message) error { return nil }

	opts := &options{
		codec:     codec,
		onMessage: onMessage,
	}

	err := checkOptions(opts)
	if err != nil {
		t.Fatalf("checkOptions failed: %v", err)
	}

	// Default onError should return Disconnect
	if opts.onError(errors.New("test")) != Disconnect {
		t.Error("default onError should return Disconnect")
	}
}

func TestConn_Addr(t *testing.T) {
	serverConn, clientConn := createTestTCPPair(t)
	defer serverConn.Close()
	defer clientConn.Close()

	codec := &mockCodec{}
	onMessage := func(msg Message) error { return nil }

	conn, err := NewConn(serverConn,
		CustomCodecOption(codec),
		OnMessageOption(onMessage),
	)
	if err != nil {
		t.Fatalf("NewConn failed: %v", err)
	}

	addr := conn.Addr()
	if addr == nil {
		t.Error("Addr returned nil")
	}
}

func TestConn_Write(t *testing.T) {
	serverConn, clientConn := createTestTCPPair(t)
	defer serverConn.Close()
	defer clientConn.Close()

	codec := &mockCodec{}
	onMessage := func(msg Message) error { return nil }

	conn, err := NewConn(serverConn,
		CustomCodecOption(codec),
		OnMessageOption(onMessage),
		BufferSizeOption(1),
	)
	if err != nil {
		t.Fatalf("NewConn failed: %v", err)
	}

	msg := mockMessage{body: []byte("hello")}
	err = conn.Write(msg)
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}
}

func TestConn_Write_ChannelBlocked(t *testing.T) {
	serverConn, clientConn := createTestTCPPair(t)
	defer serverConn.Close()
	defer clientConn.Close()

	codec := &mockCodec{}
	onMessage := func(msg Message) error { return nil }

	conn, err := NewConn(serverConn,
		CustomCodecOption(codec),
		OnMessageOption(onMessage),
		BufferSizeOption(1),
	)
	if err != nil {
		t.Fatalf("NewConn failed: %v", err)
	}

	msg := mockMessage{body: []byte("hello")}

	// Fill the channel
	err = conn.Write(msg)
	if err != nil {
		t.Fatalf("first Write failed: %v", err)
	}

	// This should fail because channel is blocked
	err = conn.Write(msg)
	if err != ErrBufferFull {
		t.Errorf("expected ErrBufferFull, got %v", err)
	}
}

func TestConn_Write_EncodeError(t *testing.T) {
	serverConn, clientConn := createTestTCPPair(t)
	defer serverConn.Close()
	defer clientConn.Close()

	encodeErr := errors.New("encode error")
	codec := &mockCodec{
		encodeFunc: func(msg Message) ([]byte, error) {
			return nil, encodeErr
		},
	}
	onMessage := func(msg Message) error { return nil }

	conn, err := NewConn(serverConn,
		CustomCodecOption(codec),
		OnMessageOption(onMessage),
	)
	if err != nil {
		t.Fatalf("NewConn failed: %v", err)
	}

	msg := mockMessage{body: []byte("hello")}
	err = conn.Write(msg)
	if err != encodeErr {
		t.Errorf("expected encode error, got %v", err)
	}
}

func TestConn_WriteBlocking(t *testing.T) {
	serverConn, clientConn := createTestTCPPair(t)
	defer serverConn.Close()
	defer clientConn.Close()

	codec := &mockCodec{}
	onMessage := func(msg Message) error { return nil }

	conn, err := NewConn(serverConn,
		CustomCodecOption(codec),
		OnMessageOption(onMessage),
		BufferSizeOption(1),
	)
	if err != nil {
		t.Fatalf("NewConn failed: %v", err)
	}

	msg := mockMessage{body: []byte("hello")}

	// Fill the channel
	err = conn.Write(msg)
	if err != nil {
		t.Fatalf("first Write failed: %v", err)
	}

	// WriteBlocking with canceled context should fail
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = conn.WriteBlocking(ctx, msg)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestConn_WriteTimeout(t *testing.T) {
	serverConn, clientConn := createTestTCPPair(t)
	defer serverConn.Close()
	defer clientConn.Close()

	codec := &mockCodec{}
	onMessage := func(msg Message) error { return nil }

	conn, err := NewConn(serverConn,
		CustomCodecOption(codec),
		OnMessageOption(onMessage),
		BufferSizeOption(1),
	)
	if err != nil {
		t.Fatalf("NewConn failed: %v", err)
	}

	msg := mockMessage{body: []byte("hello")}

	// Fill the channel
	err = conn.Write(msg)
	if err != nil {
		t.Fatalf("first Write failed: %v", err)
	}

	// WriteTimeout should fail after timeout
	err = conn.WriteTimeout(msg, time.Millisecond*10)
	if err != ErrBufferFull {
		t.Errorf("expected ErrBufferFull, got %v", err)
	}
}

func TestConn_Run_ContextCanceled(t *testing.T) {
	serverConn, clientConn := createTestTCPPair(t)
	defer serverConn.Close()
	defer clientConn.Close()

	codec := &mockCodec{}
	onMessage := func(msg Message) error { return nil }

	conn, err := NewConn(serverConn,
		CustomCodecOption(codec),
		OnMessageOption(onMessage),
	)
	if err != nil {
		t.Fatalf("NewConn failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- conn.Run(ctx)
	}()

	// Cancel context
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for Run to complete")
	}
}

func TestConn_Run_ReadWrite(t *testing.T) {
	serverConn, clientConn := createTestTCPPair(t)

	receivedMsg := make(chan []byte, 1)
	codec := &mockCodec{}
	onMessage := func(msg Message) error {
		receivedMsg <- msg.Body()
		return nil
	}

	conn, err := NewConn(serverConn,
		CustomCodecOption(codec),
		OnMessageOption(onMessage),
		HeartbeatOption(time.Second*5),
	)
	if err != nil {
		t.Fatalf("NewConn failed: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- conn.Run(context.Background())
	}()

	// Send data from client
	testData := []byte("hello world")
	_, err = clientConn.Write(testData)
	if err != nil {
		t.Fatalf("client write failed: %v", err)
	}

	// Wait for message
	select {
	case received := <-receivedMsg:
		if string(received) != string(testData) {
			t.Errorf("received = %s, want %s", received, testData)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for message")
	}

	// Close client connection to trigger read error and exit
	clientConn.Close()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for Run to complete")
	}
}

func TestConn_Run_DecodeError(t *testing.T) {
	serverConn, clientConn := createTestTCPPair(t)
	defer clientConn.Close()

	decodeErr := errors.New("decode error")
	codec := &mockCodec{
		decodeFunc: func(r io.Reader) (Message, error) {
			buf := make([]byte, 1024)
			r.Read(buf)
			return nil, decodeErr
		},
	}
	onMessage := func(msg Message) error { return nil }

	conn, err := NewConn(serverConn,
		CustomCodecOption(codec),
		OnMessageOption(onMessage),
		HeartbeatOption(time.Second*5),
	)
	if err != nil {
		t.Fatalf("NewConn failed: %v", err)
	}

	ctx := context.Background()
	done := make(chan error, 1)
	go func() {
		done <- conn.Run(ctx)
	}()

	// Send data to trigger decode
	_, err = clientConn.Write([]byte("test"))
	if err != nil {
		t.Fatalf("client write failed: %v", err)
	}

	select {
	case err := <-done:
		if err != decodeErr {
			t.Errorf("expected decode error, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for Run to complete")
	}
}

func TestConn_Run_OnMessageError(t *testing.T) {
	serverConn, clientConn := createTestTCPPair(t)
	defer clientConn.Close()

	onMessageErr := errors.New("onMessage error")
	codec := &mockCodec{}
	onMessage := func(msg Message) error {
		return onMessageErr
	}

	conn, err := NewConn(serverConn,
		CustomCodecOption(codec),
		OnMessageOption(onMessage),
		HeartbeatOption(time.Second*5),
	)
	if err != nil {
		t.Fatalf("NewConn failed: %v", err)
	}

	ctx := context.Background()
	done := make(chan error, 1)
	go func() {
		done <- conn.Run(ctx)
	}()

	// Send data to trigger onMessage
	_, err = clientConn.Write([]byte("test"))
	if err != nil {
		t.Fatalf("client write failed: %v", err)
	}

	select {
	case err := <-done:
		if err != onMessageErr {
			t.Errorf("expected onMessage error, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for Run to complete")
	}
}

func TestConn_Run_WriteLoop(t *testing.T) {
	serverConn, clientConn := createTestTCPPair(t)
	defer serverConn.Close()

	codec := &mockCodec{}
	onMessage := func(msg Message) error { return nil }

	conn, err := NewConn(serverConn,
		CustomCodecOption(codec),
		OnMessageOption(onMessage),
		HeartbeatOption(time.Second*5),
	)
	if err != nil {
		t.Fatalf("NewConn failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- conn.Run(ctx)
	}()

	// Write message from server side
	msg := mockMessage{body: []byte("server message")}
	err = conn.Write(msg)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Read from client side
	buf := make([]byte, 1024)
	clientConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := clientConn.Read(buf)
	if err != nil {
		t.Fatalf("client read failed: %v", err)
	}

	if string(buf[:n]) != "server message" {
		t.Errorf("received = %s, want 'server message'", buf[:n])
	}

	cancel()
}

func TestConn_Run_ReadError_OnErrorReturnsContinue(t *testing.T) {
	serverConn, clientConn := createTestTCPPair(t)

	codec := &mockCodec{}
	onMessage := func(msg Message) error { return nil }
	onError := func(err error) ErrorAction { return Continue }

	conn, err := NewConn(serverConn,
		CustomCodecOption(codec),
		OnMessageOption(onMessage),
		OnErrorOption(onError),
		HeartbeatOption(time.Millisecond*100),
	)
	if err != nil {
		t.Fatalf("NewConn failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- conn.Run(ctx)
	}()

	// Close client to cause read error
	clientConn.Close()

	// Since onError returns Continue, the error should be suppressed
	// Eventually context will be canceled

	// Give some time for the read to happen
	time.Sleep(time.Millisecond * 200)
	cancel()

	select {
	case <-done:
		// Success - Run completed
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for Run to complete")
	}
}

func TestConn_close(t *testing.T) {
	serverConn, clientConn := createTestTCPPair(t)
	defer clientConn.Close()

	codec := &mockCodec{}
	onMessage := func(msg Message) error { return nil }

	conn, err := NewConn(serverConn,
		CustomCodecOption(codec),
		OnMessageOption(onMessage),
	)
	if err != nil {
		t.Fatalf("NewConn failed: %v", err)
	}

	if err := conn.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Verify IsClosed returns true
	if !conn.IsClosed() {
		t.Error("expected IsClosed to return true after Close")
	}

	// Verify connection is closed by trying to write
	_, err = serverConn.Write([]byte("test"))
	if err == nil {
		t.Error("expected error after close")
	}
}

func TestNewClientConnWithOptions(t *testing.T) {
	serverConn, clientConn := createTestTCPPair(t)
	defer serverConn.Close()
	defer clientConn.Close()

	opts := options{
		codec:         &mockCodec{},
		onMessage:     func(msg Message) error { return nil },
		bufferSize:    5,
		heartbeat:     time.Minute,
		maxReadLength: 4096,
		logger:        defaultLogger(),
	}

	conn := newClientConnWithOptions(serverConn, opts)

	if conn.rawConn != serverConn {
		t.Error("rawConn not set correctly")
	}

	if conn.opts.heartbeat != time.Minute {
		t.Errorf("heartbeat = %v, want %v", conn.opts.heartbeat, time.Minute)
	}

	if cap(conn.sendMsg) != 5 {
		t.Errorf("sendMsg capacity = %d, want 5", cap(conn.sendMsg))
	}
}

func TestConn_WriteLoop_WriteError(t *testing.T) {
	serverConn, clientConn := createTestTCPPair(t)

	codec := &mockCodec{}
	onMessage := func(msg Message) error { return nil }

	conn, err := NewConn(serverConn,
		CustomCodecOption(codec),
		OnMessageOption(onMessage),
		HeartbeatOption(time.Second*5),
	)
	if err != nil {
		t.Fatalf("NewConn failed: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- conn.Run(context.Background())
	}()

	// Give time for Run to start
	time.Sleep(time.Millisecond * 50)

	// Close client to make write fail
	clientConn.Close()

	// Write message - this should eventually trigger write error in writeLoop
	msg := mockMessage{body: []byte("test")}
	conn.Write(msg)

	select {
	case <-done:
		// Run completed due to write error
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for Run to complete")
	}
}

func TestConn_Write_OnErrorReturnsContinue(t *testing.T) {
	serverConn, clientConn := createTestTCPPair(t)

	codec := &mockCodec{}
	onMessage := func(msg Message) error { return nil }
	// onError returns Continue - meaning we don't want to disconnect
	onError := func(err error) ErrorAction { return Continue }

	conn, err := NewConn(serverConn,
		CustomCodecOption(codec),
		OnMessageOption(onMessage),
		OnErrorOption(onError),
		HeartbeatOption(time.Millisecond*100),
	)
	if err != nil {
		t.Fatalf("NewConn failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- conn.Run(ctx)
	}()

	// Give time for Run to start
	time.Sleep(time.Millisecond * 50)

	// Close client to cause write error
	clientConn.Close()

	// Write message - onError returns Continue so error is suppressed
	msg := mockMessage{body: []byte("test")}
	conn.Write(msg)

	// Give time for write to happen
	time.Sleep(time.Millisecond * 200)

	// Cancel context to exit
	cancel()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for Run to complete")
	}
}

func TestConn_WriteLoop_ContextCanceled(t *testing.T) {
	serverConn, clientConn := createTestTCPPair(t)

	codec := &mockCodec{}
	onMessage := func(msg Message) error { return nil }

	conn, err := NewConn(serverConn,
		CustomCodecOption(codec),
		OnMessageOption(onMessage),
		HeartbeatOption(time.Millisecond*100),
	)
	if err != nil {
		t.Fatalf("NewConn failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- conn.Run(ctx)
	}()

	// Give time for Run to start both loops
	time.Sleep(time.Millisecond * 50)

	// Close client to make readLoop exit due to read error
	// This will trigger context cancellation via errgroup
	// Then writeLoop should exit via ctx.Done()
	clientConn.Close()

	// Also cancel context explicitly
	cancel()

	select {
	case <-done:
		// Success - either readLoop or writeLoop exited
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for Run to complete")
	}
}

func TestConn_Write_Success(t *testing.T) {
	serverConn, clientConn := createTestTCPPair(t)
	defer serverConn.Close()

	codec := &mockCodec{}
	onMessage := func(msg Message) error { return nil }

	conn, err := NewConn(serverConn,
		CustomCodecOption(codec),
		OnMessageOption(onMessage),
		HeartbeatOption(time.Second*5),
	)
	if err != nil {
		t.Fatalf("NewConn failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- conn.Run(ctx)
	}()

	// Give time for Run to start
	time.Sleep(time.Millisecond * 50)

	// Write message
	msg := mockMessage{body: []byte("hello")}
	err = conn.Write(msg)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Read from client side to verify write succeeded
	buf := make([]byte, 1024)
	clientConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := clientConn.Read(buf)
	if err != nil {
		t.Fatalf("client read failed: %v", err)
	}

	if string(buf[:n]) != "hello" {
		t.Errorf("received = %s, want 'hello'", buf[:n])
	}

	cancel()
}

func TestConn_writeLoop_Direct(t *testing.T) {
	serverConn, clientConn := createTestTCPPair(t)
	defer clientConn.Close()

	codec := &mockCodec{}
	onMessage := func(msg Message) error { return nil }

	conn, err := NewConn(serverConn,
		CustomCodecOption(codec),
		OnMessageOption(onMessage),
	)
	if err != nil {
		t.Fatalf("NewConn failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- conn.writeLoop(ctx)
	}()

	// Give time for writeLoop to start and block on select
	time.Sleep(time.Millisecond * 50)

	// Cancel context to trigger ctx.Done() case
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for writeLoop to complete")
	}
}

func TestConn_write_Direct(t *testing.T) {
	serverConn, clientConn := createTestTCPPair(t)
	defer serverConn.Close()

	codec := &mockCodec{}
	onMessage := func(msg Message) error { return nil }

	conn, err := NewConn(serverConn,
		CustomCodecOption(codec),
		OnMessageOption(onMessage),
	)
	if err != nil {
		t.Fatalf("NewConn failed: %v", err)
	}

	// Test successful write
	err = conn.write([]byte("test data"))
	if err != nil {
		t.Errorf("write failed: %v", err)
	}

	// Verify data was written
	buf := make([]byte, 1024)
	clientConn.SetReadDeadline(time.Now().Add(time.Second))
	n, err := clientConn.Read(buf)
	if err != nil {
		t.Fatalf("client read failed: %v", err)
	}
	if string(buf[:n]) != "test data" {
		t.Errorf("received = %s, want 'test data'", buf[:n])
	}
}

func TestConn_write_ErrorWithOnErrorContinue(t *testing.T) {
	serverConn, clientConn := createTestTCPPair(t)

	codec := &mockCodec{}
	onMessage := func(msg Message) error { return nil }
	onError := func(err error) ErrorAction { return Continue }

	conn, err := NewConn(serverConn,
		CustomCodecOption(codec),
		OnMessageOption(onMessage),
		OnErrorOption(onError),
	)
	if err != nil {
		t.Fatalf("NewConn failed: %v", err)
	}

	// Close client to cause write error
	clientConn.Close()

	// Write should succeed (return nil) because onError returns Continue
	err = conn.write([]byte("test"))
	if err != nil {
		t.Errorf("write should return nil when onError returns Continue, got %v", err)
	}
}

func TestConn_write_ErrorWithOnErrorDisconnect(t *testing.T) {
	serverConn, clientConn := createTestTCPPair(t)

	codec := &mockCodec{}
	onMessage := func(msg Message) error { return nil }
	// Default onError returns Disconnect

	conn, err := NewConn(serverConn,
		CustomCodecOption(codec),
		OnMessageOption(onMessage),
		HeartbeatOption(time.Millisecond*50),
	)
	if err != nil {
		t.Fatalf("NewConn failed: %v", err)
	}

	// Close both ends to ensure write fails
	clientConn.Close()
	serverConn.Close()

	// Write should return error because connection is closed
	err = conn.write([]byte("test"))
	if err == nil {
		t.Error("write should return error when connection is closed")
	}
}

func TestConn_writeLoop_WriteError_Direct(t *testing.T) {
	serverConn, clientConn := createTestTCPPair(t)

	codec := &mockCodec{}
	onMessage := func(msg Message) error { return nil }

	conn, err := NewConn(serverConn,
		CustomCodecOption(codec),
		OnMessageOption(onMessage),
		HeartbeatOption(time.Millisecond*50),
	)
	if err != nil {
		t.Fatalf("NewConn failed: %v", err)
	}

	// Close both ends to cause write error
	clientConn.Close()
	serverConn.Close()

	// Send a message to the channel
	conn.sendMsg <- []byte("test")

	// Run writeLoop with timeout context
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err = conn.writeLoop(ctx)

	// writeLoop should return with an error (either write error or context timeout)
	if err == nil {
		t.Error("writeLoop should return error when write fails")
	}
}
