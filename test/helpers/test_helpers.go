package helpers

import (
	"database/sql"
	"fmt"
	"net"
	"testing"
	"time"

	"go-imap/internal/db"
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

// SetupTestServer creates a test IMAP server with in-memory database
func SetupTestServer(t *testing.T) *server.TestInterface {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Initialize database schema with metadata table
	schema := `
	CREATE TABLE IF NOT EXISTS user_metadata (
		username TEXT PRIMARY KEY,
		created_at TEXT DEFAULT CURRENT_TIMESTAMP
	);
	`
	if _, err = db.Exec(schema); err != nil {
		t.Fatalf("Failed to initialize test database schema: %v", err)
	}

	imapServer := server.NewIMAPServer(db)
	return server.NewTestInterface(imapServer)
}

// TestServerWithDB creates a test server with a specific database
func TestServerWithDB(db *sql.DB) *server.TestInterface {
	imapServer := server.NewIMAPServer(db)
	return server.NewTestInterface(imapServer)
}

// CreateTestDB creates an in-memory SQLite database for testing
func CreateTestDB(t *testing.T) *sql.DB {
	database, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	
	// Create user_metadata table
	metadataSchema := `
	CREATE TABLE IF NOT EXISTS user_metadata (
		username TEXT PRIMARY KEY,
		created_at TEXT DEFAULT CURRENT_TIMESTAMP
	);
	`
	if _, err = database.Exec(metadataSchema); err != nil {
		t.Fatalf("Failed to initialize test database metadata schema: %v", err)
	}
	
	return database
}

// CreateTestUserTable creates a table for a test user
func CreateTestUserTable(t *testing.T, database *sql.DB, username string) {
	err := db.CreateUserTable(database, username)
	if err != nil {
		t.Fatalf("Failed to create user table for %s: %v", username, err)
	}
}

// InsertTestMail inserts a test mail into a user's table
func InsertTestMail(t *testing.T, database *sql.DB, username, subject, sender, recipient, folder string) {
	tableName := db.GetUserTableName(username)
	query := fmt.Sprintf(
		"INSERT INTO %s (subject, sender, recipient, date_sent, raw_message, flags, folder) VALUES (?, ?, ?, ?, ?, ?, ?)",
		tableName,
	)
	rawMessage := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\nTest message body", sender, recipient, subject)
	_, err := database.Exec(query, subject, sender, recipient, "01-Jan-2024 12:00:00 +0000", rawMessage, "", folder)
	if err != nil {
		t.Fatalf("Failed to insert test mail: %v", err)
	}
}
