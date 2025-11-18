//go:build test
// +build test

package selection

import (
	"strings"
	"testing"

	"raven/internal/models"

)

// TestCloseCommand_Unauthenticated tests CLOSE before authentication
func TestCloseCommand_Unauthenticated(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	state := &models.ClientState{
		Authenticated: false,
	}

	srv.HandleClose(conn, "C001", state)

	response := conn.GetWrittenData()
	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Should have 1 line: NO response
	if len(lines) != 1 {
		t.Fatalf("Expected 1 response line, got %d: %v", len(lines), lines)
	}

	// Check NO response for unauthenticated
	expectedNO := "C001 NO Please authenticate first"
	if lines[0] != expectedNO {
		t.Errorf("Expected '%s', got: '%s'", expectedNO, lines[0])
	}
}

// TestCloseCommand_AuthenticatedNoMailbox tests CLOSE when authenticated but no mailbox selected
func TestCloseCommand_AuthenticatedNoMailbox(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	state := &models.ClientState{
		Authenticated:     true,
		Username:          "testuser",
		SelectedMailboxID: 0, // No mailbox selected
	}

	srv.HandleClose(conn, "C002", state)

	response := conn.GetWrittenData()
	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Should have 1 line: NO response
	if len(lines) != 1 {
		t.Fatalf("Expected 1 response line, got %d: %v", len(lines), lines)
	}

	// Check NO response for no selected mailbox
	expectedNO := "C002 NO No mailbox selected"
	if lines[0] != expectedNO {
		t.Errorf("Expected '%s', got: '%s'", expectedNO, lines[0])
	}
}

// TestCloseCommand_WithSelectedMailbox tests CLOSE with selected mailbox (no deleted messages)
func TestCloseCommand_WithSelectedMailbox(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()

	// Setup authenticated state with selected mailbox
	state := SetupAuthenticatedState(t, srv, "testuser")
	database := GetDatabaseFromServer(srv)
	mailboxID, err := GetMailboxID(t, database, state.UserID, "INBOX")
	if err != nil {
		t.Fatalf("Failed to get INBOX mailbox: %v", err)
	}
	state.SelectedMailboxID = mailboxID
	state.SelectedFolder = "INBOX"
	_ = GetUserDBByID(t, database, state.UserID)

	srv.HandleClose(conn, "C003", state)

	response := conn.GetWrittenData()
	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Should have only completion (no untagged EXPUNGE responses per RFC 3501)
	if len(lines) != 1 {
		t.Fatalf("Expected 1 response line, got %d: %v", len(lines), lines)
	}

	// Should be tagged OK response
	expectedOK := "C003 OK CLOSE completed"
	if lines[0] != expectedOK {
		t.Errorf("Expected '%s', got: '%s'", expectedOK, lines[0])
	}

	// Verify state was reset to authenticated state
	if state.SelectedMailboxID != 0 {
		t.Error("SelectedMailboxID should be 0 after CLOSE")
	}
	if state.SelectedFolder != "" {
		t.Error("SelectedFolder should be empty after CLOSE")
	}
}

// TestCloseCommand_NoExpungeResponses tests that CLOSE does not send EXPUNGE responses
// Per RFC 3501: No untagged EXPUNGE responses are sent
func TestCloseCommand_NoExpungeResponses(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()

	// Setup authenticated state with selected mailbox
	state := SetupAuthenticatedState(t, srv, "testuser")
	database := GetDatabaseFromServer(srv)
	mailboxID, err := GetMailboxID(t, database, state.UserID, "INBOX")
	if err != nil {
		t.Fatalf("Failed to get INBOX mailbox: %v", err)
	}
	state.SelectedMailboxID = mailboxID
	state.SelectedFolder = "INBOX"
	userDB := GetUserDBByID(t, database, state.UserID)


	// Insert a test message with \Deleted flag
	InsertTestMail(t, database, "testuser", "Test Subject", "sender@example.com", "testuser@localhost", "INBOX")

	// Add \Deleted flag to the message
	userDB.Exec(`UPDATE message_mailbox SET flags = '\Deleted' WHERE mailbox_id = ?`, mailboxID)

	srv.HandleClose(conn, "C004", state)

	response := conn.GetWrittenData()

	// Should NOT contain EXPUNGE response
	if strings.Contains(response, "EXPUNGE") {
		t.Errorf("CLOSE should not send EXPUNGE responses, got: %s", response)
	}

	// Should only contain OK completion
	if !strings.Contains(response, "C004 OK CLOSE completed") {
		t.Errorf("Expected OK completion, got: %s", response)
	}
}

