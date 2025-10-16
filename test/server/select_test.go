//go:build test
// +build test

package server_test

import (
	"fmt"
	"strings"
	"testing"

	"go-imap/internal/db"
	"go-imap/internal/models"
	"go-imap/test/helpers"
)

// TestSelectCommand_BasicFlow tests the basic SELECT command with an existing mailbox
func TestSelectCommand_BasicFlow(t *testing.T) {
	s, cleanup := helpers.SetupTestServer(t)
	defer cleanup()

	conn := helpers.NewMockTLSConn()
	state := helpers.SetupAuthenticatedState(t, s, "testuser")

	// Insert test messages
	database := helpers.CreateTestDB(t)
	defer database.Close()
	helpers.CreateTestUserTable(t, database, "testuser")
	helpers.InsertTestMail(t, database, "testuser", "Test Subject 1", "sender1@example.com", "recipient@example.com", "INBOX")
	helpers.InsertTestMail(t, database, "testuser", "Test Subject 2", "sender2@example.com", "recipient@example.com", "INBOX")
	
	s = helpers.TestServerWithDB(database)

	// Execute SELECT command
	s.HandleSelect(conn, "A001", []string{"A001", "SELECT", "INBOX"}, state)

	response := conn.GetWrittenData()

	// Verify REQUIRED untagged responses per RFC 3501
	if !strings.Contains(response, "* FLAGS") {
		t.Errorf("Missing FLAGS untagged response. Got: %s", response)
	}
	if !strings.Contains(response, "EXISTS") {
		t.Errorf("Missing EXISTS untagged response. Got: %s", response)
	}
	if !strings.Contains(response, "RECENT") {
		t.Errorf("Missing RECENT untagged response. Got: %s", response)
	}

	// Verify REQUIRED OK untagged responses
	if !strings.Contains(response, "OK [UIDVALIDITY") {
		t.Errorf("Missing UIDVALIDITY OK untagged response. Got: %s", response)
	}
	if !strings.Contains(response, "OK [UIDNEXT") {
		t.Errorf("Missing UIDNEXT OK untagged response. Got: %s", response)
	}
	if !strings.Contains(response, "OK [PERMANENTFLAGS") {
		t.Errorf("Missing PERMANENTFLAGS OK untagged response. Got: %s", response)
	}

	// Verify tagged OK response with READ-WRITE
	if !strings.Contains(response, "A001 OK [READ-WRITE] SELECT completed") {
		t.Errorf("Missing or incorrect tagged OK response. Got: %s", response)
	}

	// Verify state was updated
	if state.SelectedFolder != "INBOX" {
		t.Errorf("Expected SelectedFolder to be 'INBOX', got: %s", state.SelectedFolder)
	}
}

// TestSelectCommand_WithUnseenMessages tests SELECT with unseen messages
func TestSelectCommand_WithUnseenMessages(t *testing.T) {
	database := helpers.CreateTestDB(t)
	defer database.Close()
	helpers.CreateTestUser(t, database, "testuser")

	// Insert test messages using helpers
	messageID1 := helpers.InsertTestMail(t, database, "testuser", "Seen Message", "sender@example.com", "recipient@example.com", "INBOX")
	helpers.InsertTestMail(t, database, "testuser", "First Unseen", "sender@example.com", "recipient@example.com", "INBOX")
	helpers.InsertTestMail(t, database, "testuser", "Second Unseen", "sender@example.com", "recipient@example.com", "INBOX")
	
	// Set first message as seen
	_, err := database.Exec("UPDATE message_mailbox SET flags = ? WHERE message_id = ?", "\\Seen", messageID1)
	if err != nil {
		t.Fatalf("Failed to set message as seen: %v", err)
	}

	s := helpers.TestServerWithDB(database)
	conn := helpers.NewMockTLSConn()
	state := helpers.SetupAuthenticatedState(t, s, "testuser")

	s.HandleSelect(conn, "A002", []string{"A002", "SELECT", "INBOX"}, state)

	response := conn.GetWrittenData()

	// Should include UNSEEN response pointing to the first unseen message (sequence number 2)
	if !strings.Contains(response, "OK [UNSEEN 2]") {
		t.Errorf("Expected UNSEEN response with sequence number 2. Got: %s", response)
	}

	// Verify EXISTS count
	if !strings.Contains(response, "* 3 EXISTS") {
		t.Errorf("Expected 3 EXISTS. Got: %s", response)
	}
}

