package delivery_test

import (
	"strings"
	"testing"
	"time"

	"raven/internal/db"
	"raven/internal/delivery/config"
	"raven/internal/delivery/lmtp"
	"raven/test/helpers"
)

// LMTP integration: 6 scenarios focusing on cross-module behavior
// 1 file, 4–8 tests (SuccessFlow, DeliverSuccess, ErrorPropagation, Concurrency, DataConsistency, ShutdownRecovery)

// TestLMTP_SuccessFlow ensures server starts and accepts LHLO/QUIT
func TestLMTP_SuccessFlow(t *testing.T) {
	dbm := helpers.SetupTestDatabase(t)
	defer helpers.TeardownTestDatabase(t, dbm)

	addr, _, cleanup := helpers.StartTestLMTPServer(t, dbm.DBManager)
	defer cleanup()

	client := helpers.ConnectLMTP(t, addr)
	defer func() { _ = client.Close() }()

	if _, err := client.LHLO("localhost"); err != nil {
		t.Fatalf("LHLO failed: %v", err)
	}
	if _, err := client.QUIT(); err != nil {
		t.Fatalf("QUIT failed: %v", err)
	}
}

// TestLMTP_DeliverSuccess delivers to an existing user's INBOX
func TestLMTP_DeliverSuccess(t *testing.T) {
	dbm := helpers.SetupTestDatabase(t)
	defer helpers.TeardownTestDatabase(t, dbm)

	td := helpers.CreateTestUser(t, dbm.DBManager, "alice@example.com")

	addr, _, cleanup := helpers.StartTestLMTPServer(t, dbm.DBManager)
	defer cleanup()

	client := helpers.ConnectLMTP(t, addr)
	defer func() { _ = client.Close() }()

	if _, err := client.LHLO("mx.local"); err != nil {
		t.Fatalf("LHLO failed: %v", err)
	}
	if _, err := client.MAILFROM("sender@example.com"); err != nil {
		t.Fatalf("MAIL FROM failed: %v", err)
	}
	if _, err := client.RCPTTO("alice@example.com"); err != nil {
		t.Fatalf("RCPT TO failed: %v", err)
	}
	// RFC-compliant message with required headers
	msg := "From: sender@example.com\r\nTo: alice@example.com\r\nDate: Tue, 10 Dec 2025 23:44:37 +0000\r\nSubject: Test\r\n\r\nHello"
	if _, err := client.DATA([]byte(msg)); err != nil {
		t.Fatalf("DATA failed: %v", err)
	}

	// Properly close the LMTP session
	if _, err := client.QUIT(); err != nil {
		t.Fatalf("QUIT failed: %v", err)
	}

	t.Log("LMTP session completed successfully, verifying message delivery...")

	// Verify message count increased in INBOX
	userDB, err := dbm.GetUserDB(td.UserID)
	if err != nil {
		t.Fatalf("get user DB: %v", err)
	}
	t.Logf("Retrieved user database for UserID: %d", td.UserID)

	inboxID, err := db.GetMailboxByNamePerUser(userDB, td.UserID, "INBOX")
	if err != nil {
		t.Fatalf("get INBOX id: %v", err)
	}
	t.Logf("Found INBOX mailbox with ID: %d for user %d", inboxID, td.UserID)

	// Count messages
	count, err := db.GetMessageCountPerUser(userDB, inboxID)
	if err != nil {
		t.Fatalf("get message count: %v", err)
	}
	t.Logf("Message count in INBOX: %d", count)

	if count < 1 {
		t.Fatalf("expected at least 1 message in INBOX, got %d", count)
	}

	t.Logf("✓ Message delivery verified: %d message(s) found in INBOX", count)
}

