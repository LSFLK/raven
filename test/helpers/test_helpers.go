//go:build test
// +build test

package helpers

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"go-imap/internal/db"
	"go-imap/internal/delivery/parser"
	"go-imap/internal/models"
	"go-imap/internal/server"

	_ "github.com/mattn/go-sqlite3"
)

// MockConn implements net.Conn for testing
type MockConn struct {
	readBuffer  []byte
	writeBuffer []byte
	readPos     int
	closed      bool
}

func NewMockConn() *MockConn {
	return &MockConn{
		readBuffer:  make([]byte, 0),
		writeBuffer: make([]byte, 0),
	}
}

func (m *MockConn) Read(b []byte) (int, error) {
	if m.readPos >= len(m.readBuffer) {
		return 0, net.ErrClosed
	}
	n := copy(b, m.readBuffer[m.readPos:])
	m.readPos += n
	return n, nil
}

func (m *MockConn) Write(b []byte) (int, error) {
	m.writeBuffer = append(m.writeBuffer, b...)
	return len(b), nil
}

func (m *MockConn) Close() error {
	m.closed = true
	return nil
}

func (m *MockConn) LocalAddr() net.Addr                { return nil }
func (m *MockConn) RemoteAddr() net.Addr               { return nil }
func (m *MockConn) SetDeadline(t time.Time) error      { return nil }
func (m *MockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *MockConn) SetWriteDeadline(t time.Time) error { return nil }

func (m *MockConn) GetWrittenData() string {
	return string(m.writeBuffer)
}

func (m *MockConn) ClearWriteBuffer() {
	m.writeBuffer = m.writeBuffer[:0]
}

func (m *MockConn) AddReadData(data string) {
	m.readBuffer = append(m.readBuffer, []byte(data)...)
}

// MockTLSConn wraps MockConn to simulate TLS connection
type MockTLSConn struct {
	*MockConn
}

func NewMockTLSConn() *MockTLSConn {
	return &MockTLSConn{
		MockConn: NewMockConn(),
	}
}

// Indicate to server code that this mock represents a TLS connection
func (m *MockTLSConn) IsTLS() bool { return true }

// Interface for mock connections to allow polymorphism
type MockConnInterface interface {
	net.Conn
	GetWrittenData() string
	ClearWriteBuffer()
	AddReadData(string)
}

// Ensure MockConn implements MockConnInterface
var _ MockConnInterface = (*MockConn)(nil)
var _ MockConnInterface = (*MockTLSConn)(nil)

// SetupTestServer creates a test IMAP server with in-memory database using new schema
func SetupTestServer(t *testing.T) (*server.TestInterface, func()) {
	// Create a temporary database file (we need a file for InitDB to work)
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Initialize database with new normalized schema
	database, err := db.InitDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize test database: %v", err)
	}

	imapServer := server.NewIMAPServer(database)
	testInterface := server.NewTestInterface(imapServer)

	// Generate test certificates for STARTTLS
	certPath, keyPath, certCleanup := GenerateTestCertificates(t)
	testInterface.SetTLSCertificates(certPath, keyPath)

	cleanup := func() {
		certCleanup()
		database.Close()
		// Temp dir cleanup is handled by t.TempDir()
	}
	return testInterface, cleanup
}

// SetupTestServerSimple creates a test IMAP server without cleanup function
// for backward compatibility with existing tests
func SetupTestServerSimple(t *testing.T) *server.TestInterface {
	srv, _ := SetupTestServer(t)
	return srv
}

// TestServerWithDB creates a test server with a specific database
func TestServerWithDB(database *sql.DB) *server.TestInterface {
	imapServer := server.NewIMAPServer(database)
	return server.NewTestInterface(imapServer)
}

// CreateTestDB creates an in-memory SQLite database for testing with new schema
func CreateTestDB(t *testing.T) *sql.DB {
	// Create a temporary database file
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Initialize database with new normalized schema
	database, err := db.InitDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize test database: %v", err)
	}

	return database
}

// CreateTestUser creates a test user with default mailboxes in the new schema
func CreateTestUser(t *testing.T, database *sql.DB, username string) (userID int64) {
	domain := "localhost"
	if strings.Contains(username, "@") {
		parts := strings.Split(username, "@")
		username = parts[0]
		domain = parts[1]
	}

	// Create domain
	domainID, err := db.GetOrCreateDomain(database, domain)
	if err != nil {
		t.Fatalf("Failed to create domain: %v", err)
	}

	// Create user
	userID, err = db.GetOrCreateUser(database, username, domainID)
	if err != nil {
		t.Fatalf("Failed to create user %s: %v", username, err)
	}

	// Create default mailboxes
	defaultMailboxes := []struct {
		name       string
		specialUse string
	}{
		{"INBOX", "\\Inbox"},
		{"Sent", "\\Sent"},
		{"Drafts", "\\Drafts"},
		{"Trash", "\\Trash"},
	}

	for _, mbx := range defaultMailboxes {
		_, err := db.CreateMailbox(database, userID, mbx.name, mbx.specialUse)
		if err != nil && !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("Failed to create mailbox %s: %v", mbx.name, err)
		}
	}

	return userID
}