// TestSelectCommand_EmptyMailbox tests SELECT on an empty mailbox
func TestSelectCommand_EmptyMailbox(t *testing.T) {
	database := helpers.CreateTestDB(t)
	defer database.Close()
	helpers.CreateTestUserTable(t, database, "testuser")

	s := helpers.TestServerWithDB(database)
	conn := helpers.NewMockTLSConn()
	state := helpers.SetupAuthenticatedState(t, s, "testuser")

	s.HandleSelect(conn, "A003", []string{"A003", "SELECT", "INBOX"}, state)

	response := conn.GetWrittenData()

	// Should have 0 messages
	if !strings.Contains(response, "* 0 EXISTS") {
		t.Errorf("Expected 0 EXISTS for empty mailbox. Got: %s", response)
	}
	if !strings.Contains(response, "* 0 RECENT") {
		t.Errorf("Expected 0 RECENT for empty mailbox. Got: %s", response)
	}

	// Should still have all required responses
	if !strings.Contains(response, "* FLAGS") {
		t.Errorf("Missing FLAGS response. Got: %s", response)
	}
	if !strings.Contains(response, "OK [UIDNEXT 1]") {
		t.Errorf("Expected UIDNEXT 1 for empty mailbox. Got: %s", response)
	}

	// Should complete successfully
	if !strings.Contains(response, "A003 OK [READ-WRITE] SELECT completed") {
		t.Errorf("Expected successful completion. Got: %s", response)
	}
}

// TestSelectCommand_UnauthenticatedUser tests SELECT without authentication
func TestSelectCommand_UnauthenticatedUser(t *testing.T) {
	s, cleanup := helpers.SetupTestServer(t)
	defer cleanup()

	conn := helpers.NewMockTLSConn()
	state := &models.ClientState{
		Authenticated: false,
	}

	s.HandleSelect(conn, "A004", []string{"A004", "SELECT", "INBOX"}, state)

	response := conn.GetWrittenData()

	// Should get NO response
	if !strings.Contains(response, "A004 NO Please authenticate first") {
		t.Errorf("Expected authentication required error. Got: %s", response)
	}

	// Should NOT set SelectedFolder
	if state.SelectedFolder != "" {
		t.Errorf("Expected SelectedFolder to remain empty, got: %s", state.SelectedFolder)
	}
}

// TestSelectCommand_MissingMailboxName tests SELECT without mailbox name
func TestSelectCommand_MissingMailboxName(t *testing.T) {
	s, cleanup := helpers.SetupTestServer(t)
	defer cleanup()

	conn := helpers.NewMockTLSConn()
	state := helpers.SetupAuthenticatedState(t, s, "testuser")

	s.HandleSelect(conn, "A005", []string{"A005", "SELECT"}, state)

	response := conn.GetWrittenData()

	// Should get BAD response
	if !strings.Contains(response, "A005 BAD SELECT requires folder name") {
		t.Errorf("Expected BAD response for missing folder name. Got: %s", response)
	}
}

// TestSelectCommand_QuotedMailboxName tests SELECT with quoted mailbox name
func TestSelectCommand_QuotedMailboxName(t *testing.T) {
	database := helpers.CreateTestDB(t)
	defer database.Close()
	userID := helpers.CreateTestUser(t, database, "testuser")

	// Create a mailbox with spaces and insert a message
	_, err := db.CreateMailbox(database, userID, "Sent Items", "")
	if err != nil {
		t.Fatalf("Failed to create mailbox: %v", err)
	}
	helpers.InsertTestMail(t, database, "testuser", "Test", "sender@example.com", "recipient@example.com", "Sent Items")

	s := helpers.TestServerWithDB(database)
	conn := helpers.NewMockTLSConn()
	state := helpers.SetupAuthenticatedState(t, s, "testuser")

	s.HandleSelect(conn, "A006", []string{"A006", "SELECT", "\"Sent Items\""}, state)

	response := conn.GetWrittenData()

	// Should successfully select the folder
	if !strings.Contains(response, "A006 OK [READ-WRITE] SELECT completed") {
		t.Errorf("Expected successful selection of quoted folder name. Got: %s", response)
	}

	// Verify the folder was selected (quotes should be stripped)
	if state.SelectedFolder != "Sent Items" {
		t.Errorf("Expected SelectedFolder to be 'Sent Items', got: %s", state.SelectedFolder)
	}

	// Should show the correct message count
	if !strings.Contains(response, "* 1 EXISTS") {
		t.Errorf("Expected 1 EXISTS. Got: %s", response)
	}
}

