# Test Directory

This directory contains integration and end-to-end tests used to validate the Raven mail server. The tests are written in Go and use real components where possible (SQLite for persistence, real LMTP/IMAP sockets). Docker is not required for the default e2e harness — tests start local servers on ephemeral ports.

## Layout (high level)

- `test/helpers/` - shared helpers for tests (database setup, server helpers, LMTP/IMAP clients, fixtures)
- `test/integration/` - focused integration tests per component
- `test/e2e/` - end-to-end tests (LMTP → DB → IMAP flow)
- `test/fixtures/` - sample emails and JSON/YAML fixtures

## Quick commands

- Run all unit/integration tests in repository:

```bash
go test ./... -v
```

- Run integration tests only:

```bash
go test ./test/integration/... -v
```

- Run the end-to-end suite (starts local IMAP/LMTP servers, uses temporary DBs):

```bash
make test-e2e
# or
go test ./test/e2e/... -v
```

- Run a single e2e test by name:

```bash
go test ./test/e2e -run TestE2E_LMTP_To_IMAP_ReceiveEmail -v
```

## Helpers overview

Key helpers available in `test/helpers` (examples):
- `SetupTestDatabase(t)` / `TeardownTestDatabase(t, db)` - create and remove a temp SQLite DB manager
- `StartTestIMAPServer(t, dbManager)` - start an IMAP server on a random port (supports STARTTLS in tests)
- `StartTestLMTPServer(t, dbManager)` - start an LMTP server on a random port
- `ConnectIMAP(t, addr)` / `ConnectLMTP(t, addr)` - thin protocol clients used by tests
- `CreateTestUser(t, dbManager, email)` - convenience to create test domain/user and return ids
- `BuildSimpleEmail(sender, recipient, subj, body)` - builds a minimal RFC-1123-style message

These helpers are the authoritative source of how tests are wired together; prefer reading the helper code for exact behavior.

## Best practices for working with tests
- Tests create temporary databases under `os.MkdirTemp()` and clean them up with `TeardownTestDatabase`.
- Tests start servers on `127.0.0.1:0` (ephemeral ports) and read back the listener address.
- IMAP tests currently use STARTTLS (test client will upgrade the connection); the test helper sets up self-signed certs and the client uses `InsecureSkipVerify=true` for the test harness only.
- Keep tests deterministic: prefer `env.WaitDelivery()` helper or short, bounded waits over long sleeps.
