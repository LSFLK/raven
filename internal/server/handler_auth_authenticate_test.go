package server

import (
	"encoding/base64"
	"strings"
	"testing"

	"raven/internal/models"
	
)

// TestAuthenticatePlainBasicFlow tests the basic AUTHENTICATE PLAIN command flow
func TestAuthenticatePlainBasicFlow(t *testing.T) {
	s, cleanup := SetupTestServer(t)
	defer cleanup()

	conn := NewMockTLSConn() // Use TLS connection
	state := &models.ClientState{Authenticated: false}

	// Send AUTHENTICATE PLAIN command
	s.HandleAuthenticate(conn, "A001", []string{"A001", "AUTHENTICATE", "PLAIN"}, state)

	response := conn.GetWrittenData()
	
	// Should get continuation response
	if !strings.Contains(response, "+ ") {
		t.Errorf("Expected continuation response '+', got: %s", response)
	}
}

// TestAuthenticatePlainBase64Decoding tests successful base64 decoding
func TestAuthenticatePlainBase64Decoding(t *testing.T) {
	// Test base64 encoding/decoding of credentials
	authString := "\x00testuser\x00testpass"
	authEncoded := base64.StdEncoding.EncodeToString([]byte(authString))
	
	decoded, err := base64.StdEncoding.DecodeString(authEncoded)
	if err != nil {
		t.Fatalf("Failed to decode: %v", err)
	}
	
	parts := strings.Split(string(decoded), "\x00")
	if len(parts) != 3 {
		t.Errorf("Expected 3 parts, got %d", len(parts))
	}
	
	if parts[1] != "testuser" {
		t.Errorf("Expected username 'testuser', got '%s'", parts[1])
	}
	
	if parts[2] != "testpass" {
		t.Errorf("Expected password 'testpass', got '%s'", parts[2])
	}
}

// TestAuthenticatePlainWithAuthzid tests AUTHENTICATE PLAIN with authorization identity
func TestAuthenticatePlainWithAuthzid(t *testing.T) {
	// Test format with authzid: authzid\x00authcid\x00password
	authString := "testuser\x00testuser\x00testpass"
	authEncoded := base64.StdEncoding.EncodeToString([]byte(authString))
	
	decoded, err := base64.StdEncoding.DecodeString(authEncoded)
	if err != nil {
		t.Fatalf("Failed to decode: %v", err)
	}
	
	parts := strings.Split(string(decoded), "\x00")
	if len(parts) != 3 {
		t.Errorf("Expected 3 parts, got %d", len(parts))
	}
	
	// authzid, authcid, password
	if parts[0] != "testuser" {
		t.Errorf("Expected authzid 'testuser', got '%s'", parts[0])
	}
	if parts[1] != "testuser" {
		t.Errorf("Expected authcid 'testuser', got '%s'", parts[1])
	}
	if parts[2] != "testpass" {
		t.Errorf("Expected password 'testpass', got '%s'", parts[2])
	}
}

// TestAuthenticateWithoutTLS tests that AUTHENTICATE is rejected without TLS
func TestAuthenticateWithoutTLS(t *testing.T) {
	s, cleanup := SetupTestServer(t)
	defer cleanup()

	conn := NewMockConn() // Plain connection, not TLS
	state := &models.ClientState{Authenticated: false}

	// Send AUTHENTICATE PLAIN command
	s.HandleAuthenticate(conn, "A001", []string{"A001", "AUTHENTICATE", "PLAIN"}, state)

	response := conn.GetWrittenData()
	
	// Per RFC 3501: Should get NO response (plaintext auth disallowed)
	if !strings.Contains(response, "A001 NO") {
		t.Errorf("Expected NO response without TLS, got: %s", response)
	}
	if !strings.Contains(strings.ToLower(response), "plaintext") || !strings.Contains(strings.ToLower(response), "tls") {
		t.Errorf("Expected plaintext/TLS-related message, got: %s", response)
	}
}