// TestSelectCommand_SwitchingMailboxes tests selecting a different mailbox
func TestSelectCommand_SwitchingMailboxes(t *testing.T) {
	database := helpers.CreateTestDB(t)
	defer database.Close()
	helpers.CreateTestUser(t, database, "testuser")

	// Insert messages into different folders using helpers
	helpers.InsertTestMail(t, database, "testuser", "Inbox Msg", "sender@example.com", "recipient@example.com", "INBOX")
	helpers.InsertTestMail(t, database, "testuser", "Draft Msg 1", "sender@example.com", "recipient@example.com", "Drafts")
	helpers.InsertTestMail(t, database, "testuser", "Draft Msg 2", "sender@example.com", "recipient@example.com", "Drafts")

	s := helpers.TestServerWithDB(database)
	conn := helpers.NewMockTLSConn()
	state := helpers.SetupAuthenticatedState(t, s, "testuser")

	// First, select INBOX
	s.HandleSelect(conn, "A007", []string{"A007", "SELECT", "INBOX"}, state)
	response1 := conn.GetWrittenData()

	if !strings.Contains(response1, "* 1 EXISTS") {
		t.Errorf("Expected 1 message in INBOX. Got: %s", response1)
	}
	if state.SelectedFolder != "INBOX" {
		t.Errorf("Expected SelectedFolder to be 'INBOX', got: %s", state.SelectedFolder)
	}

	// Clear buffer and select Drafts
	conn.ClearWriteBuffer()
	s.HandleSelect(conn, "A008", []string{"A008", "SELECT", "Drafts"}, state)
	response2 := conn.GetWrittenData()

	if !strings.Contains(response2, "* 2 EXISTS") {
		t.Errorf("Expected 2 messages in Drafts. Got: %s", response2)
	}
	if state.SelectedFolder != "Drafts" {
		t.Errorf("Expected SelectedFolder to be 'Drafts', got: %s", state.SelectedFolder)
	}
}

// TestSelectCommand_StateTracking tests that state is properly tracked
func TestSelectCommand_StateTracking(t *testing.T) {
	database := helpers.CreateTestDB(t)
	defer database.Close()
	helpers.CreateTestUserTable(t, database, "testuser")

	// Insert messages with different flags
	messageID1 := helpers.InsertTestMail(t, database, "testuser", "Seen Msg", "sender@example.com", "recipient@example.com", "INBOX")
	helpers.InsertTestMail(t, database, "testuser", "Unseen Msg 1", "sender@example.com", "recipient@example.com", "INBOX")
	helpers.InsertTestMail(t, database, "testuser", "Unseen Msg 2", "sender@example.com", "recipient@example.com", "INBOX")
	
	// Set first message as seen
	_, err := database.Exec("UPDATE message_mailbox SET flags = ? WHERE message_id = ?", "\\Seen", messageID1)
	if err != nil {
		t.Fatalf("Failed to set message as seen: %v", err)
	}

	s := helpers.TestServerWithDB(database)
	conn := helpers.NewMockTLSConn()
	state := helpers.SetupAuthenticatedState(t, s, "testuser")

	s.HandleSelect(conn, "A009", []string{"A009", "SELECT", "INBOX"}, state)

	// Verify state tracking fields are set
	if state.LastMessageCount != 3 {
		t.Errorf("Expected LastMessageCount to be 3, got: %d", state.LastMessageCount)
	}
	if state.LastRecentCount != 2 {
		t.Errorf("Expected LastRecentCount to be 2 (unseen messages), got: %d", state.LastRecentCount)
	}
}

