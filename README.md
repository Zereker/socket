# socket

A simple, high-performance TCP server framework for Go.

[![Go Reference](https://pkg.go.dev/badge/github.com/Zereker/socket.svg)](https://pkg.go.dev/github.com/Zereker/socket)
[![CI](https://github.com/Zereker/socket/actions/workflows/ci.yml/badge.svg)](https://github.com/Zereker/socket/actions/workflows/ci.yml)

## Features

- **Simple API** - Easy to use with functional options pattern
- **Custom Codec** - Pluggable message encoding/decoding via `io.Reader`
- **Graceful Shutdown** - Context-based cancellation support
- **Heartbeat** - Automatic read/write deadline management
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
| `HeartbeatOption(duration)` | Set heartbeat interval | 30s |
| `BufferSizeOption(size)` | Set send channel buffer size | 1 |
| `MessageMaxSize(size)` | Set max message size | 1MB |
| `LoggerOption(logger)` | Set custom logger | slog default |

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

## Write Methods

Three ways to send messages:

```go
// Non-blocking write, returns ErrBufferFull if channel is full
conn.Write(msg)

// Blocking write with context cancellation
conn.WriteBlocking(ctx, msg)

// Write with timeout
conn.WriteTimeout(msg, 5*time.Second)
```

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
