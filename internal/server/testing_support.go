package server

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

	"raven/internal/db"
	"raven/internal/delivery/parser"
	"raven/internal/models"

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

// SetupTestServer creates a test IMAP server with DBManager and per-user databases
func SetupTestServer(t *testing.T) (*TestInterface, func()) {
	// Create a temporary directory for databases
	tmpDir := t.TempDir()

	// Initialize DBManager with the temp directory
	dbManager, err := db.NewDBManager(tmpDir)
	if err != nil {
		t.Fatalf("Failed to initialize DBManager: %v", err)
	}

	imapServer := NewIMAPServer(dbManager)
	testInterface := NewTestInterface(imapServer)

	// Generate test certificates for STARTTLS
	certPath, keyPath, certCleanup := GenerateTestCertificates(t)
	testInterface.SetTLSCertificates(certPath, keyPath)

	cleanup := func() {
		certCleanup()
		dbManager.Close()
		// Temp dir cleanup is handled by t.TempDir()
	}
	return testInterface, cleanup
}

// SetupTestServerSimple creates a test IMAP server without cleanup function
// for backward compatibility with existing tests
func SetupTestServerSimple(t *testing.T) *TestInterface {
	srv, _ := SetupTestServer(t)
	return srv
}

// TestServerWithDBManager creates a test server with a specific DBManager
func TestServerWithDBManager(dbManager *db.DBManager) *TestInterface {
	imapServer := NewIMAPServer(dbManager)
	return NewTestInterface(imapServer)
}

// TestServerWithDB creates a test server with a specific database - supports both *sql.DB and *db.DBManager
func TestServerWithDB(database interface{}) *TestInterface {
	switch v := database.(type) {
	case *db.DBManager:
		return TestServerWithDBManager(v)
	case *sql.DB:
		// For legacy tests, wrap in a temporary DBManager
		// This is a fallback for old-style tests, but ideally all tests should use DBManager
		panic("TestServerWithDB: *sql.DB no longer supported, please use *db.DBManager from CreateTestDB()")
	default:
		panic(fmt.Sprintf("TestServerWithDB: unsupported database type: %T", database))
	}
}

// CreateTestDB creates a DBManager for testing with new per-user schema
func CreateTestDB(t *testing.T) *db.DBManager {
	// Create a temporary directory for databases
	tmpDir := t.TempDir()

	// Initialize DBManager with the temp directory
	dbManager, err := db.NewDBManager(tmpDir)
	if err != nil {
		t.Fatalf("Failed to initialize test DBManager: %v", err)
	}

	return dbManager
}

// CreateTestUser creates a test user with default mailboxes - supports both old and new DB architecture
func CreateTestUser(t *testing.T, database interface{}, username string) (userID int64) {
	// Handle both *sql.DB (old tests with shared DB) and *db.DBManager (new per-user DB architecture)
	var sharedDB *sql.DB
	var dbManager *db.DBManager
	
	switch v := database.(type) {
	case *sql.DB:
		sharedDB = v
	case *db.DBManager:
		dbManager = v
		sharedDB = v.GetSharedDB()
	default:
		t.Fatalf("CreateTestUser: unsupported database type: %T", database)
	}

	domain := "localhost"
	if strings.Contains(username, "@") {
		parts := strings.Split(username, "@")
		username = parts[0]
		domain = parts[1]
	}

	// Create domain in shared database
	domainID, err := db.GetOrCreateDomain(sharedDB, domain)
	if err != nil {
		t.Fatalf("Failed to create domain: %v", err)
	}

	// Create user in shared database
	userID, err = db.GetOrCreateUser(sharedDB, username, domainID)
	if err != nil {
		t.Fatalf("Failed to create user %s: %v", username, err)
	}

	// If using DBManager, initialize user database (creates default mailboxes automatically)
	if dbManager != nil {
		_, err := dbManager.GetUserDB(userID)
		if err != nil {
			t.Fatalf("Failed to initialize user database: %v", err)
		}
	} else {
		// Old path: create mailboxes manually in shared DB
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
			_, err := db.CreateMailbox(sharedDB, userID, mbx.name, mbx.specialUse)
			if err != nil && !strings.Contains(err.Error(), "already exists") {
				t.Fatalf("Failed to create mailbox %s: %v", mbx.name, err)
			}
		}
	}

	return userID
}

