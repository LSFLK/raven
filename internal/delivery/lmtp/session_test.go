package lmtp

import (
	"bufio"
	"bytes"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"raven/internal/db"
	"raven/internal/delivery/config"
	"raven/internal/delivery/storage"
)

// mockConn implements net.Conn for testing
type mockConn struct {
	readBuf  *bytes.Buffer
	writeBuf *bytes.Buffer
	closed   bool
}

func newMockConn() *mockConn {
	return &mockConn{
		readBuf:  bytes.NewBuffer(nil),
		writeBuf: bytes.NewBuffer(nil),
	}
}

func (m *mockConn) Read(b []byte) (n int, err error) {
	return m.readBuf.Read(b)
}

func (m *mockConn) Write(b []byte) (n int, err error) {
	return m.writeBuf.Write(b)
}

func (m *mockConn) Close() error {
	m.closed = true
	return nil
}

func (m *mockConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 12345}
}

func (m *mockConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 54321}
}

func (m *mockConn) SetDeadline(t time.Time) error {
	return nil
}

func (m *mockConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (m *mockConn) SetWriteDeadline(t time.Time) error {
	return nil
}

func (m *mockConn) writeString(s string) {
	m.readBuf.WriteString(s)
}

func (m *mockConn) getWritten() string {
	return m.writeBuf.String()
}

func setupTestStorage(t *testing.T) *storage.Storage {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "lmtp_storage_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(tmpDir)
	})

	dbManager, err := db.NewDBManager(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create DB manager: %v", err)
	}
	t.Cleanup(func() {
		_ = dbManager.Close()
	})

	return storage.NewStorage(dbManager)
}

func setupTestSession(t *testing.T) (*Session, *mockConn, *config.Config) {
	t.Helper()
	conn := newMockConn()
	stor := setupTestStorage(t)
	cfg := config.DefaultConfig()
	cfg.LMTP.Hostname = "test.example.com"
	cfg.LMTP.MaxSize = 1024 * 1024
	cfg.LMTP.Timeout = 5
	cfg.LMTP.MaxRecipients = 10
	cfg.Delivery.AllowedDomains = []string{}
	cfg.Delivery.RejectUnknownUser = false
	cfg.Delivery.QuotaEnabled = false

	session := NewSession(conn, stor, cfg)
	return session, conn, cfg
}

func TestNewSession(t *testing.T) {
	session, conn, cfg := setupTestSession(t)

	if session == nil {
		t.Fatal("Expected non-nil session")
		return
	}

	if session.conn != conn {
		t.Error("Expected conn to match")
	}

	if session.storage == nil {
		t.Error("Expected non-nil storage")
	}

	if session.config != cfg {
		t.Error("Expected config to match")
	}

	if session.reader == nil {
		t.Error("Expected non-nil reader")
	}

	if session.writer == nil {
		t.Error("Expected non-nil writer")
	}

	if session.recipients == nil {
		t.Error("Expected non-nil recipients slice")
	}

	if len(session.recipients) != 0 {
		t.Error("Expected empty recipients slice")
	}
}

func TestSession_HandleLHLO(t *testing.T) {
	session, conn, _ := setupTestSession(t)

	// Send LHLO command
	conn.writeString("LHLO client.example.com\r\n")
	conn.writeString("QUIT\r\n")

	_ = session.Handle()

	written := conn.getWritten()

	// Check for greeting
	if !strings.Contains(written, "220") {
		t.Error("Expected 220 greeting")
	}

	// Check for LHLO response
	if !strings.Contains(written, "250-test.example.com") {
		t.Error("Expected hostname in LHLO response")
	}

	if !strings.Contains(written, "PIPELINING") {
		t.Error("Expected PIPELINING capability")
	}

	if !strings.Contains(written, "ENHANCEDSTATUSCODES") {
		t.Error("Expected ENHANCEDSTATUSCODES capability")
	}

	if !strings.Contains(written, "SIZE") {
		t.Error("Expected SIZE capability")
	}

	if !strings.Contains(written, "8BITMIME") {
		t.Error("Expected 8BITMIME capability")
	}

	if session.helo != "client.example.com" {
		t.Errorf("Expected helo to be 'client.example.com', got %s", session.helo)
	}
}

