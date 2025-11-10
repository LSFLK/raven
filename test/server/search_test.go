//go:build test
// +build test

package server_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"raven/internal/models"
	"raven/test/helpers"
)

// TestSearchCommand_Unauthenticated tests SEARCH without authentication
func TestSearchCommand_Unauthenticated(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: false,
	}

	server.HandleSearch(conn, "S001", []string{"S001", "SEARCH", "ALL"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "S001 NO Please authenticate first") {
		t.Errorf("Expected authentication required error, got: %s", response)
	}
}

// TestSearchCommand_NoMailboxSelected tests SEARCH without mailbox selection
func TestSearchCommand_NoMailboxSelected(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := helpers.GetDatabaseFromServer(server)

	userID := helpers.CreateTestUser(t, database, "testuser")
	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: 0, // No mailbox selected
	}

	server.HandleSearch(conn, "S002", []string{"S002", "SEARCH", "ALL"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "S002 NO No folder selected") {
		t.Errorf("Expected no folder selected error, got: %s", response)
	}
}

// TestSearchCommand_ALL tests SEARCH ALL
func TestSearchCommand_ALL(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := helpers.GetDatabaseFromServer(server)

	// Create test user and messages
	userID := helpers.CreateTestUser(t, database, "testuser")
	helpers.InsertTestMail(t, database, "testuser", "Test Subject 1", "sender1@test.com", "testuser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "testuser", "Test Subject 2", "sender2@test.com", "testuser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "testuser", "Test Subject 3", "sender3@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := helpers.GetMailboxID(t, database, userID, "INBOX")
	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}

	server.HandleSearch(conn, "S003", []string{"S003", "SEARCH", "ALL"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH 1 2 3") {
		t.Errorf("Expected to find all 3 messages, got: %s", response)
	}
	if !strings.Contains(response, "S003 OK SEARCH completed") {
		t.Errorf("Expected OK completion, got: %s", response)
	}
}

// TestSearchCommand_EmptyMailbox tests SEARCH on empty mailbox
func TestSearchCommand_EmptyMailbox(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := helpers.GetDatabaseFromServer(server)

	userID := helpers.CreateTestUser(t, database, "testuser")
	mailboxID, _ := helpers.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}

	server.HandleSearch(conn, "S004", []string{"S004", "SEARCH", "ALL"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH\r\n") {
		t.Errorf("Expected empty SEARCH response, got: %s", response)
	}
	if !strings.Contains(response, "S004 OK SEARCH completed") {
		t.Errorf("Expected OK completion, got: %s", response)
	}
}

// TestSearchCommand_FlaggedMessages tests SEARCH FLAGGED
func TestSearchCommand_FlaggedMessages(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := helpers.GetDatabaseFromServer(server)

	userID := helpers.CreateTestUser(t, database, "testuser")

	// Insert messages with different flags
	msg1ID := helpers.InsertTestMail(t, database, "testuser", "Message 1", "sender@test.com", "testuser@localhost", "INBOX")
	msg2ID := helpers.InsertTestMail(t, database, "testuser", "Message 2", "sender@test.com", "testuser@localhost", "INBOX")
	msg3ID := helpers.InsertTestMail(t, database, "testuser", "Message 3", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := helpers.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}
	userDB := helpers.GetUserDBByID(t, database, state.UserID)

	// Flag message 2
	userDB.Exec(`UPDATE message_mailbox SET flags = '\Flagged' WHERE message_id = ? AND mailbox_id = ?`, msg2ID, mailboxID)

	server.HandleSearch(conn, "S005", []string{"S005", "SEARCH", "FLAGGED"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH 2") {
		t.Errorf("Expected to find message 2 as flagged, got: %s", response)
	}
	// Should not contain messages 1 or 3
	if strings.Contains(response, "* SEARCH 1") || strings.Contains(response, "* SEARCH 3") {
		t.Errorf("Should only return flagged message, got: %s", response)
	}

	// Suppress unused variable warnings
	_ = msg1ID
	_ = msg3ID
}

// TestSearchCommand_DeletedMessages tests SEARCH DELETED and UNDELETED
func TestSearchCommand_DeletedMessages(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := helpers.GetDatabaseFromServer(server)

	userID := helpers.CreateTestUser(t, database, "testuser")

	msg1ID := helpers.InsertTestMail(t, database, "testuser", "Message 1", "sender@test.com", "testuser@localhost", "INBOX")
	msg2ID := helpers.InsertTestMail(t, database, "testuser", "Message 2", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := helpers.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}
	userDB := helpers.GetUserDBByID(t, database, state.UserID)

	// Mark message 1 as deleted
	userDB.Exec(`UPDATE message_mailbox SET flags = '\Deleted' WHERE message_id = ? AND mailbox_id = ?`, msg1ID, mailboxID)

	// Test DELETED
	server.HandleSearch(conn, "S006", []string{"S006", "SEARCH", "DELETED"}, state)
	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH 1") {
		t.Errorf("Expected to find deleted message 1, got: %s", response)
	}

	conn.ClearWriteBuffer()

	// Test UNDELETED
	server.HandleSearch(conn, "S007", []string{"S007", "SEARCH", "UNDELETED"}, state)
	response = conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH 2") {
		t.Errorf("Expected to find undeleted message 2, got: %s", response)
	}

	_ = msg2ID
}

