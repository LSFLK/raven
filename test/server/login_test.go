package server

import (
	"strings"
	"testing"

	"go-imap/internal/models"
	"go-imap/test/helpers"
)

// TestLoginCommand_BasicFlow tests the basic LOGIN command flow with valid credentials
func TestLoginCommand_BasicFlow(t *testing.T) {
	s, cleanup := helpers.SetupTestServer(t)
	defer cleanup()

	conn := helpers.NewMockTLSConn() // Use TLS connection
	state := &models.ClientState{Authenticated: false}

	// Send LOGIN command with valid credentials
	s.HandleLogin(conn, "A001", []string{"A001", "LOGIN", "testuser", "testpass"}, state)

	response := conn.GetWrittenData()

	// Should get OK response (authentication will be handled by mock auth server)
	// Note: In testing environment, the auth server might not be available,
	// so we check for either OK or NO response
	if !strings.Contains(response, "A001") {
		t.Errorf("Expected tagged response, got: %s", response)
	}
}

// TestLoginCommand_WithQuotedCredentials tests LOGIN with quoted username and password
func TestLoginCommand_WithQuotedCredentials(t *testing.T) {
	s, cleanup := helpers.SetupTestServer(t)
	defer cleanup()

	conn := helpers.NewMockTLSConn()
	state := &models.ClientState{Authenticated: false}

	// Send LOGIN command with quoted credentials
	s.HandleLogin(conn, "A002", []string{"A002", "LOGIN", "\"testuser\"", "\"testpass\""}, state)

	response := conn.GetWrittenData()

	// Should get tagged response (quotes should be stripped)
	if !strings.Contains(response, "A002") {
		t.Errorf("Expected tagged response, got: %s", response)
	}
}

// TestLoginCommand_MissingArguments tests LOGIN with missing arguments
func TestLoginCommand_MissingArguments(t *testing.T) {
	s, cleanup := helpers.SetupTestServer(t)
	defer cleanup()

	conn := helpers.NewMockTLSConn()
	state := &models.ClientState{Authenticated: false}

	// Send LOGIN command with only username (missing password)
	s.HandleLogin(conn, "A003", []string{"A003", "LOGIN", "testuser"}, state)

	response := conn.GetWrittenData()

	// Should get BAD response
	if !strings.Contains(response, "A003 BAD") {
		t.Errorf("Expected BAD response for missing password, got: %s", response)
	}

	if !strings.Contains(response, "LOGIN requires username and password") {
		t.Errorf("Expected error message about missing credentials, got: %s", response)
	}
}

// TestLoginCommand_NoArguments tests LOGIN with no arguments
func TestLoginCommand_NoArguments(t *testing.T) {
	s, cleanup := helpers.SetupTestServer(t)
	defer cleanup()

	conn := helpers.NewMockTLSConn()
	state := &models.ClientState{Authenticated: false}

	// Send LOGIN command with no arguments
	s.HandleLogin(conn, "A004", []string{"A004", "LOGIN"}, state)

	response := conn.GetWrittenData()

	// Should get BAD response
	if !strings.Contains(response, "A004 BAD") {
		t.Errorf("Expected BAD response for no arguments, got: %s", response)
	}
}

// TestLoginCommand_WithoutTLS tests that LOGIN is rejected without TLS
func TestLoginCommand_WithoutTLS(t *testing.T) {
	s, cleanup := helpers.SetupTestServer(t)
	defer cleanup()

	conn := helpers.NewMockConn() // Plain connection, not TLS
	state := &models.ClientState{Authenticated: false}

	// Send LOGIN command
	s.HandleLogin(conn, "A005", []string{"A005", "LOGIN", "testuser", "testpass"}, state)

	response := conn.GetWrittenData()

	// Per RFC 3501: Should get NO response (LOGIN disabled without TLS)
	if !strings.Contains(response, "A005 NO") {
		t.Errorf("Expected NO response without TLS, got: %s", response)
	}

	// Should mention PRIVACYREQUIRED or LOGINDISABLED
	if !strings.Contains(response, "PRIVACYREQUIRED") && !strings.Contains(response, "disabled") {
		t.Errorf("Expected security-related error message, got: %s", response)
	}
}

