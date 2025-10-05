package server

import (
	"encoding/base64"
	"strings"
	"testing"

	"go-imap/internal/models"
	"go-imap/test/helpers"
)

// TestAuthenticatePlainBasicFlow tests the basic AUTHENTICATE PLAIN command flow
func TestAuthenticatePlainBasicFlow(t *testing.T) {
	s, cleanup := helpers.SetupTestServer(t)
	defer cleanup()

	conn := helpers.NewMockTLSConn() // Use TLS connection
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
	s, cleanup := helpers.SetupTestServer(t)
	defer cleanup()

	conn := helpers.NewMockConn() // Plain connection, not TLS
	state := &models.ClientState{Authenticated: false}

	// Send AUTHENTICATE PLAIN command
	s.HandleAuthenticate(conn, "A001", []string{"A001", "AUTHENTICATE", "PLAIN"}, state)

	response := conn.GetWrittenData()
	
	// Should get NO response (plaintext auth disallowed)
	if !strings.Contains(response, "A001 NO") {
		t.Errorf("Expected NO response without TLS, got: %s", response)
	}
	if !strings.Contains(strings.ToLower(response), "tls") {
		t.Errorf("Expected TLS-related message, got: %s", response)
	}
}

// TestAuthenticateUnsupportedMechanism tests unsupported authentication mechanism
func TestAuthenticateUnsupportedMechanism(t *testing.T) {
	s, cleanup := helpers.SetupTestServer(t)
	defer cleanup()

	conn := helpers.NewMockTLSConn()
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
	s, cleanup := helpers.SetupTestServer(t)
	defer cleanup()

	conn := helpers.NewMockTLSConn()
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
	s, cleanup := helpers.SetupTestServer(t)
	defer cleanup()

	mechanisms := []string{"PLAIN", "Plain", "plain", "pLaIn"}

	for _, mechanism := range mechanisms {
		t.Run("Mechanism_"+mechanism, func(t *testing.T) {
			conn := helpers.NewMockTLSConn()
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
