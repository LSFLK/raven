//go:build test
// +build test

package server_test

import (
	"strings"
	"testing"

	"go-imap/internal/models"
	"go-imap/test/helpers"
)

// TestLsubCommand_BasicUsage tests LSUB with basic wildcard patterns
func TestLsubCommand_BasicUsage(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := helpers.SetupAuthenticatedState(t, server, "testuser")

	// Subscribe to some mailboxes first
	server.HandleSubscribe(conn, "S1", []string{"S1", "SUBSCRIBE", "INBOX"}, state)
	server.HandleSubscribe(conn, "S2", []string{"S2", "SUBSCRIBE", "Sent"}, state)
	server.HandleSubscribe(conn, "S3", []string{"S3", "SUBSCRIBE", "Drafts"}, state)
	conn.ClearWriteBuffer()

	// Test LSUB with * wildcard (all mailboxes)
	server.HandleLsub(conn, "A001", []string{"A001", "LSUB", `""`, "*"}, state)

	response := conn.GetWrittenData()

	// Should list all subscribed mailboxes
	if !strings.Contains(response, "* LSUB") {
		t.Errorf("Expected LSUB responses")
	}
	if !strings.Contains(response, "INBOX") {
		t.Errorf("Expected INBOX in response")
	}
	if !strings.Contains(response, "Sent") {
		t.Errorf("Expected Sent in response")
	}
	if !strings.Contains(response, "Drafts") {
		t.Errorf("Expected Drafts in response")
	}
	if !strings.Contains(response, "A001 OK LSUB completed") {
		t.Errorf("Expected OK response")
	}
}

// TestLsubCommand_NotAuthenticated tests LSUB without authentication
func TestLsubCommand_NotAuthenticated(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: false,
	}

	server.HandleLsub(conn, "A001", []string{"A001", "LSUB", `""`, "*"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A001 NO Please authenticate first") {
		t.Errorf("Expected authentication required error, got: %s", response)
	}
}

// TestLsubCommand_MissingArguments tests LSUB with missing arguments
func TestLsubCommand_MissingArguments(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := helpers.SetupAuthenticatedState(t, server, "testuser")

	// Test with only one argument
	server.HandleLsub(conn, "A001", []string{"A001", "LSUB", `""`}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A001 BAD LSUB command requires reference and mailbox arguments") {
		t.Errorf("Expected BAD response for missing arguments, got: %s", response)
	}
}

// TestLsubCommand_EmptyPattern tests LSUB with empty pattern (hierarchy delimiter query)
func TestLsubCommand_EmptyPattern(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := helpers.SetupAuthenticatedState(t, server, "testuser")

	// LSUB "" "" should return hierarchy delimiter
	server.HandleLsub(conn, "A001", []string{"A001", "LSUB", `""`, `""`}, state)

	response := conn.GetWrittenData()

	// Should return hierarchy delimiter with \Noselect
	if !strings.Contains(response, `* LSUB (\Noselect) "/" ""`) {
		t.Errorf("Expected hierarchy delimiter response, got: %s", response)
	}
	if !strings.Contains(response, "A001 OK LSUB completed") {
		t.Errorf("Expected OK response")
	}
}

// TestLsubCommand_PercentWildcard tests LSUB with % wildcard
func TestLsubCommand_PercentWildcard(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := helpers.SetupAuthenticatedState(t, server, "testuser")

	// Subscribe to mailboxes at different hierarchy levels
	server.HandleSubscribe(conn, "S1", []string{"S1", "SUBSCRIBE", "INBOX"}, state)
	server.HandleSubscribe(conn, "S2", []string{"S2", "SUBSCRIBE", "Work"}, state)
	server.HandleSubscribe(conn, "S3", []string{"S3", "SUBSCRIBE", "Personal"}, state)
	conn.ClearWriteBuffer()

	// Test LSUB with % wildcard (should only match top-level, not hierarchies)
	server.HandleLsub(conn, "A001", []string{"A001", "LSUB", `""`, "%"}, state)

	response := conn.GetWrittenData()

	// Should list top-level subscribed mailboxes only
	if !strings.Contains(response, "INBOX") {
		t.Errorf("Expected INBOX in response")
	}
	if !strings.Contains(response, "A001 OK LSUB completed") {
		t.Errorf("Expected OK response")
	}
}

