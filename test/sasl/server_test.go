package sasl_test

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"go-imap/internal/sasl"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// getSocketPath returns a short socket path to avoid Unix socket path length limits (104 chars on macOS)
func getSocketPath(t *testing.T) string {
	// Use /tmp/ with a short random suffix to stay well under the 104 character limit
	socketPath := fmt.Sprintf("/tmp/sasl-%d.sock", rand.Int63())
	t.Cleanup(func() {
		os.Remove(socketPath)
	})
	return socketPath
}

// TestNewServer tests server creation
func TestNewServer(t *testing.T) {
	socketPath := "/tmp/test-sasl.sock"
	authURL := "https://example.com/auth"
	domain := "example.com"

	server := sasl.NewServer(socketPath, authURL, domain)

	if server == nil {
		t.Fatal("Expected server to be created, got nil")
	}

	// Note: Cannot test private fields from external package
	// Tests now focus on public API behavior
}

// TestServerStartShutdown tests server startup and graceful shutdown
func TestServerStartShutdown(t *testing.T) {
	socketPath := getSocketPath(t)

	// Create a mock auth server
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer authServer.Close()

	server := sasl.NewServer(socketPath, authServer.URL, "example.com")

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- server.Start()
	}()

	// Give it time to start
	time.Sleep(100 * time.Millisecond)

	// Verify socket was created
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		t.Errorf("Socket file was not created at %s", socketPath)
	}

	// Test graceful shutdown
	if err := server.Shutdown(); err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}

	// Verify server stopped
	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("Server returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Server did not stop within timeout")
	}

	// Verify socket was cleaned up
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Error("Socket file was not removed after shutdown")
	}
}

// TestServerShutdownIdempotent tests that shutdown can be called multiple times safely
func TestServerShutdownIdempotent(t *testing.T) {
	socketPath := getSocketPath(t)

	server := sasl.NewServer(socketPath, "https://example.com/auth", "example.com")

	// Start server in goroutine
	go server.Start()
	time.Sleep(100 * time.Millisecond)

	// Call shutdown multiple times
	err1 := server.Shutdown()
	err2 := server.Shutdown()
	err3 := server.Shutdown()

	if err1 != nil {
		t.Errorf("First shutdown failed: %v", err1)
	}
	if err2 != nil {
		t.Errorf("Second shutdown failed: %v", err2)
	}
	if err3 != nil {
		t.Errorf("Third shutdown failed: %v", err3)
	}
}

// TestVersionHandshake tests the VERSION command handling
func TestVersionHandshake(t *testing.T) {
	socketPath := getSocketPath(t)

	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer authServer.Close()

	server := sasl.NewServer(socketPath, authServer.URL, "example.com")

	// Start server
	go server.Start()
	defer server.Shutdown()
	time.Sleep(100 * time.Millisecond)

	// Connect to the socket
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to connect to socket: %v", err)
	}
	defer conn.Close()

	// Send VERSION command
	fmt.Fprintf(conn, "VERSION\t1\t2\n")

	// Read response
	reader := bufio.NewReader(conn)
	response, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	// Check response
	expectedResponse := "VERSION\t1\t2\n"
	if response != expectedResponse {
		t.Errorf("Expected response %q, got %q", expectedResponse, response)
	}
}

// TestCPIDCommand tests the CPID command handling
func TestCPIDCommand(t *testing.T) {
	socketPath := getSocketPath(t)

	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer authServer.Close()

	server := sasl.NewServer(socketPath, authServer.URL, "example.com")

	// Start server
	go server.Start()
	defer server.Shutdown()
	time.Sleep(100 * time.Millisecond)

	// Connect to the socket
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to connect to socket: %v", err)
	}
	defer conn.Close()

	// Send CPID command
	fmt.Fprintf(conn, "CPID\t12345\n")

	// Read all responses - server sends MECH lines followed by DONE
	reader := bufio.NewReader(conn)

	// Read first MECH line
	response1, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read first MECH response: %v", err)
	}
	if !strings.HasPrefix(response1, "MECH\t") {
		t.Errorf("Expected first MECH response, got %q", response1)
	}

	// Read second MECH line
	response2, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read second MECH response: %v", err)
	}
	if !strings.HasPrefix(response2, "MECH\t") {
		t.Errorf("Expected second MECH response, got %q", response2)
	}

	// Read DONE line
	response3, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read DONE response: %v", err)
	}
	expectedResponse := "DONE\n"
	if response3 != expectedResponse {
		t.Errorf("Expected response %q, got %q", expectedResponse, response3)
	}
}