// TestSelectCommand_UIDNext tests UIDNEXT calculation
func TestSelectCommand_UIDNext(t *testing.T) {
	database := helpers.CreateTestDB(t)
	defer database.Close()
	helpers.CreateTestUser(t, database, "testuser")

	// Insert messages to establish UIDs using helpers
	helpers.InsertTestMail(t, database, "testuser", "Msg 1", "sender@example.com", "recipient@example.com", "INBOX")
	helpers.InsertTestMail(t, database, "testuser", "Msg 2", "sender@example.com", "recipient@example.com", "INBOX")
	helpers.InsertTestMail(t, database, "testuser", "Msg 3", "sender@example.com", "recipient@example.com", "INBOX")

	s := helpers.TestServerWithDB(database)
	conn := helpers.NewMockTLSConn()
	state := helpers.SetupAuthenticatedState(t, s, "testuser")

	s.HandleSelect(conn, "A010", []string{"A010", "SELECT", "INBOX"}, state)
	response := conn.GetWrittenData()

	// UIDNEXT should be max(id) + 1 = 4
	if !strings.Contains(response, "OK [UIDNEXT 4]") {
		t.Errorf("Expected UIDNEXT 4. Got: %s", response)
	}
}

// TestExamineCommand_ReadOnly tests the EXAMINE command (read-only mode)
func TestExamineCommand_ReadOnly(t *testing.T) {
	database := helpers.CreateTestDB(t)
	defer database.Close()
	helpers.CreateTestUserTable(t, database, "testuser")
	helpers.InsertTestMail(t, database, "testuser", "Test Message", "sender@example.com", "recipient@example.com", "INBOX")

	s := helpers.TestServerWithDB(database)
	conn := helpers.NewMockTLSConn()
	state := helpers.SetupAuthenticatedState(t, s, "testuser")

	// EXAMINE should use same handler but return READ-ONLY
	s.HandleExamine(conn, "A011", []string{"A011", "EXAMINE", "INBOX"}, state)
	response := conn.GetWrittenData()

	// Should complete with READ-ONLY
	if !strings.Contains(response, "A011 OK [READ-ONLY] EXAMINE completed") {
		t.Errorf("Expected READ-ONLY completion for EXAMINE. Got: %s", response)
	}

	// Should still have all required responses
	if !strings.Contains(response, "* FLAGS") {
		t.Errorf("Missing FLAGS response. Got: %s", response)
	}
	if !strings.Contains(response, "* 1 EXISTS") {
		t.Errorf("Missing EXISTS response. Got: %s", response)
	}
}

// TestSelectCommand_RFC3501_Example tests the exact example from RFC 3501
func TestSelectCommand_RFC3501_Example(t *testing.T) {
	database := helpers.CreateTestDB(t)
	defer database.Close()
	helpers.CreateTestUserTable(t, database, "testuser")

	// Create a scenario similar to RFC 3501 example:
	// 172 messages total, message 12 is first unseen, 1 recent

	// Insert 11 seen messages
	for i := 1; i <= 11; i++ {
		messageID := helpers.InsertTestMail(t, database, "testuser", fmt.Sprintf("Msg %d", i), "sender@example.com", "recipient@example.com", "INBOX")
		// Set as seen
		_, err := database.Exec("UPDATE message_mailbox SET flags = ? WHERE message_id = ?", "\\Seen", messageID)
		if err != nil {
			t.Fatalf("Failed to set message %d as seen: %v", i, err)
		}
	}

	// Insert message 12 - first unseen
	helpers.InsertTestMail(t, database, "testuser", "Msg 12", "sender@example.com", "recipient@example.com", "INBOX")

	// Insert remaining unseen messages
	for i := 13; i <= 20; i++ {
		helpers.InsertTestMail(t, database, "testuser", fmt.Sprintf("Msg %d", i), "sender@example.com", "recipient@example.com", "INBOX")
	}

	s := helpers.TestServerWithDB(database)
	conn := helpers.NewMockTLSConn()
	state := helpers.SetupAuthenticatedState(t, s, "testuser")

	s.HandleSelect(conn, "A142", []string{"A142", "SELECT", "INBOX"}, state)
	response := conn.GetWrittenData()

	// Verify key elements from RFC example
	if !strings.Contains(response, "* 20 EXISTS") {
		t.Errorf("Expected EXISTS count. Got: %s", response)
	}
	if !strings.Contains(response, "OK [UNSEEN 12]") {
		t.Errorf("Expected first unseen to be message 12. Got: %s", response)
	}
	if !strings.Contains(response, "OK [UIDVALIDITY") {
		t.Errorf("Expected UIDVALIDITY. Got: %s", response)
	}
	if !strings.Contains(response, "OK [UIDNEXT") {
		t.Errorf("Expected UIDNEXT. Got: %s", response)
	}
	if !strings.Contains(response, "* FLAGS") {
		t.Errorf("Expected FLAGS. Got: %s", response)
	}
	if !strings.Contains(response, "OK [PERMANENTFLAGS") {
		t.Errorf("Expected PERMANENTFLAGS. Got: %s", response)
	}
	if !strings.Contains(response, "A142 OK [READ-WRITE] SELECT completed") {
		t.Errorf("Expected tagged OK response. Got: %s", response)
	}
}

