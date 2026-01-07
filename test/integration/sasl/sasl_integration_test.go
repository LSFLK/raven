package sasl_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"raven/internal/sasl"
	"raven/test/helpers"
)

// TestSASLAuthenticationFlow tests the complete SASL authentication flow
func TestSASLAuthenticationFlow(t *testing.T) {
	// Setup mock authentication server
	mockAuth := helpers.SetupMockAuthServer(t)
	defer mockAuth.Close()

	// Create SASL server with test socket
	socketPath := helpers.GetTestSocketPath(t, "sasl-auth-flow")
	server := sasl.NewServer(socketPath, "", mockAuth.URL+"/auth", "example.com")

	// Start SASL server
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Start()
	}()
	defer func() {
		_ = server.Shutdown()
		// Wait for server to stop or timeout
		select {
		case <-serverErr:
		case <-time.After(2 * time.Second):
			t.Log("Server shutdown timed out")
		}
	}()

	// Wait for server to start
	helpers.WaitForUnixSocket(t, socketPath, 3*time.Second)
	t.Logf("SASL server started on socket: %s", socketPath)

	// Test authentication with valid credentials
	client := helpers.ConnectSASL(t, socketPath)
	defer func() { _ = client.Close() }()

	t.Log("Testing valid authentication flow...")

	// Perform version handshake
	client.SendCommand("VERSION\t1\t2")
	response := client.ReadResponse()
	if !strings.HasPrefix(response, "VERSION\t1\t2") {
		t.Errorf("Expected VERSION response, got: %s", response)
	}
	t.Logf("✓ Version handshake successful: %s", response)

	// Send CPID (client process ID)
	client.SendCommand("CPID\t12345")

	// Read mechanism announcements
	responses := client.ReadMultipleResponses()
	t.Logf("Received %d mechanism responses", len(responses))

	var foundPlain, foundLogin bool
	for _, resp := range responses {
		t.Logf("Mechanism response: %s", resp)
		if strings.Contains(resp, "MECH\tPLAIN") {
			foundPlain = true
		}
		if strings.Contains(resp, "MECH\tLOGIN") {
			foundLogin = true
		}
	}

	if !foundPlain {
		t.Error("PLAIN mechanism not announced")
	}
	if !foundLogin {
		t.Error("LOGIN mechanism not announced")
	}
	t.Log("✓ Authentication mechanisms announced successfully")

	// Test PLAIN authentication with valid credentials
	username := "alice"
	password := "validpass123"
	credentials := fmt.Sprintf("\x00%s\x00%s", username, password)
	encoded := base64.StdEncoding.EncodeToString([]byte(credentials))

	authCmd := fmt.Sprintf("AUTH\t1\tPLAIN\tservice=smtp\tresp=%s", encoded)
	client.SendCommand(authCmd)

	authResponse := client.ReadResponse()
	t.Logf("Authentication response: %s", authResponse)

	if !strings.HasPrefix(authResponse, "OK\t1") {
		t.Errorf("Expected OK response, got: %s", authResponse)
	}
	if !strings.Contains(authResponse, fmt.Sprintf("user=%s", username)) {
		t.Errorf("Expected user=%s in response, got: %s", username, authResponse)
	}
	t.Log("✓ PLAIN authentication with valid credentials successful")
}

// TestSASLAuthenticationFailure tests authentication with invalid credentials
func TestSASLAuthenticationFailure(t *testing.T) {
	// Setup mock auth server that rejects credentials
	mockAuth := helpers.SetupMockAuthServerWithResponse(t, http.StatusUnauthorized, `{"error":"Invalid credentials"}`)
	defer mockAuth.Close()

	socketPath := helpers.GetTestSocketPath(t, "sasl-auth-failure")
	server := sasl.NewServer(socketPath, "", mockAuth.URL+"/auth", "example.com")

	// Start server
	go func() { _ = server.Start() }()
	defer func() { _ = server.Shutdown() }()

	helpers.WaitForUnixSocket(t, socketPath, 3*time.Second)
	t.Logf("SASL server started for failure test on socket: %s", socketPath)

	client := helpers.ConnectSASL(t, socketPath)
	defer func() { _ = client.Close() }()

	t.Log("Testing authentication failure flow...")

	// Version handshake
	client.SendCommand("VERSION\t1\t2")
	client.ReadResponse() // Ignore response

	// CPID
	client.SendCommand("CPID\t12345")
	client.ReadMultipleResponses() // Read all mechanism announcements

	// Test authentication with invalid credentials
	username := "alice"
	password := "wrongpassword"
	credentials := fmt.Sprintf("\x00%s\x00%s", username, password)
	encoded := base64.StdEncoding.EncodeToString([]byte(credentials))

	authCmd := fmt.Sprintf("AUTH\t2\tPLAIN\tservice=smtp\tresp=%s", encoded)
	client.SendCommand(authCmd)

	failResponse := client.ReadResponse()
	t.Logf("Authentication failure response: %s", failResponse)

	if !strings.HasPrefix(failResponse, "FAIL\t2") {
		t.Errorf("Expected FAIL response, got: %s", failResponse)
	}
	if !strings.Contains(failResponse, "reason=Invalid credentials") {
		t.Errorf("Expected 'Invalid credentials' reason, got: %s", failResponse)
	}
	t.Log("✓ Authentication failure handled correctly")
}