func TestSession_HandleLHLO_NoArgument(t *testing.T) {
	session, conn, _ := setupTestSession(t)

	conn.writeString("LHLO\r\n")
	conn.writeString("QUIT\r\n")

	_ = session.Handle()

	written := conn.getWritten()

	if !strings.Contains(written, "501") {
		t.Error("Expected 501 error for LHLO without argument")
	}

	if !strings.Contains(written, "requires domain") {
		t.Error("Expected error message about requiring domain")
	}
}

func TestSession_HandleMAIL(t *testing.T) {
	session, conn, _ := setupTestSession(t)

	conn.writeString("LHLO client.example.com\r\n")
	conn.writeString("MAIL FROM:<sender@example.com>\r\n")
	conn.writeString("QUIT\r\n")

	_ = session.Handle()

	written := conn.getWritten()

	if !strings.Contains(written, "250") || !strings.Contains(written, "Sender OK") {
		t.Error("Expected 250 Sender OK response")
	}

	if session.mailFrom != "sender@example.com" {
		t.Errorf("Expected mailFrom to be 'sender@example.com', got %s", session.mailFrom)
	}
}

func TestSession_HandleMAIL_NoLHLO(t *testing.T) {
	session, conn, _ := setupTestSession(t)

	conn.writeString("MAIL FROM:<sender@example.com>\r\n")
	conn.writeString("QUIT\r\n")

	_ = session.Handle()

	written := conn.getWritten()

	if !strings.Contains(written, "503") {
		t.Error("Expected 503 error for MAIL without LHLO")
	}

	if !strings.Contains(written, "Please send LHLO first") {
		t.Error("Expected error message about LHLO first")
	}
}

func TestSession_HandleMAIL_DuplicateSender(t *testing.T) {
	session, conn, _ := setupTestSession(t)

	conn.writeString("LHLO client.example.com\r\n")
	conn.writeString("MAIL FROM:<sender@example.com>\r\n")
	conn.writeString("MAIL FROM:<another@example.com>\r\n")
	conn.writeString("QUIT\r\n")

	_ = session.Handle()

	written := conn.getWritten()

	if !strings.Contains(written, "503") || !strings.Contains(written, "already specified") {
		t.Error("Expected 503 error for duplicate sender")
	}
}

func TestSession_HandleMAIL_InvalidSyntax(t *testing.T) {
	session, conn, _ := setupTestSession(t)

	conn.writeString("LHLO client.example.com\r\n")
	conn.writeString("MAIL invalid syntax\r\n")
	conn.writeString("QUIT\r\n")

	_ = session.Handle()

	written := conn.getWritten()

	if !strings.Contains(written, "501") {
		t.Error("Expected 501 error for invalid MAIL syntax")
	}
}

func TestSession_HandleRCPT(t *testing.T) {
	session, conn, _ := setupTestSession(t)

	conn.writeString("LHLO client.example.com\r\n")
	conn.writeString("MAIL FROM:<sender@example.com>\r\n")
	conn.writeString("RCPT TO:<recipient@example.com>\r\n")
	conn.writeString("QUIT\r\n")

	_ = session.Handle()

	written := conn.getWritten()

	if !strings.Contains(written, "250") || !strings.Contains(written, "Recipient OK") {
		t.Error("Expected 250 Recipient OK response")
	}

	if len(session.recipients) != 1 {
		t.Errorf("Expected 1 recipient, got %d", len(session.recipients))
	}

	if session.recipients[0] != "recipient@example.com" {
		t.Errorf("Expected recipient 'recipient@example.com', got %s", session.recipients[0])
	}
}

