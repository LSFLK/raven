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

// TestUIDCommand_Unauthenticated tests UID command without authentication
func TestUIDCommand_Unauthenticated(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()

	state := &models.ClientState{
		Authenticated: false,
	}

	server.HandleUID(conn, "U001", []string{"UID", "UID", "FETCH", "1", "FLAGS"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "U001 NO Please authenticate first") {
		t.Errorf("Expected authentication error, got: %s", response)
	}
}

// TestUIDCommand_NoMailboxSelected tests UID command without mailbox selection
func TestUIDCommand_NoMailboxSelected(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := server.GetDB().(*sql.DB)

	userID := helpers.CreateTestUser(t, database, "uiduser")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		SelectedMailboxID: 0,
	}

	server.HandleUID(conn, "U002", []string{"UID", "UID", "FETCH", "1", "FLAGS"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "U002 NO No mailbox selected") {
		t.Errorf("Expected no mailbox selected error, got: %s", response)
	}
}

// TestUIDFetch_RFC3501Example tests RFC 3501 example: UID FETCH 4827313:4828442 FLAGS
func TestUIDFetch_RFC3501Example(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := server.GetDB().(*sql.DB)

	userID := helpers.CreateTestUser(t, database, "uiduser")

	// Insert messages with specific UIDs
	msgID1 := helpers.InsertTestMail(t, database, "uiduser", "Message 1", "sender@test.com", "uiduser@localhost", "INBOX")
	msgID2 := helpers.InsertTestMail(t, database, "uiduser", "Message 2", "sender@test.com", "uiduser@localhost", "INBOX")
	msgID3 := helpers.InsertTestMail(t, database, "uiduser", "Message 3", "sender@test.com", "uiduser@localhost", "INBOX")

	inboxID, _ := db.GetMailboxByName(database, userID, "INBOX")

	// Set specific UIDs (simulating real UIDs)
	database.Exec("UPDATE message_mailbox SET uid = 4827313, flags = '\\Seen' WHERE message_id = ?", msgID1)
	database.Exec("UPDATE message_mailbox SET uid = 4827943, flags = '\\Seen' WHERE message_id = ?", msgID2)
	database.Exec("UPDATE message_mailbox SET uid = 4828442, flags = '\\Seen' WHERE message_id = ?", msgID3)

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		SelectedMailboxID: inboxID,
	}

	// UID FETCH 4827313:4828442 FLAGS
	server.HandleUID(conn, "A999", []string{"UID", "UID", "FETCH", "4827313:4828442", "FLAGS"}, state)

	response := conn.GetWrittenData()

	// Should have 3 FETCH responses
	if !strings.Contains(response, "* 1 FETCH") {
		t.Errorf("Expected message 1 FETCH response, got: %s", response)
	}
	if !strings.Contains(response, "UID 4827313") {
		t.Errorf("Expected UID 4827313, got: %s", response)
	}
	if !strings.Contains(response, "* 2 FETCH") {
		t.Errorf("Expected message 2 FETCH response, got: %s", response)
	}
	if !strings.Contains(response, "UID 4827943") {
		t.Errorf("Expected UID 4827943, got: %s", response)
	}
	if !strings.Contains(response, "* 3 FETCH") {
		t.Errorf("Expected message 3 FETCH response, got: %s", response)
	}
	if !strings.Contains(response, "UID 4828442") {
		t.Errorf("Expected UID 4828442, got: %s", response)
	}
	if !strings.Contains(response, "A999 OK UID FETCH completed") {
		t.Errorf("Expected OK response, got: %s", response)
	}
}

// TestUIDFetch_AlwaysIncludesUID tests that UID is always included in response
func TestUIDFetch_AlwaysIncludesUID(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := server.GetDB().(*sql.DB)

	userID := helpers.CreateTestUser(t, database, "uiduser")
	helpers.InsertTestMail(t, database, "uiduser", "Message 1", "sender@test.com", "uiduser@localhost", "INBOX")

	inboxID, _ := db.GetMailboxByName(database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		SelectedMailboxID: inboxID,
	}

	// UID FETCH 1 FLAGS (UID not explicitly requested)
	server.HandleUID(conn, "U003", []string{"UID", "UID", "FETCH", "1", "FLAGS"}, state)

	response := conn.GetWrittenData()

	// UID must be included even though not requested
	if !strings.Contains(response, "UID 1") {
		t.Errorf("Expected UID to be included in response, got: %s", response)
	}
	if !strings.Contains(response, "FLAGS") {
		t.Errorf("Expected FLAGS in response, got: %s", response)
	}
}

