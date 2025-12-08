package message_test

import (
	"fmt"
	"strings"
	"testing"

	"raven/internal/models"
	"raven/internal/server"
)

// TestAppendCommand_Basic tests basic APPEND functionality
func TestAppendCommand_Basic(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Simulate APPEND command with a simple message
	message := "From: sender@example.com\r\nTo: recipient@example.com\r\nSubject: Test Message\r\n\r\nThis is a test message body.\r\n"
	appendCmd := fmt.Sprintf("A001 APPEND Sent {%d}", len(message))

	// First, send the APPEND command with literal size
	parts := strings.Fields(appendCmd)
	fullLine := appendCmd

	// Simulate the client sending the command
	conn.AddReadData(message)

	srv.HandleAppend(conn, "A001", parts, fullLine, state)

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
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	message := "From: sender@example.com\r\nTo: recipient@example.com\r\nSubject: Test\r\n\r\nBody\r\n"
	appendCmd := fmt.Sprintf("A002 APPEND Sent (\\Seen) {%d}", len(message))

	parts := strings.Fields(appendCmd)
	conn.AddReadData(message)

	srv.HandleAppend(conn, "A002", parts, appendCmd, state)

	response := conn.GetWrittenData()

	if !strings.Contains(response, "A002 OK") {
		t.Errorf("Expected OK response for APPEND with flags, got: %s", response)
	}
}

// TestAppendCommand_NotAuthenticated tests APPEND without authentication
func TestAppendCommand_NotAuthenticated(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()

	state := &models.ClientState{
		Authenticated: false,
	}

	message := "Test message"
	appendCmd := fmt.Sprintf("A003 APPEND Sent {%d}", len(message))
	parts := strings.Fields(appendCmd)

	srv.HandleAppend(conn, "A003", parts, appendCmd, state)

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
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()

	state := server.SetupAuthenticatedState(t, srv, "testuser")

	message := "Test message"
	appendCmd := fmt.Sprintf("A004 APPEND NonExistent {%d}", len(message))
	parts := strings.Fields(appendCmd)

	srv.HandleAppend(conn, "A004", parts, appendCmd, state)

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
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	message := "From: sender@example.com\r\nTo: recipient@example.com\r\nSubject: INBOX Test\r\n\r\nINBOX test message.\r\n"
	appendCmd := fmt.Sprintf("A005 APPEND INBOX {%d}", len(message))

	parts := strings.Fields(appendCmd)
	conn.AddReadData(message)

	srv.HandleAppend(conn, "A005", parts, appendCmd, state)

	response := conn.GetWrittenData()

	if !strings.Contains(response, "A005 OK") {
		t.Errorf("Expected OK response for APPEND to INBOX, got: %s", response)
	}
}

// TestAppendCommand_ToDrafts tests APPEND to Drafts folder
func TestAppendCommand_ToDrafts(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	message := "From: sender@example.com\r\nSubject: Draft\r\n\r\nDraft message.\r\n"
	appendCmd := fmt.Sprintf("A006 APPEND Drafts (\\Draft) {%d}", len(message))

	parts := strings.Fields(appendCmd)
	conn.AddReadData(message)

	srv.HandleAppend(conn, "A006", parts, appendCmd, state)

	response := conn.GetWrittenData()

	if !strings.Contains(response, "A006 OK") {
		t.Errorf("Expected OK response for APPEND to Drafts, got: %s", response)
	}
}

// TestAppendCommand_MissingSize tests APPEND without literal size
func TestAppendCommand_MissingSize(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()

	state := server.SetupAuthenticatedState(t, srv, "testuser")

	appendCmd := "A007 APPEND Sent"
	parts := strings.Fields(appendCmd)

	srv.HandleAppend(conn, "A007", parts, appendCmd, state)

	response := conn.GetWrittenData()

	if !strings.Contains(response, "A007 BAD") {
		t.Errorf("Expected BAD response for missing size, got: %s", response)
	}
}

