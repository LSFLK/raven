//go:build test
// +build test

package server

import (
	"strings"
	"testing"

	"raven/internal/models"
	
)

// TestStoreCommand_Unauthenticated tests STORE without authentication
func TestStoreCommand_Unauthenticated(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	state := &models.ClientState{
		Authenticated: false,
	}

	srv.HandleStore(conn, "S001", []string{"S001", "STORE", "1", "FLAGS", "(\\Seen)"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "S001 NO Please authenticate first") {
		t.Errorf("Expected authentication required error, got: %s", response)
	}
}

// TestStoreCommand_NoMailboxSelected tests STORE without mailbox selection
func TestStoreCommand_NoMailboxSelected(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	database := GetDatabaseFromServer(server)

	userID := CreateTestUser(t, database, "testuser")
	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: 0, // No mailbox selected
	}

	srv.HandleStore(conn, "S002", []string{"S002", "STORE", "1", "FLAGS", "(\\Seen)"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "S002 NO No mailbox selected") {
		t.Errorf("Expected no mailbox selected error, got: %s", response)
	}
}

// TestStoreCommand_FLAGS_Replace tests STORE FLAGS (replace all flags)
func TestStoreCommand_FLAGS_Replace(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	database := GetDatabaseFromServer(server)

	userID := CreateTestUser(t, database, "testuser")
	msgID := InsertTestMail(t, database, "testuser", "Test", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}
	userDB := GetUserDBByID(t, database, state.UserID)

	// Set initial flags
	userDB.Exec(`UPDATE message_mailbox SET flags = '\Seen \Answered' WHERE message_id = ? AND mailbox_id = ?`, msgID, mailboxID)

	// Replace flags with \Deleted
	srv.HandleStore(conn, "S003", []string{"S003", "STORE", "1", "FLAGS", "(\\Deleted)"}, state)

	response := conn.GetWrittenData()
	// Should return untagged FETCH with new flags
	if !strings.Contains(response, "* 1 FETCH") {
		t.Errorf("Expected untagged FETCH response, got: %s", response)
	}
	if !strings.Contains(response, "\\Deleted") {
		t.Errorf("Expected \\Deleted flag, got: %s", response)
	}
	if strings.Contains(response, "\\Seen") || strings.Contains(response, "\\Answered") {
		t.Errorf("Old flags should be replaced, got: %s", response)
	}
	if !strings.Contains(response, "S003 OK STORE completed") {
		t.Errorf("Expected OK completion, got: %s", response)
	}

	// Verify in database
	var flags string
	userDB.QueryRow(`SELECT flags FROM message_mailbox WHERE message_id = ? AND mailbox_id = ?`, msgID, mailboxID).Scan(&flags)
	if !strings.Contains(flags, "\\Deleted") {
		t.Errorf("Expected \\Deleted in database, got: %s", flags)
	}
}

// TestStoreCommand_AddFlags tests STORE +FLAGS (add flags)
func TestStoreCommand_AddFlags(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	database := GetDatabaseFromServer(server)

	userID := CreateTestUser(t, database, "testuser")
	msgID := InsertTestMail(t, database, "testuser", "Test", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}
	userDB := GetUserDBByID(t, database, state.UserID)

	// Set initial flag
	userDB.Exec(`UPDATE message_mailbox SET flags = '\Seen' WHERE message_id = ? AND mailbox_id = ?`, msgID, mailboxID)

	// Add \Deleted flag
	srv.HandleStore(conn, "S004", []string{"S004", "STORE", "1", "+FLAGS", "(\\Deleted)"}, state)

	response := conn.GetWrittenData()
	// Should have both flags
	if !strings.Contains(response, "\\Seen") {
		t.Errorf("Expected \\Seen flag to remain, got: %s", response)
	}
	if !strings.Contains(response, "\\Deleted") {
		t.Errorf("Expected \\Deleted flag to be added, got: %s", response)
	}
}

// TestStoreCommand_RemoveFlags tests STORE -FLAGS (remove flags)
func TestStoreCommand_RemoveFlags(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	database := GetDatabaseFromServer(server)

	userID := CreateTestUser(t, database, "testuser")
	msgID := InsertTestMail(t, database, "testuser", "Test", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}
	userDB := GetUserDBByID(t, database, state.UserID)

	// Set initial flags
	userDB.Exec(`UPDATE message_mailbox SET flags = '\Seen \Deleted \Flagged' WHERE message_id = ? AND mailbox_id = ?`, msgID, mailboxID)

	// Remove \Deleted flag
	srv.HandleStore(conn, "S005", []string{"S005", "STORE", "1", "-FLAGS", "(\\Deleted)"}, state)

	response := conn.GetWrittenData()
	// Should have remaining flags but not \Deleted
	if !strings.Contains(response, "\\Seen") || !strings.Contains(response, "\\Flagged") {
		t.Errorf("Expected \\Seen and \\Flagged to remain, got: %s", response)
	}
	if strings.Contains(response, "\\Deleted") {
		t.Errorf("Expected \\Deleted to be removed, got: %s", response)
	}
}

// TestStoreCommand_FLAGS_SILENT tests STORE FLAGS.SILENT
func TestStoreCommand_FLAGS_SILENT(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	database := GetDatabaseFromServer(server)

	userID := CreateTestUser(t, database, "testuser")
	msgID := InsertTestMail(t, database, "testuser", "Test", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}
	userDB := GetUserDBByID(t, database, state.UserID)

	// Store with .SILENT - should NOT return untagged FETCH
	srv.HandleStore(conn, "S006", []string{"S006", "STORE", "1", "FLAGS.SILENT", "(\\Seen)"}, state)

	response := conn.GetWrittenData()
	// Should NOT contain untagged FETCH
	if strings.Contains(response, "* 1 FETCH") {
		t.Errorf("Expected no untagged FETCH with .SILENT, got: %s", response)
	}
	// Should still have OK completion
	if !strings.Contains(response, "S006 OK STORE completed") {
		t.Errorf("Expected OK completion, got: %s", response)
	}

	// Verify flags were still updated in database
	var flags string
	userDB.QueryRow(`SELECT flags FROM message_mailbox WHERE message_id = ? AND mailbox_id = ?`, msgID, mailboxID).Scan(&flags)
	if !strings.Contains(flags, "\\Seen") {
		t.Errorf("Expected \\Seen in database even with .SILENT, got: %s", flags)
	}
}

// TestStoreCommand_AddFLAGS_SILENT tests STORE +FLAGS.SILENT
func TestStoreCommand_AddFLAGS_SILENT(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	database := GetDatabaseFromServer(server)

	userID := CreateTestUser(t, database, "testuser")
	InsertTestMail(t, database, "testuser", "Test", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}

	srv.HandleStore(conn, "S007", []string{"S007", "STORE", "1", "+FLAGS.SILENT", "(\\Deleted)"}, state)

	response := conn.GetWrittenData()
	if strings.Contains(response, "* 1 FETCH") {
		t.Errorf("Expected no untagged FETCH with .SILENT, got: %s", response)
	}
	if !strings.Contains(response, "S007 OK STORE completed") {
		t.Errorf("Expected OK completion, got: %s", response)
	}
}

// TestStoreCommand_RemoveFLAGS_SILENT tests STORE -FLAGS.SILENT
func TestStoreCommand_RemoveFLAGS_SILENT(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	database := GetDatabaseFromServer(server)

	userID := CreateTestUser(t, database, "testuser")
	msgID := InsertTestMail(t, database, "testuser", "Test", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}
	userDB := GetUserDBByID(t, database, state.UserID)

	userDB.Exec(`UPDATE message_mailbox SET flags = '\Seen \Deleted' WHERE message_id = ? AND mailbox_id = ?`, msgID, mailboxID)

	srv.HandleStore(conn, "S008", []string{"S008", "STORE", "1", "-FLAGS.SILENT", "(\\Deleted)"}, state)

	response := conn.GetWrittenData()
	if strings.Contains(response, "* 1 FETCH") {
		t.Errorf("Expected no untagged FETCH with .SILENT, got: %s", response)
	}
}

// TestStoreCommand_SequenceRange tests STORE with sequence range
func TestStoreCommand_SequenceRange(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	database := GetDatabaseFromServer(server)

	userID := CreateTestUser(t, database, "testuser")
	InsertTestMail(t, database, "testuser", "Msg 1", "sender@test.com", "testuser@localhost", "INBOX")
	InsertTestMail(t, database, "testuser", "Msg 2", "sender@test.com", "testuser@localhost", "INBOX")
	InsertTestMail(t, database, "testuser", "Msg 3", "sender@test.com", "testuser@localhost", "INBOX")
	InsertTestMail(t, database, "testuser", "Msg 4", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}

	// RFC 3501 Example: STORE 2:4 +FLAGS (\Deleted)
	srv.HandleStore(conn, "A003", []string{"A003", "STORE", "2:4", "+FLAGS", "(\\Deleted)"}, state)

	response := conn.GetWrittenData()
	// Should return FETCH for messages 2, 3, and 4
	if !strings.Contains(response, "* 2 FETCH") {
		t.Errorf("Expected FETCH response for message 2, got: %s", response)
	}
	if !strings.Contains(response, "* 3 FETCH") {
		t.Errorf("Expected FETCH response for message 3, got: %s", response)
	}
	if !strings.Contains(response, "* 4 FETCH") {
		t.Errorf("Expected FETCH response for message 4, got: %s", response)
	}
	// Should NOT return FETCH for message 1
	if strings.Contains(response, "* 1 FETCH") {
		t.Errorf("Should not have FETCH for message 1, got: %s", response)
	}
	if !strings.Contains(response, "A003 OK STORE completed") {
		t.Errorf("Expected OK completion, got: %s", response)
	}
}

// TestStoreCommand_RFC3501Example tests the exact example from RFC 3501
func TestStoreCommand_RFC3501Example(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	database := GetDatabaseFromServer(server)

	userID := CreateTestUser(t, database, "testuser")
	msg1ID := InsertTestMail(t, database, "testuser", "Msg 1", "sender@test.com", "testuser@localhost", "INBOX")
	msg2ID := InsertTestMail(t, database, "testuser", "Msg 2", "sender@test.com", "testuser@localhost", "INBOX")
	msg3ID := InsertTestMail(t, database, "testuser", "Msg 3", "sender@test.com", "testuser@localhost", "INBOX")
	msg4ID := InsertTestMail(t, database, "testuser", "Msg 4", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}
	userDB := GetUserDBByID(t, database, state.UserID)

	// Set initial flags like in RFC example
	userDB.Exec(`UPDATE message_mailbox SET flags = '\Seen' WHERE message_id = ? AND mailbox_id = ?`, msg2ID, mailboxID)
	userDB.Exec(`UPDATE message_mailbox SET flags = '\Flagged \Seen' WHERE message_id = ? AND mailbox_id = ?`, msg4ID, mailboxID)

	// RFC 3501 Example: C: A003 STORE 2:4 +FLAGS (\Deleted)
	// Expected responses:
	// S: * 2 FETCH (FLAGS (\Deleted \Seen))
	// S: * 3 FETCH (FLAGS (\Deleted))
	// S: * 4 FETCH (FLAGS (\Deleted \Flagged \Seen))
	// S: A003 OK STORE completed
	srv.HandleStore(conn, "A003", []string{"A003", "STORE", "2:4", "+FLAGS", "(\\Deleted)"}, state)

	response := conn.GetWrittenData()

	// All should have \Deleted added
	lines := strings.Split(response, "\r\n")
	foundMsg2, foundMsg3, foundMsg4 := false, false, false

	for _, line := range lines {
		if strings.Contains(line, "* 2 FETCH") {
			foundMsg2 = true
			// Should have both \Deleted and \Seen
			if !strings.Contains(line, "\\Deleted") || !strings.Contains(line, "\\Seen") {
				t.Errorf("Message 2 should have \\Deleted and \\Seen, got: %s", line)
			}
		}
		if strings.Contains(line, "* 3 FETCH") {
			foundMsg3 = true
			// Should have only \Deleted (no prior flags)
			if !strings.Contains(line, "\\Deleted") {
				t.Errorf("Message 3 should have \\Deleted, got: %s", line)
			}
		}
		if strings.Contains(line, "* 4 FETCH") {
			foundMsg4 = true
			// Should have \Deleted, \Flagged, and \Seen
			if !strings.Contains(line, "\\Deleted") || !strings.Contains(line, "\\Flagged") || !strings.Contains(line, "\\Seen") {
				t.Errorf("Message 4 should have \\Deleted, \\Flagged, and \\Seen, got: %s", line)
			}
		}
	}

	if !foundMsg2 || !foundMsg3 || !foundMsg4 {
		t.Errorf("Expected FETCH responses for messages 2, 3, and 4")
	}

	_ = msg1ID
	_ = msg3ID
}

// TestStoreCommand_MultipleFlags tests STORE with multiple flags
func TestStoreCommand_MultipleFlags(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	database := GetDatabaseFromServer(server)

	userID := CreateTestUser(t, database, "testuser")
	InsertTestMail(t, database, "testuser", "Test", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}

	// Add multiple flags at once
	srv.HandleStore(conn, "S009", []string{"S009", "STORE", "1", "+FLAGS", "(\\Seen", "\\Deleted", "\\Flagged)"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "\\Seen") || !strings.Contains(response, "\\Deleted") || !strings.Contains(response, "\\Flagged") {
		t.Errorf("Expected all three flags, got: %s", response)
	}
}

// TestStoreCommand_InvalidDataItem tests STORE with invalid data item
func TestStoreCommand_InvalidDataItem(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	database := GetDatabaseFromServer(server)

	userID := CreateTestUser(t, database, "testuser")
	InsertTestMail(t, database, "testuser", "Test", "sender@test.com", "testuser@localhost", "INBOX")
	mailboxID, _ := GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}

	srv.HandleStore(conn, "S010", []string{"S010", "STORE", "1", "INVALID", "(\\Seen)"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "S010 BAD") {
		t.Errorf("Expected BAD response for invalid data item, got: %s", response)
	}
}

// TestStoreCommand_BadSyntax tests STORE with missing arguments
func TestStoreCommand_BadSyntax(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	database := GetDatabaseFromServer(server)

	userID := CreateTestUser(t, database, "testuser")
	mailboxID, _ := GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}

	// Missing flags argument
	srv.HandleStore(conn, "S011", []string{"S011", "STORE", "1", "FLAGS"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "S011 BAD") {
		t.Errorf("Expected BAD response for missing arguments, got: %s", response)
	}
}

// TestStoreCommand_NoRecentFlag tests that \Recent flag cannot be set by client
func TestStoreCommand_NoRecentFlag(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	database := GetDatabaseFromServer(server)

	userID := CreateTestUser(t, database, "testuser")
	msgID := InsertTestMail(t, database, "testuser", "Test", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}
	userDB := GetUserDBByID(t, database, state.UserID)

	// Try to set \Recent (should be ignored)
	srv.HandleStore(conn, "S012", []string{"S012", "STORE", "1", "FLAGS", "(\\Recent", "\\Seen)"}, state)

	response := conn.GetWrittenData()
	// \Recent should NOT appear in flags (server-managed only)
	// But \Seen should be there
	if !strings.Contains(response, "\\Seen") {
		t.Errorf("Expected \\Seen flag, got: %s", response)
	}

	// Verify in database - \Recent should not be there
	var flags string
	userDB.QueryRow(`SELECT flags FROM message_mailbox WHERE message_id = ? AND mailbox_id = ?`, msgID, mailboxID).Scan(&flags)
	if strings.Contains(flags, "\\Recent") {
		t.Errorf("\\Recent flag should not be settable by client, got: %s", flags)
	}
}

// TestStoreCommand_EmptyFlags tests STORE FLAGS with empty flag list
func TestStoreCommand_EmptyFlags(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	database := GetDatabaseFromServer(server)

	userID := CreateTestUser(t, database, "testuser")
	msgID := InsertTestMail(t, database, "testuser", "Test", "sender@test.com", "testuser@localhost", "INBOX")

	mailboxID, _ := GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}
	userDB := GetUserDBByID(t, database, state.UserID)

	// Set some initial flags
	userDB.Exec(`UPDATE message_mailbox SET flags = '\Seen \Flagged' WHERE message_id = ? AND mailbox_id = ?`, msgID, mailboxID)

	// Clear all flags
	srv.HandleStore(conn, "S013", []string{"S013", "STORE", "1", "FLAGS", "()"}, state)

	response := conn.GetWrittenData()
	// Should have empty flags
	if !strings.Contains(response, "FLAGS ()") {
		t.Errorf("Expected empty flags, got: %s", response)
	}

	// Verify in database
	var flags string
	userDB.QueryRow(`SELECT flags FROM message_mailbox WHERE message_id = ? AND mailbox_id = ?`, msgID, mailboxID).Scan(&flags)
	if flags != "" {
		t.Errorf("Expected empty flags in database, got: %s", flags)
	}
}

// TestStoreCommand_TagHandling tests various tag formats
func TestStoreCommand_TagHandling(t *testing.T) {
	srv := SetupTestServerSimple(t)
	database := GetDatabaseFromServer(server)

	userID := CreateTestUser(t, database, "testuser")
	InsertTestMail(t, database, "testuser", "Test", "sender@test.com", "testuser@localhost", "INBOX")
	mailboxID, _ := GetMailboxID(t, database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: mailboxID,
	}

	testCases := []string{"A001", "store1", "TAG-123"}

	for _, tag := range testCases {
		t.Run("Tag "+tag, func(t *testing.T) {
			conn := NewMockConn()
			srv.HandleStore(conn, tag, []string{tag, "STORE", "1", "FLAGS", "(\\Seen)"}, state)

			response := conn.GetWrittenData()
			expectedCompletion := tag + " OK STORE completed"

			if !strings.Contains(response, expectedCompletion) {
				t.Errorf("Expected tag %s in completion, got: %s", tag, response)
			}
		})
	}
}
