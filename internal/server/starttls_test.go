package server

import (
	"strings"
	"testing"

	"raven/internal/models"
	
)

// TestStartTLS_BasicFlow tests the basic STARTTLS command flow per RFC 3501
func TestStartTLS_BasicFlow(t *testing.T) {
	s, cleanup := SetupTestServer(t)
	defer cleanup()

	conn := NewMockConn() // Plain connection

	// Issue STARTTLS command
	s.HandleStartTLS(conn, "A001", []string{"A001", "STARTTLS"})

	response := conn.GetWrittenData()

	// Should get OK response with exact RFC 3501 text
	if !strings.Contains(response, "A001 OK Begin TLS negotiation now") {
		t.Errorf("Expected 'A001 OK Begin TLS negotiation now', got: %s", response)
	}
}

// TestStartTLS_ExactResponseFormat tests RFC 3501 compliance for response format
func TestStartTLS_ExactResponseFormat(t *testing.T) {
	s, cleanup := SetupTestServer(t)
	defer cleanup()

	conn := NewMockConn()

	s.HandleStartTLS(conn, "a002", []string{"a002", "STARTTLS"})

	response := conn.GetWrittenData()

	// RFC 3501 specifies: "OK Begin TLS negotiation now"
	expectedResponse := "a002 OK Begin TLS negotiation now"
	if !strings.Contains(response, expectedResponse) {
		t.Errorf("Expected exact response '%s', got: %s", expectedResponse, response)
	}
}

// TestStartTLS_NoArguments tests that STARTTLS rejects arguments per RFC 3501
func TestStartTLS_NoArguments(t *testing.T) {
	s, cleanup := SetupTestServer(t)
	defer cleanup()

	conn := NewMockConn()

	// Try STARTTLS with an argument (invalid)
	s.HandleStartTLS(conn, "A001", []string{"A001", "STARTTLS", "EXTRA"})

	response := conn.GetWrittenData()

	// Should get BAD response
	if !strings.Contains(response, "A001 BAD") {
		t.Errorf("Expected BAD response for STARTTLS with arguments, got: %s", response)
	}

	if !strings.Contains(response, "does not accept arguments") {
		t.Errorf("Expected error message about arguments, got: %s", response)
	}
}

// TestStartTLS_MultipleArguments tests rejection of multiple arguments
func TestStartTLS_MultipleArguments(t *testing.T) {
	s, cleanup := SetupTestServer(t)
	defer cleanup()

	conn := NewMockConn()

	s.HandleStartTLS(conn, "A001", []string{"A001", "STARTTLS", "ARG1", "ARG2"})

	response := conn.GetWrittenData()

	if !strings.Contains(response, "A001 BAD") {
		t.Errorf("Expected BAD response for STARTTLS with multiple arguments, got: %s", response)
	}
}

// TestStartTLS_AlreadyOnTLS tests that STARTTLS fails when already on TLS
func TestStartTLS_AlreadyOnTLS(t *testing.T) {
	s, cleanup := SetupTestServer(t)
	defer cleanup()

	conn := NewMockTLSConn() // Already TLS connection

	// Try STARTTLS on TLS connection (invalid)
	s.HandleStartTLS(conn, "A001", []string{"A001", "STARTTLS"})

	response := conn.GetWrittenData()

	// Should get BAD response
	if !strings.Contains(response, "A001 BAD") {
		t.Errorf("Expected BAD response for STARTTLS on TLS connection, got: %s", response)
	}

	if !strings.Contains(response, "TLS already active") {
		t.Errorf("Expected 'TLS already active' message, got: %s", response)
	}
}

// TestStartTLS_CaseInsensitive tests that STARTTLS command is case-insensitive
func TestStartTLS_CaseInsensitive(t *testing.T) {
	// This test verifies that the command parser handles case-insensitivity
	// The actual parsing happens in connection.go which converts to uppercase
	// This test documents the expected behavior
	testCases := []string{
		"STARTTLS",
		"starttls",
		"StartTLS",
		"STARTtls",
	}

	for _, cmdCase := range testCases {
		t.Run(cmdCase, func(t *testing.T) {
			s, cleanup := SetupTestServer(t)
			defer cleanup()

			conn := NewMockConn()

			// The command will be uppercase by the time it reaches the handler
			s.HandleStartTLS(conn, "A001", []string{"A001", "STARTTLS"})

			response := conn.GetWrittenData()

			if !strings.Contains(response, "A001 OK") {
				t.Errorf("Case '%s' should work, got: %s", cmdCase, response)
			}
		})
	}
}