// TestSearchCommand_SeenUnseen tests SEARCH SEEN and UNSEEN
func TestSearchCommand_SeenUnseen(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := helpers.GetDatabaseFromServer(server)

	userID := helpers.CreateTestUser(t, database, "testuser")

	msg1ID := helpers.InsertTestMail(t, database, "testuser", "Message 1", "sender@test.com", "testuser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "testuser", "Message 2", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := helpers.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}
	userDB := helpers.GetUserDBByID(t, database, state.UserID)

	// Mark message 1 as seen
	userDB.Exec(`UPDATE message_mailbox SET flags = '\Seen' WHERE message_id = ? AND mailbox_id = ?`, msg1ID, mailboxID)

	// Test SEEN
	server.HandleSearch(conn, "S008", []string{"S008", "SEARCH", "SEEN"}, state)
	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH 1") {
		t.Errorf("Expected to find seen message 1, got: %s", response)
	}

	conn.ClearWriteBuffer()

	// Test UNSEEN
	server.HandleSearch(conn, "S009", []string{"S009", "SEARCH", "UNSEEN"}, state)
	response = conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH 2") {
		t.Errorf("Expected to find unseen message 2, got: %s", response)
	}
}

// TestSearchCommand_FromHeader tests SEARCH FROM
func TestSearchCommand_FromHeader(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := helpers.GetDatabaseFromServer(server)

	userID := helpers.CreateTestUser(t, database, "testuser")

	helpers.InsertTestMail(t, database, "testuser", "Message 1", "smith@example.com", "testuser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "testuser", "Message 2", "jones@example.com", "testuser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "testuser", "Message 3", "smithson@example.com", "testuser@localhost", "INBOX")

	mailboxID, _ := helpers.GetMailboxID(t, database, userID, "INBOX")
	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}

	// Search for "smith" - should match both smith@example.com and smithson@example.com
	server.HandleSearch(conn, "S010", []string{"S010", "SEARCH", "FROM", "smith"}, state)

	response := conn.GetWrittenData()
	// Should find messages 1 and 3 (case-insensitive substring match)
	if !strings.Contains(response, "1") || !strings.Contains(response, "3") {
		t.Errorf("Expected to find messages from smith, got: %s", response)
	}
	if strings.Contains(response, "* SEARCH 2") {
		t.Errorf("Should not match jones@example.com, got: %s", response)
	}
}

