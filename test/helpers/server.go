package helpers

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"raven/internal/db"
	"raven/internal/delivery/config"
	"raven/internal/delivery/lmtp"
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
				// Log the error for debugging but don't fail the test
				_ = err // Explicitly acknowledge we're ignoring the error
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
func StartTestLMTPServer(t *testing.T, dbManager *db.DBManager) (addr string, srv *lmtp.Server, cleanup func()) {
	t.Helper()

	cfg := &config.Config{}
	cfg.LMTP.TCPAddress = "127.0.0.1:0"
	cfg.LMTP.UnixSocket = ""
	cfg.LMTP.Hostname = "localhost"
	cfg.LMTP.MaxSize = 1024 * 1024
	cfg.LMTP.MaxRecipients = 50
	cfg.Delivery.DefaultFolder = "INBOX"
	cfg.Delivery.AllowedDomains = []string{"example.com"}
	cfg.Delivery.RejectUnknownUser = true

	srv = lmtp.NewServer(dbManager, cfg)

	go func() { _ = srv.Start() }()

	// Wait until TCP listener is ready
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if ln := srv.TCPAddr(); ln != nil {
			addr = ln.String()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if addr == "" {
		t.Fatalf("LMTP TCP listener did not start")
	}

	cleanup = func() { _ = srv.Shutdown() }
	t.Logf("Test LMTP server started on %s", addr)
	return addr, srv, cleanup
}

// LMTPClient is a simple client to speak LMTP for tests
type LMTPClient struct {
	conn   net.Conn
	reader *bufio.Reader
}

func ConnectLMTP(t *testing.T, addr string) *LMTPClient {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to connect LMTP: %v", err)
	}
	c := &LMTPClient{conn: conn, reader: bufio.NewReader(conn)}
	// Read greeting
	if _, err := c.ReadLine(); err != nil {
		_ = conn.Close()
		t.Fatalf("Failed to read LMTP greeting: %v", err)
	}
	return c
}

func (c *LMTPClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
func (c *LMTPClient) ReadLine() (string, error) {
	line, err := c.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
func (c *LMTPClient) SendLine(line string) error {
	_, err := c.conn.Write([]byte(line + "\r\n"))
	return err
}

func (c *LMTPClient) LHLO(domain string) ([]string, error) {
	_ = c.SendLine("LHLO " + domain)
	return c.readUntilStatus()
}
func (c *LMTPClient) MAILFROM(addr string) ([]string, error) {
	_ = c.SendLine("MAIL FROM:<" + addr + ">")
	return c.readUntilStatus()
}
func (c *LMTPClient) RCPTTO(addr string) ([]string, error) {
	_ = c.SendLine("RCPT TO:<" + addr + ">")
	lines, err := c.readUntilStatus()
	if err != nil {
		return nil, err
	}
	// Check if the final line indicates error (5xx or 4xx)
	if len(lines) > 0 {
		lastLine := lines[len(lines)-1]
		if strings.HasPrefix(lastLine, "5") || strings.HasPrefix(lastLine, "4") {
			return lines, fmt.Errorf("RCPT failed: %s", lastLine)
		}
	}
	return lines, nil
}
func (c *LMTPClient) DATA(body []byte) ([]string, error) {
	// Send DATA command and expect 354 intermediate response
	if err := c.SendLine("DATA"); err != nil {
		return nil, err
	}
	// Read 354 line
	line, err := c.ReadLine()
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(line, "354") {
		// return the line as part of responses
		return []string{line}, fmt.Errorf("expected 354, got: %s", line)
	}
	// Send body followed by CRLF . CRLF terminator
	// Ensure body ends with CRLF
	if !strings.HasSuffix(string(body), "\r\n") {
		body = append(body, '\r', '\n')
	}
	if _, err := c.conn.Write(body); err != nil {
		return nil, err
	}
	if err := c.SendLine("."); err != nil {
		return nil, err
	}
	// Read final status lines until status
	return c.readUntilStatus()
}
func (c *LMTPClient) QUIT() ([]string, error) { _ = c.SendLine("QUIT"); return c.readUntilStatus() }

func (c *LMTPClient) readUntilStatus() ([]string, error) {
	var lines []string
	for {
		line, err := c.ReadLine()
		if err != nil {
			return nil, err
		}
		lines = append(lines, line)
		// Handle multiline capability (e.g., 250-... followed by final 250 ...)
		if strings.HasPrefix(line, "250-") {
			// continuation line; keep reading
			continue
		}
		// Terminal statuses without continuation
		if strings.HasPrefix(line, "250 ") || strings.HasPrefix(line, "2") || strings.HasPrefix(line, "4") || strings.HasPrefix(line, "5") || strings.Contains(line, "OK") || strings.Contains(line, "ERROR") {
			break
		}
	}
	return lines, nil
}

// isTLSConn checks if the underlying connection is already TLS
func isTLSConn(conn net.Conn) bool {
	_, ok := conn.(*tls.Conn)
	return ok
}

// SASL testing support structures and functions

// SASLClient represents a SASL client connection for testing
type SASLClient struct {
	conn   net.Conn
	reader *bufio.Reader
	closed bool
	mu     sync.Mutex
}

// ConnectSASL creates a new SASL client connection to the specified Unix socket
func ConnectSASL(t *testing.T, socketPath string) *SASLClient {
	t.Helper()

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to connect to SASL socket %s: %v", socketPath, err)
	}

	return &SASLClient{
		conn:   conn,
		reader: bufio.NewReader(conn),
		closed: false,
	}
}

// SendCommand sends a command to the SASL server
func (c *SASLClient) SendCommand(command string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	_, err := c.conn.Write([]byte(command + "\n"))
	if err != nil {
		// Connection might be closed, ignore error
		_ = err // Explicitly acknowledge we're ignoring the error
	}
}

// ReadResponse reads a single response line from the SASL server
func (c *SASLClient) ReadResponse() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return ""
	}

	line, err := c.reader.ReadString('\n')
	if err != nil {
		return ""
	}

	return strings.TrimRight(line, "\r\n")
}