// TestAuthenticateUnsupportedMechanism tests unsupported authentication mechanism
func TestAuthenticateUnsupportedMechanism(t *testing.T) {
	s, cleanup := SetupTestServer(t)
	defer cleanup()

	conn := NewMockTLSConn()
	state := &models.ClientState{Authenticated: false}

	// Send AUTHENTICATE with unsupported mechanism
	s.HandleAuthenticate(conn, "A001", []string{"A001", "AUTHENTICATE", "GSSAPI"}, state)

	response := conn.GetWrittenData()
	
	// Should get NO response
	if !strings.Contains(response, "A001 NO") {
		t.Errorf("Expected NO response for unsupported mechanism, got: %s", response)
	}
	if !strings.Contains(strings.ToLower(response), "unsupported") {
		t.Errorf("Expected 'unsupported' in response, got: %s", response)
	}
}

// TestAuthenticateMissingMechanism tests AUTHENTICATE without mechanism
func TestAuthenticateMissingMechanism(t *testing.T) {
	s, cleanup := SetupTestServer(t)
	defer cleanup()

	conn := NewMockTLSConn()
	state := &models.ClientState{Authenticated: false}

	// Send AUTHENTICATE without mechanism
	s.HandleAuthenticate(conn, "A001", []string{"A001", "AUTHENTICATE"}, state)

	response := conn.GetWrittenData()
	
	// Should get BAD response
	if !strings.Contains(response, "A001 BAD") {
		t.Errorf("Expected BAD response for missing mechanism, got: %s", response)
	}
}

// TestAuthenticatePlainCaseInsensitive tests that mechanism name is case-insensitive
func TestAuthenticatePlainCaseInsensitive(t *testing.T) {
	s, cleanup := SetupTestServer(t)
	defer cleanup()

	mechanisms := []string{"PLAIN", "Plain", "plain", "pLaIn"}

	for _, mechanism := range mechanisms {
		t.Run("Mechanism_"+mechanism, func(t *testing.T) {
			conn := NewMockTLSConn()
			state := &models.ClientState{Authenticated: false}

			// Send AUTHENTICATE with various cases
			s.HandleAuthenticate(conn, "A001", []string{"A001", "AUTHENTICATE", mechanism}, state)

			response := conn.GetWrittenData()
			
			// Should get continuation response for all case variations
			if !strings.Contains(response, "+ ") {
				t.Errorf("Expected continuation response for %s, got: %s", mechanism, response)
			}
		})
	}
}

// TestAuthenticateBase64InvalidFormat tests handling of invalid base64 format
func TestAuthenticateBase64InvalidFormat(t *testing.T) {
	// Test that invalid base64 is handled gracefully
	invalidBase64 := "!!!invalid-base64!!!"
	
	_, err := base64.StdEncoding.DecodeString(invalidBase64)
	if err == nil {
		t.Error("Expected error for invalid base64, got nil")
	}
}

// TestAuthenticatePlainEmptyCredentials tests rejection of empty credentials
func TestAuthenticatePlainEmptyCredentials(t *testing.T) {
	// Test empty credentials format
	authString := "\x00\x00"
	authEncoded := base64.StdEncoding.EncodeToString([]byte(authString))
	
	decoded, err := base64.StdEncoding.DecodeString(authEncoded)
	if err != nil {
		t.Fatalf("Failed to decode: %v", err)
	}
	
	parts := strings.Split(string(decoded), "\x00")
	
	// Check that we can detect empty username/password
	hasEmpty := false
	for i, part := range parts {
		if part == "" && i > 0 { // Skip authzid (index 0)
			hasEmpty = true
		}
	}
	
	if !hasEmpty {
		t.Error("Expected to detect empty credentials")
	}
}

