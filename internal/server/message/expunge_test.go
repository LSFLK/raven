package message_test

import (
	"strings"
	"testing"

	"raven/internal/models"
	"raven/internal/server"
)

// TestExpungeCommand_Unauthenticated tests EXPUNGE before authentication
func TestExpungeCommand_Unauthenticated(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := &models.ClientState{
		Authenticated: false,
	}

	srv.HandleExpunge(conn, "E001", state)

	response := conn.GetWrittenData()
	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Should have 1 line: NO response
	if len(lines) != 1 {
		t.Fatalf("Expected 1 response line, got %d: %v", len(lines), lines)
	}

	// Check NO response for unauthenticated
	expectedNO := "E001 NO Please authenticate first"
	if lines[0] != expectedNO {
		t.Errorf("Expected '%s', got: '%s'", expectedNO, lines[0])
	}
}

// TestExpungeCommand_AuthenticatedNoMailbox tests EXPUNGE when authenticated but no mailbox selected
func TestExpungeCommand_AuthenticatedNoMailbox(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := &models.ClientState{
		Authenticated:     true,
		Username:          "testuser",
		SelectedMailboxID: 0, // No mailbox selected
	}

	srv.HandleExpunge(conn, "E002", state)

	response := conn.GetWrittenData()
	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Should have 1 line: NO response
	if len(lines) != 1 {
		t.Fatalf("Expected 1 response line, got %d: %v", len(lines), lines)
	}

	// Check NO response for no selected mailbox
	expectedNO := "E002 NO No mailbox selected"
	if lines[0] != expectedNO {
		t.Errorf("Expected '%s', got: '%s'", expectedNO, lines[0])
	}
}

// TestExpungeCommand_NoDeletedMessages tests EXPUNGE with no deleted messages
func TestExpungeCommand_NoDeletedMessages(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()

	// Setup authenticated state with selected mailbox
	state := server.SetupAuthenticatedState(t, srv, "testuser")
	database := server.GetDatabaseFromServer(srv)
	mailboxID, err := server.GetMailboxID(t, database, state.UserID, "INBOX")
	if err != nil {
		t.Fatalf("Failed to get INBOX mailbox: %v", err)
	}
	state.SelectedMailboxID = mailboxID
	state.SelectedFolder = "INBOX"

	// Insert some messages (but don't mark them as deleted)
	server.InsertTestMail(t, database, "testuser", "Message 1", "sender@example.com", "testuser@localhost", "INBOX")
	server.InsertTestMail(t, database, "testuser", "Message 2", "sender@example.com", "testuser@localhost", "INBOX")

	srv.HandleExpunge(conn, "E003", state)

	response := conn.GetWrittenData()
	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Should have only 1 line: OK completion (no EXPUNGE responses)
	if len(lines) != 1 {
		t.Fatalf("Expected 1 response line, got %d: %v", len(lines), lines)
	}

	// Should be tagged OK response
	expectedOK := "E003 OK EXPUNGE completed"
	if lines[0] != expectedOK {
		t.Errorf("Expected '%s', got: '%s'", expectedOK, lines[0])
	}
}

// TestExpungeCommand_SingleDeletedMessage tests EXPUNGE with one deleted message
func TestExpungeCommand_SingleDeletedMessage(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()

	// Setup authenticated state with selected mailbox
	state := server.SetupAuthenticatedState(t, srv, "testuser")
	database := server.GetDatabaseFromServer(srv)
	mailboxID, err := server.GetMailboxID(t, database, state.UserID, "INBOX")
	if err != nil {
		t.Fatalf("Failed to get INBOX mailbox: %v", err)
	}
	state.SelectedMailboxID = mailboxID
	state.SelectedFolder = "INBOX"

	// Insert messages
	server.InsertTestMail(t, database, "testuser", "Message 1", "sender@example.com", "testuser@localhost", "INBOX")
	msg2ID := server.InsertTestMail(t, database, "testuser", "Message 2", "sender@example.com", "testuser@localhost", "INBOX")
	server.InsertTestMail(t, database, "testuser", "Message 3", "sender@example.com", "testuser@localhost", "INBOX")

	userDB := server.GetUserDBByID(t, database, state.UserID)

	// Mark message 2 as deleted
	userDB.Exec(`UPDATE message_mailbox SET flags = '\Deleted' WHERE mailbox_id = ? AND message_id = ?`, mailboxID, msg2ID)

	srv.HandleExpunge(conn, "E004", state)

	response := conn.GetWrittenData()
	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Should have 2 lines: 1 EXPUNGE response + 1 OK completion
	if len(lines) != 2 {
		t.Fatalf("Expected 2 response lines, got %d: %v", len(lines), lines)
	}

	// First line should be EXPUNGE for sequence 2
	expectedExpunge := "* 2 EXPUNGE"
	if lines[0] != expectedExpunge {
		t.Errorf("Expected '%s', got: '%s'", expectedExpunge, lines[0])
	}

	// Last line should be tagged OK response
	expectedOK := "E004 OK EXPUNGE completed"
	if lines[1] != expectedOK {
		t.Errorf("Expected '%s', got: '%s'", expectedOK, lines[1])
	}

	// Verify message was deleted
	var countAfter int
	userDB.QueryRow(`SELECT COUNT(*) FROM message_mailbox WHERE mailbox_id = ?`, mailboxID).Scan(&countAfter)
	if countAfter != 2 {
		t.Errorf("Expected 2 messages after EXPUNGE, got %d", countAfter)
	}
}