// CreateTestUserTable creates a user with mailboxes (compatibility function)
func CreateTestUserTable(t *testing.T, database interface{}, username string) {
	CreateTestUser(t, database, username)
}

// InsertTestMail inserts a test mail into a user's mailbox using new schema
func InsertTestMail(t *testing.T, database interface{}, username, subject, sender, recipient, folder string) int64 {
	// Handle both *sql.DB (old tests with shared DB) and *db.DBManager (new per-user DB architecture)
	var sharedDB *sql.DB
	var userDB *sql.DB
	var dbManager *db.DBManager
	
	switch v := database.(type) {
	case *sql.DB:
		sharedDB = v
		userDB = v // Old architecture uses same DB
	case *db.DBManager:
		dbManager = v
		sharedDB = v.GetSharedDB()
	default:
		t.Fatalf("InsertTestMail: unsupported database type: %T", database)
	}

	// Get user
	domain := "localhost"
	if strings.Contains(username, "@") {
		parts := strings.Split(username, "@")
		username = parts[0]
		domain = parts[1]
	}

	domainID, err := db.GetOrCreateDomain(sharedDB, domain)
	if err != nil {
		t.Fatalf("Failed to get domain: %v", err)
	}

	userID, err := db.GetOrCreateUser(sharedDB, username, domainID)
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}

	// Get user database if using DBManager
	if dbManager != nil {
		userDB, err = dbManager.GetUserDB(userID)
		if err != nil {
			t.Fatalf("Failed to get user database: %v", err)
		}
	}

	// Get or create mailbox
	var mailboxID int64
	if dbManager != nil {
		mailboxID, err = db.GetMailboxByNamePerUser(userDB, userID, folder)
		if err != nil {
			mailboxID, err = db.CreateMailboxPerUser(userDB, userID, folder, "")
			if err != nil {
				t.Fatalf("Failed to create mailbox: %v", err)
			}
		}
	} else {
		mailboxID, err = db.GetMailboxByName(userDB, userID, folder)
		if err != nil {
			mailboxID, err = db.CreateMailbox(userDB, userID, folder, "")
			if err != nil {
				t.Fatalf("Failed to create mailbox: %v", err)
			}
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

	var messageID int64
	if dbManager != nil {
		messageID, err = parser.StoreMessagePerUser(userDB, parsed)
		if err != nil {
			t.Fatalf("Failed to store test message: %v", err)
		}
		// Add message to mailbox
		err = db.AddMessageToMailboxPerUser(userDB, messageID, mailboxID, "", time.Now())
		if err != nil {
			t.Fatalf("Failed to add message to mailbox: %v", err)
		}
	} else {
		messageID, err = parser.StoreMessage(userDB, parsed)
		if err != nil {
			t.Fatalf("Failed to store test message: %v", err)
		}
		// Add message to mailbox
		err = db.AddMessageToMailbox(userDB, messageID, mailboxID, "", time.Now())
		if err != nil {
			t.Fatalf("Failed to add message to mailbox: %v", err)
		}
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
func CreateMailbox(t *testing.T, database interface{}, username, mailboxName string) {
	// Handle both *sql.DB and *db.DBManager
	var sharedDB *sql.DB
	var dbManager *db.DBManager
	
	switch v := database.(type) {
	case *sql.DB:
		sharedDB = v
	case *db.DBManager:
		dbManager = v
		sharedDB = v.GetSharedDB()
	default:
		t.Fatalf("CreateMailbox: unsupported database type: %T", database)
	}
	
	domain := "localhost"
	if strings.Contains(username, "@") {
		parts := strings.Split(username, "@")
		username = parts[0]
		domain = parts[1]
	}

	domainID, err := db.GetOrCreateDomain(sharedDB, domain)
	if err != nil {
		t.Fatalf("Failed to get domain: %v", err)
	}

	userID, err := db.GetOrCreateUser(sharedDB, username, domainID)
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}

	// Use per-user DB if we have a DBManager, otherwise use shared DB (legacy)
	if dbManager != nil {
		userDB, err := dbManager.GetUserDB(userID)
		if err != nil {
			t.Fatalf("Failed to get user database: %v", err)
		}
		_, err = db.CreateMailboxPerUser(userDB, userID, mailboxName, "")
		if err != nil && !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("Failed to create mailbox %s for user %s: %v", mailboxName, username, err)
		}
	} else {
		_, err = db.CreateMailbox(sharedDB, userID, mailboxName, "")
		if err != nil && !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("Failed to create mailbox %s for user %s: %v", mailboxName, username, err)
		}
	}
}