// TestCloseCommand_DeletesMessagesWithDeletedFlag tests that CLOSE deletes messages with \Deleted flag
func TestCloseCommand_DeletesMessagesWithDeletedFlag(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()

	// Setup authenticated state with selected mailbox
	state := SetupAuthenticatedState(t, srv, "testuser")
	database := GetDatabaseFromServer(srv)
	mailboxID, err := GetMailboxID(t, database, state.UserID, "INBOX")
	if err != nil {
		t.Fatalf("Failed to get INBOX mailbox: %v", err)
	}
	state.SelectedMailboxID = mailboxID
	state.SelectedFolder = "INBOX"
	userDB := GetUserDBByID(t, database, state.UserID)


	// Insert test messages
	msg1ID := InsertTestMail(t, database, "testuser", "Message 1", "sender@example.com", "testuser@localhost", "INBOX")
	msg2ID := InsertTestMail(t, database, "testuser", "Message 2", "sender@example.com", "testuser@localhost", "INBOX")
	InsertTestMail(t, database, "testuser", "Message 3", "sender@example.com", "testuser@localhost", "INBOX")

	// Mark first two messages as deleted
	userDB.Exec(`UPDATE message_mailbox SET flags = '\Deleted' WHERE mailbox_id = ? AND message_id = ?`, mailboxID, msg1ID)
	userDB.Exec(`UPDATE message_mailbox SET flags = '\Deleted' WHERE mailbox_id = ? AND message_id = ?`, mailboxID, msg2ID)

	// Count messages before CLOSE
	var countBefore int
	userDB.QueryRow(`SELECT COUNT(*) FROM message_mailbox WHERE mailbox_id = ?`, mailboxID).Scan(&countBefore)
	if countBefore != 3 {
		t.Fatalf("Expected 3 messages before CLOSE, got %d", countBefore)
	}

	// Count deleted messages
	var deletedCount int
	userDB.QueryRow(`SELECT COUNT(*) FROM message_mailbox WHERE mailbox_id = ? AND flags LIKE '%\Deleted%'`, mailboxID).Scan(&deletedCount)
	if deletedCount != 2 {
		t.Fatalf("Expected 2 deleted messages, got %d", deletedCount)
	}

	srv.HandleClose(conn, "C005", state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "C005 OK CLOSE completed") {
		t.Errorf("Expected OK completion, got: %s", response)
	}

	// Count messages after CLOSE - should have only 1 left (the non-deleted one)
	var countAfter int
	userDB.QueryRow(`SELECT COUNT(*) FROM message_mailbox WHERE mailbox_id = ?`, mailboxID).Scan(&countAfter)
	if countAfter != 1 {
		t.Errorf("Expected 1 message after CLOSE (deleted messages removed), got %d", countAfter)
	}

	// Verify no messages with \Deleted flag remain
	var deletedAfter int
	userDB.QueryRow(`SELECT COUNT(*) FROM message_mailbox WHERE mailbox_id = ? AND flags LIKE '%\Deleted%'`, mailboxID).Scan(&deletedAfter)
	if deletedAfter != 0 {
		t.Errorf("Expected 0 deleted messages after CLOSE, got %d", deletedAfter)
	}
}

// TestCloseCommand_StateReset tests that CLOSE resets client state to authenticated
func TestCloseCommand_StateReset(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()

	// Setup authenticated state with selected mailbox
	state := SetupAuthenticatedState(t, srv, "testuser")
	database := GetDatabaseFromServer(srv)
	mailboxID, err := GetMailboxID(t, database, state.UserID, "INBOX")
	if err != nil {
		t.Fatalf("Failed to get INBOX mailbox: %v", err)
	}
	state.SelectedMailboxID = mailboxID
	state.SelectedFolder = "INBOX"
	_ = GetUserDBByID(t, database, state.UserID)
	state.LastMessageCount = 10
	state.LastRecentCount = 5
	state.UIDValidity = 12345
	state.UIDNext = 100

	srv.HandleClose(conn, "C006", state)

	// Verify all state fields are reset
	if state.SelectedMailboxID != 0 {
		t.Errorf("Expected SelectedMailboxID to be 0, got %d", state.SelectedMailboxID)
	}
	if state.SelectedFolder != "" {
		t.Errorf("Expected SelectedFolder to be empty, got %s", state.SelectedFolder)
	}
	if state.LastMessageCount != 0 {
		t.Errorf("Expected LastMessageCount to be 0, got %d", state.LastMessageCount)
	}
	if state.LastRecentCount != 0 {
		t.Errorf("Expected LastRecentCount to be 0, got %d", state.LastRecentCount)
	}
	if state.UIDValidity != 0 {
		t.Errorf("Expected UIDValidity to be 0, got %d", state.UIDValidity)
	}
	if state.UIDNext != 0 {
		t.Errorf("Expected UIDNext to be 0, got %d", state.UIDNext)
	}

	// User should still be authenticated
	if !state.Authenticated {
		t.Error("User should still be authenticated after CLOSE")
	}
}