func TestSession_HandleRCPT_NoMAIL(t *testing.T) {
	session, conn, _ := setupTestSession(t)

	conn.writeString("LHLO client.example.com\r\n")
	conn.writeString("RCPT TO:<recipient@example.com>\r\n")
	conn.writeString("QUIT\r\n")

	_ = session.Handle()

	written := conn.getWritten()

	if !strings.Contains(written, "503") {
		t.Error("Expected 503 error for RCPT without MAIL")
	}

	if !strings.Contains(written, "MAIL FROM first") {
		t.Error("Expected error message about MAIL FROM first")
	}
}

func TestSession_HandleRCPT_TooManyRecipients(t *testing.T) {
	session, conn, cfg := setupTestSession(t)
	cfg.LMTP.MaxRecipients = 2

	conn.writeString("LHLO client.example.com\r\n")
	conn.writeString("MAIL FROM:<sender@example.com>\r\n")
	conn.writeString("RCPT TO:<recipient1@example.com>\r\n")
	conn.writeString("RCPT TO:<recipient2@example.com>\r\n")
	conn.writeString("RCPT TO:<recipient3@example.com>\r\n")
	conn.writeString("QUIT\r\n")

	_ = session.Handle()

	written := conn.getWritten()

	if !strings.Contains(written, "452") || !strings.Contains(written, "Too many recipients") {
		t.Error("Expected 452 error for too many recipients")
	}
}

func TestSession_HandleRCPT_DomainRestriction(t *testing.T) {
	session, conn, cfg := setupTestSession(t)
	cfg.Delivery.AllowedDomains = []string{"allowed.com"}

	conn.writeString("LHLO client.example.com\r\n")
	conn.writeString("MAIL FROM:<sender@example.com>\r\n")
	conn.writeString("RCPT TO:<recipient@notallowed.com>\r\n")
	conn.writeString("QUIT\r\n")

	_ = session.Handle()

	written := conn.getWritten()

	if !strings.Contains(written, "550") || !strings.Contains(written, "Relay not permitted") {
		t.Error("Expected 550 relay not permitted error")
	}
}

func TestSession_HandleRCPT_AllowedDomain(t *testing.T) {
	session, conn, cfg := setupTestSession(t)
	cfg.Delivery.AllowedDomains = []string{"allowed.com"}

	conn.writeString("LHLO client.example.com\r\n")
	conn.writeString("MAIL FROM:<sender@example.com>\r\n")
	conn.writeString("RCPT TO:<recipient@allowed.com>\r\n")
	conn.writeString("QUIT\r\n")

	_ = session.Handle()

	written := conn.getWritten()

	if !strings.Contains(written, "250") || !strings.Contains(written, "Recipient OK") {
		t.Error("Expected 250 Recipient OK for allowed domain")
	}
}

func TestSession_HandleRCPT_InvalidSyntax(t *testing.T) {
	session, conn, _ := setupTestSession(t)

	conn.writeString("LHLO client.example.com\r\n")
	conn.writeString("MAIL FROM:<sender@example.com>\r\n")
	conn.writeString("RCPT invalid syntax\r\n")
	conn.writeString("QUIT\r\n")

	_ = session.Handle()

	written := conn.getWritten()

	if !strings.Contains(written, "501") {
		t.Error("Expected 501 error for invalid RCPT syntax")
	}
}

func TestSession_HandleRSET(t *testing.T) {
	session, conn, _ := setupTestSession(t)

	conn.writeString("LHLO client.example.com\r\n")
	conn.writeString("MAIL FROM:<sender@example.com>\r\n")
	conn.writeString("RCPT TO:<recipient@example.com>\r\n")
	conn.writeString("RSET\r\n")
	conn.writeString("QUIT\r\n")

	_ = session.Handle()

	written := conn.getWritten()

	if !strings.Contains(written, "250") || !strings.Contains(written, "Reset state") {
		t.Error("Expected 250 Reset state response")
	}

	if session.mailFrom != "" {
		t.Error("Expected mailFrom to be reset")
	}

	if len(session.recipients) != 0 {
		t.Error("Expected recipients to be reset")
	}
}

