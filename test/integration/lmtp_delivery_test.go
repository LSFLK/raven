package integration_test

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"raven/internal/db"
	"raven/internal/server"
)

// MockSMTPServer is a lightweight SMTP server for testing.
type MockSMTPServer struct {
	listener net.Listener
	wg       sync.WaitGroup
	received chan string
	addr     string
}

// NewMockSMTPServer creates and starts a new mock SMTP server.
func NewMockSMTPServer(t *testing.T) *MockSMTPServer {
	t.Helper()
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to start mock SMTP server: %v", err)
	}

	server := &MockSMTPServer{
		listener: listener,
		received: make(chan string, 1),
		addr:     listener.Addr().String(),
	}

	server.wg.Add(1)
	go func() {
		defer server.wg.Done()
		server.handleConnections(t)
	}()

	return server
}

// Addr returns the address of the mock SMTP server.
func (s *MockSMTPServer) Addr() string {
	return s.addr
}

// Close stops the mock SMTP server.
func (s *MockSMTPServer) Close() {
	s.listener.Close()
	s.wg.Wait()
	close(s.received)
}

// handleConnections accepts and handles incoming connections.
func (s *MockSMTPServer) handleConnections(t *testing.T) {
	t.Helper()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return // Listener was closed
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer conn.Close()
			s.handleConnection(t, conn)
		}()
	}
}

// handleConnection handles a single SMTP connection.
func (s *MockSMTPServer) handleConnection(t *testing.T, conn net.Conn) {
	t.Helper()
	reader := bufio.NewReader(conn)
	var data strings.Builder

	// Greet the client
	fmt.Fprint(conn, "220 mock.smtp.server ESMTP\r\n")

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		data.WriteString(line)
		line = strings.ToUpper(strings.TrimSpace(line))

		switch {
		case strings.HasPrefix(line, "LHLO"):
			fmt.Fprint(conn, "250 OK\r\n")
		case strings.HasPrefix(line, "MAIL FROM"):
			fmt.Fprint(conn, "250 OK\r\n")
		case strings.HasPrefix(line, "RCPT TO"):
			fmt.Fprint(conn, "250 OK\r\n")
		case strings.HasPrefix(line, "DATA"):
			fmt.Fprint(conn, "354 End data with <CR><LF>.<CR><LF>\r\n")
			// Read email body
			for {
				bodyLine, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				data.WriteString(bodyLine)
				if bodyLine == ".\r\n" {
					break
				}
			}
			fmt.Fprint(conn, "250 OK: queued as 12345\r\n")
		case strings.HasPrefix(line, "QUIT"):
			fmt.Fprint(conn, "221 Bye\r\n")
			s.received <- data.String()
			return
		}
	}
}

// TestLMTPDelivery tests the delivery of an email to a mock SMTP server.
func TestLMTPDelivery(t *testing.T) {
	// 1. Setup mock SMTP server
	smtpServer := NewMockSMTPServer(t)
	defer smtpServer.Close()

	// 2. Setup Raven server
	ss, cleanup := server.SetupTestServer(t)
	defer cleanup()

	username := "testuser@example.com"
	state := server.SetupAuthenticatedState(t, ss, username)

	// 3. Create a message
	dbManager := ss.GetDBManager().(*db.DBManager)
	userDB, err := dbManager.GetUserDB(state.UserID)
	if err != nil {
		t.Fatalf("Failed to get user database: %v", err)
	}

	subject := "LMTP Delivery Test"
	body := "This is a test of LMTP delivery."
	fullMessage := fmt.Sprintf("Subject: %s\r\n\r\n%s", subject, body)
	messageID, err := db.CreateMessage(userDB, subject, "", "", time.Now(), int64(len(fullMessage)))
	if err != nil {
		t.Fatalf("Failed to create message: %v", err)
	}

	// 4. Trigger delivery
	ds := delivery.New(dbManager)
	recipient := "recipient@example.com"
	err = ds.Queue(messageID, "sender@example.com", recipient)
	if err != nil {
		t.Fatalf("Failed to queue message for delivery: %v", err)
	}

	// 5. Run delivery process
	go ds.Start()
	defer ds.Stop()

	// 6. Assert
	select {
	case receivedEmail := <-smtpServer.received:
		t.Logf("Received email:\n%s", receivedEmail)
		if !strings.Contains(receivedEmail, subject) {
			t.Errorf("Expected email subject '%s' not found in received email", subject)
		}
		if !strings.Contains(receivedEmail, body) {
			t.Errorf("Expected email body '%s' not found in received email", body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for email to be delivered")
	}
}