// TestLMTP_ErrorPropagation verifies RCPT failure for unknown user
func TestLMTP_ErrorPropagation(t *testing.T) {
	dbm := helpers.SetupTestDatabase(t)
	defer helpers.TeardownTestDatabase(t, dbm)

	addr, _, cleanup := helpers.StartTestLMTPServer(t, dbm.DBManager)
	defer cleanup()

	client := helpers.ConnectLMTP(t, addr)
	defer func() { _ = client.Close() }()

	if _, err := client.LHLO("mx.local"); err != nil {
		t.Fatalf("LHLO failed: %v", err)
	}
	if _, err := client.MAILFROM("sender@example.com"); err != nil {
		t.Fatalf("MAIL FROM failed: %v", err)
	}
	// RCPT to a non-existent user should fail
	if _, err := client.RCPTTO("nouser@example.com"); err == nil {
		t.Fatalf("expected RCPT TO failure for unknown user")
	}
	if _, err := client.QUIT(); err != nil {
		t.Fatalf("QUIT failed: %v", err)
	}
}

// TestLMTP_Concurrency concurrent deliveries to multiple users
func TestLMTP_Concurrency(t *testing.T) {
	dbm := helpers.SetupTestDatabase(t)
	defer helpers.TeardownTestDatabase(t, dbm)

	users := []string{"a@example.com", "b@example.com", "c@example.com", "d@example.com"}
	for _, u := range users {
		td := helpers.CreateTestUser(t, dbm.DBManager, u)
		t.Logf("Created test user: %s (UserID: %d)", u, td.UserID)
	}

	addr, _, cleanup := helpers.StartTestLMTPServer(t, dbm.DBManager)
	defer cleanup()

	t.Logf("Starting concurrent delivery to %d users...", len(users))

	errs := make(chan error, len(users))
	for _, u := range users {
		go func(recipient string) {
			c := helpers.ConnectLMTP(t, addr)
			defer func() { _ = c.Close() }()
			if _, e := c.LHLO("mx.local"); e != nil {
				errs <- e
				return
			}
			if _, e := c.MAILFROM("sender@example.com"); e != nil {
				errs <- e
				return
			}
			if _, e := c.RCPTTO(recipient); e != nil {
				errs <- e
				return
			}
			msg := "From: sender@example.com\r\nTo: " + recipient + "\r\nDate: Tue, 10 Dec 2025 23:44:37 +0000\r\nSubject: Concurrency\r\n\r\nHi"
			if _, e := c.DATA([]byte(msg)); e != nil {
				errs <- e
				return
			}
			if _, e := c.QUIT(); e != nil {
				errs <- e
				return
			}
			t.Logf("✓ Successfully delivered message to: %s", recipient)
			errs <- nil
		}(u)
	}

	deliveredCount := 0
	for range users {
		if err := <-errs; err != nil {
			t.Errorf("delivery err: %v", err)
		} else {
			deliveredCount++
		}
	}

	t.Logf("Concurrent delivery completed: %d/%d successful", deliveredCount, len(users))
}