// TestSASLPlainWithoutInitialResponse tests PLAIN authentication without initial response
func TestSASLPlainWithoutInitialResponse(t *testing.T) {
	mockAuth := helpers.SetupMockAuthServer(t)
	defer mockAuth.Close()

	socketPath := helpers.GetTestSocketPath(t, "sasl-plain-continuation")
	server := sasl.NewServer(socketPath, "", mockAuth.URL+"/auth", "example.com")

	go func() { _ = server.Start() }()
	defer func() { _ = server.Shutdown() }()

	helpers.WaitForUnixSocket(t, socketPath, 3*time.Second)

	client := helpers.ConnectSASL(t, socketPath)
	defer func() { _ = client.Close() }()

	t.Log("Testing PLAIN authentication without initial response...")

	// Handshake
	client.SendCommand("VERSION\t1\t2")
	client.ReadResponse()
	client.SendCommand("CPID\t12345")
	client.ReadMultipleResponses()

	// Start AUTH without response
	client.SendCommand("AUTH\t3\tPLAIN\tservice=smtp")

	contResponse := client.ReadResponse()
	t.Logf("Continuation response: %s", contResponse)

	if !strings.HasPrefix(contResponse, "CONT\t3") {
		t.Errorf("Expected CONT response, got: %s", contResponse)
	}
	t.Log("✓ PLAIN authentication continuation handled correctly")
}

// TestSASLInvalidMechanism tests authentication with unsupported mechanism
func TestSASLInvalidMechanism(t *testing.T) {
	mockAuth := helpers.SetupMockAuthServer(t)
	defer mockAuth.Close()

	socketPath := helpers.GetTestSocketPath(t, "sasl-invalid-mech")
	server := sasl.NewServer(socketPath, "", mockAuth.URL+"/auth", "example.com")

	go func() { _ = server.Start() }()
	defer func() { _ = server.Shutdown() }()

	helpers.WaitForUnixSocket(t, socketPath, 3*time.Second)

	client := helpers.ConnectSASL(t, socketPath)
	defer func() { _ = client.Close() }()

	t.Log("Testing authentication with unsupported mechanism...")

	// Handshake
	client.SendCommand("VERSION\t1\t2")
	client.ReadResponse()
	client.SendCommand("CPID\t12345")
	client.ReadMultipleResponses()

	// Try unsupported mechanism
	client.SendCommand("AUTH\t4\tCRAM-MD5\tservice=smtp")

	failResponse := client.ReadResponse()
	t.Logf("Unsupported mechanism response: %s", failResponse)

	if !strings.HasPrefix(failResponse, "FAIL\t4") {
		t.Errorf("Expected FAIL response, got: %s", failResponse)
	}
	if !strings.Contains(failResponse, "reason=Unsupported mechanism") {
		t.Errorf("Expected 'Unsupported mechanism' reason, got: %s", failResponse)
	}
	t.Log("✓ Unsupported mechanism handled correctly")
}