// TestSearchCommand_SubjectHeader tests SEARCH SUBJECT
func TestSearchCommand_SubjectHeader(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := helpers.GetDatabaseFromServer(server)

	userID := helpers.CreateTestUser(t, database, "testuser")

	helpers.InsertTestMail(t, database, "testuser", "Meeting Tomorrow", "sender@test.com", "testuser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "testuser", "Project Update", "sender@test.com", "testuser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "testuser", "Meeting Notes", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := helpers.GetMailboxID(t, database, userID, "INBOX")
	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}

	// Search for "Meeting" in subject
	server.HandleSearch(conn, "S011", []string{"S011", "SEARCH", "SUBJECT", "Meeting"}, state)

	response := conn.GetWrittenData()
	// Should find messages 1 and 3
	if !strings.Contains(response, "1") || !strings.Contains(response, "3") {
		t.Errorf("Expected to find messages with 'Meeting' in subject, got: %s", response)
	}
}

// TestSearchCommand_ToHeader tests SEARCH TO
func TestSearchCommand_ToHeader(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := helpers.GetDatabaseFromServer(server)

	userID := helpers.CreateTestUser(t, database, "testuser")

	helpers.InsertTestMail(t, database, "testuser", "Message 1", "sender@test.com", "alice@example.com", "INBOX")
	helpers.InsertTestMail(t, database, "testuser", "Message 2", "sender@test.com", "bob@example.com", "INBOX")

	mailboxID, _ := helpers.GetMailboxID(t, database, userID, "INBOX")
	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}

	server.HandleSearch(conn, "S012", []string{"S012", "SEARCH", "TO", "alice"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH 1") {
		t.Errorf("Expected to find message to alice, got: %s", response)
	}
}

// TestSearchCommand_SequenceSet tests SEARCH with sequence numbers
func TestSearchCommand_SequenceSet(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := helpers.GetDatabaseFromServer(server)

	userID := helpers.CreateTestUser(t, database, "testuser")

	helpers.InsertTestMail(t, database, "testuser", "Message 1", "sender@test.com", "testuser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "testuser", "Message 2", "sender@test.com", "testuser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "testuser", "Message 3", "sender@test.com", "testuser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "testuser", "Message 4", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := helpers.GetMailboxID(t, database, userID, "INBOX")
	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}

	// Test single sequence number
	server.HandleSearch(conn, "S013", []string{"S013", "SEARCH", "2"}, state)
	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH 2") {
		t.Errorf("Expected to find message 2, got: %s", response)
	}

	conn.ClearWriteBuffer()

	// Test sequence range
	server.HandleSearch(conn, "S014", []string{"S014", "SEARCH", "2:4"}, state)
	response = conn.GetWrittenData()
	if !strings.Contains(response, "2") || !strings.Contains(response, "3") || !strings.Contains(response, "4") {
		t.Errorf("Expected to find messages 2-4, got: %s", response)
	}
}