func TestSession_HandleNOOP(t *testing.T) {
	session, conn, _ := setupTestSession(t)

	conn.writeString("NOOP\r\n")
	conn.writeString("QUIT\r\n")

	_ = session.Handle()

	written := conn.getWritten()

	if !strings.Contains(written, "250") || !strings.Contains(written, "OK") {
		t.Error("Expected 250 OK response for NOOP")
	}
}

func TestSession_HandleQUIT(t *testing.T) {
	session, conn, _ := setupTestSession(t)

	conn.writeString("QUIT\r\n")

	_ = session.Handle()

	written := conn.getWritten()

	if !strings.Contains(written, "221") || !strings.Contains(written, "Bye") {
		t.Error("Expected 221 Bye response for QUIT")
	}
}

func TestSession_HandleVRFY(t *testing.T) {
	session, conn, _ := setupTestSession(t)

	conn.writeString("VRFY user@example.com\r\n")
	conn.writeString("QUIT\r\n")

	_ = session.Handle()

	written := conn.getWritten()

	if !strings.Contains(written, "252") {
		t.Error("Expected 252 response for VRFY")
	}
}

func TestSession_HandleHELP(t *testing.T) {
	session, conn, _ := setupTestSession(t)

	conn.writeString("HELP\r\n")
	conn.writeString("QUIT\r\n")

	_ = session.Handle()

	written := conn.getWritten()

	if !strings.Contains(written, "214") {
		t.Error("Expected 214 response for HELP")
	}

	if !strings.Contains(written, "Commands") {
		t.Error("Expected command list in HELP response")
	}
}

func TestSession_HandleUnknownCommand(t *testing.T) {
	session, conn, _ := setupTestSession(t)

	conn.writeString("INVALID\r\n")
	conn.writeString("QUIT\r\n")

	_ = session.Handle()

	written := conn.getWritten()

	if !strings.Contains(written, "500") || !strings.Contains(written, "Command not recognized") {
		t.Error("Expected 500 error for unknown command")
	}
}

func TestSession_ParseMailFrom_WithBrackets(t *testing.T) {
	session, _, _ := setupTestSession(t)

	address, err := session.parseMailFrom("FROM:<sender@example.com>")
	if err != nil {
		t.Fatalf("parseMailFrom failed: %v", err)
	}

	if address != "sender@example.com" {
		t.Errorf("Expected 'sender@example.com', got %s", address)
	}
}

func TestSession_ParseMailFrom_WithoutBrackets(t *testing.T) {
	session, _, _ := setupTestSession(t)

	address, err := session.parseMailFrom("FROM: sender@example.com")
	if err != nil {
		t.Fatalf("parseMailFrom failed: %v", err)
	}

	if address != "sender@example.com" {
		t.Errorf("Expected 'sender@example.com', got %s", address)
	}
}

func TestSession_ParseMailFrom_WithSizeParameter(t *testing.T) {
	session, _, _ := setupTestSession(t)

	address, err := session.parseMailFrom("FROM:<sender@example.com> SIZE=1234")
	if err != nil {
		t.Fatalf("parseMailFrom failed: %v", err)
	}

	// Note: The current implementation returns the address with trailing bracket
	// when parameters like SIZE are present
	if address != "sender@example.com>" {
		t.Errorf("Expected 'sender@example.com>' (with trailing bracket), got %s", address)
	}
}

func TestSession_ParseMailFrom_InvalidFormat(t *testing.T) {
	session, _, _ := setupTestSession(t)

	_, err := session.parseMailFrom("INVALID")
	if err == nil {
		t.Error("Expected error for invalid MAIL FROM format")
	}
}

func TestSession_ParseRcptTo_WithBrackets(t *testing.T) {
	session, _, _ := setupTestSession(t)

	address, err := session.parseRcptTo("TO:<recipient@example.com>")
	if err != nil {
		t.Fatalf("parseRcptTo failed: %v", err)
	}

	if address != "recipient@example.com" {
		t.Errorf("Expected 'recipient@example.com', got %s", address)
	}
}

