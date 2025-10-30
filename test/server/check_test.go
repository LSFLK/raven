//go:build test
// +build test

package server

import (
	"strings"
	"testing"

	"go-imap/internal/models"
	"go-imap/test/helpers"
)

// TestCheckCommand_Unauthenticated tests CHECK before authentication
func TestCheckCommand_Unauthenticated(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: false,
	}

	server.HandleCheck(conn, "C001", state)

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

// TestCheckCommand_AuthenticatedNoMailbox tests CHECK when authenticated but no mailbox selected
func TestCheckCommand_AuthenticatedNoMailbox(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated:      true,
		Username:           "testuser",
		SelectedMailboxID:  0, // No mailbox selected
	}

	server.HandleCheck(conn, "C002", state)

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

// TestCheckCommand_WithSelectedMailbox tests CHECK with selected mailbox
func TestCheckCommand_WithSelectedMailbox(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()

	// Setup authenticated state with selected mailbox
	state := helpers.SetupAuthenticatedState(t, server, "testuser")

	// Set selected mailbox ID (INBOX was created by SetupAuthenticatedState)
	database := helpers.GetDatabaseFromServer(server)
	mailboxID, err := helpers.GetMailboxID(t, database, state.UserID, "INBOX")
	if err != nil {
		t.Fatalf("Failed to get INBOX mailbox: %v", err)
	}
	state.SelectedMailboxID = mailboxID
	state.SelectedFolder = "INBOX"

	server.HandleCheck(conn, "C003", state)

	response := conn.GetWrittenData()
	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Should have only completion (no untagged responses per RFC 3501)
	if len(lines) < 1 {
		t.Fatalf("Expected at least 1 response line, got %d: %v", len(lines), lines)
	}

	// Last line should be tagged OK response
	lastLine := lines[len(lines)-1]
	expectedOK := "C003 OK CHECK completed"
	if lastLine != expectedOK {
		t.Errorf("Expected '%s', got: '%s'", expectedOK, lastLine)
	}
}

// TestCheckCommand_NoExistsResponse tests that CHECK does not send EXISTS responses
// Per RFC 3501: "There is no guarantee that an EXISTS untagged response will
// happen as a result of CHECK"
func TestCheckCommand_NoExistsResponse(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()

	// Setup authenticated state with selected mailbox
	state := helpers.SetupAuthenticatedState(t, server, "testuser")
	database := helpers.GetDatabaseFromServer(server)
	mailboxID, err := helpers.GetMailboxID(t, database, state.UserID, "INBOX")
	if err != nil {
		t.Fatalf("Failed to get INBOX mailbox: %v", err)
	}
	state.SelectedMailboxID = mailboxID
	state.SelectedFolder = "INBOX"

	server.HandleCheck(conn, "C004", state)

	response := conn.GetWrittenData()

	// Should NOT contain EXISTS response
	if strings.Contains(response, "EXISTS") {
		t.Errorf("CHECK should not send EXISTS responses, got: %s", response)
	}

	// Should NOT contain RECENT response
	if strings.Contains(response, "RECENT") {
		t.Errorf("CHECK should not send RECENT responses, got: %s", response)
	}

	// Should only contain OK completion
	if !strings.Contains(response, "C004 OK CHECK completed") {
		t.Errorf("Expected OK completion, got: %s", response)
	}
}

// TestCheckCommand_AlwaysSucceeds tests that CHECK always returns OK when mailbox is selected
func TestCheckCommand_AlwaysSucceeds(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()

	// Setup authenticated state with selected mailbox
	state := helpers.SetupAuthenticatedState(t, server, "testuser")
	database := helpers.GetDatabaseFromServer(server)
	mailboxID, err := helpers.GetMailboxID(t, database, state.UserID, "INBOX")
	if err != nil {
		t.Fatalf("Failed to get INBOX mailbox: %v", err)
	}
	state.SelectedMailboxID = mailboxID
	state.SelectedFolder = "INBOX"

	server.HandleCheck(conn, "T001", state)

	response := conn.GetWrittenData()

	// Should always end with OK
	if !strings.Contains(response, " OK CHECK completed") {
		t.Errorf("CHECK should always succeed when mailbox is selected, got: %s", response)
	}

	// Should contain the correct tag
	expectedOK := "T001 OK CHECK completed"
	if !strings.Contains(response, expectedOK) {
		t.Errorf("Expected to find '%s' in response, got: %s", expectedOK, response)
	}
}

