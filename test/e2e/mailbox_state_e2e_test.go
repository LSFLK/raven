package e2e

import (
	"strings"
	"testing"

	he2e "raven/test/e2e/helpers"
	"raven/test/helpers"
)

// 3️⃣ Mailbox & UIDVALIDITY Correctness
func TestE2E_IMAP_UID_Sequence_AndUIDValidity(t *testing.T) {
	// Arrange
	env := &he2e.Env{}
	env.Start(t)
	defer env.Stop()
	defer env.Teardown()

	user := "carol@example.com"
	helpers.CreateTestUser(t, env.DB.DBManager, user)

	sender := "sender@ext.com"
	subject1 := "First Message"
	subject2 := "Second Message"
	subject3 := "Third Message"
	msg1 := helpers.BuildSimpleEmail(sender, user, subject1, "one")
	msg2 := helpers.BuildSimpleEmail(sender, user, subject2, "two")
	msg3 := helpers.BuildSimpleEmail(sender, user, subject3, "three")

	// Deliver three messages
	lc := helpers.ConnectLMTP(t, env.LMTPAddr)
	defer func() { _ = lc.Close() }()

	for i, msg := range []string{msg1, msg2, msg3} {
		_, _ = lc.LHLO("mx")
		_, _ = lc.MAILFROM(sender)
		_, _ = lc.RCPTTO(user)
		_, err := lc.DATA([]byte(msg))
		if err != nil {
			t.Fatalf("Delivery %d failed: %v", i+1, err)
		}
	}

	env.WaitDelivery()

	// Act: IMAP operations
	c := helpers.ConnectIMAP(t, env.IMAP.Address)
	defer func() { _ = c.Close() }()
	_ = c.Login(user, "password123")

	if err := c.Select("INBOX"); err != nil {
		t.Fatalf("select: %v", err)
	}

	// Assert: Verify sequence 1:3 exists
	t.Run("Initial_Sequence", func(t *testing.T) {
		resp, err := c.Fetch("1:3", "(UID ENVELOPE)")
		if err != nil {
			t.Fatalf("fetch sequence failed: %v", err)
		}

		t.Logf("Initial fetch response: %v", resp)

		// Verify we got responses for 3 messages
		fetchCount := 0
		for _, line := range resp {
			if strings.Contains(line, "FETCH") {
				fetchCount++
			}
		}

		if fetchCount < 3 {
			t.Errorf("Expected 3 messages, got %d fetch responses", fetchCount)
		}
	})

	// Mark first message for deletion and expunge
	t.Run("EXPUNGE_Behavior", func(t *testing.T) {
		// Mark message 1 as deleted
		err := c.Store("1", "+FLAGS (\\Deleted)")
		if err != nil {
			t.Fatalf("STORE \\Deleted failed: %v", err)
		}

		// Expunge
		resp, err := c.SendCommand("EXPUNGE")
		if err != nil {
			t.Fatalf("EXPUNGE failed: %v", err)
		}

		t.Logf("EXPUNGE response: %v", resp)

		// After EXPUNGE, message 2 becomes sequence 1, message 3 becomes sequence 2
		// BUT UIDs should remain stable
		resp2, err := c.Fetch("1:2", "UID")
		if err != nil {
			t.Fatalf("fetch after expunge failed: %v", err)
		}

		t.Logf("Post-EXPUNGE UIDs: %v", resp2)

		// Verify we now have 2 messages
		fetchCount := 0
		for _, line := range resp2 {
			if strings.Contains(line, "FETCH") {
				fetchCount++
			}
		}

		if fetchCount != 2 {
			t.Errorf("After EXPUNGE expected 2 messages, got %d", fetchCount)
		}
	})

	// Validate UIDVALIDITY consistency
	t.Run("UIDVALIDITY_Consistency", func(t *testing.T) {
		// Re-select to get fresh UIDVALIDITY
		resp, err := c.SendCommand("SELECT INBOX")
		if err != nil {
			t.Fatalf("Re-select failed: %v", err)
		}

		uidValidityFound := false
		for _, line := range resp {
			if strings.Contains(line, "UIDVALIDITY") {
				uidValidityFound = true
				t.Logf("UIDVALIDITY: %s", line)
				break
			}
		}

		if !uidValidityFound {
			t.Error("UIDVALIDITY not returned in SELECT response")
		}
	})
}