// TestCloseCommand_ResponseFormat tests the format of CLOSE responses
func TestCloseCommand_ResponseFormat(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()

	state := SetupAuthenticatedState(t, srv, "testuser")
	database := GetDatabaseFromServer(srv)
	mailboxID, err := GetMailboxID(t, database, state.UserID, "INBOX")
	if err != nil {
		t.Fatalf("Failed to get INBOX mailbox: %v", err)
	}
	state.SelectedMailboxID = mailboxID
	state.SelectedFolder = "INBOX"
	_ = GetUserDBByID(t, database, state.UserID)

	srv.HandleClose(conn, "FORMAT", state)

	response := conn.GetWrittenData()

	// Check that response ends with CRLF
	if !strings.HasSuffix(response, "\r\n") {
		t.Errorf("Response should end with CRLF")
	}

	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Should have only 1 line (tagged completion)
	if len(lines) != 1 {
		t.Errorf("Expected 1 line, got %d: %v", len(lines), lines)
	}

	// Line should be tagged completion
	if !strings.HasPrefix(lines[0], "FORMAT OK CLOSE completed") {
		t.Errorf("Line should be tagged completion, got: %s", lines[0])
	}
}

// TestCloseCommand_TagHandling tests various tag formats
func TestCloseCommand_TagHandling(t *testing.T) {
	testCases := []struct {
		tag         string
		expectedTag string
	}{
		{"A001", "A001"},
		{"close1", "close1"},
		{"TAG-123", "TAG-123"},
		{"*", "*"},
		{"", ""},
		{"VERY-LONG-TAG-NAME-FOR-CLOSE", "VERY-LONG-TAG-NAME-FOR-CLOSE"},
	}

	for _, tc := range testCases {
		t.Run("Tag_"+tc.tag, func(t *testing.T) {
			srv := SetupTestServerSimple(t)
			conn := NewMockConn()

			state := SetupAuthenticatedState(t, srv, "testuser")
			database := GetDatabaseFromServer(srv)
			mailboxID, err := GetMailboxID(t, database, state.UserID, "INBOX")
			if err != nil {
				t.Fatalf("Failed to get INBOX mailbox: %v", err)
			}
			state.SelectedMailboxID = mailboxID
			state.SelectedFolder = "INBOX"
	_ = GetUserDBByID(t, database, state.UserID)

			srv.HandleClose(conn, tc.tag, state)

			response := conn.GetWrittenData()
			expectedOK := tc.expectedTag + " OK CLOSE completed"

			if !strings.Contains(response, expectedOK) {
				t.Errorf("Expected '%s' in response, got: %s", expectedOK, response)
			}
		})
	}
}