// SubscribeToMailbox subscribes a user to a mailbox (compatibility wrapper for new schema)
func SubscribeToMailbox(t *testing.T, database interface{}, username, mailboxName string) {
	var sharedDB *sql.DB
	var userDB *sql.DB
	var err error

	switch v := database.(type) {
	case *sql.DB:
		sharedDB = v
		userDB = v
	case *db.DBManager:
		sharedDB = v.GetSharedDB()
	default:
		t.Fatalf("SubscribeToMailbox: unsupported database type: %T", database)
	}

	domain := "localhost"
	if strings.Contains(username, "@") {
		parts := strings.Split(username, "@")
		username = parts[0]
		domain = parts[1]
	}

	domainID, err := db.GetOrCreateDomain(sharedDB, domain)
	if err != nil {
		t.Fatalf("Failed to get domain: %v", err)
	}

	userID, err := db.GetOrCreateUser(sharedDB, username, domainID)
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}

	// Get user DB if using DBManager
	if dbManager, ok := database.(*db.DBManager); ok {
		userDB, err = dbManager.GetUserDB(userID)
		if err != nil {
			t.Fatalf("Failed to get user database: %v", err)
		}
		err = db.SubscribeToMailboxPerUser(userDB, userID, mailboxName)
	} else {
		err = db.SubscribeToMailbox(userDB, userID, mailboxName)
	}
	
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
func SetupAuthenticatedState(t *testing.T, server *TestInterface, username string) *models.ClientState {
	dbManager := server.GetDBManager().(*db.DBManager)
	sharedDB := dbManager.GetSharedDB()
	
	// Parse username and domain
	domain := "localhost"
	if strings.Contains(username, "@") {
		parts := strings.Split(username, "@")
		username = parts[0]
		domain = parts[1]
	}

	// Create domain and user in shared database
	domainID, err := db.GetOrCreateDomain(sharedDB, domain)
	if err != nil {
		t.Fatalf("Failed to get domain: %v", err)
	}

	userID, err := db.GetOrCreateUser(sharedDB, username, domainID)
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}

	// Initialize user database (creates default mailboxes)
	_, err = dbManager.GetUserDB(userID)
	if err != nil {
		t.Fatalf("Failed to initialize user database: %v", err)
	}

	return &models.ClientState{
		Authenticated: true,
		Username:      username,
		UserID:        userID,
		DomainID:      domainID,
	}
}

// GetDBManager gets the DBManager from a test server
func GetDBManager(t *testing.T, srv interface{}) *db.DBManager {
	type dbManagerGetter interface {
		GetDBManager() interface{}
	}
	
	if getter, ok := srv.(dbManagerGetter); ok {
		return getter.GetDBManager().(*db.DBManager)
	}
	t.Fatalf("Server does not implement GetDBManager()")
	return nil
}

// GetUserDB gets a user's database from the test server
func GetUserDB(t *testing.T, srv interface{}, userID int64) *sql.DB {
	dbManager := GetDBManager(t, srv)
	userDB, err := dbManager.GetUserDB(userID)
	if err != nil {
		t.Fatalf("Failed to get user database: %v", err)
	}
	return userDB
}

// GetSharedDB gets the shared database from the test server
func GetSharedDB(t *testing.T, srv interface{}) *sql.DB {
	dbManager := GetDBManager(t, srv)
	return dbManager.GetSharedDB()
}

