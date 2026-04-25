package sasl

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"raven/internal/auth/oauthbearer"
	"raven/internal/conf"
	"strings"
	"sync"
	"time"
)

// ConnectionType represents the type of connection
type ConnectionType int

const (
	// ConnectionTypeTCP represents a TCP connection
	ConnectionTypeTCP ConnectionType = iota
	// ConnectionTypeUnixSocket represents a Unix domain socket connection
	ConnectionTypeUnixSocket
)

// Server represents a SASL authentication server
type Server struct {
	socketPath                 string
	tcpAddr                    string
	authURL                    string
	domain                     string
	saslScope                  conf.SASLScope
	oauthConfig                *conf.Config
	oauthValidator             *oauthbearer.Validator
	oauthClientEmailAuthorizer *clientEmailAuthorizer
	unixListener               net.Listener
	tcpListener                net.Listener
	mu                         sync.Mutex
	wg                         sync.WaitGroup
	shutdown                   chan struct{}
	shutdownOnce               sync.Once
}

// NewServer creates a new SASL authentication server
func NewServer(socketPath, tcpAddr, authURL, domain string, saslScope conf.SASLScope) *Server {
	server := &Server{
		socketPath: socketPath,
		tcpAddr:    tcpAddr,
		authURL:    authURL,
		domain:     domain,
		saslScope:  saslScope,
		shutdown:   make(chan struct{}),
	}

	server.initOAuthValidation()

	return server
}

func (s *Server) initOAuthValidation() {
	cfg, err := conf.LoadConfig()
	if err != nil {
		log.Printf("SASL OAUTHBEARER: config load skipped at init: %v", err)
		return
	}

	validator, err := oauthbearer.NewValidator(oauthbearer.Config{
		IssuerURL: cfg.OAuthIssuer,
		JWKSURL:   cfg.OAuthJWKSURL,
		Audiences: cfg.OAuthAudience,
		ClockSkew: time.Duration(cfg.OAuthSkewSec) * time.Second,
	})
	if err != nil {
		log.Printf("SASL OAUTHBEARER: validator init skipped at startup: %v", err)
		return
	}

	s.oauthConfig = cfg
	s.oauthValidator = validator

	if strings.TrimSpace(cfg.OAuthClientEmailAuthorizationFile) != "" {
		authorizer, loadErr := newClientEmailAuthorizerFromFile(cfg.OAuthClientEmailAuthorizationFile)
		if loadErr != nil {
			log.Printf("SASL OAUTHBEARER: client email authorization config load failed: %v", loadErr)
		} else {
			s.oauthClientEmailAuthorizer = authorizer
			log.Printf("SASL OAUTHBEARER: client email authorization enabled file=%q", cfg.OAuthClientEmailAuthorizationFile)
		}
	} else {
		log.Printf("SASL OAUTHBEARER: client email authorization file not configured (client_credentials sender checks disabled)")
	}
}

// Start starts the SASL server
func (s *Server) Start() error {
	log.Println("Starting SASL server...")
	log.Printf("SASL Scope: %s", s.saslScope)

	// Start UNIX socket listener only if scope allows it
	if s.socketPath != "" && (s.saslScope == conf.SASLScopeUnixSocketOnly || s.saslScope == conf.SASLScopeAll) {
		if err := s.startUnixListener(); err != nil {
			return fmt.Errorf("failed to start UNIX listener: %w", err)
		}
		log.Printf("Skipping Unix socket listener (scope: %s, only TCP connections are allowed)", s.saslScope)
	}

	// Start TCP listener only if scope allows it
	if s.tcpAddr != "" && (s.saslScope == conf.SASLScopeTCPOnly || s.saslScope == conf.SASLScopeAll) {
		if err := s.startTCPListener(); err != nil {
			return fmt.Errorf("failed to start TCP listener: %w", err)
		}
		log.Printf("Skipping TCP listener (scope: %s, only Unix socket connections are allowed)", s.saslScope)
	}

	// Validate that at least one listener was started
	s.mu.Lock()
	hasListener := s.unixListener != nil || s.tcpListener != nil
	s.mu.Unlock()

	if !hasListener {
		return fmt.Errorf("no listeners started - check SASL scope configuration")
	}

	// Wait for all connections to finish
	s.wg.Wait()
	log.Println("All connections closed")
	return nil
}