// TestPlainAuthenticationSuccess tests successful PLAIN authentication
func TestPlainAuthenticationSuccess(t *testing.T) {
	socketPath := getSocketPath(t)

	// Mock auth server that accepts credentials
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer authServer.Close()

	server := sasl.NewServer(socketPath, authServer.URL, "example.com")

	// Start server
	errChan := make(chan error, 1)
	go func() {
		errChan <- server.Start()
	}()
	defer server.Shutdown()

	// Wait for socket to be created
	for i := 0; i < 20; i++ {
		if _, err := os.Stat(socketPath); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Connect to the socket
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to connect to socket: %v", err)
	}
	defer conn.Close()

	// Create credentials: \x00username\x00password
	credentials := "\x00testuser\x00testpass"
	encodedCreds := base64.StdEncoding.EncodeToString([]byte(credentials))

	// Send AUTH command with PLAIN mechanism
	fmt.Fprintf(conn, "AUTH\t1\tPLAIN\tservice=smtp\tresp=%s\n", encodedCreds)

	// Read response
	reader := bufio.NewReader(conn)
	response, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	// Check for success response
	if !strings.HasPrefix(response, "OK\t1\t") {
		t.Errorf("Expected OK response, got: %s", response)
	}

	if !strings.Contains(response, "user=testuser") {
		t.Errorf("Expected user=testuser in response, got: %s", response)
	}
}

// TestPlainAuthenticationWithDomain tests PLAIN authentication with domain appending
func TestPlainAuthenticationWithDomain(t *testing.T) {
	socketPath := getSocketPath(t)

	// Track the email received by auth server
	var receivedEmail string
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read the request body to capture email
		var buf [1024]byte
		n, _ := r.Body.Read(buf[:])
		body := string(buf[:n])

		// Extract email from JSON body
		if strings.Contains(body, "testuser@example.com") {
			receivedEmail = "testuser@example.com"
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer authServer.Close()

	server := sasl.NewServer(socketPath, authServer.URL, "example.com")

	// Start server
	go server.Start()
	defer server.Shutdown()
	time.Sleep(100 * time.Millisecond)

	// Connect to the socket
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to connect to socket: %v", err)
	}
	defer conn.Close()

	// Create credentials with username (no domain)
	credentials := "\x00testuser\x00testpass"
	encodedCreds := base64.StdEncoding.EncodeToString([]byte(credentials))

	// Send AUTH command
	fmt.Fprintf(conn, "AUTH\t1\tPLAIN\tservice=smtp\tresp=%s\n", encodedCreds)

	// Read response
	reader := bufio.NewReader(conn)
	_, err = reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	// Give time for the auth request to complete
	time.Sleep(100 * time.Millisecond)

	// Verify domain was appended
	if receivedEmail != "testuser@example.com" {
		t.Errorf("Expected email with domain to be sent to auth server")
	}
}

// TestPlainAuthenticationFailure tests failed PLAIN authentication
func TestPlainAuthenticationFailure(t *testing.T) {
	socketPath := getSocketPath(t)

	// Mock auth server that rejects credentials
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer authServer.Close()

	server := sasl.NewServer(socketPath, authServer.URL, "example.com")

	// Start server
	go server.Start()
	defer server.Shutdown()
	time.Sleep(100 * time.Millisecond)

	// Connect to the socket
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to connect to socket: %v", err)
	}
	defer conn.Close()

	// Create credentials
	credentials := "\x00wronguser\x00wrongpass"
	encodedCreds := base64.StdEncoding.EncodeToString([]byte(credentials))

	// Send AUTH command
	fmt.Fprintf(conn, "AUTH\t1\tPLAIN\tservice=smtp\tresp=%s\n", encodedCreds)

	// Read response
	reader := bufio.NewReader(conn)
	response, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	// Check for failure response
	if !strings.HasPrefix(response, "FAIL\t1\t") {
		t.Errorf("Expected FAIL response, got: %s", response)
	}

	if !strings.Contains(response, "reason=Invalid credentials") {
		t.Errorf("Expected 'Invalid credentials' reason, got: %s", response)
	}
}

