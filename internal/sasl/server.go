package sasl

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Server represents a SASL authentication server
type Server struct {
	socketPath    string
	authURL       string
	domain        string
	listener      net.Listener
	wg            sync.WaitGroup
	shutdown      chan struct{}
	shutdownOnce  sync.Once
}

// NewServer creates a new SASL authentication server
func NewServer(socketPath, authURL, domain string) *Server {
	return &Server{
		socketPath: socketPath,
		authURL:    authURL,
		domain:     domain,
		shutdown:   make(chan struct{}),
	}
}

// Start starts the SASL server
func (s *Server) Start() error {
	// Remove existing socket file if it exists
	if err := os.RemoveAll(s.socketPath); err != nil {
		return fmt.Errorf("failed to remove existing socket: %v", err)
	}

	// Create Unix socket listener
	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("failed to create Unix socket: %v", err)
	}
	s.listener = listener

	// Set socket permissions (0666 so Postfix can access it)
	if err := os.Chmod(s.socketPath, 0666); err != nil {
		listener.Close()
		return fmt.Errorf("failed to set socket permissions: %v", err)
	}

	log.Printf("SASL server listening on Unix socket: %s", s.socketPath)
	log.Printf("Using authentication URL: %s", s.authURL)
	log.Printf("Domain: %s", s.domain)

	// Accept connections
	for {
		select {
		case <-s.shutdown:
			return nil
		default:
		}

		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-s.shutdown:
				return nil
			default:
				log.Printf("Accept error: %v", err)
				continue
			}
		}

		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown() error {
	var err error
	s.shutdownOnce.Do(func() {
		close(s.shutdown)
		if s.listener != nil {
			err = s.listener.Close()
		}
		s.wg.Wait()
		os.Remove(s.socketPath)
	})
	return err
}

// handleConnection handles a single SASL authentication connection
func (s *Server) handleConnection(conn net.Conn) {
	defer s.wg.Done()
	defer conn.Close()

	scanner := bufio.NewScanner(conn)

	// Set read deadline to prevent hanging connections
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))

	for scanner.Scan() {
		line := scanner.Text()
		log.Printf("SASL received: %s", line)

		// Parse Dovecot auth protocol
		// Format: AUTH\t<id>\t<mechanism>\t[service=<service>]\t[resp=<base64>]
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			log.Printf("Invalid SASL request format: %s", line)
			continue
		}

		command := parts[0]

		switch command {
		case "VERSION":
			// Respond to version handshake
			response := "VERSION\t1\t2\n"
			conn.Write([]byte(response))
			log.Printf("SASL sent: %s", strings.TrimSpace(response))

		case "CPID":
			// Client process ID - acknowledge
			response := "DONE\n"
			conn.Write([]byte(response))
			log.Printf("SASL sent: %s", strings.TrimSpace(response))

		case "AUTH":
			s.handleAuth(conn, parts)

		default:
			log.Printf("Unknown SASL command: %s", command)
		}

		// Reset read deadline for next command
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Scanner error: %v", err)
	}
}

// handleAuth handles authentication requests
func (s *Server) handleAuth(conn net.Conn, parts []string) {
	// AUTH format: AUTH\t<id>\t<mechanism>\t[service=<service>]\t[resp=<base64>]
	// Example: AUTH	1	PLAIN	service=smtp	resp=AHRlc3RAdGVzdC5jb20AdGVzdDEyMw==

	if len(parts) < 3 {
		log.Printf("Invalid AUTH command format, parts: %d", len(parts))
		return
	}

	id := parts[1]
	mechanism := parts[2]

	log.Printf("AUTH request: id=%s, mechanism=%s", id, mechanism)

	// Parse additional parameters
	var service, resp string
	var respProvided bool
	for i := 3; i < len(parts); i++ {
		if strings.HasPrefix(parts[i], "service=") {
			service = strings.TrimPrefix(parts[i], "service=")
		} else if strings.HasPrefix(parts[i], "resp=") {
			resp = strings.TrimPrefix(parts[i], "resp=")
			respProvided = true
		}
	}

	log.Printf("Service: %s, Response present: %v", service, respProvided)

	switch strings.ToUpper(mechanism) {
	case "PLAIN":
		s.handlePlain(conn, id, resp, respProvided)
	case "LOGIN":
		s.handleLogin(conn, id, resp)
	default:
		// Unsupported mechanism
		response := fmt.Sprintf("FAIL\t%s\treason=Unsupported mechanism\n", id)
		conn.Write([]byte(response))
		log.Printf("SASL sent: %s", strings.TrimSpace(response))
	}
}