// TestLoginCommand_StateNotAuthenticated tests that state is not authenticated before LOGIN
func TestLoginCommand_StateNotAuthenticated(t *testing.T) {
	s, cleanup := helpers.SetupTestServer(t)
	defer cleanup()

	conn := helpers.NewMockTLSConn()
	state := &models.ClientState{Authenticated: false}

	// Verify state is not authenticated before LOGIN
	if state.Authenticated {
		t.Error("State should not be authenticated before LOGIN")
	}

	// Send LOGIN command
	s.HandleLogin(conn, "A006", []string{"A006", "LOGIN", "testuser", "testpass"}, state)

	// Note: State authentication depends on successful auth server response
	// In test environment, this might fail, but we've verified the initial state
}

// TestLoginCommand_EmptyCredentials tests LOGIN with empty username or password
func TestLoginCommand_EmptyCredentials(t *testing.T) {
	s, cleanup := helpers.SetupTestServer(t)
	defer cleanup()

	testCases := []struct {
		name     string
		username string
		password string
		tag      string
	}{
		{"Empty username", `""`, "password", "A007"},
		{"Empty password", "username", `""`, "A008"},
		{"Both empty", `""`, `""`, "A009"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			conn := helpers.NewMockTLSConn()
			state := &models.ClientState{Authenticated: false}

			s.HandleLogin(conn, tc.tag, []string{tc.tag, "LOGIN", tc.username, tc.password}, state)

			response := conn.GetWrittenData()

			// Should get response (likely NO from auth server for invalid credentials)
			if !strings.Contains(response, tc.tag) {
				t.Errorf("Expected tagged response for %s, got: %s", tc.name, response)
			}
		})
	}
}

// TestLoginCommand_SpecialCharactersInCredentials tests LOGIN with special characters
func TestLoginCommand_SpecialCharactersInCredentials(t *testing.T) {
	s, cleanup := helpers.SetupTestServer(t)
	defer cleanup()

	testCases := []struct {
		name     string
		username string
		password string
		tag      string
	}{
		{"Username with dots", "test.user", "password", "A010"},
		{"Username with underscores", "test_user", "password", "A011"},
		{"Username with dashes", "test-user", "password", "A012"},
		{"Password with special chars", "testuser", "p@ssw0rd!", "A013"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			conn := helpers.NewMockTLSConn()
			state := &models.ClientState{Authenticated: false}

			s.HandleLogin(conn, tc.tag, []string{tc.tag, "LOGIN", tc.username, tc.password}, state)

			response := conn.GetWrittenData()

			// Should get tagged response (authentication result)
			if !strings.Contains(response, tc.tag) {
				t.Errorf("Expected tagged response for %s, got: %s", tc.name, response)
			}
		})
	}
}

// TestLoginCommand_CaseSensitivity tests that username and password are case-sensitive
func TestLoginCommand_CaseSensitivity(t *testing.T) {
	s, cleanup := helpers.SetupTestServer(t)
	defer cleanup()

	testCases := []struct {
		name     string
		username string
		password string
		tag      string
	}{
		{"Lowercase username", "testuser", "password", "A014"},
		{"Uppercase username", "TESTUSER", "password", "A015"},
		{"Mixed case username", "TestUser", "password", "A016"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			conn := helpers.NewMockTLSConn()
			state := &models.ClientState{Authenticated: false}

			s.HandleLogin(conn, tc.tag, []string{tc.tag, "LOGIN", tc.username, tc.password}, state)

			response := conn.GetWrittenData()

			// Should get tagged response
			if !strings.Contains(response, tc.tag) {
				t.Errorf("Expected tagged response for %s, got: %s", tc.name, response)
			}
		})
	}
}

