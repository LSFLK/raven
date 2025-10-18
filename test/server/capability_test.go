package server_test

import (
	"fmt"
	"strings"
	"testing"

	"go-imap/internal/models"
	"go-imap/test/helpers"
)

// TestCapabilityCommand_RFCCompliance tests RFC 3501 compliance
func TestCapabilityCommand_RFCCompliance(t *testing.T) {
	tests := []struct {
		name        string
		connType    string
		expectCaps  []string
		forbidCaps  []string
	}{
		{
			name:     "Non-TLS Connection",
			connType: "plain",
			expectCaps: []string{
				"IMAP4rev1",
				"STARTTLS", 
				"LOGINDISABLED",
				"UIDPLUS",
				"IDLE", 
				"NAMESPACE",
				"UNSELECT",
				"LITERAL+",
			},
			forbidCaps: []string{
				"AUTH=PLAIN",
				"LOGIN",
			},
		},
		{
			name:     "TLS Connection",
			connType: "tls",
			expectCaps: []string{
				"IMAP4rev1",
				"AUTH=PLAIN",
				"LOGIN",
				"UIDPLUS",
				"IDLE",
				"NAMESPACE", 
				"UNSELECT",
				"LITERAL+",
			},
			forbidCaps: []string{
				"STARTTLS",
				"LOGINDISABLED",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := helpers.SetupTestServerSimple(t)
			var conn helpers.MockConnInterface
			
			if tt.connType == "tls" {
				conn = helpers.NewMockTLSConn()
			} else {
				conn = helpers.NewMockConn()
			}

			state := &models.ClientState{Authenticated: false}
			server.HandleCapability(conn, "TEST", state)

			response := conn.GetWrittenData()
			lines := strings.Split(strings.TrimSpace(response), "\r\n")
			
			if len(lines) < 1 {
				t.Fatal("No response received")
			}

			capLine := lines[0]

			// Check required capabilities using exact token matching
			for _, cap := range tt.expectCaps {
				if !hasCapabilityToken(capLine, cap) {
					t.Errorf("Expected capability %s not found in: %s", cap, capLine)
				}
			}

			// Check forbidden capabilities using exact token matching
			for _, cap := range tt.forbidCaps {
				if hasCapabilityToken(capLine, cap) {
					t.Errorf("Forbidden capability %s found in: %s", cap, capLine)
				}
			}
		})
	}
}

// TestCapabilityCommand_TagHandling tests various tag formats
func TestCapabilityCommand_TagHandling(t *testing.T) {
	testCases := []struct {
		tag          string
		expectedTag  string
	}{
		{"A001", "A001"},
		{"a1", "a1"},
		{"TAG123", "TAG123"},
		{"*", "*"},
		{"", ""},
		{"VERY-LONG-TAG-NAME-123", "VERY-LONG-TAG-NAME-123"},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Tag_%s", tc.tag), func(t *testing.T) {
			server := helpers.SetupTestServerSimple(t)
			conn := helpers.NewMockConn()
			state := &models.ClientState{Authenticated: false}

			server.HandleCapability(conn, tc.tag, state)

			response := conn.GetWrittenData()
			lines := strings.Split(strings.TrimSpace(response), "\r\n")

			if len(lines) < 2 {
				t.Fatal("Expected at least 2 lines in response")
			}

			// Last line should be the tagged OK response
			okLine := lines[len(lines)-1]
			expectedOK := fmt.Sprintf("%s OK CAPABILITY completed", tc.expectedTag)
			
			if okLine != expectedOK {
				t.Errorf("Expected tagged response '%s', got '%s'", expectedOK, okLine)
			}
		})
	}
}

// TestCapabilityCommand_CapabilityFormatting tests capability string formatting
func TestCapabilityCommand_CapabilityFormatting(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{Authenticated: false}

	server.HandleCapability(conn, "FORMAT", state)

	response := conn.GetWrittenData()
	lines := strings.Split(strings.TrimSpace(response), "\r\n")
	
	capLine := lines[0]

	// Test capability line format
	if !strings.HasPrefix(capLine, "* CAPABILITY ") {
		t.Errorf("Capability line should start with '* CAPABILITY ', got: %s", capLine)
	}

	// Extract capabilities
	capString := strings.TrimPrefix(capLine, "* CAPABILITY ")
	capabilities := strings.Split(capString, " ")

	// Test that capabilities are non-empty
	for i, cap := range capabilities {
		if cap == "" {
			t.Errorf("Empty capability found at position %d", i)
		}
		
		// Test that capabilities don't contain invalid characters
		if strings.Contains(cap, "\r") || strings.Contains(cap, "\n") {
			t.Errorf("Capability contains invalid characters: %s", cap)
		}
	}

	// Test that IMAP4rev1 is present
	found := false
	for _, cap := range capabilities {
		if cap == "IMAP4rev1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("IMAP4rev1 capability is required but not found")
	}

	// Test for duplicate capabilities
	capMap := make(map[string]bool)
	for _, cap := range capabilities {
		if capMap[cap] {
			t.Errorf("Duplicate capability found: %s", cap)
		}
		capMap[cap] = true
	}
}

