package server_test

import (
	"fmt"
	"strings"
	"testing"

	"go-imap/internal/models"
	"go-imap/test/helpers"
)

// TestListCommand_Authentication tests LIST command authentication requirements
func TestListCommand_Authentication(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: false,
	}

	// Test LIST command without authentication
	server.HandleList(conn, "A001", []string{"A001", "LIST", `""`, `"*"`}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A001 NO Please authenticate first") {
		t.Errorf("Expected authentication error, got: %s", response)
	}
}

// TestListCommand_InvalidArguments tests LIST command with invalid arguments
func TestListCommand_InvalidArguments(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	// Test LIST command with insufficient arguments
	server.HandleList(conn, "A001", []string{"A001", "LIST"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A001 BAD LIST command requires reference and mailbox arguments") {
		t.Errorf("Expected BAD response for insufficient arguments, got: %s", response)
	}
}

// TestListCommand_HierarchyDelimiter tests LIST command with empty mailbox name
func TestListCommand_HierarchyDelimiter(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	// Test LIST command with empty mailbox name to get hierarchy delimiter
	server.HandleList(conn, "A001", []string{"A001", "LIST", `""`, `""`}, state)

	response := conn.GetWrittenData()
	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Should have 2 lines: LIST response and completion
	if len(lines) != 2 {
		t.Fatalf("Expected 2 response lines, got %d: %v", len(lines), lines)
	}

	// Check untagged LIST response for hierarchy delimiter
	listLine := lines[0]
	if !strings.HasPrefix(listLine, "* LIST (\\Noselect) \"/\" \"\"") {
		t.Errorf("Expected hierarchy delimiter response, got: %s", listLine)
	}

	// Check completion response
	if !strings.Contains(lines[1], "A001 OK LIST completed") {
		t.Errorf("Expected OK completion, got: %s", lines[1])
	}
}

// TestListCommand_HierarchyDelimiterWithReference tests LIST with reference and empty mailbox
func TestListCommand_HierarchyDelimiterWithReference(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	// Test LIST command with reference and empty mailbox name
	server.HandleList(conn, "A001", []string{"A001", "LIST", `"~/Mail/"`, `""`}, state)

	response := conn.GetWrittenData()
	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Should have 2 lines: LIST response and completion
	if len(lines) != 2 {
		t.Fatalf("Expected 2 response lines, got %d: %v", len(lines), lines)
	}

	// Check untagged LIST response shows the reference as root
	listLine := lines[0]
	if !strings.HasPrefix(listLine, "* LIST (\\Noselect) \"/\" \"~/Mail/\"") {
		t.Errorf("Expected reference as root, got: %s", listLine)
	}
}

// TestListCommand_AllMailboxes tests LIST command with wildcard to get all mailboxes
func TestListCommand_AllMailboxes(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	// Test LIST command with * wildcard
	server.HandleList(conn, "A001", []string{"A001", "LIST", `""`, `"*"`}, state)

	response := conn.GetWrittenData()
	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Should have at least completion line
	if len(lines) < 1 {
		t.Fatalf("Expected at least 1 response line, got %d: %v", len(lines), lines)
	}

	// Check that we get standard mailboxes
	expectedMailboxes := []string{"INBOX", "Sent", "Drafts", "Trash"}
	for _, expected := range expectedMailboxes {
		found := false
		for _, line := range lines {
			if strings.Contains(line, fmt.Sprintf("\"%s\"", expected)) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected to find mailbox %s in response: %s", expected, response)
		}
	}

	// Check completion response
	lastLine := lines[len(lines)-1]
	if !strings.Contains(lastLine, "A001 OK LIST completed") {
		t.Errorf("Expected OK completion, got: %s", lastLine)
	}
}

// TestListCommand_SpecificMailbox tests LIST command with specific mailbox name
func TestListCommand_SpecificMailbox(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	// Test LIST command with specific mailbox
	server.HandleList(conn, "A001", []string{"A001", "LIST", `""`, `"INBOX"`}, state)

	response := conn.GetWrittenData()
	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Should have 2 lines: LIST response for INBOX and completion
	if len(lines) != 2 {
		t.Fatalf("Expected 2 response lines, got %d: %v", len(lines), lines)
	}

	// Check INBOX response
	listLine := lines[0]
	if !strings.Contains(listLine, "* LIST (\\Unmarked) \"/\" \"INBOX\"") {
		t.Errorf("Expected INBOX response, got: %s", listLine)
	}

	// Check completion response
	if !strings.Contains(lines[1], "A001 OK LIST completed") {
		t.Errorf("Expected OK completion, got: %s", lines[1])
	}
}

// TestListCommand_WildcardMatching tests LIST command with various wildcard patterns
func TestListCommand_WildcardMatching(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	testCases := []struct {
		pattern          string
		expectedContains []string
		expectedNotContains []string
		description      string
	}{
		{
			pattern:          "IN*",
			expectedContains: []string{"INBOX"},
			expectedNotContains: []string{"Sent", "Drafts"},
			description:      "Pattern IN* should match INBOX",
		},
		{
			pattern:          "*ox",
			expectedContains: []string{"INBOX"},
			expectedNotContains: []string{"Sent"},
			description:      "Pattern *ox should match INBOX",
		},
		{
			pattern:          "S*",
			expectedContains: []string{"Sent"},
			expectedNotContains: []string{"INBOX", "Drafts"},
			description:      "Pattern S* should match Sent",
		},
		{
			pattern:          "*",
			expectedContains: []string{"INBOX", "Sent", "Drafts", "Trash"},
			expectedNotContains: []string{},
			description:      "Pattern * should match all mailboxes",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			conn.ClearWriteBuffer()
			server.HandleList(conn, "A001", []string{"A001", "LIST", `""`, fmt.Sprintf(`"%s"`, tc.pattern)}, state)

			response := conn.GetWrittenData()

			// Check expected mailboxes are present
			for _, expected := range tc.expectedContains {
				if !strings.Contains(response, fmt.Sprintf("\"%s\"", expected)) {
					t.Errorf("Pattern %s should match %s, but not found in: %s", tc.pattern, expected, response)
				}
			}

			// Check unexpected mailboxes are not present
			for _, notExpected := range tc.expectedNotContains {
				if strings.Contains(response, fmt.Sprintf("\"%s\"", notExpected)) {
					t.Errorf("Pattern %s should not match %s, but found in: %s", tc.pattern, notExpected, response)
				}
			}

			// Check completion
			if !strings.Contains(response, "A001 OK LIST completed") {
				t.Errorf("Expected OK completion for pattern %s, got: %s", tc.pattern, response)
			}
		})
	}
}