// TestSelectCommand_AllMessagesSeen tests SELECT when all messages are seen (no UNSEEN)
func TestSelectCommand_AllMessagesSeen(t *testing.T) {
	database := helpers.CreateTestDB(t)
	defer database.Close()
	helpers.CreateTestUserTable(t, database, "testuser")

	// Insert only seen messages
	messageID1 := helpers.InsertTestMail(t, database, "testuser", "Seen 1", "sender@example.com", "recipient@example.com", "INBOX")
	messageID2 := helpers.InsertTestMail(t, database, "testuser", "Seen 2", "sender@example.com", "recipient@example.com", "INBOX")
	
	// Set both messages as seen
	_, err := database.Exec("UPDATE message_mailbox SET flags = ? WHERE message_id = ?", "\\Seen", messageID1)
	if err != nil {
		t.Fatalf("Failed to set message 1 as seen: %v", err)
	}
	_, err = database.Exec("UPDATE message_mailbox SET flags = ? WHERE message_id = ?", "\\Seen", messageID2)
	if err != nil {
		t.Fatalf("Failed to set message 2 as seen: %v", err)
	}

	s := helpers.TestServerWithDB(database)
	conn := helpers.NewMockTLSConn()
	state := helpers.SetupAuthenticatedState(t, s, "testuser")

	s.HandleSelect(conn, "A012", []string{"A012", "SELECT", "INBOX"}, state)
	response := conn.GetWrittenData()

	// Should NOT include UNSEEN response when all messages are seen
	if strings.Contains(response, "OK [UNSEEN") {
		t.Errorf("Should not include UNSEEN when all messages are seen. Got: %s", response)
	}

	// Should still complete successfully
	if !strings.Contains(response, "A012 OK [READ-WRITE] SELECT completed") {
		t.Errorf("Expected successful completion. Got: %s", response)
	}

	// RECENT count should be 0 (all are seen)
	if !strings.Contains(response, "* 0 RECENT") {
		t.Errorf("Expected 0 RECENT messages. Got: %s", response)
	}
}