// TestLMTP_DataConsistency verifies unseen count and flags after delivery
func TestLMTP_DataConsistency(t *testing.T) {
	dbm := helpers.SetupTestDatabase(t)
	defer helpers.TeardownTestDatabase(t, dbm)

	td := helpers.CreateTestUser(t, dbm.DBManager, "alice@example.com")
	t.Logf("Created test user: alice@example.com (UserID: %d)", td.UserID)

	addr, _, cleanup := helpers.StartTestLMTPServer(t, dbm.DBManager)
	defer cleanup()

	c := helpers.ConnectLMTP(t, addr)
	defer func() { _ = c.Close() }()

	t.Log("Delivering message for flag consistency test...")
	_, _ = c.LHLO("mx.local")
	_, _ = c.MAILFROM("sender@example.com")
	_, _ = c.RCPTTO("alice@example.com")
	msg := "From: sender@example.com\r\nTo: alice@example.com\r\nDate: Tue, 10 Dec 2025 23:44:37 +0000\r\nSubject: Flags\r\n\r\nNew"
	_, _ = c.DATA([]byte(msg))
	_, _ = c.QUIT()

	t.Log("Message delivered, verifying database consistency...")

	userDB, _ := dbm.GetUserDB(td.UserID)
	inboxID, _ := db.GetMailboxByNamePerUser(userDB, td.UserID, "INBOX")
	t.Logf("Using INBOX mailbox ID: %d", inboxID)

	// Verify unseen count increments
	unseen, err := db.GetUnseenCountPerUser(userDB, inboxID)
	if err != nil {
		t.Fatalf("get unseen: %v", err)
	}
	t.Logf("Unseen message count: %d", unseen)
	if unseen < 1 {
		t.Fatalf("expected unseen >= 1, got %d", unseen)
	}

	// Optionally update flags and verify
	msgs, err := db.GetMessagesByMailboxPerUser(userDB, inboxID)
	if err != nil || len(msgs) == 0 {
		t.Fatalf("get messages: %v", err)
	}
	t.Logf("Retrieved %d message(s) from INBOX", len(msgs))

	// Mark first as seen
	t.Logf("Marking message %d as \\Seen", msgs[0])
	_ = db.UpdateMessageFlagsPerUser(userDB, inboxID, msgs[0], "\\Seen")
	flags, err := db.GetMessageFlagsPerUser(userDB, inboxID, msgs[0])
	if err != nil {
		t.Fatalf("get flags: %v", err)
	}
	t.Logf("Message flags after update: %q", flags)
	if !strings.Contains(flags, "\\Seen") {
		t.Fatalf("expected \\Seen flag, got %q", flags)
	}
	t.Log("✓ Flag consistency verified successfully")
}

// TestLMTP_ShutdownRecovery ensures server can shutdown and restart cleanly
func TestLMTP_ShutdownRecovery(t *testing.T) {
	dbm := helpers.SetupTestDatabase(t)
	defer helpers.TeardownTestDatabase(t, dbm)

	t.Log("Starting initial LMTP server...")
	_, srv, _ := helpers.StartTestLMTPServer(t, dbm.DBManager)
	// Shutdown first server (don't use cleanup since we're manually shutting down)
	t.Log("Shutting down initial server...")
	_ = srv.Shutdown()

	t.Log("Restarting LMTP server with fresh configuration...")
	// Restart on a new listener with full config
	cfg := &config.Config{}
	cfg.LMTP.TCPAddress = "127.0.0.1:0"
	cfg.LMTP.UnixSocket = ""
	cfg.LMTP.Hostname = "localhost"
	cfg.LMTP.MaxSize = 1024 * 1024
	cfg.LMTP.MaxRecipients = 50
	cfg.Delivery.DefaultFolder = "INBOX"
	cfg.Delivery.AllowedDomains = []string{"example.com"}
	cfg.Delivery.RejectUnknownUser = true

	srv2 := lmtp.NewServer(dbm.DBManager, cfg)
	go func() { _ = srv2.Start() }()

	// Wait for addr with longer timeout and sleep
	var addr2 string
	t.Log("Waiting for restarted server to become available...")
	for i := 0; i < 100; i++ {
		if ln := srv2.TCPAddr(); ln != nil {
			addr2 = ln.String()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if addr2 == "" {
		t.Fatalf("LMTP did not restart")
	}
	t.Logf("✓ LMTP server restarted successfully on: %s", addr2)

	t.Log("Testing connection to restarted server...")
	c := helpers.ConnectLMTP(t, addr2)
	defer func() { _ = c.Close() }()
	_, _ = c.LHLO("mx.local")
	_, _ = c.QUIT()
	t.Log("✓ Successfully connected to and communicated with restarted server")

	// Cleanup second server
	t.Log("Cleaning up restarted server...")
	_ = srv2.Shutdown()
	t.Log("✓ Shutdown recovery test completed")
}