// TestExpungeCommand_MultipleDeletedMessages tests EXPUNGE with multiple deleted messages
// This tests the RFC 3501 example scenario
func TestExpungeCommand_MultipleDeletedMessages(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()

	// Setup authenticated state with selected mailbox
	state := server.SetupAuthenticatedState(t, srv, "testuser")
	database := server.GetDatabaseFromServer(srv)
	mailboxID, err := server.GetMailboxID(t, database, state.UserID, "INBOX")
	if err != nil {
		t.Fatalf("Failed to get INBOX mailbox: %v", err)
	}
	state.SelectedMailboxID = mailboxID
	state.SelectedFolder = "INBOX"

	// Insert 11 messages to match RFC 3501 example
	var messageIDs []int64
	for i := 1; i <= 11; i++ {
		msgID := server.InsertTestMail(t, database, "testuser",
			"Message "+string(rune('0'+i)), "sender@example.com", "testuser@localhost", "INBOX")
		messageIDs = append(messageIDs, msgID)
	}

	userDB := server.GetUserDBByID(t, database, state.UserID)

	// Mark messages 3, 4, 7, and 11 as deleted (indices 2, 3, 6, 10)
	// Per RFC 3501 example
	userDB.Exec(`UPDATE message_mailbox SET flags = '\Deleted' WHERE mailbox_id = ? AND message_id = ?`, mailboxID, messageIDs[2])
	userDB.Exec(`UPDATE message_mailbox SET flags = '\Deleted' WHERE mailbox_id = ? AND message_id = ?`, mailboxID, messageIDs[3])
	userDB.Exec(`UPDATE message_mailbox SET flags = '\Deleted' WHERE mailbox_id = ? AND message_id = ?`, mailboxID, messageIDs[6])
	userDB.Exec(`UPDATE message_mailbox SET flags = '\Deleted' WHERE mailbox_id = ? AND message_id = ?`, mailboxID, messageIDs[10])

	srv.HandleExpunge(conn, "A202", state)

	response := conn.GetWrittenData()
	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Should have 5 lines: 4 EXPUNGE responses + 1 OK completion
	if len(lines) != 5 {
		t.Fatalf("Expected 5 response lines, got %d: %v", len(lines), lines)
	}

	// Per RFC 3501 example, the EXPUNGE responses should be:
	// * 3 EXPUNGE (for message 3)
	// * 3 EXPUNGE (for message 4, which is now at position 3)
	// * 5 EXPUNGE (for message 7, which is now at position 5)
	// * 8 EXPUNGE (for message 11, which is now at position 8)
	expectedExpunges := []string{
		"* 3 EXPUNGE",
		"* 3 EXPUNGE",
		"* 5 EXPUNGE",
		"* 8 EXPUNGE",
	}

	for i, expected := range expectedExpunges {
		if lines[i] != expected {
			t.Errorf("Line %d: Expected '%s', got: '%s'", i, expected, lines[i])
		}
	}

	// Last line should be tagged OK response
	expectedOK := "A202 OK EXPUNGE completed"
	if lines[4] != expectedOK {
		t.Errorf("Expected '%s', got: '%s'", expectedOK, lines[4])
	}

	// Verify 7 messages remain (11 - 4 deleted)
	var countAfter int
	userDB.QueryRow(`SELECT COUNT(*) FROM message_mailbox WHERE mailbox_id = ?`, mailboxID).Scan(&countAfter)
	if countAfter != 7 {
		t.Errorf("Expected 7 messages after EXPUNGE, got %d", countAfter)
	}
}