// TestCheckCommand_ResponseFormat tests the format of CHECK responses
func TestCheckCommand_ResponseFormat(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()

	state := helpers.SetupAuthenticatedState(t, server, "testuser")
	database := helpers.GetDatabaseFromServer(server)
	mailboxID, err := helpers.GetMailboxID(t, database, state.UserID, "INBOX")
	if err != nil {
		t.Fatalf("Failed to get INBOX mailbox: %v", err)
	}
	state.SelectedMailboxID = mailboxID
	state.SelectedFolder = "INBOX"

	server.HandleCheck(conn, "FORMAT", state)

	response := conn.GetWrittenData()

	// Check that response ends with CRLF
	if !strings.HasSuffix(response, "\r\n") {
		t.Errorf("Response should end with CRLF")
	}

	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Last line should be tagged completion
	lastLine := lines[len(lines)-1]
	if !strings.HasPrefix(lastLine, "FORMAT OK CHECK completed") {
		t.Errorf("Last line should be tagged completion, got: %s", lastLine)
	}
}

// TestCheckCommand_MultipleInvocations tests calling CHECK multiple times
func TestCheckCommand_MultipleInvocations(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()

	state := helpers.SetupAuthenticatedState(t, server, "testuser")
	database := helpers.GetDatabaseFromServer(server)
	mailboxID, err := helpers.GetMailboxID(t, database, state.UserID, "INBOX")
	if err != nil {
		t.Fatalf("Failed to get INBOX mailbox: %v", err)
	}
	state.SelectedMailboxID = mailboxID
	state.SelectedFolder = "INBOX"

	// Call CHECK multiple times with different tags
	server.HandleCheck(conn, "M001", state)
	server.HandleCheck(conn, "M002", state)
	server.HandleCheck(conn, "M003", state)

	response := conn.GetWrittenData()

	// Should have all three completions
	if !strings.Contains(response, "M001 OK CHECK completed") {
		t.Error("Missing M001 completion")
	}
	if !strings.Contains(response, "M002 OK CHECK completed") {
		t.Error("Missing M002 completion")
	}
	if !strings.Contains(response, "M003 OK CHECK completed") {
		t.Error("Missing M003 completion")
	}
}

// TestCheckCommand_StateTracking tests that CHECK updates state correctly
func TestCheckCommand_StateTracking(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()

	// Setup authenticated state with selected mailbox
	state := helpers.SetupAuthenticatedState(t, server, "testuser")
	database := helpers.GetDatabaseFromServer(server)
	mailboxID, err := helpers.GetMailboxID(t, database, state.UserID, "INBOX")
	if err != nil {
		t.Fatalf("Failed to get INBOX mailbox: %v", err)
	}
	state.SelectedMailboxID = mailboxID
	state.SelectedFolder = "INBOX"

	// Store initial state
	initialCount := state.LastMessageCount
	initialRecent := state.LastRecentCount

	server.HandleCheck(conn, "TRACK", state)

	// After CHECK, state should be updated with current mailbox state
	// The actual values depend on database state, but state should be synchronized

	response := conn.GetWrittenData()
	if !strings.Contains(response, "TRACK OK CHECK completed") {
		t.Errorf("Expected CHECK completion, got: %s", response)
	}

	// State should be updated (values may be same or different depending on DB)
	// We just verify the command succeeded
	t.Logf("Initial: count=%d, recent=%d; After: count=%d, recent=%d",
		initialCount, initialRecent, state.LastMessageCount, state.LastRecentCount)
}