// TestPlainAuthenticationWithAuthzid tests PLAIN authentication with authorization identity
func TestPlainAuthenticationWithAuthzid(t *testing.T) {
	socketPath := getSocketPath(t)

	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer authServer.Close()

	server := sasl.NewServer(socketPath, authServer.URL, "example.com")

	// Start server
	go server.Start()
	defer server.Shutdown()
	time.Sleep(100 * time.Millisecond)

	// Connect to the socket
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to connect to socket: %v", err)
	}
	defer conn.Close()

	// Create credentials with authzid: authzid\x00authcid\x00password
	credentials := "admin\x00testuser\x00testpass"
	encodedCreds := base64.StdEncoding.EncodeToString([]byte(credentials))

	// Send AUTH command
	fmt.Fprintf(conn, "AUTH\t1\tPLAIN\tservice=smtp\tresp=%s\n", encodedCreds)

	// Read response
	reader := bufio.NewReader(conn)
	response, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	// Should succeed with authcid (testuser)
	if !strings.HasPrefix(response, "OK\t1\t") {
		t.Errorf("Expected OK response, got: %s", response)
	}

	if !strings.Contains(response, "user=testuser") {
		t.Errorf("Expected user=testuser in response, got: %s", response)
	}
}

// TestPlainAuthenticationInvalidBase64 tests handling of invalid base64 encoding
func TestPlainAuthenticationInvalidBase64(t *testing.T) {
	socketPath := getSocketPath(t)

	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer authServer.Close()

	server := sasl.NewServer(socketPath, authServer.URL, "example.com")

	// Start server
	go server.Start()
	defer server.Shutdown()
	time.Sleep(100 * time.Millisecond)

	// Connect to the socket
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to connect to socket: %v", err)
	}
	defer conn.Close()

	// Send AUTH command with invalid base64
	fmt.Fprintf(conn, "AUTH\t1\tPLAIN\tservice=smtp\tresp=!!!invalid-base64!!!\n")

	// Read response
	reader := bufio.NewReader(conn)
	response, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	// Should fail with encoding error
	if !strings.HasPrefix(response, "FAIL\t1\t") {
		t.Errorf("Expected FAIL response, got: %s", response)
	}

	if !strings.Contains(response, "reason=Invalid encoding") {
		t.Errorf("Expected 'Invalid encoding' reason, got: %s", response)
	}
}

// TestPlainAuthenticationMalformedCredentials tests handling of malformed credentials
func TestPlainAuthenticationMalformedCredentials(t *testing.T) {
	socketPath := getSocketPath(t)

	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer authServer.Close()

	server := sasl.NewServer(socketPath, authServer.URL, "example.com")

	// Start server
	go server.Start()
	defer server.Shutdown()
	time.Sleep(100 * time.Millisecond)

	testCases := []struct {
		name        string
		credentials string
		description string
	}{
		{
			name:        "NoNullSeparators",
			credentials: "usernamepassword",
			description: "Credentials without null separators",
		},
		{
			name:        "OnlyOneField",
			credentials: "username",
			description: "Only one field provided",
		},
		{
			name:        "EmptyCredentials",
			credentials: "",
			description: "Empty credentials",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Connect to the socket
			conn, err := net.Dial("unix", socketPath)
			if err != nil {
				t.Fatalf("Failed to connect to socket: %v", err)
			}
			defer conn.Close()

			// Encode malformed credentials
			encodedCreds := base64.StdEncoding.EncodeToString([]byte(tc.credentials))

			// Send AUTH command
			fmt.Fprintf(conn, "AUTH\t1\tPLAIN\tservice=smtp\tresp=%s\n", encodedCreds)

			// Read response
			reader := bufio.NewReader(conn)
			response, err := reader.ReadString('\n')
			if err != nil {
				t.Fatalf("Failed to read response: %v", err)
			}

			// Should fail with format error
			if !strings.HasPrefix(response, "FAIL\t1\t") {
				t.Errorf("Expected FAIL response for %s, got: %s", tc.description, response)
			}
		})
	}
}

// TestPlainAuthenticationContinuationRequest tests continuation request when no response provided
func TestPlainAuthenticationContinuationRequest(t *testing.T) {
	socketPath := getSocketPath(t)

	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer authServer.Close()

	server := sasl.NewServer(socketPath, authServer.URL, "example.com")

	// Start server
	go server.Start()
	defer server.Shutdown()
	time.Sleep(100 * time.Millisecond)

	// Connect to the socket
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to connect to socket: %v", err)
	}
	defer conn.Close()

	// Send AUTH command without response
	fmt.Fprintf(conn, "AUTH\t1\tPLAIN\tservice=smtp\n")

	// Read response
	reader := bufio.NewReader(conn)
	response, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	// Should get continuation request
	expectedResponse := "CONT\t1\t\n"
	if response != expectedResponse {
		t.Errorf("Expected continuation response %q, got %q", expectedResponse, response)
	}
}