// TestSASLMalformedCredentials tests authentication with malformed credentials
func TestSASLMalformedCredentials(t *testing.T) {
	mockAuth := helpers.SetupMockAuthServer(t)
	defer mockAuth.Close()

	socketPath := helpers.GetTestSocketPath(t, "sasl-malformed")
	server := sasl.NewServer(socketPath, "", mockAuth.URL+"/auth", "example.com")

	go func() { _ = server.Start() }()
	defer func() { _ = server.Shutdown() }()

	helpers.WaitForUnixSocket(t, socketPath, 3*time.Second)

	client := helpers.ConnectSASL(t, socketPath)
	defer func() { _ = client.Close() }()

	t.Log("Testing authentication with malformed credentials...")

	// Handshake
	client.SendCommand("VERSION\t1\t2")
	client.ReadResponse()
	client.SendCommand("CPID\t12345")
	client.ReadMultipleResponses()

	testCases := []struct {
		name        string
		credentials string
		expectError string
	}{
		{
			name:        "Invalid base64",
			credentials: "invalid_base64_data!!!",
			expectError: "Invalid encoding",
		},
		{
			name:        "Missing password",
			credentials: base64.StdEncoding.EncodeToString([]byte("username")),
			expectError: "Invalid credentials format",
		},
		{
			name:        "Empty response",
			credentials: "",
			expectError: "Invalid credentials format",
		},
	}

	for i, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			authCmd := fmt.Sprintf("AUTH\t%d\tPLAIN\tservice=smtp\tresp=%s", 10+i, tc.credentials)
			client.SendCommand(authCmd)

			failResponse := client.ReadResponse()
			t.Logf("Malformed credentials response for %s: %s", tc.name, failResponse)

			if !strings.HasPrefix(failResponse, fmt.Sprintf("FAIL\t%d", 10+i)) {
				t.Errorf("Expected FAIL response for %s, got: %s", tc.name, failResponse)
			}
			if !strings.Contains(failResponse, fmt.Sprintf("reason=%s", tc.expectError)) {
				t.Errorf("Expected '%s' reason for %s, got: %s", tc.expectError, tc.name, failResponse)
			}
		})
	}
	t.Log("✓ Malformed credentials handled correctly")
}

// TestSASLConcurrentConnections tests multiple concurrent SASL connections
func TestSASLConcurrentConnections(t *testing.T) {
	mockAuth := helpers.SetupMockAuthServer(t)
	defer mockAuth.Close()

	socketPath := helpers.GetTestSocketPath(t, "sasl-concurrent")
	server := sasl.NewServer(socketPath, "", mockAuth.URL+"/auth", "example.com")

	go func() { _ = server.Start() }()
	defer func() { _ = server.Shutdown() }()

	helpers.WaitForUnixSocket(t, socketPath, 3*time.Second)

	t.Log("Testing concurrent SASL connections...")

	const numConnections = 5
	var wg sync.WaitGroup
	errors := make(chan error, numConnections)

	for i := 0; i < numConnections; i++ {
		wg.Add(1)
		go func(connID int) {
			defer wg.Done()

			client := helpers.ConnectSASL(t, socketPath)
			defer func() { _ = client.Close() }()

			// Handshake
			client.SendCommand("VERSION\t1\t2")
			if resp := client.ReadResponse(); !strings.HasPrefix(resp, "VERSION") {
				errors <- fmt.Errorf("conn %d: invalid version response: %s", connID, resp)
				return
			}

			client.SendCommand("CPID\t12345")
			client.ReadMultipleResponses() // Read mechanisms

			// Authenticate
			username := fmt.Sprintf("user%d", connID)
			password := "validpass123"
			credentials := fmt.Sprintf("\x00%s\x00%s", username, password)
			encoded := base64.StdEncoding.EncodeToString([]byte(credentials))

			authCmd := fmt.Sprintf("AUTH\t%d\tPLAIN\tservice=smtp\tresp=%s", connID, encoded)
			client.SendCommand(authCmd)

			authResponse := client.ReadResponse()
			if !strings.HasPrefix(authResponse, fmt.Sprintf("OK\t%d", connID)) {
				errors <- fmt.Errorf("conn %d: authentication failed: %s", connID, authResponse)
				return
			}

			t.Logf("✓ Connection %d authenticated successfully", connID)
		}(i)
	}

	wg.Wait()
	close(errors)

	var errorCount int
	for err := range errors {
		t.Error(err)
		errorCount++
	}

	if errorCount == 0 {
		t.Logf("✓ All %d concurrent connections handled successfully", numConnections)
	} else {
		t.Errorf("Failed: %d out of %d connections had errors", errorCount, numConnections)
	}
}

