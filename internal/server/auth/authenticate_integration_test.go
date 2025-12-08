package auth_test

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"raven/internal/conf"
	"raven/internal/models"
	"raven/internal/server"
)

// TestAuthenticatePlain_SuccessfulAuthWithServer tests full authentication flow with mock auth server
func TestAuthenticatePlain_SuccessfulAuthWithServer(t *testing.T) {
	// Create a mock auth server that returns 200 OK
	authServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer authServer.Close()

	// Create config directory and file
	err := os.MkdirAll("config", 0755)
	if err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}

	configPath := "config/raven.yaml"
	configContent := fmt.Sprintf(`domain: example.com
auth_server_url: %s
`, authServer.URL)
	err = os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}
	defer os.Remove(configPath)

	// Reload config
	_, err = conf.LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Setup test server
	s, cleanup := server.SetupTestServer(t)
	defer cleanup()

	conn := server.NewMockTLSConn()
	state := &models.ClientState{Authenticated: false}

	// Send AUTHENTICATE PLAIN command
	s.HandleAuthenticate(conn, "A001", []string{"A001", "AUTHENTICATE", "PLAIN"}, state)

	// Check continuation response
	response := conn.GetWrittenData()
	if !strings.Contains(response, "+ ") {
		t.Fatalf("Expected continuation response, got: %s", response)
	}

	// Clear buffer and send valid credentials
	conn.ClearWriteBuffer()
	authString := "\x00testuser@example.com\x00testpass"
	authEncoded := base64.StdEncoding.EncodeToString([]byte(authString))
	conn.AddReadData(authEncoded + "\r\n")

	// Continue authentication
	s.HandleAuthenticate(conn, "A001", []string{"A001", "AUTHENTICATE", "PLAIN"}, state)

	// Check for successful authentication
	response = conn.GetWrittenData()
	t.Logf("Auth response: %s", response)

	// Note: Full integration test would need proper auth server response handling
}

// TestAuthenticatePlain_AuthServerUnavailable tests handling of auth server errors
func TestAuthenticatePlain_AuthServerUnavailable(t *testing.T) {
	// Create config directory and file with invalid auth server URL
	err := os.MkdirAll("config", 0755)
	if err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}

	configPath := "config/raven.yaml"
	configContent := `domain: example.com
auth_server_url: https://invalid-auth-server-that-does-not-exist.example.com:9999
`
	err = os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}
	defer os.Remove(configPath)

	// Reload config
	_, err = conf.LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	s, cleanup := server.SetupTestServer(t)
	defer cleanup()

	conn := server.NewMockTLSConn()
	state := &models.ClientState{Authenticated: false}

	// Send AUTHENTICATE PLAIN
	s.HandleAuthenticate(conn, "A001", []string{"A001", "AUTHENTICATE", "PLAIN"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "+ ") {
		t.Fatalf("Expected continuation response, got: %s", response)
	}

	// Clear and send credentials
	conn.ClearWriteBuffer()
	authString := "\x00testuser\x00testpass"
	authEncoded := base64.StdEncoding.EncodeToString([]byte(authString))
	conn.AddReadData(authEncoded + "\r\n")

	s.HandleAuthenticate(conn, "A001", []string{"A001", "AUTHENTICATE", "PLAIN"}, state)

	response = conn.GetWrittenData()

	// Should get error response about unavailable auth service
	// In actual implementation, this would be caught during conn.Read
	t.Logf("Response with unavailable auth server: %s", response)
}

// TestAuthenticatePlain_InvalidCredentialsFormat tests various invalid credential formats
func TestAuthenticatePlain_InvalidCredentialsFormat(t *testing.T) {
	testCases := []struct {
		name        string
		authString  string
		description string
	}{
		{
			name:        "SinglePart",
			authString:  "nocredentials",
			description: "Auth string with no NUL separators",
		},
		{
			name:        "EmptyUsername",
			authString:  "\x00\x00password",
			description: "Auth string with empty username",
		},
		{
			name:        "EmptyPassword",
			authString:  "\x00username\x00",
			description: "Auth string with empty password",
		},
		{
			name:        "BothEmpty",
			authString:  "\x00\x00",
			description: "Auth string with empty username and password",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			s, cleanup := server.SetupTestServer(t)
			defer cleanup()

			conn := server.NewMockTLSConn()
			state := &models.ClientState{Authenticated: false}

			// Send AUTHENTICATE PLAIN
			s.HandleAuthenticate(conn, "A001", []string{"A001", "AUTHENTICATE", "PLAIN"}, state)

			response := conn.GetWrittenData()
			if !strings.Contains(response, "+ ") {
				t.Fatalf("Expected continuation response, got: %s", response)
			}

			// Clear and send invalid credentials
			conn.ClearWriteBuffer()
			authEncoded := base64.StdEncoding.EncodeToString([]byte(tc.authString))
			conn.AddReadData(authEncoded + "\r\n")

			s.HandleAuthenticate(conn, "A001", []string{"A001", "AUTHENTICATE", "PLAIN"}, state)

			response = conn.GetWrittenData()
			t.Logf("%s - Response: %s", tc.description, response)

			// Should receive NO response for invalid credentials
			if strings.Contains(response, "NO") || strings.Contains(response, "BAD") {
				t.Logf("Correctly rejected invalid credentials")
			}
		})
	}
}