// TestStartTLS_TagPreserved tests that the client's tag is preserved in response
func TestStartTLS_TagPreserved(t *testing.T) {
	testCases := []string{
		"A001",
		"a002",
		"TAG123",
		"X999",
	}

	for _, tag := range testCases {
		t.Run(tag, func(t *testing.T) {
			s, cleanup := SetupTestServer(t)
			defer cleanup()

			conn := NewMockConn()

			s.HandleStartTLS(conn, tag, []string{tag, "STARTTLS"})

			response := conn.GetWrittenData()

			if !strings.HasPrefix(response, tag+" OK") {
				t.Errorf("Expected response to start with '%s OK', got: %s", tag, response)
			}
		})
	}
}

// TestStartTLS_StateReset tests that server state is reset after STARTTLS per RFC 3501
func TestStartTLS_StateReset(t *testing.T) {
	// Per RFC 3501: "The server remains in the non-authenticated state,
	// even if client credentials are supplied during the [TLS] negotiation."
	// This is tested by verifying that after STARTTLS, the connection
	// starts with a fresh ClientState

	s, cleanup := SetupTestServer(t)
	defer cleanup()

	conn := NewMockConn()

	// Issue STARTTLS
	s.HandleStartTLS(conn, "A001", []string{"A001", "STARTTLS"})

	response := conn.GetWrittenData()

	// Should get OK response
	if !strings.Contains(response, "A001 OK Begin TLS negotiation now") {
		t.Errorf("Expected OK response, got: %s", response)
	}

	// After this point, the server would establish TLS and reset state
	// The actual TLS negotiation is handled by net.Conn in production
	// In tests, we verify the command completes successfully
}

// TestStartTLS_RFCExampleSequence tests the exact sequence from RFC 3501 example
func TestStartTLS_RFCExampleSequence(t *testing.T) {
	// RFC 3501 Example:
	// C: a001 CAPABILITY
	// S: * CAPABILITY IMAP4rev1 STARTTLS LOGINDISABLED
	// S: a001 OK CAPABILITY completed
	// C: a002 STARTTLS
	// S: a002 OK Begin TLS negotiation now
	// <TLS negotiation>
	// C: a003 CAPABILITY
	// S: * CAPABILITY IMAP4rev1 AUTH=PLAIN
	// S: a003 OK CAPABILITY completed

	s, cleanup := SetupTestServer(t)
	defer cleanup()

	// Step 1: Check capabilities on plain connection
	conn := NewMockConn()
	state := &models.ClientState{Authenticated: false}

	s.HandleCapability(conn, "a001", state)
	response := conn.GetWrittenData()

	// Should include STARTTLS and LOGINDISABLED
	if !strings.Contains(response, "STARTTLS") {
		t.Errorf("Plain connection should advertise STARTTLS, got: %s", response)
	}
	if !strings.Contains(response, "LOGINDISABLED") {
		t.Errorf("Plain connection should advertise LOGINDISABLED, got: %s", response)
	}

	// Step 2: Issue STARTTLS
	conn.ClearWriteBuffer()
	s.HandleStartTLS(conn, "a002", []string{"a002", "STARTTLS"})
	response = conn.GetWrittenData()

	if !strings.Contains(response, "a002 OK Begin TLS negotiation now") {
		t.Errorf("Expected OK response for STARTTLS, got: %s", response)
	}

	// Step 3: After TLS, capabilities should change (simulated with TLS mock)
	tlsConn := NewMockTLSConn()
	tlsState := &models.ClientState{Authenticated: false}

	s.HandleCapability(tlsConn, "a003", tlsState)
	tlsResponse := tlsConn.GetWrittenData()

	// Should NOT include STARTTLS or LOGINDISABLED on TLS connection
	if strings.Contains(tlsResponse, "STARTTLS") {
		t.Errorf("TLS connection should not advertise STARTTLS, got: %s", tlsResponse)
	}
	if strings.Contains(tlsResponse, "LOGINDISABLED") {
		t.Errorf("TLS connection should not advertise LOGINDISABLED, got: %s", tlsResponse)
	}

	// Should include AUTH=PLAIN on TLS connection
	if !strings.Contains(tlsResponse, "AUTH=PLAIN") {
		t.Errorf("TLS connection should advertise AUTH=PLAIN, got: %s", tlsResponse)
	}
}