// TestExpungeCommand_StateUpdate tests that EXPUNGE updates client state
func TestExpungeCommand_StateUpdate(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()

	// Setup authenticated state with selected mailbox
	state := server.SetupAuthenticatedState(t, srv, "testuser")
	database := server.GetDatabaseFromServer(srv)
	mailboxID, err := server.GetMailboxID(t, database, state.UserID, "INBOX")
	if err != nil {
		t.Fatalf("Failed to get INBOX mailbox: %v", err)
	}
	state.SelectedMailboxID = mailboxID
	state.SelectedFolder = "INBOX"

	// Insert and delete some messages
	msg1ID := server.InsertTestMail(t, database, "testuser", "Message 1", "sender@example.com", "testuser@localhost", "INBOX")
	server.InsertTestMail(t, database, "testuser", "Message 2", "sender@example.com", "testuser@localhost", "INBOX")

	userDB := server.GetUserDBByID(t, database, state.UserID)
	userDB.Exec(`UPDATE message_mailbox SET flags = '\Deleted' WHERE mailbox_id = ? AND message_id = ?`, mailboxID, msg1ID)

	// Set initial state
	state.LastMessageCount = 2

	srv.HandleExpunge(conn, "E005", state)

	// Verify state was updated
	if state.LastMessageCount != 1 {
		t.Errorf("Expected LastMessageCount to be 1, got %d", state.LastMessageCount)
	}

	// Mailbox should still be selected
	if state.SelectedMailboxID != mailboxID {
		t.Error("Mailbox should still be selected after EXPUNGE")
	}
}

// TestExpungeCommand_ResponseFormat tests the format of EXPUNGE responses
func TestExpungeCommand_ResponseFormat(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()

	state := server.SetupAuthenticatedState(t, srv, "testuser")
	database := server.GetDatabaseFromServer(srv)
	mailboxID, err := server.GetMailboxID(t, database, state.UserID, "INBOX")
	if err != nil {
		t.Fatalf("Failed to get INBOX mailbox: %v", err)
	}
	state.SelectedMailboxID = mailboxID
	state.SelectedFolder = "INBOX"

	srv.HandleExpunge(conn, "FORMAT", state)

	response := conn.GetWrittenData()

	// Check that response ends with CRLF
	if !strings.HasSuffix(response, "\r\n") {
		t.Errorf("Response should end with CRLF")
	}

	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Last line should be tagged completion
	lastLine := lines[len(lines)-1]
	if !strings.HasPrefix(lastLine, "FORMAT OK EXPUNGE completed") {
		t.Errorf("Last line should be tagged completion, got: %s", lastLine)
	}
}

// TestExpungeCommand_TagHandling tests various tag formats
func TestExpungeCommand_TagHandling(t *testing.T) {
	testCases := []struct {
		tag         string
		expectedTag string
	}{
		{"A001", "A001"},
		{"expunge1", "expunge1"},
		{"TAG-123", "TAG-123"},
		{"*", "*"},
		{"", ""},
	}

	for _, tc := range testCases {
		t.Run("Tag_"+tc.tag, func(t *testing.T) {
			srv := server.SetupTestServerSimple(t)
			conn := server.NewMockConn()

			state := server.SetupAuthenticatedState(t, srv, "testuser")
			database := server.GetDatabaseFromServer(srv)
			mailboxID, err := server.GetMailboxID(t, database, state.UserID, "INBOX")
			if err != nil {
				t.Fatalf("Failed to get INBOX mailbox: %v", err)
			}
			state.SelectedMailboxID = mailboxID
			state.SelectedFolder = "INBOX"

			srv.HandleExpunge(conn, tc.tag, state)

			response := conn.GetWrittenData()
			expectedOK := tc.expectedTag + " OK EXPUNGE completed"

			if !strings.Contains(response, expectedOK) {
				t.Errorf("Expected '%s' in response, got: %s", expectedOK, response)
			}
		})
	}
}

