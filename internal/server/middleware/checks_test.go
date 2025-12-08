package middleware_test

import (
	"database/sql"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"raven/internal/models"
	"raven/internal/server/middleware"
)

// MockServer implements ServerInterface for testing
type MockServer struct {
	responses          []string
	getUserDBError     error
	getSelectedDBError error
	userDB             *sql.DB
	selectedDB         *sql.DB
}

func NewMockServer() *MockServer {
	return &MockServer{
		responses: make([]string, 0),
	}
}

func (m *MockServer) SendResponse(conn net.Conn, response string) {
	m.responses = append(m.responses, response)
	// Also write to the connection for compatibility
	_, _ = conn.Write([]byte(response + "\r\n"))
}

func (m *MockServer) GetUserDB(userID int64) (*sql.DB, error) {
	if m.getUserDBError != nil {
		return nil, m.getUserDBError
	}
	return m.userDB, nil
}

func (m *MockServer) GetSelectedDB(state *models.ClientState) (*sql.DB, int64, error) {
	if m.getSelectedDBError != nil {
		return nil, 0, m.getSelectedDBError
	}
	return m.selectedDB, state.UserID, nil
}

func (m *MockServer) GetSharedDB() *sql.DB {
	return nil
}

func (m *MockServer) GetLastResponse() string {
	if len(m.responses) == 0 {
		return ""
	}
	return m.responses[len(m.responses)-1]
}

func (m *MockServer) ClearResponses() {
	m.responses = make([]string, 0)
}

// MockConn implements net.Conn for testing
type MockConn struct {
	writeBuffer []byte
}

func NewMockConn() *MockConn {
	return &MockConn{
		writeBuffer: make([]byte, 0),
	}
}

func (m *MockConn) Read(b []byte) (int, error) { return 0, nil }
func (m *MockConn) Write(b []byte) (int, error) {
	m.writeBuffer = append(m.writeBuffer, b...)
	return len(b), nil
}
func (m *MockConn) Close() error                       { return nil }
func (m *MockConn) LocalAddr() net.Addr                { return nil }
func (m *MockConn) RemoteAddr() net.Addr               { return nil }
func (m *MockConn) SetDeadline(t time.Time) error      { return nil }
func (m *MockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *MockConn) SetWriteDeadline(t time.Time) error { return nil }

func (m *MockConn) GetWrittenData() string {
	return string(m.writeBuffer)
}

// ============================================================================
// RequireAuth Tests
// ============================================================================

func TestRequireAuth_Authenticated(t *testing.T) {
	server := NewMockServer()
	conn := NewMockConn()
	called := false

	handler := func(conn net.Conn, tag string, parts []string, state *models.ClientState) {
		called = true
	}

	wrapped := middleware.RequireAuth(server, handler)

	state := &models.ClientState{
		Authenticated: true,
	}

	wrapped(conn, "A001", []string{"A001", "TEST"}, state)

	if !called {
		t.Error("Expected handler to be called for authenticated user")
	}

	if len(server.responses) > 0 {
		t.Errorf("Expected no error responses, got: %v", server.responses)
	}
}

func TestRequireAuth_Unauthenticated(t *testing.T) {
	server := NewMockServer()
	conn := NewMockConn()
	called := false

	handler := func(conn net.Conn, tag string, parts []string, state *models.ClientState) {
		called = true
	}

	wrapped := middleware.RequireAuth(server, handler)

	state := &models.ClientState{
		Authenticated: false,
	}

	wrapped(conn, "A001", []string{"A001", "TEST"}, state)

	if called {
		t.Error("Expected handler not to be called for unauthenticated user")
	}

	response := server.GetLastResponse()
	if !strings.Contains(response, "A001 NO Please authenticate first") {
		t.Errorf("Expected authentication error, got: %s", response)
	}
}

// ============================================================================
// RequireMailboxSelected Tests
// ============================================================================

func TestRequireMailboxSelected_MailboxSelected(t *testing.T) {
	server := NewMockServer()
	conn := NewMockConn()
	called := false

	handler := func(conn net.Conn, tag string, parts []string, state *models.ClientState) {
		called = true
	}

	wrapped := middleware.RequireMailboxSelected(server, handler)

	state := &models.ClientState{
		SelectedMailboxID: 1,
	}

	wrapped(conn, "A002", []string{"A002", "FETCH", "1", "FLAGS"}, state)

	if !called {
		t.Error("Expected handler to be called when mailbox is selected")
	}

	if len(server.responses) > 0 {
		t.Errorf("Expected no error responses, got: %v", server.responses)
	}
}