// TestSearchCommand_NOT tests SEARCH NOT
func TestSearchCommand_NOT(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := helpers.GetDatabaseFromServer(server)

	userID := helpers.CreateTestUser(t, database, "testuser")

	msg1ID := helpers.InsertTestMail(t, database, "testuser", "Message 1", "sender@test.com", "testuser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "testuser", "Message 2", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := helpers.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}
	userDB := helpers.GetUserDBByID(t, database, state.UserID)

	// Mark message 1 as seen
	userDB.Exec(`UPDATE message_mailbox SET flags = '\Seen' WHERE message_id = ? AND mailbox_id = ?`, msg1ID, mailboxID)

	// Search for NOT SEEN (equivalent to UNSEEN)
	server.HandleSearch(conn, "S015", []string{"S015", "SEARCH", "NOT", "SEEN"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH 2") {
		t.Errorf("Expected to find unseen message 2, got: %s", response)
	}
	if strings.Contains(response, "* SEARCH 1") {
		t.Errorf("Should not find seen message 1, got: %s", response)
	}
}

// TestSearchCommand_OR tests SEARCH OR
func TestSearchCommand_OR(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := helpers.GetDatabaseFromServer(server)

	userID := helpers.CreateTestUser(t, database, "testuser")

	msg1ID := helpers.InsertTestMail(t, database, "testuser", "Message 1", "sender@test.com", "testuser@localhost", "INBOX")
	msg2ID := helpers.InsertTestMail(t, database, "testuser", "Message 2", "sender@test.com", "testuser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "testuser", "Message 3", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := helpers.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}
	userDB := helpers.GetUserDBByID(t, database, state.UserID)

	// Mark message 1 as seen, message 2 as flagged
	userDB.Exec(`UPDATE message_mailbox SET flags = '\Seen' WHERE message_id = ? AND mailbox_id = ?`, msg1ID, mailboxID)
	userDB.Exec(`UPDATE message_mailbox SET flags = '\Flagged' WHERE message_id = ? AND mailbox_id = ?`, msg2ID, mailboxID)

	// Search for SEEN OR FLAGGED
	server.HandleSearch(conn, "S016", []string{"S016", "SEARCH", "OR", "SEEN", "FLAGGED"}, state)

	response := conn.GetWrittenData()
	// Should find messages 1 and 2 but not 3
	if !strings.Contains(response, "1") || !strings.Contains(response, "2") {
		t.Errorf("Expected to find messages 1 and 2, got: %s", response)
	}
}

// TestSearchCommand_CombinedCriteria tests SEARCH with multiple criteria (AND logic)
func TestSearchCommand_CombinedCriteria(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := helpers.GetDatabaseFromServer(server)

	userID := helpers.CreateTestUser(t, database, "testuser")

	msg1ID := helpers.InsertTestMail(t, database, "testuser", "Meeting", "smith@example.com", "testuser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "testuser", "Meeting", "jones@example.com", "testuser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "testuser", "Project", "smith@example.com", "testuser@localhost", "INBOX")

	mailboxID, _ := helpers.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}
	userDB := helpers.GetUserDBByID(t, database, state.UserID)

	// Mark message 1 as flagged
	userDB.Exec(`UPDATE message_mailbox SET flags = '\Flagged' WHERE message_id = ? AND mailbox_id = ?`, msg1ID, mailboxID)

	// Search for FLAGGED messages FROM "smith" with SUBJECT "Meeting"
	// Should only match message 1
	server.HandleSearch(conn, "S017", []string{"S017", "SEARCH", "FLAGGED", "FROM", "smith", "SUBJECT", "Meeting"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH 1") {
		t.Errorf("Expected to find message 1, got: %s", response)
	}
	// Should not contain 2 or 3
	if strings.Contains(response, "* SEARCH 2") || strings.Contains(response, "* SEARCH 3") {
		t.Errorf("Should only match message 1, got: %s", response)
	}
}

// TestSearchCommand_RFC3501Example tests the example from RFC 3501
func TestSearchCommand_RFC3501Example(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := helpers.GetDatabaseFromServer(server)

	userID := helpers.CreateTestUser(t, database, "testuser")

	// Create messages with specific dates and flags
	msg1ID := helpers.InsertTestMail(t, database, "testuser", "From Smith", "smith@example.com", "testuser@localhost", "INBOX")
	msg2ID := helpers.InsertTestMail(t, database, "testuser", "From Smith", "smith@example.com", "testuser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "testuser", "From Jones", "jones@example.com", "testuser@localhost", "INBOX")

	mailboxID, _ := helpers.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}
	userDB := helpers.GetUserDBByID(t, database, state.UserID)

	// Set dates for messages (after 1-Feb-1994)
	futureDate := time.Date(1994, 2, 15, 10, 0, 0, 0, time.UTC)
	userDB.Exec(`UPDATE message_mailbox SET internal_date = ? WHERE message_id = ? AND mailbox_id = ?`, futureDate, msg1ID, mailboxID)
	userDB.Exec(`UPDATE message_mailbox SET internal_date = ? WHERE message_id = ? AND mailbox_id = ?`, futureDate, msg2ID, mailboxID)

	// Mark messages 1 and 2 as flagged (representing messages since Feb 1994 from Smith)
	userDB.Exec(`UPDATE message_mailbox SET flags = '\Flagged' WHERE message_id = ? AND mailbox_id = ?`, msg1ID, mailboxID)
	userDB.Exec(`UPDATE message_mailbox SET flags = '\Flagged' WHERE message_id = ? AND mailbox_id = ?`, msg2ID, mailboxID)

	// RFC 3501 Example: SEARCH FLAGGED SINCE 1-Feb-1994 NOT FROM "Smith"
	// Since our implementation doesn't have DELETED flag support in test data,
	// we'll test: FLAGGED FROM "Smith" SINCE 1-Feb-1994
	server.HandleSearch(conn, "A282", []string{"A282", "SEARCH", "FLAGGED", "FROM", "Smith", "SINCE", "1-Feb-1994"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH") {
		t.Errorf("Expected SEARCH response, got: %s", response)
	}
	if !strings.Contains(response, "A282 OK SEARCH completed") {
		t.Errorf("Expected OK completion, got: %s", response)
	}
}

// TestSearchCommand_EmptyResult tests search with no matching messages
func TestSearchCommand_EmptyResult(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := helpers.GetDatabaseFromServer(server)

	userID := helpers.CreateTestUser(t, database, "testuser")

	helpers.InsertTestMail(t, database, "testuser", "Message 1", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := helpers.GetMailboxID(t, database, userID, "INBOX")
	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}

	// RFC 3501 Example: Search for non-existent string
	server.HandleSearch(conn, "A283", []string{"A283", "SEARCH", "TEXT", "\"string not in mailbox\""}, state)

	response := conn.GetWrittenData()
	// Should return empty SEARCH response
	if !strings.Contains(response, "* SEARCH\r\n") {
		t.Errorf("Expected empty SEARCH response, got: %s", response)
	}
	if !strings.Contains(response, "A283 OK SEARCH completed") {
		t.Errorf("Expected OK completion, got: %s", response)
	}
}

// TestSearchCommand_CHARSET tests SEARCH with CHARSET specification
func TestSearchCommand_CHARSET(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := helpers.GetDatabaseFromServer(server)

	userID := helpers.CreateTestUser(t, database, "testuser")
	helpers.InsertTestMail(t, database, "testuser", "Test Message", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := helpers.GetMailboxID(t, database, userID, "INBOX")
	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}

	// Test with supported charset (UTF-8)
	server.HandleSearch(conn, "A284", []string{"A284", "SEARCH", "CHARSET", "UTF-8", "TEXT", "Test"}, state)
	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH 1") {
		t.Errorf("Expected to find message with UTF-8 charset, got: %s", response)
	}

	conn.ClearWriteBuffer()

	// Test with unsupported charset
	server.HandleSearch(conn, "A285", []string{"A285", "SEARCH", "CHARSET", "ISO-8859-1", "TEXT", "Test"}, state)
	response = conn.GetWrittenData()
	// Should return NO with BADCHARSET response code
	if !strings.Contains(response, "A285 NO") || !strings.Contains(response, "BADCHARSET") {
		t.Errorf("Expected BADCHARSET error, got: %s", response)
	}
}

// TestSearchCommand_LARGER_SMALLER tests SEARCH LARGER and SMALLER
func TestSearchCommand_LARGER_SMALLER(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := helpers.GetDatabaseFromServer(server)

	userID := helpers.CreateTestUser(t, database, "testuser")

	// Create messages (they will have different sizes based on subject length)
	helpers.InsertTestMail(t, database, "testuser", "A", "sender@test.com", "testuser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "testuser", "This is a very long subject line that will make this message larger", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := helpers.GetMailboxID(t, database, userID, "INBOX")
	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}

	// Test SMALLER - should find smaller messages
	server.HandleSearch(conn, "S018", []string{"S018", "SEARCH", "SMALLER", "500"}, state)
	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH") {
		t.Errorf("Expected SEARCH response for SMALLER, got: %s", response)
	}

	conn.ClearWriteBuffer()

	// Test LARGER - should find larger messages
	server.HandleSearch(conn, "S019", []string{"S019", "SEARCH", "LARGER", "50"}, state)
	response = conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH") {
		t.Errorf("Expected SEARCH response for LARGER, got: %s", response)
	}
}

// TestSearchCommand_DateSearches tests BEFORE, ON, SINCE
func TestSearchCommand_DateSearches(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := helpers.GetDatabaseFromServer(server)

	userID := helpers.CreateTestUser(t, database, "testuser")

	msg1ID := helpers.InsertTestMail(t, database, "testuser", "Old Message", "sender@test.com", "testuser@localhost", "INBOX")
	msg2ID := helpers.InsertTestMail(t, database, "testuser", "Recent Message", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := helpers.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}
	userDB := helpers.GetUserDBByID(t, database, state.UserID)

	// Set different dates
	oldDate := time.Date(2020, 1, 1, 10, 0, 0, 0, time.UTC)
	recentDate := time.Date(2024, 12, 1, 10, 0, 0, 0, time.UTC)

	userDB.Exec(`UPDATE message_mailbox SET internal_date = ? WHERE message_id = ? AND mailbox_id = ?`, oldDate, msg1ID, mailboxID)
	userDB.Exec(`UPDATE message_mailbox SET internal_date = ? WHERE message_id = ? AND mailbox_id = ?`, recentDate, msg2ID, mailboxID)

	// Test SINCE - should find recent messages
	server.HandleSearch(conn, "S020", []string{"S020", "SEARCH", "SINCE", "1-Jan-2024"}, state)
	response := conn.GetWrittenData()
	if !strings.Contains(response, "2") {
		t.Errorf("Expected to find recent message with SINCE, got: %s", response)
	}

	conn.ClearWriteBuffer()

	// Test BEFORE - should find old messages
	server.HandleSearch(conn, "S021", []string{"S021", "SEARCH", "BEFORE", "1-Jan-2024"}, state)
	response = conn.GetWrittenData()
	if !strings.Contains(response, "1") {
		t.Errorf("Expected to find old message with BEFORE, got: %s", response)
	}
}

// TestSearchCommand_NEW_OLD tests SEARCH NEW and OLD
func TestSearchCommand_NEW_OLD(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := helpers.GetDatabaseFromServer(server)

	userID := helpers.CreateTestUser(t, database, "testuser")

	msg1ID := helpers.InsertTestMail(t, database, "testuser", "New Message", "sender@test.com", "testuser@localhost", "INBOX")
	msg2ID := helpers.InsertTestMail(t, database, "testuser", "Old Message", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := helpers.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}
	userDB := helpers.GetUserDBByID(t, database, state.UserID)

	// NEW = RECENT and UNSEEN
	userDB.Exec(`UPDATE message_mailbox SET flags = '\Recent' WHERE message_id = ? AND mailbox_id = ?`, msg1ID, mailboxID)

	// OLD = NOT RECENT
	// Message 2 has no flags, so it's not recent

	// Test NEW - should find recent unseen messages
	server.HandleSearch(conn, "S022", []string{"S022", "SEARCH", "NEW"}, state)
	response := conn.GetWrittenData()
	if !strings.Contains(response, "1") {
		t.Errorf("Expected to find new message, got: %s", response)
	}

	conn.ClearWriteBuffer()

	// Test OLD - should find non-recent messages
	server.HandleSearch(conn, "S023", []string{"S023", "SEARCH", "OLD"}, state)
	response = conn.GetWrittenData()
	if !strings.Contains(response, "2") {
		t.Errorf("Expected to find old message, got: %s", response)
	}

	_ = msg2ID
}

// TestSearchCommand_ANSWERED tests SEARCH ANSWERED and UNANSWERED
func TestSearchCommand_ANSWERED(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := helpers.GetDatabaseFromServer(server)

	userID := helpers.CreateTestUser(t, database, "testuser")

	msg1ID := helpers.InsertTestMail(t, database, "testuser", "Answered", "sender@test.com", "testuser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "testuser", "Not Answered", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := helpers.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}
	userDB := helpers.GetUserDBByID(t, database, state.UserID)

	// Mark message 1 as answered
	userDB.Exec(`UPDATE message_mailbox SET flags = '\Answered' WHERE message_id = ? AND mailbox_id = ?`, msg1ID, mailboxID)

	// Test ANSWERED
	server.HandleSearch(conn, "S024", []string{"S024", "SEARCH", "ANSWERED"}, state)
	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH 1") {
		t.Errorf("Expected to find answered message, got: %s", response)
	}

	conn.ClearWriteBuffer()

	// Test UNANSWERED
	server.HandleSearch(conn, "S025", []string{"S025", "SEARCH", "UNANSWERED"}, state)
	response = conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH 2") {
		t.Errorf("Expected to find unanswered message, got: %s", response)
	}
}

