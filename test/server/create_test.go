package server_test

import (
	"strings"
	"testing"

	"go-imap/internal/models"
	"go-imap/test/helpers"
)

// TestCreateCommand_Unauthenticated tests CREATE command without authentication
func TestCreateCommand_Unauthenticated(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: false,
	}

	// Test CREATE command without authentication
	server.HandleCreate(conn, "A001", []string{"A001", "CREATE", "TestFolder"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A001 NO Please authenticate first") {
		t.Errorf("Expected authentication required error, got: %s", response)
	}
}

// TestCreateCommand_InvalidArguments tests CREATE command with invalid arguments
func TestCreateCommand_InvalidArguments(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	// Test CREATE command without mailbox name
	server.HandleCreate(conn, "A001", []string{"A001", "CREATE"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A001 BAD CREATE requires mailbox name") {
		t.Errorf("Expected BAD response for missing mailbox name, got: %s", response)
	}
}

// TestCreateCommand_CreateINBOX tests attempting to create INBOX
func TestCreateCommand_CreateINBOX(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	// Test creating INBOX (should fail)
	server.HandleCreate(conn, "A001", []string{"A001", "CREATE", "INBOX"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A001 NO Cannot create INBOX - it already exists") {
		t.Errorf("Expected INBOX creation error, got: %s", response)
	}
}

// TestCreateCommand_CreateINBOXCaseInsensitive tests attempting to create inbox with different case
func TestCreateCommand_CreateINBOXCaseInsensitive(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	// Test creating inbox (lowercase - should fail)
	server.HandleCreate(conn, "A001", []string{"A001", "CREATE", "inbox"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A001 NO Cannot create INBOX - it already exists") {
		t.Errorf("Expected INBOX creation error for lowercase, got: %s", response)
	}
}

// TestCreateCommand_EmptyMailboxName tests creating mailbox with empty name
func TestCreateCommand_EmptyMailboxName(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	// Test creating mailbox with empty name
	server.HandleCreate(conn, "A001", []string{"A001", "CREATE", "\"\""}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A001 NO Cannot create mailbox with empty name") {
		t.Errorf("Expected empty name error, got: %s", response)
	}
}

// TestCreateCommand_ValidMailbox tests creating a valid mailbox
func TestCreateCommand_ValidMailbox(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	// Test creating a valid mailbox
	server.HandleCreate(conn, "A001", []string{"A001", "CREATE", "Projects"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A001 OK CREATE completed") {
		t.Errorf("Expected successful creation, got: %s", response)
	}
}

// TestCreateCommand_QuotedMailboxName tests creating mailbox with quoted name
func TestCreateCommand_QuotedMailboxName(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	// Test creating a quoted mailbox name
	server.HandleCreate(conn, "A001", []string{"A001", "CREATE", "\"My Projects\""}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A001 OK CREATE completed") {
		t.Errorf("Expected successful creation of quoted mailbox, got: %s", response)
	}
}

// TestCreateCommand_DuplicateMailbox tests creating the same mailbox twice
func TestCreateCommand_DuplicateMailbox(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	// Create mailbox first time
	server.HandleCreate(conn, "A001", []string{"A001", "CREATE", "TestDupe"}, state)
	response1 := conn.GetWrittenData()
	if !strings.Contains(response1, "A001 OK CREATE completed") {
		t.Errorf("Expected successful first creation, got: %s", response1)
	}

	// Reset connection buffer
	conn.ClearWriteBuffer()

	// Try to create the same mailbox again
	server.HandleCreate(conn, "A002", []string{"A002", "CREATE", "TestDupe"}, state)
	response2 := conn.GetWrittenData()
	if !strings.Contains(response2, "A002 NO Mailbox already exists") {
		t.Errorf("Expected mailbox already exists error, got: %s", response2)
	}
}

// TestCreateCommand_DefaultMailboxes tests attempting to create default mailboxes
func TestCreateCommand_DefaultMailboxes(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	defaultMailboxes := []string{"Sent", "Drafts", "Trash"}

	for _, mailbox := range defaultMailboxes {
		conn.ClearWriteBuffer()
		server.HandleCreate(conn, "A001", []string{"A001", "CREATE", mailbox}, state)
		response := conn.GetWrittenData()
		if !strings.Contains(response, "A001 NO Mailbox already exists") {
			t.Errorf("Expected error creating default mailbox %s, got: %s", mailbox, response)
		}
	}
}

// TestCreateCommand_HierarchicalMailbox tests creating hierarchical mailboxes
func TestCreateCommand_HierarchicalMailbox(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	// Test creating hierarchical mailbox
	server.HandleCreate(conn, "A001", []string{"A001", "CREATE", "Projects/Work/Important"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A001 OK CREATE completed") {
		t.Errorf("Expected successful creation of hierarchical mailbox, got: %s", response)
	}
}

// TestCreateCommand_TrailingHierarchySeparator tests creating mailbox with trailing separator
func TestCreateCommand_TrailingHierarchySeparator(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	// Test creating mailbox with trailing hierarchy separator
	server.HandleCreate(conn, "A001", []string{"A001", "CREATE", "Projects/"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "A001 OK CREATE completed") {
		t.Errorf("Expected successful creation with trailing separator, got: %s", response)
	}

	// Verify that the mailbox was created without the trailing separator
	// by trying to create it again without the separator
	conn.ClearWriteBuffer()
	server.HandleCreate(conn, "A002", []string{"A002", "CREATE", "Projects"}, state)
	response2 := conn.GetWrittenData()
	if !strings.Contains(response2, "A002 NO Mailbox already exists") {
		t.Errorf("Expected mailbox already exists (trailing separator should be removed), got: %s", response2)
	}
}

// TestCreateCommand_ListShowsCreatedMailbox tests that LIST shows newly created mailboxes
func TestCreateCommand_ListShowsCreatedMailbox(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	// Create a new mailbox
	server.HandleCreate(conn, "A001", []string{"A001", "CREATE", "NewMailbox"}, state)
	createResponse := conn.GetWrittenData()
	if !strings.Contains(createResponse, "A001 OK CREATE completed") {
		t.Errorf("Expected successful creation, got: %s", createResponse)
	}

	// Reset connection buffer
	conn.ClearWriteBuffer()

	// List mailboxes
	server.HandleList(conn, "A002", []string{"A002", "LIST", "", "*"}, state)
	listResponse := conn.GetWrittenData()
	
	// Check that the new mailbox appears in the LIST response
	if !strings.Contains(listResponse, "NewMailbox") {
		t.Errorf("Expected LIST to show created mailbox 'NewMailbox', got: %s", listResponse)
	}
	
	if !strings.Contains(listResponse, "A002 OK LIST completed") {
		t.Errorf("Expected successful LIST completion, got: %s", listResponse)
	}
}

// TestCreateCommand_MultipleUsers tests that mailboxes are user-specific
func TestCreateCommand_MultipleUsers(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	
	// User 1 creates a mailbox
	state1 := &models.ClientState{
		Authenticated: true,
		Username:      "user1",
	}
	
	server.HandleCreate(conn, "A001", []string{"A001", "CREATE", "User1Mailbox"}, state1)
	response1 := conn.GetWrittenData()
	if !strings.Contains(response1, "A001 OK CREATE completed") {
		t.Errorf("Expected successful creation for user1, got: %s", response1)
	}

	// Reset connection buffer
	conn.ClearWriteBuffer()

	// User 2 should be able to create a mailbox with the same name
	state2 := &models.ClientState{
		Authenticated: true,
		Username:      "user2",
	}
	
	server.HandleCreate(conn, "A002", []string{"A002", "CREATE", "User1Mailbox"}, state2)
	response2 := conn.GetWrittenData()
	if !strings.Contains(response2, "A002 OK CREATE completed") {
		t.Errorf("Expected successful creation for user2 with same mailbox name, got: %s", response2)
	}
}

// TestCreateCommand_RFCExample tests the example from RFC 3501
func TestCreateCommand_RFCExample(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	// Test the RFC 3501 example: CREATE owatagusiam/
	server.HandleCreate(conn, "A003", []string{"A003", "CREATE", "owatagusiam/"}, state)
	response1 := conn.GetWrittenData()
	if !strings.Contains(response1, "A003 OK CREATE completed") {
		t.Errorf("Expected successful creation of owatagusiam/, got: %s", response1)
	}

	// Reset connection buffer
	conn.ClearWriteBuffer()

	// Test the RFC 3501 example: CREATE owatagusiam/blurdybloop
	server.HandleCreate(conn, "A004", []string{"A004", "CREATE", "owatagusiam/blurdybloop"}, state)
	response2 := conn.GetWrittenData()
	if !strings.Contains(response2, "A004 OK CREATE completed") {
		t.Errorf("Expected successful creation of owatagusiam/blurdybloop, got: %s", response2)
	}
}
