package lmtp

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"raven/internal/db"
	"raven/internal/delivery/config"
	"raven/internal/delivery/storage"
)

// Server represents an LMTP server
type Server struct {
	dbManager    *db.DBManager
	config       *config.Config
	storage      *storage.Storage
	unixListener net.Listener
	tcpListener  net.Listener
	wg           sync.WaitGroup
	shutdown     chan struct{}
	mu           sync.Mutex
}

// NewServer creates a new LMTP server
func NewServer(dbManager *db.DBManager, cfg *config.Config) *Server {
	return &Server{
		dbManager: dbManager,
		config:    cfg,
		storage:   storage.NewStorage(dbManager),
		shutdown:  make(chan struct{}),
	}
}

// Start starts the LMTP server on configured listeners
func (s *Server) Start() error {
	log.Println("Starting LMTP server...")

	// Start UNIX socket listener if configured
	if s.config.LMTP.UnixSocket != "" {
		if err := s.startUnixListener(); err != nil {
			return fmt.Errorf("failed to start UNIX listener: %w", err)
		}
	}

	// Start TCP listener if configured
	if s.config.LMTP.TCPAddress != "" {
		if err := s.startTCPListener(); err != nil {
			return fmt.Errorf("failed to start TCP listener: %w", err)
		}
	}

	// Wait for all connections to finish
	s.wg.Wait()
	log.Println("All connections closed")
	return nil
}

// startUnixListener starts listening on a UNIX socket
func (s *Server) startUnixListener() error {
	// Remove existing socket file if it exists
	os.Remove(s.config.LMTP.UnixSocket)

	listener, err := net.Listen("unix", s.config.LMTP.UnixSocket)
	if err != nil {
		return err
	}

	s.unixListener = listener
	log.Printf("LMTP server listening on UNIX socket: %s", s.config.LMTP.UnixSocket)

	// Set socket permissions
	if err := os.Chmod(s.config.LMTP.UnixSocket, 0666); err != nil {
		log.Printf("Warning: failed to set socket permissions: %v", err)
	}

	s.wg.Add(1)
	go s.acceptConnections(listener, "unix")

	return nil
}

// startTCPListener starts listening on a TCP address
func (s *Server) startTCPListener() error {
	// Configure TCP listener with keep-alive
	lc := net.ListenConfig{
		KeepAlive: 30 * time.Second, // Send keep-alive probes every 30 seconds
		Control:   nil,
	}

	listener, err := lc.Listen(context.Background(), "tcp", s.config.LMTP.TCPAddress)
	if err != nil {
		return err
	}

	s.tcpListener = listener
	log.Printf("LMTP server listening on TCP: %s (with keep-alive enabled)", s.config.LMTP.TCPAddress)

	s.wg.Add(1)
	go s.acceptConnections(listener, "tcp")

	return nil
}

// acceptConnections accepts incoming connections
func (s *Server) acceptConnections(listener net.Listener, listenerType string) {
	defer s.wg.Done()

	for {
		select {
		case <-s.shutdown:
			log.Printf("Stopping %s listener...", listenerType)
			return
		default:
		}

		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-s.shutdown:
				return
			default:
				log.Printf("Accept error on %s listener: %v", listenerType, err)
				continue
			}
		}

		log.Printf("New %s connection from: %s", listenerType, conn.RemoteAddr())

		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

// handleConnection handles a single LMTP connection
func (s *Server) handleConnection(conn net.Conn) {
	defer s.wg.Done()
	defer conn.Close()

	// Configure TCP options for better connection stability
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		// Enable TCP keep-alive to detect dead connections
		if err := tcpConn.SetKeepAlive(true); err != nil {
			log.Printf("Warning: failed to enable keep-alive: %v", err)
		}

		// Set keep-alive period to 30 seconds
		if err := tcpConn.SetKeepAlivePeriod(30 * time.Second); err != nil {
			log.Printf("Warning: failed to set keep-alive period: %v", err)
		}

		// Disable Nagle's algorithm for better small packet handling (LMTP protocol)
		if err := tcpConn.SetNoDelay(true); err != nil {
			log.Printf("Warning: failed to set TCP_NODELAY: %v", err)
		}

		log.Printf("TCP options configured for connection from %s", conn.RemoteAddr())
	}

	session := NewSession(conn, s.storage, s.config)
	if err := session.Handle(); err != nil {
		log.Printf("Session error from %s: %v", conn.RemoteAddr(), err)
	}

	log.Printf("Connection closed: %s", conn.RemoteAddr())
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.Println("Shutting down LMTP server...")

	// Signal shutdown
	close(s.shutdown)

	// Close listeners
	var errs []error

	if s.unixListener != nil {
		if err := s.unixListener.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing UNIX listener: %w", err))
		}
		// Clean up socket file
		if s.config.LMTP.UnixSocket != "" {
			os.Remove(s.config.LMTP.UnixSocket)
		}
	}

	if s.tcpListener != nil {
		if err := s.tcpListener.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing TCP listener: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %v", errs)
	}

	log.Println("LMTP server shutdown complete")
	return nil
}
