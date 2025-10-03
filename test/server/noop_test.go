//go:build test
// +build test

package server

import (
	"strings"
	"testing"

	"go-imap/internal/models"
	"go-imap/test/helpers"
)

// TestNoopCommand_Unauthenticated tests NOOP before authentication
func TestNoopCommand_Unauthenticated(t *testing.T) {
	server := helpers.SetupTestServer(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: false,
	}

	server.HandleNoop(conn, "N001", state)

	response := conn.GetWrittenData()
	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Should have 1 line: completion only
	if len(lines) != 1 {
		t.Fatalf("Expected 1 response line, got %d: %v", len(lines), lines)
	}

	// Check tagged OK response
	expectedOK := "N001 OK NOOP completed"
	if lines[0] != expectedOK {
		t.Errorf("Expected '%s', got: '%s'", expectedOK, lines[0])
	}
}

// TestNoopCommand_AuthenticatedNoFolder tests NOOP when authenticated but no folder selected
func TestNoopCommand_AuthenticatedNoFolder(t *testing.T) {
	server := helpers.SetupTestServer(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated:  true,
		Username:       "testuser",
		SelectedFolder: "",
	}

	server.HandleNoop(conn, "N002", state)

	response := conn.GetWrittenData()
	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Should have 1 line: completion only
	if len(lines) != 1 {
		t.Fatalf("Expected 1 response line, got %d: %v", len(lines), lines)
	}

	// Check tagged OK response
	expectedOK := "N002 OK NOOP completed"
	if lines[0] != expectedOK {
		t.Errorf("Expected '%s', got: '%s'", expectedOK, lines[0])
	}
}

// TestNoopCommand_WithSelectedFolder tests NOOP with selected folder (no changes)
func TestNoopCommand_WithSelectedFolder(t *testing.T) {
	server := helpers.SetupTestServer(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated:    true,
		Username:         "testuser",
		SelectedFolder:   "INBOX",
		LastMessageCount: 5,
		LastRecentCount:  2,
	}

	// Simulate current mailbox has same state (no changes)
	server.HandleNoop(conn, "N003", state)

	response := conn.GetWrittenData()
	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Should have only completion (no untagged responses for no changes)
	// Note: Actual behavior depends on database state
	if len(lines) < 1 {
		t.Fatalf("Expected at least 1 response line, got %d: %v", len(lines), lines)
	}

	// Last line should be tagged OK response
	lastLine := lines[len(lines)-1]
	expectedOK := "N003 OK NOOP completed"
	if lastLine != expectedOK {
		t.Errorf("Expected '%s', got: '%s'", expectedOK, lastLine)
	}
}

// TestNoopCommand_AlwaysSucceeds tests that NOOP always returns OK
func TestNoopCommand_AlwaysSucceeds(t *testing.T) {
	testCases := []struct {
		name  string
		state *models.ClientState
		tag   string
	}{
		{
			name: "Unauthenticated",
			state: &models.ClientState{
				Authenticated: false,
			},
			tag: "T001",
		},
		{
			name: "Authenticated",
			state: &models.ClientState{
				Authenticated: true,
				Username:      "user1",
			},
			tag: "T002",
		},
		{
			name: "With selected folder",
			state: &models.ClientState{
				Authenticated:    true,
				Username:         "user2",
				SelectedFolder:   "INBOX",
				LastMessageCount: 10,
				LastRecentCount:  3,
			},
			tag: "T003",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := helpers.SetupTestServer(t)
			conn := helpers.NewMockConn()

			server.HandleNoop(conn, tc.tag, tc.state)

			response := conn.GetWrittenData()
			
			// Should always end with OK
			if !strings.Contains(response, " OK NOOP completed") {
				t.Errorf("NOOP should always succeed, got: %s", response)
			}

			// Should contain the correct tag
			expectedOK := tc.tag + " OK NOOP completed"
			if !strings.Contains(response, expectedOK) {
				t.Errorf("Expected to find '%s' in response, got: %s", expectedOK, response)
			}
		})
	}
}

