package e2e

import (
	"sync"
	"testing"

	he2e "raven/test/e2e/helpers"
	"raven/test/helpers"
)

// 6️⃣ Concurrent Delivery + Read
func TestE2E_ConcurrentDeliveryAndFetch(t *testing.T) {
	// Arrange
	env := &he2e.Env{}
	env.Start(t)
	defer env.Stop()
	defer env.Teardown()

	user := "dave@example.com"
	helpers.CreateTestUser(t, env.DB.DBManager, user)

	sender := "sender@ext.com"

	// Act: concurrent deliveries + read
	wg := sync.WaitGroup{}
	deliver := func(i int) {
		defer wg.Done()
		lc := helpers.ConnectLMTP(t, env.LMTPAddr)
		defer func() { _ = lc.Close() }()
		_, _ = lc.LHLO("mx")
		_, _ = lc.MAILFROM(sender)
		_, _ = lc.RCPTTO(user)
		msg := helpers.BuildSimpleEmail(sender, user, "Msg "+string(rune('A'+i)), "body")
		_, _ = lc.DATA([]byte(msg))
	}

	reads := func() {
		defer wg.Done()
		c := helpers.ConnectIMAP(t, env.IMAP.Address)
		defer func() { _ = c.Close() }()
		_ = c.Login(user, "password123")
		_ = c.Select("INBOX")
		_, _ = c.Fetch("1:*", "ENVELOPE")
	}

	wg.Add(6)
	for i := 0; i < 4; i++ {
		go deliver(i)
	}
	for i := 0; i < 2; i++ {
		go reads()
	}
	wg.Wait()
	env.WaitDelivery()

	// Assert minimal: inbox list should succeed
	c := helpers.ConnectIMAP(t, env.IMAP.Address)
	defer func() { _ = c.Close() }()
	_ = c.Login(user, "password123")
	if _, err := c.List("", "*"); err != nil {
		t.Fatalf("list: %v", err)
	}
}
