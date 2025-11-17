package server

import (
	"strings"
	"testing"

	"raven/internal/models"
	
)

// HasCapabilityToken is a helper to check capability tokens exactly (avoids substring matches like LOGIN in LOGINDISABLED)
func HasCapabilityToken(line, token string) bool {
	line = strings.TrimSpace(line)
	const prefix = "* CAPABILITY "
	if !strings.HasPrefix(line, prefix) {
		return false
	}
	caps := strings.Fields(strings.TrimSpace(line[len(prefix):]))
	for _, c := range caps {
		if c == token {
			return true
		}
	}
	return false
}

// TestCapabilityCommand_NonTLSConnection tests CAPABILITY command over non-TLS connection
func TestCapabilityCommand_NonTLSConnection(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	state := &models.ClientState{
		Authenticated: false,
	}

	// Test CAPABILITY command
	srv.HandleCapability(conn, "A001", state)

	response := conn.GetWrittenData()
	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Should have 2 lines: capability response and completion
	if len(lines) != 2 {
		t.Fatalf("Expected 2 response lines, got %d: %v", len(lines), lines)
	}

	// Check untagged CAPABILITY response
	capLine := lines[0]
	if !strings.HasPrefix(capLine, "* CAPABILITY ") {
		t.Errorf("Expected CAPABILITY response to start with '* CAPABILITY ', got: %s", capLine)
	}

	// Must include IMAP4rev1
	if !strings.Contains(capLine, "IMAP4rev1") {
		t.Errorf("CAPABILITY response must include IMAP4rev1, got: %s", capLine)
	}

	// For non-TLS connections, should include STARTTLS and LOGINDISABLED
	if !strings.Contains(capLine, "STARTTLS") {
		t.Errorf("Non-TLS connection should advertise STARTTLS, got: %s", capLine)
	}

	if !strings.Contains(capLine, "LOGINDISABLED") {
		t.Errorf("Non-TLS connection should advertise LOGINDISABLED, got: %s", capLine)
	}

	// Should NOT include AUTH=PLAIN or LOGIN for non-TLS
	if strings.Contains(capLine, "AUTH=PLAIN") {
		t.Errorf("Non-TLS connection should not advertise AUTH=PLAIN, got: %s", capLine)
	}

	// Use exact token matching to avoid matching LOGIN within LOGINDISABLED
	if HasCapabilityToken(capLine, "LOGIN") {
		t.Errorf("Non-TLS connection should not advertise LOGIN, got: %s", capLine)
	}

	// Should include extension capabilities
	extensionCaps := []string{"UIDPLUS", "IDLE", "NAMESPACE", "UNSELECT", "LITERAL+"}
	for _, cap := range extensionCaps {
		if !strings.Contains(capLine, cap) {
			t.Errorf("Expected capability %s to be advertised, got: %s", cap, capLine)
		}
	}

	// Check tagged OK response
	okLine := lines[1]
	expectedOK := "A001 OK CAPABILITY completed"
	if okLine != expectedOK {
		t.Errorf("Expected tagged OK response '%s', got: '%s'", expectedOK, okLine)
	}
}

// TestCapabilityCommand_TLSConnection tests CAPABILITY command over TLS connection
func TestCapabilityCommand_TLSConnection(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockTLSConn()
	state := &models.ClientState{
		Authenticated: false,
	}

	// Test CAPABILITY command
	srv.HandleCapability(conn, "B002", state)

	response := conn.GetWrittenData()
	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Should have 2 lines: capability response and completion
	if len(lines) != 2 {
		t.Fatalf("Expected 2 response lines, got %d: %v", len(lines), lines)
	}

	capLine := lines[0]

	// Must include IMAP4rev1
	if !strings.Contains(capLine, "IMAP4rev1") {
		t.Errorf("CAPABILITY response must include IMAP4rev1, got: %s", capLine)
	}

	// For TLS connections, should include AUTH=PLAIN and LOGIN
	if !strings.Contains(capLine, "AUTH=PLAIN") {
		t.Errorf("TLS connection should advertise AUTH=PLAIN, got: %s", capLine)
	}

	if !strings.Contains(capLine, "LOGIN") {
		t.Errorf("TLS connection should advertise LOGIN, got: %s", capLine)
	}

	// Should NOT include STARTTLS or LOGINDISABLED for TLS connections
	if strings.Contains(capLine, "STARTTLS") {
		t.Errorf("TLS connection should not advertise STARTTLS, got: %s", capLine)
	}

	if strings.Contains(capLine, "LOGINDISABLED") {
		t.Errorf("TLS connection should not advertise LOGINDISABLED, got: %s", capLine)
	}

	// Check tagged OK response
	okLine := lines[1]
	expectedOK := "B002 OK CAPABILITY completed"
	if okLine != expectedOK {
		t.Errorf("Expected tagged OK response '%s', got: '%s'", expectedOK, okLine)
	}
}