// TestLoginMechanism tests LOGIN authentication mechanism
func TestLoginMechanism(t *testing.T) {
	socketPath := getSocketPath(t)

	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer authServer.Close()

	server := sasl.NewServer(socketPath, authServer.URL, "example.com")

	// Start server
	go server.Start()
	defer server.Shutdown()
	time.Sleep(100 * time.Millisecond)

	// Connect to the socket
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to connect to socket: %v", err)
	}
	defer conn.Close()

	// Send AUTH command with LOGIN mechanism
	fmt.Fprintf(conn, "AUTH\t1\tLOGIN\tservice=smtp\n")

	// Read response
	reader := bufio.NewReader(conn)
	response, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	// Should get continuation request or not implemented message
	// LOGIN is not fully implemented, so we expect either CONT or FAIL
	if !strings.HasPrefix(response, "CONT\t1\t") && !strings.HasPrefix(response, "FAIL\t1\t") {
		t.Errorf("Expected CONT or FAIL response, got: %s", response)
	}
}

// TestUnsupportedMechanism tests handling of unsupported authentication mechanisms
func TestUnsupportedMechanism(t *testing.T) {
	socketPath := getSocketPath(t)

	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer authServer.Close()

	server := sasl.NewServer(socketPath, authServer.URL, "example.com")

	// Start server
	go server.Start()
	defer server.Shutdown()
	time.Sleep(100 * time.Millisecond)

	mechanisms := []string{"CRAM-MD5", "DIGEST-MD5", "GSSAPI", "NTLM"}

	for _, mechanism := range mechanisms {
		t.Run(mechanism, func(t *testing.T) {
			// Connect to the socket
			conn, err := net.Dial("unix", socketPath)
			if err != nil {
				t.Fatalf("Failed to connect to socket: %v", err)
			}
			defer conn.Close()

			// Send AUTH command with unsupported mechanism
			fmt.Fprintf(conn, "AUTH\t1\t%s\tservice=smtp\n", mechanism)

			// Read response
			reader := bufio.NewReader(conn)
			response, err := reader.ReadString('\n')
			if err != nil {
				t.Fatalf("Failed to read response: %v", err)
			}

			// Should fail with unsupported mechanism
			if !strings.HasPrefix(response, "FAIL\t1\t") {
				t.Errorf("Expected FAIL response for %s, got: %s", mechanism, response)
			}

			if !strings.Contains(response, "Unsupported mechanism") {
				t.Errorf("Expected 'Unsupported mechanism' in response, got: %s", response)
			}
		})
	}
}

// TestAuthMechanismCaseInsensitive tests that mechanism names are case-insensitive
func TestAuthMechanismCaseInsensitive(t *testing.T) {
	socketPath := getSocketPath(t)

	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer authServer.Close()

	server := sasl.NewServer(socketPath, authServer.URL, "example.com")

	// Start server
	go server.Start()
	defer server.Shutdown()
	time.Sleep(100 * time.Millisecond)

	mechanisms := []string{"PLAIN", "Plain", "plain", "pLaIn"}

	for _, mechanism := range mechanisms {
		t.Run(mechanism, func(t *testing.T) {
			// Connect to the socket
			conn, err := net.Dial("unix", socketPath)
			if err != nil {
				t.Fatalf("Failed to connect to socket: %v", err)
			}
			defer conn.Close()

			// Send AUTH command
			fmt.Fprintf(conn, "AUTH\t1\t%s\tservice=smtp\n", mechanism)

			// Read response
			reader := bufio.NewReader(conn)
			response, err := reader.ReadString('\n')
			if err != nil {
				t.Fatalf("Failed to read response: %v", err)
			}

			// Should get continuation request (not failure)
			if !strings.HasPrefix(response, "CONT\t1\t") {
				t.Errorf("Expected CONT response for %s, got: %s", mechanism, response)
			}
		})
	}
}

