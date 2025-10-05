//go:build test
// +build test

package server

import (
	"fmt"
	"strings"
	"testing"

	"go-imap/internal/models"
	"go-imap/test/helpers"
)

// TestAppendCommand_Basic tests basic APPEND functionality
func TestAppendCommand_Basic(t *testing.T) {
	db := helpers.CreateTestDB(t)
	helpers.CreateTestUserTable(t, db, "testuser")
	server := helpers.TestServerWithDB(db)
	conn := helpers.NewMockConn()

	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	// Simulate APPEND command with a simple message
	message := "From: sender@example.com\r\nTo: recipient@example.com\r\nSubject: Test Message\r\n\r\nThis is a test message body.\r\n"
	appendCmd := fmt.Sprintf("A001 APPEND Sent {%d}", len(message))
	
	// First, send the APPEND command with literal size
	parts := strings.Fields(appendCmd)
	fullLine := appendCmd
	
	// Simulate the client sending the command
	conn.AddReadData(message)
	
	server.HandleAppend(conn, "A001", parts, fullLine, state)

	response := conn.GetWrittenData()
	
	// Should receive continuation response first, then OK
	if !strings.Contains(response, "+ Ready for literal data") {
		t.Errorf("Expected continuation response, got: %s", response)
	}
	
	if !strings.Contains(response, "A001 OK") {
		t.Errorf("Expected OK response, got: %s", response)
	}
	
	if !strings.Contains(response, "APPENDUID") {
		t.Errorf("Expected APPENDUID in response, got: %s", response)
	}
}

// TestAppendCommand_WithFlags tests APPEND with flags
func TestAppendCommand_WithFlags(t *testing.T) {
	db := helpers.CreateTestDB(t)
	helpers.CreateTestUserTable(t, db, "testuser")
	server := helpers.TestServerWithDB(db)
	conn := helpers.NewMockConn()

	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	message := "From: sender@example.com\r\nTo: recipient@example.com\r\nSubject: Test\r\n\r\nBody\r\n"
	appendCmd := fmt.Sprintf("A002 APPEND Sent (\\Seen) {%d}", len(message))
	
	parts := strings.Fields(appendCmd)
	conn.AddReadData(message)
	
	server.HandleAppend(conn, "A002", parts, appendCmd, state)

	response := conn.GetWrittenData()
	
	if !strings.Contains(response, "A002 OK") {
		t.Errorf("Expected OK response for APPEND with flags, got: %s", response)
	}
}

// TestAppendCommand_NotAuthenticated tests APPEND without authentication
func TestAppendCommand_NotAuthenticated(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()

	state := &models.ClientState{
		Authenticated: false,
	}

	message := "Test message"
	appendCmd := fmt.Sprintf("A003 APPEND Sent {%d}", len(message))
	parts := strings.Fields(appendCmd)
	
	server.HandleAppend(conn, "A003", parts, appendCmd, state)

	response := conn.GetWrittenData()
	
	if !strings.Contains(response, "A003 NO") {
		t.Errorf("Expected NO response for unauthenticated APPEND, got: %s", response)
	}
	
	if !strings.Contains(response, "authenticate first") {
		t.Errorf("Expected authentication error message, got: %s", response)
	}
}

// TestAppendCommand_InvalidFolder tests APPEND to non-existent folder
func TestAppendCommand_InvalidFolder(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()

	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	message := "Test message"
	appendCmd := fmt.Sprintf("A004 APPEND NonExistent {%d}", len(message))
	parts := strings.Fields(appendCmd)
	
	server.HandleAppend(conn, "A004", parts, appendCmd, state)

	response := conn.GetWrittenData()
	
	if !strings.Contains(response, "A004 NO") {
		t.Errorf("Expected NO response for invalid folder, got: %s", response)
	}
	
	if !strings.Contains(response, "TRYCREATE") {
		t.Errorf("Expected TRYCREATE in response, got: %s", response)
	}
}

// TestAppendCommand_ToINBOX tests APPEND to INBOX folder
func TestAppendCommand_ToINBOX(t *testing.T) {
	db := helpers.CreateTestDB(t)
	helpers.CreateTestUserTable(t, db, "testuser")
	server := helpers.TestServerWithDB(db)
	conn := helpers.NewMockConn()

	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	message := "From: sender@example.com\r\nTo: recipient@example.com\r\nSubject: INBOX Test\r\n\r\nINBOX test message.\r\n"
	appendCmd := fmt.Sprintf("A005 APPEND INBOX {%d}", len(message))
	
	parts := strings.Fields(appendCmd)
	conn.AddReadData(message)
	
	server.HandleAppend(conn, "A005", parts, appendCmd, state)

	response := conn.GetWrittenData()
	
	if !strings.Contains(response, "A005 OK") {
		t.Errorf("Expected OK response for APPEND to INBOX, got: %s", response)
	}
}

// TestAppendCommand_ToDrafts tests APPEND to Drafts folder
func TestAppendCommand_ToDrafts(t *testing.T) {
	db := helpers.CreateTestDB(t)
	helpers.CreateTestUserTable(t, db, "testuser")
	server := helpers.TestServerWithDB(db)
	conn := helpers.NewMockConn()

	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	message := "From: sender@example.com\r\nSubject: Draft\r\n\r\nDraft message.\r\n"
	appendCmd := fmt.Sprintf("A006 APPEND Drafts (\\Draft) {%d}", len(message))
	
	parts := strings.Fields(appendCmd)
	conn.AddReadData(message)
	
	server.HandleAppend(conn, "A006", parts, appendCmd, state)

	response := conn.GetWrittenData()
	
	if !strings.Contains(response, "A006 OK") {
		t.Errorf("Expected OK response for APPEND to Drafts, got: %s", response)
	}
}

// TestAppendCommand_MissingSize tests APPEND without literal size
func TestAppendCommand_MissingSize(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()

	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	appendCmd := "A007 APPEND Sent"
	parts := strings.Fields(appendCmd)
	
	server.HandleAppend(conn, "A007", parts, appendCmd, state)

	response := conn.GetWrittenData()
	
	if !strings.Contains(response, "A007 BAD") {
		t.Errorf("Expected BAD response for missing size, got: %s", response)
	}
}