// TestExpungeCommand_RFC3501Compliance tests RFC 3501 compliance
func TestExpungeCommand_RFC3501Compliance(t *testing.T) {
	t.Run("Requires Selected state", func(t *testing.T) {
		srv := server.SetupTestServerSimple(t)
		conn := server.NewMockConn()

		// Authenticated but no mailbox selected
		state := &models.ClientState{
			Authenticated:     true,
			Username:          "testuser",
			SelectedMailboxID: 0,
		}

		srv.HandleExpunge(conn, "RFC1", state)

		response := conn.GetWrittenData()
		if !strings.Contains(response, "RFC1 NO") {
			t.Error("EXPUNGE must fail when no mailbox is selected per RFC 3501")
		}
	})

	t.Run("Always succeeds in Selected state", func(t *testing.T) {
		srv := server.SetupTestServerSimple(t)
		conn := server.NewMockConn()

		state := server.SetupAuthenticatedState(t, srv, "testuser")
		database := server.GetDatabaseFromServer(srv)
		mailboxID, err := server.GetMailboxID(t, database, state.UserID, "INBOX")
		if err != nil {
			t.Fatalf("Failed to get INBOX mailbox: %v", err)
		}
		state.SelectedMailboxID = mailboxID
		state.SelectedFolder = "INBOX"

		srv.HandleExpunge(conn, "RFC2", state)

		response := conn.GetWrittenData()
		if !strings.Contains(response, "RFC2 OK") {
			t.Error("EXPUNGE must always succeed in Selected state per RFC 3501")
		}
	})

	t.Run("Sends EXPUNGE responses", func(t *testing.T) {
		srv := server.SetupTestServerSimple(t)
		conn := server.NewMockConn()

		state := server.SetupAuthenticatedState(t, srv, "testuser")
		database := server.GetDatabaseFromServer(srv)
		mailboxID, err := server.GetMailboxID(t, database, state.UserID, "INBOX")
		if err != nil {
			t.Fatalf("Failed to get INBOX mailbox: %v", err)
		}
		state.SelectedMailboxID = mailboxID
		state.SelectedFolder = "INBOX"

		// Insert and delete a message
		msgID := server.InsertTestMail(t, database, "testuser", "Test", "sender@example.com", "testuser@localhost", "INBOX")

		userDB := server.GetUserDBByID(t, database, state.UserID)
		userDB.Exec(`UPDATE message_mailbox SET flags = '\Deleted' WHERE mailbox_id = ? AND message_id = ?`, mailboxID, msgID)

		srv.HandleExpunge(conn, "RFC3", state)

		response := conn.GetWrittenData()

		// Should contain EXPUNGE response (per RFC 3501)
		if !strings.Contains(response, "EXPUNGE") {
			t.Error("EXPUNGE must send EXPUNGE responses per RFC 3501")
		}

		// Should complete successfully
		if !strings.Contains(response, "RFC3 OK") {
			t.Error("EXPUNGE should complete successfully")
		}
	})

	t.Run("Stays in Selected state", func(t *testing.T) {
		srv := server.SetupTestServerSimple(t)
		conn := server.NewMockConn()

		state := server.SetupAuthenticatedState(t, srv, "testuser")
		database := server.GetDatabaseFromServer(srv)
		mailboxID, err := server.GetMailboxID(t, database, state.UserID, "INBOX")
		if err != nil {
			t.Fatalf("Failed to get INBOX mailbox: %v", err)
		}
		state.SelectedMailboxID = mailboxID
		state.SelectedFolder = "INBOX"

		srv.HandleExpunge(conn, "RFC4", state)

		// After EXPUNGE, should still be in Selected state
		if state.SelectedMailboxID == 0 || state.SelectedFolder == "" {
			t.Error("EXPUNGE should stay in Selected state (not return to authenticated)")
		}

		// Should still be authenticated
		if !state.Authenticated {
			t.Error("User should still be authenticated after EXPUNGE")
		}
	})
}

