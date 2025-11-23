# Testing Guide

This guide covers how to run and write tests for the Raven mail server project.

## Test Organization

Tests are co-located with source code throughout the `internal/` directory using Go's standard `_test.go` naming convention.

### Test Structure

```
internal/
├── conf/                    # Configuration tests (100% coverage)
│   └── *_test.go
├── db/                      # Database layer tests (79.6% coverage)
│   └── *_test.go
├── delivery/
│   ├── config/             # Delivery config tests (80.6% coverage)
│   ├── lmtp/               # LMTP server tests (85.7% coverage)
│   ├── parser/             # Email parser tests (18.2% coverage)
│   └── storage/            # Storage tests (77.5% coverage)
├── sasl/                    # SASL authentication tests (87.1% coverage)
│   └── server_test.go
└── server/
    ├── auth/               # Auth handler tests (38.6% coverage)
    ├── mailbox/            # Mailbox handler tests (43.5% coverage)
    ├── response/           # Response formatting tests (77% coverage)
    └── utils/              # Server utilities tests (96.4% coverage)
```

## Running Tests

### Run All Tests

```bash
make test
```

### Run Tests by Package

```bash
# Configuration
make test-conf

# Database
make test-db

# Delivery Service
make test-delivery

# SASL Authentication
make test-sasl

# Server Utilities
make test-utils

# Server Response
make test-response

# Delivery Storage
make test-storage
```

### Run Specific IMAP Command Tests

```bash
make test-capability
make test-login
make test-authenticate
make test-select
make test-create
make test-delete
make test-rename
make test-list
make test-fetch
make test-store
make test-search
make test-append
make test-noop
make test-logout
```

### Run Database Tests

```bash
# All database tests
make test-db

# Specific database test categories
make test-db-init
make test-db-domain
make test-db-user
make test-db-mailbox
make test-db-message
make test-db-blob
make test-db-role
make test-db-subscription
make test-db-manager
```

### Run Tests with Options

```bash
# Verbose output
make test-verbose

# Race detection
make test-race

# Run specific test
make test-single TEST=TestCapabilityCommand
```

## CI/CD Integration

Tests run automatically on pull requests via GitHub Actions in `.github/workflows/ci.yml`.

### Test Jobs

- `test-database` - Database layer tests
- `test-imap-auth` - IMAP authentication tests
- `test-imap-mailbox` - IMAP mailbox management tests
- `test-imap-messages` - IMAP message operations tests
- `test-imap-state` - IMAP mailbox state tests
- `test-delivery` - Delivery service tests
- `test-sasl` - SASL authentication tests
- `test-conf` - Configuration tests
- `test-utils` - Server utilities tests
- `test-response` - Server response tests

All tests must pass before PRs can be merged.

## Writing Tests

### Test File Naming

Follow Go conventions:
- Test files: `*_test.go`
- Test functions: `func TestFunctionName(t *testing.T)`
- Benchmark functions: `func BenchmarkFunctionName(b *testing.B)`

### Example Test Structure

```go
package mypackage

import "testing"

func TestMyFunction(t *testing.T) {
    // Arrange
    input := "test input"

    // Act
    result := MyFunction(input)

    // Assert
    if result != expected {
        t.Errorf("expected %v, got %v", expected, result)
    }
}
```

### Test Categories

#### Unit Tests
- Test individual functions and methods
- Use table-driven tests for multiple cases
- Mock external dependencies

#### Integration Tests
- Test component interactions
- Use in-memory SQLite databases
- Test full command flows

#### RFC Compliance Tests
- Verify IMAP4rev1 (RFC 3501) compliance
- Test protocol format validation
- Verify command behaviors

## Test Coverage

Current overall coverage: **38.5%**

### Well-Covered Packages (>75%)
- `internal/conf` - 100%
- `internal/server/utils` - 96.4%
- `internal/sasl` - 87.1%
- `internal/delivery/lmtp` - 85.7%
- `internal/delivery/config` - 80.6%
- `internal/db` - 79.6%
- `internal/delivery/storage` - 77.5%
- `internal/server/response` - 77%

### Areas Needing Tests
- `cmd/*` packages - 0% coverage
- `internal/server` - 10.1% coverage
- `internal/server/extension` - 0% coverage
- `internal/server/message` - 0% coverage
- `internal/server/middleware` - 0% coverage
- `internal/server/selection` - 0% coverage
- `internal/server/uid` - 0% coverage
- `internal/delivery/parser` - 18.2% coverage

## Best Practices

1. **Co-locate tests** with source code in the same package
2. **Use table-driven tests** for testing multiple scenarios
3. **Test edge cases** and error conditions
4. **Keep tests isolated** - each test should be independent
5. **Use meaningful test names** that describe what is being tested
6. **Clean up resources** in tests (use `t.Cleanup()`)
7. **Test public APIs** primarily, internal implementation secondarily
8. **Avoid test interdependencies** - tests should run in any order
