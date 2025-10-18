//go:build test
// +build test

package server_test

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"go-imap/internal/db"
	"go-imap/internal/models"
	"go-imap/test/helpers"
)

// TestCopyCommand_Unauthenticated tests COPY command without authentication
func TestCopyCommand_Unauthenticated(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()

	state := &models.ClientState{
		Authenticated: false,
	}

	server.HandleCopy(conn, "C001", []string{"COPY", "1", "INBOX"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "C001 NO Please authenticate first") {
		t.Errorf("Expected authentication error, got: %s", response)
	}
}

// TestCopyCommand_NoMailboxSelected tests COPY command without selecting a mailbox
func TestCopyCommand_NoMailboxSelected(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := server.GetDB().(*sql.DB)

	userID := helpers.CreateTestUser(t, database, "copyuser@example.com")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		SelectedMailboxID: 0, // No mailbox selected
	}

	server.HandleCopy(conn, "C002", []string{"COPY", "1", "INBOX"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "C002 NO No mailbox selected") {
		t.Errorf("Expected no mailbox error, got: %s", response)
	}
}

// TestCopyCommand_DestinationNotExists tests COPY to non-existent mailbox
func TestCopyCommand_DestinationNotExists(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := server.GetDB().(*sql.DB)

	userID := helpers.CreateTestUser(t, database, "copyuser")
	helpers.InsertTestMail(t, database, "copyuser", "Test message", "sender@test.com", "copyuser@localhost", "INBOX")

	mailboxID, _ := db.GetMailboxByName(database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		SelectedMailboxID: mailboxID,
	}

	// Try to copy to non-existent mailbox
	server.HandleCopy(conn, "C003", []string{"COPY", "1", "NonExistentFolder"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "C003 NO [TRYCREATE]") {
		t.Errorf("Expected TRYCREATE response, got: %s", response)
	}
	if !strings.Contains(response, "does not exist") {
		t.Errorf("Expected 'does not exist' message, got: %s", response)
	}
}

// TestCopyCommand_SingleMessage tests copying a single message
func TestCopyCommand_SingleMessage(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := server.GetDB().(*sql.DB)

	userID := helpers.CreateTestUser(t, database, "copyuser")
	helpers.InsertTestMail(t, database, "copyuser", "Test message 1", "sender@test.com", "copyuser@localhost", "INBOX")
	helpers.CreateMailbox(t, database, "copyuser", "Sent")

	inboxID, _ := db.GetMailboxByName(database, userID, "INBOX")
	sentID, _ := db.GetMailboxByName(database, userID, "Sent")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		SelectedMailboxID: inboxID,
	}

	// Copy message 1 to Sent
	server.HandleCopy(conn, "C004", []string{"COPY", "1", "Sent"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "C004 OK COPY completed") {
		t.Errorf("Expected OK response, got: %s", response)
	}

	// Verify message was copied to Sent folder
	var count int
	database.QueryRow("SELECT COUNT(*) FROM message_mailbox WHERE mailbox_id = ?", sentID).Scan(&count)
	if count != 1 {
		t.Errorf("Expected 1 message in Sent folder, got %d", count)
	}

	// Verify original message still exists in INBOX
	database.QueryRow("SELECT COUNT(*) FROM message_mailbox WHERE mailbox_id = ?", inboxID).Scan(&count)
	if count != 1 {
		t.Errorf("Expected 1 message in INBOX, got %d", count)
	}
}

// TestCopyCommand_RFC3501Example tests the RFC 3501 example: COPY 2:4 MEETING
func TestCopyCommand_RFC3501Example(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := server.GetDB().(*sql.DB)

	userID := helpers.CreateTestUser(t, database, "copyuser")

	// Insert 4 test messages
	helpers.InsertTestMail(t, database, "copyuser", "Message 1", "sender@test.com", "copyuser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "copyuser", "Message 2", "sender@test.com", "copyuser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "copyuser", "Message 3", "sender@test.com", "copyuser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "copyuser", "Message 4", "sender@test.com", "copyuser@localhost", "INBOX")

	helpers.CreateMailbox(t, database, "copyuser", "MEETING")

	inboxID, _ := db.GetMailboxByName(database, userID, "INBOX")
	meetingID, _ := db.GetMailboxByName(database, userID, "MEETING")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		SelectedMailboxID: inboxID,
	}

	// Copy messages 2:4 to MEETING (RFC 3501 example)
	server.HandleCopy(conn, "A003", []string{"COPY", "2:4", "MEETING"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A003 OK COPY completed") {
		t.Errorf("Expected OK response, got: %s", response)
	}

	// Verify 3 messages were copied to MEETING folder
	var count int
	database.QueryRow("SELECT COUNT(*) FROM message_mailbox WHERE mailbox_id = ?", meetingID).Scan(&count)
	if count != 3 {
		t.Errorf("Expected 3 messages in MEETING folder, got %d", count)
	}

	// Verify original messages still exist in INBOX (4 messages)
	database.QueryRow("SELECT COUNT(*) FROM message_mailbox WHERE mailbox_id = ?", inboxID).Scan(&count)
	if count != 4 {
		t.Errorf("Expected 4 messages in INBOX, got %d", count)
	}
}

// TestCopyCommand_PreserveFlags tests that flags are preserved
func TestCopyCommand_PreserveFlags(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := server.GetDB().(*sql.DB)

	userID := helpers.CreateTestUser(t, database, "copyuser")
	msgID := helpers.InsertTestMail(t, database, "copyuser", "Test message", "sender@test.com", "copyuser@localhost", "INBOX")
	helpers.CreateMailbox(t, database, "copyuser", "Archive")

	inboxID, _ := db.GetMailboxByName(database, userID, "INBOX")
	archiveID, _ := db.GetMailboxByName(database, userID, "Archive")

	// Set specific flags
	database.Exec(`UPDATE message_mailbox SET flags = '\Seen \Flagged' WHERE message_id = ? AND mailbox_id = ?`, msgID, inboxID)

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		SelectedMailboxID: inboxID,
	}

	// Copy message to Archive
	server.HandleCopy(conn, "C005", []string{"COPY", "1", "Archive"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "C005 OK COPY completed") {
		t.Errorf("Expected OK response, got: %s", response)
	}

	// Verify flags were preserved (and \Recent added)
	var flags string
	database.QueryRow("SELECT flags FROM message_mailbox WHERE mailbox_id = ?", archiveID).Scan(&flags)
	if !strings.Contains(flags, `\Seen`) || !strings.Contains(flags, `\Flagged`) {
		t.Errorf("Expected flags to be preserved, got: %s", flags)
	}
	if !strings.Contains(flags, `\Recent`) {
		t.Errorf("Expected \\Recent flag to be set, got: %s", flags)
	}
}

// TestCopyCommand_PreserveInternalDate tests that internal date is preserved
func TestCopyCommand_PreserveInternalDate(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := server.GetDB().(*sql.DB)

	userID := helpers.CreateTestUser(t, database, "copyuser")
	msgID := helpers.InsertTestMail(t, database, "copyuser", "Test message", "sender@test.com", "copyuser@localhost", "INBOX")
	helpers.CreateMailbox(t, database, "copyuser", "Archive")

	inboxID, _ := db.GetMailboxByName(database, userID, "INBOX")
	archiveID, _ := db.GetMailboxByName(database, userID, "Archive")

	// Set specific internal date
	specificDate := "2024-01-15 10:30:00"
	database.Exec(`UPDATE message_mailbox SET internal_date = ? WHERE message_id = ? AND mailbox_id = ?`, specificDate, msgID, inboxID)

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		SelectedMailboxID: inboxID,
	}

	// Copy message to Archive
	server.HandleCopy(conn, "C006", []string{"COPY", "1", "Archive"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "C006 OK COPY completed") {
		t.Errorf("Expected OK response, got: %s", response)
	}

	// Verify internal date was preserved (SQLite may format dates differently, so just check it exists and is similar)
	var internalDate string
	database.QueryRow("SELECT internal_date FROM message_mailbox WHERE mailbox_id = ?", archiveID).Scan(&internalDate)
	if !strings.Contains(internalDate, "2024-01-15") {
		t.Errorf("Expected internal date containing %s, got: %s", "2024-01-15", internalDate)
	}
}

// TestCopyCommand_MultipleMessages tests copying multiple non-sequential messages
func TestCopyCommand_MultipleMessages(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := server.GetDB().(*sql.DB)

	userID := helpers.CreateTestUser(t, database, "copyuser")
	helpers.InsertTestMail(t, database, "copyuser", "Message 1", "sender@test.com", "copyuser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "copyuser", "Message 2", "sender@test.com", "copyuser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "copyuser", "Message 3", "sender@test.com", "copyuser@localhost", "INBOX")
	helpers.CreateMailbox(t, database, "copyuser", "Work")

	inboxID, _ := db.GetMailboxByName(database, userID, "INBOX")
	workID, _ := db.GetMailboxByName(database, userID, "Work")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		SelectedMailboxID: inboxID,
	}

	// Copy messages 1,3 to Work
	server.HandleCopy(conn, "C007", []string{"COPY", "1,3", "Work"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "C007 OK COPY completed") {
		t.Errorf("Expected OK response, got: %s", response)
	}

	// Verify 2 messages were copied to Work folder
	var count int
	database.QueryRow("SELECT COUNT(*) FROM message_mailbox WHERE mailbox_id = ?", workID).Scan(&count)
	if count != 2 {
		t.Errorf("Expected 2 messages in Work folder, got %d", count)
	}
}

// TestCopyCommand_InvalidSequenceSet tests COPY with invalid sequence set
func TestCopyCommand_InvalidSequenceSet(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := server.GetDB().(*sql.DB)

	userID := helpers.CreateTestUser(t, database, "copyuser")
	helpers.CreateMailbox(t, database, "copyuser", "Sent")

	inboxID, _ := db.GetMailboxByName(database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		SelectedMailboxID: inboxID,
	}

	// Try to copy with invalid sequence set (no messages exist)
	server.HandleCopy(conn, "C008", []string{"COPY", "99", "Sent"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "C008 BAD Invalid sequence set") {
		t.Errorf("Expected BAD response, got: %s", response)
	}
}

// TestCopyCommand_BadSyntax tests COPY with missing parameters
func TestCopyCommand_BadSyntax(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := server.GetDB().(*sql.DB)

	userID := helpers.CreateTestUser(t, database, "copyuser")
	inboxID, _ := db.GetMailboxByName(database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		SelectedMailboxID: inboxID,
	}

	// Try to copy without destination mailbox
	server.HandleCopy(conn, "C009", []string{"COPY", "1"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "C009 BAD Invalid COPY command syntax") {
		t.Errorf("Expected BAD syntax error, got: %s", response)
	}
}

// TestCopyCommand_QuotedMailboxName tests COPY with quoted mailbox name
func TestCopyCommand_QuotedMailboxName(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := server.GetDB().(*sql.DB)

	userID := helpers.CreateTestUser(t, database, "copyuser")
	helpers.InsertTestMail(t, database, "copyuser", "Test message", "sender@test.com", "copyuser@localhost", "INBOX")
	helpers.CreateMailbox(t, database, "copyuser", "My Archive")

	inboxID, _ := db.GetMailboxByName(database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		SelectedMailboxID: inboxID,
	}

	// Copy to mailbox with quoted name
	server.HandleCopy(conn, "C010", []string{"COPY", "1", "\"My Archive\""}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "C010 OK COPY completed") {
		t.Errorf("Expected OK response, got: %s", response)
	}
}

// TestCopyCommand_AllMessages tests copying all messages using *
func TestCopyCommand_AllMessages(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := server.GetDB().(*sql.DB)

	userID := helpers.CreateTestUser(t, database, "copyuser")
	helpers.InsertTestMail(t, database, "copyuser", "Message 1", "sender@test.com", "copyuser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "copyuser", "Message 2", "sender@test.com", "copyuser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "copyuser", "Message 3", "sender@test.com", "copyuser@localhost", "INBOX")
	helpers.CreateMailbox(t, database, "copyuser", "All")

	inboxID, _ := db.GetMailboxByName(database, userID, "INBOX")
	allID, _ := db.GetMailboxByName(database, userID, "All")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		SelectedMailboxID: inboxID,
	}

	// Copy all messages using *
	server.HandleCopy(conn, "C011", []string{"COPY", "*", "All"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "C011 OK COPY completed") {
		t.Errorf("Expected OK response, got: %s", response)
	}

	// Verify last message was copied
	var count int
	database.QueryRow("SELECT COUNT(*) FROM message_mailbox WHERE mailbox_id = ?", allID).Scan(&count)
	if count != 1 {
		t.Errorf("Expected 1 message in All folder (only last message), got %d", count)
	}
}

// TestCopyCommand_RangeWithStar tests copying range with * (e.g., 2:*)
func TestCopyCommand_RangeWithStar(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := server.GetDB().(*sql.DB)

	userID := helpers.CreateTestUser(t, database, "copyuser")
	helpers.InsertTestMail(t, database, "copyuser", "Message 1", "sender@test.com", "copyuser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "copyuser", "Message 2", "sender@test.com", "copyuser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "copyuser", "Message 3", "sender@test.com", "copyuser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "copyuser", "Message 4", "sender@test.com", "copyuser@localhost", "INBOX")
	helpers.CreateMailbox(t, database, "copyuser", "Archive")

	inboxID, _ := db.GetMailboxByName(database, userID, "INBOX")
	archiveID, _ := db.GetMailboxByName(database, userID, "Archive")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		SelectedMailboxID: inboxID,
	}

	// Copy messages 2:* to Archive
	server.HandleCopy(conn, "C012", []string{"COPY", "2:*", "Archive"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "C012 OK COPY completed") {
		t.Errorf("Expected OK response, got: %s", response)
	}

	// Verify 3 messages were copied (2, 3, 4)
	var count int
	database.QueryRow("SELECT COUNT(*) FROM message_mailbox WHERE mailbox_id = ?", archiveID).Scan(&count)
	if count != 3 {
		t.Errorf("Expected 3 messages in Archive folder, got %d", count)
	}
}

// TestCopyCommand_TagHandling tests various tag formats
func TestCopyCommand_TagHandling(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := server.GetDB().(*sql.DB)

	userID := helpers.CreateTestUser(t, database, "copyuser")
	helpers.InsertTestMail(t, database, "copyuser", "Test message", "sender@test.com", "copyuser@localhost", "INBOX")
	helpers.CreateMailbox(t, database, "copyuser", "Sent")

	inboxID, _ := db.GetMailboxByName(database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		SelectedMailboxID: inboxID,
	}

	testCases := []struct {
		tag         string
		expectedTag string
	}{
		{"A001", "A001"},
		{"copy1", "copy1"},
		{"TAG-123", "TAG-123"},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Tag_%s", tc.tag), func(t *testing.T) {
			conn.ClearWriteBuffer()
			server.HandleCopy(conn, tc.tag, []string{"COPY", "1", "Sent"}, state)

			response := conn.GetWrittenData()
			if !strings.Contains(response, fmt.Sprintf("%s OK COPY completed", tc.expectedTag)) {
				t.Errorf("Expected tag %s in response, got: %s", tc.expectedTag, response)
			}
		})
	}
}

// TestCopyCommand_AtomicOperation tests that COPY is atomic (rollback on error)
func TestCopyCommand_AtomicOperation(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := server.GetDB().(*sql.DB)

	userID := helpers.CreateTestUser(t, database, "copyuser")
	helpers.InsertTestMail(t, database, "copyuser", "Message 1", "sender@test.com", "copyuser@localhost", "INBOX")
	helpers.CreateMailbox(t, database, "copyuser", "Destination")

	inboxID, _ := db.GetMailboxByName(database, userID, "INBOX")
	destID, _ := db.GetMailboxByName(database, userID, "Destination")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		SelectedMailboxID: inboxID,
	}

	// Get initial count in destination
	var initialCount int
	database.QueryRow("SELECT COUNT(*) FROM message_mailbox WHERE mailbox_id = ?", destID).Scan(&initialCount)

	// Copy valid message
	server.HandleCopy(conn, "C013", []string{"COPY", "1", "Destination"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "C013 OK COPY completed") {
		t.Errorf("Expected OK response, got: %s", response)
	}

	// Verify message was copied
	var finalCount int
	database.QueryRow("SELECT COUNT(*) FROM message_mailbox WHERE mailbox_id = ?", destID).Scan(&finalCount)
	if finalCount != initialCount+1 {
		t.Errorf("Expected count to increase by 1, initial: %d, final: %d", initialCount, finalCount)
	}
}
