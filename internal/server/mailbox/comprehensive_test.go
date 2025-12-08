//go:build test

package mailbox_test

import (
	"strings"
	"testing"

	"raven/internal/server"
)

// ===== Additional LSUB Tests to improve coverage =====

// TestLsubCommand_WithReference tests LSUB with non-empty reference
func TestLsubCommand_WithReference(t *testing.T) {
	testDB := server.CreateTestDB(t)
	srv := server.TestServerWithDBManager(testDB)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	server.SubscribeToMailbox(t, testDB, "testuser", "Foo/Bar/Baz")
	server.SubscribeToMailbox(t, testDB, "testuser", "Foo/Qux")

	// LSUB with non-empty reference
	srv.HandleLsub(conn, "A001", []string{"A001", "LSUB", `"Foo/"`, "%"}, state)

	response := conn.GetWrittenData()

	if !strings.Contains(response, "Foo/Qux") {
		t.Errorf("Expected Foo/Qux in response: %s", response)
	}
	if !strings.Contains(response, "A001 OK LSUB completed") {
		t.Errorf("Expected OK completion")
	}
}

// TestLsubCommand_MultipleImpliedParents tests multiple implied parent folders
func TestLsubCommand_MultipleImpliedParents(t *testing.T) {
	testDB := server.CreateTestDB(t)
	srv := server.TestServerWithDBManager(testDB)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Subscribe to deeply nested mailboxes without subscribing to parents
	server.SubscribeToMailbox(t, testDB, "testuser", "A/B/C/D")
	server.SubscribeToMailbox(t, testDB, "testuser", "A/B/E")

	// LSUB with % should return implied parents with \Noselect
	srv.HandleLsub(conn, "A001", []string{"A001", "LSUB", `""`, "%"}, state)

	response := conn.GetWrittenData()

	// Should have implied parent "A" with \Noselect
	if !strings.Contains(response, "\\Noselect") {
		t.Errorf("Expected \\Noselect attribute for implied parent")
	}
	if !strings.Contains(response, "A001 OK LSUB completed") {
		t.Errorf("Expected OK completion")
	}
}

// TestLsubCommand_MixedSubscriptions tests mix of subscribed and implied parents
func TestLsubCommand_MixedSubscriptions(t *testing.T) {
	testDB := server.CreateTestDB(t)
	srv := server.TestServerWithDBManager(testDB)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Subscribe to parent and child
	server.SubscribeToMailbox(t, testDB, "testuser", "Parent")
	server.SubscribeToMailbox(t, testDB, "testuser", "Parent/Child")

	// LSUB should return both
	srv.HandleLsub(conn, "A001", []string{"A001", "LSUB", `""`, "*"}, state)

	response := conn.GetWrittenData()

	if !strings.Contains(response, "Parent") {
		t.Errorf("Expected Parent in response")
	}
	if !strings.Contains(response, "Parent/Child") {
		t.Errorf("Expected Parent/Child in response")
	}
	if !strings.Contains(response, "A001 OK LSUB completed") {
		t.Errorf("Expected OK completion")
	}
}

// ===== Additional LIST Tests to improve coverage =====

// TestListCommand_MultipleMailboxes tests LIST with many mailboxes
func TestListCommand_MultipleMailboxes(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	database := server.GetDatabaseFromServer(srv)

	// Create many mailboxes
	mailboxNames := []string{
		"Archive", "Projects", "Clients", "Personal",
		"Work/2024", "Work/2023", "Work/2022",
		"Projects/Active", "Projects/Completed",
	}

	for _, name := range mailboxNames {
		server.CreateMailbox(t, database, "testuser", name)
	}

	// List all with *
	srv.HandleList(conn, "A001", []string{"A001", "LIST", `""`, "*"}, state)

	response := conn.GetWrittenData()

	// Check that all mailboxes are listed
	for _, name := range mailboxNames {
		if !strings.Contains(response, name) {
			t.Errorf("Expected %s in LIST response", name)
		}
	}

	if !strings.Contains(response, "A001 OK LIST completed") {
		t.Errorf("Expected OK completion")
	}
}

// TestListCommand_SpecialCharactersInMailboxName tests LIST with special characters
func TestListCommand_SpecialCharactersInMailboxName(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	database := server.GetDatabaseFromServer(srv)

	// Create mailbox with special characters
	server.CreateMailbox(t, database, "testuser", "Test-Box")
	server.CreateMailbox(t, database, "testuser", "Test_Box")
	server.CreateMailbox(t, database, "testuser", "Test.Box")

	// List with wildcard
	srv.HandleList(conn, "A001", []string{"A001", "LIST", `""`, "Test*"}, state)

	response := conn.GetWrittenData()

	// Should match all Test* mailboxes
	if !strings.Contains(response, "Test-Box") {
		t.Errorf("Expected Test-Box in response")
	}
	if !strings.Contains(response, "Test_Box") {
		t.Errorf("Expected Test_Box in response")
	}
	if !strings.Contains(response, "Test.Box") {
		t.Errorf("Expected Test.Box in response")
	}

	if !strings.Contains(response, "A001 OK LIST completed") {
		t.Errorf("Expected OK completion")
	}
}