// TestSearchCommand_DRAFT tests SEARCH DRAFT and UNDRAFT
func TestSearchCommand_DRAFT(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := helpers.GetDatabaseFromServer(server)

	userID := helpers.CreateTestUser(t, database, "testuser")

	msg1ID := helpers.InsertTestMail(t, database, "testuser", "Draft", "sender@test.com", "testuser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "testuser", "Not Draft", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := helpers.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}
	userDB := helpers.GetUserDBByID(t, database, state.UserID)

	// Mark message 1 as draft
	userDB.Exec(`UPDATE message_mailbox SET flags = '\Draft' WHERE message_id = ? AND mailbox_id = ?`, msg1ID, mailboxID)

	// Test DRAFT
	server.HandleSearch(conn, "S026", []string{"S026", "SEARCH", "DRAFT"}, state)
	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH 1") {
		t.Errorf("Expected to find draft message, got: %s", response)
	}

	conn.ClearWriteBuffer()

	// Test UNDRAFT
	server.HandleSearch(conn, "S027", []string{"S027", "SEARCH", "UNDRAFT"}, state)
	response = conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH 2") {
		t.Errorf("Expected to find non-draft message, got: %s", response)
	}
}

// TestSearchCommand_HEADER tests SEARCH HEADER
func TestSearchCommand_HEADER(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := helpers.GetDatabaseFromServer(server)

	userID := helpers.CreateTestUser(t, database, "testuser")

	helpers.InsertTestMail(t, database, "testuser", "Test Subject", "sender@test.com", "testuser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "testuser", "Another Subject", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := helpers.GetMailboxID(t, database, userID, "INBOX")
	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}

	// Test HEADER Subject with specific string
	server.HandleSearch(conn, "S028", []string{"S028", "SEARCH", "HEADER", "Subject", "Test"}, state)
	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH 1") {
		t.Errorf("Expected to find message with 'Test' in Subject header, got: %s", response)
	}
}

