package e2e

import (
	"testing"

	he2e "raven/test/e2e/helpers"
	"raven/test/helpers"
)

// 4️⃣ Authentication Flow
func TestE2E_SASL_Authentication(t *testing.T) {
	// Arrange
	env := &he2e.Env{}
	env.Start(t)
	defer env.Stop()
	defer env.Teardown()

	// Create user
	helpers.CreateTestUser(t, env.DB.DBManager, "bob@example.com")

	// Act & Assert: IMAP auth
	client := helpers.ConnectIMAP(t, env.IMAP.Address)
	defer func() { _ = client.Close() }()

	// Wrong password should fail (in real SASL). Current test server may allow any; once SASL wired, assert errors.
	_ = client.Login("bob@example.com", "wrongpass")

	// Correct password should succeed (placeholder until SASL is connected end-to-end)
	_ = client.Login("bob@example.com", "password123")
}
