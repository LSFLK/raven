package lmtp

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"raven/internal/db"
	"raven/internal/delivery/config"
)

func setupTestDBManager(t *testing.T) *db.DBManager {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "lmtp_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(tmpDir)
	})

	dbManager, err := db.NewDBManager(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create DB manager: %v", err)
	}
	t.Cleanup(func() {
		_ = dbManager.Close()
	})

	return dbManager
}

func setupTestConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg := config.DefaultConfig()

	tmpDir, err := os.MkdirTemp("", "lmtp_sock_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(tmpDir)
	})

	cfg.LMTP.UnixSocket = filepath.Join(tmpDir, "test.sock")
	cfg.LMTP.TCPAddress = "127.0.0.1:0" // Use any available port
	cfg.LMTP.Timeout = 5
	cfg.LMTP.MaxSize = 1024 * 1024 // 1MB for tests
	cfg.LMTP.Hostname = "test.example.com"
	cfg.LMTP.MaxRecipients = 10
	cfg.Delivery.DefaultFolder = "INBOX"

	return cfg
}

func TestNewServer(t *testing.T) {
	dbManager := setupTestDBManager(t)
	cfg := setupTestConfig(t)

	server := NewServer(dbManager, cfg)

	if server == nil {
		t.Fatal("Expected non-nil server")
	}

	if server.dbManager == nil {
		t.Error("Expected non-nil dbManager")
	}

	if server.config == nil {
		t.Error("Expected non-nil config")
	}

	if server.storage == nil {
		t.Error("Expected non-nil storage")
	}

	if server.shutdown == nil {
		t.Error("Expected non-nil shutdown channel")
	}
}

func TestServer_StartUnixListener(t *testing.T) {
	dbManager := setupTestDBManager(t)
	cfg := setupTestConfig(t)
	cfg.LMTP.TCPAddress = "" // Disable TCP listener

	server := NewServer(dbManager, cfg)

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- server.Start()
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Check if socket file was created
	if _, err := os.Stat(cfg.LMTP.UnixSocket); os.IsNotExist(err) {
		t.Error("Expected socket file to be created")
	}

	// Check socket permissions
	info, err := os.Stat(cfg.LMTP.UnixSocket)
	if err != nil {
		t.Fatalf("Failed to stat socket file: %v", err)
	}

	expectedPerm := os.FileMode(0666)
	if info.Mode().Perm() != expectedPerm {
		t.Errorf("Expected socket permissions %v, got %v", expectedPerm, info.Mode().Perm())
	}

	// Try to connect to the socket
	conn, err := net.Dial("unix", cfg.LMTP.UnixSocket)
	if err != nil {
		t.Fatalf("Failed to connect to UNIX socket: %v", err)
	}
	_ = conn.Close()

	// Shutdown server
	if err := server.Shutdown(); err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}

	// Wait for Start to finish
	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("Start returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Error("Server did not shut down in time")
	}

	// Verify socket file is cleaned up
	if _, err := os.Stat(cfg.LMTP.UnixSocket); !os.IsNotExist(err) {
		t.Error("Expected socket file to be removed after shutdown")
	}
}

func TestServer_StartTCPListener(t *testing.T) {
	dbManager := setupTestDBManager(t)
	cfg := setupTestConfig(t)
	cfg.LMTP.UnixSocket = "" // Disable UNIX socket

	server := NewServer(dbManager, cfg)

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- server.Start()
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Get the actual address (port was 0, so OS assigned one)
	tcpAddr := server.TCPAddr()
	if tcpAddr == nil {
		t.Fatal("Expected TCP listener to be created")
	}

	addr := tcpAddr.String()

	// Try to connect to the TCP address
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Failed to connect to TCP address: %v", err)
	}
	_ = conn.Close()

	// Shutdown server
	if err := server.Shutdown(); err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}

	// Wait for Start to finish
	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("Start returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Error("Server did not shut down in time")
	}
}

func TestServer_StartBothListeners(t *testing.T) {
	dbManager := setupTestDBManager(t)
	cfg := setupTestConfig(t)

	server := NewServer(dbManager, cfg)

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- server.Start()
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Check UNIX socket
	if _, err := os.Stat(cfg.LMTP.UnixSocket); os.IsNotExist(err) {
		t.Error("Expected socket file to be created")
	}

	// Check TCP listener
	tcpAddrVal := server.TCPAddr()
	if tcpAddrVal == nil {
		t.Fatal("Expected TCP listener to be created")
	}

	// Try to connect to both
	unixConn, err := net.Dial("unix", cfg.LMTP.UnixSocket)
	if err != nil {
		t.Errorf("Failed to connect to UNIX socket: %v", err)
	} else {
		_ = unixConn.Close()
	}

	tcpAddr := tcpAddrVal.String()
	tcpConn, err := net.Dial("tcp", tcpAddr)
	if err != nil {
		t.Errorf("Failed to connect to TCP address: %v", err)
	} else {
		_ = tcpConn.Close()
	}

	// Shutdown server
	if err := server.Shutdown(); err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}

	// Wait for Start to finish
	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("Start returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Error("Server did not shut down in time")
	}
}