// TestExamineCommand_RFC3501_Compliance tests EXAMINE per RFC 3501 specifications
func TestExamineCommand_RFC3501_Compliance(t *testing.T) {
	database := helpers.CreateTestDB(t)
	defer database.Close()
	userID := helpers.CreateTestUser(t, database, "testuser")

	// Create the blurdybloop mailbox
	_, err := db.CreateMailbox(database, userID, "blurdybloop", "")
	if err != nil {
		t.Fatalf("Failed to create blurdybloop mailbox: %v", err)
	}

	// Create a mailbox with messages similar to RFC 3501 example
	// Insert 7 seen messages
	for i := 1; i <= 7; i++ {
		messageID := helpers.InsertTestMail(t, database, "testuser", fmt.Sprintf("Msg %d", i), "sender@example.com", "recipient@example.com", "blurdybloop")
		// Set as seen
		_, err := database.Exec("UPDATE message_mailbox SET flags = ? WHERE message_id = ?", "\\Seen", messageID)
		if err != nil {
			t.Fatalf("Failed to set message %d as seen: %v", i, err)
		}
	}

	// Insert message 8 - first unseen
	helpers.InsertTestMail(t, database, "testuser", "Msg 8", "sender@example.com", "recipient@example.com", "blurdybloop")

	// Insert remaining unseen messages (9-17)
	for i := 9; i <= 17; i++ {
		helpers.InsertTestMail(t, database, "testuser", fmt.Sprintf("Msg %d", i), "sender@example.com", "recipient@example.com", "blurdybloop")
	}

	s := helpers.TestServerWithDB(database)
	conn := helpers.NewMockTLSConn()
	state := helpers.SetupAuthenticatedState(t, s, "testuser")

	s.HandleExamine(conn, "A932", []string{"A932", "EXAMINE", "blurdybloop"}, state)
	response := conn.GetWrittenData()

	// Verify all REQUIRED responses per RFC 3501
	if !strings.Contains(response, "* 17 EXISTS") {
		t.Errorf("Expected 17 EXISTS. Got: %s", response)
	}

	if !strings.Contains(response, "* OK [UNSEEN 8]") {
		t.Errorf("Expected UNSEEN 8. Got: %s", response)
	}

	if !strings.Contains(response, "* OK [UIDVALIDITY") {
		t.Errorf("Expected UIDVALIDITY. Got: %s", response)
	}

	if !strings.Contains(response, "* OK [UIDNEXT") {
		t.Errorf("Expected UIDNEXT. Got: %s", response)
	}

	if !strings.Contains(response, "* FLAGS") {
		t.Errorf("Expected FLAGS. Got: %s", response)
	}

	// CRITICAL: EXAMINE must return empty PERMANENTFLAGS
	if !strings.Contains(response, "* OK [PERMANENTFLAGS ()]") {
		t.Errorf("Expected empty PERMANENTFLAGS for EXAMINE. Got: %s", response)
	}

	// Must complete with READ-ONLY
	if !strings.Contains(response, "A932 OK [READ-ONLY] EXAMINE completed") {
		t.Errorf("Expected READ-ONLY completion. Got: %s", response)
	}

	// Verify the mailbox is selected
	if state.SelectedFolder != "blurdybloop" {
		t.Errorf("Expected SelectedFolder to be 'blurdybloop', got: %s", state.SelectedFolder)
	}
}

// TestExamineCommand_EmptyPermanentFlags tests that EXAMINE returns empty PERMANENTFLAGS
func TestExamineCommand_EmptyPermanentFlags(t *testing.T) {
	database := helpers.CreateTestDB(t)
	defer database.Close()
	helpers.CreateTestUserTable(t, database, "testuser")
	helpers.InsertTestMail(t, database, "testuser", "Test Message", "sender@example.com", "recipient@example.com", "INBOX")

	s := helpers.TestServerWithDB(database)
	conn := helpers.NewMockTLSConn()
	state := helpers.SetupAuthenticatedState(t, s, "testuser")

	s.HandleExamine(conn, "A100", []string{"A100", "EXAMINE", "INBOX"}, state)
	response := conn.GetWrittenData()

	// EXAMINE must return empty PERMANENTFLAGS list
	if !strings.Contains(response, "[PERMANENTFLAGS ()]") {
		t.Errorf("EXAMINE must return empty PERMANENTFLAGS. Got: %s", response)
	}

	// Should NOT contain the full permanent flags list
	if strings.Contains(response, "[PERMANENTFLAGS (\\Answered \\Flagged \\Deleted \\Seen \\Draft \\*)]") {
		t.Errorf("EXAMINE should not return modifiable PERMANENTFLAGS. Got: %s", response)
	}
}

// TestSelectVsExamine_PermanentFlags tests the difference in PERMANENTFLAGS between SELECT and EXAMINE
func TestSelectVsExamine_PermanentFlags(t *testing.T) {
	database := helpers.CreateTestDB(t)
	defer database.Close()
	userID := helpers.CreateTestUser(t, database, "testuser")
	helpers.InsertTestMail(t, database, "testuser", "Test Message", "sender@example.com", "recipient@example.com", "INBOX")

	s := helpers.TestServerWithDB(database)

	// Test SELECT
	connSelect := helpers.NewMockTLSConn()
	stateSelect := helpers.SetupAuthenticatedState(t, s, "testuser")

	s.HandleSelect(connSelect, "A101", []string{"A101", "SELECT", "INBOX"}, stateSelect)
	selectResponse := connSelect.GetWrittenData()

	// SELECT should have full PERMANENTFLAGS
	if !strings.Contains(selectResponse, "[PERMANENTFLAGS (\\Answered \\Flagged \\Deleted \\Seen \\Draft \\*)]") {
		t.Errorf("SELECT should return full PERMANENTFLAGS. Got: %s", selectResponse)
	}

	// SELECT should be READ-WRITE
	if !strings.Contains(selectResponse, "[READ-WRITE]") {
		t.Errorf("SELECT should return READ-WRITE. Got: %s", selectResponse)
	}

	// Test EXAMINE
	connExamine := helpers.NewMockTLSConn()
	stateExamine := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
		UserID:        userID,
	}

	s.HandleExamine(connExamine, "A102", []string{"A102", "EXAMINE", "INBOX"}, stateExamine)
	examineResponse := connExamine.GetWrittenData()

	// EXAMINE should have empty PERMANENTFLAGS
	if !strings.Contains(examineResponse, "[PERMANENTFLAGS ()]") {
		t.Errorf("EXAMINE should return empty PERMANENTFLAGS. Got: %s", examineResponse)
	}

	// EXAMINE should be READ-ONLY
	if !strings.Contains(examineResponse, "[READ-ONLY]") {
		t.Errorf("EXAMINE should return READ-ONLY. Got: %s", examineResponse)
	}
}