// TestCloseCommand_RFC3501Compliance tests RFC 3501 compliance
func TestCloseCommand_RFC3501Compliance(t *testing.T) {
	t.Run("Requires Selected state", func(t *testing.T) {
		srv := SetupTestServerSimple(t)
		conn := NewMockConn()

		// Authenticated but no mailbox selected
		state := &models.ClientState{
			Authenticated:     true,
			Username:          "testuser",
			SelectedMailboxID: 0,
		}

		srv.HandleClose(conn, "RFC1", state)

		response := conn.GetWrittenData()
		if !strings.Contains(response, "RFC1 NO") {
			t.Error("CLOSE must fail when no mailbox is selected per RFC 3501")
		}
	})

	t.Run("Always succeeds in Selected state", func(t *testing.T) {
		srv := SetupTestServerSimple(t)
		conn := NewMockConn()

		state := SetupAuthenticatedState(t, srv, "testuser")
		database := GetDatabaseFromServer(srv)
		mailboxID, err := GetMailboxID(t, database, state.UserID, "INBOX")
		if err != nil {
			t.Fatalf("Failed to get INBOX mailbox: %v", err)
		}
		state.SelectedMailboxID = mailboxID
		state.SelectedFolder = "INBOX"

		srv.HandleClose(conn, "RFC2", state)

		response := conn.GetWrittenData()
		if !strings.Contains(response, "RFC2 OK") {
			t.Error("CLOSE must always succeed in Selected state per RFC 3501")
		}
	})

	t.Run("No EXPUNGE responses", func(t *testing.T) {
		srv := SetupTestServerSimple(t)
		conn := NewMockConn()

		state := SetupAuthenticatedState(t, srv, "testuser")
		database := GetDatabaseFromServer(srv)
		mailboxID, err := GetMailboxID(t, database, state.UserID, "INBOX")
		if err != nil {
			t.Fatalf("Failed to get INBOX mailbox: %v", err)
		}
		state.SelectedMailboxID = mailboxID
		state.SelectedFolder = "INBOX"
		userDB := GetUserDBByID(t, database, state.UserID)

		// Insert and delete messages
		InsertTestMail(t, database, "testuser", "Test", "sender@example.com", "testuser@localhost", "INBOX")
		userDB.Exec(`UPDATE message_mailbox SET flags = '\Deleted' WHERE mailbox_id = ?`, mailboxID)

		srv.HandleClose(conn, "RFC3", state)

		response := conn.GetWrittenData()

		// Should not contain EXPUNGE (per RFC 3501)
		if strings.Contains(response, "EXPUNGE") {
			t.Error("CLOSE must not send EXPUNGE responses per RFC 3501")
		}

		// Should complete successfully
		if !strings.Contains(response, "RFC3 OK") {
			t.Error("CLOSE should complete successfully")
		}
	})

	t.Run("Returns to authenticated state", func(t *testing.T) {
		srv := SetupTestServerSimple(t)
		conn := NewMockConn()

		state := SetupAuthenticatedState(t, srv, "testuser")
		database := GetDatabaseFromServer(srv)
		mailboxID, err := GetMailboxID(t, database, state.UserID, "INBOX")
		if err != nil {
			t.Fatalf("Failed to get INBOX mailbox: %v", err)
		}
		state.SelectedMailboxID = mailboxID
		state.SelectedFolder = "INBOX"

		srv.HandleClose(conn, "RFC4", state)

		// After CLOSE, should be in authenticated state (no mailbox selected)
		if state.SelectedMailboxID != 0 || state.SelectedFolder != "" {
			t.Error("CLOSE should return to authenticated state (no mailbox selected)")
		}

		// But should still be authenticated
		if !state.Authenticated {
			t.Error("User should still be authenticated after CLOSE")
		}
	})
}

// TestCloseCommand_MultipleInvocations tests calling CLOSE after it already closed
func TestCloseCommand_MultipleInvocations(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()

	state := SetupAuthenticatedState(t, srv, "testuser")
	database := GetDatabaseFromServer(srv)
	mailboxID, err := GetMailboxID(t, database, state.UserID, "INBOX")
	if err != nil {
		t.Fatalf("Failed to get INBOX mailbox: %v", err)
	}
	state.SelectedMailboxID = mailboxID
	state.SelectedFolder = "INBOX"
	_ = GetUserDBByID(t, database, state.UserID)

	// First CLOSE should succeed
	srv.HandleClose(conn, "M001", state)
	response1 := conn.GetWrittenData()
	if !strings.Contains(response1, "M001 OK CLOSE completed") {
		t.Error("First CLOSE should succeed")
	}

	// Second CLOSE should fail (no mailbox selected)
	conn.ClearWriteBuffer()
	srv.HandleClose(conn, "M002", state)
	response2 := conn.GetWrittenData()
	if !strings.Contains(response2, "M002 NO No mailbox selected") {
		t.Error("Second CLOSE should fail with NO response")
	}
}

// TestCloseCommand_PreservesMessageData tests that CLOSE only removes from mailbox, not message data
func TestCloseCommand_PreservesMessageData(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()

	state := SetupAuthenticatedState(t, srv, "testuser")
	database := GetDatabaseFromServer(srv)
	mailboxID, err := GetMailboxID(t, database, state.UserID, "INBOX")
	if err != nil {
		t.Fatalf("Failed to get INBOX mailbox: %v", err)
	}
	state.SelectedMailboxID = mailboxID
	state.SelectedFolder = "INBOX"
	userDB := GetUserDBByID(t, database, state.UserID)

	// Insert a test message
	messageID := InsertTestMail(t, database, "testuser", "Test Subject", "sender@example.com", "testuser@localhost", "INBOX")

	// Mark it as deleted
	userDB.Exec(`UPDATE message_mailbox SET flags = '\Deleted' WHERE mailbox_id = ? AND message_id = ?`, mailboxID, messageID)

	srv.HandleClose(conn, "PRESERVE", state)

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
