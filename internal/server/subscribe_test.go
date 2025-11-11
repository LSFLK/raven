package server

import (
	"strings"
	"testing"

	"raven/internal/models"
	
)

// TestSubscribeCommand_ValidMailbox tests SUBSCRIBE command with valid mailbox
func TestSubscribeCommand_ValidMailbox(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	state := SetupAuthenticatedState(t, srv, "testuser")

	// Test SUBSCRIBE command
	srv.HandleSubscribe(conn, "A001", []string{"A001", "SUBSCRIBE", "TestFolder"}, state)

	response := conn.GetWrittenData()
	expectedResponse := "A001 OK SUBSCRIBE completed"

	if !strings.Contains(response, expectedResponse) {
		t.Errorf("Expected response to contain '%s', got: %s", expectedResponse, response)
	}
}

// TestSubscribeCommand_QuotedMailbox tests SUBSCRIBE command with quoted mailbox name
func TestSubscribeCommand_QuotedMailbox(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	state := SetupAuthenticatedState(t, srv, "testuser")

	// Test SUBSCRIBE command with quoted mailbox
	srv.HandleSubscribe(conn, "A002", []string{"A002", "SUBSCRIBE", "\"Test Folder\""}, state)

	response := conn.GetWrittenData()
	expectedResponse := "A002 OK SUBSCRIBE completed"

	if !strings.Contains(response, expectedResponse) {
		t.Errorf("Expected response to contain '%s', got: %s", expectedResponse, response)
	}
}

// TestSubscribeCommand_INBOXMailbox tests SUBSCRIBE command with INBOX
func TestSubscribeCommand_INBOXMailbox(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	state := SetupAuthenticatedState(t, srv, "testuser")

	// Test SUBSCRIBE command with INBOX
	srv.HandleSubscribe(conn, "A003", []string{"A003", "SUBSCRIBE", "INBOX"}, state)

	response := conn.GetWrittenData()
	expectedResponse := "A003 OK SUBSCRIBE completed"

	if !strings.Contains(response, expectedResponse) {
		t.Errorf("Expected response to contain '%s', got: %s", expectedResponse, response)
	}
}

// TestSubscribeCommand_NotAuthenticated tests SUBSCRIBE command without authentication
func TestSubscribeCommand_NotAuthenticated(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	state := &models.ClientState{
		Authenticated: false,
	}

	// Test SUBSCRIBE command without authentication
	srv.HandleSubscribe(conn, "A004", []string{"A004", "SUBSCRIBE", "TestFolder"}, state)

	response := conn.GetWrittenData()
	expectedResponse := "A004 NO Please authenticate first"

	if !strings.Contains(response, expectedResponse) {
		t.Errorf("Expected response to contain '%s', got: %s", expectedResponse, response)
	}
}

// TestSubscribeCommand_MissingArgument tests SUBSCRIBE command without mailbox argument
func TestSubscribeCommand_MissingArgument(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	state := SetupAuthenticatedState(t, srv, "testuser")

	// Test SUBSCRIBE command without mailbox argument
	srv.HandleSubscribe(conn, "A005", []string{"A005", "SUBSCRIBE"}, state)

	response := conn.GetWrittenData()
	expectedResponse := "A005 BAD SUBSCRIBE command requires a mailbox argument"

	if !strings.Contains(response, expectedResponse) {
		t.Errorf("Expected response to contain '%s', got: %s", expectedResponse, response)
	}
}

// TestSubscribeCommand_EmptyMailboxName tests SUBSCRIBE command with empty mailbox name
func TestSubscribeCommand_EmptyMailboxName(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	state := SetupAuthenticatedState(t, srv, "testuser")

	// Test SUBSCRIBE command with empty mailbox name
	srv.HandleSubscribe(conn, "A006", []string{"A006", "SUBSCRIBE", ""}, state)

	response := conn.GetWrittenData()
	expectedResponse := "A006 BAD Invalid mailbox name"

	if !strings.Contains(response, expectedResponse) {
		t.Errorf("Expected response to contain '%s', got: %s", expectedResponse, response)
	}
}

// TestUnsubscribeCommand_ValidMailbox tests UNSUBSCRIBE command with valid mailbox
func TestUnsubscribeCommand_ValidMailbox(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	state := SetupAuthenticatedState(t, srv, "testuser")

	// First subscribe to a mailbox
	srv.HandleSubscribe(conn, "A001", []string{"A001", "SUBSCRIBE", "TestFolder"}, state)
	conn.ClearWriteBuffer() // Clear previous response

	// Test UNSUBSCRIBE command
	srv.HandleUnsubscribe(conn, "A002", []string{"A002", "UNSUBSCRIBE", "TestFolder"}, state)

	response := conn.GetWrittenData()
	expectedResponse := "A002 OK UNSUBSCRIBE completed"

	if !strings.Contains(response, expectedResponse) {
		t.Errorf("Expected response to contain '%s', got: %s", expectedResponse, response)
	}
}

// TestUnsubscribeCommand_NotAuthenticated tests UNSUBSCRIBE command without authentication
func TestUnsubscribeCommand_NotAuthenticated(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	state := &models.ClientState{
		Authenticated: false,
	}

	// Test UNSUBSCRIBE command without authentication
	srv.HandleUnsubscribe(conn, "A003", []string{"A003", "UNSUBSCRIBE", "TestFolder"}, state)

	response := conn.GetWrittenData()
	expectedResponse := "A003 NO Please authenticate first"

	if !strings.Contains(response, expectedResponse) {
		t.Errorf("Expected response to contain '%s', got: %s", expectedResponse, response)
	}
}