// TestUIDFetch_NonExistentUID tests that non-existent UIDs are silently ignored
func TestUIDFetch_NonExistentUID(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := server.GetDB().(*sql.DB)

	userID := helpers.CreateTestUser(t, database, "uiduser")
	helpers.InsertTestMail(t, database, "uiduser", "Message 1", "sender@test.com", "uiduser@localhost", "INBOX")

	inboxID, _ := db.GetMailboxByName(database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		SelectedMailboxID: inboxID,
	}

	// UID FETCH 999 FLAGS (non-existent UID)
	server.HandleUID(conn, "U004", []string{"UID", "UID", "FETCH", "999", "FLAGS"}, state)

	response := conn.GetWrittenData()

	// Should return OK without error or data
	if !strings.Contains(response, "U004 OK UID FETCH completed") {
		t.Errorf("Expected OK response, got: %s", response)
	}
	// Should not have FETCH response
	if strings.Contains(response, "* ") && strings.Contains(response, "FETCH") {
		t.Errorf("Expected no FETCH response for non-existent UID, got: %s", response)
	}
}

// TestUIDFetch_StarRange tests UID FETCH with * (highest UID)
func TestUIDFetch_StarRange(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := server.GetDB().(*sql.DB)

	userID := helpers.CreateTestUser(t, database, "uiduser")
	helpers.InsertTestMail(t, database, "uiduser", "Message 1", "sender@test.com", "uiduser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "uiduser", "Message 2", "sender@test.com", "uiduser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "uiduser", "Message 3", "sender@test.com", "uiduser@localhost", "INBOX")

	inboxID, _ := db.GetMailboxByName(database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		SelectedMailboxID: inboxID,
	}

	// UID FETCH * FLAGS
	server.HandleUID(conn, "U005", []string{"UID", "UID", "FETCH", "*", "FLAGS"}, state)

	response := conn.GetWrittenData()

	// Should fetch only the last message
	if !strings.Contains(response, "* 3 FETCH") {
		t.Errorf("Expected message 3 FETCH response, got: %s", response)
	}
	if !strings.Contains(response, "UID 3") {
		t.Errorf("Expected UID 3, got: %s", response)
	}
}

// TestUIDSearch_ALL tests UID SEARCH ALL
func TestUIDSearch_ALL(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := server.GetDB().(*sql.DB)

	userID := helpers.CreateTestUser(t, database, "uiduser")
	helpers.InsertTestMail(t, database, "uiduser", "Message 1", "sender@test.com", "uiduser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "uiduser", "Message 2", "sender@test.com", "uiduser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "uiduser", "Message 3", "sender@test.com", "uiduser@localhost", "INBOX")

	inboxID, _ := db.GetMailboxByName(database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		SelectedMailboxID: inboxID,
	}

	// UID SEARCH ALL
	server.HandleUID(conn, "U006", []string{"UID", "UID", "SEARCH", "ALL"}, state)

	response := conn.GetWrittenData()

	// Should return UIDs (not sequence numbers)
	if !strings.Contains(response, "* SEARCH 1 2 3") {
		t.Errorf("Expected UIDs 1 2 3, got: %s", response)
	}
	if !strings.Contains(response, "U006 OK UID SEARCH completed") {
		t.Errorf("Expected OK response, got: %s", response)
	}
}

// TestUIDSearch_UIDRange tests UID SEARCH with UID range
func TestUIDSearch_UIDRange(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := server.GetDB().(*sql.DB)

	userID := helpers.CreateTestUser(t, database, "uiduser")

	// Insert messages with specific UIDs
	msgID1 := helpers.InsertTestMail(t, database, "uiduser", "Message 1", "sender@test.com", "uiduser@localhost", "INBOX")
	msgID2 := helpers.InsertTestMail(t, database, "uiduser", "Message 2", "sender@test.com", "uiduser@localhost", "INBOX")
	msgID3 := helpers.InsertTestMail(t, database, "uiduser", "Message 3", "sender@test.com", "uiduser@localhost", "INBOX")

	inboxID, _ := db.GetMailboxByName(database, userID, "INBOX")

	// Set specific UIDs
	database.Exec("UPDATE message_mailbox SET uid = 443 WHERE message_id = ?", msgID1)
	database.Exec("UPDATE message_mailbox SET uid = 495 WHERE message_id = ?", msgID2)
	database.Exec("UPDATE message_mailbox SET uid = 557 WHERE message_id = ?", msgID3)

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		SelectedMailboxID: inboxID,
	}

	// UID SEARCH 1:100 UID 443:557
	server.HandleUID(conn, "U007", []string{"UID", "UID", "SEARCH", "1:100", "UID", "443:557"}, state)

	response := conn.GetWrittenData()

	// Should return UIDs in the UID range
	if !strings.Contains(response, "443") && !strings.Contains(response, "495") && !strings.Contains(response, "557") {
		t.Errorf("Expected UIDs 443, 495, 557, got: %s", response)
	}
}

