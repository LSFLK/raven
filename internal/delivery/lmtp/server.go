package lmtp

import (
	"database/sql"
	"fmt"
	"log"
	"net"
	"os"
	"sync"

	"go-imap/internal/delivery/config"
	"go-imap/internal/delivery/storage"
)

// Server represents an LMTP server
type Server struct {
	db         *sql.DB
	config     *config.Config
	storage    *storage.Storage
	unixListener net.Listener
	tcpListener  net.Listener
	wg          sync.WaitGroup
	shutdown    chan struct{}
	mu          sync.Mutex
}

// NewServer creates a new LMTP server
func NewServer(db *sql.DB, cfg *config.Config) *Server {
	return &Server{
		db:       db,
		config:   cfg,
		storage:  storage.NewStorage(db),
		shutdown: make(chan struct{}),
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
	listener, err := net.Listen("tcp", s.config.LMTP.TCPAddress)
	if err != nil {
		return err
	}

	s.tcpListener = listener
	log.Printf("LMTP server listening on TCP: %s", s.config.LMTP.TCPAddress)

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
