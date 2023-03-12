package socket

import (
	"net"
	"runtime"
)

type Handler interface {
	Handle(conn *net.TCPConn)
}

type server struct {
	listener *net.TCPListener
}

func New(addr *net.TCPAddr) (*server, error) {
	listener, err := net.ListenTCP(addr.Network(), addr)
	if err != nil {
		return nil, err
	}

	return &server{
		listener: listener,
	}, nil
}

func (s *server) Serve(handler Handler) {
	for {
		conn, err := s.listener.AcceptTCP()
		if err != nil {
			runtime.Gosched()
			continue
		}

		go handler.Handle(conn)
	}
}

func (s *server) Close() error {
	return s.listener.Close()
}
