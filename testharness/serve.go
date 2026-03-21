//go:build !js

package testharness

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
)

// Server serves test files with COOP/COEP headers required for SharedArrayBuffer.
type Server struct {
	ln   net.Listener
	dir  string
	addr string
}

// NewServer creates a test server for the given directory on a random port.
func NewServer(dir string) (*Server, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("testharness: invalid server dir: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("testharness: %s is not a directory", dir)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("testharness: listen: %w", err)
	}
	return &Server{ln: ln, dir: dir, addr: ln.Addr().String()}, nil
}

// Addr returns the server's listen address (e.g. "127.0.0.1:54321").
func (s *Server) Addr() string { return s.addr }

// URL returns the full server URL.
func (s *Server) URL() string { return "http://" + s.addr }

// Start begins serving in a goroutine. Call Close to stop.
func (s *Server) Start() {
	fs := http.FileServer(http.Dir(s.dir))
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
		fs.ServeHTTP(w, r)
	})
	go func() {
		if err := http.Serve(s.ln, handler); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Printf("testharness: serve error: %v", err)
		}
	}()
}

// Close stops the server.
func (s *Server) Close() error { return s.ln.Close() }
