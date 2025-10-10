package server_test

import (
	"strings"
	"testing"

	"go-imap/internal/models"
	"go-imap/test/helpers"
)

// TestSubscribeCommand_ValidMailbox tests SUBSCRIBE command with valid mailbox
func TestSubscribeCommand_ValidMailbox(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	// Test SUBSCRIBE command
	server.HandleSubscribe(conn, "A001", []string{"A001", "SUBSCRIBE", "TestFolder"}, state)

	response := conn.GetWrittenData()
	expectedResponse := "A001 OK SUBSCRIBE completed"

	if !strings.Contains(response, expectedResponse) {
		t.Errorf("Expected response to contain '%s', got: %s", expectedResponse, response)
	}
}

// TestSubscribeCommand_QuotedMailbox tests SUBSCRIBE command with quoted mailbox name
func TestSubscribeCommand_QuotedMailbox(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	// Test SUBSCRIBE command with quoted mailbox
	server.HandleSubscribe(conn, "A002", []string{"A002", "SUBSCRIBE", "\"Test Folder\""}, state)

	response := conn.GetWrittenData()
	expectedResponse := "A002 OK SUBSCRIBE completed"

	if !strings.Contains(response, expectedResponse) {
		t.Errorf("Expected response to contain '%s', got: %s", expectedResponse, response)
	}
}

// TestSubscribeCommand_INBOXMailbox tests SUBSCRIBE command with INBOX
func TestSubscribeCommand_INBOXMailbox(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	// Test SUBSCRIBE command with INBOX
	server.HandleSubscribe(conn, "A003", []string{"A003", "SUBSCRIBE", "INBOX"}, state)

	response := conn.GetWrittenData()
	expectedResponse := "A003 OK SUBSCRIBE completed"

	if !strings.Contains(response, expectedResponse) {
		t.Errorf("Expected response to contain '%s', got: %s", expectedResponse, response)
	}
}

// TestSubscribeCommand_NotAuthenticated tests SUBSCRIBE command without authentication
func TestSubscribeCommand_NotAuthenticated(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: false,
	}

	// Test SUBSCRIBE command without authentication
	server.HandleSubscribe(conn, "A004", []string{"A004", "SUBSCRIBE", "TestFolder"}, state)

	response := conn.GetWrittenData()
	expectedResponse := "A004 NO Please authenticate first"

	if !strings.Contains(response, expectedResponse) {
		t.Errorf("Expected response to contain '%s', got: %s", expectedResponse, response)
	}
}

// TestSubscribeCommand_MissingArgument tests SUBSCRIBE command without mailbox argument
func TestSubscribeCommand_MissingArgument(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	// Test SUBSCRIBE command without mailbox argument
	server.HandleSubscribe(conn, "A005", []string{"A005", "SUBSCRIBE"}, state)

	response := conn.GetWrittenData()
	expectedResponse := "A005 BAD SUBSCRIBE command requires a mailbox argument"

	if !strings.Contains(response, expectedResponse) {
		t.Errorf("Expected response to contain '%s', got: %s", expectedResponse, response)
	}
}

// TestSubscribeCommand_EmptyMailboxName tests SUBSCRIBE command with empty mailbox name
func TestSubscribeCommand_EmptyMailboxName(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	// Test SUBSCRIBE command with empty mailbox name
	server.HandleSubscribe(conn, "A006", []string{"A006", "SUBSCRIBE", ""}, state)

	response := conn.GetWrittenData()
	expectedResponse := "A006 BAD Invalid mailbox name"

	if !strings.Contains(response, expectedResponse) {
		t.Errorf("Expected response to contain '%s', got: %s", expectedResponse, response)
	}
}

// TestUnsubscribeCommand_ValidMailbox tests UNSUBSCRIBE command with valid mailbox
func TestUnsubscribeCommand_ValidMailbox(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	// First subscribe to a mailbox
	server.HandleSubscribe(conn, "A001", []string{"A001", "SUBSCRIBE", "TestFolder"}, state)
	conn.ClearWriteBuffer() // Clear previous response

	// Test UNSUBSCRIBE command
	server.HandleUnsubscribe(conn, "A002", []string{"A002", "UNSUBSCRIBE", "TestFolder"}, state)

	response := conn.GetWrittenData()
	expectedResponse := "A002 OK UNSUBSCRIBE completed"

	if !strings.Contains(response, expectedResponse) {
		t.Errorf("Expected response to contain '%s', got: %s", expectedResponse, response)
	}
}