func TestRequireMailboxSelected_NoMailboxSelected(t *testing.T) {
	server := NewMockServer()
	conn := NewMockConn()
	called := false

	handler := func(conn net.Conn, tag string, parts []string, state *models.ClientState) {
		called = true
	}

	wrapped := middleware.RequireMailboxSelected(server, handler)

	state := &models.ClientState{
		SelectedMailboxID: 0,
	}

	wrapped(conn, "A002", []string{"A002", "FETCH", "1", "FLAGS"}, state)

	if called {
		t.Error("Expected handler not to be called when no mailbox is selected")
	}

	response := server.GetLastResponse()
	if !strings.Contains(response, "A002 NO No folder selected") {
		t.Errorf("Expected no folder error, got: %s", response)
	}
}

// ============================================================================
// RequireAuthAndMailbox Tests
// ============================================================================

func TestRequireAuthAndMailbox_BothValid(t *testing.T) {
	server := NewMockServer()
	conn := NewMockConn()
	called := false

	handler := func(conn net.Conn, tag string, parts []string, state *models.ClientState) {
		called = true
	}

	wrapped := middleware.RequireAuthAndMailbox(server, handler)

	state := &models.ClientState{
		Authenticated:     true,
		SelectedMailboxID: 1,
	}

	wrapped(conn, "A003", []string{"A003", "SEARCH", "ALL"}, state)

	if !called {
		t.Error("Expected handler to be called when both auth and mailbox are valid")
	}

	if len(server.responses) > 0 {
		t.Errorf("Expected no error responses, got: %v", server.responses)
	}
}

func TestRequireAuthAndMailbox_NotAuthenticated(t *testing.T) {
	server := NewMockServer()
	conn := NewMockConn()
	called := false

	handler := func(conn net.Conn, tag string, parts []string, state *models.ClientState) {
		called = true
	}

	wrapped := middleware.RequireAuthAndMailbox(server, handler)

	state := &models.ClientState{
		Authenticated:     false,
		SelectedMailboxID: 1,
	}

	wrapped(conn, "A003", []string{"A003", "SEARCH", "ALL"}, state)

	if called {
		t.Error("Expected handler not to be called when not authenticated")
	}

	response := server.GetLastResponse()
	if !strings.Contains(response, "A003 NO Please authenticate first") {
		t.Errorf("Expected authentication error, got: %s", response)
	}
}

func TestRequireAuthAndMailbox_NoMailboxSelected(t *testing.T) {
	server := NewMockServer()
	conn := NewMockConn()
	called := false

	handler := func(conn net.Conn, tag string, parts []string, state *models.ClientState) {
		called = true
	}

	wrapped := middleware.RequireAuthAndMailbox(server, handler)

	state := &models.ClientState{
		Authenticated:     true,
		SelectedMailboxID: 0,
	}

	wrapped(conn, "A003", []string{"A003", "SEARCH", "ALL"}, state)

	if called {
		t.Error("Expected handler not to be called when no mailbox is selected")
	}

	response := server.GetLastResponse()
	if !strings.Contains(response, "A003 NO No folder selected") {
		t.Errorf("Expected no folder error, got: %s", response)
	}
}

func TestRequireAuthAndMailbox_NeitherValid(t *testing.T) {
	server := NewMockServer()
	conn := NewMockConn()
	called := false

	handler := func(conn net.Conn, tag string, parts []string, state *models.ClientState) {
		called = true
	}

	wrapped := middleware.RequireAuthAndMailbox(server, handler)

	state := &models.ClientState{
		Authenticated:     false,
		SelectedMailboxID: 0,
	}

	wrapped(conn, "A003", []string{"A003", "SEARCH", "ALL"}, state)

	if called {
		t.Error("Expected handler not to be called when neither auth nor mailbox are valid")
	}

	// Should fail on auth check first
	response := server.GetLastResponse()
	if !strings.Contains(response, "A003 NO Please authenticate first") {
		t.Errorf("Expected authentication error, got: %s", response)
	}
}

// ============================================================================
// ValidateMinArgs Tests
// ============================================================================