func TestServer_StartNoListeners(t *testing.T) {
	dbManager := setupTestDBManager(t)
	cfg := setupTestConfig(t)
	cfg.LMTP.UnixSocket = ""
	cfg.LMTP.TCPAddress = ""

	server := NewServer(dbManager, cfg)

	// Start should return quickly since no listeners are configured
	err := server.Start()
	if err != nil {
		t.Errorf("Start returned error: %v", err)
	}
}

func TestServer_StartUnixListener_InvalidPath(t *testing.T) {
	dbManager := setupTestDBManager(t)
	cfg := setupTestConfig(t)
	cfg.LMTP.UnixSocket = "/invalid/nonexistent/path/test.sock"
	cfg.LMTP.TCPAddress = ""

	server := NewServer(dbManager, cfg)

	err := server.Start()
	if err == nil {
		t.Error("Expected error for invalid UNIX socket path")
	}
}

func TestServer_StartTCPListener_InvalidAddress(t *testing.T) {
	dbManager := setupTestDBManager(t)
	cfg := setupTestConfig(t)
	cfg.LMTP.UnixSocket = ""
	cfg.LMTP.TCPAddress = "invalid:address:format"

	server := NewServer(dbManager, cfg)

	err := server.Start()
	if err == nil {
		t.Error("Expected error for invalid TCP address")
	}
}

func TestServer_Shutdown_Graceful(t *testing.T) {
	dbManager := setupTestDBManager(t)
	cfg := setupTestConfig(t)

	server := NewServer(dbManager, cfg)

	// Start server
	errChan := make(chan error, 1)
	go func() {
		errChan <- server.Start()
	}()

	time.Sleep(100 * time.Millisecond)

	// Shutdown should complete without error
	if err := server.Shutdown(); err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}

	// Wait for Start to complete
	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("Start returned error after shutdown: %v", err)
		}
	case <-time.After(time.Second):
		t.Error("Server did not complete after shutdown")
	}
}

func TestServer_HandleConnection_TCPOptions(t *testing.T) {
	dbManager := setupTestDBManager(t)
	cfg := setupTestConfig(t)
	cfg.LMTP.UnixSocket = ""

	server := NewServer(dbManager, cfg)

	// Start server
	go func() {
		_ = server.Start()
	}()

	time.Sleep(100 * time.Millisecond)

	// Connect to server
	tcpAddr := server.TCPAddr().String()
	conn, err := net.Dial("tcp", tcpAddr)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Verify TCP connection
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		t.Fatal("Expected TCP connection")
	}

	// These options should be set by the server
	// We can't directly verify them, but we can ensure connection is stable
	_ = tcpConn

	// Send QUIT to cleanly close
	_, _ = conn.Write([]byte("QUIT\r\n"))

	// Wait for response
	buf := make([]byte, 256)
	_, _ = conn.Read(buf)

	// Shutdown server
	if err := server.Shutdown(); err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}
}

func TestServer_HandleConnection_UnixSocket(t *testing.T) {
	dbManager := setupTestDBManager(t)
	cfg := setupTestConfig(t)
	cfg.LMTP.TCPAddress = ""

	server := NewServer(dbManager, cfg)

	// Start server
	go func() {
		_ = server.Start()
	}()

	time.Sleep(100 * time.Millisecond)

	// Connect to UNIX socket
	conn, err := net.Dial("unix", cfg.LMTP.UnixSocket)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Read greeting
	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("Failed to read greeting: %v", err)
	}

	greeting := string(buf[:n])
	if !contains(greeting, "220") {
		t.Errorf("Expected 220 greeting, got: %s", greeting)
	}

	// Send QUIT
	_, err = conn.Write([]byte("QUIT\r\n"))
	if err != nil {
		t.Fatalf("Failed to send QUIT: %v", err)
	}

	// Read QUIT response
	n, err = conn.Read(buf)
	if err != nil {
		t.Fatalf("Failed to read QUIT response: %v", err)
	}

	response := string(buf[:n])
	if !contains(response, "221") {
		t.Errorf("Expected 221 response, got: %s", response)
	}

	// Shutdown server
	if err := server.Shutdown(); err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}
}

func TestServer_MultipleConnections(t *testing.T) {
	dbManager := setupTestDBManager(t)
	cfg := setupTestConfig(t)
	cfg.LMTP.UnixSocket = ""

	server := NewServer(dbManager, cfg)

	// Start server
	go func() {
		_ = server.Start()
	}()

	time.Sleep(100 * time.Millisecond)

	tcpAddr := server.TCPAddr().String()

	// Create multiple connections
	numConns := 5
	conns := make([]net.Conn, numConns)
	for i := 0; i < numConns; i++ {
		conn, err := net.Dial("tcp", tcpAddr)
		if err != nil {
			t.Fatalf("Failed to create connection %d: %v", i, err)
		}
		conns[i] = conn
		defer func(c net.Conn) { _ = c.Close() }(conn)
	}

	// Send QUIT to all connections
	for i, conn := range conns {
		_, err := conn.Write([]byte("QUIT\r\n"))
		if err != nil {
			t.Errorf("Failed to send QUIT on connection %d: %v", i, err)
		}
	}

	// Read responses
	for i, conn := range conns {
		buf := make([]byte, 256)
		_, err := conn.Read(buf)
		if err != nil {
			t.Errorf("Failed to read response on connection %d: %v", i, err)
		}
	}

	// Shutdown server
	if err := server.Shutdown(); err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
