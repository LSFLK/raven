package mailbox_test

import (
	"strings"
	"testing"

	"raven/internal/models"
	"raven/internal/server"
)

// TestRenameCommand_Unauthenticated tests RENAME command without authentication
func TestRenameCommand_Unauthenticated(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := &models.ClientState{
		Authenticated: false,
	}

	// Test RENAME command without authentication
	srv.HandleRename(conn, "A001", []string{"A001", "RENAME", "oldbox", "newbox"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A001 NO Please authenticate first") {
		t.Errorf("Expected authentication required error, got: %s", response)
	}
}

// TestRenameCommand_InvalidArguments tests RENAME command with invalid arguments
func TestRenameCommand_InvalidArguments(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Test RENAME command without enough arguments
	srv.HandleRename(conn, "A001", []string{"A001", "RENAME", "oldbox"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A001 BAD RENAME requires existing and new mailbox names") {
		t.Errorf("Expected BAD response for missing arguments, got: %s", response)
	}
}

// TestRenameCommand_EmptyMailboxNames tests RENAME command with empty mailbox names
func TestRenameCommand_EmptyMailboxNames(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Test RENAME command with empty old name
	srv.HandleRename(conn, "A001", []string{"A001", "RENAME", "\"\"", "newbox"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A001 BAD Invalid mailbox names") {
		t.Errorf("Expected BAD response for empty old name, got: %s", response)
	}

	// Clear buffer and test empty new name
	conn.ClearWriteBuffer()
	srv.HandleRename(conn, "A002", []string{"A002", "RENAME", "oldbox", "\"\""}, state)

	response = conn.GetWrittenData()
	if !strings.Contains(response, "A002 BAD Invalid mailbox names") {
		t.Errorf("Expected BAD response for empty new name, got: %s", response)
	}
}

// TestRenameCommand_NonExistentMailbox tests renaming a non-existent mailbox
func TestRenameCommand_NonExistentMailbox(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Test renaming a non-existent mailbox
	srv.HandleRename(conn, "A001", []string{"A001", "RENAME", "NonExistent", "NewName"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A001 NO Source mailbox does not exist") {
		t.Errorf("Expected source not found error, got: %s", response)
	}
}

// TestRenameCommand_RenameToExistingMailbox tests renaming to an existing mailbox
func TestRenameCommand_RenameToExistingMailbox(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Create source and destination mailboxes
	srv.HandleCreate(conn, "A001", []string{"A001", "CREATE", "SourceBox"}, state)
	srv.HandleCreate(conn, "A002", []string{"A002", "CREATE", "DestBox"}, state)

	// Clear buffer
	conn.ClearWriteBuffer()

	// Try to rename to existing mailbox
	srv.HandleRename(conn, "A003", []string{"A003", "RENAME", "SourceBox", "DestBox"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A003 NO Destination mailbox already exists") {
		t.Errorf("Expected destination exists error, got: %s", response)
	}
}

// TestRenameCommand_RenameToINBOX tests renaming to INBOX
func TestRenameCommand_RenameToINBOX(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Create a source mailbox
	srv.HandleCreate(conn, "A001", []string{"A001", "CREATE", "SourceBox"}, state)

	// Clear buffer
	conn.ClearWriteBuffer()

	// Try to rename to INBOX
	srv.HandleRename(conn, "A002", []string{"A002", "RENAME", "SourceBox", "INBOX"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A002 NO Cannot rename to INBOX") {
		t.Errorf("Expected cannot rename to INBOX error, got: %s", response)
	}
}

// TestRenameCommand_RenameINBOX tests renaming INBOX (special case)
func TestRenameCommand_RenameINBOX(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Rename INBOX to a new name
	srv.HandleRename(conn, "A001", []string{"A001", "RENAME", "INBOX", "old-mail"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A001 OK RENAME completed") {
		t.Errorf("Expected successful INBOX rename, got: %s", response)
	}

	// Verify the new mailbox exists
	conn.ClearWriteBuffer()
	srv.HandleList(conn, "A002", []string{"A002", "LIST", "\"\"", "*"}, state)

	response = conn.GetWrittenData()
	if !strings.Contains(response, "old-mail") {
		t.Errorf("Expected new mailbox to exist after INBOX rename, got: %s", response)
	}

	// INBOX should still exist (empty)
	if !strings.Contains(response, "INBOX") {
		t.Errorf("Expected INBOX to still exist after rename, got: %s", response)
	}
}

// TestRenameCommand_ValidRename tests successful mailbox renaming
func TestRenameCommand_ValidRename(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Create a source mailbox
	srv.HandleCreate(conn, "A001", []string{"A001", "CREATE", "OldName"}, state)

	// Clear buffer
	conn.ClearWriteBuffer()

	// Rename the mailbox
	srv.HandleRename(conn, "A002", []string{"A002", "RENAME", "OldName", "NewName"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A002 OK RENAME completed") {
		t.Errorf("Expected successful rename, got: %s", response)
	}

	// Verify the rename by listing mailboxes
	conn.ClearWriteBuffer()
	srv.HandleList(conn, "A003", []string{"A003", "LIST", "\"\"", "*"}, state)

	response = conn.GetWrittenData()
	if !strings.Contains(response, "NewName") {
		t.Errorf("Expected new mailbox to exist after rename, got: %s", response)
	}

	if strings.Contains(response, "\"OldName\"") {
		t.Errorf("Expected old mailbox to not exist after rename, got: %s", response)
	}
}

// TestRenameCommand_HierarchicalRename tests renaming with hierarchical names
func TestRenameCommand_HierarchicalRename(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Create hierarchical mailboxes
	srv.HandleCreate(conn, "A001", []string{"A001", "CREATE", "foo"}, state)
	srv.HandleCreate(conn, "A002", []string{"A002", "CREATE", "foo/bar"}, state)
	srv.HandleCreate(conn, "A003", []string{"A003", "CREATE", "foo/baz"}, state)

	// Clear buffer
	conn.ClearWriteBuffer()

	// Rename the parent mailbox
	srv.HandleRename(conn, "A004", []string{"A004", "RENAME", "foo", "zap"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A004 OK RENAME completed") {
		t.Errorf("Expected successful hierarchical rename, got: %s", response)
	}

	// Verify the rename by listing mailboxes
	conn.ClearWriteBuffer()
	srv.HandleList(conn, "A005", []string{"A005", "LIST", "\"\"", "*"}, state)

	response = conn.GetWrittenData()

	// Should have new names
	if !strings.Contains(response, "zap") {
		t.Errorf("Expected 'zap' to exist after rename, got: %s", response)
	}
	if !strings.Contains(response, "zap/bar") {
		t.Errorf("Expected 'zap/bar' to exist after rename, got: %s", response)
	}
	if !strings.Contains(response, "zap/baz") {
		t.Errorf("Expected 'zap/baz' to exist after rename, got: %s", response)
	}

	// Should not have old names
	if strings.Contains(response, "\"foo\"") && !strings.Contains(response, "\"zap\"") {
		t.Errorf("Expected old 'foo' to not exist after rename, got: %s", response)
	}
	if strings.Contains(response, "foo/bar") && !strings.Contains(response, "zap/bar") {
		t.Errorf("Expected old 'foo/bar' to not exist after rename, got: %s", response)
	}
}

// TestRenameCommand_CreateSuperiorHierarchy tests creating superior hierarchy during rename
func TestRenameCommand_CreateSuperiorHierarchy(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Create a simple mailbox
	srv.HandleCreate(conn, "A001", []string{"A001", "CREATE", "simple"}, state)

	// Clear buffer
	conn.ClearWriteBuffer()

	// Rename to a hierarchical name that requires creating superior hierarchy
	srv.HandleRename(conn, "A002", []string{"A002", "RENAME", "simple", "baz/rag/zowie"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A002 OK RENAME completed") {
		t.Errorf("Expected successful rename with hierarchy creation, got: %s", response)
	}

	// Verify the hierarchy was created
	conn.ClearWriteBuffer()
	srv.HandleList(conn, "A003", []string{"A003", "LIST", "\"\"", "*"}, state)

	response = conn.GetWrittenData()

	// Should have the target mailbox
	if !strings.Contains(response, "baz/rag/zowie") {
		t.Errorf("Expected 'baz/rag/zowie' to exist after rename, got: %s", response)
	}

	// Should have superior hierarchy mailboxes
	if !strings.Contains(response, "\"baz\"") {
		t.Errorf("Expected 'baz' to exist as superior hierarchy, got: %s", response)
	}
	if !strings.Contains(response, "baz/rag") {
		t.Errorf("Expected 'baz/rag' to exist as superior hierarchy, got: %s", response)
	}

	// Should not have the old mailbox
	if strings.Contains(response, "\"simple\"") && !strings.Contains(response, "baz/rag/zowie") {
		t.Errorf("Expected old 'simple' to not exist after rename, got: %s", response)
	}
}

// TestRenameCommand_QuotedNames tests renaming with quoted mailbox names
func TestRenameCommand_QuotedNames(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Create a mailbox with quoted name
	srv.HandleCreate(conn, "A001", []string{"A001", "CREATE", "\"My Old Box\""}, state)

	// Clear buffer
	conn.ClearWriteBuffer()

	// Rename using quoted names
	srv.HandleRename(conn, "A002", []string{"A002", "RENAME", "\"My Old Box\"", "\"My New Box\""}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A002 OK RENAME completed") {
		t.Errorf("Expected successful rename with quoted names, got: %s", response)
	}

	// Verify the rename
	conn.ClearWriteBuffer()
	srv.HandleList(conn, "A003", []string{"A003", "LIST", "\"\"", "*"}, state)

	response = conn.GetWrittenData()
	if !strings.Contains(response, "My New Box") {
		t.Errorf("Expected 'My New Box' to exist after rename, got: %s", response)
	}
}

// TestRenameCommand_CaseInsensitiveINBOX tests INBOX renaming is case-insensitive
func TestRenameCommand_CaseInsensitiveINBOX(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Rename inbox (lowercase) to a new name
	srv.HandleRename(conn, "A001", []string{"A001", "RENAME", "inbox", "old-inbox"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A001 OK RENAME completed") {
		t.Errorf("Expected successful INBOX rename (case-insensitive), got: %s", response)
	}

	// Verify the new mailbox exists
	conn.ClearWriteBuffer()
	srv.HandleList(conn, "A002", []string{"A002", "LIST", "\"\"", "*"}, state)

	response = conn.GetWrittenData()
	if !strings.Contains(response, "old-inbox") {
		t.Errorf("Expected new mailbox to exist after INBOX rename, got: %s", response)
	}
}

// TestRenameCommand_Multiple tests multiple rename operations
func TestRenameCommand_Multiple(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Create multiple mailboxes
	srv.HandleCreate(conn, "A001", []string{"A001", "CREATE", "Box1"}, state)
	srv.HandleCreate(conn, "A002", []string{"A002", "CREATE", "Box2"}, state)
	srv.HandleCreate(conn, "A003", []string{"A003", "CREATE", "Box3"}, state)

	// Clear buffer
	conn.ClearWriteBuffer()

	// Rename them in sequence
	srv.HandleRename(conn, "A004", []string{"A004", "RENAME", "Box1", "NewBox1"}, state)
	srv.HandleRename(conn, "A005", []string{"A005", "RENAME", "Box2", "NewBox2"}, state)
	srv.HandleRename(conn, "A006", []string{"A006", "RENAME", "Box3", "NewBox3"}, state)

	response := conn.GetWrittenData()

	// All operations should succeed
	expectedResponses := []string{
		"A004 OK RENAME completed",
		"A005 OK RENAME completed",
		"A006 OK RENAME completed",
	}

	for _, expected := range expectedResponses {
		if !strings.Contains(response, expected) {
			t.Errorf("Expected '%s' in response, got: %s", expected, response)
		}
	}

	// Verify all renames worked
	conn.ClearWriteBuffer()
	srv.HandleList(conn, "A007", []string{"A007", "LIST", "\"\"", "*"}, state)

	response = conn.GetWrittenData()
	newBoxes := []string{"NewBox1", "NewBox2", "NewBox3"}
	for _, box := range newBoxes {
		if !strings.Contains(response, box) {
			t.Errorf("Expected '%s' to exist after rename, got: %s", box, response)
		}
	}
}

// TestRenameCommand_ErrorRecovery tests error recovery after failed renames
func TestRenameCommand_ErrorRecovery(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Create a mailbox
	srv.HandleCreate(conn, "A001", []string{"A001", "CREATE", "TestBox"}, state)

	// Clear buffer
	conn.ClearWriteBuffer()

	// Try to rename non-existent mailbox (should fail)
	srv.HandleRename(conn, "A002", []string{"A002", "RENAME", "NonExistent", "NewName"}, state)

	// Try to rename to existing name (should fail)
	srv.HandleRename(conn, "A003", []string{"A003", "RENAME", "TestBox", "INBOX"}, state)

	// Try valid rename (should succeed)
	srv.HandleRename(conn, "A004", []string{"A004", "RENAME", "TestBox", "ValidName"}, state)

	response := conn.GetWrittenData()

	// Check that errors were reported correctly and valid operation succeeded
	if !strings.Contains(response, "A002 NO Source mailbox does not exist") {
		t.Errorf("Expected source not found error for A002, got: %s", response)
	}

	if !strings.Contains(response, "A003 NO Cannot rename to INBOX") {
		t.Errorf("Expected cannot rename to INBOX error for A003, got: %s", response)
	}

	if !strings.Contains(response, "A004 OK RENAME completed") {
		t.Errorf("Expected successful rename for A004, got: %s", response)
	}
}