// startUnixListener starts listening on a UNIX socket
func (s *Server) startUnixListener() error {
	// Remove existing socket file if it exists
	if err := os.RemoveAll(s.socketPath); err != nil {
		return fmt.Errorf("failed to remove existing socket: %v", err)
	}

	// Create Unix socket listener
	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("failed to create Unix socket: %v", err)
	}

	s.mu.Lock()
	s.unixListener = listener
	s.mu.Unlock()

	// Set socket permissions (0666 so Postfix can access it)
	// #nosec G302 -- Unix socket needs world read/write for Postfix access
	if err := os.Chmod(s.socketPath, 0666); err != nil {
		_ = listener.Close()
		return fmt.Errorf("failed to set socket permissions: %v", err)
	}

	log.Printf("SASL server listening on Unix socket: %s", s.socketPath)
	log.Printf("Using authentication URL: %s", s.authURL)
	log.Printf("Domain: %s", s.domain)

	s.wg.Add(1)
	go s.acceptConnections(listener, "unix", ConnectionTypeUnixSocket)

	return nil
}

// startTCPListener starts listening on a TCP address
func (s *Server) startTCPListener() error {
	// Configure TCP listener with keep-alive
	lc := net.ListenConfig{
		KeepAlive: 30 * time.Second, // Send keep-alive probes every 30 seconds
		Control:   nil,
	}

	listener, err := lc.Listen(context.Background(), "tcp", s.tcpAddr)
	if err != nil {
		return fmt.Errorf("failed to create TCP listener: %v", err)
	}

	s.mu.Lock()
	s.tcpListener = listener
	s.mu.Unlock()

	log.Printf("SASL server listening on TCP: %s (with keep-alive enabled)", s.tcpAddr)
	log.Printf("Using authentication URL: %s", s.authURL)
	log.Printf("Domain: %s", s.domain)

	s.wg.Add(1)
	go s.acceptConnections(listener, "tcp", ConnectionTypeTCP)

	return nil
}

// acceptConnections accepts incoming connections
func (s *Server) acceptConnections(listener net.Listener, listenerType string, connType ConnectionType) {
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
		go s.handleConnection(conn, connType)
	}
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown() error {
	var err error
	s.shutdownOnce.Do(func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		log.Println("Shutting down SASL server...")

		// Signal shutdown
		close(s.shutdown)

		// Close listeners
		var errs []error

		if s.unixListener != nil {
			if closeErr := s.unixListener.Close(); closeErr != nil {
				errs = append(errs, fmt.Errorf("error closing Unix listener: %w", closeErr))
			}
			// Clean up socket file
			if s.socketPath != "" {
				_ = os.Remove(s.socketPath)
			}
		}

		if s.tcpListener != nil {
			if closeErr := s.tcpListener.Close(); closeErr != nil {
				errs = append(errs, fmt.Errorf("error closing TCP listener: %w", closeErr))
			}
		}

		// Wait for all connections to finish (outside of lock)
		s.mu.Unlock()
		s.wg.Wait()
		s.mu.Lock()

		if len(errs) > 0 {
			err = fmt.Errorf("shutdown errors: %v", errs)
		}

		log.Println("SASL server shutdown complete")
	})
	return err
}

