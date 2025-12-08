package mailbox_test

import (
	"fmt"
	"strings"
	"testing"

	"raven/internal/models"
	"raven/internal/server"
)

// TestStatusCommand_Authentication tests STATUS command authentication requirements
func TestStatusCommand_Authentication(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := &models.ClientState{
		Authenticated: false,
	}

	// Test STATUS command without authentication
	srv.HandleStatus(conn, "A001", []string{"A001", "STATUS", "INBOX", "(MESSAGES)"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A001 NO Please authenticate first") {
		t.Errorf("Expected authentication error, got: %s", response)
	}
}

// TestStatusCommand_InvalidArguments tests STATUS command with invalid arguments
func TestStatusCommand_InvalidArguments(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Test STATUS command with insufficient arguments (no mailbox)
	srv.HandleStatus(conn, "A001", []string{"A001", "STATUS"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A001 BAD STATUS requires mailbox name and status data items") {
		t.Errorf("Expected BAD response for insufficient arguments, got: %s", response)
	}
}

// TestStatusCommand_NoStatusItems tests STATUS command without status data items
func TestStatusCommand_NoStatusItems(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Test STATUS command without status items
	srv.HandleStatus(conn, "A001", []string{"A001", "STATUS", "INBOX"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A001 BAD STATUS requires mailbox name and status data items") {
		t.Errorf("Expected BAD response for missing status items, got: %s", response)
	}
}

// TestStatusCommand_EmptyMailboxName tests STATUS command with empty mailbox name
func TestStatusCommand_EmptyMailboxName(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Test STATUS command with empty mailbox name
	srv.HandleStatus(conn, "A001", []string{"A001", "STATUS", `""`, "(MESSAGES)"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A001 BAD Invalid mailbox name") {
		t.Errorf("Expected BAD response for empty mailbox name, got: %s", response)
	}
}

// TestStatusCommand_NonExistentMailbox tests STATUS command with non-existent mailbox
func TestStatusCommand_NonExistentMailbox(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Ensure user table exists
	database := server.GetDatabaseFromServer(srv)
	server.CreateTestUser(t, database, "testuser")

	// Test STATUS command with non-existent mailbox
	srv.HandleStatus(conn, "A001", []string{"A001", "STATUS", "NonExistent", "(MESSAGES)"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A001 NO STATUS failure: no status for that name") {
		t.Errorf("Expected NO response for non-existent mailbox, got: %s", response)
	}
}

// TestStatusCommand_MessagesItem tests STATUS command with MESSAGES item
func TestStatusCommand_MessagesItem(t *testing.T) {
	// Create test database and user
	database := server.CreateTestDB(t)
	defer func() {
		if err := database.Close(); err != nil {
			t.Logf("Failed to close database during cleanup: %v", err)
		}
	}()
	server.CreateTestUser(t, database, "testuser")

	// Create server with the database
	srv := server.TestServerWithDBManager(database)

	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Test STATUS command with MESSAGES item on INBOX
	srv.HandleStatus(conn, "A042", []string{"A042", "STATUS", "INBOX", "(MESSAGES)"}, state)

	response := conn.GetWrittenData()
	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Should have 2 lines: STATUS response and completion
	if len(lines) != 2 {
		t.Fatalf("Expected 2 response lines, got %d: %v", len(lines), lines)
	}

	// Check untagged STATUS response
	statusLine := lines[0]
	if !strings.HasPrefix(statusLine, `* STATUS "INBOX" (MESSAGES`) {
		t.Errorf("Expected STATUS response with MESSAGES, got: %s", statusLine)
	}

	// Verify MESSAGES value is present
	if !strings.Contains(statusLine, "MESSAGES 0") {
		t.Errorf("Expected MESSAGES 0 in response, got: %s", statusLine)
	}

	// Check completion response
	if !strings.Contains(lines[1], "A042 OK STATUS completed") {
		t.Errorf("Expected OK completion, got: %s", lines[1])
	}
}

// TestStatusCommand_MultipleItems tests STATUS command with multiple status items
func TestStatusCommand_MultipleItems(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Ensure user table exists
	database := server.GetDatabaseFromServer(srv)
	server.CreateTestUser(t, database, "testuser")

	// Test STATUS command with multiple items (as per RFC 3501 example)
	srv.HandleStatus(conn, "A042", []string{"A042", "STATUS", "INBOX", "(UIDNEXT", "MESSAGES)"}, state)

	response := conn.GetWrittenData()
	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Should have 2 lines: STATUS response and completion
	if len(lines) != 2 {
		t.Fatalf("Expected 2 response lines, got %d: %v", len(lines), lines)
	}

	// Check untagged STATUS response
	statusLine := lines[0]
	if !strings.HasPrefix(statusLine, `* STATUS "INBOX"`) {
		t.Errorf("Expected STATUS response, got: %s", statusLine)
	}

	// Verify both UIDNEXT and MESSAGES are present
	if !strings.Contains(statusLine, "UIDNEXT") {
		t.Errorf("Expected UIDNEXT in response, got: %s", statusLine)
	}
	if !strings.Contains(statusLine, "MESSAGES") {
		t.Errorf("Expected MESSAGES in response, got: %s", statusLine)
	}

	// Check completion response
	if !strings.Contains(lines[1], "A042 OK STATUS completed") {
		t.Errorf("Expected OK completion, got: %s", lines[1])
	}
}

// TestStatusCommand_AllItems tests STATUS command with all defined status items
func TestStatusCommand_AllItems(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Ensure user table exists
	database := server.GetDatabaseFromServer(srv)
	server.CreateTestUser(t, database, "testuser")

	// Test STATUS command with all status items
	srv.HandleStatus(conn, "A001", []string{"A001", "STATUS", "INBOX", "(MESSAGES", "RECENT", "UIDNEXT", "UIDVALIDITY", "UNSEEN)"}, state)

	response := conn.GetWrittenData()
	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Should have 2 lines: STATUS response and completion
	if len(lines) != 2 {
		t.Fatalf("Expected 2 response lines, got %d: %v", len(lines), lines)
	}

	// Check untagged STATUS response
	statusLine := lines[0]
	if !strings.HasPrefix(statusLine, `* STATUS "INBOX"`) {
		t.Errorf("Expected STATUS response, got: %s", statusLine)
	}

	// Verify all items are present
	requiredItems := []string{"MESSAGES", "RECENT", "UIDNEXT", "UIDVALIDITY", "UNSEEN"}
	for _, item := range requiredItems {
		if !strings.Contains(statusLine, item) {
			t.Errorf("Expected %s in response, got: %s", item, statusLine)
		}
	}

	// Check completion response
	if !strings.Contains(lines[1], "A001 OK STATUS completed") {
		t.Errorf("Expected OK completion, got: %s", lines[1])
	}
}

// TestStatusCommand_RecentItem tests STATUS command with RECENT item
func TestStatusCommand_RecentItem(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Ensure user table exists
	database := server.GetDatabaseFromServer(srv)
	server.CreateTestUser(t, database, "testuser")

	// Test STATUS command with RECENT item
	srv.HandleStatus(conn, "A001", []string{"A001", "STATUS", "INBOX", "(RECENT)"}, state)

	response := conn.GetWrittenData()
	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Should have 2 lines: STATUS response and completion
	if len(lines) != 2 {
		t.Fatalf("Expected 2 response lines, got %d: %v", len(lines), lines)
	}

	// Check untagged STATUS response
	statusLine := lines[0]
	if !strings.Contains(statusLine, "RECENT") {
		t.Errorf("Expected RECENT in response, got: %s", statusLine)
	}

	// Check completion response
	if !strings.Contains(lines[1], "A001 OK STATUS completed") {
		t.Errorf("Expected OK completion, got: %s", lines[1])
	}
}

// TestStatusCommand_UidnextItem tests STATUS command with UIDNEXT item
func TestStatusCommand_UidnextItem(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Ensure user table exists
	database := server.GetDatabaseFromServer(srv)
	server.CreateTestUser(t, database, "testuser")

	// Test STATUS command with UIDNEXT item
	srv.HandleStatus(conn, "A001", []string{"A001", "STATUS", "INBOX", "(UIDNEXT)"}, state)

	response := conn.GetWrittenData()
	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Should have 2 lines: STATUS response and completion
	if len(lines) != 2 {
		t.Fatalf("Expected 2 response lines, got %d: %v", len(lines), lines)
	}

	// Check untagged STATUS response
	statusLine := lines[0]
	if !strings.Contains(statusLine, "UIDNEXT") {
		t.Errorf("Expected UIDNEXT in response, got: %s", statusLine)
	}

	// UIDNEXT should be at least 1 for empty mailbox
	if !strings.Contains(statusLine, "UIDNEXT 1") {
		t.Errorf("Expected UIDNEXT 1 for empty mailbox, got: %s", statusLine)
	}

	// Check completion response
	if !strings.Contains(lines[1], "A001 OK STATUS completed") {
		t.Errorf("Expected OK completion, got: %s", lines[1])
	}
}

// TestStatusCommand_UidvalidityItem tests STATUS command with UIDVALIDITY item
func TestStatusCommand_UidvalidityItem(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Ensure user table exists
	database := server.GetDatabaseFromServer(srv)
	server.CreateTestUser(t, database, "testuser")

	// Test STATUS command with UIDVALIDITY item
	srv.HandleStatus(conn, "A001", []string{"A001", "STATUS", "INBOX", "(UIDVALIDITY)"}, state)

	response := conn.GetWrittenData()
	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Should have 2 lines: STATUS response and completion
	if len(lines) != 2 {
		t.Fatalf("Expected 2 response lines, got %d: %v", len(lines), lines)
	}

	// Check untagged STATUS response
	statusLine := lines[0]
	if !strings.Contains(statusLine, "UIDVALIDITY") {
		t.Errorf("Expected UIDVALIDITY in response, got: %s", statusLine)
	}

	// Check completion response
	if !strings.Contains(lines[1], "A001 OK STATUS completed") {
		t.Errorf("Expected OK completion, got: %s", lines[1])
	}
}

// TestStatusCommand_UnseenItem tests STATUS command with UNSEEN item
func TestStatusCommand_UnseenItem(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Ensure user table exists
	database := server.GetDatabaseFromServer(srv)
	server.CreateTestUser(t, database, "testuser")

	// Test STATUS command with UNSEEN item
	srv.HandleStatus(conn, "A001", []string{"A001", "STATUS", "INBOX", "(UNSEEN)"}, state)

	response := conn.GetWrittenData()
	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Should have 2 lines: STATUS response and completion
	if len(lines) != 2 {
		t.Fatalf("Expected 2 response lines, got %d: %v", len(lines), lines)
	}

	// Check untagged STATUS response
	statusLine := lines[0]
	if !strings.Contains(statusLine, "UNSEEN") {
		t.Errorf("Expected UNSEEN in response, got: %s", statusLine)
	}

	// Check completion response
	if !strings.Contains(lines[1], "A001 OK STATUS completed") {
		t.Errorf("Expected OK completion, got: %s", lines[1])
	}
}

// TestStatusCommand_QuotedMailboxName tests STATUS command with quoted mailbox name
func TestStatusCommand_QuotedMailboxName(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Ensure user table exists and create a mailbox
	database := server.GetDatabaseFromServer(srv)
	server.CreateTestUser(t, database, "testuser")
	server.CreateMailbox(t, database, "testuser", "Test Folder")

	// Test STATUS command with quoted mailbox name
	srv.HandleStatus(conn, "A001", []string{"A001", "STATUS", `"Test Folder"`, "(MESSAGES)"}, state)

	response := conn.GetWrittenData()
	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Should have 2 lines: STATUS response and completion
	if len(lines) != 2 {
		t.Fatalf("Expected 2 response lines, got %d: %v", len(lines), lines)
	}

	// Check untagged STATUS response
	statusLine := lines[0]
	if !strings.Contains(statusLine, `"Test Folder"`) {
		t.Errorf("Expected quoted mailbox name in response, got: %s", statusLine)
	}

	// Check completion response
	if !strings.Contains(lines[1], "A001 OK STATUS completed") {
		t.Errorf("Expected OK completion, got: %s", lines[1])
	}
}

// TestStatusCommand_CustomMailbox tests STATUS command on a custom mailbox
func TestStatusCommand_CustomMailbox(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Ensure user table exists and create a mailbox
	database := server.GetDatabaseFromServer(srv)
	server.CreateTestUser(t, database, "testuser")
	server.CreateMailbox(t, database, "testuser", "Projects")

	// Test STATUS command on custom mailbox
	srv.HandleStatus(conn, "A001", []string{"A001", "STATUS", "Projects", "(MESSAGES", "UIDNEXT)"}, state)

	response := conn.GetWrittenData()
	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Should have 2 lines: STATUS response and completion
	if len(lines) != 2 {
		t.Fatalf("Expected 2 response lines, got %d: %v", len(lines), lines)
	}

	// Check untagged STATUS response
	statusLine := lines[0]
	if !strings.Contains(statusLine, `"Projects"`) {
		t.Errorf("Expected Projects in response, got: %s", statusLine)
	}

	// Check completion response
	if !strings.Contains(lines[1], "A001 OK STATUS completed") {
		t.Errorf("Expected OK completion, got: %s", lines[1])
	}
}

// TestStatusCommand_WithMessages tests STATUS command on a mailbox with messages
func TestStatusCommand_WithMessages(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Ensure user table exists
	database := server.GetDatabaseFromServer(srv)
	server.CreateTestUser(t, database, "testuser")

	// Insert test messages into INBOX using helpers
	for i := 1; i <= 3; i++ {
		server.InsertTestMail(t, database, "testuser",
			fmt.Sprintf("Test Subject %d", i),
			"sender@example.com",
			"testuser@example.com",
			"INBOX")
	}

	// Test STATUS command
	srv.HandleStatus(conn, "A001", []string{"A001", "STATUS", "INBOX", "(MESSAGES", "UIDNEXT)"}, state)

	response := conn.GetWrittenData()
	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Should have 2 lines: STATUS response and completion
	if len(lines) != 2 {
		t.Fatalf("Expected 2 response lines, got %d: %v", len(lines), lines)
	}

	// Check untagged STATUS response
	statusLine := lines[0]
	if !strings.Contains(statusLine, "MESSAGES 3") {
		t.Errorf("Expected MESSAGES 3, got: %s", statusLine)
	}
	if !strings.Contains(statusLine, "UIDNEXT 4") {
		t.Errorf("Expected UIDNEXT 4, got: %s", statusLine)
	}

	// Check completion response
	if !strings.Contains(lines[1], "A001 OK STATUS completed") {
		t.Errorf("Expected OK completion, got: %s", lines[1])
	}
}

// TestStatusCommand_InvalidStatusItem tests STATUS command with unknown status item
func TestStatusCommand_InvalidStatusItem(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Ensure user table exists
	database := server.GetDatabaseFromServer(srv)
	server.CreateTestUser(t, database, "testuser")

	// Test STATUS command with invalid status item
	srv.HandleStatus(conn, "A001", []string{"A001", "STATUS", "INBOX", "(INVALID)"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A001 BAD Unknown status data item") {
		t.Errorf("Expected BAD response for invalid status item, got: %s", response)
	}
}

// TestStatusCommand_DefaultMailboxes tests STATUS command on default mailboxes
func TestStatusCommand_DefaultMailboxes(t *testing.T) {
	defaultMailboxes := []string{"INBOX", "Sent", "Drafts", "Trash"}

	for _, mailbox := range defaultMailboxes {
		t.Run(mailbox, func(t *testing.T) {
			srv := server.SetupTestServerSimple(t)
			conn := server.NewMockConn()
			state := server.SetupAuthenticatedState(t, srv, "testuser")

			// Test STATUS command
			srv.HandleStatus(conn, "A001", []string{"A001", "STATUS", mailbox, "(MESSAGES)"}, state)

			response := conn.GetWrittenData()
			lines := strings.Split(strings.TrimSpace(response), "\r\n")

			// Should have 2 lines: STATUS response and completion
			if len(lines) != 2 {
				t.Fatalf("Expected 2 response lines for %s, got %d: %v", mailbox, len(lines), lines)
			}

			// Check untagged STATUS response
			statusLine := lines[0]
			if !strings.Contains(statusLine, fmt.Sprintf(`"%s"`, mailbox)) {
				t.Errorf("Expected %s in response, got: %s", mailbox, statusLine)
			}

			// Check completion response
			if !strings.Contains(lines[1], "A001 OK STATUS completed") {
				t.Errorf("Expected OK completion for %s, got: %s", mailbox, lines[1])
			}
		})
	}
}

// TestStatusCommand_RFC3501Example tests the exact example from RFC 3501
func TestStatusCommand_RFC3501Example(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Ensure user table exists and create the mailbox
	database := server.GetDatabaseFromServer(srv)
	server.CreateTestUser(t, database, "testuser")
	server.CreateMailbox(t, database, "testuser", "blurdybloop")

	// Test STATUS command as per RFC 3501 example: STATUS blurdybloop (UIDNEXT MESSAGES)
	srv.HandleStatus(conn, "A042", []string{"A042", "STATUS", "blurdybloop", "(UIDNEXT", "MESSAGES)"}, state)

	response := conn.GetWrittenData()
	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Should have 2 lines: STATUS response and completion
	if len(lines) != 2 {
		t.Fatalf("Expected 2 response lines, got %d: %v", len(lines), lines)
	}

	// Check untagged STATUS response
	statusLine := lines[0]
	if !strings.HasPrefix(statusLine, `* STATUS "blurdybloop"`) {
		t.Errorf("Expected STATUS response for blurdybloop, got: %s", statusLine)
	}

	// Should contain both MESSAGES and UIDNEXT
	if !strings.Contains(statusLine, "MESSAGES") {
		t.Errorf("Expected MESSAGES in response, got: %s", statusLine)
	}
	if !strings.Contains(statusLine, "UIDNEXT") {
		t.Errorf("Expected UIDNEXT in response, got: %s", statusLine)
	}

	// Check completion response
	if !strings.Contains(lines[1], "A042 OK STATUS completed") {
		t.Errorf("Expected OK completion, got: %s", lines[1])
	}
}

// TestStatusCommand_CaseInsensitiveItems tests STATUS command with mixed case items
func TestStatusCommand_CaseInsensitiveItems(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Ensure user table exists
	database := server.GetDatabaseFromServer(srv)
	server.CreateTestUser(t, database, "testuser")

	// Test STATUS command with mixed case items
	srv.HandleStatus(conn, "A001", []string{"A001", "STATUS", "INBOX", "(messages", "UidNext)"}, state)

	response := conn.GetWrittenData()
	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Should have 2 lines: STATUS response and completion
	if len(lines) != 2 {
		t.Fatalf("Expected 2 response lines, got %d: %v", len(lines), lines)
	}

	// Check untagged STATUS response - should normalize to uppercase
	statusLine := lines[0]
	if !strings.Contains(statusLine, "MESSAGES") {
		t.Errorf("Expected MESSAGES in response, got: %s", statusLine)
	}
	if !strings.Contains(statusLine, "UIDNEXT") {
		t.Errorf("Expected UIDNEXT in response, got: %s", statusLine)
	}

	// Check completion response
	if !strings.Contains(lines[1], "A001 OK STATUS completed") {
		t.Errorf("Expected OK completion, got: %s", lines[1])
	}
}