// TestExamineCommand_UnauthenticatedUser tests EXAMINE without authentication
func TestExamineCommand_UnauthenticatedUser(t *testing.T) {
	s, cleanup := helpers.SetupTestServer(t)
	defer cleanup()

	conn := helpers.NewMockTLSConn()
	state := &models.ClientState{
		Authenticated: false,
	}

	s.HandleExamine(conn, "A103", []string{"A103", "EXAMINE", "INBOX"}, state)
	response := conn.GetWrittenData()

	// Should get NO response
	if !strings.Contains(response, "A103 NO Please authenticate first") {
		t.Errorf("Expected authentication required error. Got: %s", response)
	}
}

// TestExamineCommand_MissingMailboxName tests EXAMINE without mailbox name
func TestExamineCommand_MissingMailboxName(t *testing.T) {
	s, cleanup := helpers.SetupTestServer(t)
	defer cleanup()

	conn := helpers.NewMockTLSConn()
	state := helpers.SetupAuthenticatedState(t, s, "testuser")

	s.HandleExamine(conn, "A104", []string{"A104", "EXAMINE"}, state)
	response := conn.GetWrittenData()

	// Should get BAD response mentioning EXAMINE
	if !strings.Contains(response, "A104 BAD EXAMINE requires folder name") {
		t.Errorf("Expected BAD response for missing folder name. Got: %s", response)
	}
}

// TestExamineCommand_ResponseOrder tests the response order matches RFC 3501 example
func TestExamineCommand_ResponseOrder(t *testing.T) {
	database := helpers.CreateTestDB(t)
	defer database.Close()
	helpers.CreateTestUserTable(t, database, "testuser")
	helpers.InsertTestMail(t, database, "testuser", "Test Message", "sender@example.com", "recipient@example.com", "INBOX")

	s := helpers.TestServerWithDB(database)
	conn := helpers.NewMockTLSConn()
	state := helpers.SetupAuthenticatedState(t, s, "testuser")

	s.HandleExamine(conn, "A105", []string{"A105", "EXAMINE", "INBOX"}, state)
	response := conn.GetWrittenData()

	// Per RFC 3501 EXAMINE example, the order should be:
	// EXISTS, RECENT, OK responses (UNSEEN, UIDVALIDITY, UIDNEXT), FLAGS, PERMANENTFLAGS, tagged OK

	lines := strings.Split(response, "\n")
	var existsLine, recentLine, flagsLine, permanentFlagsLine int

	for i, line := range lines {
		if strings.Contains(line, "EXISTS") {
			existsLine = i
		}
		if strings.Contains(line, "RECENT") {
			recentLine = i
		}
		if strings.Contains(line, "* FLAGS") {
			flagsLine = i
		}
		if strings.Contains(line, "PERMANENTFLAGS") {
			permanentFlagsLine = i
		}
	}

	// EXISTS should come before RECENT
	if existsLine >= recentLine {
		t.Errorf("EXISTS should come before RECENT")
	}

	// FLAGS should come after EXISTS and RECENT for EXAMINE
	if flagsLine != 0 && flagsLine < recentLine {
		t.Errorf("For EXAMINE, FLAGS should come after EXISTS and RECENT")
	}

	// PERMANENTFLAGS should come after FLAGS
	if permanentFlagsLine != 0 && permanentFlagsLine < flagsLine {
		t.Errorf("PERMANENTFLAGS should come after FLAGS")
	}
}