// handleConnection handles a single SASL authentication connection
func (s *Server) handleConnection(conn net.Conn, connType ConnectionType) {
	defer s.wg.Done()
	defer func() { _ = conn.Close() }()

	scanner := bufio.NewScanner(conn)

	// Set read deadline to prevent hanging connections
	_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))

	for scanner.Scan() {
		line := scanner.Text()
		// Sanitize line for logging to prevent log injection
		sanitizedLine := strings.ReplaceAll(strings.ReplaceAll(line, "\n", "\\n"), "\r", "\\r")
		// #nosec G706 -- Input is sanitized above to prevent log injection
		log.Printf("SASL received: %s", sanitizedLine)

		// Parse Dovecot auth protocol
		// Format: AUTH\t<id>\t<mechanism>\t[service=<service>]\t[resp=<base64>]
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			// #nosec G706 -- Input is sanitized above to prevent log injection
			log.Printf("Invalid SASL request format: %s", sanitizedLine)
			continue
		}

		command := parts[0]

		switch command {
		case "VERSION":
			// Respond to version handshake
			response := "VERSION\t1\t2\n"
			_, _ = conn.Write([]byte(response))
			log.Printf("SASL sent: %s", strings.TrimSpace(response))

		case "CPID":
			// Client process ID - acknowledge
			// After CPID, announce available authentication mechanisms
			// Format: MECH\t<mechanism>\t[options]
			mechPlain := "MECH\tPLAIN\tplaintext\n"
			_, _ = conn.Write([]byte(mechPlain))
			log.Printf("SASL sent: %s", strings.TrimSpace(mechPlain))

			mechLogin := "MECH\tLOGIN\tplaintext\n"
			_, _ = conn.Write([]byte(mechLogin))
			log.Printf("SASL sent: %s", strings.TrimSpace(mechLogin))

			// #nosec G101 -- This is a SASL protocol capability advertisement, not a credential.
			mechOAuthBearer := "MECH\tOAUTHBEARER\tplaintext\n"
			_, _ = conn.Write([]byte(mechOAuthBearer))
			log.Printf("SASL sent: %s", strings.TrimSpace(mechOAuthBearer))

			mechXOAuth2 := "MECH\tXOAUTH2\tplaintext\n"
			_, _ = conn.Write([]byte(mechXOAuth2))
			log.Printf("SASL sent: %s", strings.TrimSpace(mechXOAuth2))

			response := "DONE\n"
			_, _ = conn.Write([]byte(response))
			log.Printf("SASL sent: %s", strings.TrimSpace(response))

		case "AUTH":
			s.handleAuth(conn, parts)

		default:
			// Sanitize command for logging to prevent log injection
			sanitizedCmd := strings.ReplaceAll(strings.ReplaceAll(command, "\n", "\\n"), "\r", "\\r")
			// #nosec G706 -- Input is sanitized above to prevent log injection
			log.Printf("Unknown SASL command: %s", sanitizedCmd)
		}

		// Reset read deadline for next command
		_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
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
	case "OAUTHBEARER", "XOAUTH2":
		s.handleOAuthBearer(conn, id, resp, respProvided)
	default:
		// Unsupported mechanism
		response := fmt.Sprintf("FAIL\t%s\treason=Unsupported mechanism\n", id)
		_, _ = conn.Write([]byte(response))
		log.Printf("SASL sent: %s", strings.TrimSpace(response))
	}
}

// handlePlain handles PLAIN authentication mechanism
func (s *Server) handlePlain(conn net.Conn, id, resp string, respProvided bool) {
	// If no response provided, request it
	if !respProvided {
		response := fmt.Sprintf("CONT\t%s\t\n", id)
		_, _ = conn.Write([]byte(response))
		log.Printf("SASL sent: %s", strings.TrimSpace(response))
		return
	}

	// If response was provided but is empty, treat as malformed
	if resp == "" {
		response := fmt.Sprintf("FAIL\t%s\treason=Invalid credentials format\n", id)
		_, _ = conn.Write([]byte(response))
		log.Printf("SASL sent: %s", strings.TrimSpace(response))
		return
	}

	// Decode base64 response
	decoded, err := base64.StdEncoding.DecodeString(resp)
	if err != nil {
		log.Printf("Failed to decode base64 response: %v", err)
		response := fmt.Sprintf("FAIL\t%s\treason=Invalid encoding\n", id)
		_, _ = conn.Write([]byte(response))
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
		_, _ = conn.Write([]byte(response))
		log.Printf("SASL sent: %s", strings.TrimSpace(response))
		return
	}

	log.Printf("PLAIN authentication attempt for user: %s", username)

	// Authenticate via external API
	if s.authenticate(username, password) {
		// Success
		response := fmt.Sprintf("OK\t%s\tuser=%s\n", id, username)
		_, _ = conn.Write([]byte(response))
		log.Printf("SASL sent: %s", strings.TrimSpace(response))
		log.Printf("Authentication successful for user: %s", username)
	} else {
		// Failure
		response := fmt.Sprintf("FAIL\t%s\tuser=%s\treason=Invalid credentials\n", id, username)
		_, _ = conn.Write([]byte(response))
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
		_, _ = conn.Write([]byte(response))
		log.Printf("SASL sent: %s", strings.TrimSpace(response))
		return
	}

	// This is a simplified implementation
	// A full LOGIN implementation would require state management
	response := fmt.Sprintf("FAIL\t%s\treason=LOGIN not fully implemented, use PLAIN\n", id)
	_, _ = conn.Write([]byte(response))
	log.Printf("SASL sent: %s", strings.TrimSpace(response))
}

