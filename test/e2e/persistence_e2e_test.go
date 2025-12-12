package e2e

import (
	"testing"

	he2e "raven/test/e2e/helpers"
	"raven/test/helpers"
)

// 9️⃣ IMAP After Server Restart
func TestE2E_ServerRestart_Persistence(t *testing.T) {
	// Arrange
	env := &he2e.Env{}
	env.Start(t)
	defer env.Teardown() // We'll stop/start manually

	user := "eve@example.com"
	helpers.CreateTestUser(t, env.DB.DBManager, user)

	sender := "sender@ext.com"
	msg := helpers.BuildSimpleEmail(sender, user, "Persist", "body")

	lc := helpers.ConnectLMTP(t, env.LMTPAddr)
	defer func() { _ = lc.Close() }()
	_, _ = lc.LHLO("mx")
	_, _ = lc.MAILFROM(sender)
	_, _ = lc.RCPTTO(user)
	_, _ = lc.DATA([]byte(msg))
	env.WaitDelivery()

	// Stop servers
	env.Stop()

	// Restart servers (DB stays)
	env.Start(t)
	defer env.Stop()

	// Assert the mail is still visible
	c := helpers.ConnectIMAP(t, env.IMAP.Address)
	defer func() { _ = c.Close() }()
	_ = c.Login(user, "password123")
	if err := c.Select("INBOX"); err != nil {
		t.Fatalf("select inbox: %v", err)
	}
	if _, err := c.Fetch("1", "ENVELOPE"); err != nil {
		t.Fatalf("fetch after restart: %v", err)
	}
}
