# socket

A simple, high-performance TCP server framework for Go.

[![Go Reference](https://pkg.go.dev/badge/github.com/Zereker/socket.svg)](https://pkg.go.dev/github.com/Zereker/socket)
[![CI](https://github.com/Zereker/socket/actions/workflows/ci.yml/badge.svg)](https://github.com/Zereker/socket/actions/workflows/ci.yml)

## Features

- **Simple API** - Easy to use with functional options pattern
- **Custom Codec** - Pluggable message encoding/decoding via `io.Reader`
- **Graceful Shutdown** - Context-based cancellation support
- **Idle Timeout** - Automatic read/write deadline management for connection health
- **Error Handling** - Flexible error handling with `Disconnect` or `Continue` actions
- **Structured Logging** - Built-in `slog` integration

## Requirements

- Go 1.23+

## Installation

```bash
go get github.com/Zereker/socket
```

## Quick Start

```go
package main

import (
    "context"
    "io"
    "log"
    "net"

    "github.com/Zereker/socket"
)

// Define your message type
type Message struct {
    Data []byte
}

func (m Message) Length() int  { return len(m.Data) }
func (m Message) Body() []byte { return m.Data }

// Implement the Codec interface
type SimpleCodec struct{}

func (c *SimpleCodec) Decode(r io.Reader) (socket.Message, error) {
    buf := make([]byte, 1024)
    n, err := r.Read(buf)
    if err != nil {
        return nil, err
    }
    return Message{Data: buf[:n]}, nil
}

func (c *SimpleCodec) Encode(msg socket.Message) ([]byte, error) {
    return msg.Body(), nil
}

// Implement the Handler interface
type EchoHandler struct{}

func (h *EchoHandler) Handle(tcpConn *net.TCPConn) {
    conn, err := socket.NewConn(tcpConn,
        socket.CustomCodecOption(&SimpleCodec{}),
        socket.OnMessageOption(func(msg socket.Message) error {
            // Echo the message back
            return conn.Write(msg)
        }),
    )
    if err != nil {
        log.Printf("failed to create connection: %v", err)
        return
    }
    conn.Run(context.Background())
}

func main() {
    addr, _ := net.ResolveTCPAddr("tcp", "127.0.0.1:8080")
    server, err := socket.New(addr)
    if err != nil {
        log.Fatal(err)
    }

    log.Println("server listening on", addr)
    server.Serve(context.Background(), &EchoHandler{})
}
```

## Configuration Options

| Option | Description | Default |
|--------|-------------|---------|
| `CustomCodecOption(codec)` | Set message codec (required) | - |
| `OnMessageOption(handler)` | Set message handler (required) | - |
| `OnErrorOption(handler)` | Set error handler | Disconnect on error |
| `IdleTimeoutOption(duration)` | Set idle timeout for read/write deadlines | 30s |
| `BufferSizeOption(size)` | Set send channel buffer size | 1 |
| `MessageMaxSize(size)` | Set max message size | 1MB |
| `LoggerOption(logger)` | Set custom logger | slog default |

> **Note:** The idle timeout sets TCP read/write deadlines but does not send ping/pong packets. For active connection health checking, implement heartbeat messages in your application protocol.

## Error Handling

Control how errors are handled with `OnErrorOption`:

```go
socket.OnErrorOption(func(err error) socket.ErrorAction {
    if isTemporaryError(err) {
        return socket.Continue  // Suppress error and continue
    }
    return socket.Disconnect    // Close the connection
})
```

## Connection Management

```go
// Gracefully close the connection
conn.Close()

// Check if connection is closed
if conn.IsClosed() {
    // Handle closed connection
}

// Get remote address
addr := conn.Addr()
```

## Write Methods

Three ways to send messages with different blocking behaviors:

```go
// Non-blocking write (fire-and-forget)
// Returns ErrBufferFull immediately if channel is full
// Best for: non-critical data, custom backpressure handling
err := conn.Write(msg)
if errors.Is(err, socket.ErrBufferFull) {
    // Handle backpressure: drop, retry, or use blocking write
}

// Blocking write with context cancellation
// Waits for buffer space, respects context timeout/cancellation
// Best for: critical messages that must be delivered
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
conn.WriteBlocking(ctx, msg)

// Write with timeout
// Waits up to the specified duration for buffer space
// Best for: simple timeout without context management
conn.WriteTimeout(msg, 5*time.Second)
```

All write methods return `ErrConnectionClosed` if the connection is closed.

### Backpressure Handling

When `ErrBufferFull` is returned, it indicates the receiver is not consuming messages fast enough. Strategies:
- **Drop**: Acceptable for metrics, heartbeats, or non-critical updates
- **Retry with backoff**: For important but delay-tolerant messages
- **Switch to blocking**: Use `WriteBlocking` when delivery is critical
- **Flow control**: Implement application-level rate limiting

## Custom Logger

Implement the `Logger` interface or use `slog`:

```go
type Logger interface {
    Debug(msg string, args ...any)
    Info(msg string, args ...any)
    Warn(msg string, args ...any)
    Error(msg string, args ...any)
}
```

## License

MIT License - see [LICENSE](LICENSE) for details.
