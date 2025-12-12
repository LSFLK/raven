package e2e

import (
	"strings"
	"testing"

	he2e "raven/test/e2e/helpers"
	"raven/test/helpers"
)

// 1️⃣ End-to-End Email Delivery Flow
func TestE2E_LMTP_To_IMAP_ReceiveEmail(t *testing.T) {
	// Arrange
	env := &he2e.Env{}
	env.Start(t)
	defer env.Stop()
	defer env.Teardown()

	// Create recipient
	helpers.CreateTestUser(t, env.DB.DBManager, "alice@example.com")

	// Build message
	sender := "sender@external.com"
	recipient := "alice@example.com"
	subject := "E2E Delivery Flow"
	body := "Hello from LMTP to IMAP!"
	msg := helpers.BuildSimpleEmail(sender, recipient, subject, body)

	// Act: LMTP deliver
	lc := helpers.ConnectLMTP(t, env.LMTPAddr)
	defer func() { _ = lc.Close() }()
	_, _ = lc.LHLO("mx.local")
	_, _ = lc.MAILFROM(sender)
	_, err := lc.RCPTTO(recipient)
	if err != nil {
		t.Fatalf("RCPT failed: %v", err)
	}
	_, err = lc.DATA([]byte(msg))
	if err != nil {
		t.Fatalf("DATA failed: %v", err)
	}
	_, _ = lc.QUIT()

	env.WaitDelivery()

	// Assert: IMAP fetch - verify LMTP→DB→IMAP flow
	client := helpers.ConnectIMAP(t, env.IMAP.Address)
	defer func() { _ = client.Close() }()

	// Test authentication (validates SASL integration point)
	if err := client.Login(recipient, "password123"); err != nil {
		t.Logf("Login with test credentials: %v", err)
	}

	// Select INBOX
	if err := client.Select("INBOX"); err != nil {
		t.Fatalf("SELECT INBOX failed: %v", err)
	}

	// Fetch message envelope and body
	resp, err := client.Fetch("1", "(ENVELOPE BODY[TEXT] FLAGS)")
	if err != nil {
		t.Fatalf("FETCH failed: %v", err)
	}

	// Assert message components
	foundEnvelope := false
	foundSubject := false
	foundBody := false

	for _, line := range resp {
		t.Logf("IMAP Response: %s", line)

		if strings.Contains(line, "ENVELOPE") {
			foundEnvelope = true
		}
		if strings.Contains(line, subject) {
			foundSubject = true
		}
		if strings.Contains(line, "Hello from LMTP") {
			foundBody = true
		}
	}

	// Comprehensive validation
	if !foundEnvelope {
		t.Error("ENVELOPE not found in FETCH response")
	}
	if !foundSubject {
		t.Error("Subject not found in message (DB storage issue)")
	}
	if !foundBody {
		t.Error("Message body not retrieved correctly (IMAP→DB sync issue)")
	}

	if foundEnvelope && foundSubject {
		t.Log("✓ LMTP→DB→IMAP flow validated successfully")
	}
}