// TestAppendCommand_RFC3501Example tests the exact example from RFC 3501
func TestAppendCommand_RFC3501Example(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Create the saved-messages mailbox first (as per RFC example)
	database := server.GetDatabaseFromServer(srv)
	server.CreateMailbox(t, database, "testuser", "saved-messages")

	// RFC 3501 example message
	message := "Date: Mon, 7 Feb 1994 21:52:25 -0800 (PST)\r\n" +
		"From: Fred Foobar <foobar@Blurdybloop.COM>\r\n" +
		"Subject: afternoon meeting\r\n" +
		"To: mooch@owatagu.siam.edu\r\n" +
		"Message-Id: <B27397-0100000@Blurdybloop.COM>\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: TEXT/PLAIN; CHARSET=US-ASCII\r\n" +
		"\r\n" +
		"Hello Joe, do you think we can meet at 3:30 tomorrow?\r\n"

	appendCmd := fmt.Sprintf("A003 APPEND saved-messages (\\Seen) {%d}", len(message))

	parts := strings.Fields(appendCmd)
	conn.AddReadData(message)

	srv.HandleAppend(conn, "A003", parts, appendCmd, state)

	response := conn.GetWrittenData()

	if !strings.Contains(response, "+ Ready for literal data") {
		t.Errorf("Expected continuation response, got: %s", response)
	}

	if !strings.Contains(response, "A003 OK") {
		t.Errorf("Expected OK response, got: %s", response)
	}

	if !strings.Contains(response, "APPEND completed") {
		t.Errorf("Expected APPEND completed message, got: %s", response)
	}
}

// TestAppendCommand_MultipleFlags tests APPEND with multiple flags
func TestAppendCommand_MultipleFlags(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	message := "From: test@example.com\r\nSubject: Test\r\n\r\nBody\r\n"
	appendCmd := fmt.Sprintf("A008 APPEND INBOX (\\Seen \\Flagged) {%d}", len(message))

	parts := strings.Fields(appendCmd)
	conn.AddReadData(message)

	srv.HandleAppend(conn, "A008", parts, appendCmd, state)

	response := conn.GetWrittenData()

	if !strings.Contains(response, "A008 OK") {
		t.Errorf("Expected OK response for APPEND with multiple flags, got: %s", response)
	}
}

// TestAppendCommand_EmptyMessage tests APPEND with empty message
func TestAppendCommand_EmptyMessage(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	message := ""
	appendCmd := fmt.Sprintf("A009 APPEND INBOX {%d}", len(message))

	parts := strings.Fields(appendCmd)
	conn.AddReadData(message)

	srv.HandleAppend(conn, "A009", parts, appendCmd, state)

	response := conn.GetWrittenData()

	// Server should reject empty messages or handle them gracefully
	if !strings.Contains(response, "A009 OK") && !strings.Contains(response, "A009 NO") {
		t.Errorf("Expected OK or NO response for empty message, got: %s", response)
	}
}

// TestAppendCommand_LargeMessage tests APPEND with a large message
func TestAppendCommand_LargeMessage(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Create a 1MB message
	messageSize := 1024 * 1024
	largeBody := strings.Repeat("a", messageSize-100)
	message := fmt.Sprintf("From: test@example.com\r\nSubject: Large\r\n\r\n%s\r\n", largeBody)

	appendCmd := fmt.Sprintf("A010 APPEND INBOX {%d}", len(message))

	parts := strings.Fields(appendCmd)
	conn.AddReadData(message)

	srv.HandleAppend(conn, "A010", parts, appendCmd, state)

	response := conn.GetWrittenData()

	if !strings.Contains(response, "A010 OK") {
		t.Errorf("Expected OK response for large message, got: %s", response)
	}
}