// TestUIDStore_FLAGS tests UID STORE FLAGS operation
func TestUIDStore_FLAGS(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := server.GetDB().(*sql.DB)

	userID := helpers.CreateTestUser(t, database, "uiduser")
	msgID := helpers.InsertTestMail(t, database, "uiduser", "Message 1", "sender@test.com", "uiduser@localhost", "INBOX")

	inboxID, _ := db.GetMailboxByName(database, userID, "INBOX")

	// Set initial flags
	database.Exec("UPDATE message_mailbox SET flags = '\\Seen' WHERE message_id = ?", msgID)

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		SelectedMailboxID: inboxID,
	}

	// UID STORE 1 FLAGS (\Deleted)
	server.HandleUID(conn, "U008", []string{"UID", "UID", "STORE", "1", "FLAGS", "(\\Deleted)"}, state)

	response := conn.GetWrittenData()

	// Should return FETCH with UID
	if !strings.Contains(response, "* 1 FETCH") {
		t.Errorf("Expected FETCH response, got: %s", response)
	}
	if !strings.Contains(response, "UID 1") {
		t.Errorf("Expected UID in response, got: %s", response)
	}
	if !strings.Contains(response, "\\Deleted") {
		t.Errorf("Expected \\Deleted flag, got: %s", response)
	}
	if !strings.Contains(response, "U008 OK UID STORE completed") {
		t.Errorf("Expected OK response, got: %s", response)
	}
}

// TestUIDStore_SILENT tests UID STORE with .SILENT suffix
func TestUIDStore_SILENT(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := server.GetDB().(*sql.DB)

	userID := helpers.CreateTestUser(t, database, "uiduser")
	helpers.InsertTestMail(t, database, "uiduser", "Message 1", "sender@test.com", "uiduser@localhost", "INBOX")

	inboxID, _ := db.GetMailboxByName(database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		SelectedMailboxID: inboxID,
	}

	// UID STORE 1 FLAGS.SILENT (\Deleted)
	server.HandleUID(conn, "U009", []string{"UID", "UID", "STORE", "1", "FLAGS.SILENT", "(\\Deleted)"}, state)

	response := conn.GetWrittenData()

	// Should NOT return FETCH response
	if strings.Contains(response, "* 1 FETCH") {
		t.Errorf("Expected no FETCH response with .SILENT, got: %s", response)
	}
	if !strings.Contains(response, "U009 OK UID STORE completed") {
		t.Errorf("Expected OK response, got: %s", response)
	}
}

// TestUIDStore_NonExistentUID tests UID STORE with non-existent UID
func TestUIDStore_NonExistentUID(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := server.GetDB().(*sql.DB)

	userID := helpers.CreateTestUser(t, database, "uiduser")
	helpers.InsertTestMail(t, database, "uiduser", "Message 1", "sender@test.com", "uiduser@localhost", "INBOX")

	inboxID, _ := db.GetMailboxByName(database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		SelectedMailboxID: inboxID,
	}

	// UID STORE 999 FLAGS (\Deleted) - non-existent UID
	server.HandleUID(conn, "U010", []string{"UID", "UID", "STORE", "999", "FLAGS", "(\\Deleted)"}, state)

	response := conn.GetWrittenData()

	// Should return OK without error
	if !strings.Contains(response, "U010 OK UID STORE completed") {
		t.Errorf("Expected OK response, got: %s", response)
	}
}

// TestUIDCopy_SingleMessage tests UID COPY with single UID
func TestUIDCopy_SingleMessage(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := server.GetDB().(*sql.DB)

	userID := helpers.CreateTestUser(t, database, "uiduser")
	helpers.InsertTestMail(t, database, "uiduser", "Message 1", "sender@test.com", "uiduser@localhost", "INBOX")
	helpers.CreateMailbox(t, database, "uiduser", "Sent")

	inboxID, _ := db.GetMailboxByName(database, userID, "INBOX")
	sentID, _ := db.GetMailboxByName(database, userID, "Sent")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		SelectedMailboxID: inboxID,
	}

	// UID COPY 1 Sent
	server.HandleUID(conn, "U011", []string{"UID", "UID", "COPY", "1", "Sent"}, state)

	response := conn.GetWrittenData()

	if !strings.Contains(response, "U011 OK UID COPY completed") {
		t.Errorf("Expected OK response, got: %s", response)
	}

	// Verify message was copied
	var count int
	database.QueryRow("SELECT COUNT(*) FROM message_mailbox WHERE mailbox_id = ?", sentID).Scan(&count)
	if count != 1 {
		t.Errorf("Expected 1 message in Sent folder, got %d", count)
	}
}