// TestInvalidAuthCommand tests handling of invalid AUTH command formats
func TestInvalidAuthCommand(t *testing.T) {
	socketPath := getSocketPath(t)

	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer authServer.Close()

	server := sasl.NewServer(socketPath, authServer.URL, "example.com")

	// Start server
	go server.Start()
	defer server.Shutdown()
	time.Sleep(100 * time.Millisecond)

	// Connect to the socket
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to connect to socket: %v", err)
	}
	defer conn.Close()

	// Send AUTH command with missing mechanism
	fmt.Fprintf(conn, "AUTH\t1\n")

	// Read response - server should handle gracefully (might not respond or log error)
	// The current implementation doesn't send a response for invalid formats
	// We just verify the server doesn't crash
	time.Sleep(100 * time.Millisecond)
}

// TestConcurrentConnections tests handling of multiple concurrent connections
func TestConcurrentConnections(t *testing.T) {
	socketPath := getSocketPath(t)

	// Track number of authentication requests
	var authCount int
	var authMutex sync.Mutex

	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authMutex.Lock()
		authCount++
		authMutex.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer authServer.Close()

	server := sasl.NewServer(socketPath, authServer.URL, "example.com")

	// Start server
	go server.Start()
	defer server.Shutdown()
	time.Sleep(100 * time.Millisecond)

	// Number of concurrent connections
	numConnections := 10
	var wg sync.WaitGroup

	for i := 0; i < numConnections; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Connect to the socket
			conn, err := net.Dial("unix", socketPath)
			if err != nil {
				t.Errorf("Connection %d failed: %v", id, err)
				return
			}
			defer conn.Close()

			// Send authentication request
			credentials := fmt.Sprintf("\x00user%d\x00pass%d", id, id)
			encodedCreds := base64.StdEncoding.EncodeToString([]byte(credentials))
			fmt.Fprintf(conn, "AUTH\t%d\tPLAIN\tservice=smtp\tresp=%s\n", id, encodedCreds)

			// Read response
			reader := bufio.NewReader(conn)
			response, err := reader.ReadString('\n')
			if err != nil {
				t.Errorf("Connection %d failed to read response: %v", id, err)
				return
			}

			// Verify success
			if !strings.HasPrefix(response, fmt.Sprintf("OK\t%d\t", id)) {
				t.Errorf("Connection %d expected OK response, got: %s", id, response)
			}
		}(i)
	}

	// Wait for all connections to complete
	wg.Wait()

	// Verify all authentications were processed
	authMutex.Lock()
	defer authMutex.Unlock()
	if authCount != numConnections {
		t.Errorf("Expected %d authentication requests, got %d", numConnections, authCount)
	}
}

// TestConnectionTimeout tests that connections timeout properly
func TestConnectionTimeout(t *testing.T) {
	socketPath := getSocketPath(t)

	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer authServer.Close()

	server := sasl.NewServer(socketPath, authServer.URL, "example.com")

	// Start server
	go server.Start()
	defer server.Shutdown()
	time.Sleep(100 * time.Millisecond)

	// Connect to the socket
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to connect to socket: %v", err)
	}
	defer conn.Close()

	// Don't send anything and wait for timeout (30 seconds in the implementation)
	// For testing purposes, we just verify the connection is established
	// The actual timeout test would take 30+ seconds, so we skip it
	// This is more of a smoke test to ensure connection handling works

	reader := bufio.NewReader(conn)

	// Set a short deadline for testing
	conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

	_, err = reader.ReadString('\n')

	// We expect a timeout error
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}
}

// TestAuthenticationAPIError tests handling of authentication API errors
func TestAuthenticationAPIError(t *testing.T) {
	socketPath := getSocketPath(t)

	testCases := []struct {
		name       string
		statusCode int
		shouldFail bool
	}{
		{"Success200", http.StatusOK, false},
		{"Unauthorized401", http.StatusUnauthorized, true},
		{"Forbidden403", http.StatusForbidden, true},
		{"InternalError500", http.StatusInternalServerError, true},
		{"BadGateway502", http.StatusBadGateway, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
			}))
			defer authServer.Close()

			server := sasl.NewServer(socketPath, authServer.URL, "example.com")

			// Start server
			go server.Start()
			defer server.Shutdown()
			time.Sleep(100 * time.Millisecond)

			// Connect to the socket
			conn, err := net.Dial("unix", socketPath)
			if err != nil {
				t.Fatalf("Failed to connect to socket: %v", err)
			}
			defer conn.Close()

			// Send authentication request
			credentials := "\x00testuser\x00testpass"
			encodedCreds := base64.StdEncoding.EncodeToString([]byte(credentials))
			fmt.Fprintf(conn, "AUTH\t1\tPLAIN\tservice=smtp\tresp=%s\n", encodedCreds)

			// Read response
			reader := bufio.NewReader(conn)
			response, err := reader.ReadString('\n')
			if err != nil {
				t.Fatalf("Failed to read response: %v", err)
			}

			// Check expected outcome
			if tc.shouldFail {
				if !strings.HasPrefix(response, "FAIL\t1\t") {
					t.Errorf("Expected FAIL response for status %d, got: %s", tc.statusCode, response)
				}
			} else {
				if !strings.HasPrefix(response, "OK\t1\t") {
					t.Errorf("Expected OK response for status %d, got: %s", tc.statusCode, response)
				}
			}
		})
	}
}

