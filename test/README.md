# Test Directory Structure

This directory contains all tests for the Go IMAP Server project, organized separately from the source code for better maintainability and clarity.

## Directory Structure

```
test/
├── README.md                    # This file
├── helpers/                     # Test helper utilities and mock objects
│   └── test_helpers.go         # Common test utilities, mock connections, test server setup
└── server/                      # Tests for server functionality
    ├── capability_test.go      # Comprehensive CAPABILITY command tests
    └── handlers_test.go        # Integration tests for various IMAP handlers
```

## Test Organization

### helpers/
Contains reusable test utilities:
- **MockConn**: Mock network connection for testing
- **MockTLSConn**: Mock TLS connection for testing TLS-specific behavior
- **SetupTestServer**: Creates test IMAP server instances with in-memory databases
- **TestInterface**: Provides access to internal server methods for testing

### server/
Contains all server-related tests:
- **capability_test.go**: RFC 3501 compliance tests, edge cases, performance tests
- **handlers_test.go**: Integration tests for IMAP command handlers

## Running Tests

Use the provided Makefile targets:

```bash
# Run all tests
make test

# Run only capability tests
make test-capability

# Run tests with verbose output
make test-verbose

# Run tests with coverage report
make test-coverage

# Run tests with race detection
make test-race

# Run benchmarks
make bench

# Run a specific test
make test-single TEST=TestCapabilityCommand_NonTLSConnection
```

## Test Categories

### Unit Tests
- Individual command handler testing
- Response format validation
- Edge case handling

### Integration Tests
- Full command flow testing
- State management validation
- Connection type behavior

### Performance Tests
- Memory usage validation
- Concurrent access testing
- Benchmark comparisons

### Compliance Tests
- RFC 3501 IMAP4rev1 compliance
- Capability advertisement correctness
- Protocol format validation

## Writing New Tests

When adding new tests:

1. Use the helper utilities in `test/helpers/` for consistency
2. Follow the naming convention: `Test{Component}_{TestName}`
3. Add benchmarks for performance-critical code: `Benchmark{Component}`
4. Include both positive and negative test cases
5. Test edge cases and error conditions
6. Ensure RFC compliance where applicable

### Example Test Structure

```go
func TestNewCommand_BasicFunctionality(t *testing.T) {
    server := helpers.SetupTestServer(t)
    conn := helpers.NewMockConn()
    state := &models.ClientState{Authenticated: false}

    server.HandleNewCommand(conn, "TAG", state)

    response := conn.GetWrittenData()
    // Validate response...
}
```

## Mock Objects

### MockConn
Simulates a network connection with:
- Buffered read/write operations
- Connection state tracking
- Data inspection methods

### MockTLSConn
Extends MockConn to simulate TLS connections for testing TLS-specific behavior such as:
- Capability advertisement changes
- Authentication method availability

## Test Data

Tests use in-memory SQLite databases created fresh for each test to ensure isolation and reproducibility.

## Coverage

The test suite aims for high coverage of:
- All IMAP command handlers
- Error conditions and edge cases  
- RFC compliance scenarios
- Performance characteristics

Generate coverage reports with:
```bash
make test-coverage
open coverage.html
```