// TestCapabilityCommand_ResponseFormat tests the exact format of CAPABILITY response
func TestCapabilityCommand_ResponseFormat(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	state := &models.ClientState{
		Authenticated: false,
	}

	srv.HandleCapability(conn, "C003", state)

	response := conn.GetWrittenData()
	
	// Check that response ends with CRLF
	if !strings.HasSuffix(response, "\r\n") {
		t.Errorf("Response should end with CRLF")
	}

	lines := strings.Split(strings.TrimSpace(response), "\r\n")
	capLine := lines[0]

	// Check format: "* CAPABILITY <capabilities>"
	parts := strings.Split(capLine, " ")
	if len(parts) < 3 {
		t.Errorf("CAPABILITY response should have at least 3 parts, got: %v", parts)
	}

	if parts[0] != "*" {
		t.Errorf("First part should be '*', got: %s", parts[0])
	}

	if parts[1] != "CAPABILITY" {
		t.Errorf("Second part should be 'CAPABILITY', got: %s", parts[1])
	}

	// Capabilities should be space-separated
	capabilities := parts[2:]
	if len(capabilities) == 0 {
		t.Errorf("Should have at least one capability listed")
	}

	// Check that IMAP4rev1 is the first or among the first capabilities
	found := false
	for _, cap := range capabilities {
		if cap == "IMAP4rev1" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("IMAP4rev1 should be among the capabilities: %v", capabilities)
	}
}

// TestCapabilityCommand_MultipleInvocations tests calling CAPABILITY multiple times
func TestCapabilityCommand_MultipleInvocations(t *testing.T) {
	srv := SetupTestServerSimple(t)
	conn := NewMockConn()
	state := &models.ClientState{
		Authenticated: false,
	}

	// Call CAPABILITY multiple times with different tags
	srv.HandleCapability(conn, "D001", state)
	srv.HandleCapability(conn, "D002", state)

	response := conn.GetWrittenData()
	lines := strings.Split(strings.TrimSpace(response), "\r\n")

	// Should have 4 lines: 2 capability responses + 2 completions
	if len(lines) != 4 {
		t.Fatalf("Expected 4 response lines, got %d: %v", len(lines), lines)
	}

	// Check first invocation
	if !strings.HasPrefix(lines[0], "* CAPABILITY ") {
		t.Errorf("First response should be CAPABILITY, got: %s", lines[0])
	}
	if lines[1] != "D001 OK CAPABILITY completed" {
		t.Errorf("Expected 'D001 OK CAPABILITY completed', got: %s", lines[1])
	}

	// Check second invocation  
	if !strings.HasPrefix(lines[2], "* CAPABILITY ") {
		t.Errorf("Third response should be CAPABILITY, got: %s", lines[2])
	}
	if lines[3] != "D002 OK CAPABILITY completed" {
		t.Errorf("Expected 'D002 OK CAPABILITY completed', got: %s", lines[3])
	}

	// Both capability lists should be identical
	if lines[0] != lines[2] {
		t.Errorf("Capability responses should be identical:\nFirst:  %s\nSecond: %s", lines[0], lines[2])
	}
}

// TestCapabilityCommand_AuthenticationStateDoesNotAffectCapabilities tests that 
// authentication state doesn't change capabilities (connection type does)
func TestCapabilityCommand_AuthenticationStateDoesNotAffectCapabilities(t *testing.T) {
	srv := SetupTestServerSimple(t)

	// Test with unauthenticated state
	conn1 := NewMockConn()
	unauthState := &models.ClientState{
		Authenticated: false,
	}
	srv.HandleCapability(conn1, "E001", unauthState)
	unauthResponse := conn1.GetWrittenData()

	// Test with authenticated state using a new connection
	conn2 := NewMockConn()
	authState := &models.ClientState{
		Authenticated: true,
		Username:      "testuser",
	}
	srv.HandleCapability(conn2, "E002", authState)
	authResponse := conn2.GetWrittenData()

	// Extract capability lines (first line of each response)
	unauthLines := strings.Split(strings.TrimSpace(unauthResponse), "\r\n")
	authLines := strings.Split(strings.TrimSpace(authResponse), "\r\n")

	unauthCapLine := unauthLines[0]
	authCapLine := authLines[0]

	// Capability list should be the same regardless of authentication state
	// (for the same connection type)
	if unauthCapLine != authCapLine {
		t.Errorf("Capabilities should be same regardless of auth state:\nUnauth: %s\nAuth:   %s", 
			unauthCapLine, authCapLine)
	}
}

// BenchmarkCapabilityCommand benchmarks the CAPABILITY command performance
func BenchmarkCapabilityCommand(b *testing.B) {
	srv := SetupTestServerSimple(&testing.T{})
	state := &models.ClientState{
		Authenticated: false,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn := NewMockConn()
		srv.HandleCapability(conn, "BENCH", state)
	}
}

// TestCapabilityCommand_ConcurrentAccess tests concurrent CAPABILITY requests
func TestCapabilityCommand_ConcurrentAccess(t *testing.T) {
	srv := SetupTestServerSimple(t)
	
	// Number of concurrent requests
	const numRequests = 10
	responses := make([]string, numRequests)
	done := make(chan int, numRequests)

	// Launch concurrent CAPABILITY requests
	for i := 0; i < numRequests; i++ {
		go func(index int) {
			conn := NewMockConn()
			state := &models.ClientState{
				Authenticated: false,
			}
			srv.HandleCapability(conn, "CONCURRENT", state)
			responses[index] = conn.GetWrittenData()
			done <- index
		}(i)
	}

	// Wait for all requests to complete
	for i := 0; i < numRequests; i++ {
		<-done
	}

	// All responses should be identical
	baseResponse := responses[0]
	for i := 1; i < numRequests; i++ {
		if responses[i] != baseResponse {
			t.Errorf("Concurrent request %d produced different response:\nBase: %s\nGot:  %s", 
				i, baseResponse, responses[i])
		}
	}
}