// TestUnsubscribeCommand_MissingArgument tests UNSUBSCRIBE command without mailbox argument
func TestUnsubscribeCommand_MissingArgument(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	state := SetupAuthenticatedState(t, srv, "testuser")

	// Test UNSUBSCRIBE command without mailbox argument
	srv.HandleUnsubscribe(conn, "A004", []string{"A004", "UNSUBSCRIBE"}, state)

	response := conn.GetWrittenData()
	expectedResponse := "A004 BAD UNSUBSCRIBE requires mailbox name"

	if !strings.Contains(response, expectedResponse) {
		t.Errorf("Expected response to contain '%s', got: %s", expectedResponse, response)
	}
}

// TestUnsubscribeCommand_EmptyMailboxName tests UNSUBSCRIBE command with empty mailbox name
func TestUnsubscribeCommand_EmptyMailboxName(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	state := SetupAuthenticatedState(t, srv, "testuser")

	// Test UNSUBSCRIBE command with empty mailbox name
	srv.HandleUnsubscribe(conn, "A005", []string{"A005", "UNSUBSCRIBE", ""}, state)

	response := conn.GetWrittenData()
	expectedResponse := "A005 BAD Invalid mailbox name"

	if !strings.Contains(response, expectedResponse) {
		t.Errorf("Expected response to contain '%s', got: %s", expectedResponse, response)
	}
}

// TestUnsubscribeCommand_QuotedMailbox tests UNSUBSCRIBE command with quoted mailbox name
func TestUnsubscribeCommand_QuotedMailbox(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	state := SetupAuthenticatedState(t, srv, "testuser")

	// First subscribe to a quoted mailbox
	srv.HandleSubscribe(conn, "A001", []string{"A001", "SUBSCRIBE", "\"Test Folder\""}, state)
	conn.ClearWriteBuffer() // Clear previous response

	// Test UNSUBSCRIBE command with quoted mailbox
	srv.HandleUnsubscribe(conn, "A006", []string{"A006", "UNSUBSCRIBE", "\"Test Folder\""}, state)

	response := conn.GetWrittenData()
	expectedResponse := "A006 OK UNSUBSCRIBE completed"

	if !strings.Contains(response, expectedResponse) {
		t.Errorf("Expected response to contain '%s', got: %s", expectedResponse, response)
	}
}

// TestUnsubscribeCommand_NonSubscribed tests UNSUBSCRIBE command with mailbox that was never subscribed
func TestUnsubscribeCommand_NonSubscribed(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	state := SetupAuthenticatedState(t, srv, "testuser")

	// Test UNSUBSCRIBE command on a mailbox that was never subscribed
	srv.HandleUnsubscribe(conn, "A007", []string{"A007", "UNSUBSCRIBE", "NonExistentFolder"}, state)

	response := conn.GetWrittenData()
	expectedResponse := "A007 NO UNSUBSCRIBE failure: can't unsubscribe that name"

	if !strings.Contains(response, expectedResponse) {
		t.Errorf("Expected response to contain '%s', got: %s", expectedResponse, response)
	}
}

// TestUnsubscribeCommand_ExampleFromRFC tests the exact example from RFC 3501
func TestUnsubscribeCommand_ExampleFromRFC(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	state := SetupAuthenticatedState(t, srv, "testuser")

	// First subscribe to the RFC example mailbox
	srv.HandleSubscribe(conn, "A001", []string{"A001", "SUBSCRIBE", "#news.comp.mail.mime"}, state)
	conn.ClearWriteBuffer() // Clear previous response

	// Test the RFC example: C: A002 UNSUBSCRIBE #news.comp.mail.mime
	// Expected: S: A002 OK UNSUBSCRIBE completed
	srv.HandleUnsubscribe(conn, "A002", []string{"A002", "UNSUBSCRIBE", "#news.comp.mail.mime"}, state)

	response := conn.GetWrittenData()
	expectedResponse := "A002 OK UNSUBSCRIBE completed"

	if !strings.Contains(response, expectedResponse) {
		t.Errorf("Expected response to contain '%s', got: %s", expectedResponse, response)
	}
}

// TestSubscribeCommand_ExampleFromRFC tests the exact example from RFC 3501
func TestSubscribeCommand_ExampleFromRFC(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	state := SetupAuthenticatedState(t, srv, "testuser")

	// Test the RFC example: C: A002 SUBSCRIBE #news.comp.mail.mime
	srv.HandleSubscribe(conn, "A002", []string{"A002", "SUBSCRIBE", "#news.comp.mail.mime"}, state)

	response := conn.GetWrittenData()
	expectedResponse := "A002 OK SUBSCRIBE completed"

	if !strings.Contains(response, expectedResponse) {
		t.Errorf("Expected response to contain '%s', got: %s", expectedResponse, response)
	}
}

// TestSubscribeCommand_DuplicateSubscription tests subscribing to the same mailbox twice
func TestSubscribeCommand_DuplicateSubscription(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	state := SetupAuthenticatedState(t, srv, "testuser")

	// Subscribe to a mailbox twice
	srv.HandleSubscribe(conn, "A001", []string{"A001", "SUBSCRIBE", "TestFolder"}, state)
	srv.HandleSubscribe(conn, "A002", []string{"A002", "SUBSCRIBE", "TestFolder"}, state)

	response := conn.GetWrittenData()
	
	// Both commands should succeed
	if !strings.Contains(response, "A001 OK SUBSCRIBE completed") {
		t.Errorf("First SUBSCRIBE should succeed")
	}
	if !strings.Contains(response, "A002 OK SUBSCRIBE completed") {
		t.Errorf("Duplicate SUBSCRIBE should also succeed")
	}
}