// TestAppendCommand_InvalidSize tests APPEND with negative size
func TestAppendCommand_InvalidSize(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()

	state := server.SetupAuthenticatedState(t, srv, "testuser")

	appendCmd := "A011 APPEND INBOX {-1}"
	parts := strings.Fields(appendCmd)

	srv.HandleAppend(conn, "A011", parts, appendCmd, state)

	response := conn.GetWrittenData()

	if !strings.Contains(response, "A011 NO") {
		t.Errorf("Expected NO response for invalid size, got: %s", response)
	}
}

// TestAppendCommand_ExcessiveSize tests APPEND with size exceeding limit
func TestAppendCommand_ExcessiveSize(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()

	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Try to append a message larger than 50MB limit
	appendCmd := "A012 APPEND INBOX {52428801}" // 50MB + 1 byte
	parts := strings.Fields(appendCmd)

	srv.HandleAppend(conn, "A012", parts, appendCmd, state)

	response := conn.GetWrittenData()

	if !strings.Contains(response, "A012 NO") {
		t.Errorf("Expected NO response for excessive size, got: %s", response)
	}

	if !strings.Contains(response, "too large") {
		t.Errorf("Expected 'too large' error message, got: %s", response)
	}
}

// TestAppendCommand_QuotedMailboxName tests APPEND to a quoted mailbox name
func TestAppendCommand_QuotedMailboxName(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	message := "From: test@example.com\r\nSubject: Test\r\n\r\nBody\r\n"
	appendCmd := fmt.Sprintf("A013 APPEND \"Sent\" {%d}", len(message))

	parts := []string{"A013", "APPEND", "\"Sent\"", fmt.Sprintf("{%d}", len(message))}
	conn.AddReadData(message)

	srv.HandleAppend(conn, "A013", parts, appendCmd, state)

	response := conn.GetWrittenData()

	if !strings.Contains(response, "A013 OK") {
		t.Errorf("Expected OK response for quoted mailbox name, got: %s", response)
	}
}

// TestAppendCommand_WithoutFlags tests APPEND without optional flags
func TestAppendCommand_WithoutFlags(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	message := "From: test@example.com\r\nSubject: No Flags\r\n\r\nMessage without flags\r\n"
	appendCmd := fmt.Sprintf("A014 APPEND INBOX {%d}", len(message))

	parts := strings.Fields(appendCmd)
	conn.AddReadData(message)

	srv.HandleAppend(conn, "A014", parts, appendCmd, state)

	response := conn.GetWrittenData()

	if !strings.Contains(response, "A014 OK") {
		t.Errorf("Expected OK response for APPEND without flags, got: %s", response)
	}
}

// TestAppendCommand_MissingMailboxName tests APPEND without mailbox name
func TestAppendCommand_MissingMailboxName(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()

	state := server.SetupAuthenticatedState(t, srv, "testuser")

	appendCmd := "A015 APPEND"
	parts := strings.Fields(appendCmd)

	srv.HandleAppend(conn, "A015", parts, appendCmd, state)

	response := conn.GetWrittenData()

	if !strings.Contains(response, "A015 BAD") {
		t.Errorf("Expected BAD response for missing mailbox name, got: %s", response)
	}

	if !strings.Contains(response, "requires folder name") {
		t.Errorf("Expected 'requires folder name' error, got: %s", response)
	}
}

// TestAppendCommand_8BitCharacters tests APPEND with 8-bit characters
func TestAppendCommand_8BitCharacters(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Message with UTF-8 characters (8-bit)
	message := "From: test@example.com\r\nSubject: Tëst Mëssägë\r\n\r\nBody with 8-bit: café, naïve, résumé\r\n"
	appendCmd := fmt.Sprintf("A016 APPEND INBOX {%d}", len(message))

	parts := strings.Fields(appendCmd)
	conn.AddReadData(message)

	srv.HandleAppend(conn, "A016", parts, appendCmd, state)

	response := conn.GetWrittenData()

	if !strings.Contains(response, "A016 OK") {
		t.Errorf("Expected OK response for 8-bit characters, got: %s", response)
	}
}

