package helpers

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"raven/internal/db"
	"raven/internal/server"
)

// TestIMAPServer wraps an IMAP server for testing
type TestIMAPServer struct {
	Address  string
	Listener net.Listener
	Server   *server.IMAPServer
	done     chan struct{}
	// Test config and auth stub
	configPath string
	authSrv    *http.Server
}

// StartTestIMAPServer starts a test IMAP server on a random port
// Returns the server address and a cleanup function
func StartTestIMAPServer(t *testing.T, dbManager *db.DBManager) *TestIMAPServer {
	t.Helper()

	// Listen on random available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}

	// Create IMAP server
	imapServer := server.NewIMAPServer(dbManager)

	// Generate and set test TLS certificates so STARTTLS is available
	certPath, keyPath, _ := server.GenerateTestCertificates(t)
	imapServer.SetTLSCertificates(certPath, keyPath)

	// Start an auth stub HTTPS server that accepts any credentials
	authMux := http.NewServeMux()
	authMux.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// Create TLS config for auth server using the same test certs
	authTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{{
			Certificate: [][]byte{},
			PrivateKey:  nil,
		}},
	}
	// Load the test certificate for the auth server
	authCert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Fatalf("Failed to load auth server certs: %v", err)
	}
	authTLSConfig.Certificates = []tls.Certificate{authCert}

	authSrv := &http.Server{
		Addr:      "127.0.0.1:0",
		Handler:   authMux,
		TLSConfig: authTLSConfig,
	}

	// Listen on random port for auth server
	authLn, err := net.Listen("tcp", authSrv.Addr)
	if err != nil {
		t.Fatalf("Failed to start auth stub: %v", err)
	}

	// Wrap listener with TLS
	authTLSLn := tls.NewListener(authLn, authTLSConfig)
	go func() { _ = authSrv.Serve(authTLSLn) }()
	authURL := "https://" + authLn.Addr().String() + "/auth"

	// Write temporary config pointing to stub auth server
	cfgDir := filepath.Join("config")
	_ = os.MkdirAll(cfgDir, 0o755)
	cfgPath := filepath.Join(cfgDir, "raven.yaml")
	cfgContent := []byte("domain: localhost\nauth_server_url: " + authURL + "\n")
	if err := os.WriteFile(cfgPath, cfgContent, 0o644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	testServer := &TestIMAPServer{
		Address:    listener.Addr().String(),
		Listener:   listener,
		Server:     imapServer,
		done:       make(chan struct{}),
		configPath: cfgPath,
		authSrv:    authSrv,
	}

	// Start accepting connections
	go func() {
		defer close(testServer.done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				// Server is shutting down
				return
			}
			go imapServer.HandleConnection(conn)
		}
	}()

	t.Logf("Test IMAP server started on %s", testServer.Address)
	return testServer
}

// Stop stops the test IMAP server
func (s *TestIMAPServer) Stop(t *testing.T) {
	t.Helper()

	if s.Listener != nil {
		_ = s.Listener.Close()
	}

	// Wait for server to finish with timeout
	select {
	case <-s.done:
		t.Logf("Test IMAP server stopped")
	case <-time.After(5 * time.Second):
		t.Logf("Warning: Test IMAP server stop timeout")
	}

	// Cleanup test config and stop auth stub
	if s.authSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = s.authSrv.Shutdown(ctx)
		cancel()
	}
	if s.configPath != "" {
		_ = os.Remove(s.configPath)
	}
}

// IMAPClient is a simple IMAP client for integration testing
type IMAPClient struct {
	conn   net.Conn
	reader *bufio.Reader
	tagNum int
}

// NewIMAPClient creates a new IMAP client with the given connection
func NewIMAPClient(conn net.Conn) *IMAPClient {
	return &IMAPClient{
		conn:   conn,
		reader: bufio.NewReader(conn),
		tagNum: 0,
	}
}

// ConnectIMAP creates an IMAP client connection for testing
func ConnectIMAP(t *testing.T, addr string) *IMAPClient {
	t.Helper()

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to connect to IMAP server: %v", err)
	}

	client := &IMAPClient{
		conn:   conn,
		reader: bufio.NewReader(conn),
		tagNum: 0,
	}

	// Read server greeting
	greeting, err := client.ReadLine()
	if err != nil {
		_ = conn.Close()
		t.Fatalf("Failed to read greeting: %v", err)
	}

	if !strings.HasPrefix(greeting, "* OK") {
		_ = conn.Close()
		t.Fatalf("Invalid greeting: %s", greeting)
	}

	// If server indicates LOGIN is disabled on insecure connection, attempt STARTTLS
	if strings.Contains(strings.ToUpper(greeting), "LOGINDISABLED") || strings.Contains(strings.ToUpper(greeting), "STARTTLS") {
		if err := client.StartTLS(); err != nil {
			_ = conn.Close()
			t.Fatalf("Failed to establish TLS via STARTTLS: %v", err)
		}
	}

	t.Logf("IMAP client connected to %s", addr)
	return client
}

// StartTLS upgrades the IMAP connection to TLS using STARTTLS
func (c *IMAPClient) StartTLS() error {
	// Issue STARTTLS command
	responses, err := c.SendCommand("STARTTLS")
	if err != nil {
		return err
	}
	lastLine := responses[len(responses)-1]
	if !strings.Contains(lastLine, "OK") {
		return fmt.Errorf("STARTTLS failed: %s", lastLine)
	}

	// Wrap existing connection with TLS
	tlsConn := tls.Client(c.conn, &tls.Config{InsecureSkipVerify: true})
	if err := tlsConn.Handshake(); err != nil {
		return fmt.Errorf("TLS handshake failed: %v", err)
	}

	c.conn = tlsConn
	c.reader = bufio.NewReader(tlsConn)
	return nil
}

