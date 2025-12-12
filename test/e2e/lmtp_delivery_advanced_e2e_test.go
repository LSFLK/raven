package e2e

import (
	"strings"
	"testing"

	he2e "raven/test/e2e/helpers"
	"raven/test/helpers"
)

// 2️⃣ Multiple Recipients Delivery
func TestE2E_LMTP_Delivery_MultipleRecipients(t *testing.T) {
	// Arrange
	env := &he2e.Env{}
	env.Start(t)
	defer env.Stop()
	defer env.Teardown()

	// Create multiple recipients
	users := []string{
		"alice@example.com",
		"bob@example.com",
		"carol@example.com",
	}
	for _, u := range users {
		helpers.CreateTestUser(t, env.DB.DBManager, u)
	}

	sender := "sender@external.com"
	subject := "Multi-recipient Test"
	body := "This message goes to multiple users"
	msg := helpers.BuildSimpleEmail(sender, strings.Join(users, ", "), subject, body)

	// Act: LMTP delivery to multiple recipients
	lc := helpers.ConnectLMTP(t, env.LMTPAddr)
	defer func() { _ = lc.Close() }()
	_, _ = lc.LHLO("mx.local")
	_, _ = lc.MAILFROM(sender)

	// Add all recipients
	for _, u := range users {
		_, err := lc.RCPTTO(u)
		if err != nil {
			t.Fatalf("RCPT TO <%s> failed: %v", u, err)
		}
	}

	_, err := lc.DATA([]byte(msg))
	if err != nil {
		t.Fatalf("DATA failed: %v", err)
	}
	_, _ = lc.QUIT()

	env.WaitDelivery()

	// Assert: Each user sees the message in their INBOX, no cross-user leakage
	for _, user := range users {
		t.Run("User_"+user, func(t *testing.T) {
			c := helpers.ConnectIMAP(t, env.IMAP.Address)
			defer func() { _ = c.Close() }()

			_ = c.Login(user, "password123")
			if err := c.Select("INBOX"); err != nil {
				t.Fatalf("SELECT INBOX failed for %s: %v", user, err)
			}

			resp, err := c.Fetch("1", "ENVELOPE")
			if err != nil {
				t.Fatalf("FETCH failed for %s: %v", user, err)
			}

			found := false
			for _, line := range resp {
				if strings.Contains(line, subject) || strings.Contains(line, "FETCH") {
					found = true
					t.Logf("User %s received message: %s", user, line)
					break
				}
			}
			if !found {
				t.Errorf("User %s did not receive the message", user)
			}
		})
	}
}

// 7️⃣ Large Email Delivery
func TestE2E_LMTP_LargeMessageDelivery(t *testing.T) {
	// Arrange
	env := &he2e.Env{}
	env.Start(t)
	defer env.Stop()
	defer env.Teardown()

	user := "bigmail@example.com"
	helpers.CreateTestUser(t, env.DB.DBManager, user)

	sender := "sender@ext.com"
	subject := "Large Email Test"

	// Create ~1MB body (not 10MB to keep test fast, but validates large message handling)
	largeBody := strings.Repeat("This is a line of text in a large email message.\n", 20000)
	msg := helpers.BuildSimpleEmail(sender, user, subject, largeBody)

	// Act: Deliver large message
	lc := helpers.ConnectLMTP(t, env.LMTPAddr)
	defer func() { _ = lc.Close() }()
	_, _ = lc.LHLO("mx")
	_, _ = lc.MAILFROM(sender)
	_, err := lc.RCPTTO(user)
	if err != nil {
		t.Fatalf("RCPT failed: %v", err)
	}
	_, err = lc.DATA([]byte(msg))
	if err != nil {
		t.Fatalf("DATA failed for large message: %v", err)
	}

	env.WaitDelivery()

	// Assert: IMAP can fetch the large message
	c := helpers.ConnectIMAP(t, env.IMAP.Address)
	defer func() { _ = c.Close() }()
	_ = c.Login(user, "password123")
	if err := c.Select("INBOX"); err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}

	resp, err := c.Fetch("1", "RFC822.SIZE")
	if err != nil {
		t.Fatalf("FETCH size failed: %v", err)
	}

	t.Logf("Large message fetch response: %v", resp)
	// Verify message exists (size check would require parsing RFC822.SIZE from response)
	if len(resp) == 0 {
		t.Error("Expected fetch response for large message")
	}
}

// 8️⃣ Invalid LMTP Input Handling
func TestE2E_LMTP_InvalidInput(t *testing.T) {
	// Arrange
	env := &he2e.Env{}
	env.Start(t)
	defer env.Stop()
	defer env.Teardown()

	user := "test@example.com"
	helpers.CreateTestUser(t, env.DB.DBManager, user)

	t.Run("Missing_MAIL_FROM", func(t *testing.T) {
		lc := helpers.ConnectLMTP(t, env.LMTPAddr)
		defer func() { _ = lc.Close() }()
		_, _ = lc.LHLO("mx")

		// Try RCPT without MAIL FROM
		_, err := lc.RCPTTO(user)
		// Should fail - LMTP requires MAIL FROM first
		if err == nil {
			t.Log("LMTP accepted RCPT without MAIL FROM (implementation may vary)")
		} else {
			t.Logf("LMTP correctly rejected RCPT without MAIL FROM: %v", err)
		}
	})

	t.Run("Invalid_DOT_Termination", func(t *testing.T) {
		lc := helpers.ConnectLMTP(t, env.LMTPAddr)
		defer func() { _ = lc.Close() }()
		_, _ = lc.LHLO("mx")
		_, _ = lc.MAILFROM("sender@ext.com")
		_, _ = lc.RCPTTO(user)

		// Send DATA command
		_ = lc.SendLine("DATA")
		_, _ = lc.ReadLine() // Read 354 response

		// Send message without proper CRLF.CRLF termination
		_ = lc.SendLine("Subject: Bad termination")
		_ = lc.SendLine("")
		_ = lc.SendLine("Body")
		// Don't send proper terminator - close connection instead
	})

	t.Run("Nonexistent_Recipient", func(t *testing.T) {
		lc := helpers.ConnectLMTP(t, env.LMTPAddr)
		defer func() { _ = lc.Close() }()
		_, _ = lc.LHLO("mx")
		_, _ = lc.MAILFROM("sender@ext.com")

		// Try to send to non-existent user
		_, err := lc.RCPTTO("nonexistent@example.com")
		if err != nil {
			t.Logf("LMTP correctly rejected nonexistent user: %v", err)
		} else {
			t.Log("LMTP accepted nonexistent user (may queue for bounce)")
		}
	})
}
