package main

import (
	"context"
	"log"
	"net"
	"sync"
	"sync/atomic"
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

type codec struct {
}

func (c *codec) Decode(data []byte) (socket.Message, error) {
	return message{body: data}, nil
}

func (c *codec) Encode(message socket.Message) ([]byte, error) {
	return message.Body(), nil
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
	errorOption := socket.OnErrorOption(func(err error) bool {
		log.Println(err)
		return true
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

	// 阻塞到这块
	if err = newConn.Run(context.Background()); err != nil {
		s.deleteConn(connID)
	}
}

func (s *Server) addConn(connID int64, conn *socket.Conn) {
	s.Lock()
	defer s.Unlock()

	log.Printf("add new conn, connID: %d, addr: %s", connID, conn.Addr())
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
		log.Println(err)
		return
	}

	log.Printf("server start at %s\n", addr.String())
	server.Serve(newHandler(time.Now().Unix()))
}