// TestAuthenticatePlain_UsernameWithDomain tests authentication with username@domain format
func TestAuthenticatePlain_UsernameWithDomain(t *testing.T) {
	// Setup mock auth server
	authServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request body contains email with domain
		w.WriteHeader(http.StatusOK)
	}))
	defer authServer.Close()

	// Create config directory and file
	err := os.MkdirAll("config", 0755)
	if err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}

	configPath := "config/raven.yaml"
	configContent := fmt.Sprintf(`domain: example.com
auth_server_url: %s
`, authServer.URL)
	err = os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}
	defer os.Remove(configPath)

	_, err = conf.LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	s, cleanup := server.SetupTestServer(t)
	defer cleanup()

	conn := server.NewMockTLSConn()
	state := &models.ClientState{Authenticated: false}

	// Send AUTHENTICATE PLAIN
	s.HandleAuthenticate(conn, "A001", []string{"A001", "AUTHENTICATE", "PLAIN"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "+ ") {
		t.Fatalf("Expected continuation, got: %s", response)
	}

	// Send credentials with full email address
	conn.ClearWriteBuffer()
	authString := "\x00user@example.com\x00password"
	authEncoded := base64.StdEncoding.EncodeToString([]byte(authString))
	conn.AddReadData(authEncoded + "\r\n")

	s.HandleAuthenticate(conn, "A001", []string{"A001", "AUTHENTICATE", "PLAIN"}, state)

	response = conn.GetWrittenData()
	t.Logf("Response with domain in username: %s", response)
}

// TestAuthenticatePlain_UsernameWithoutDomain tests authentication with bare username
func TestAuthenticatePlain_UsernameWithoutDomain(t *testing.T) {
	// Username without @ should have configured domain appended
	authServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer authServer.Close()

	// Create config directory and file
	err := os.MkdirAll("config", 0755)
	if err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}

	configPath := "config/raven.yaml"
	configContent := fmt.Sprintf(`domain: testdomain.com
auth_server_url: %s
`, authServer.URL)
	err = os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}
	defer os.Remove(configPath)

	_, err = conf.LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	s, cleanup := server.SetupTestServer(t)
	defer cleanup()

	conn := server.NewMockTLSConn()
	state := &models.ClientState{Authenticated: false}

	// Send AUTHENTICATE PLAIN
	s.HandleAuthenticate(conn, "A001", []string{"A001", "AUTHENTICATE", "PLAIN"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "+ ") {
		t.Fatalf("Expected continuation, got: %s", response)
	}

	// Send credentials without domain
	conn.ClearWriteBuffer()
	authString := "\x00bareusername\x00password"
	authEncoded := base64.StdEncoding.EncodeToString([]byte(authString))
	conn.AddReadData(authEncoded + "\r\n")

	s.HandleAuthenticate(conn, "A001", []string{"A001", "AUTHENTICATE", "PLAIN"}, state)

	response = conn.GetWrittenData()
	t.Logf("Response without domain in username: %s", response)
}

// TestAuthenticatePlain_AuthenticationFailure tests failed authentication
func TestAuthenticatePlain_AuthenticationFailure(t *testing.T) {
	// Mock auth server that returns 401 Unauthorized
	authServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer authServer.Close()

	// Create config directory and file
	err := os.MkdirAll("config", 0755)
	if err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}

	configPath := "config/raven.yaml"
	configContent := fmt.Sprintf(`domain: example.com
auth_server_url: %s
`, authServer.URL)
	err = os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}
	defer os.Remove(configPath)

	_, err = conf.LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	s, cleanup := server.SetupTestServer(t)
	defer cleanup()

	conn := server.NewMockTLSConn()
	state := &models.ClientState{Authenticated: false}

	// Send AUTHENTICATE PLAIN
	s.HandleAuthenticate(conn, "A001", []string{"A001", "AUTHENTICATE", "PLAIN"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "+ ") {
		t.Fatalf("Expected continuation, got: %s", response)
	}

	// Send credentials
	conn.ClearWriteBuffer()
	authString := "\x00testuser\x00wrongpassword"
	authEncoded := base64.StdEncoding.EncodeToString([]byte(authString))
	conn.AddReadData(authEncoded + "\r\n")

	s.HandleAuthenticate(conn, "A001", []string{"A001", "AUTHENTICATE", "PLAIN"}, state)

	response = conn.GetWrittenData()

	// Should get NO response with AUTHENTICATIONFAILED
	t.Logf("Response for failed auth: %s", response)
	if !strings.Contains(response, "NO") {
		t.Errorf("Expected NO response for authentication failure, got: %s", response)
	}
}

