# Test Directory Structure

This directory contains all tests for the Raven email server, organized according to Go best practices.

## Directory Structure

```
test/
├── helpers/              # Shared utilities for all tests
│   ├── database.go      # Database setup and teardown utilities
│   ├── fixtures.go      # Test data loading utilities
│   └── server.go        # Server setup utilities (IMAP, LMTP)
│
├── integration/         # Integration tests (cross-module behavior)
│   ├── db/              # Database integration tests
│   ├── delivery/        # LMTP delivery integration tests  
│   ├── sasl/            # SASL authentication integration tests
│   ├── server/          # IMAP server integration tests
│   └── verify-infrastructure.sh
│
├── e2e/                 # End-to-end system tests
│   └── e2e_test.go      # Full workflow tests with Docker
│
├── fixtures/            # Shared test data
│   ├── *.eml            # Test email files
│   ├── *.json           # Configuration fixtures
│   └── *.yaml           # YAML configuration files
│
└── README.md            # This file
```

## Test Categories

### Unit Tests
- Located within individual packages alongside source code
- Test individual functions and methods in isolation
- Run with: `go test ./internal/...`

### Integration Tests (`test/integration/`)
- Test interactions between modules and components
- Use real databases, mock external services
- Test specific components like DB, LMTP, SASL, IMAP
- Run with: `go test ./test/integration/...`

### End-to-End Tests (`test/e2e/`)
- **Enterprise-grade testing** following industry best practices
- Test complete workflows with **real services** (no Docker simulation)
- **Fresh isolated environment** for each test with temporary databases
- **Arrange → Act → Assert** pattern with proper resource cleanup
- **Health checks and retry logic** instead of sleep-based waits
- **Dedicated test configuration** separate from production configs
- Run with: `make test-e2e` or `go test ./test/e2e/...`

#### E2E Test Categories:
- **IMAP Tests**: `make test-e2e-imap` - Protocol connectivity, authentication, session lifecycle
- **Delivery Tests**: `make test-e2e-delivery` - Email delivery simulation and roundtrip testing
- **Coverage**: `make test-e2e-coverage` - E2E tests with coverage analysis

## Shared Utilities (`test/helpers/`)

The helpers package provides common utilities used across all test types:

- **Database helpers**: Setup/teardown test databases, create test users
- **Docker helpers**: Manage containerized test environments  
- **Fixture helpers**: Load test emails and configuration data
- **Server helpers**: Start/stop test IMAP and LMTP servers

## Usage Examples

### Running Tests

```bash
# Run all tests (short mode, skips Docker tests)
go test ./test/... -v -short

# Run integration tests only
go test ./test/integration/... -v

# Run end-to-end tests (requires Docker)
go test ./test/e2e/... -v

# Run specific component tests
go test ./test/integration/db/... -v
```

### Using Helpers in Tests

```go
import "raven/test/helpers"

func TestExample(t *testing.T) {
    // Setup test database
    dbManager := helpers.SetupTestDatabase(t)
    defer helpers.TeardownTestDatabase(t, dbManager)
    
    // Load test fixtures
    email := helpers.LoadSimpleEmail(t)
    users := helpers.LoadTestUsers(t)
    
    // Start test servers
    imapServer := helpers.StartTestIMAPServer(t, dbManager.DBManager)
    defer imapServer.Stop(t)
}
```