func TestValidateMinArgs_ValidArgs(t *testing.T) {
	server := NewMockServer()
	conn := NewMockConn()
	called := false

	handler := func(conn net.Conn, tag string, parts []string, state *models.ClientState) {
		called = true
	}

	wrapped := middleware.ValidateMinArgs(server, 3, "Command requires at least 3 arguments", handler)

	state := &models.ClientState{}

	wrapped(conn, "A004", []string{"A004", "SELECT", "INBOX"}, state)

	if !called {
		t.Error("Expected handler to be called with valid argument count")
	}

	if len(server.responses) > 0 {
		t.Errorf("Expected no error responses, got: %v", server.responses)
	}
}

func TestValidateMinArgs_InsufficientArgs(t *testing.T) {
	server := NewMockServer()
	conn := NewMockConn()
	called := false

	handler := func(conn net.Conn, tag string, parts []string, state *models.ClientState) {
		called = true
	}

	wrapped := middleware.ValidateMinArgs(server, 4, "Command requires at least 4 arguments", handler)

	state := &models.ClientState{}

	wrapped(conn, "A004", []string{"A004", "SELECT"}, state)

	if called {
		t.Error("Expected handler not to be called with insufficient arguments")
	}

	response := server.GetLastResponse()
	if !strings.Contains(response, "A004 BAD Command requires at least 4 arguments") {
		t.Errorf("Expected BAD argument error, got: %s", response)
	}
}

func TestValidateMinArgs_ExactMinimum(t *testing.T) {
	server := NewMockServer()
	conn := NewMockConn()
	called := false

	handler := func(conn net.Conn, tag string, parts []string, state *models.ClientState) {
		called = true
	}

	wrapped := middleware.ValidateMinArgs(server, 2, "Command requires at least 2 arguments", handler)

	state := &models.ClientState{}

	wrapped(conn, "A004", []string{"A004", "LIST"}, state)

	if !called {
		t.Error("Expected handler to be called with exact minimum arguments")
	}

	if len(server.responses) > 0 {
		t.Errorf("Expected no error responses, got: %v", server.responses)
	}
}

func TestValidateMinArgs_MoreThanMinimum(t *testing.T) {
	server := NewMockServer()
	conn := NewMockConn()
	called := false

	handler := func(conn net.Conn, tag string, parts []string, state *models.ClientState) {
		called = true
	}

	wrapped := middleware.ValidateMinArgs(server, 2, "Command requires at least 2 arguments", handler)

	state := &models.ClientState{}

	wrapped(conn, "A004", []string{"A004", "LIST", "INBOX", "SENT"}, state)

	if !called {
		t.Error("Expected handler to be called with more than minimum arguments")
	}

	if len(server.responses) > 0 {
		t.Errorf("Expected no error responses, got: %v", server.responses)
	}
}

// ============================================================================
// WithSelectedDB Tests
// ============================================================================

func TestWithSelectedDB_Success(t *testing.T) {
	server := NewMockServer()
	server.selectedDB = &sql.DB{} // Mock DB
	conn := NewMockConn()
	called := false
	var receivedDB *sql.DB

	handler := func(conn net.Conn, tag string, parts []string, state *models.ClientState, db *sql.DB) {
		called = true
		receivedDB = db
	}

	wrapped := middleware.WithSelectedDB(server, handler)

	state := &models.ClientState{
		UserID: 1,
	}

	wrapped(conn, "A005", []string{"A005", "FETCH", "1", "FLAGS"}, state)

	if !called {
		t.Error("Expected handler to be called when DB is available")
	}

	if receivedDB != server.selectedDB {
		t.Error("Expected handler to receive the correct database")
	}

	if len(server.responses) > 0 {
		t.Errorf("Expected no error responses, got: %v", server.responses)
	}
}

func TestWithSelectedDB_Error(t *testing.T) {
	server := NewMockServer()
	server.getSelectedDBError = errors.New("database error")
	conn := NewMockConn()
	called := false

	handler := func(conn net.Conn, tag string, parts []string, state *models.ClientState, db *sql.DB) {
		called = true
	}

	wrapped := middleware.WithSelectedDB(server, handler)

	state := &models.ClientState{
		UserID: 1,
	}

	wrapped(conn, "A005", []string{"A005", "FETCH", "1", "FLAGS"}, state)

	if called {
		t.Error("Expected handler not to be called when DB error occurs")
	}

	response := server.GetLastResponse()
	if !strings.Contains(response, "A005 NO Database error") {
		t.Errorf("Expected database error response, got: %s", response)
	}
}