// TestUnsubscribeCommand_NotAuthenticated tests UNSUBSCRIBE command without authentication
func TestUnsubscribeCommand_NotAuthenticated(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: false,
	}

	// Test UNSUBSCRIBE command without authentication
	server.HandleUnsubscribe(conn, "A003", []string{"A003", "UNSUBSCRIBE", "TestFolder"}, state)

	response := conn.GetWrittenData()
	expectedResponse := "A003 NO Please authenticate first"

	if !strings.Contains(response, expectedResponse) {
		t.Errorf("Expected response to contain '%s', got: %s", expectedResponse, response)
	}
}

// TestLsubCommand_WithSubscriptions tests LSUB command with actual subscriptions
func TestLsubCommand_WithSubscriptions(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	// Subscribe to some mailboxes
	server.HandleSubscribe(conn, "A001", []string{"A001", "SUBSCRIBE", "INBOX"}, state)
	server.HandleSubscribe(conn, "A002", []string{"A002", "SUBSCRIBE", "TestFolder"}, state)
	server.HandleSubscribe(conn, "A003", []string{"A003", "SUBSCRIBE", "Drafts"}, state)
	conn.ClearWriteBuffer() // Clear subscription responses

	// Test LSUB command
	server.HandleLsub(conn, "A004", []string{"A004", "LSUB", "\"\"", "*"}, state)

	response := conn.GetWrittenData()
	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Should contain subscribed mailboxes
	foundINBOX := false
	foundTestFolder := false
	foundDrafts := false
	foundCompleted := false

	for _, line := range lines {
		if strings.Contains(line, "* LSUB") {
			if strings.Contains(line, "INBOX") {
				foundINBOX = true
			}
			if strings.Contains(line, "TestFolder") {
				foundTestFolder = true
			}
			if strings.Contains(line, "Drafts") {
				foundDrafts = true
			}
		}
		if strings.Contains(line, "A004 OK LSUB completed") {
			foundCompleted = true
		}
	}

	if !foundINBOX {
		t.Errorf("Expected LSUB response to include INBOX")
	}
	if !foundTestFolder {
		t.Errorf("Expected LSUB response to include TestFolder")
	}
	if !foundDrafts {
		t.Errorf("Expected LSUB response to include Drafts with \\Drafts attribute")
	}
	if !foundCompleted {
		t.Errorf("Expected LSUB command to complete successfully")
	}
}

// TestSubscribeCommand_ExampleFromRFC tests the exact example from RFC 3501
func TestSubscribeCommand_ExampleFromRFC(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	// Test the RFC example: C: A002 SUBSCRIBE #news.comp.mail.mime
	server.HandleSubscribe(conn, "A002", []string{"A002", "SUBSCRIBE", "#news.comp.mail.mime"}, state)

	response := conn.GetWrittenData()
	expectedResponse := "A002 OK SUBSCRIBE completed"

	if !strings.Contains(response, expectedResponse) {
		t.Errorf("Expected response to contain '%s', got: %s", expectedResponse, response)
	}
}

// TestSubscribeCommand_DuplicateSubscription tests subscribing to the same mailbox twice
func TestSubscribeCommand_DuplicateSubscription(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	// Subscribe to a mailbox twice
	server.HandleSubscribe(conn, "A001", []string{"A001", "SUBSCRIBE", "TestFolder"}, state)
	server.HandleSubscribe(conn, "A002", []string{"A002", "SUBSCRIBE", "TestFolder"}, state)

	response := conn.GetWrittenData()
	
	// Both commands should succeed
	if !strings.Contains(response, "A001 OK SUBSCRIBE completed") {
		t.Errorf("First SUBSCRIBE should succeed")
	}
	if !strings.Contains(response, "A002 OK SUBSCRIBE completed") {
		t.Errorf("Duplicate SUBSCRIBE should also succeed")
	}
}