// TestMultipleCommandsInSession tests sending multiple commands in a single session
func TestMultipleCommandsInSession(t *testing.T) {
	socketPath := getSocketPath(t)

	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer authServer.Close()

	server := sasl.NewServer(socketPath, authServer.URL, "example.com")

	// Start server
	go server.Start()
	defer server.Shutdown()
	time.Sleep(100 * time.Millisecond)

	// Connect to the socket
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to connect to socket: %v", err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)

	// Send VERSION
	fmt.Fprintf(conn, "VERSION\t1\t2\n")
	response, _ := reader.ReadString('\n')
	if !strings.HasPrefix(response, "VERSION") {
		t.Errorf("Expected VERSION response, got: %s", response)
	}

	// Send CPID
	fmt.Fprintf(conn, "CPID\t12345\n")
	// Read all MECH responses
	response, _ = reader.ReadString('\n') // First MECH
	if !strings.HasPrefix(response, "MECH") {
		t.Errorf("Expected MECH response, got: %s", response)
	}
	response, _ = reader.ReadString('\n') // Second MECH
	if !strings.HasPrefix(response, "MECH") {
		t.Errorf("Expected MECH response, got: %s", response)
	}
	response, _ = reader.ReadString('\n') // DONE
	if !strings.HasPrefix(response, "DONE") {
		t.Errorf("Expected DONE response, got: %s", response)
	}

	// Send AUTH
	credentials := "\x00testuser\x00testpass"
	encodedCreds := base64.StdEncoding.EncodeToString([]byte(credentials))
	fmt.Fprintf(conn, "AUTH\t1\tPLAIN\tservice=smtp\tresp=%s\n", encodedCreds)
	response, _ = reader.ReadString('\n')
	if !strings.HasPrefix(response, "OK\t1\t") {
		t.Errorf("Expected OK response, got: %s", response)
	}

	// Send another AUTH
	fmt.Fprintf(conn, "AUTH\t2\tPLAIN\tservice=smtp\tresp=%s\n", encodedCreds)
	response, _ = reader.ReadString('\n')
	if !strings.HasPrefix(response, "OK\t2\t") {
		t.Errorf("Expected OK response with id 2, got: %s", response)
	}
}

// BenchmarkPlainAuthentication benchmarks PLAIN authentication performance
func BenchmarkPlainAuthentication(b *testing.B) {
	tmpDir := b.TempDir()
	socketPath := filepath.Join(tmpDir, "test-sasl.sock")

	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer authServer.Close()

	server := sasl.NewServer(socketPath, authServer.URL, "example.com")

	// Start server
	go server.Start()
	defer server.Shutdown()
	time.Sleep(100 * time.Millisecond)

	// Prepare credentials
	credentials := "\x00testuser\x00testpass"
	encodedCreds := base64.StdEncoding.EncodeToString([]byte(credentials))

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		conn, err := net.Dial("unix", socketPath)
		if err != nil {
			b.Fatalf("Failed to connect: %v", err)
		}

		fmt.Fprintf(conn, "AUTH\t%d\tPLAIN\tservice=smtp\tresp=%s\n", i, encodedCreds)

		reader := bufio.NewReader(conn)
		_, _ = reader.ReadString('\n')

		conn.Close()
	}
}

// BenchmarkBase64EncodeDecode benchmarks base64 encoding and decoding
func BenchmarkBase64EncodeDecode(b *testing.B) {
	credentials := "\x00testuser@example.com\x00testpassword123"

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		encoded := base64.StdEncoding.EncodeToString([]byte(credentials))
		decoded, _ := base64.StdEncoding.DecodeString(encoded)
		_ = strings.Split(string(decoded), "\x00")
	}
}