// ============================================================================
// WithUserDB Tests
// ============================================================================

func TestWithUserDB_Success(t *testing.T) {
	server := NewMockServer()
	server.userDB = &sql.DB{} // Mock DB
	conn := NewMockConn()
	called := false
	var receivedDB *sql.DB

	handler := func(conn net.Conn, tag string, parts []string, state *models.ClientState, db *sql.DB) {
		called = true
		receivedDB = db
	}

	wrapped := middleware.WithUserDB(server, handler)

	state := &models.ClientState{
		UserID: 1,
	}

	wrapped(conn, "A006", []string{"A006", "CREATE", "INBOX"}, state)

	if !called {
		t.Error("Expected handler to be called when user DB is available")
	}

	if receivedDB != server.userDB {
		t.Error("Expected handler to receive the correct user database")
	}

	if len(server.responses) > 0 {
		t.Errorf("Expected no error responses, got: %v", server.responses)
	}
}

func TestWithUserDB_Error(t *testing.T) {
	server := NewMockServer()
	server.getUserDBError = errors.New("user database error")
	conn := NewMockConn()
	called := false

	handler := func(conn net.Conn, tag string, parts []string, state *models.ClientState, db *sql.DB) {
		called = true
	}

	wrapped := middleware.WithUserDB(server, handler)

	state := &models.ClientState{
		UserID: 1,
	}

	wrapped(conn, "A006", []string{"A006", "CREATE", "INBOX"}, state)

	if called {
		t.Error("Expected handler not to be called when user DB error occurs")
	}

	response := server.GetLastResponse()
	if !strings.Contains(response, "A006 NO Database error") {
		t.Errorf("Expected database error response, got: %s", response)
	}
}

// ============================================================================
// Integration Tests - Combining Middleware
// ============================================================================

func TestCombinedMiddleware_AuthAndValidation(t *testing.T) {
	server := NewMockServer()
	conn := NewMockConn()
	called := false

	handler := func(conn net.Conn, tag string, parts []string, state *models.ClientState) {
		called = true
	}

	// Combine RequireAuth and ValidateMinArgs
	wrapped := middleware.RequireAuth(
		server,
		middleware.ValidateMinArgs(server, 3, "Invalid command", handler),
	)

	// Test with authenticated user and valid args
	state := &models.ClientState{
		Authenticated: true,
	}

	wrapped(conn, "A007", []string{"A007", "SELECT", "INBOX"}, state)

	if !called {
		t.Error("Expected handler to be called with auth and valid args")
	}
}

func TestCombinedMiddleware_AuthFailsFirst(t *testing.T) {
	server := NewMockServer()
	conn := NewMockConn()
	called := false

	handler := func(conn net.Conn, tag string, parts []string, state *models.ClientState) {
		called = true
	}

	// Combine RequireAuth and ValidateMinArgs
	wrapped := middleware.RequireAuth(
		server,
		middleware.ValidateMinArgs(server, 3, "Invalid command", handler),
	)

	// Test with unauthenticated user (should fail before args check)
	state := &models.ClientState{
		Authenticated: false,
	}

	wrapped(conn, "A007", []string{"A007"}, state)

	if called {
		t.Error("Expected handler not to be called")
	}

	response := server.GetLastResponse()
	if !strings.Contains(response, "Please authenticate first") {
		t.Errorf("Expected auth error first, got: %s", response)
	}
}

func TestCombinedMiddleware_AllChecks(t *testing.T) {
	server := NewMockServer()
	server.selectedDB = &sql.DB{}
	conn := NewMockConn()
	called := false

	handler := func(conn net.Conn, tag string, parts []string, state *models.ClientState, db *sql.DB) {
		called = true
	}

	// Combine all middleware: Auth + Mailbox + Validation + DB
	wrapped := middleware.RequireAuthAndMailbox(
		server,
		middleware.ValidateMinArgs(
			server,
			3,
			"Invalid command",
			middleware.WithSelectedDB(server, handler),
		),
	)

	state := &models.ClientState{
		Authenticated:     true,
		SelectedMailboxID: 1,
		UserID:            1,
	}

	wrapped(conn, "A008", []string{"A008", "FETCH", "1:*", "FLAGS"}, state)

	if !called {
		t.Error("Expected handler to be called with all checks passing")
	}
}