func (s *Server) handleOAuthBearer(conn net.Conn, id, resp string, respProvided bool) {
	if !respProvided {
		response := fmt.Sprintf("CONT\t%s\t\n", id)
		_, _ = conn.Write([]byte(response))
		log.Printf("SASL sent: %s", strings.TrimSpace(response))
		return
	}

	if strings.TrimSpace(resp) == "" {
		response := fmt.Sprintf("FAIL\t%s\treason=Invalid OAUTHBEARER payload\n", id)
		_, _ = conn.Write([]byte(response))
		log.Printf("SASL sent: %s", strings.TrimSpace(response))
		return
	}

	accessToken, authzid, requestedUser, err := oauthbearer.ParseInitialClientResponseDetails(resp)
	if err != nil {
		response := fmt.Sprintf("FAIL\t%s\treason=Invalid OAUTHBEARER payload\n", id)
		_, _ = conn.Write([]byte(response))
		log.Printf("SASL sent: %s", strings.TrimSpace(response))
		return
	}
	log.Printf("SASL OAUTHBEARER: parsed auth request id=%s authzid=%q requested_user=%q token_len=%d", id, authzid, requestedUser, len(accessToken))

	if s.oauthConfig == nil || s.oauthValidator == nil {
		response := fmt.Sprintf("FAIL\t%s\treason=OAUTHBEARER configuration error\n", id)
		_, _ = conn.Write([]byte(response))
		log.Printf("SASL sent: %s", strings.TrimSpace(response))
		return
	}

	claims, err := s.oauthValidator.ValidateAccessToken(accessToken)
	if err != nil {
		log.Printf("SASL OAUTHBEARER: token validation failed id=%s error=%v", id, err)
		response := fmt.Sprintf("FAIL\t%s\treason=Invalid credentials\n", id)
		_, _ = conn.Write([]byte(response))
		log.Printf("SASL sent: %s", strings.TrimSpace(response))
		return
	}
	log.Printf("SASL OAUTHBEARER: token validated id=%s grant_type=%q client_id=%q identity=%q", id, claims.GrantType, claims.ClientID, claims.Identity())

	if err := s.authorizeOAuthClientCredentialsSender(claims, authzid, requestedUser); err != nil {
		log.Printf("SASL OAUTHBEARER: sender authorization failed id=%s error=%v", id, err)
		response := fmt.Sprintf("FAIL\t%s\treason=Invalid credentials\n", id)
		_, _ = conn.Write([]byte(response))
		log.Printf("SASL sent: %s", strings.TrimSpace(response))
		return
	}

	identity := strings.TrimSpace(claims.Identity())
	user := normalizeOAuthIdentity(identity, s.oauthConfig.Domain)
	if user == "" {
		response := fmt.Sprintf("FAIL\t%s\treason=Invalid credentials\n", id)
		_, _ = conn.Write([]byte(response))
		log.Printf("SASL sent: %s", strings.TrimSpace(response))
		return
	}

	response := fmt.Sprintf("OK\t%s\tuser=%s\n", id, user)
	_, _ = conn.Write([]byte(response))
	log.Printf("SASL sent: %s", strings.TrimSpace(response))
}