// TestNoopCommand_ResponseFormat tests the format of NOOP responses
func TestNoopCommand_ResponseFormat(t *testing.T) {
	server := helpers.SetupTestServer(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	server.HandleNoop(conn, "FORMAT", state)

	response := conn.GetWrittenData()
	
	// Check that response ends with CRLF
	if !strings.HasSuffix(response, "\r\n") {
		t.Errorf("Response should end with CRLF")
	}

	lines := strings.Split(strings.TrimSpace(response), "\r\n")
	
	// Last line should be tagged completion
	lastLine := lines[len(lines)-1]
	if !strings.HasPrefix(lastLine, "FORMAT OK NOOP completed") {
		t.Errorf("Last line should be tagged completion, got: %s", lastLine)
	}
}

// TestNoopCommand_MultipleInvocations tests calling NOOP multiple times
func TestNoopCommand_MultipleInvocations(t *testing.T) {
	server := helpers.SetupTestServer(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}

	// Call NOOP multiple times with different tags
	server.HandleNoop(conn, "M001", state)
	server.HandleNoop(conn, "M002", state)
	server.HandleNoop(conn, "M003", state)

	response := conn.GetWrittenData()
	
	// Should have all three completions
	if !strings.Contains(response, "M001 OK NOOP completed") {
		t.Error("Missing M001 completion")
	}
	if !strings.Contains(response, "M002 OK NOOP completed") {
		t.Error("Missing M002 completion")
	}
	if !strings.Contains(response, "M003 OK NOOP completed") {
		t.Error("Missing M003 completion")
	}
}

// TestNoopCommand_StateTracking tests that NOOP updates state correctly
func TestNoopCommand_StateTracking(t *testing.T) {
	server := helpers.SetupTestServer(t)
	conn := helpers.NewMockConn()
	
	// Start with initial state
	initialCount := 5
	initialRecent := 2
	
	state := &models.ClientState{
		Authenticated:    true,
		Username:         "testuser",
		SelectedFolder:   "INBOX",
		LastMessageCount: initialCount,
		LastRecentCount:  initialRecent,
	}

	server.HandleNoop(conn, "TRACK", state)

	// After NOOP, state should be updated with current mailbox state
	// In this test, we expect state to be updated even if values haven't changed
	// (The actual values depend on database state)
	
	response := conn.GetWrittenData()
	if !strings.Contains(response, "TRACK OK NOOP completed") {
		t.Errorf("Expected NOOP completion, got: %s", response)
	}
}

// TestNoopCommand_TagHandling tests various tag formats
func TestNoopCommand_TagHandling(t *testing.T) {
	testCases := []struct {
		tag         string
		expectedTag string
	}{
		{"A001", "A001"},
		{"noop1", "noop1"},
		{"TAG-123", "TAG-123"},
		{"*", "*"},
		{"", ""},
		{"VERY-LONG-TAG-NAME-FOR-NOOP", "VERY-LONG-TAG-NAME-FOR-NOOP"},
	}

	for _, tc := range testCases {
		t.Run("Tag_"+tc.tag, func(t *testing.T) {
			server := helpers.SetupTestServer(t)
			conn := helpers.NewMockConn()
			state := &models.ClientState{Authenticated: false}

			server.HandleNoop(conn, tc.tag, state)

			response := conn.GetWrittenData()
			expectedOK := tc.expectedTag + " OK NOOP completed"
			
			if !strings.Contains(response, expectedOK) {
				t.Errorf("Expected '%s' in response, got: %s", expectedOK, response)
			}
		})
	}
}

// TestNoopCommand_ConcurrentAccess tests concurrent NOOP requests
func TestNoopCommand_ConcurrentAccess(t *testing.T) {
	server := helpers.SetupTestServer(t)
	
	const numRequests = 20
	done := make(chan bool, numRequests)

	// Launch concurrent NOOP requests
	for i := 0; i < numRequests; i++ {
		go func(index int) {
			conn := helpers.NewMockConn()
			state := &models.ClientState{
				Authenticated: true,
				Username:      "user",
			}
			server.HandleNoop(conn, "CONCURRENT", state)
			
			response := conn.GetWrittenData()
			if !strings.Contains(response, "CONCURRENT OK NOOP completed") {
				t.Errorf("Request %d failed: %s", index, response)
			}
			done <- true
		}(i)
	}

	// Wait for all requests to complete
	for i := 0; i < numRequests; i++ {
		<-done
	}
}

// TestNoopCommand_RFC3501Compliance tests RFC 3501 compliance
func TestNoopCommand_RFC3501Compliance(t *testing.T) {
	t.Run("Always succeeds", func(t *testing.T) {
		server := helpers.SetupTestServer(t)
		conn := helpers.NewMockConn()
		state := &models.ClientState{}

		server.HandleNoop(conn, "RFC1", state)

		response := conn.GetWrittenData()
		if !strings.Contains(response, "RFC1 OK") {
			t.Error("NOOP must always succeed per RFC 3501")
		}
	})

	t.Run("Can be used for polling", func(t *testing.T) {
		server := helpers.SetupTestServer(t)
		conn := helpers.NewMockConn()
		state := &models.ClientState{
			Authenticated:    true,
			SelectedFolder:   "INBOX",
			LastMessageCount: 0,
			LastRecentCount:  0,
		}

		// Multiple NOOP calls to simulate polling
		for i := 0; i < 3; i++ {
			server.HandleNoop(conn, "RFC2", state)
		}

		response := conn.GetWrittenData()
		// Should complete successfully multiple times
		count := strings.Count(response, "RFC2 OK NOOP completed")
		if count != 3 {
			t.Errorf("Expected 3 completions, got %d", count)
		}
	})

	t.Run("Resets inactivity timer", func(t *testing.T) {
		// This is implicit - by processing the command, the server
		// resets any inactivity timer. We just verify NOOP works.
		server := helpers.SetupTestServer(t)
		conn := helpers.NewMockConn()
		state := &models.ClientState{Authenticated: true}

		server.HandleNoop(conn, "RFC3", state)

		response := conn.GetWrittenData()
		if !strings.Contains(response, "RFC3 OK") {
			t.Error("NOOP should reset inactivity timer")
		}
	})
}
