package helpers

import (
	"testing"
	"time"

	"raven/test/helpers"
)

// Env encapsulates a full E2E environment: DB + LMTP + IMAP
// It intentionally uses real dependencies and network listeners.
type Env struct {
	T        *testing.T
	DB       *helpers.TestDBManager
	IMAP     *helpers.TestIMAPServer
	LMTPAddr string
	LMTPStop func()
}

// Start starts DB, LMTP and IMAP servers on random ports.
func (e *Env) Start(t *testing.T) {
	t.Helper()
	e.T = t

	// Database
	e.DB = helpers.SetupTestDatabase(t)

	// LMTP
	addr, _, cleanup := helpers.StartTestLMTPServer(t, e.DB.DBManager)
	e.LMTPAddr = addr
	e.LMTPStop = cleanup

	// IMAP
	e.IMAP = helpers.StartTestIMAPServer(t, e.DB.DBManager)

	// Brief readiness wait (servers expose listeners immediately; keep minimal)
	t.Log("E2E environment started: LMTP=" + e.LMTPAddr + ", IMAP=" + e.IMAP.Address)
}

// Stop stops servers but keeps DB unless explicitly torn down.
func (e *Env) Stop() {
	if e.IMAP != nil {
		e.IMAP.Stop(e.T)
	}
	if e.LMTPStop != nil {
		e.LMTPStop()
	}
}

// Teardown removes DB files.
func (e *Env) Teardown() {
	if e.DB != nil {
		helpers.TeardownTestDatabase(e.T, e.DB)
	}
}

// WaitDelivery Wait small deterministic delay for delivery pipeline.
func (e *Env) WaitDelivery() { time.Sleep(300 * time.Millisecond) }
