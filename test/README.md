# Test Directory Structure

This directory contains all tests for the Go IMAP Server project, organized separately from the source code for better maintainability and clarity.

## Directory Structure

```
test/
├── README.md                    # This file
├── helpers/                     # Test helper utilities and mock objects
│   └── test_helpers.go         # Common test utilities, mock connections, test server setup
└── server/                      # Tests for server functionality
    ├── capability_test.go      # CAPABILITY command tests
    ├── authenticate_test.go    # AUTHENTICATE command tests
    ├── login_test.go           # LOGIN command tests
    ├── starttls_test.go        # STARTTLS command tests
    ├── noop_test.go            # NOOP command tests
    ├── logout_test.go          # LOGOUT command tests
    ├── append_test.go          # APPEND command tests
    ├── select_test.go          # SELECT/EXAMINE command tests
    ├── create_test.go          # CREATE command tests
    ├── delete_test.go          # DELETE command tests
    ├── rename_test.go          # RENAME command tests
    ├── subscribe_test.go       # SUBSCRIBE/UNSUBSCRIBE command tests
    ├── lsub_test.go            # LSUB command tests (NEW)
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
Contains all server-related tests organized by IMAP command:

#### Connection & Authentication Tests
- **capability_test.go**: CAPABILITY command - RFC 3501 compliance, TLS detection
- **authenticate_test.go**: AUTHENTICATE command - SASL PLAIN mechanism with base64
- **login_test.go**: LOGIN command - Basic authentication with TLS requirement
- **starttls_test.go**: STARTTLS command - TLS upgrade functionality
- **logout_test.go**: LOGOUT command - Session termination

#### Mailbox Management Tests
- **create_test.go**: CREATE command - Mailbox creation with hierarchy support
- **delete_test.go**: DELETE command - Mailbox deletion with RFC 3501 rules
- **rename_test.go**: RENAME command - Mailbox renaming with hierarchy handling
- **subscribe_test.go**: SUBSCRIBE/UNSUBSCRIBE commands - Subscription management
- **lsub_test.go**: LSUB command - List subscribed mailboxes with wildcard support (13 tests)
- **status_test.go**: STATUS command - Get mailbox status information with all data items (19 tests)

#### Message Operation Tests
- **select_test.go**: SELECT/EXAMINE commands - Mailbox selection for read-write/read-only
- **append_test.go**: APPEND command - Adding messages to mailboxes

#### Server Interaction Tests
- **noop_test.go**: NOOP command - Keepalive and mailbox state updates
- **handlers_test.go**: Integration tests for various IMAP command handlers

## Running Tests

Use the provided Makefile targets:

```bash
# Run all tests
make test

# Run all command tests (comprehensive suite)
make test-commands

# Run individual command tests
make test-capability        # CAPABILITY command
make test-authenticate      # AUTHENTICATE command
make test-login             # LOGIN command
make test-starttls          # STARTTLS command
make test-noop              # NOOP command
make test-logout            # LOGOUT command
make test-append            # APPEND command
make test-select            # SELECT command
make test-examine           # EXAMINE command
make test-create            # CREATE command
make test-list              # LIST command
make test-list-extended     # LIST with wildcards, hierarchies, RFC tests
make test-delete            # DELETE command
make test-rename            # RENAME command
make test-subscribe         # SUBSCRIBE command
make test-unsubscribe       # UNSUBSCRIBE command
make test-lsub              # LSUB command
make test-status            # STATUS command (NEW)

# Run tests with verbose output
make test-verbose

# Run tests with coverage report
make test-coverage

# Run tests with race detection
make test-race

# Run benchmarks
make bench

# Run a specific test
make test-single TEST=TestLsubCommand_ImpliedParentWithNoselect
```

## Test Categories

### Unit Tests
- Individual command handler testing
- Response format validation
- Edge case handling
- Error condition testing

### Integration Tests
- Full command flow testing
- State management validation
- Connection type behavior
- Multi-command sequences

### Performance Tests
- Memory usage validation
- Concurrent access testing
- Benchmark comparisons

### Compliance Tests
- **RFC 3501 IMAP4rev1 compliance**
- Capability advertisement correctness
- Protocol format validation
- Wildcard pattern matching (LIST, LSUB)
- Hierarchy handling (CREATE, DELETE, RENAME, LSUB)
- Subscription semantics (SUBSCRIBE, UNSUBSCRIBE, LSUB)

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

## Test Statistics

### Command Test Coverage
| Command | Test File | Test Count | Status |
|---------|-----------|------------|--------|
| CAPABILITY | capability_test.go | 15+ | ✅ Pass |
| AUTHENTICATE | authenticate_test.go | 12+ | ✅ Pass |
| LOGIN | login_test.go | 10+ | ✅ Pass |
| STARTTLS | starttls_test.go | 8+ | ✅ Pass |
| NOOP | noop_test.go | 6+ | ✅ Pass |
| LOGOUT | logout_test.go | 4+ | ✅ Pass |
| APPEND | append_test.go | 8+ | ✅ Pass |
| SELECT | select_test.go | 10+ | ✅ Pass |
| EXAMINE | select_test.go | 5+ | ✅ Pass |
| CREATE | create_test.go | 12+ | ✅ Pass |
| DELETE | delete_test.go | 10+ | ✅ Pass |
| RENAME | rename_test.go | 15+ | ✅ Pass |
| SUBSCRIBE | subscribe_test.go | 8 | ✅ Pass |
| UNSUBSCRIBE | subscribe_test.go | 8 | ✅ Pass |
| LSUB | lsub_test.go | 13 | ✅ Pass |
| STATUS | status_test.go | **19** | ✅ Pass |
| **Total** | | **163+** | ✅ All Pass |

### Coverage Goals
The test suite aims for high coverage of:
- All IMAP command handlers (15+ commands covered)
- Error conditions and edge cases
- RFC 3501 compliance scenarios
- Wildcard pattern matching
- Hierarchy operations
- Security requirements (TLS, authentication)
- Performance characteristics

### Generate Coverage Reports
```bash
make test-coverage
open coverage.html
```

### CI/CD Integration
All tests run automatically on pull requests via GitHub Actions:
- Go 1.23 environment
- Full test suite execution
- All commands including LSUB and STATUS tested
- Test failures block PR merges
