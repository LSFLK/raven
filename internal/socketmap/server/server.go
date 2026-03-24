package server

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"raven/internal/socketmap/cache"
	"raven/internal/socketmap/config"
	"raven/internal/socketmap/handler"
	"raven/internal/socketmap/protocol"
)

// Server represents the socketmap TCP server
type Server struct {
	cfg         *config.Config
	cache       *cache.Cache
	listener    net.Listener
	activeConns sync.WaitGroup
}

// New creates a new Server instance
func New(cfg *config.Config, cacheManager *cache.Cache) *Server {
	return &Server{
		cfg:   cfg,
		cache: cacheManager,
	}
}

// Start starts the TCP server
func (s *Server) Start() error {
	bindAddr := fmt.Sprintf("%s:%s", s.cfg.Host, s.cfg.Port)

	listener, err := net.Listen("tcp", bindAddr)
	if err != nil {
		return fmt.Errorf("failed to start listener: %w", err)
	}

	s.listener = listener
	log.Printf("✓ Socketmap service listening on %s", bindAddr)
	log.Println("Ready to accept connections from Postfix")
	log.Println("")

	// Accept connections
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("⚠ Error accepting connection: %v", err)
			continue
		}

		log.Printf("╔════════════════════════════════════════════════╗")
		log.Printf("║ New connection from %s", conn.RemoteAddr())
		log.Printf("╚════════════════════════════════════════════════╝")

		// Handle connection in goroutine
		s.activeConns.Add(1)
		go func() {
			defer s.activeConns.Done()
			s.handleConnection(conn)
		}()
	}
}

// handleConnection handles a single client connection
func (s *Server) handleConnection(conn net.Conn) {
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("⚠ Error closing connection %s: %v", conn.RemoteAddr(), err)
		}
	}()
	defer log.Printf("Connection closed: %s", conn.RemoteAddr())

	log.Printf("  Connection established, using netstring protocol...")
	reader := bufio.NewReader(conn)

	for {
		// Set read timeout to prevent hanging connections
		if err := conn.SetReadDeadline(time.Now().Add(30 * time.Second)); err != nil {
			log.Printf("⚠ Error setting read deadline for %s: %v", conn.RemoteAddr(), err)
			return
		}

		log.Printf("  Waiting to read netstring from connection...")

		// Read request using netstring protocol
		request, err := protocol.ReadNetstring(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				log.Printf("  Connection closed by client (EOF)")
			} else {
				log.Printf("⚠ Error reading netstring from %s: %v", conn.RemoteAddr(), err)
				log.Printf("  Possible causes:")
				log.Printf("  1. Client sent non-netstring data")
				log.Printf("  2. Connection interrupted")
				log.Printf("  3. Protocol version mismatch")
			}
			return
		}

		// Log raw request received
		log.Printf("← Received netstring: %q (length: %d)", request, len(request))

		if request == "" {
			log.Printf("⚠ Received empty request, skipping...")
			continue
		}

		log.Printf("  Processing request: %q", request)

		// Process the request
		response := handler.ProcessRequest(request, s.cfg, s.cache)
		log.Printf("→ Preparing response: %q", response)

		// Send response using netstring protocol
		if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
			log.Printf("⚠ Error setting write deadline for %s: %v", conn.RemoteAddr(), err)
			return
		}
		err = protocol.WriteNetstring(conn, response)
		if err != nil {
			log.Printf("⚠ Error writing netstring to %s: %v", conn.RemoteAddr(), err)
			return
		}
		log.Printf("  Successfully sent netstring response (length: %d)", len(response))
	}
}

// Close gracefully shuts down the server
func (s *Server) Close() error {
	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			return fmt.Errorf("failed to close listener: %w", err)
		}
	}
	s.activeConns.Wait()
	return nil
}