// TestSASLServerShutdownGraceful tests graceful server shutdown
func TestSASLServerShutdownGraceful(t *testing.T) {
	mockAuth := helpers.SetupMockAuthServer(t)
	defer mockAuth.Close()

	socketPath := helpers.GetTestSocketPath(t, "sasl-shutdown")
	server := sasl.NewServer(socketPath, "", mockAuth.URL+"/auth", "example.com")

	t.Log("Testing graceful SASL server shutdown...")

	// Start server
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Start()
	}()

	helpers.WaitForUnixSocket(t, socketPath, 3*time.Second)

	// Verify socket exists
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		t.Fatalf("Socket file was not created at %s", socketPath)
	}
	t.Logf("✓ Socket file created: %s", socketPath)

	// Connect a client before shutdown
	client := helpers.ConnectSASL(t, socketPath)
	client.SendCommand("VERSION\t1\t2")
	client.ReadResponse()
	t.Log("✓ Client connected before shutdown")

	// Properly close the client connection before shutdown to avoid server timeout
	_ = client.Close()
	t.Log("✓ Client connection closed")

	// Initiate graceful shutdown
	shutdownErr := server.Shutdown()
	if shutdownErr != nil {
		t.Errorf("Shutdown returned error: %v", shutdownErr)
	}
	t.Log("✓ Shutdown initiated")

	// Verify server stops
	select {
	case err := <-serverErr:
		if err != nil {
			t.Errorf("Server returned error: %v", err)
		}
		t.Log("✓ Server stopped gracefully")
	case <-time.After(3 * time.Second): // Reduced timeout since we closed client properly
		t.Error("Server did not stop within timeout")
	}

	// Verify socket cleanup
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Error("Socket file was not removed after shutdown")
	} else {
		t.Log("✓ Socket file cleaned up")
	}

	// Test idempotent shutdown
	err2 := server.Shutdown()
	if err2 != nil {
		t.Errorf("Second shutdown returned error: %v", err2)
	}
	t.Log("✓ Idempotent shutdown works")
}

// TestSASLAuthenticationServerTimeout tests authentication server timeout handling
func TestSASLAuthenticationServerTimeout(t *testing.T) {
	// Create mock server that times out
	mockAuth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate timeout by sleeping longer than client timeout (10s)
		// Using 12 seconds to ensure timeout while keeping test duration reasonable
		time.Sleep(12 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer mockAuth.Close()

	socketPath := helpers.GetTestSocketPath(t, "sasl-timeout")
	server := sasl.NewServer(socketPath, "", mockAuth.URL+"/auth", "example.com")

	go func() { _ = server.Start() }()
	defer func() { _ = server.Shutdown() }()

	helpers.WaitForUnixSocket(t, socketPath, 3*time.Second)

	client := helpers.ConnectSASL(t, socketPath)
	defer func() { _ = client.Close() }()

	t.Log("Testing authentication server timeout handling...")

	// Handshake
	client.SendCommand("VERSION\t1\t2")
	client.ReadResponse()
	client.SendCommand("CPID\t12345")
	client.ReadMultipleResponses()

	// Authenticate with timeout expected
	username := "alice"
	password := "validpass123"
	credentials := fmt.Sprintf("\x00%s\x00%s", username, password)
	encoded := base64.StdEncoding.EncodeToString([]byte(credentials))

	authCmd := fmt.Sprintf("AUTH\t1\tPLAIN\tservice=smtp\tresp=%s", encoded)
	client.SendCommand(authCmd)

	// Should get FAIL due to timeout
	failResponse := client.ReadResponse()
	t.Logf("Timeout response: %s", failResponse)

	if !strings.HasPrefix(failResponse, "FAIL\t1") {
		t.Errorf("Expected FAIL response due to timeout, got: %s", failResponse)
	}
	t.Log("✓ Authentication server timeout handled correctly")
}

// TestSASLDomainHandling tests domain handling in authentication
func TestSASLDomainHandling(t *testing.T) {
	// Mock auth server that logs the request to verify domain handling
	var lastRequest *http.Request
	var lastBody []byte

	mockAuth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastRequest = r
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		lastBody = body
		w.WriteHeader(http.StatusOK)
	}))
	defer mockAuth.Close()

	socketPath := helpers.GetTestSocketPath(t, "sasl-domain")
	server := sasl.NewServer(socketPath, "", mockAuth.URL+"/auth", "example.com")

	go func() { _ = server.Start() }()
	defer func() { _ = server.Shutdown() }()

	helpers.WaitForUnixSocket(t, socketPath, 3*time.Second)

	client := helpers.ConnectSASL(t, socketPath)
	defer func() { _ = client.Close() }()

	t.Log("Testing domain handling in authentication...")

	// Handshake
	client.SendCommand("VERSION\t1\t2")
	client.ReadResponse()
	client.SendCommand("CPID\t12345")
	client.ReadMultipleResponses()

	testCases := []struct {
		name          string
		username      string
		expectedEmail string
	}{
		{
			name:          "Username without domain",
			username:      "alice",
			expectedEmail: "alice@example.com",
		},
		{
			name:          "Username with domain",
			username:      "bob@custom.com",
			expectedEmail: "bob@custom.com",
		},
	}

	for i, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			password := "validpass123"
			credentials := fmt.Sprintf("\x00%s\x00%s", tc.username, password)
			encoded := base64.StdEncoding.EncodeToString([]byte(credentials))

			authCmd := fmt.Sprintf("AUTH\t%d\tPLAIN\tservice=smtp\tresp=%s", 20+i, encoded)
			client.SendCommand(authCmd)

			client.ReadResponse() // Read response

			// Verify the request sent to auth server
			if lastRequest == nil {
				t.Errorf("No request received by auth server")
				return
			}

			var authRequest map[string]interface{}
			if err := json.Unmarshal(lastBody, &authRequest); err != nil {
				t.Errorf("Failed to parse auth request: %v", err)
				return
			}

			if email, ok := authRequest["email"].(string); ok {
				if email != tc.expectedEmail {
					t.Errorf("Expected email %s, got %s", tc.expectedEmail, email)
				} else {
					t.Logf("✓ Correct email sent to auth server: %s", email)
				}
			} else {
				t.Errorf("Email field not found in auth request")
			}
		})
	}
}

