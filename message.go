package socket

import "io"

// Message is the interface for messages transmitted over the connection.
// Implementations should provide the message length and body.
type Message interface {
	// Length returns the length of the message body.
	Length() int
	// Body returns the raw message data.
	Body() []byte
}

// Codec is the interface for message encoding and decoding.
// Applications should implement this interface to define their own
// message serialization format (e.g., JSON, Protocol Buffers, etc.).
//
// The Decode method reads from an io.Reader, which allows the codec to handle
// TCP stream reassembly by reading exactly the number of bytes needed for
// a complete message. This solves the TCP packet fragmentation problem.
type Codec interface {
	// Decode reads and decodes a complete message from the reader.
	// The implementation should read exactly the bytes needed for one message.
	// This handles TCP packet fragmentation by allowing the codec to control
	// how many bytes are read.
	Decode(r io.Reader) (Message, error)
	// Encode encodes a Message into raw bytes for transmission.
	Encode(Message) ([]byte, error)
}