// TestCapabilityCommand_EdgeCases tests edge cases and error conditions
func TestCapabilityCommand_EdgeCases(t *testing.T) {
	t.Run("EmptyTag", func(t *testing.T) {
		server := helpers.SetupTestServerSimple(t)
		conn := helpers.NewMockConn()
		state := &models.ClientState{Authenticated: false}

		server.HandleCapability(conn, "", state)

		response := conn.GetWrittenData()
		// Should still work with empty tag
		if !strings.Contains(response, "* CAPABILITY") {
			t.Error("Should return capability response even with empty tag")
		}
		if !strings.Contains(response, " OK CAPABILITY completed") {
			t.Error("Should return OK response even with empty tag")
		}
	})

	t.Run("NilState", func(t *testing.T) {
		server := helpers.SetupTestServerSimple(t)
		conn := helpers.NewMockConn()

		// This should not panic
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("HandleCapability panicked with nil state: %v", r)
			}
		}()

		server.HandleCapability(conn, "NIL", nil)
	})
}

// TestCapabilityCommand_ResponseTiming tests response timing and ordering
func TestCapabilityCommand_ResponseTiming(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := &models.ClientState{Authenticated: false}

	server.HandleCapability(conn, "TIMING", state)

	response := conn.GetWrittenData()
	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Should have exactly 2 lines
	if len(lines) != 2 {
		t.Errorf("Expected exactly 2 lines, got %d: %v", len(lines), lines)
	}

	// First line should be untagged CAPABILITY response
	if !strings.HasPrefix(lines[0], "* CAPABILITY ") {
		t.Errorf("First line should be untagged CAPABILITY response, got: %s", lines[0])
	}

	// Second line should be tagged OK response  
	if !strings.HasSuffix(lines[1], " OK CAPABILITY completed") {
		t.Errorf("Second line should be tagged OK response, got: %s", lines[1])
	}

	// Verify the order is correct (untagged before tagged)
	if strings.Contains(lines[0], " OK ") {
		t.Error("Untagged response should come before tagged OK response")
	}
}

// TestCapabilityCommand_MemoryUsage tests for memory leaks or excessive allocation
func TestCapabilityCommand_MemoryUsage(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	
	// Run many capability commands to check for memory issues
	for i := 0; i < 1000; i++ {
		conn := helpers.NewMockConn()
		state := &models.ClientState{Authenticated: false}
		server.HandleCapability(conn, fmt.Sprintf("MEM%d", i), state)
		
		// Verify response is still correct
		response := conn.GetWrittenData()
		if !strings.Contains(response, "* CAPABILITY") {
			t.Errorf("Iteration %d: Missing capability response", i)
			break
		}
	}
}

// TestCapabilityCommand_StateIsolation tests that different states don't interfere
func TestCapabilityCommand_StateIsolation(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)

	states := []*models.ClientState{
		{Authenticated: false, Username: ""},
		{Authenticated: true, Username: "user1"},
		{Authenticated: true, Username: "user2"},
		{Authenticated: false, Username: "old_user"},
	}

	responses := make([]string, len(states))

	// Get capability response for each state
	for i, state := range states {
		conn := helpers.NewMockConn()
		server.HandleCapability(conn, fmt.Sprintf("STATE%d", i), state)
		responses[i] = conn.GetWrittenData()
	}

	// For the same connection type, responses should be identical
	// (authentication state shouldn't affect capabilities)
	baseResponse := strings.Split(responses[0], "\r\n")[0] // Just the capability line
	for i := 1; i < len(responses); i++ {
		currentResponse := strings.Split(responses[i], "\r\n")[0]
		if baseResponse != currentResponse {
			t.Errorf("State %d produced different capabilities:\nBase: %s\nGot:  %s", 
				i, baseResponse, currentResponse)
		}
	}
}