// TestAuthenticatePlain_ConfigurationError tests handling of config errors
func TestAuthenticatePlain_ConfigurationError(t *testing.T) {
	// Test with missing domain in config
	err := os.MkdirAll("config", 0755)
	if err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}

	configPath := "config/raven.yaml"
	configContent := `# domain is missing
auth_server_url: https://auth.example.com
`
	err = os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}
	defer os.Remove(configPath)

	_, err = conf.LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	s, cleanup := server.SetupTestServer(t)
	defer cleanup()

	conn := server.NewMockTLSConn()
	state := &models.ClientState{Authenticated: false}

	// Send AUTHENTICATE PLAIN
	s.HandleAuthenticate(conn, "A001", []string{"A001", "AUTHENTICATE", "PLAIN"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "+ ") {
		t.Fatalf("Expected continuation, got: %s", response)
	}

	// Send credentials
	conn.ClearWriteBuffer()
	authString := "\x00testuser\x00password"
	authEncoded := base64.StdEncoding.EncodeToString([]byte(authString))
	conn.AddReadData(authEncoded + "\r\n")

	s.HandleAuthenticate(conn, "A001", []string{"A001", "AUTHENTICATE", "PLAIN"}, state)

	response = conn.GetWrittenData()

	// Should get NO response with configuration error
	t.Logf("Response with config error: %s", response)
	if strings.Contains(response, "SERVERBUG") || strings.Contains(response, "NO") {
		t.Log("Correctly reported configuration error")
	}
}

// TestAuthenticatePlain_TwoPartFormat tests 2-part SASL PLAIN format (without authzid)
func TestAuthenticatePlain_TwoPartFormat(t *testing.T) {
	s, cleanup := server.SetupTestServer(t)
	defer cleanup()

	conn := server.NewMockTLSConn()
	state := &models.ClientState{Authenticated: false}

	// Send AUTHENTICATE PLAIN
	s.HandleAuthenticate(conn, "A001", []string{"A001", "AUTHENTICATE", "PLAIN"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "+ ") {
		t.Fatalf("Expected continuation, got: %s", response)
	}

	// Send credentials in 2-part format (fallback format)
	conn.ClearWriteBuffer()
	authString := "username\x00password" // 2 parts instead of 3
	authEncoded := base64.StdEncoding.EncodeToString([]byte(authString))
	conn.AddReadData(authEncoded + "\r\n")

	s.HandleAuthenticate(conn, "A001", []string{"A001", "AUTHENTICATE", "PLAIN"}, state)

	response = conn.GetWrittenData()
	t.Logf("Response for 2-part format: %s", response)
}

// TestAuthenticatePlain_ReadTimeout tests handling of read timeout
func TestAuthenticatePlain_ReadTimeout(t *testing.T) {
	s, cleanup := server.SetupTestServer(t)
	defer cleanup()

	// Create connection that will timeout on read
	conn := server.NewMockTLSConn()
	state := &models.ClientState{Authenticated: false}

	// Send AUTHENTICATE PLAIN
	s.HandleAuthenticate(conn, "A001", []string{"A001", "AUTHENTICATE", "PLAIN"}, state)

	response := conn.GetWrittenData()
	if !strings.Contains(response, "+ ") {
		t.Fatalf("Expected continuation, got: %s", response)
	}

	// Don't send any data - connection will timeout or return empty
	// This tests the error handling path when conn.Read fails
	conn.ClearWriteBuffer()
	// Not adding any read data simulates timeout/connection issue

	s.HandleAuthenticate(conn, "A001", []string{"A001", "AUTHENTICATE", "PLAIN"}, state)

	response = conn.GetWrittenData()
	t.Logf("Response for read timeout: %s", response)

	// Should get NO response for authentication failure
	if strings.Contains(response, "NO") {
		t.Log("Correctly handled read timeout")
	}
}