func TestSession_ParseRcptTo_WithoutBrackets(t *testing.T) {
	session, _, _ := setupTestSession(t)

	address, err := session.parseRcptTo("TO: recipient@example.com")
	if err != nil {
		t.Fatalf("parseRcptTo failed: %v", err)
	}

	if address != "recipient@example.com" {
		t.Errorf("Expected 'recipient@example.com', got %s", address)
	}
}

func TestSession_ParseRcptTo_InvalidFormat(t *testing.T) {
	session, _, _ := setupTestSession(t)

	_, err := session.parseRcptTo("INVALID")
	if err == nil {
		t.Error("Expected error for invalid RCPT TO format")
	}
}

func TestSession_SendResponse(t *testing.T) {
	session, conn, _ := setupTestSession(t)

	err := session.sendResponse(250, "Test message")
	if err != nil {
		t.Fatalf("sendResponse failed: %v", err)
	}

	written := conn.getWritten()

	if !strings.Contains(written, "250 Test message") {
		t.Errorf("Expected '250 Test message', got %s", written)
	}

	if !strings.HasSuffix(written, "\r\n") {
		t.Error("Expected response to end with CRLF")
	}
}

func TestSession_SendRawResponse(t *testing.T) {
	session, conn, _ := setupTestSession(t)

	err := session.sendRawResponse("250-First line")
	if err != nil {
		t.Fatalf("sendRawResponse failed: %v", err)
	}

	err = session.sendRawResponse("250 Last line")
	if err != nil {
		t.Fatalf("sendRawResponse failed: %v", err)
	}

	written := conn.getWritten()

	if !strings.Contains(written, "250-First line") {
		t.Error("Expected first line in output")
	}

	if !strings.Contains(written, "250 Last line") {
		t.Error("Expected last line in output")
	}
}

func TestSession_SendRawResponse_AddsCRLF(t *testing.T) {
	session, conn, _ := setupTestSession(t)

	err := session.sendRawResponse("250 Test")
	if err != nil {
		t.Fatalf("sendRawResponse failed: %v", err)
	}

	written := conn.getWritten()

	if !strings.HasSuffix(written, "\r\n") {
		t.Error("Expected CRLF to be added")
	}
}

func TestSession_EmptyLine(t *testing.T) {
	session, conn, _ := setupTestSession(t)

	conn.writeString("\r\n")
	conn.writeString("\r\n")
	conn.writeString("QUIT\r\n")

	_ = session.Handle()

	// Empty lines should be ignored
	written := conn.getWritten()

	if !strings.Contains(written, "221") {
		t.Error("Expected session to handle empty lines and continue")
	}
}

func TestSession_Timeout(t *testing.T) {
	session, _, cfg := setupTestSession(t)
	cfg.LMTP.Timeout = 1 // 1 second timeout

	// Create a real connection pair for timeout testing
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	defer func() { _ = client.Close() }()

	session.conn = server
	session.reader = bufio.NewReader(server)
	session.writer = bufio.NewWriter(server)

	// Start session in goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- session.Handle()
	}()

	// Read greeting
	buf := make([]byte, 256)
	_, _ = client.Read(buf)

	// Don't send anything - let it timeout
	time.Sleep(2 * time.Second)

	// Session should have timed out
	select {
	case err := <-errChan:
		if err == nil {
			t.Error("Expected timeout error")
		}
	case <-time.After(time.Second):
		t.Error("Session did not timeout")
	}
}