// handlePlain handles PLAIN authentication mechanism
func (s *Server) handlePlain(conn net.Conn, id, resp string, respProvided bool) {
	// If no response provided, request it
	if !respProvided {
		response := fmt.Sprintf("CONT\t%s\t\n", id)
		conn.Write([]byte(response))
		log.Printf("SASL sent: %s", strings.TrimSpace(response))
		return
	}

	// If response was provided but is empty, treat as malformed
	if resp == "" {
		response := fmt.Sprintf("FAIL\t%s\treason=Invalid credentials format\n", id)
		conn.Write([]byte(response))
		log.Printf("SASL sent: %s", strings.TrimSpace(response))
		return
	}

	// Decode base64 response
	decoded, err := base64.StdEncoding.DecodeString(resp)
	if err != nil {
		log.Printf("Failed to decode base64 response: %v", err)
		response := fmt.Sprintf("FAIL\t%s\treason=Invalid encoding\n", id)
		conn.Write([]byte(response))
		log.Printf("SASL sent: %s", strings.TrimSpace(response))
		return
	}

	// PLAIN format: [authzid]\x00authcid\x00password
	parts := strings.Split(string(decoded), "\x00")

	var username, password string
	if len(parts) >= 3 {
		// Format: authzid\x00username\x00password
		username = parts[1]
		password = parts[2]
	} else if len(parts) == 2 {
		// Format: username\x00password
		username = parts[0]
		password = parts[1]
	} else {
		log.Printf("Invalid PLAIN format, parts: %d", len(parts))
		response := fmt.Sprintf("FAIL\t%s\treason=Invalid credentials format\n", id)
		conn.Write([]byte(response))
		log.Printf("SASL sent: %s", strings.TrimSpace(response))
		return
	}

	log.Printf("PLAIN authentication attempt for user: %s", username)

	// Authenticate via external API
	if s.authenticate(username, password) {
		// Success
		response := fmt.Sprintf("OK\t%s\tuser=%s\n", id, username)
		conn.Write([]byte(response))
		log.Printf("SASL sent: %s", strings.TrimSpace(response))
		log.Printf("Authentication successful for user: %s", username)
	} else {
		// Failure
		response := fmt.Sprintf("FAIL\t%s\tuser=%s\treason=Invalid credentials\n", id, username)
		conn.Write([]byte(response))
		log.Printf("SASL sent: %s", strings.TrimSpace(response))
		log.Printf("Authentication failed for user: %s", username)
	}
}

// handleLogin handles LOGIN authentication mechanism
func (s *Server) handleLogin(conn net.Conn, id, resp string) {
	// LOGIN is a two-step process
	// Step 1: Request username
	// Step 2: Request password

	// For simplicity, we'll treat it similar to PLAIN for now
	// In a full implementation, you'd need to maintain state between requests

	if resp == "" {
		// Request username
		response := fmt.Sprintf("CONT\t%s\tUsername:\n", id)
		conn.Write([]byte(response))
		log.Printf("SASL sent: %s", strings.TrimSpace(response))
		return
	}

	// This is a simplified implementation
	// A full LOGIN implementation would require state management
	response := fmt.Sprintf("FAIL\t%s\treason=LOGIN not fully implemented, use PLAIN\n", id)
	conn.Write([]byte(response))
	log.Printf("SASL sent: %s", strings.TrimSpace(response))
}

// authenticate validates credentials against external API
func (s *Server) authenticate(username, password string) bool {
	// Construct email address
	email := username
	if !strings.Contains(username, "@") {
		email = username + "@" + s.domain
	}

	// Prepare JSON request
	requestBody := fmt.Sprintf(`{"email":"%s","password":"%s"}`, email, password)

	// Create HTTP request
	req, err := http.NewRequest("POST", s.authURL, strings.NewReader(requestBody))
	if err != nil {
		log.Printf("Failed to create HTTP request: %v", err)
		return false
	}
	req.Header.Set("Content-Type", "application/json")

	// Create HTTP client with TLS config
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true, // Match IMAP server behavior
	}
	transport := &http.Transport{TLSClientConfig: tlsConfig}
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	// Make request
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Authentication API request failed: %v", err)
		return false
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode == 200 {
		log.Printf("Authentication API returned success for user: %s", email)
		return true
	}

	log.Printf("Authentication API returned status %d for user: %s", resp.StatusCode, email)
	return false
}
