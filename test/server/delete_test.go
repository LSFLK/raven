package server_test

import (
	"strings"
	"testing"

	"raven/internal/models"
	"raven/test/helpers"
)

// TestDeleteCommand_Unauthenticated tests DELETE command without authentication
func TestDeleteCommand_Unauthenticated(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: false,
	}

	// Test DELETE command without authentication
	server.HandleDelete(conn, "A001", []string{"A001", "DELETE", "TestFolder"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A001 NO Please authenticate first") {
		t.Errorf("Expected authentication required error, got: %s", response)
	}
}

// TestDeleteCommand_InvalidArguments tests DELETE command with invalid arguments
func TestDeleteCommand_InvalidArguments(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := helpers.SetupAuthenticatedState(t, server, "testuser")

	// Test DELETE command without mailbox name
	server.HandleDelete(conn, "A001", []string{"A001", "DELETE"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A001 BAD DELETE requires mailbox name") {
		t.Errorf("Expected BAD response for missing mailbox name, got: %s", response)
	}
}

// TestDeleteCommand_DeleteINBOX tests attempting to delete INBOX
func TestDeleteCommand_DeleteINBOX(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := helpers.SetupAuthenticatedState(t, server, "testuser")

	// Test deleting INBOX (should fail)
	server.HandleDelete(conn, "A001", []string{"A001", "DELETE", "INBOX"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A001 NO Cannot delete INBOX") {
		t.Errorf("Expected INBOX deletion error, got: %s", response)
	}
}

// TestDeleteCommand_DeleteINBOXCaseInsensitive tests attempting to delete inbox with different case
func TestDeleteCommand_DeleteINBOXCaseInsensitive(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := helpers.SetupAuthenticatedState(t, server, "testuser")

	// Test deleting inbox (lowercase - should fail)
	server.HandleDelete(conn, "A001", []string{"A001", "DELETE", "inbox"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A001 NO Cannot delete INBOX") {
		t.Errorf("Expected INBOX deletion error for lowercase, got: %s", response)
	}
}

// TestDeleteCommand_EmptyMailboxName tests deleting mailbox with empty name
func TestDeleteCommand_EmptyMailboxName(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := helpers.SetupAuthenticatedState(t, server, "testuser")

	// Test deleting mailbox with empty name
	server.HandleDelete(conn, "A001", []string{"A001", "DELETE", "\"\""}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A001 BAD Invalid mailbox name") {
		t.Errorf("Expected empty name error, got: %s", response)
	}
}

// TestDeleteCommand_NonExistentMailbox tests deleting a mailbox that doesn't exist
func TestDeleteCommand_NonExistentMailbox(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := helpers.SetupAuthenticatedState(t, server, "testuser")

	// Test deleting a non-existent mailbox
	server.HandleDelete(conn, "A001", []string{"A001", "DELETE", "NonExistent"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A001 NO Mailbox does not exist") {
		t.Errorf("Expected mailbox not found error, got: %s", response)
	}
}

// TestDeleteCommand_ValidMailbox tests deleting a valid mailbox
func TestDeleteCommand_ValidMailbox(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := helpers.SetupAuthenticatedState(t, server, "testuser")

	// First create a mailbox
	server.HandleCreate(conn, "A001", []string{"A001", "CREATE", "TestDelete"}, state)
	createResponse := conn.GetWrittenData()
	if !strings.Contains(createResponse, "A001 OK CREATE completed") {
		t.Errorf("Expected successful creation, got: %s", createResponse)
	}

	// Reset connection buffer
	conn.ClearWriteBuffer()

	// Then delete it
	server.HandleDelete(conn, "A002", []string{"A002", "DELETE", "TestDelete"}, state)
	deleteResponse := conn.GetWrittenData()
	if !strings.Contains(deleteResponse, "A002 OK DELETE completed") {
		t.Errorf("Expected successful deletion, got: %s", deleteResponse)
	}
}

// TestDeleteCommand_QuotedMailboxName tests deleting mailbox with quoted name
func TestDeleteCommand_QuotedMailboxName(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := helpers.SetupAuthenticatedState(t, server, "testuser")

	// Create a mailbox with quoted name
	server.HandleCreate(conn, "A001", []string{"A001", "CREATE", "\"My Projects\""}, state)
	createResponse := conn.GetWrittenData()
	if !strings.Contains(createResponse, "A001 OK CREATE completed") {
		t.Errorf("Expected successful creation of quoted mailbox, got: %s", createResponse)
	}

	// Reset connection buffer
	conn.ClearWriteBuffer()

	// Delete the quoted mailbox
	server.HandleDelete(conn, "A002", []string{"A002", "DELETE", "\"My Projects\""}, state)
	deleteResponse := conn.GetWrittenData()
	if !strings.Contains(deleteResponse, "A002 OK DELETE completed") {
		t.Errorf("Expected successful deletion of quoted mailbox, got: %s", deleteResponse)
	}
}

// TestDeleteCommand_HierarchicalMailboxWithInferior tests deleting a mailbox that has inferior hierarchical names
func TestDeleteCommand_HierarchicalMailboxWithInferior(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := helpers.SetupAuthenticatedState(t, server, "testuser")

	// Create parent and child mailboxes
	server.HandleCreate(conn, "A001", []string{"A001", "CREATE", "Projects"}, state)
	conn.ClearWriteBuffer()
	server.HandleCreate(conn, "A002", []string{"A002", "CREATE", "Projects/Work"}, state)
	conn.ClearWriteBuffer()

	// Try to delete parent mailbox (should fail due to inferior names)
	server.HandleDelete(conn, "A003", []string{"A003", "DELETE", "Projects"}, state)
	response := conn.GetWrittenData()
	if !strings.Contains(response, "A003 NO Name \"Projects\" has inferior hierarchical names") {
		t.Errorf("Expected inferior hierarchical names error, got: %s", response)
	}
}

// TestDeleteCommand_HierarchicalMailboxChild tests deleting a child mailbox
func TestDeleteCommand_HierarchicalMailboxChild(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := helpers.SetupAuthenticatedState(t, server, "testuser")

	// Create parent and child mailboxes
	server.HandleCreate(conn, "A001", []string{"A001", "CREATE", "Projects"}, state)
	conn.ClearWriteBuffer()
	server.HandleCreate(conn, "A002", []string{"A002", "CREATE", "Projects/Work"}, state)
	conn.ClearWriteBuffer()

	// Delete child mailbox (should succeed)
	server.HandleDelete(conn, "A003", []string{"A003", "DELETE", "Projects/Work"}, state)
	response := conn.GetWrittenData()
	if !strings.Contains(response, "A003 OK DELETE completed") {
		t.Errorf("Expected successful deletion of child mailbox, got: %s", response)
	}
}

// TestDeleteCommand_RFCExample tests the examples from RFC 3501
func TestDeleteCommand_RFCExample(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := helpers.SetupAuthenticatedState(t, server, "testuser")

	// Create mailboxes as per RFC example
	server.HandleCreate(conn, "A001", []string{"A001", "CREATE", "blurdybloop"}, state)
	conn.ClearWriteBuffer()
	server.HandleCreate(conn, "A002", []string{"A002", "CREATE", "foo"}, state)
	conn.ClearWriteBuffer()
	server.HandleCreate(conn, "A003", []string{"A003", "CREATE", "foo/bar"}, state)
	conn.ClearWriteBuffer()

	// Delete blurdybloop (should succeed)
	server.HandleDelete(conn, "A683", []string{"A683", "DELETE", "blurdybloop"}, state)
	response1 := conn.GetWrittenData()
	if !strings.Contains(response1, "A683 OK DELETE completed") {
		t.Errorf("Expected successful deletion of blurdybloop, got: %s", response1)
	}

	// Reset connection buffer
	conn.ClearWriteBuffer()

	// Try to delete foo (should fail due to foo/bar existing)
	server.HandleDelete(conn, "A684", []string{"A684", "DELETE", "foo"}, state)
	response2 := conn.GetWrittenData()
	if !strings.Contains(response2, "A684 NO Name \"foo\" has inferior hierarchical names") {
		t.Errorf("Expected inferior hierarchical names error for foo, got: %s", response2)
	}

	// Reset connection buffer
	conn.ClearWriteBuffer()

	// Delete foo/bar (should succeed)
	server.HandleDelete(conn, "A685", []string{"A685", "DELETE", "foo/bar"}, state)
	response3 := conn.GetWrittenData()
	if !strings.Contains(response3, "A685 OK DELETE completed") {
		t.Errorf("Expected successful deletion of foo/bar, got: %s", response3)
	}

	// Reset connection buffer
	conn.ClearWriteBuffer()

	// Now delete foo (should succeed since no more inferior names)
	server.HandleDelete(conn, "A687", []string{"A687", "DELETE", "foo"}, state)
	response4 := conn.GetWrittenData()
	if !strings.Contains(response4, "A687 OK DELETE completed") {
		t.Errorf("Expected successful deletion of foo after removing inferiors, got: %s", response4)
	}
}

// TestDeleteCommand_MultipleUsers tests that deletion is user-specific
func TestDeleteCommand_MultipleUsers(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()

	// User 1 creates and deletes a mailbox
	state1 := helpers.SetupAuthenticatedState(t, server, "user1")

	server.HandleCreate(conn, "A001", []string{"A001", "CREATE", "UserSpecific"}, state1)
	conn.ClearWriteBuffer()
	server.HandleDelete(conn, "A002", []string{"A002", "DELETE", "UserSpecific"}, state1)
	response1 := conn.GetWrittenData()
	if !strings.Contains(response1, "A002 OK DELETE completed") {
		t.Errorf("Expected successful deletion for user1, got: %s", response1)
	}

	// Reset connection buffer
	conn.ClearWriteBuffer()

	// User 2 should not be affected - trying to delete same named mailbox should fail
	state2 := helpers.SetupAuthenticatedState(t, server, "user2")

	server.HandleDelete(conn, "A003", []string{"A003", "DELETE", "UserSpecific"}, state2)
	response2 := conn.GetWrittenData()
	if !strings.Contains(response2, "A003 NO Mailbox does not exist") {
		t.Errorf("Expected mailbox not found for user2, got: %s", response2)
	}
}

// TestDeleteCommand_ListVerification tests that deleted mailbox no longer appears in LIST
func TestDeleteCommand_ListVerification(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := helpers.SetupAuthenticatedState(t, server, "testuser")

	// Create a mailbox
	server.HandleCreate(conn, "A001", []string{"A001", "CREATE", "ToBeDeleted"}, state)
	conn.ClearWriteBuffer()

	// Verify it appears in LIST
	server.HandleList(conn, "A002", []string{"A002", "LIST", "", "*"}, state)
	listResponse1 := conn.GetWrittenData()
	if !strings.Contains(listResponse1, "ToBeDeleted") {
		t.Errorf("Expected LIST to show created mailbox, got: %s", listResponse1)
	}

	// Reset connection buffer
	conn.ClearWriteBuffer()

	// Delete the mailbox
	server.HandleDelete(conn, "A003", []string{"A003", "DELETE", "ToBeDeleted"}, state)
	deleteResponse := conn.GetWrittenData()
	if !strings.Contains(deleteResponse, "A003 OK DELETE completed") {
		t.Errorf("Expected successful deletion, got: %s", deleteResponse)
	}

	// Reset connection buffer
	conn.ClearWriteBuffer()

	// Verify it no longer appears in LIST
	server.HandleList(conn, "A004", []string{"A004", "LIST", "", "*"}, state)
	listResponse2 := conn.GetWrittenData()
	if strings.Contains(listResponse2, "ToBeDeleted") {
		t.Errorf("Expected LIST to not show deleted mailbox, got: %s", listResponse2)
	}
}

// TestDeleteCommand_DefaultMailboxes tests attempting to delete default mailboxes
func TestDeleteCommand_DefaultMailboxes(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := helpers.SetupAuthenticatedState(t, server, "testuser")

	// Default mailboxes that should not be deletable (except INBOX which has special handling)
	defaultMailboxes := []string{"Sent", "Drafts", "Trash"}

	for _, mailbox := range defaultMailboxes {
		conn.ClearWriteBuffer()
		server.HandleDelete(conn, "A001", []string{"A001", "DELETE", mailbox}, state)
		response := conn.GetWrittenData()
		
		// These are system mailboxes, they should be protected from deletion
		if !strings.Contains(response, "A001 NO") {
			t.Errorf("Expected NO response (protected default mailbox) for %s, got: %s", mailbox, response)
		}
	}
}
