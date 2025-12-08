package message_test

import (
	"strings"
	"testing"

	"raven/internal/models"
	"raven/internal/server"
)

// ============================================================================
// Advanced SEARCH Tests - Cover search helper functions
// ============================================================================

// TestSearchCommand_HeaderExists tests SEARCH with HEADER
func TestSearchCommand_HeaderExists(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	database := server.GetDatabaseFromServer(srv)

	userID := server.CreateTestUser(t, database, "testuser")
	server.InsertTestMail(t, database, "testuser", "Test Subject", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := server.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
		SelectedFolder:    "INBOX",
	}

	// SEARCH HEADER
	srv.HandleSearch(conn, "S001", []string{"S001", "SEARCH", "HEADER", "Subject", "Test"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH") {
		t.Errorf("Expected SEARCH response, got: %s", response)
	}
	if !strings.Contains(response, "S001 OK") {
		t.Errorf("Expected OK response, got: %s", response)
	}
}

// TestSearchCommand_LargerSize tests SEARCH LARGER
func TestSearchCommand_LargerSize(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	database := server.GetDatabaseFromServer(srv)

	userID := server.CreateTestUser(t, database, "testuser")
	server.InsertTestMail(t, database, "testuser", "Test Subject", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := server.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
		SelectedFolder:    "INBOX",
	}

	// SEARCH LARGER
	srv.HandleSearch(conn, "S002", []string{"S002", "SEARCH", "LARGER", "10"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH") {
		t.Errorf("Expected SEARCH response, got: %s", response)
	}
	if !strings.Contains(response, "S002 OK") {
		t.Errorf("Expected OK response, got: %s", response)
	}
}

// TestSearchCommand_SmallerSize tests SEARCH SMALLER
func TestSearchCommand_SmallerSize(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	database := server.GetDatabaseFromServer(srv)

	userID := server.CreateTestUser(t, database, "testuser")
	server.InsertTestMail(t, database, "testuser", "Test Subject", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := server.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
		SelectedFolder:    "INBOX",
	}

	// SEARCH SMALLER
	srv.HandleSearch(conn, "S003", []string{"S003", "SEARCH", "SMALLER", "999999"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH") {
		t.Errorf("Expected SEARCH response, got: %s", response)
	}
	if !strings.Contains(response, "S003 OK") {
		t.Errorf("Expected OK response, got: %s", response)
	}
}

// TestSearchCommand_BeforeDate tests SEARCH BEFORE
func TestSearchCommand_BeforeDate(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	database := server.GetDatabaseFromServer(srv)

	userID := server.CreateTestUser(t, database, "testuser")
	server.InsertTestMail(t, database, "testuser", "Test Subject", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := server.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
		SelectedFolder:    "INBOX",
	}

	// SEARCH BEFORE
	srv.HandleSearch(conn, "S004", []string{"S004", "SEARCH", "BEFORE", "31-Dec-2099"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH") {
		t.Errorf("Expected SEARCH response, got: %s", response)
	}
	if !strings.Contains(response, "S004 OK") {
		t.Errorf("Expected OK response, got: %s", response)
	}
}

// TestSearchCommand_OnDate tests SEARCH ON
func TestSearchCommand_OnDate(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	database := server.GetDatabaseFromServer(srv)

	userID := server.CreateTestUser(t, database, "testuser")
	server.InsertTestMail(t, database, "testuser", "Test Subject", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := server.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
		SelectedFolder:    "INBOX",
	}

	// SEARCH ON (today's date)
	srv.HandleSearch(conn, "S005", []string{"S005", "SEARCH", "ON", "24-Nov-2025"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH") {
		t.Errorf("Expected SEARCH response, got: %s", response)
	}
	if !strings.Contains(response, "S005 OK") {
		t.Errorf("Expected OK response, got: %s", response)
	}
}

// TestSearchCommand_SinceDate tests SEARCH SINCE
func TestSearchCommand_SinceDate(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	database := server.GetDatabaseFromServer(srv)

	userID := server.CreateTestUser(t, database, "testuser")
	server.InsertTestMail(t, database, "testuser", "Test Subject", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := server.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
		SelectedFolder:    "INBOX",
	}

	// SEARCH SINCE
	srv.HandleSearch(conn, "S006", []string{"S006", "SEARCH", "SINCE", "1-Jan-2020"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH") {
		t.Errorf("Expected SEARCH response, got: %s", response)
	}
	if !strings.Contains(response, "S006 OK") {
		t.Errorf("Expected OK response, got: %s", response)
	}
}

// TestSearchCommand_SentBefore tests SEARCH SENTBEFORE
func TestSearchCommand_SentBefore(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	database := server.GetDatabaseFromServer(srv)

	userID := server.CreateTestUser(t, database, "testuser")
	server.InsertTestMail(t, database, "testuser", "Test Subject", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := server.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
		SelectedFolder:    "INBOX",
	}

	// SEARCH SENTBEFORE
	srv.HandleSearch(conn, "S007", []string{"S007", "SEARCH", "SENTBEFORE", "31-Dec-2099"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH") {
		t.Errorf("Expected SEARCH response, got: %s", response)
	}
	if !strings.Contains(response, "S007 OK") {
		t.Errorf("Expected OK response, got: %s", response)
	}
}

// TestSearchCommand_SentOn tests SEARCH SENTON
func TestSearchCommand_SentOn(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	database := server.GetDatabaseFromServer(srv)

	userID := server.CreateTestUser(t, database, "testuser")
	server.InsertTestMail(t, database, "testuser", "Test Subject", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := server.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
		SelectedFolder:    "INBOX",
	}

	// SEARCH SENTON
	srv.HandleSearch(conn, "S008", []string{"S008", "SEARCH", "SENTON", "24-Nov-2025"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH") {
		t.Errorf("Expected SEARCH response, got: %s", response)
	}
	if !strings.Contains(response, "S008 OK") {
		t.Errorf("Expected OK response, got: %s", response)
	}
}

// TestSearchCommand_SentSince tests SEARCH SENTSINCE
func TestSearchCommand_SentSince(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	database := server.GetDatabaseFromServer(srv)

	userID := server.CreateTestUser(t, database, "testuser")
	server.InsertTestMail(t, database, "testuser", "Test Subject", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := server.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
		SelectedFolder:    "INBOX",
	}

	// SEARCH SENTSINCE
	srv.HandleSearch(conn, "S009", []string{"S009", "SEARCH", "SENTSINCE", "1-Jan-2020"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH") {
		t.Errorf("Expected SEARCH response, got: %s", response)
	}
	if !strings.Contains(response, "S009 OK") {
		t.Errorf("Expected OK response, got: %s", response)
	}
}

// TestSearchCommand_TextSearch tests SEARCH TEXT
func TestSearchCommand_TextSearch(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	database := server.GetDatabaseFromServer(srv)

	userID := server.CreateTestUser(t, database, "testuser")
	server.InsertTestMail(t, database, "testuser", "Test Subject", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := server.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
		SelectedFolder:    "INBOX",
	}

	// SEARCH TEXT
	srv.HandleSearch(conn, "S010", []string{"S010", "SEARCH", "TEXT", "Test"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH") {
		t.Errorf("Expected SEARCH response, got: %s", response)
	}
	if !strings.Contains(response, "S010 OK") {
		t.Errorf("Expected OK response, got: %s", response)
	}
}

// TestSearchCommand_BodySearch tests SEARCH BODY
func TestSearchCommand_BodySearch(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	database := server.GetDatabaseFromServer(srv)

	userID := server.CreateTestUser(t, database, "testuser")
	server.InsertTestMail(t, database, "testuser", "Test Subject", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := server.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
		SelectedFolder:    "INBOX",
	}

	// SEARCH BODY
	srv.HandleSearch(conn, "S011", []string{"S011", "SEARCH", "BODY", "test"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH") {
		t.Errorf("Expected SEARCH response, got: %s", response)
	}
	if !strings.Contains(response, "S011 OK") {
		t.Errorf("Expected OK response, got: %s", response)
	}
}

// TestSearchCommand_NotCriteria tests SEARCH NOT
func TestSearchCommand_NotCriteria(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	database := server.GetDatabaseFromServer(srv)

	userID := server.CreateTestUser(t, database, "testuser")
	server.InsertTestMail(t, database, "testuser", "Test Subject", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := server.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
		SelectedFolder:    "INBOX",
	}

	// SEARCH NOT DELETED
	srv.HandleSearch(conn, "S012", []string{"S012", "SEARCH", "NOT", "DELETED"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH") {
		t.Errorf("Expected SEARCH response, got: %s", response)
	}
	if !strings.Contains(response, "S012 OK") {
		t.Errorf("Expected OK response, got: %s", response)
	}
}

// TestSearchCommand_OrCriteria tests SEARCH OR
func TestSearchCommand_OrCriteria(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	database := server.GetDatabaseFromServer(srv)

	userID := server.CreateTestUser(t, database, "testuser")
	server.InsertTestMail(t, database, "testuser", "Test Subject", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := server.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
		SelectedFolder:    "INBOX",
	}

	// SEARCH OR SEEN DELETED
	srv.HandleSearch(conn, "S013", []string{"S013", "SEARCH", "OR", "SEEN", "DELETED"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH") {
		t.Errorf("Expected SEARCH response, got: %s", response)
	}
	if !strings.Contains(response, "S013 OK") {
		t.Errorf("Expected OK response, got: %s", response)
	}
}

// ============================================================================
// SEARCH with HEADER (tests hasHeader function)
// ============================================================================

// TestSearchCommand_HeaderWithoutValue tests SEARCH with HEADER field only (no value)
func TestSearchCommand_HeaderWithoutValue(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	database := server.GetDatabaseFromServer(srv)

	userID := server.CreateTestUser(t, database, "testuser")
	server.InsertTestMail(t, database, "testuser", "Test Subject", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := server.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
		SelectedFolder:    "INBOX",
	}

	// SEARCH HEADER with empty value tests hasHeader function
	srv.HandleSearch(conn, "H001", []string{"H001", "SEARCH", "HEADER", "Subject", `""`}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH") {
		t.Errorf("Expected SEARCH response, got: %s", response)
	}
	if !strings.Contains(response, "H001 OK") {
		t.Errorf("Expected OK response, got: %s", response)
	}
}

// TestSearchCommand_HeaderExistence tests searching for existence of a header
func TestSearchCommand_HeaderExistence(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	database := server.GetDatabaseFromServer(srv)

	userID := server.CreateTestUser(t, database, "testuser")
	server.InsertTestMail(t, database, "testuser", "Test Subject", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := server.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
		SelectedFolder:    "INBOX",
	}

	// SEARCH for existence of From header (empty search string)
	srv.HandleSearch(conn, "H002", []string{"H002", "SEARCH", "HEADER", "From", `""`}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH 1") {
		t.Errorf("Expected message to match (From header exists), got: %s", response)
	}
}

// TestSearchCommand_NonExistentHeader tests searching for non-existent header
func TestSearchCommand_NonExistentHeader(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	database := server.GetDatabaseFromServer(srv)

	userID := server.CreateTestUser(t, database, "testuser")
	server.InsertTestMail(t, database, "testuser", "Test Subject", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := server.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
		SelectedFolder:    "INBOX",
	}

	// SEARCH for non-existent header
	srv.HandleSearch(conn, "H003", []string{"H003", "SEARCH", "HEADER", "X-NonExistent", `""`}, state)

	response := conn.GetWrittenData()
	// Should return empty search result
	if !strings.Contains(response, "* SEARCH") {
		t.Errorf("Expected SEARCH response, got: %s", response)
	}
}

// ============================================================================
// Additional Coverage Tests
// ============================================================================

// TestSearchCommand_SequenceRange tests various sequence set formats
func TestSearchCommand_SequenceRange(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	database := server.GetDatabaseFromServer(srv)

	userID := server.CreateTestUser(t, database, "testuser")
	server.InsertTestMail(t, database, "testuser", "Test 1", "sender@test.com", "testuser@localhost", "INBOX")
	server.InsertTestMail(t, database, "testuser", "Test 2", "sender@test.com", "testuser@localhost", "INBOX")
	server.InsertTestMail(t, database, "testuser", "Test 3", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := server.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
		SelectedFolder:    "INBOX",
	}

	// Test sequence range
	srv.HandleSearch(conn, "SEQ001", []string{"SEQ001", "SEARCH", "1:2"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH") {
		t.Errorf("Expected SEARCH response, got: %s", response)
	}
}

// TestSearchCommand_UIDSet tests UID set in SEARCH
func TestSearchCommand_UIDSet(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	database := server.GetDatabaseFromServer(srv)

	userID := server.CreateTestUser(t, database, "testuser")
	server.InsertTestMail(t, database, "testuser", "Test 1", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := server.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
		SelectedFolder:    "INBOX",
	}

	// Test UID search
	srv.HandleSearch(conn, "UID001", []string{"UID001", "SEARCH", "UID", "1"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH") {
		t.Errorf("Expected SEARCH response, got: %s", response)
	}
}

// TestSearchCommand_Keyword tests SEARCH KEYWORD
func TestSearchCommand_Keyword(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	database := server.GetDatabaseFromServer(srv)

	userID := server.CreateTestUser(t, database, "testuser")
	server.InsertTestMail(t, database, "testuser", "Test", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := server.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
		SelectedFolder:    "INBOX",
	}

	// SEARCH KEYWORD (tests requiresArgument function)
	srv.HandleSearch(conn, "K001", []string{"K001", "SEARCH", "KEYWORD", "test"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH") {
		t.Errorf("Expected SEARCH response, got: %s", response)
	}
}

// TestSearchCommand_UnKeyword tests SEARCH UNKEYWORD
func TestSearchCommand_UnKeyword(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	database := server.GetDatabaseFromServer(srv)

	userID := server.CreateTestUser(t, database, "testuser")
	server.InsertTestMail(t, database, "testuser", "Test", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := server.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
		SelectedFolder:    "INBOX",
	}

	// SEARCH UNKEYWORD
	srv.HandleSearch(conn, "K002", []string{"K002", "SEARCH", "UNKEYWORD", "test"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH") {
		t.Errorf("Expected SEARCH response, got: %s", response)
	}
}

// TestSearchCommand_BCC tests SEARCH BCC
func TestSearchCommand_BCC(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	database := server.GetDatabaseFromServer(srv)

	userID := server.CreateTestUser(t, database, "testuser")
	server.InsertTestMail(t, database, "testuser", "Test", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := server.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
		SelectedFolder:    "INBOX",
	}

	// SEARCH BCC
	srv.HandleSearch(conn, "B001", []string{"B001", "SEARCH", "BCC", "test@example.com"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH") {
		t.Errorf("Expected SEARCH response, got: %s", response)
	}
}

// TestSearchCommand_CC tests SEARCH CC
func TestSearchCommand_CC(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	database := server.GetDatabaseFromServer(srv)

	userID := server.CreateTestUser(t, database, "testuser")
	server.InsertTestMail(t, database, "testuser", "Test", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := server.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
		SelectedFolder:    "INBOX",
	}

	// SEARCH CC
	srv.HandleSearch(conn, "C001", []string{"C001", "SEARCH", "CC", "test@example.com"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH") {
		t.Errorf("Expected SEARCH response, got: %s", response)
	}
}

// TestSearchCommand_From tests SEARCH FROM
func TestSearchCommand_From(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	database := server.GetDatabaseFromServer(srv)

	userID := server.CreateTestUser(t, database, "testuser")
	server.InsertTestMail(t, database, "testuser", "Test", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := server.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
		SelectedFolder:    "INBOX",
	}

	// SEARCH FROM
	srv.HandleSearch(conn, "F001", []string{"F001", "SEARCH", "FROM", "sender"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH 1") {
		t.Errorf("Expected message to match, got: %s", response)
	}
}

// TestSearchCommand_To tests SEARCH TO
func TestSearchCommand_To(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	database := server.GetDatabaseFromServer(srv)

	userID := server.CreateTestUser(t, database, "testuser")
	server.InsertTestMail(t, database, "testuser", "Test", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := server.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
		SelectedFolder:    "INBOX",
	}

	// SEARCH TO
	srv.HandleSearch(conn, "T001", []string{"T001", "SEARCH", "TO", "testuser"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH 1") {
		t.Errorf("Expected message to match, got: %s", response)
	}
}

// TestSearchCommand_Subject tests SEARCH SUBJECT
func TestSearchCommand_Subject(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	database := server.GetDatabaseFromServer(srv)

	userID := server.CreateTestUser(t, database, "testuser")
	server.InsertTestMail(t, database, "testuser", "Important Message", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := server.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
		SelectedFolder:    "INBOX",
	}

	// SEARCH SUBJECT
	srv.HandleSearch(conn, "SU001", []string{"SU001", "SEARCH", "SUBJECT", "Important"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH 1") {
		t.Errorf("Expected message to match, got: %s", response)
	}
}

// ============================================================================
// FETCH Edge Cases - Improve coverage of processFetchForMessage
// ============================================================================

// TestFetchCommand_BodyPeekHeaderFields tests BODY.PEEK[HEADER.FIELDS (...)]
func TestFetchCommand_BodyPeekHeaderFields(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	database := server.GetDatabaseFromServer(srv)

	userID := server.CreateTestUser(t, database, "testuser")
	server.InsertTestMail(t, database, "testuser", "Test Subject", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := server.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}

	srv.HandleFetch(conn, "F101", []string{"F101", "FETCH", "1", "BODY.PEEK[HEADER.FIELDS (FROM SUBJECT)]"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "BODY[HEADER.FIELDS") {
		t.Errorf("Expected BODY[HEADER.FIELDS in response, got: %s", response)
	}
}

// TestFetchCommand_BodyPartial tests BODY[]<partial>
func TestFetchCommand_BodyPartial(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	database := server.GetDatabaseFromServer(srv)

	userID := server.CreateTestUser(t, database, "testuser")
	server.InsertTestMail(t, database, "testuser", "Test Subject", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := server.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}

	srv.HandleFetch(conn, "F102", []string{"F102", "FETCH", "1", "BODY[]<0.100>"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "BODY[]") {
		t.Errorf("Expected BODY[] in response, got: %s", response)
	}
}

// TestFetchCommand_BodyPart tests BODY[n] for multipart messages
func TestFetchCommand_BodyPart(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	database := server.GetDatabaseFromServer(srv)

	userID := server.CreateTestUser(t, database, "testuser")
	server.InsertTestMail(t, database, "testuser", "Test Subject", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := server.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}

	srv.HandleFetch(conn, "F103", []string{"F103", "FETCH", "1", "BODY[1]"}, state)

	response := conn.GetWrittenData()
	// May or may not have part 1, but should handle gracefully
	if !strings.Contains(response, "F103") {
		t.Errorf("Expected response with tag, got: %s", response)
	}
}