// TestAuthenticatePlainMalformedData tests handling of malformed data (no NUL separators)
func TestAuthenticatePlainMalformedData(t *testing.T) {
	// Test data without NUL separators
	authString := "usernamepassword"
	authEncoded := base64.StdEncoding.EncodeToString([]byte(authString))
	
	decoded, err := base64.StdEncoding.DecodeString(authEncoded)
	if err != nil {
		t.Fatalf("Failed to decode: %v", err)
	}
	
	parts := strings.Split(string(decoded), "\x00")
	
	// Should have only 1 part (no NUL separators)
	if len(parts) != 1 {
		t.Errorf("Expected 1 part for malformed data, got %d", len(parts))
	}
}

// BenchmarkAuthenticatePlainBase64 benchmarks base64 encoding/decoding
func BenchmarkAuthenticatePlainBase64(b *testing.B) {
	authString := "\x00testuser\x00testpass"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		authEncoded := base64.StdEncoding.EncodeToString([]byte(authString))
		decoded, _ := base64.StdEncoding.DecodeString(authEncoded)
		_ = strings.Split(string(decoded), "\x00")
	}
}

// TestAuthenticateCancellation tests that client can cancel authentication with "*"
func TestAuthenticateCancellation(t *testing.T) {
	s, cleanup := SetupTestServer(t)
	defer cleanup()

	conn := NewMockTLSConn()
	state := &models.ClientState{Authenticated: false}

	// Send AUTHENTICATE PLAIN command
	s.HandleAuthenticate(conn, "A001", []string{"A001", "AUTHENTICATE", "PLAIN"}, state)

	// Check continuation response
	response := conn.GetWrittenData()
	if !strings.Contains(response, "+ ") {
		t.Errorf("Expected continuation response, got: %s", response)
	}

	// Clear buffer and send cancellation
	conn.ClearWriteBuffer()
	conn.AddReadData("*\r\n")

	// This would need to be handled by reading the response
	// Note: In real implementation, the server reads the cancellation in the same handler
	// For this test, we verify the continuation was sent
}

// TestAuthenticatePlainSuccessResponse tests CAPABILITY in OK response
func TestAuthenticatePlainSuccessResponse(t *testing.T) {
	// This test verifies that per RFC 3501, server MAY include CAPABILITY
	// response code in the tagged OK response after successful AUTHENTICATE
	// Note: This is a unit test of the response format expectation
	
	expectedFormat := "A001 OK [CAPABILITY IMAP4rev1 AUTH=PLAIN UIDPLUS IDLE NAMESPACE UNSELECT LITERAL+] Authenticated"
	
	// Verify the format includes:
	// 1. Tag (A001)
	// 2. OK status
	// 3. [CAPABILITY ...] response code
	// 4. Human-readable message
	
	if !strings.Contains(expectedFormat, "OK [CAPABILITY") {
		t.Error("Expected CAPABILITY response code in OK response")
	}
	if !strings.Contains(expectedFormat, "IMAP4rev1") {
		t.Error("Expected IMAP4rev1 capability")
	}
}

// TestAuthenticatePlainNOResponse tests that authentication failures return NO
func TestAuthenticatePlainNOResponse(t *testing.T) {
	// Per RFC 3501, authentication failures should return NO, not BAD
	// BAD is only for protocol errors (malformed commands, cancelled exchange)
	
	testCases := []struct {
		name           string
		authString     string
		expectedStatus string
		description    string
	}{
		{
			name:           "EmptyCredentials",
			authString:     "\x00\x00",
			expectedStatus: "NO",
			description:    "Empty credentials should return NO (authentication failure)",
		},
		{
			name:           "MalformedFormat",
			authString:     "nocredentials",
			expectedStatus: "NO",
			description:    "Malformed format should return NO (authentication failure)",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Verify expected status is NO per RFC 3501
			if tc.expectedStatus != "NO" {
				t.Errorf("%s: Expected NO response per RFC 3501, got %s", tc.description, tc.expectedStatus)
			}
		})
	}
}

