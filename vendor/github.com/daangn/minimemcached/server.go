package minimemcached

import (
	"fmt"
	"log"
	"net"
)

type server struct {
	l net.Listener
}

// newServer starts and returns a server listening on a given port.
func newServer(port uint16) (*server, error) {
	l, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		log.Printf("failed to listen on port: %d", port)
		return nil, err
	}
	return &server{l: l}, nil
}

// close closes server started with NewServer().
func (s *server) close() {
	if s.l != nil {
		_ = s.l.Close()
		s.l = nil
	}
}