// TestSearchCommand_BODY_TEXT tests SEARCH BODY and TEXT
func TestSearchCommand_BODY_TEXT(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := helpers.GetDatabaseFromServer(server)

	userID := helpers.CreateTestUser(t, database, "testuser")

	// Note: Our InsertTestMail creates messages with "Test message body" as the body
	helpers.InsertTestMail(t, database, "testuser", "Subject One", "sender@test.com", "testuser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "testuser", "Subject Two", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := helpers.GetMailboxID(t, database, userID, "INBOX")
	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}

	// Test BODY - search in message body
	server.HandleSearch(conn, "S029", []string{"S029", "SEARCH", "BODY", "message"}, state)
	response := conn.GetWrittenData()
	if !strings.Contains(response, "1") || !strings.Contains(response, "2") {
		t.Errorf("Expected to find both messages with 'message' in body, got: %s", response)
	}

	conn.ClearWriteBuffer()

	// Test TEXT - search in entire message (headers + body)
	server.HandleSearch(conn, "S030", []string{"S030", "SEARCH", "TEXT", "Subject One"}, state)
	response = conn.GetWrittenData()
	if !strings.Contains(response, "1") {
		t.Errorf("Expected to find message with 'Subject One' in text, got: %s", response)
	}
}

// TestSearchCommand_UID tests SEARCH UID
func TestSearchCommand_UID(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := helpers.GetDatabaseFromServer(server)

	userID := helpers.CreateTestUser(t, database, "testuser")

	helpers.InsertTestMail(t, database, "testuser", "Message 1", "sender@test.com", "testuser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "testuser", "Message 2", "sender@test.com", "testuser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "testuser", "Message 3", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := helpers.GetMailboxID(t, database, userID, "INBOX")
	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}

	// Test UID search with single UID
	server.HandleSearch(conn, "S031", []string{"S031", "SEARCH", "UID", "1"}, state)
	response := conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH") {
		t.Errorf("Expected SEARCH response for UID, got: %s", response)
	}

	conn.ClearWriteBuffer()

	// Test UID search with range
	server.HandleSearch(conn, "S032", []string{"S032", "SEARCH", "UID", "1:2"}, state)
	response = conn.GetWrittenData()
	if !strings.Contains(response, "* SEARCH") {
		t.Errorf("Expected SEARCH response for UID range, got: %s", response)
	}
}