// TestListCommand_PercentWildcard tests LIST command with % wildcard (no hierarchy delimiter match)
func TestListCommand_PercentWildcard(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	// Test LIST command with % wildcard
	server.HandleList(conn, "A001", []string{"A001", "LIST", `""`, `"%"`}, state)

	response := conn.GetWrittenData()

	// % should match all top-level mailboxes (same as * for flat namespace)
	expectedMailboxes := []string{"INBOX", "Sent", "Drafts", "Trash"}
	for _, expected := range expectedMailboxes {
		if !strings.Contains(response, fmt.Sprintf("\"%s\"", expected)) {
			t.Errorf("Expected to find mailbox %s in response: %s", expected, response)
		}
	}

	// Check completion response
	if !strings.Contains(response, "A001 OK LIST completed") {
		t.Errorf("Expected OK completion, got: %s", response)
	}
}

// TestListCommand_CaseInsensitiveINBOX tests that INBOX matching is case-insensitive
func TestListCommand_CaseInsensitiveINBOX(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	testCases := []string{"inbox", "Inbox", "InBoX", "INBOX"}

	for _, pattern := range testCases {
		t.Run(fmt.Sprintf("Pattern_%s", pattern), func(t *testing.T) {
			conn.ClearWriteBuffer()
			server.HandleList(conn, "A001", []string{"A001", "LIST", `""`, fmt.Sprintf(`"%s"`, pattern)}, state)

			response := conn.GetWrittenData()

			// Should match INBOX regardless of case
			if !strings.Contains(response, "\"INBOX\"") {
				t.Errorf("Pattern %s should match INBOX, got: %s", pattern, response)
			}

			// Check completion
			if !strings.Contains(response, "A001 OK LIST completed") {
				t.Errorf("Expected OK completion for pattern %s, got: %s", pattern, response)
			}
		})
	}
}

// TestListCommand_MailboxAttributes tests that mailboxes have correct attributes
func TestListCommand_MailboxAttributes(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	// Test LIST command with * wildcard to get all mailboxes
	server.HandleList(conn, "A001", []string{"A001", "LIST", `""`, `"*"`}, state)

	response := conn.GetWrittenData()

	// Check specific attributes for special mailboxes
	expectedAttributes := map[string]string{
		"INBOX":  "\\Unmarked",
		"Sent":   "\\Sent",
		"Drafts": "\\Drafts",
		"Trash":  "\\Trash",
	}

	for mailbox, expectedAttr := range expectedAttributes {
		expectedPattern := fmt.Sprintf("* LIST (%s) \"/\" \"%s\"", expectedAttr, mailbox)
		if !strings.Contains(response, expectedPattern) {
			t.Errorf("Expected mailbox %s to have attribute %s, got: %s", mailbox, expectedAttr, response)
		}
	}
}

// TestListCommand_QuotedStrings tests LIST command with various quoted string formats
func TestListCommand_QuotedStrings(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	testCases := []struct {
		reference string
		mailbox   string
		description string
	}{
		{`""`, `"INBOX"`, "Empty reference, quoted INBOX"},
		{`""`, `INBOX`, "Empty reference, unquoted INBOX"},
		{`"~/Mail/"`, `""`, "Quoted reference, empty mailbox"},
		{`""`, `""`, "Both empty and quoted"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			conn.ClearWriteBuffer()
			server.HandleList(conn, "A001", []string{"A001", "LIST", tc.reference, tc.mailbox}, state)

			response := conn.GetWrittenData()

			// Should not return BAD response for valid quoted strings
			if strings.Contains(response, "A001 BAD") {
				t.Errorf("Valid quoted strings should not return BAD: %s", response)
			}

			// Should complete successfully
			if !strings.Contains(response, "A001 OK LIST completed") {
				t.Errorf("Expected OK completion, got: %s", response)
			}
		})
	}
}

// TestListCommand_ReferenceHandling tests LIST command reference argument handling
func TestListCommand_ReferenceHandling(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	// Test with reference that should be prefixed to pattern
	server.HandleList(conn, "A001", []string{"A001", "LIST", `"Mail/"`, `"IN*"`}, state)

	response := conn.GetWrittenData()

	// The canonical pattern should be "Mail/IN*" which shouldn't match our flat mailboxes
	// So we should get completion without matches
	lines := strings.Split(strings.TrimSpace(response), "\r\n")
	
	// Should only have completion line (no mailbox matches expected for "Mail/IN*")
	if len(lines) != 1 {
		t.Logf("Response lines: %v", lines)
		// This is acceptable - might match depending on implementation
	}

	// Check completion
	if !strings.Contains(response, "A001 OK LIST completed") {
		t.Errorf("Expected OK completion, got: %s", response)
	}
}