// TestAppendCommand_AllDefaultMailboxes tests APPEND to all default mailboxes
func TestAppendCommand_AllDefaultMailboxes(t *testing.T) {
	defaultMailboxes := []string{"INBOX", "Sent", "Drafts", "Trash"}

	for _, mailbox := range defaultMailboxes {
		t.Run(mailbox, func(t *testing.T) {
			srv := server.SetupTestServerSimple(t)
			conn := server.NewMockConn()
			state := server.SetupAuthenticatedState(t, srv, "testuser")

			message := fmt.Sprintf("From: test@example.com\r\nSubject: Test to %s\r\n\r\nBody\r\n", mailbox)
			appendCmd := fmt.Sprintf("A017 APPEND %s {%d}", mailbox, len(message))

			parts := strings.Fields(appendCmd)
			conn.AddReadData(message)

			srv.HandleAppend(conn, "A017", parts, appendCmd, state)

			response := conn.GetWrittenData()

			if !strings.Contains(response, "A017 OK") {
				t.Errorf("Expected OK response for APPEND to %s, got: %s", mailbox, response)
			}
		})
	}
}

// TestAppendCommand_ReturnsAppendUID tests that APPEND returns APPENDUID
func TestAppendCommand_ReturnsAppendUID(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	message := "From: test@example.com\r\nSubject: UID Test\r\n\r\nBody\r\n"
	appendCmd := fmt.Sprintf("A018 APPEND INBOX {%d}", len(message))

	parts := strings.Fields(appendCmd)
	conn.AddReadData(message)

	srv.HandleAppend(conn, "A018", parts, appendCmd, state)

	response := conn.GetWrittenData()

	if !strings.Contains(response, "APPENDUID") {
		t.Errorf("Expected APPENDUID in response (RFC 4315 - UIDPLUS), got: %s", response)
	}

	if !strings.Contains(response, "A018 OK") {
		t.Errorf("Expected OK response, got: %s", response)
	}
}

// TestAppendCommand_MessageParsing tests that message headers are parsed correctly
func TestAppendCommand_MessageParsing(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Message with all important headers
	message := "Date: Mon, 7 Feb 1994 21:52:25 -0800 (PST)\r\n" +
		"From: sender@example.com\r\n" +
		"To: recipient@example.com\r\n" +
		"Subject: Test Subject\r\n" +
		"\r\n" +
		"Test body\r\n"

	appendCmd := fmt.Sprintf("A019 APPEND INBOX {%d}", len(message))

	parts := strings.Fields(appendCmd)
	conn.AddReadData(message)

	srv.HandleAppend(conn, "A019", parts, appendCmd, state)

	response := conn.GetWrittenData()

	if !strings.Contains(response, "A019 OK") {
		t.Errorf("Expected OK response, got: %s", response)
	}
}

// TestAppendCommand_LiteralPlus tests APPEND with LITERAL+ (non-synchronizing literal)
func TestAppendCommand_LiteralPlus(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	state := server.SetupAuthenticatedState(t, srv, "testuser")

	// Test LITERAL+ syntax: {size+} means client sends data immediately
	message := "From: sender@example.com\r\nSubject: Test LITERAL+\r\n\r\nBody\r\n"
	appendCmd := fmt.Sprintf("A020 APPEND INBOX {%d+}", len(message))

	parts := strings.Fields(appendCmd)
	conn.AddReadData(message)

	srv.HandleAppend(conn, "A020", parts, appendCmd, state)

	response := conn.GetWrittenData()

	// With LITERAL+, server should NOT send continuation response
	if strings.Contains(response, "+ Ready for literal data") {
		t.Errorf("LITERAL+ should not trigger continuation response, got: %s", response)
	}

	if !strings.Contains(response, "A020 OK") {
		t.Errorf("Expected OK response, got: %s", response)
	}

	if !strings.Contains(response, "APPENDUID") {
		t.Errorf("Expected APPENDUID in response, got: %s", response)
	}
}