// CreateTestUserTable creates a user with mailboxes (compatibility function)
func CreateTestUserTable(t *testing.T, database *sql.DB, username string) {
	CreateTestUser(t, database, username)
}

// InsertTestMail inserts a test mail into a user's mailbox using new schema
func InsertTestMail(t *testing.T, database *sql.DB, username, subject, sender, recipient, folder string) int64 {
	// Get user
	domain := "localhost"
	if strings.Contains(username, "@") {
		parts := strings.Split(username, "@")
		username = parts[0]
		domain = parts[1]
	}

	domainID, err := db.GetOrCreateDomain(database, domain)
	if err != nil {
		t.Fatalf("Failed to get domain: %v", err)
	}

	userID, err := db.GetOrCreateUser(database, username, domainID)
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}

	// Get or create mailbox
	mailboxID, err := db.GetMailboxByName(database, userID, folder)
	if err != nil {
		mailboxID, err = db.CreateMailbox(database, userID, folder, "")
		if err != nil {
			t.Fatalf("Failed to create mailbox: %v", err)
		}
	}

	// Create raw message
	rawMessage := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\n\r\nTest message body",
		sender, recipient, subject, time.Now().Format(time.RFC1123Z))

	// Parse and store message
	parsed, err := parser.ParseMIMEMessage(rawMessage)
	if err != nil {
		t.Fatalf("Failed to parse test message: %v", err)
	}

	messageID, err := parser.StoreMessage(database, parsed)
	if err != nil {
		t.Fatalf("Failed to store test message: %v", err)
	}

	// Add message to mailbox
	err = db.AddMessageToMailbox(database, messageID, mailboxID, "", time.Now())
	if err != nil {
		t.Fatalf("Failed to add message to mailbox: %v", err)
	}

	return messageID
}

// TestConn is a bidirectional pipe connection for testing
type TestConn struct {
	reader       *io.PipeReader
	writer       *io.PipeWriter
	localReader  *io.PipeReader
	localWriter  *io.PipeWriter
	closed       bool
	mu           sync.Mutex
	isTLS        bool
	readTimeout  bool
}

// NewTestConn creates a new bidirectional test connection
func NewTestConn() *TestConn {
	// Create two pipes for bidirectional communication
	serverRead, clientWrite := io.Pipe()
	clientRead, serverWrite := io.Pipe()

	return &TestConn{
		reader:      serverRead,
		writer:      serverWrite,
		localReader: clientRead,
		localWriter: clientWrite,
		closed:      false,
		isTLS:       false,
		readTimeout: false,
	}
}

// MarkAsTLS marks this connection as a TLS connection
func (t *TestConn) MarkAsTLS() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.isTLS = true
}

// IsTLS returns whether this is a TLS connection
func (t *TestConn) IsTLS() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.isTLS
}

// SetReadTimeout simulates read timeout
func (t *TestConn) SetReadTimeout(timeout bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.readTimeout = timeout
}

// Read reads data from the server
func (t *TestConn) Read(b []byte) (int, error) {
	t.mu.Lock()
	timeout := t.readTimeout
	t.mu.Unlock()

	if timeout {
		return 0, io.EOF
	}

	return t.reader.Read(b)
}

// Write writes data to the server
func (t *TestConn) Write(b []byte) (int, error) {
	return t.writer.Write(b)
}

// Close closes the connection
func (t *TestConn) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}

	t.closed = true
	t.reader.Close()
	t.writer.Close()
	t.localReader.Close()
	t.localWriter.Close()
	return nil
}

func (t *TestConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}
}

func (t *TestConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 54321}
}

func (t *TestConn) SetDeadline(time.Time) error      { return nil }
func (t *TestConn) SetReadDeadline(time.Time) error  { return nil }
func (t *TestConn) SetWriteDeadline(time.Time) error { return nil }

// ReadLine reads a line from the server (from client perspective)
func ReadLine(conn *TestConn) string {
	reader := bufio.NewReader(conn.localReader)
	line, err := reader.ReadString('\n')
	if err != nil {
		return ""
	}
	return strings.TrimRight(line, "\r\n")
}

// WriteLine writes a line to the server (from client perspective)
func WriteLine(conn *TestConn, line string) {
	conn.localWriter.Write([]byte(line + "\r\n"))
}