// TestUIDCopy_UIDRange tests UID COPY with UID range
func TestUIDCopy_UIDRange(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := server.GetDB().(*sql.DB)

	userID := helpers.CreateTestUser(t, database, "uiduser")
	helpers.InsertTestMail(t, database, "uiduser", "Message 1", "sender@test.com", "uiduser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "uiduser", "Message 2", "sender@test.com", "uiduser@localhost", "INBOX")
	helpers.InsertTestMail(t, database, "uiduser", "Message 3", "sender@test.com", "uiduser@localhost", "INBOX")
	helpers.CreateMailbox(t, database, "uiduser", "Archive")

	inboxID, _ := db.GetMailboxByName(database, userID, "INBOX")
	archiveID, _ := db.GetMailboxByName(database, userID, "Archive")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		SelectedMailboxID: inboxID,
	}

	// UID COPY 1:2 Archive
	server.HandleUID(conn, "U012", []string{"UID", "UID", "COPY", "1:2", "Archive"}, state)

	response := conn.GetWrittenData()

	if !strings.Contains(response, "U012 OK UID COPY completed") {
		t.Errorf("Expected OK response, got: %s", response)
	}

	// Verify 2 messages were copied
	var count int
	database.QueryRow("SELECT COUNT(*) FROM message_mailbox WHERE mailbox_id = ?", archiveID).Scan(&count)
	if count != 2 {
		t.Errorf("Expected 2 messages in Archive folder, got %d", count)
	}
}

// TestUIDCopy_NonExistentDestination tests UID COPY to non-existent mailbox
func TestUIDCopy_NonExistentDestination(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := server.GetDB().(*sql.DB)

	userID := helpers.CreateTestUser(t, database, "uiduser")
	helpers.InsertTestMail(t, database, "uiduser", "Message 1", "sender@test.com", "uiduser@localhost", "INBOX")

	inboxID, _ := db.GetMailboxByName(database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		SelectedMailboxID: inboxID,
	}

	// UID COPY 1 NonExistent
	server.HandleUID(conn, "U013", []string{"UID", "UID", "COPY", "1", "NonExistent"}, state)

	response := conn.GetWrittenData()

	if !strings.Contains(response, "U013 NO [TRYCREATE]") {
		t.Errorf("Expected TRYCREATE response, got: %s", response)
	}
}

// TestUIDCopy_NonExistentUID tests UID COPY with non-existent UID
func TestUIDCopy_NonExistentUID(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := server.GetDB().(*sql.DB)

	userID := helpers.CreateTestUser(t, database, "uiduser")
	helpers.InsertTestMail(t, database, "uiduser", "Message 1", "sender@test.com", "uiduser@localhost", "INBOX")
	helpers.CreateMailbox(t, database, "uiduser", "Sent")

	inboxID, _ := db.GetMailboxByName(database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		SelectedMailboxID: inboxID,
	}

	// UID COPY 999 Sent (non-existent UID)
	server.HandleUID(conn, "U014", []string{"UID", "UID", "COPY", "999", "Sent"}, state)

	response := conn.GetWrittenData()

	// Should return OK without error
	if !strings.Contains(response, "U014 OK UID COPY completed") {
		t.Errorf("Expected OK response, got: %s", response)
	}
}

// TestUIDCommand_BadSubCommand tests UID with unknown sub-command
func TestUIDCommand_BadSubCommand(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := server.GetDB().(*sql.DB)

	userID := helpers.CreateTestUser(t, database, "uiduser")
	inboxID, _ := db.GetMailboxByName(database, userID, "INBOX")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		SelectedMailboxID: inboxID,
	}

	// UID INVALID
	server.HandleUID(conn, "U015", []string{"UID", "UID", "INVALID"}, state)

	response := conn.GetWrittenData()

	if !strings.Contains(response, "U015 BAD Unknown UID command: INVALID") {
		t.Errorf("Expected BAD response, got: %s", response)
	}
}

// TestUIDCommand_TagHandling tests various tag formats
func TestUIDCommand_TagHandling(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	database := server.GetDB().(*sql.DB)

	userID := helpers.CreateTestUser(t, database, "uiduser")
	helpers.InsertTestMail(t, database, "uiduser", "Message 1", "sender@test.com", "uiduser@localhost", "INBOX")

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
		{"uid1", "uid1"},
		{"TAG-123", "TAG-123"},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Tag_%s", tc.tag), func(t *testing.T) {
			conn.ClearWriteBuffer()
			server.HandleUID(conn, tc.tag, []string{"UID", "UID", "FETCH", "1", "FLAGS"}, state)

			response := conn.GetWrittenData()
			if !strings.Contains(response, fmt.Sprintf("%s OK UID FETCH completed", tc.expectedTag)) {
				t.Errorf("Expected tag %s in response, got: %s", tc.expectedTag, response)
			}
		})
	}
}