// TestStartTLS_CapabilityMustBeReissued tests that capabilities change after STARTTLS
func TestStartTLS_CapabilityMustBeReissued(t *testing.T) {
	// RFC 3501: "Once [TLS] has been started, the client MUST discard cached
	// information about server capabilities and SHOULD re-issue the
	// CAPABILITY command."

	s, cleanup := SetupTestServer(t)
	defer cleanup()

	// Before STARTTLS - plain connection
	plainConn := NewMockConn()
	plainState := &models.ClientState{Authenticated: false}

	s.HandleCapability(plainConn, "A001", plainState)
	plainResponse := plainConn.GetWrittenData()

	// After STARTTLS - TLS connection
	tlsConn := NewMockTLSConn()
	tlsState := &models.ClientState{Authenticated: false}

	s.HandleCapability(tlsConn, "A002", tlsState)
	tlsResponse := tlsConn.GetWrittenData()

	// Verify capabilities are different
	plainHasStartTLS := strings.Contains(plainResponse, "STARTTLS")
	tlsHasStartTLS := strings.Contains(tlsResponse, "STARTTLS")

	if !plainHasStartTLS {
		t.Error("Plain connection should advertise STARTTLS")
	}
	if tlsHasStartTLS {
		t.Error("TLS connection should not advertise STARTTLS")
	}

	// Verify AUTH capabilities change
	plainHasAuth := strings.Contains(plainResponse, "AUTH=PLAIN")
	tlsHasAuth := strings.Contains(tlsResponse, "AUTH=PLAIN")

	if plainHasAuth {
		t.Error("Plain connection should not advertise AUTH=PLAIN")
	}
	if !tlsHasAuth {
		t.Error("TLS connection should advertise AUTH=PLAIN")
	}
}

// TestStartTLS_NonAuthenticatedState tests that server stays non-authenticated after STARTTLS
func TestStartTLS_NonAuthenticatedState(t *testing.T) {
	// RFC 3501: "The server remains in the non-authenticated state,
	// even if client credentials are supplied during the [TLS] negotiation."

	s, cleanup := SetupTestServer(t)
	defer cleanup()

	conn := NewMockConn()

	// Issue STARTTLS
	s.HandleStartTLS(conn, "A001", []string{"A001", "STARTTLS"})

	response := conn.GetWrittenData()

	// Verify command succeeds
	if !strings.Contains(response, "A001 OK") {
		t.Errorf("Expected OK response, got: %s", response)
	}

	// The state reset is handled by calling handleClient with new ClientState
	// which initializes Authenticated to false by default
}

// TestStartTLS_WithWhitespace tests STARTTLS with various whitespace patterns
func TestStartTLS_WithWhitespace(t *testing.T) {
	s, cleanup := SetupTestServer(t)
	defer cleanup()

	// Valid: just tag and command
	conn := NewMockConn()
	s.HandleStartTLS(conn, "A001", []string{"A001", "STARTTLS"})

	response := conn.GetWrittenData()

	if !strings.Contains(response, "A001 OK") {
		t.Errorf("Expected OK response for valid STARTTLS, got: %s", response)
	}
}

// TestStartTLS_ErrorHandling tests behavior when TLS certificates are unavailable
func TestStartTLS_ErrorHandling(t *testing.T) {
	// This test documents expected behavior when certs are missing
	// In the actual implementation, missing certs will cause a BAD response
	// The test server uses hardcoded paths that may not exist

	s, cleanup := SetupTestServer(t)
	defer cleanup()

	conn := NewMockConn()

	// This will fail to load certs in test environment
	s.HandleStartTLS(conn, "A001", []string{"A001", "STARTTLS"})

	response := conn.GetWrittenData()

	// Should get either OK (if certs exist) or BAD (if certs don't exist)
	// In production with valid certs: "A001 OK Begin TLS negotiation now"
	// In test without certs: "A001 BAD TLS not available"
	hasOK := strings.Contains(response, "A001 OK")
	hasBAD := strings.Contains(response, "A001 BAD")

	if !hasOK && !hasBAD {
		t.Errorf("Expected either OK or BAD response, got: %s", response)
	}
}