// ReadMultiLine reads multiple lines until a tagged response
func ReadMultiLine(conn *TestConn, tag string) []string {
	var lines []string
	for {
		line := ReadLine(conn)
		if line == "" {
			break
		}
		lines = append(lines, line)
		if strings.HasPrefix(line, tag+" ") {
			break
		}
	}
	return lines
}

// GenerateTestCertificates generates self-signed certificates for testing STARTTLS
// Returns the paths to the cert and key files, and a cleanup function
func GenerateTestCertificates(t *testing.T) (certPath, keyPath string, cleanup func()) {
	// Create temporary directory for certificates
	tmpDir := t.TempDir()
	certPath = filepath.Join(tmpDir, "fullchain.pem")
	keyPath = filepath.Join(tmpDir, "privkey.pem")

	// Generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	// Create certificate template
	notBefore := time.Now()
	notAfter := notBefore.Add(24 * time.Hour) // Valid for 24 hours

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("Failed to generate serial number: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Test IMAP Server"},
			CommonName:   "localhost",
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	// Create self-signed certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("Failed to create certificate: %v", err)
	}

	// Write certificate to file
	certFile, err := os.Create(certPath)
	if err != nil {
		t.Fatalf("Failed to create cert file: %v", err)
	}
	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		certFile.Close()
		t.Fatalf("Failed to encode certificate: %v", err)
	}
	certFile.Close()

	// Write private key to file
	keyFile, err := os.Create(keyPath)
	if err != nil {
		t.Fatalf("Failed to create key file: %v", err)
	}
	privBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	if err := pem.Encode(keyFile, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: privBytes}); err != nil {
		keyFile.Close()
		t.Fatalf("Failed to encode private key: %v", err)
	}
	keyFile.Close()

	cleanup = func() {
		// Cleanup is handled by t.TempDir()
	}

	return certPath, keyPath, cleanup
}

// CreateTLSConfig creates a TLS configuration with test certificates
func CreateTLSConfig(t *testing.T) (*tls.Config, func()) {
	certPath, keyPath, cleanup := GenerateTestCertificates(t)

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Fatalf("Failed to load test certificates: %v", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	return tlsConfig, cleanup
}

// CreateMailbox creates a mailbox for a user in the test database (new schema)
func CreateMailbox(t *testing.T, database *sql.DB, username, mailboxName string) {
	domain := "localhost"
	if strings.Contains(username, "@") {
		parts := strings.Split(username, "@")
		username = parts[0]
		domain = parts[1]
	}

	domainID, err := db.GetOrCreateDomain(database, domain)
	if err != nil {
		t.Fatalf("Failed to get domain: %v", err)
	}

	userID, err := db.GetOrCreateUser(database, username, domainID)
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}

	_, err = db.CreateMailbox(database, userID, mailboxName, "")
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Failed to create mailbox %s for user %s: %v", mailboxName, username, err)
	}
}

// SubscribeToMailbox subscribes a user to a mailbox (compatibility wrapper for new schema)
func SubscribeToMailbox(t *testing.T, database *sql.DB, username, mailboxName string) {
	domain := "localhost"
	if strings.Contains(username, "@") {
		parts := strings.Split(username, "@")
		username = parts[0]
		domain = parts[1]
	}

	domainID, err := db.GetOrCreateDomain(database, domain)
	if err != nil {
		t.Fatalf("Failed to get domain: %v", err)
	}

	userID, err := db.GetOrCreateUser(database, username, domainID)
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}

	err = db.SubscribeToMailbox(database, userID, mailboxName)
	if err != nil {
		t.Fatalf("Failed to subscribe to mailbox %s for user %s: %v", mailboxName, username, err)
	}
}

// GetUserID returns the user ID for a username (helper for tests)
func GetUserID(t *testing.T, database *sql.DB, username string) int64 {
	domain := "localhost"
	if strings.Contains(username, "@") {
		parts := strings.Split(username, "@")
		username = parts[0]
		domain = parts[1]
	}

	domainID, err := db.GetOrCreateDomain(database, domain)
	if err != nil {
		t.Fatalf("Failed to get domain: %v", err)
	}

	userID, err := db.GetOrCreateUser(database, username, domainID)
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}

	return userID
}

// SetupAuthenticatedState creates an authenticated state with proper user setup in database
func SetupAuthenticatedState(t *testing.T, server *server.TestInterface, username string) *models.ClientState {
	database := server.GetDB().(*sql.DB)
	userID := CreateTestUser(t, database, username)

	domain := "localhost"
	if strings.Contains(username, "@") {
		parts := strings.Split(username, "@")
		username = parts[0]
		domain = parts[1]
	}

	domainID, err := db.GetOrCreateDomain(database, domain)
	if err != nil {
		t.Fatalf("Failed to get domain: %v", err)
	}

	return &models.ClientState{
		Authenticated: true,
		Username:      username,
		UserID:        userID,
		DomainID:      domainID,
	}
}