// TestCheckCommand_TagHandling tests various tag formats
func TestCheckCommand_TagHandling(t *testing.T) {
	testCases := []struct {
		tag         string
		expectedTag string
	}{
		{"A001", "A001"},
		{"check1", "check1"},
		{"TAG-123", "TAG-123"},
		{"*", "*"},
		{"", ""},
		{"VERY-LONG-TAG-NAME-FOR-CHECK", "VERY-LONG-TAG-NAME-FOR-CHECK"},
	}

	for _, tc := range testCases {
		t.Run("Tag_"+tc.tag, func(t *testing.T) {
			server := helpers.SetupTestServerSimple(t)
			conn := helpers.NewMockConn()

			state := helpers.SetupAuthenticatedState(t, server, "testuser")
			database := helpers.GetDatabaseFromServer(server)
	mailboxID, err := helpers.GetMailboxID(t, database, state.UserID, "INBOX")
	if err != nil {
		t.Fatalf("Failed to get INBOX mailbox: %v", err)
	}
	state.SelectedMailboxID = mailboxID
	state.SelectedFolder = "INBOX"

			server.HandleCheck(conn, tc.tag, state)

			response := conn.GetWrittenData()
			expectedOK := tc.expectedTag + " OK CHECK completed"

			if !strings.Contains(response, expectedOK) {
				t.Errorf("Expected '%s' in response, got: %s", expectedOK, response)
			}
		})
	}
}

// TestCheckCommand_ConcurrentAccess tests concurrent CHECK requests
func TestCheckCommand_ConcurrentAccess(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)

	const numRequests = 20
	done := make(chan bool, numRequests)

	// Launch concurrent CHECK requests
	for i := 0; i < numRequests; i++ {
		go func(index int) {
			conn := helpers.NewMockConn()
			state := helpers.SetupAuthenticatedState(t, server, "user")
			database := helpers.GetDatabaseFromServer(server)
	mailboxID, err := helpers.GetMailboxID(t, database, state.UserID, "INBOX")
	if err != nil {
		t.Fatalf("Failed to get INBOX mailbox: %v", err)
	}
	state.SelectedMailboxID = mailboxID
	state.SelectedFolder = "INBOX"

			server.HandleCheck(conn, "CONCURRENT", state)

			response := conn.GetWrittenData()
			if !strings.Contains(response, "CONCURRENT OK CHECK completed") {
				t.Errorf("Request %d failed: %s", index, response)
			}
			done <- true
		}(i)
	}

	// Wait for all requests to complete
	for i := 0; i < numRequests; i++ {
		<-done
	}
}

// TestCheckCommand_RFC3501Compliance tests RFC 3501 compliance
func TestCheckCommand_RFC3501Compliance(t *testing.T) {
	t.Run("Requires Selected state", func(t *testing.T) {
		server := helpers.SetupTestServerSimple(t)
		conn := helpers.NewMockConn()

		// Authenticated but no mailbox selected
		state := &models.ClientState{
			Authenticated: true,
			Username:      "testuser",
			SelectedMailboxID: 0,
		}

		server.HandleCheck(conn, "RFC1", state)

		response := conn.GetWrittenData()
		if !strings.Contains(response, "RFC1 NO") {
			t.Error("CHECK must fail when no mailbox is selected per RFC 3501")
		}
	})

	t.Run("Always succeeds in Selected state", func(t *testing.T) {
		server := helpers.SetupTestServerSimple(t)
		conn := helpers.NewMockConn()

		state := helpers.SetupAuthenticatedState(t, server, "testuser")
		database := helpers.GetDatabaseFromServer(server)
	mailboxID, err := helpers.GetMailboxID(t, database, state.UserID, "INBOX")
	if err != nil {
		t.Fatalf("Failed to get INBOX mailbox: %v", err)
	}
	state.SelectedMailboxID = mailboxID
	state.SelectedFolder = "INBOX"

		server.HandleCheck(conn, "RFC2", state)

		response := conn.GetWrittenData()
		if !strings.Contains(response, "RFC2 OK") {
			t.Error("CHECK must always succeed in Selected state per RFC 3501")
		}
	})

	t.Run("No guaranteed EXISTS response", func(t *testing.T) {
		server := helpers.SetupTestServerSimple(t)
		conn := helpers.NewMockConn()

		state := helpers.SetupAuthenticatedState(t, server, "testuser")
		database := helpers.GetDatabaseFromServer(server)
	mailboxID, err := helpers.GetMailboxID(t, database, state.UserID, "INBOX")
	if err != nil {
		t.Fatalf("Failed to get INBOX mailbox: %v", err)
	}
	state.SelectedMailboxID = mailboxID
	state.SelectedFolder = "INBOX"

		// Multiple CHECK calls should not generate EXISTS responses
		for i := 0; i < 3; i++ {
			server.HandleCheck(conn, "RFC3", state)
		}

		response := conn.GetWrittenData()
		// Should complete successfully but no EXISTS responses
		count := strings.Count(response, "RFC3 OK CHECK completed")
		if count != 3 {
			t.Errorf("Expected 3 completions, got %d", count)
		}

		// Should not contain EXISTS (per RFC 3501)
		if strings.Contains(response, "EXISTS") {
			t.Error("CHECK should not guarantee EXISTS responses per RFC 3501")
		}
	})

	t.Run("Housekeeping operation", func(t *testing.T) {
		// This test verifies that CHECK performs housekeeping
		// by synchronizing in-memory state with database state
		server := helpers.SetupTestServerSimple(t)
		conn := helpers.NewMockConn()

		state := helpers.SetupAuthenticatedState(t, server, "testuser")
		database := helpers.GetDatabaseFromServer(server)
	mailboxID, err := helpers.GetMailboxID(t, database, state.UserID, "INBOX")
	if err != nil {
		t.Fatalf("Failed to get INBOX mailbox: %v", err)
	}
	state.SelectedMailboxID = mailboxID
	state.SelectedFolder = "INBOX"

		// Manually set state to inconsistent values
		state.LastMessageCount = 999
		state.LastRecentCount = 999

		server.HandleCheck(conn, "RFC4", state)

		response := conn.GetWrittenData()
		if !strings.Contains(response, "RFC4 OK") {
			t.Error("CHECK should complete successfully")
		}

		// After CHECK, state should be synchronized with database
		// (actual values depend on database, but they should be updated)
		if state.LastMessageCount == 999 && state.LastRecentCount == 999 {
			t.Error("CHECK should update state during housekeeping")
		}
	})
}