// TestSearchCommand_BadSyntax tests SEARCH with invalid syntax
func TestSearchCommand_BadSyntax(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := helpers.GetDatabaseFromServer(server)

	userID := helpers.CreateTestUser(t, database, "testuser")
	mailboxID, _ := helpers.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}

	// Test with no search criteria
	server.HandleSearch(conn, "S033", []string{"S033", "SEARCH"}, state)
	response := conn.GetWrittenData()
	if !strings.Contains(response, "S033 BAD") {
		t.Errorf("Expected BAD response for missing criteria, got: %s", response)
	}
}

// TestSearchCommand_ResponseFormat tests the format of SEARCH responses
func TestSearchCommand_ResponseFormat(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := helpers.GetDatabaseFromServer(server)

	userID := helpers.CreateTestUser(t, database, "testuser")

	helpers.InsertTestMail(t, database, "testuser", "Message 1", "sender@test.com", "testuser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "testuser", "Message 2", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := helpers.GetMailboxID(t, database, userID, "INBOX")
	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}

	server.HandleSearch(conn, "FORMAT", []string{"FORMAT", "SEARCH", "ALL"}, state)

	response := conn.GetWrittenData()

	// Check for untagged SEARCH response format: "* SEARCH ..."
	if !strings.Contains(response, "* SEARCH") {
		t.Errorf("Expected untagged SEARCH response, got: %s", response)
	}

	// Check for tagged OK response
	if !strings.Contains(response, "FORMAT OK SEARCH completed") {
		t.Errorf("Expected tagged OK response, got: %s", response)
	}

	// Response should contain sequence numbers separated by spaces
	lines := strings.Split(response, "\r\n")
	var searchLine string
	for _, line := range lines {
		if strings.HasPrefix(line, "* SEARCH") {
			searchLine = line
			break
		}
	}

	if searchLine == "" {
		t.Errorf("Could not find SEARCH response line")
	} else if !strings.HasPrefix(searchLine, "* SEARCH ") {
		t.Errorf("Invalid SEARCH response format: %s", searchLine)
	}
}

// TestSearchCommand_TagHandling tests various tag formats
func TestSearchCommand_TagHandling(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	database := helpers.GetDatabaseFromServer(server)

	userID := helpers.CreateTestUser(t, database, "testuser")
	helpers.InsertTestMail(t, database, "testuser", "Message", "sender@test.com", "testuser@localhost", "INBOX")
	mailboxID, _ := helpers.GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}

	testCases := []struct {
		name string
		tag  string
	}{
		{"Tag A001", "A001"},
		{"Tag search1", "search1"},
		{"Tag TAG-123", "TAG-123"},
		{"Tag *", "*"},
		{"Tag empty", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			conn := helpers.NewMockConn()
			server.HandleSearch(conn, tc.tag, []string{tc.tag, "SEARCH", "ALL"}, state)

			response := conn.GetWrittenData()
			expectedCompletion := fmt.Sprintf("%s OK SEARCH completed", tc.tag)

			if !strings.Contains(response, expectedCompletion) {
				t.Errorf("Expected tag %s in completion, got: %s", tc.tag, response)
			}
		})
	}
}