// ReadMultipleResponses reads multiple response lines until DONE or timeout
func (c *SASLClient) ReadMultipleResponses() []string {
	var responses []string
	timeout := time.After(2 * time.Second)

	for {
		select {
		case <-timeout:
			return responses
		default:
			response := c.ReadResponse()
			if response == "" {
				return responses
			}
			responses = append(responses, response)
			if strings.HasPrefix(response, "DONE") {
				return responses
			}
		}
	}
}

// Close closes the SASL client connection
func (c *SASLClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Mock authentication server helpers

// MockAuthServer represents a mock authentication server for testing
type MockAuthServer struct {
	*httptest.Server
	Requests []MockAuthRequest
	mu       sync.Mutex
}

// MockAuthRequest captures details of an authentication request
type MockAuthRequest struct {
	Email    string
	Password string
	Headers  http.Header
	Time     time.Time
}

// SetupMockAuthServer creates a mock authentication server that accepts any credentials
func SetupMockAuthServer(t *testing.T) *MockAuthServer {
	return SetupMockAuthServerWithResponse(t, http.StatusOK, `{"status":"ok"}`)
}

// SetupMockAuthServerWithResponse creates a mock auth server with custom response
func SetupMockAuthServerWithResponse(t *testing.T, statusCode int, response string) *MockAuthServer {
	mock := &MockAuthServer{
		Requests: make([]MockAuthRequest, 0),
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse request body
		if r.Body != nil {
			bodyBytes, err := io.ReadAll(r.Body)
			if err == nil {
				var authReq map[string]string
				if json.Unmarshal(bodyBytes, &authReq) == nil {
					mock.mu.Lock()
					mock.Requests = append(mock.Requests, MockAuthRequest{
						Email:    authReq["email"],
						Password: authReq["password"],
						Headers:  r.Header.Clone(),
						Time:     time.Now(),
					})
					mock.mu.Unlock()
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(response))
	})

	// Create TLS server for more realistic testing
	mock.Server = httptest.NewTLSServer(handler)

	t.Logf("Mock auth server started at: %s", mock.URL)
	return mock
}

// GetRequests returns captured authentication requests
func (m *MockAuthServer) GetRequests() []MockAuthRequest {
	m.mu.Lock()
	defer m.mu.Unlock()

	requests := make([]MockAuthRequest, len(m.Requests))
	copy(requests, m.Requests)
	return requests
}

// GetLastRequest returns the most recent authentication request
func (m *MockAuthServer) GetLastRequest() *MockAuthRequest {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.Requests) == 0 {
		return nil
	}

	return &m.Requests[len(m.Requests)-1]
}

// ClearRequests clears the captured requests
func (m *MockAuthServer) ClearRequests() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Requests = m.Requests[:0]
}

// Test socket helpers

// GetTestSocketPath generates a unique socket path for testing
func GetTestSocketPath(t *testing.T, testName string) string {
	t.Helper()

	// Use /tmp with test name and random suffix to avoid conflicts
	socketPath := filepath.Join("/tmp", fmt.Sprintf("raven-%s-%d.sock", testName, time.Now().UnixNano()))

	// Ensure cleanup
	t.Cleanup(func() {
		_ = os.Remove(socketPath)
	})

	return socketPath
}

// WaitForUnixSocket waits for a Unix socket to be available
func WaitForUnixSocket(t *testing.T, socketPath string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(socketPath); err == nil {
			// Socket exists, try to connect to verify it's ready
			conn, err := net.Dial("unix", socketPath)
			if err == nil {
				_ = conn.Close()
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("Unix socket %s not available within %v", socketPath, timeout)
}