// TestCheckCommand_VsNoop tests the difference between CHECK and NOOP
func TestCheckCommand_VsNoop(t *testing.T) {
	t.Run("CHECK requires Selected state, NOOP does not", func(t *testing.T) {
		server := helpers.SetupTestServerSimple(t)

		// Test NOOP without selected mailbox
		connNoop := helpers.NewMockConn()
		stateNoop := &models.ClientState{
			Authenticated: true,
			Username:      "testuser",
		}
		server.HandleNoop(connNoop, "N001", stateNoop)
		noopResponse := connNoop.GetWrittenData()

		// Test CHECK without selected mailbox
		connCheck := helpers.NewMockConn()
		stateCheck := &models.ClientState{
			Authenticated: true,
			Username:      "testuser",
		}
		server.HandleCheck(connCheck, "C001", stateCheck)
		checkResponse := connCheck.GetWrittenData()

		// NOOP should succeed
		if !strings.Contains(noopResponse, "N001 OK") {
			t.Error("NOOP should succeed without selected mailbox")
		}

		// CHECK should fail
		if !strings.Contains(checkResponse, "C001 NO") {
			t.Error("CHECK should fail without selected mailbox")
		}
	})

	t.Run("NOOP used for polling, CHECK for housekeeping", func(t *testing.T) {
		// This is documented in RFC 3501 but behavioral difference is subtle
		// NOOP can send EXISTS, CHECK does not guarantee it
		server := helpers.SetupTestServerSimple(t)

		state := helpers.SetupAuthenticatedState(t, server, "testuser")
		database := helpers.GetDatabaseFromServer(server)
	mailboxID, err := helpers.GetMailboxID(t, database, state.UserID, "INBOX")
	if err != nil {
		t.Fatalf("Failed to get INBOX mailbox: %v", err)
	}
	state.SelectedMailboxID = mailboxID
	state.SelectedFolder = "INBOX"

		// CHECK should not send untagged responses
		connCheck := helpers.NewMockConn()
		server.HandleCheck(connCheck, "C001", state)
		checkResponse := connCheck.GetWrittenData()

		checkLines := strings.Split(strings.TrimSpace(checkResponse), "\r\n")
		// CHECK should typically have just 1 line (the completion)
		// while NOOP might have more (EXISTS, RECENT, etc.)

		if len(checkLines) > 1 {
			for _, line := range checkLines[:len(checkLines)-1] {
				if strings.HasPrefix(line, "*") {
					t.Logf("Warning: CHECK sent untagged response: %s", line)
				}
			}
		}
	})
}