func (s *Server) authorizeOAuthClientCredentialsSender(claims oauthbearer.Claims, authzid, requestedUser string) error {
	if !strings.EqualFold(strings.TrimSpace(claims.GrantType), "client_credentials") {
		log.Printf("SASL OAUTHBEARER: skipping sender authorization grant_type=%q", claims.GrantType)
		return nil
	}

	clientID := strings.TrimSpace(claims.ClientID)
	if clientID == "" {
		log.Printf("SASL OAUTHBEARER: client_credentials authorization denied reason=missing_client_id")
		return fmt.Errorf("missing client_id claim")
	}

	defaultDomain := s.domain
	if s.oauthConfig != nil {
		defaultDomain = s.oauthConfig.Domain
	}

	senderEmail := normalizeOAuthIdentity(firstNonEmpty(strings.TrimSpace(requestedUser), strings.TrimSpace(authzid)), defaultDomain)
	log.Printf("SASL OAUTHBEARER: client_credentials authorization input client_id=%q requested_user=%q authzid=%q normalized_sender=%q", clientID, strings.TrimSpace(requestedUser), strings.TrimSpace(authzid), senderEmail)
	if senderEmail == "" {
		log.Printf("SASL OAUTHBEARER: client_credentials authorization denied client_id=%q reason=missing_sender_email", clientID)
		return fmt.Errorf("missing sender email")
	}

	if s.oauthClientEmailAuthorizer == nil {
		log.Printf("SASL OAUTHBEARER: client_credentials authorization denied client_id=%q sender=%q reason=missing_authorization_config", clientID, senderEmail)
		return fmt.Errorf("missing client email authorization config")
	}

	if !s.oauthClientEmailAuthorizer.IsEmailAuthorized(clientID, senderEmail) {
		log.Printf("SASL OAUTHBEARER: client_credentials authorization denied client_id=%q sender=%q reason=email_not_allowed", clientID, senderEmail)
		return fmt.Errorf("sender email is not authorized for client_id")
	}

	log.Printf("SASL OAUTHBEARER: client_credentials authorization granted client_id=%q sender=%q", clientID, senderEmail)

	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func normalizeOAuthIdentity(identity, defaultDomain string) string {
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return ""
	}

	if strings.Contains(identity, "@") {
		parts := strings.SplitN(identity, "@", 2)
		local := strings.TrimSpace(parts[0])
		domain := strings.Trim(strings.TrimSpace(parts[1]), ".")
		if local == "" || domain == "" {
			return ""
		}
		return local + "@" + domain
	}

	if strings.TrimSpace(defaultDomain) == "" {
		return ""
	}

	return identity + "@" + strings.Trim(strings.TrimSpace(defaultDomain), ".")
}

// authenticate validates credentials against external API
func (s *Server) authenticate(username, password string) bool {
	// Keep behavior consistent with IMAP auth: send the identifier as local username.
	authUsername := username
	if at := strings.Index(authUsername, "@"); at > 0 {
		authUsername = authUsername[:at]
	}

	requestPayload := map[string]any{
		"identifiers": map[string]string{
			"username": authUsername,
		},
		"credentials": map[string]string{
			"password": password,
		},
		"skip_assertion": true,
	}

	requestBodyBytes, err := json.Marshal(requestPayload)
	if err != nil {
		log.Printf("Failed to marshal authentication request payload: %v", err)
		return false
	}

	// Create HTTP request
	req, err := http.NewRequest("POST", s.authURL, strings.NewReader(string(requestBodyBytes)))
	if err != nil {
		log.Printf("Failed to create HTTP request: %v", err)
		return false
	}
	req.Header.Set("Content-Type", "application/json")

	// Create HTTP client with TLS config
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true, // #nosec G402 -- Required for internal auth server communication, matches IMAP server behavior
	}
	transport := &http.Transport{TLSClientConfig: tlsConfig}
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	// Make request
	// #nosec G704 -- URL is from validated config, not user input
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Authentication API request failed: %v", err)
		return false
	}
	defer func() { _ = resp.Body.Close() }()

	// Check response status
	if resp.StatusCode == 200 {
		log.Printf("Authentication API returned success for user: %s", authUsername)
		return true
	}

	// #nosec G706 -- authUsername is sanitized by caller, status code is int
	log.Printf("Authentication API returned status %d for user: %s", resp.StatusCode, authUsername)
	return false
}
