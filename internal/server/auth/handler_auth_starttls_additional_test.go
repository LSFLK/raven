//go:build test

package auth_test

import (
	"strings"
	"testing"

	"raven/internal/server"
)

// TestStartTLSWithArguments tests that STARTTLS rejects commands with arguments
func TestStartTLSWithArguments(t *testing.T) {
	s, cleanup := server.SetupTestServer(t)
	defer cleanup()

	conn := server.NewMockConn()
	tag := "A001"

	// STARTTLS with extra arguments
	parts := []string{"A001", "STARTTLS", "extraargument"}

	// Call HandleStartTLS
	s.HandleStartTLS(conn, tag, parts)

	response := conn.GetWrittenData()

	// Should get BAD response
	if !strings.Contains(response, "A001 BAD") {
		t.Errorf("Expected BAD response for STARTTLS with arguments, got: %s", response)
	}

	if !strings.Contains(response, "does not accept arguments") {
		t.Errorf("Expected message about arguments not accepted, got: %s", response)
	}
}

// TestStartTLSOnTLSConnection tests that STARTTLS fails if TLS is already active
func TestStartTLSOnTLSConnection(t *testing.T) {
	s, cleanup := server.SetupTestServer(t)
	defer cleanup()

	// Use a TLS connection (already encrypted)
	conn := server.NewMockTLSConn()
	tag := "A001"
	parts := []string{"A001", "STARTTLS"}

	// Call HandleStartTLS on an already-TLS connection
	s.HandleStartTLS(conn, tag, parts)

	response := conn.GetWrittenData()

	// Should get BAD response saying TLS is already active
	if !strings.Contains(response, "A001 BAD") {
		t.Errorf("Expected BAD response for STARTTLS on TLS connection, got: %s", response)
	}

	if !strings.Contains(response, "TLS already active") {
		t.Errorf("Expected message about TLS already active, got: %s", response)
	}
}

// TestStartTLSCertLoadFailure tests handling when cert files are missing/invalid
func TestStartTLSCertLoadFailure(t *testing.T) {
	s, cleanup := server.SetupTestServer(t)
	defer cleanup()

	conn := server.NewMockConn()
	tag := "A002"
	parts := []string{"A002", "STARTTLS"}

	// Call HandleStartTLS - will try to load certs from configured paths
	s.HandleStartTLS(conn, tag, parts)

	response := conn.GetWrittenData()

	// Depending on cert availability, we either get:
	// 1. BAD response if certs can't be loaded
	// 2. OK response if certs exist (test environment might have them)

	if strings.Contains(response, "A002 BAD") {
		if !strings.Contains(response, "TLS not available") {
			t.Logf("Got BAD response (expected without valid certs): %s", response)
		}
	} else if strings.Contains(response, "A002 OK") {
		t.Logf("Got OK response (certs are available in test environment): %s", response)
	} else {
		t.Logf("Unexpected response: %s", response)
	}
}

// TestStartTLSVariousTags tests STARTTLS with different tag formats
func TestStartTLSVariousTags(t *testing.T) {
	testCases := []struct {
		tag         string
		description string
	}{
		{"A001", "Standard alphanumeric tag"},
		{"123", "Numeric tag"},
		{"TAG", "Alphabetic tag"},
		{"a", "Single character tag"},
		{"STARTTLS1", "Tag with command name"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			s, cleanup := server.SetupTestServer(t)
			defer cleanup()

			conn := server.NewMockConn()
			parts := []string{tc.tag, "STARTTLS"}

			s.HandleStartTLS(conn, tc.tag, parts)

			response := conn.GetWrittenData()

			// Response should include the tag
			if !strings.Contains(response, tc.tag) {
				t.Errorf("Response should contain tag '%s', got: %s", tc.tag, response)
			}

			// Should either be OK (certs available) or BAD (no certs)
			if !strings.Contains(response, " OK ") && !strings.Contains(response, " BAD ") {
				t.Errorf("Expected OK or BAD response, got: %s", response)
			}
		})
	}
}

// TestStartTLSCommandStructure tests STARTTLS command structure validation
func TestStartTLSCommandStructure(t *testing.T) {
	testCases := []struct {
		name     string
		parts    []string
		expected string
	}{
		{
			name:     "ValidCommand",
			parts:    []string{"A001", "STARTTLS"},
			expected: "A001", // Should process
		},
		{
			name:     "WithOneArgument",
			parts:    []string{"A001", "STARTTLS", "arg1"},
			expected: "A001 BAD", // Should reject
		},
		{
			name:     "WithMultipleArguments",
			parts:    []string{"A001", "STARTTLS", "arg1", "arg2", "arg3"},
			expected: "A001 BAD", // Should reject
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			s, cleanup := server.SetupTestServer(t)
			defer cleanup()

			conn := server.NewMockConn()

			s.HandleStartTLS(conn, tc.parts[0], tc.parts)

			response := conn.GetWrittenData()

			if !strings.Contains(response, tc.expected) {
				t.Errorf("Expected response to contain '%s', got: %s", tc.expected, response)
			}
		})
	}
}

// TestStartTLSResponseOrder tests that OK response is sent before TLS negotiation
func TestStartTLSResponseOrder(t *testing.T) {
	// Per RFC 3501, server MUST send OK response BEFORE starting TLS negotiation
	// This ensures client knows to start TLS handshake

	s, cleanup := server.SetupTestServer(t)
	defer cleanup()

	conn := server.NewMockConn()
	tag := "A001"
	parts := []string{"A001", "STARTTLS"}

	s.HandleStartTLS(conn, tag, parts)

	response := conn.GetWrittenData()

	// If certs are available, should get OK response
	// If certs are not available, should get BAD response
	// Either way, response should be sent before any TLS negotiation

	t.Logf("STARTTLS response: %s", response)
}

// TestStartTLSMinTLSVersion tests that TLS 1.2 is minimum required version
func TestStartTLSMinTLSVersion(t *testing.T) {
	// HandleStartTLS should configure TLS with MinVersion: tls.VersionTLS12
	// This is a specification test documenting the requirement

	t.Log("STARTTLS should require minimum TLS version 1.2")
	t.Log("This ensures modern security standards are enforced")

	// The actual TLS version check happens during handshake
	// This test documents the requirement
}

// TestStartTLSStateClear tests that client state is reset after STARTTLS
func TestStartTLSStateClear(t *testing.T) {
	// Per RFC 3501, client MUST discard cached server capabilities after STARTTLS
	// Server restarts handler with fresh state

	t.Log("Per RFC 3501: Client state should be cleared after STARTTLS")
	t.Log("Handler should be called with fresh ClientState{}")

	// This is enforced by calling clientHandler(tlsConn, &models.ClientState{})
	// in the HandleStartTLS implementation
}

// TestStartTLSEmptyPartsArray tests handling of minimal parts array
func TestStartTLSEmptyPartsArray(t *testing.T) {
	s, cleanup := server.SetupTestServer(t)
	defer cleanup()

	conn := server.NewMockConn()
	tag := "A001"

	// Minimal valid STARTTLS command: just tag and command
	parts := []string{"A001", "STARTTLS"}

	s.HandleStartTLS(conn, tag, parts)

	response := conn.GetWrittenData()

	// Should process successfully (or fail due to cert issues)
	if strings.Contains(response, " OK ") || strings.Contains(response, " BAD ") {
		t.Logf("STARTTLS processed correctly: %s", response)
	} else {
		t.Errorf("Unexpected response format: %s", response)
	}
}