// TestListCommand_DeepHierarchy tests LIST with deep mailbox hierarchy
func TestListCommand_DeepHierarchy(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	database := server.GetDatabaseFromServer(srv)

	// Create deep hierarchy
	server.CreateMailbox(t, database, "testuser", "A/B/C/D/E/F")

	// List all levels with *
	srv.HandleList(conn, "A001", []string{"A001", "LIST", `""`, "*"}, state)

	response := conn.GetWrittenData()

	// Should include the deep mailbox
	if !strings.Contains(response, "A/B/C/D/E/F") {
		t.Errorf("Expected deep hierarchy mailbox in response: %s", response)
	}

	if !strings.Contains(response, "A001 OK LIST completed") {
		t.Errorf("Expected OK completion")
	}
}

// TestListCommand_PatternAtDifferentLevels tests LIST pattern at various hierarchy levels
func TestListCommand_PatternAtDifferentLevels(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	database := server.GetDatabaseFromServer(srv)

	server.CreateMailbox(t, database, "testuser", "Root/Level1/Item1")
	server.CreateMailbox(t, database, "testuser", "Root/Level1/Item2")
	server.CreateMailbox(t, database, "testuser", "Root/Level2/Item1")

	// List Root/Level1/* pattern
	srv.HandleList(conn, "A001", []string{"A001", "LIST", `""`, "Root/Level1/*"}, state)

	response := conn.GetWrittenData()

	// Should match only Level1 items
	if !strings.Contains(response, "Root/Level1/Item1") {
		t.Errorf("Expected Root/Level1/Item1 in response")
	}
	if !strings.Contains(response, "Root/Level1/Item2") {
		t.Errorf("Expected Root/Level1/Item2 in response")
	}
	// Should NOT match Level2 items
	if strings.Contains(response, "Root/Level2") {
		t.Errorf("Should not match Root/Level2 items")
	}

	if !strings.Contains(response, "A001 OK LIST completed") {
		t.Errorf("Expected OK completion")
	}
}

// TestListCommand_WildcardInMiddle tests LIST with wildcard in middle of pattern
func TestListCommand_WildcardInMiddle(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	database := server.GetDatabaseFromServer(srv)

	server.CreateMailbox(t, database, "testuser", "Test123Box")
	server.CreateMailbox(t, database, "testuser", "TestABCBox")
	server.CreateMailbox(t, database, "testuser", "TestBox")

	// List with wildcard in middle
	srv.HandleList(conn, "A001", []string{"A001", "LIST", `""`, "Test*Box"}, state)

	response := conn.GetWrittenData()

	// Should match all Test*Box patterns
	if !strings.Contains(response, "Test123Box") {
		t.Errorf("Expected Test123Box in response")
	}
	if !strings.Contains(response, "TestBox") {
		t.Errorf("Expected TestBox in response")
	}

	if !strings.Contains(response, "A001 OK LIST completed") {
		t.Errorf("Expected OK completion")
	}
}

// TestListCommand_CombinedReferencAndPattern tests LIST with both reference and pattern
func TestListCommand_CombinedReferenceAndPattern(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	database := server.GetDatabaseFromServer(srv)

	server.CreateMailbox(t, database, "testuser", "Mail/Inbox/2024")
	server.CreateMailbox(t, database, "testuser", "Mail/Inbox/2023")
	server.CreateMailbox(t, database, "testuser", "Mail/Sent/2024")

	// List with reference "Mail/Inbox/" and pattern "*"
	srv.HandleList(conn, "A001", []string{"A001", "LIST", `"Mail/Inbox/"`, "*"}, state)

	response := conn.GetWrittenData()

	// Should match Mail/Inbox/* mailboxes
	if !strings.Contains(response, "Mail/Inbox/2024") {
		t.Errorf("Expected Mail/Inbox/2024 in response")
	}
	if !strings.Contains(response, "Mail/Inbox/2023") {
		t.Errorf("Expected Mail/Inbox/2023 in response")
	}
	// Should NOT match Mail/Sent
	if strings.Contains(response, "Mail/Sent") {
		t.Errorf("Should not match Mail/Sent with Mail/Inbox/ reference")
	}

	if !strings.Contains(response, "A001 OK LIST completed") {
		t.Errorf("Expected OK completion")
	}
}