// SendCommand sends an IMAP command and returns the response
func (c *IMAPClient) SendCommand(command string) ([]string, error) {
	c.tagNum++
	tag := fmt.Sprintf("A%03d", c.tagNum)

	// Send command
	line := fmt.Sprintf("%s %s\r\n", tag, command)
	if _, err := c.conn.Write([]byte(line)); err != nil {
		return nil, fmt.Errorf("failed to write command: %v", err)
	}

	// Read response lines
	var responses []string
	for {
		line, err := c.ReadLine()
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %v", err)
		}

		responses = append(responses, line)

		// Check if this is the tagged response
		if strings.HasPrefix(line, tag+" ") {
			break
		}
	}

	return responses, nil
}

// Close closes the IMAP client connection (single implementation)
func (c *IMAPClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Login performs IMAP LOGIN authentication
func (c *IMAPClient) Login(username, password string) error {
	// Proactively attempt STARTTLS first if possible: send CAPABILITY and check STARTTLS
	responses, _ := c.SendCommand("CAPABILITY")
	for _, line := range responses {
		if strings.Contains(strings.ToUpper(line), "STARTTLS") && !isTLSConn(c.conn) {
			// Try STARTTLS before LOGIN
			if err := c.StartTLS(); err != nil {
				// If STARTTLS fails, proceed to attempt LOGIN; server may allow depending on config
			}
			break
		}
	}

	responses, err := c.SendCommand(fmt.Sprintf("LOGIN %s %s", username, password))
	if err != nil {
		return err
	}

	// Check for OK response
	lastLine := responses[len(responses)-1]
	if !strings.Contains(lastLine, "OK") {
		return fmt.Errorf("login failed: %s", lastLine)
	}

	return nil
}

// Select selects an IMAP mailbox
func (c *IMAPClient) Select(mailbox string) error {
	responses, err := c.SendCommand(fmt.Sprintf("SELECT %s", mailbox))
	if err != nil {
		return err
	}

	lastLine := responses[len(responses)-1]
	if !strings.Contains(lastLine, "OK") {
		return fmt.Errorf("select failed: %s", lastLine)
	}

	return nil
}

// List performs IMAP LIST command
func (c *IMAPClient) List(reference, mailbox string) ([]string, error) {
	responses, err := c.SendCommand(fmt.Sprintf("LIST \"%s\" \"%s\"", reference, mailbox))
	if err != nil {
		return nil, err
	}

	// Filter LIST responses
	var mailboxes []string
	for _, line := range responses {
		if strings.HasPrefix(line, "* LIST") {
			mailboxes = append(mailboxes, line)
		}
	}

	return mailboxes, nil
}

// Fetch performs IMAP FETCH command
func (c *IMAPClient) Fetch(sequence, items string) ([]string, error) {
	responses, err := c.SendCommand(fmt.Sprintf("FETCH %s %s", sequence, items))
	if err != nil {
		return nil, err
	}

	// Filter FETCH responses
	var fetches []string
	for _, line := range responses {
		if strings.HasPrefix(line, "* ") && strings.Contains(line, "FETCH") {
			fetches = append(fetches, line)
		}
	}

	return fetches, nil
}

// Store performs IMAP STORE command (flag updates)
func (c *IMAPClient) Store(sequence, flags string) error {
	responses, err := c.SendCommand(fmt.Sprintf("STORE %s %s", sequence, flags))
	if err != nil {
		return err
	}

	lastLine := responses[len(responses)-1]
	if !strings.Contains(lastLine, "OK") {
		return fmt.Errorf("store failed: %s", lastLine)
	}

	return nil
}

// Logout performs IMAP LOGOUT
func (c *IMAPClient) Logout() error {
	_, err := c.SendCommand("LOGOUT")
	if err != nil {
		return err
	}

	return c.Close()
}

// ReadLine reads a single line from the connection
func (c *IMAPClient) ReadLine() (string, error) {
	line, err := c.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// WaitForResponse waits for a specific response pattern
func (c *IMAPClient) WaitForResponse(pattern string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		line, err := c.ReadLine()
		if err != nil {
			return "", err
		}

		if strings.Contains(line, pattern) {
			return line, nil
		}
	}

	return "", fmt.Errorf("timeout waiting for pattern: %s", pattern)
}

// StartTestLMTPServer starts a test LMTP server
// Note: This is a placeholder - actual implementation depends on LMTP server structure
func StartTestLMTPServer(t *testing.T, dbManager *db.DBManager) (addr string, cleanup func()) {
	t.Helper()

	// TODO: Implement LMTP server startup
	// For now, return placeholder
	addr = "127.0.0.1:10024"
	cleanup = func() {
		t.Logf("Test LMTP server cleanup")
	}

	t.Logf("Test LMTP server started on %s (placeholder)", addr)
	return addr, cleanup
}

// DeliverViaLMTP delivers an email message via LMTP
// Note: This is a placeholder - actual implementation depends on LMTP protocol
func DeliverViaLMTP(t *testing.T, addr, recipient string, message []byte) error {
	t.Helper()

	// TODO: Implement LMTP delivery
	// For now, return placeholder
	t.Logf("Delivering email to %s via LMTP at %s (placeholder)", recipient, addr)
	return nil
}

// isTLSConn checks if the underlying connection is already TLS
func isTLSConn(conn net.Conn) bool {
	_, ok := conn.(*tls.Conn)
	return ok
}