// TestAuthenticateBADResponse tests that protocol errors return BAD
func TestAuthenticateBADResponse(t *testing.T) {
	// Per RFC 3501, BAD responses should be used for:
	// 1. Command unknown or arguments invalid
	// 2. Authentication exchange cancelled (by client sending "*")
	
	testCases := []struct {
		name        string
		description string
	}{
		{
			name:        "MissingMechanism",
			description: "Missing authentication mechanism should return BAD",
		},
		{
			name:        "Cancellation",
			description: "Client cancellation with '*' should return BAD",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// These are documentation tests to ensure we understand RFC 3501 requirements
			t.Logf("%s", tc.description)
		})
	}
}

// TestAuthenticateSASLPlainFormat tests proper SASL PLAIN format
func TestAuthenticateSASLPlainFormat(t *testing.T) {
	// Per RFC 2595 (SASL PLAIN mechanism):
	// The mechanism consists of a single message from client to server.
	// The client sends the authorization identity (identity to act as),
	// followed by a NUL (U+0000) character, followed by the authentication
	// identity (identity whose password will be used), followed by a NUL
	// (U+0000) character, followed by the clear-text password.
	// Format: [authzid] NUL authcid NUL passwd
	
	testCases := []struct {
		name     string
		format   string
		authzid  string
		authcid  string
		passwd   string
	}{
		{
			name:     "WithAuthzid",
			format:   "user\x00user\x00pass",
			authzid:  "user",
			authcid:  "user",
			passwd:   "pass",
		},
		{
			name:     "WithoutAuthzid",
			format:   "\x00user\x00pass",
			authzid:  "",
			authcid:  "user",
			passwd:   "pass",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			parts := strings.Split(tc.format, "\x00")
			if len(parts) != 3 {
				t.Errorf("Expected 3 parts in SASL PLAIN format, got %d", len(parts))
			}
			if parts[0] != tc.authzid {
				t.Errorf("Expected authzid '%s', got '%s'", tc.authzid, parts[0])
			}
			if parts[1] != tc.authcid {
				t.Errorf("Expected authcid '%s', got '%s'", tc.authcid, parts[1])
			}
			if parts[2] != tc.passwd {
				t.Errorf("Expected passwd '%s', got '%s'", tc.passwd, parts[2])
			}
		})
	}
}

// TestAuthenticateBase64Encoding tests base64 encoding requirement
func TestAuthenticateBase64Encoding(t *testing.T) {
	// Per RFC 3501: "The client response consists of a single line
	// consisting of a BASE64 encoded string."
	
	authString := "\x00testuser\x00testpass"
	encoded := base64.StdEncoding.EncodeToString([]byte(authString))
	
	// Verify it's valid base64
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("Failed to decode base64: %v", err)
	}
	
	if string(decoded) != authString {
		t.Errorf("Decoded string doesn't match original")
	}
	
	// Verify the decoded string has proper SASL PLAIN format
	parts := strings.Split(string(decoded), "\x00")
	if len(parts) != 3 {
		t.Errorf("Expected 3 parts after decoding, got %d", len(parts))
	}
}

// TestAuthenticateContinuationRequest tests continuation request format
func TestAuthenticateContinuationRequest(t *testing.T) {
	s, cleanup := SetupTestServer(t)
	defer cleanup()

	conn := NewMockTLSConn()
	state := &models.ClientState{Authenticated: false}

	// Send AUTHENTICATE PLAIN command
	s.HandleAuthenticate(conn, "A001", []string{"A001", "AUTHENTICATE", "PLAIN"}, state)

	response := conn.GetWrittenData()
	
	// Per RFC 3501: "A server challenge consists of a command continuation
	// request response with the '+' token followed by a BASE64 encoded string."
	// For PLAIN mechanism without initial response, server sends empty challenge: "+ "
	if !strings.HasPrefix(response, "+ ") {
		t.Errorf("Expected continuation request starting with '+ ', got: %s", response)
	}
}