// GetDatabaseFromServer returns the DBManager from the test server
// This is the recommended way to get the database in tests
func GetDatabaseFromServer(srv interface{}) *db.DBManager {
	type dbGetter interface {
		GetDB() interface{}
	}
	
	if getter, ok := srv.(dbGetter); ok {
		if dbMgr, ok := getter.GetDB().(*db.DBManager); ok {
			return dbMgr
		}
		panic(fmt.Sprintf("GetDB() returned unexpected type: %T", getter.GetDB()))
	}
	panic("Server does not implement GetDB()")
}

// GetMailboxID is a helper function that gets a mailbox ID for a user
// Works with both old and new database architecture
func GetMailboxID(t *testing.T, dbMgr *db.DBManager, userID int64, mailboxName string) (int64, error) {
	userDB, err := dbMgr.GetUserDB(userID)
	if err != nil {
		return 0, fmt.Errorf("failed to get user database: %v", err)
	}
	return db.GetMailboxByNamePerUser(userDB, userID, mailboxName)
}

// UpdateMessageFlags updates message flags, handling both *sql.DB and *db.DBManager
func UpdateMessageFlags(t *testing.T, database interface{}, username string, messageID int64, flags string) {
	var userDB *sql.DB
	var err error

	switch v := database.(type) {
	case *sql.DB:
		userDB = v
	case *db.DBManager:
		// Get user ID first
		sharedDB := v.GetSharedDB()
		domain := "localhost"
		if strings.Contains(username, "@") {
			parts := strings.Split(username, "@")
			username = parts[0]
			domain = parts[1]
		}

		domainID, err := db.GetOrCreateDomain(sharedDB, domain)
		if err != nil {
			t.Fatalf("Failed to get domain: %v", err)
		}

		userID, err := db.GetOrCreateUser(sharedDB, username, domainID)
		if err != nil {
			t.Fatalf("Failed to get user: %v", err)
		}

		userDB, err = v.GetUserDB(userID)
		if err != nil {
			t.Fatalf("Failed to get user database: %v", err)
		}
	default:
		t.Fatalf("UpdateMessageFlags: unsupported database type: %T", database)
	}

	_, err = userDB.Exec("UPDATE message_mailbox SET flags = ? WHERE message_id = ?", flags, messageID)
	if err != nil {
		t.Fatalf("Failed to update message flags: %v", err)
	}
}

// GetUserDBFromManager gets a user's DB from a DBManager or returns the DB directly if it's *sql.DB
func GetUserDBFromManager(t *testing.T, database interface{}, username string) *sql.DB {
	switch v := database.(type) {
	case *sql.DB:
		return v
	case *db.DBManager:
		sharedDB := v.GetSharedDB()
		domain := "localhost"
		if strings.Contains(username, "@") {
			parts := strings.Split(username, "@")
			username = parts[0]
			domain = parts[1]
		}

		domainID, err := db.GetOrCreateDomain(sharedDB, domain)
		if err != nil {
			t.Fatalf("Failed to get domain: %v", err)
		}

		userID, err := db.GetOrCreateUser(sharedDB, username, domainID)
		if err != nil {
			t.Fatalf("Failed to get user: %v", err)
		}

		userDB, err := v.GetUserDB(userID)
		if err != nil {
			t.Fatalf("Failed to get user database: %v", err)
		}
		return userDB
	default:
		t.Fatalf("GetUserDBFromManager: unsupported database type: %T", database)
		return nil
	}
}

// GetUserDBByID gets a user's DB from a DBManager by userID, or returns the DB directly if it's *sql.DB
func GetUserDBByID(t *testing.T, database interface{}, userID int64) *sql.DB {
	switch v := database.(type) {
	case *sql.DB:
		return v
	case *db.DBManager:
		userDB, err := v.GetUserDB(userID)
		if err != nil {
			t.Fatalf("Failed to get user database: %v", err)
		}
		return userDB
	default:
		t.Fatalf("GetUserDBByID: unsupported database type: %T", database)
		return nil
	}
}