// TestLsubCommand_ImpliedParentWithNoselect tests RFC 3501 special case
// When "foo/bar" is subscribed but "foo" is not, LSUB with % must return "foo" with \Noselect
func TestLsubCommand_ImpliedParentWithNoselect(t *testing.T) {
	testDB := helpers.CreateTestDB(t)
	server := helpers.TestServerWithDB(testDB)
	conn := helpers.NewMockConn()
	state := helpers.SetupAuthenticatedState(t, server, "testuser")

	// Subscribe to "Work/Projects" but NOT to "Work"
	// This creates an implied parent "Work" that should be returned with \Noselect
	helpers.SubscribeToMailbox(t, testDB, "testuser", "Work/Projects")

	// Test LSUB with % wildcard at root level
	server.HandleLsub(conn, "A001", []string{"A001", "LSUB", `""`, "%"}, state)

	response := conn.GetWrittenData()

	// Should return "Work" with \Noselect attribute
	if !strings.Contains(response, `* LSUB (\Noselect) "/" "Work"`) {
		t.Errorf("Expected implied parent 'Work' with \\Noselect, got: %s", response)
	}

	// Should NOT return "Work/Projects" at this level (% doesn't match hierarchies)
	if strings.Contains(response, "Work/Projects") {
		t.Errorf("Should not return 'Work/Projects' with %% wildcard at root level")
	}

	if !strings.Contains(response, "A001 OK LSUB completed") {
		t.Errorf("Expected OK response")
	}
}

// TestLsubCommand_HierarchyWithPercent tests LSUB with hierarchy and % wildcard
func TestLsubCommand_HierarchyWithPercent(t *testing.T) {
	testDB := helpers.CreateTestDB(t)
	server := helpers.TestServerWithDB(testDB)
	conn := helpers.NewMockConn()
	state := helpers.SetupAuthenticatedState(t, server, "testuser")

	// Subscribe to hierarchical mailboxes
	helpers.SubscribeToMailbox(t, testDB, "testuser", "Work/Projects/2024")
	helpers.SubscribeToMailbox(t, testDB, "testuser", "Work/Tasks")

	// Test LSUB with "Work/%" pattern (should match Work's immediate children)
	server.HandleLsub(conn, "A001", []string{"A001", "LSUB", `""`, "Work/%"}, state)

	response := conn.GetWrittenData()

	// Should return immediate children of Work
	if !strings.Contains(response, "Work/Tasks") {
		t.Errorf("Expected 'Work/Tasks' in response, got: %s", response)
	}

	// Should return "Work/Projects" with \Noselect (implied parent)
	if !strings.Contains(response, `* LSUB (\Noselect) "/" "Work/Projects"`) {
		t.Errorf("Expected implied parent 'Work/Projects' with \\Noselect, got: %s", response)
	}

	// Should NOT return "Work/Projects/2024" (nested hierarchy)
	if strings.Contains(response, "Work/Projects/2024") {
		t.Errorf("Should not return 'Work/Projects/2024' with Work/%% pattern")
	}

	if !strings.Contains(response, "A001 OK LSUB completed") {
		t.Errorf("Expected OK response")
	}
}

// TestLsubCommand_StarWildcard tests LSUB with * wildcard (matches all levels)
func TestLsubCommand_StarWildcard(t *testing.T) {
	testDB := helpers.CreateTestDB(t)
	server := helpers.TestServerWithDB(testDB)
	conn := helpers.NewMockConn()
	state := helpers.SetupAuthenticatedState(t, server, "testuser")

	// Subscribe to hierarchical mailboxes
	helpers.SubscribeToMailbox(t, testDB, "testuser", "Work")
	helpers.SubscribeToMailbox(t, testDB, "testuser", "Work/Projects")
	helpers.SubscribeToMailbox(t, testDB, "testuser", "Work/Projects/2024")

	// Test LSUB with * wildcard (should match all levels)
	server.HandleLsub(conn, "A001", []string{"A001", "LSUB", `""`, "*"}, state)

	response := conn.GetWrittenData()

	// Should return all subscribed mailboxes at all levels
	if !strings.Contains(response, "Work") {
		t.Errorf("Expected 'Work' in response")
	}
	if !strings.Contains(response, "Work/Projects") {
		t.Errorf("Expected 'Work/Projects' in response")
	}
	if !strings.Contains(response, "Work/Projects/2024") {
		t.Errorf("Expected 'Work/Projects/2024' in response")
	}

	if !strings.Contains(response, "A001 OK LSUB completed") {
		t.Errorf("Expected OK response")
	}
}

// TestLsubCommand_ReferenceAndPattern tests LSUB with reference prefix
func TestLsubCommand_ReferenceAndPattern(t *testing.T) {
	testDB := helpers.CreateTestDB(t)
	server := helpers.TestServerWithDB(testDB)
	conn := helpers.NewMockConn()
	state := helpers.SetupAuthenticatedState(t, server, "testuser")

	// Subscribe to hierarchical mailboxes
	helpers.SubscribeToMailbox(t, testDB, "testuser", "Work/Projects")
	helpers.SubscribeToMailbox(t, testDB, "testuser", "Work/Tasks")
	helpers.SubscribeToMailbox(t, testDB, "testuser", "Personal/Projects")

	// Test LSUB with reference "Work/" and pattern "*"
	server.HandleLsub(conn, "A001", []string{"A001", "LSUB", `"Work/"`, "*"}, state)

	response := conn.GetWrittenData()

	// Should only return mailboxes under Work/
	if !strings.Contains(response, "Work/Projects") {
		t.Errorf("Expected 'Work/Projects' in response")
	}
	if !strings.Contains(response, "Work/Tasks") {
		t.Errorf("Expected 'Work/Tasks' in response")
	}

	// Should NOT return Personal/Projects
	if strings.Contains(response, "Personal/Projects") {
		t.Errorf("Should not return 'Personal/Projects' with Work/ reference")
	}

	if !strings.Contains(response, "A001 OK LSUB completed") {
		t.Errorf("Expected OK response")
	}
}

