# Testing Guide

This guide covers how to run and write tests for the Raven IMAP server project.

## Table of Contents

- [Test Organization](#test-organization)
- [Running Tests](#running-tests)
- [Test Coverage](#test-coverage)
- [CI/CD Integration](#cicd-integration)
- [Writing Tests](#writing-tests)
- [Best Practices](#best-practices)

## Test Organization

Tests are co-located with source code throughout the `internal/` directory using Go's standard `_test.go` naming convention.

### Test Structure

```
internal/
├── conf/                    # Configuration tests (100% coverage)
│   └── config_test.go
├── db/                      # Database layer tests (79.6% coverage)
│   ├── db_manager_test.go
│   ├── sqlite_test.go
│   └── user_schema_test.go
├── delivery/
│   ├── config/             # Delivery config tests (80.6% coverage)
│   ├── lmtp/               # LMTP server tests (85.7% coverage)
│   ├── parser/             # Email parser tests (77.6% coverage)
│   └── storage/            # Storage tests (77.5% coverage)
├── models/                 # State models tests
│   └── state_test.go
├── sasl/                   # SASL authentication tests (87.1% coverage)
│   └── server_test.go
└── server/
    ├── auth/              # Auth handler tests (83.8% coverage)
    ├── extension/         # IDLE, NAMESPACE tests (86.6% coverage)
    ├── mailbox/           # Mailbox handler tests (70.4% coverage)
    ├── message/           # Message operations tests (71.8% coverage)
    ├── middleware/        # Middleware tests (100% coverage)
    ├── response/          # Response formatting tests (77% coverage)
    ├── selection/         # Mailbox selection tests (83.6% coverage)
    ├── uid/               # UID command tests (86.4% coverage)
    └── utils/             # Server utilities tests (96.4% coverage)
```

## Running Tests

### Quick Start

```bash
# Run all tests
make test

# Run all tests with race detection
make test-race

# Run all tests with coverage
make test-coverage

# Run all tests verbosely
make test-verbose
```

### Core Foundation Tests

```bash
# Database layer tests
make test-db                # All database tests
make test-db-init          # Database initialization
make test-db-domain        # Domain management
make test-db-user          # User management
make test-db-mailbox       # Mailbox operations
make test-db-message       # Message management
make test-db-blob          # Blob storage
make test-db-role          # Role mailboxes
make test-db-subscription  # Subscription management
make test-db-manager       # DB manager tests

# Configuration tests
make test-conf

# Models tests
make test-models

# Server utilities tests
make test-utils

# Server response tests
make test-response

# Server middleware tests
make test-middleware
```

### IMAP Server Component Tests

```bash
# Authentication & session
make test-capability       # CAPABILITY command
make test-login           # LOGIN command
make test-authenticate    # AUTHENTICATE command
make test-starttls        # STARTTLS command
make test-logout          # LOGOUT command

# Mailbox management
make test-create          # CREATE command
make test-delete          # DELETE command
make test-rename          # RENAME command
make test-list            # LIST command
make test-list-extended   # LIST extended tests (wildcards, hierarchy, etc.)
make test-subscribe       # SUBSCRIBE command
make test-unsubscribe     # UNSUBSCRIBE command
make test-lsub            # LSUB command
make test-status          # STATUS command

# Mailbox selection
make test-selection       # SELECT, EXAMINE, CLOSE, UNSELECT
make test-select          # SELECT command
make test-examine         # EXAMINE command

# Message operations
make test-fetch           # FETCH command
make test-store           # STORE command
make test-search          # SEARCH command
make test-copy            # COPY command
make test-append          # APPEND command
make test-expunge         # EXPUNGE command

# UID commands
make test-uid             # All UID commands (UID FETCH, UID SEARCH, etc.)

# Mailbox state
make test-noop            # NOOP command
make test-check           # CHECK command
make test-close           # CLOSE command
make test-idle            # IDLE extension
make test-namespace       # NAMESPACE extension

# Core server tests
make test-core-server     # Core server handler tests
```

### Delivery Service & SASL Tests

```bash
# SASL authentication service
make test-sasl

# Delivery service
make test-delivery        # All delivery tests
make test-parser          # Email parser tests
make test-parser-coverage # Email parser with coverage
make test-storage         # Delivery storage tests
```

### Advanced Testing

```bash
# Run specific test
make test-single TEST=TestCapabilityCommand_NonTLSConnection

# Run all command tests
make test-commands

# Run benchmarks
make bench

# Run AUTHENTICATE benchmarks
make bench-authenticate
```

## Test Coverage

Current overall coverage: **~75%+**

### Well-Covered Packages (>75%)

| Package | Coverage |
|---------|----------|
| `internal/conf` | 100% |
| `internal/server/middleware` | 100% |
| `internal/server/utils` | 96.4% |
| `internal/sasl` | 87.1% |
| `internal/server/extension` | 86.6% |
| `internal/server/uid` | 86.4% |
| `internal/delivery/lmtp` | 85.7% |
| `internal/server/auth` | 83.8% |
| `internal/server/selection` | 83.6% |
| `internal/delivery/config` | 80.6% |
| `internal/db` | 79.6% |
| `internal/delivery/parser` | 77.6% |
| `internal/delivery/storage` | 77.5% |
| `internal/server/response` | 77% |

### Packages with Good Coverage (>70%)

| Package | Coverage |
|---------|----------|
| `internal/server/message` | 71.8% |
| `internal/server/mailbox` | 70.4% |

### Areas for Improvement

- `cmd/*` packages - 0% coverage (main packages, low priority)
- `internal/server` - 10.1% coverage (core server coordination)

## CI/CD Integration

Tests run automatically on pull requests and pushes to main via GitHub Actions in `.github/workflows/ci.yml`.

### CI Pipeline Structure

The CI pipeline consists of **14 parallel jobs** organized into logical groups:

#### 1. Database Layer Tests (`test-database`)
- Database initialization
- Domain, user, and mailbox management
- Message and blob storage
- Role mailboxes and subscriptions
- Delivery queue management

#### 2. IMAP Authentication & Session (`test-imap-auth`)
- CAPABILITY, LOGIN, AUTHENTICATE
- STARTTLS, LOGOUT
- Auth coverage must be ≥80%

#### 3. IMAP Mailbox Management (`test-imap-mailbox`)
- SELECT, EXAMINE, CREATE, DELETE
- RENAME, LIST (basic and extended)
- STATUS command

#### 4. IMAP Message Operations (`test-imap-messages`)
- FETCH, STORE, SEARCH
- COPY, APPEND, UID commands

#### 5. IMAP Mailbox State (`test-imap-state`)
- NOOP, CHECK, CLOSE, EXPUNGE
- IDLE, NAMESPACE extensions
- SUBSCRIBE, UNSUBSCRIBE, LSUB

#### 6. Delivery Service Tests (`test-delivery`)
- Delivery configuration
- Email parser (basic, MIME, database storage, utilities)
- All delivery service integration tests

#### 7. SASL Authentication Service (`test-sasl`)
- Server creation and lifecycle
- Protocol handshake, CPID
- PLAIN authentication
- Authentication mechanisms (LOGIN, etc.)
- Error handling and concurrent connections

#### 8. Configuration Tests (`test-conf`)
- Configuration loading and validation

#### 9. Server Utilities Tests (`test-utils`)
- Server utility functions

#### 10. Server Response Tests (`test-response`)
- Response formatting and BODYSTRUCTURE

#### 11. Models Tests (`test-models`)
- State models and structures

#### 12. Middleware Tests (`test-middleware`)
- Server middleware components

#### 13. Core Server Tests (`test-core-server`)
- Core server handler tests

#### 14. Selection & UID Command Tests (`test-selection-uid`)
- Mailbox selection operations
- UID command implementations

#### 15. Race Detection Tests (`test-race`)
- Runs all tests with `-race` flag to detect data races
- Ensures thread safety across all packages

#### 16. Test Coverage Report (`test-coverage`)
- Generates comprehensive coverage report
- Checks critical package coverage thresholds (≥75%)
- Uploads coverage artifacts for 30 days
- **Fails if critical packages drop below 75% coverage**

### CI Requirements

All tests must pass before PRs can be merged. Critical packages (conf, utils, middleware) must maintain ≥75% test coverage.

## Writing Tests

### Test File Naming

Follow Go conventions:
- Test files: `*_test.go`
- Test functions: `func TestFunctionName(t *testing.T)`
- Benchmark functions: `func BenchmarkFunctionName(b *testing.B)`

### Example Test Structure

```go
package mypackage

import (
    "testing"
)

func TestMyFunction(t *testing.T) {
    // Arrange
    input := "test input"
    expected := "expected output"

    // Act
    result := MyFunction(input)

    // Assert
    if result != expected {
        t.Errorf("expected %v, got %v", expected, result)
    }
}
```

### Table-Driven Tests

```go
func TestMyFunction(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
    }{
        {"empty string", "", ""},
        {"single character", "a", "A"},
        {"multiple characters", "hello", "HELLO"},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := MyFunction(tt.input)
            if result != tt.expected {
                t.Errorf("expected %v, got %v", tt.expected, result)
            }
        })
    }
}
```

## Resources

- [Go Testing Package](https://pkg.go.dev/testing)
- [Table-Driven Tests in Go](https://dave.cheney.net/2019/05/07/prefer-table-driven-tests)
- [IMAP4rev1 RFC 3501](https://tools.ietf.org/html/rfc3501)
- [Go Test Best Practices](https://golang.org/doc/tutorial/add-a-test)
