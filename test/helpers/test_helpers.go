package helpers

import (
	"database/sql"
	"net"
	"testing"
	"time"

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
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	return db
}