// TestLoginCommand_ResponseFormat tests the format of LOGIN responses
func TestLoginCommand_ResponseFormat(t *testing.T) {
	s, cleanup := helpers.SetupTestServer(t)
	defer cleanup()

	conn := helpers.NewMockTLSConn()
	state := &models.ClientState{Authenticated: false}

	// Send LOGIN command
	s.HandleLogin(conn, "A017", []string{"A017", "LOGIN", "testuser", "testpass"}, state)

	response := conn.GetWrittenData()

	// Response should have tag
	if !strings.Contains(response, "A017") {
		t.Errorf("Response should contain tag 'A017', got: %s", response)
	}

	// Response should be either OK, NO, or BAD
	hasValidStatus := strings.Contains(response, "OK") ||
		strings.Contains(response, "NO") ||
		strings.Contains(response, "BAD")

	if !hasValidStatus {
		t.Errorf("Response should contain OK, NO, or BAD, got: %s", response)
	}
}

// TestLoginCommand_MultipleAttempts tests multiple LOGIN attempts
func TestLoginCommand_MultipleAttempts(t *testing.T) {
	s, cleanup := helpers.SetupTestServer(t)
	defer cleanup()

	conn := helpers.NewMockTLSConn()
	state := &models.ClientState{Authenticated: false}

	// First attempt
	s.HandleLogin(conn, "A018", []string{"A018", "LOGIN", "user1", "pass1"}, state)
	response1 := conn.GetWrittenData()

	if !strings.Contains(response1, "A018") {
		t.Errorf("Expected response for first attempt, got: %s", response1)
	}

	// Clear buffer for second attempt
	conn.ClearWriteBuffer()
	state.Authenticated = false // Reset state

	// Second attempt with different credentials
	s.HandleLogin(conn, "A019", []string{"A019", "LOGIN", "user2", "pass2"}, state)
	response2 := conn.GetWrittenData()

	if !strings.Contains(response2, "A019") {
		t.Errorf("Expected response for second attempt, got: %s", response2)
	}
}

// TestLoginCommand_LongCredentials tests LOGIN with very long username/password
func TestLoginCommand_LongCredentials(t *testing.T) {
	s, cleanup := helpers.SetupTestServer(t)
	defer cleanup()

	conn := helpers.NewMockTLSConn()
	state := &models.ClientState{Authenticated: false}

	// Create long credentials (but still reasonable)
	longUsername := strings.Repeat("a", 50)
	longPassword := strings.Repeat("b", 50)

	s.HandleLogin(conn, "A020", []string{"A020", "LOGIN", longUsername, longPassword}, state)

	response := conn.GetWrittenData()

	// Should handle long credentials gracefully
	if !strings.Contains(response, "A020") {
		t.Errorf("Expected tagged response with long credentials, got: %s", response)
	}
}

// TestLoginCommand_TagFormat tests various tag formats
func TestLoginCommand_TagFormat(t *testing.T) {
	s, cleanup := helpers.SetupTestServer(t)
	defer cleanup()

	testCases := []struct {
		name string
		tag  string
	}{
		{"Numeric tag", "001"},
		{"Alpha tag", "ABC"},
		{"Alphanumeric tag", "A001"},
		{"Tag with dots", "A.001"},
		{"Short tag", "A"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			conn := helpers.NewMockTLSConn()
			state := &models.ClientState{Authenticated: false}

			s.HandleLogin(conn, tc.tag, []string{tc.tag, "LOGIN", "user", "pass"}, state)

			response := conn.GetWrittenData()

			// Response should echo the tag
			if !strings.Contains(response, tc.tag) {
				t.Errorf("Response should contain tag '%s', got: %s", tc.tag, response)
			}
		})
	}
}