// TestSASLLoginMechanism tests LOGIN mechanism (simplified implementation)
func TestSASLLoginMechanism(t *testing.T) {
	mockAuth := helpers.SetupMockAuthServer(t)
	defer mockAuth.Close()

	socketPath := helpers.GetTestSocketPath(t, "sasl-login")
	server := sasl.NewServer(socketPath, "", mockAuth.URL+"/auth", "example.com")

	go func() { _ = server.Start() }()
	defer func() { _ = server.Shutdown() }()

	helpers.WaitForUnixSocket(t, socketPath, 3*time.Second)

	client := helpers.ConnectSASL(t, socketPath)
	defer func() { _ = client.Close() }()

	t.Log("Testing LOGIN mechanism...")

	// Handshake
	client.SendCommand("VERSION\t1\t2")
	client.ReadResponse()
	client.SendCommand("CPID\t12345")
	client.ReadMultipleResponses()

	// Try LOGIN mechanism without response (should get continuation for username)
	client.SendCommand("AUTH\t1\tLOGIN\tservice=smtp")

	contResponse := client.ReadResponse()
	t.Logf("LOGIN mechanism continuation response: %s", contResponse)

	// Should get CONT asking for username
	if !strings.HasPrefix(contResponse, "CONT\t1") {
		t.Errorf("Expected CONT response for LOGIN, got: %s", contResponse)
	}
	if !strings.Contains(contResponse, "Username:") {
		t.Errorf("Expected 'Username:' prompt, got: %s", contResponse)
	}
	t.Log("✓ LOGIN mechanism requests username as expected")

	// Try LOGIN mechanism with response (should fail as not fully implemented)
	client.SendCommand("AUTH\t2\tLOGIN\tservice=smtp\tresp=dGVzdA==") // base64 "test"

	failResponse := client.ReadResponse()
	t.Logf("LOGIN mechanism with response: %s", failResponse)

	// Should get FAIL for incomplete implementation
	if !strings.HasPrefix(failResponse, "FAIL\t2") {
		t.Errorf("Expected FAIL response for LOGIN with response, got: %s", failResponse)
	}
	if !strings.Contains(failResponse, "LOGIN not fully implemented") {
		t.Errorf("Expected 'LOGIN not fully implemented' message, got: %s", failResponse)
	}
	t.Log("✓ LOGIN mechanism correctly indicates incomplete implementation")
}