// TestExpungeCommand_VsClose tests the difference between EXPUNGE and CLOSE
func TestExpungeCommand_VsClose(t *testing.T) {
	t.Run("EXPUNGE sends responses, CLOSE does not", func(t *testing.T) {
		// Test EXPUNGE
		serverExp := server.SetupTestServerSimple(t)
		connExp := server.NewMockConn()
		stateExp := server.SetupAuthenticatedState(t, serverExp, "testuser")
		databaseExp := server.GetDatabaseFromServer(serverExp)
		mailboxIDExp, _ := server.GetMailboxID(t, databaseExp, stateExp.UserID, "INBOX")
		stateExp.SelectedMailboxID = mailboxIDExp
		stateExp.SelectedFolder = "INBOX"

		msgID := server.InsertTestMail(t, databaseExp, "testuser", "Test", "sender@example.com", "testuser@localhost", "INBOX")

		userDBExp := server.GetUserDBByID(t, databaseExp, stateExp.UserID)
		userDBExp.Exec(`UPDATE message_mailbox SET flags = '\Deleted' WHERE mailbox_id = ? AND message_id = ?`, mailboxIDExp, msgID)

		serverExp.HandleExpunge(connExp, "E001", stateExp)
		expungeResponse := connExp.GetWrittenData()

		// Test CLOSE
		serverClose := server.SetupTestServerSimple(t)
		connClose := server.NewMockConn()
		stateClose := server.SetupAuthenticatedState(t, serverClose, "testuser2")
		databaseClose := server.GetDatabaseFromServer(serverClose)
		mailboxIDClose, _ := server.GetMailboxID(t, databaseClose, stateClose.UserID, "INBOX")
		stateClose.SelectedMailboxID = mailboxIDClose
		stateClose.SelectedFolder = "INBOX"

		msgID2 := server.InsertTestMail(t, databaseClose, "testuser2", "Test", "sender@example.com", "testuser2@localhost", "INBOX")

		userDBClose := server.GetUserDBByID(t, databaseClose, stateClose.UserID)
		userDBClose.Exec(`UPDATE message_mailbox SET flags = '\Deleted' WHERE mailbox_id = ? AND message_id = ?`, mailboxIDClose, msgID2)

		serverClose.HandleClose(connClose, "C001", stateClose)
		closeResponse := connClose.GetWrittenData()

		// EXPUNGE should have EXPUNGE responses
		if !strings.Contains(expungeResponse, "EXPUNGE") {
			t.Error("EXPUNGE should send EXPUNGE responses")
		}

		// CLOSE should not have EXPUNGE responses
		if strings.Contains(closeResponse, "EXPUNGE") {
			t.Error("CLOSE should not send EXPUNGE responses")
		}
	})

	t.Run("EXPUNGE stays in Selected, CLOSE returns to authenticated", func(t *testing.T) {
		// Test EXPUNGE
		serverExp := server.SetupTestServerSimple(t)
		connExp := server.NewMockConn()
		stateExp := server.SetupAuthenticatedState(t, serverExp, "testuser")
		databaseExp := server.GetDatabaseFromServer(serverExp)
		mailboxIDExp, _ := server.GetMailboxID(t, databaseExp, stateExp.UserID, "INBOX")
		stateExp.SelectedMailboxID = mailboxIDExp
		stateExp.SelectedFolder = "INBOX"

		serverExp.HandleExpunge(connExp, "E001", stateExp)

		// EXPUNGE should stay in Selected state
		if stateExp.SelectedMailboxID == 0 {
			t.Error("EXPUNGE should keep mailbox selected")
		}

		// Test CLOSE
		serverClose := server.SetupTestServerSimple(t)
		connClose := server.NewMockConn()
		stateClose := server.SetupAuthenticatedState(t, serverClose, "testuser2")
		databaseClose := server.GetDatabaseFromServer(serverClose)
		mailboxIDClose, _ := server.GetMailboxID(t, databaseClose, stateClose.UserID, "INBOX")
		stateClose.SelectedMailboxID = mailboxIDClose
		stateClose.SelectedFolder = "INBOX"

		serverClose.HandleClose(connClose, "C001", stateClose)

		// CLOSE should return to authenticated state
		if stateClose.SelectedMailboxID != 0 {
			t.Error("CLOSE should deselect mailbox")
		}
	})
}

// TestExpungeCommand_PreservesMessageData tests that EXPUNGE only removes from mailbox, not message data
func TestExpungeCommand_PreservesMessageData(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()

	state := server.SetupAuthenticatedState(t, srv, "testuser")
	database := server.GetDatabaseFromServer(srv)
	mailboxID, err := server.GetMailboxID(t, database, state.UserID, "INBOX")
	if err != nil {
		t.Fatalf("Failed to get INBOX mailbox: %v", err)
	}
	state.SelectedMailboxID = mailboxID
	state.SelectedFolder = "INBOX"

	// Insert a test message
	messageID := server.InsertTestMail(t, database, "testuser", "Test Subject", "sender@example.com", "testuser@localhost", "INBOX")

	userDB := server.GetUserDBByID(t, database, state.UserID)

	// Mark it as deleted
	userDB.Exec(`UPDATE message_mailbox SET flags = '\Deleted' WHERE mailbox_id = ? AND message_id = ?`, mailboxID, messageID)

	srv.HandleExpunge(conn, "PRESERVE", state)

	// Message should be removed from mailbox
	var countInMailbox int
	userDB.QueryRow(`SELECT COUNT(*) FROM message_mailbox WHERE message_id = ?`, messageID).Scan(&countInMailbox)
	if countInMailbox != 0 {
		t.Errorf("Expected message to be removed from mailbox, but found %d entries", countInMailbox)
	}

	// But message data should still exist in messages table
	var countInMessages int
	userDB.QueryRow(`SELECT COUNT(*) FROM messages WHERE id = ?`, messageID).Scan(&countInMessages)
	if countInMessages != 1 {
		t.Errorf("Expected message data to be preserved in messages table, found %d entries", countInMessages)
	}
}