// TestLsubCommand_NonExistentMailboxSubscription tests that LSUB returns subscriptions
// even if the mailbox doesn't exist (per RFC 3501)
func TestLsubCommand_NonExistentMailboxSubscription(t *testing.T) {
	testDB := helpers.CreateTestDB(t)
	server := helpers.TestServerWithDB(testDB)
	conn := helpers.NewMockConn()
	state := helpers.SetupAuthenticatedState(t, server, "testuser")

	// Subscribe to a mailbox that doesn't exist
	// RFC 3501: "The server MUST NOT unilaterally remove an existing mailbox name
	// from the subscription list even if a mailbox by that name no longer exists."
	helpers.SubscribeToMailbox(t, testDB, "testuser", "DeletedMailbox")

	// Test LSUB
	server.HandleLsub(conn, "A001", []string{"A001", "LSUB", `""`, "*"}, state)

	response := conn.GetWrittenData()

	// Should still return the subscription even though mailbox doesn't exist
	if !strings.Contains(response, "DeletedMailbox") {
		t.Errorf("Expected 'DeletedMailbox' in response (subscriptions should persist), got: %s", response)
	}

	if !strings.Contains(response, "A001 OK LSUB completed") {
		t.Errorf("Expected OK response")
	}
}

// TestLsubCommand_SpecificPattern tests LSUB with specific mailbox name
func TestLsubCommand_SpecificPattern(t *testing.T) {
	testDB := helpers.CreateTestDB(t)
	server := helpers.TestServerWithDB(testDB)
	conn := helpers.NewMockConn()
	state := helpers.SetupAuthenticatedState(t, server, "testuser")

	// Subscribe to multiple mailboxes
	helpers.SubscribeToMailbox(t, testDB, "testuser", "INBOX")
	helpers.SubscribeToMailbox(t, testDB, "testuser", "Sent")
	helpers.SubscribeToMailbox(t, testDB, "testuser", "Drafts")

	// Test LSUB with specific mailbox name (no wildcards)
	server.HandleLsub(conn, "A001", []string{"A001", "LSUB", `""`, "Sent"}, state)

	response := conn.GetWrittenData()

	// Should only return Sent
	if !strings.Contains(response, `* LSUB (\Sent) "/" "Sent"`) {
		t.Errorf("Expected only 'Sent' in response, got: %s", response)
	}

	// Should NOT return INBOX or Drafts
	if strings.Contains(response, "INBOX") || strings.Contains(response, "Drafts") {
		t.Errorf("Should only return Sent, got: %s", response)
	}

	if !strings.Contains(response, "A001 OK LSUB completed") {
		t.Errorf("Expected OK response")
	}
}

// TestLsubCommand_CaseInsensitiveINBOX tests that INBOX is case-insensitive
func TestLsubCommand_CaseInsensitiveINBOX(t *testing.T) {
	testDB := helpers.CreateTestDB(t)
	server := helpers.TestServerWithDB(testDB)
	conn := helpers.NewMockConn()
	state := helpers.SetupAuthenticatedState(t, server, "testuser")

	// Subscribe to INBOX
	helpers.SubscribeToMailbox(t, testDB, "testuser", "INBOX")

	// Test LSUB with lowercase "inbox"
	server.HandleLsub(conn, "A001", []string{"A001", "LSUB", `""`, "inbox"}, state)

	response := conn.GetWrittenData()

	// Should return INBOX (case-insensitive match)
	if !strings.Contains(response, "INBOX") {
		t.Errorf("Expected INBOX in response (case-insensitive), got: %s", response)
	}

	if !strings.Contains(response, "A001 OK LSUB completed") {
		t.Errorf("Expected OK response")
	}
}

// TestLsubCommand_EmptySubscriptionList tests LSUB when user has no subscriptions
func TestLsubCommand_EmptySubscriptionList(t *testing.T) {
	testDB := helpers.CreateTestDB(t)
	server := helpers.TestServerWithDB(testDB)
	conn := helpers.NewMockConn()
	state := helpers.SetupAuthenticatedState(t, server, "newuser")

	// Don't subscribe to anything - but default mailboxes should auto-subscribe
	server.HandleLsub(conn, "A001", []string{"A001", "LSUB", `""`, "*"}, state)

	response := conn.GetWrittenData()

	// Should auto-subscribe to default mailboxes and return them
	if !strings.Contains(response, "INBOX") {
		t.Errorf("Expected auto-subscription to INBOX")
	}
	if !strings.Contains(response, "Sent") {
		t.Errorf("Expected auto-subscription to Sent")
	}

	if !strings.Contains(response, "A001 OK LSUB completed") {
		t.Errorf("Expected OK response")
	}
}
