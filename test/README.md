# Test Directory Structure

This directory contains all tests for the Raven email server, organized according to Go best practices.

## Directory Structure

```
test/
├── helpers/              # Shared utilities for all tests
│   ├── database.go      # Database setup and teardown utilities
│   ├── docker.go        # Docker environment management
│   ├── fixtures.go      # Test data loading utilities
│   └── server.go        # Server setup utilities (IMAP, LMTP)
│
├── integration/         # Integration tests (cross-module behavior)
│   ├── db/              # Database integration tests
│   ├── delivery/        # LMTP delivery integration tests  
│   ├── sasl/            # SASL authentication integration tests
│   ├── server/          # IMAP server integration tests
│   ├── docker-compose.yml    # Docker services for integration tests
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
- Test complete workflows with real services running in Docker
- Validate full email pipeline from delivery to retrieval
- Run with: `go test ./test/e2e/...`

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