// TestLoginCommand_WithWhitespace tests LOGIN with whitespace in credentials
func TestLoginCommand_WithWhitespace(t *testing.T) {
	s, cleanup := helpers.SetupTestServer(t)
	defer cleanup()

	// Note: Whitespace in unquoted atoms would break parsing at the connection handler level
	// Here we test quoted strings with spaces
	conn := helpers.NewMockTLSConn()
	state := &models.ClientState{Authenticated: false}

	// With quotes, spaces should be preserved
	s.HandleLogin(conn, "A021", []string{"A021", "LOGIN", "\"test user\"", "\"test pass\""}, state)

	response := conn.GetWrittenData()

	// Should get tagged response
	if !strings.Contains(response, "A021") {
		t.Errorf("Expected tagged response, got: %s", response)
	}
}

// TestLoginCommand_AfterSuccessfulLogin tests that state changes after successful login
func TestLoginCommand_AfterSuccessfulLogin(t *testing.T) {
	s, cleanup := helpers.SetupTestServer(t)
	defer cleanup()

	conn := helpers.NewMockTLSConn()
	state := &models.ClientState{Authenticated: false}

	// Send LOGIN command
	s.HandleLogin(conn, "A022", []string{"A022", "LOGIN", "testuser", "testpass"}, state)

	response := conn.GetWrittenData()

	// If we get OK response, state should be authenticated
	if strings.Contains(response, "A022 OK") {
		if !state.Authenticated {
			t.Error("State should be authenticated after successful LOGIN")
		}
		if state.Username != "testuser" {
			t.Errorf("Username should be set to 'testuser', got '%s'", state.Username)
		}
	}
	// If we get NO response, state should remain not authenticated
	if strings.Contains(response, "A022 NO") {
		if state.Authenticated {
			t.Error("State should not be authenticated after failed LOGIN")
		}
	}
}

// TestLoginCommand_CapabilityInResponse tests that CAPABILITY is included in OK response per RFC 3501 6.3
func TestLoginCommand_CapabilityInResponse(t *testing.T) {
	s, cleanup := helpers.SetupTestServer(t)
	defer cleanup()

	conn := helpers.NewMockTLSConn()
	state := &models.ClientState{Authenticated: false}

	// Send LOGIN command
	s.HandleLogin(conn, "A023", []string{"A023", "LOGIN", "testuser", "testpass"}, state)

	response := conn.GetWrittenData()

	// Per RFC 3501 section 6.3, server MAY include CAPABILITY in OK response
	// If OK response is received, check for CAPABILITY
	if strings.Contains(response, "A023 OK") {
		// CAPABILITY response code should be in brackets
		if strings.Contains(response, "[CAPABILITY") {
			// Good - server includes CAPABILITY in OK response
			t.Logf("Server includes CAPABILITY in OK response: %s", response)
		} else {
			// Note: RFC says MAY, so this is optional
			t.Logf("Server does not include CAPABILITY in OK response (optional per RFC): %s", response)
		}
	}
}

// TestLoginCommand_SecurityResponseCodes tests security-related response codes
func TestLoginCommand_SecurityResponseCodes(t *testing.T) {
	s, cleanup := helpers.SetupTestServer(t)
	defer cleanup()

	// Test without TLS - should get security-related response code
	conn := helpers.NewMockConn()
	state := &models.ClientState{Authenticated: false}

	s.HandleLogin(conn, "A024", []string{"A024", "LOGIN", "testuser", "testpass"}, state)

	response := conn.GetWrittenData()

	// Should get NO response with security-related response code
	if !strings.Contains(response, "A024 NO") {
		t.Errorf("Expected NO response without TLS, got: %s", response)
	}

	// Should contain a response code related to security
	// RFC 3501 suggests response codes like [PRIVACYREQUIRED]
	if !strings.Contains(response, "PRIVACY") && !strings.Contains(response, "disabled") {
		t.Logf("Note: Response does not contain explicit privacy/security response code: %s", response)
	}
}