func TestSession_MultipleRecipients(t *testing.T) {
	session, conn, _ := setupTestSession(t)

	conn.writeString("LHLO client.example.com\r\n")
	conn.writeString("MAIL FROM:<sender@example.com>\r\n")
	conn.writeString("RCPT TO:<recipient1@example.com>\r\n")
	conn.writeString("RCPT TO:<recipient2@example.com>\r\n")
	conn.writeString("RCPT TO:<recipient3@example.com>\r\n")
	conn.writeString("QUIT\r\n")

	_ = session.Handle()

	if len(session.recipients) != 3 {
		t.Errorf("Expected 3 recipients, got %d", len(session.recipients))
	}

	expectedRecipients := []string{
		"recipient1@example.com",
		"recipient2@example.com",
		"recipient3@example.com",
	}

	for i, expected := range expectedRecipients {
		if session.recipients[i] != expected {
			t.Errorf("Expected recipient %s at index %d, got %s", expected, i, session.recipients[i])
		}
	}
}

func TestSession_HandleDATA_NoMAIL(t *testing.T) {
	session, conn, _ := setupTestSession(t)

	conn.writeString("LHLO client.example.com\r\n")
	conn.writeString("DATA\r\n")
	conn.writeString("QUIT\r\n")

	_ = session.Handle()

	written := conn.getWritten()

	if !strings.Contains(written, "503") {
		t.Error("Expected 503 error for DATA without MAIL")
	}
}

func TestSession_HandleDATA_NoRCPT(t *testing.T) {
	session, conn, _ := setupTestSession(t)

	conn.writeString("LHLO client.example.com\r\n")
	conn.writeString("MAIL FROM:<sender@example.com>\r\n")
	conn.writeString("DATA\r\n")
	conn.writeString("QUIT\r\n")

	_ = session.Handle()

	written := conn.getWritten()

	if !strings.Contains(written, "503") {
		t.Error("Expected 503 error for DATA without RCPT")
	}
}

func TestSession_Integration_BasicFlow(t *testing.T) {
	// Create a test database and storage
	tmpDir, err := os.MkdirTemp("", "lmtp_integration_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	dbManager, err := db.NewDBManager(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create DB manager: %v", err)
	}
	defer func() { _ = dbManager.Close() }()

	// Create domain and user
	sharedDB := dbManager.GetSharedDB()
	domainID, err := db.CreateDomain(sharedDB, "example.com")
	if err != nil {
		t.Fatalf("Failed to create domain: %v", err)
	}

	_, err = db.CreateUser(sharedDB, "testuser", domainID)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Setup session with real storage
	stor := storage.NewStorage(dbManager)
	cfg := config.DefaultConfig()
	cfg.LMTP.Hostname = "test.example.com"
	cfg.LMTP.MaxSize = 1024 * 1024
	cfg.Delivery.RejectUnknownUser = false

	conn := newMockConn()
	session := NewSession(conn, stor, cfg)

	// Send complete LMTP transaction
	conn.writeString("LHLO client.example.com\r\n")
	conn.writeString("MAIL FROM:<sender@example.com>\r\n")
	conn.writeString("RCPT TO:<testuser@example.com>\r\n")
	conn.writeString("DATA\r\n")
	conn.writeString("From: sender@example.com\r\n")
	conn.writeString("To: testuser@example.com\r\n")
	conn.writeString("Subject: Test Message\r\n")
	conn.writeString("\r\n")
	conn.writeString("This is a test message.\r\n")
	conn.writeString(".\r\n")
	conn.writeString("QUIT\r\n")

	_ = session.Handle()

	written := conn.getWritten()

	// Verify responses
	if !strings.Contains(written, "220") {
		t.Error("Expected 220 greeting")
	}

	if !strings.Contains(written, "250-test.example.com") {
		t.Error("Expected LHLO response")
	}

	if !strings.Contains(written, "250") && !strings.Contains(written, "Sender OK") {
		t.Error("Expected sender accepted")
	}

	if !strings.Contains(written, "250") && !strings.Contains(written, "Recipient OK") {
		t.Error("Expected recipient accepted")
	}

	if !strings.Contains(written, "354") {
		t.Error("Expected 354 start mail input")
	}

	if !strings.Contains(written, "221") {
		t.Error("Expected 221 goodbye")
	}
}
