# Integration Test Infrastructure

This directory contains the complete integration test infrastructure for the Raven email server project.

## Directory Structure

```
test/integration/
├── README.md                    # This documentation
├── docker-compose.yml           # Docker test environment
├── verify-infrastructure.sh     # Infrastructure verification script
├── fixtures/                    # Test data and configurations
│   ├── *.eml                   # Email message fixtures
│   ├── test-users.json         # Test user accounts
│   ├── test-config.yaml        # Server configuration
│   └── mailbox-structures.json # Mailbox test scenarios
├── helpers/                     # Test utilities and helpers
│   ├── fixtures.go             # Fixture loading utilities
│   ├── database.go             # Database test helpers
│   ├── server.go               # IMAP/LMTP test helpers
│   └── docker.go               # Docker environment management
├── db/                         # Database integration tests
├── server/                     # IMAP server integration tests
├── delivery/                   # LMTP delivery integration tests
└── e2e/                        # End-to-end system tests
```

## Quick Start

### 1. Verify Infrastructure

Run the verification script to ensure everything is set up correctly:

```bash
cd test/integration
./verify-infrastructure.sh
```

This script checks:
- Directory structure
- Required fixture files
- Helper code compilation
- Docker environment configuration

### 2. Run Tests

#### Local Integration Tests (No Docker)
```bash
# Run all integration tests
go test ./test/integration/... -v

# Run specific test package
go test ./test/integration/helpers -v

# Run with build tags if needed
go test -tags=integration ./test/integration/... -v
```

#### Docker-based Tests
```bash
# Start test environment
cd test/integration
docker-compose up -d

# Run tests against Docker services
go test ./test/integration/e2e -v

# Stop test environment
docker-compose down -v
```

### 3. Environment Variables

Set these environment variables for specific test configurations:

```bash
export TEST_DB_DSN="sqlite:///tmp/raven-test.db"
export TEST_IMAP_ADDR="127.0.0.1:10143"
export TEST_LMTP_ADDR="127.0.0.1:10024"
export TEST_TIMEOUT="30s"
```

## Test Fixtures

### Email Fixtures

| Fixture | Purpose | Size |
|---------|---------|------|
| `simple-email.eml` | Basic email testing | ~200B |
| `multipart-email.eml` | Multipart message handling | ~500B |
| `email-with-attachment.eml` | Attachment processing | ~1KB |
| `html-email.eml` | HTML/alternative content | ~1.5KB |
| `multi-recipient-email.eml` | Multiple recipients | ~600B |
| `unicode-email.eml` | Unicode/emoji support | ~1KB |
| `large-email.eml` | Performance testing | ~3KB |

### Configuration Fixtures

- **`test-users.json`**: User accounts for multiple domains with mailbox structures
- **`test-config.yaml`**: Server configuration for testing
- **`mailbox-structures.json`**: Complex mailbox hierarchies for testing

## Helper Functions

### Fixture Loading
```go
// Load email fixtures
data := helpers.LoadSimpleEmail(t)
data := helpers.LoadMultipartEmail(t)
data := helpers.LoadUnicodeEmail(t)

// Load configuration
users := helpers.LoadTestUsers(t)
config := helpers.LoadTestConfig(t)
mailboxes := helpers.LoadMailboxStructures(t)
```

### Server Testing
```go
// Start test IMAP server
server := helpers.StartTestIMAPServer(t, dbManager)
defer server.Stop(t)

// Connect IMAP client
client := helpers.ConnectIMAP(t, server.Address)
defer client.Close()

// Send IMAP commands
responses, err := client.SendCommand("LIST \"\" \"*\"")
```

### Database Testing
```go
// Setup test database
db := helpers.SetupTestDatabase(t)
defer helpers.TeardownTestDatabase(t, db)

// Seed test data
testData := helpers.SeedTestData(t, db)
```

### Docker Environment
```go
// Start Docker test environment
dockerEnv := helpers.NewDockerTestEnvironment(t)
dockerEnv.StartFullEnvironment(t)
defer dockerEnv.Stop(t)

// Get service URLs
imapAddr := dockerEnv.GetServiceURL("raven-full", 143)
```

## Docker Services

The Docker Compose file provides several service configurations:

### Individual Services
- **`raven-imap`**: IMAP server only (port 10143)
- **`raven-lmtp`**: LMTP server only (port 10024)

### Combined Service  
- **`raven-full`**: Complete server with both IMAP and LMTP (ports 10143, 10024)

### Utility Services
- **`test-db`**: Shared database for testing (profile: database)
- **`mail-client`**: Alpine container for manual testing (profile: tools)

### Usage Examples

```bash
# Start individual services
docker-compose up raven-imap raven-lmtp

# Start full service
docker-compose up raven-full

# Start with database
docker-compose --profile database up raven-full test-db

# Start with tools
docker-compose --profile tools up
```

## Writing Integration Tests

### Test Structure
```go
package mypackage

import (
    "testing"
    "raven/test/integration/helpers"
)

func TestSomething(t *testing.T) {
    // Setup test database
    db := helpers.SetupTestDatabase(t)
    defer helpers.TeardownTestDatabase(t, db)
    
    // Load test data
    email := helpers.LoadSimpleEmail(t)
    
    // Your test logic here
}
```

### Test Categories

1. **Unit Integration Tests**: Test individual components with real dependencies
2. **Service Integration Tests**: Test IMAP/LMTP servers with clients
3. **Database Integration Tests**: Test data persistence and queries
4. **End-to-End Tests**: Test complete workflows through Docker

### Best Practices

- Use `testing.Short()` to skip long-running tests: `if testing.Short() { t.Skip() }`
- Always clean up resources with defer statements
- Use meaningful test names and subtests with `t.Run()`
- Load appropriate fixtures for each test case
- Check for Docker availability before container tests

## Implemented Integration Tests

### DB Manager Integration Tests
**File:** `test/integration/db/database_integration_test.go`

- ✅ `TestDBManager_RealFileSystem(t *testing.T)` - Tests database manager with real file system
- ✅ `TestDBManager_MultipleUserDBs_Concurrent(t *testing.T)` - Tests concurrent access to multiple user databases
- ✅ `TestDBManager_DatabaseRecovery(t *testing.T)` - Tests database recovery from corruption or errors
- ✅ `TestDBManager_TransactionRollback(t *testing.T)` - Tests transaction rollback functionality

### Data Integrity Tests
**File:** `test/integration/db/data_integrity_test.go`

- ✅ `TestDataIntegrity_ForeignKeyConstraints(t *testing.T)` - Tests foreign key constraint enforcement
  - User domain foreign key
  - Role mailbox domain foreign key
  - User role assignment foreign keys
  - Mailbox parent foreign key
- ✅ `TestDataIntegrity_CascadeDeletes(t *testing.T)` - Tests cascade delete behavior
  - Domain deletion prevention when users exist
  - User deletion cascades assignments
  - Message deletion cascades related data
- ✅ `TestDataIntegrity_UniqueConstraints(t *testing.T)` - Tests unique constraint enforcement
  - Domain unique constraint
  - User unique username per domain
  - Role mailbox unique email
  - Mailbox unique name per user
  - Message mailbox unique UID per mailbox

### Domain and User Management
**File:** `test/integration/db/user_management_integration_test.go`

- ✅ `TestUserLifecycle_CreateLoginDelete(t *testing.T)` - Tests complete user lifecycle
  - User creation with domain
  - User database and default mailboxes creation
  - User lookup by email (simulated login)
  - User disable/enable
  - User deletion
- ✅ `TestDomainManagement_MultiDomain(t *testing.T)` - Tests multi-domain operations
  - Multiple domain creation
  - Users in different domains
  - Multiple users per domain
  - Domain lookup by name
  - GetOrCreateDomain functionality
- ✅ `TestRoleMailbox_AssignmentWorkflow(t *testing.T)` - Tests role mailbox assignment workflow
  - Role mailbox creation
  - User-to-role assignments
  - Multiple role assignments per user
  - GetOrCreateRoleMailbox functionality
  - Assignment activation/deactivation
  - Role mailbox lookup by email and ID

### Mailbox Operations
**File:** `test/integration/db/mailbox_operations_test.go`

- ✅ `TestMailbox_HierarchyOperations(t *testing.T)` - Tests mailbox hierarchy operations
  - Flat mailbox structure creation
  - Hierarchical mailbox structure with parent/child relationships
  - Deep hierarchy (4 levels deep)
  - Parent mailbox deletion behavior
- ✅ `TestMailbox_SubscriptionManagement(t *testing.T)` - Tests mailbox subscription operations
  - Subscribe to custom mailboxes
  - Unsubscribe from mailboxes
  - Subscribe to default mailboxes
  - Duplicate subscription handling
  - Unsubscribe from non-subscribed mailboxes
- ✅ `TestMailbox_UIDValidity(t *testing.T)` - Tests UID validity and UID next operations
  - Initial UID validity and UID next values
  - UID validity stability over time
  - UID next increment when messages are added
  - UID uniqueness within mailboxes
  - UID validity change scenarios (mailbox reconstruction)

### Running Integration Tests

```bash
# Run all integration tests
go test ./test/integration/db/... -v

# Run specific test suites
go test ./test/integration/db -run TestDBManager -v
go test ./test/integration/db -run TestDataIntegrity -v
go test ./test/integration/db -run TestUserLifecycle -v
go test ./test/integration/db -run TestMailbox -v

# Run with race detection
go test -race ./test/integration/db/... -v

# Run with coverage
go test -cover ./test/integration/db/... -v
```

## Troubleshooting

### Common Issues

1. **Docker not available**: Tests will skip automatically
2. **Port conflicts**: Change Docker port mappings in `docker-compose.yml`
3. **Build failures**: Check Go module dependencies with `go mod tidy`
4. **Test timeouts**: Increase timeout values in test configuration

### Debug Commands

```bash
# Check Docker services
docker-compose ps
docker-compose logs raven-imap

# Test network connectivity
telnet 127.0.0.1 10143

# Validate fixtures
go run ./test/integration/helpers/validate_fixtures.go

# Check test compilation
go test -c ./test/integration/...
```

### Environment Requirements

- **Go 1.19+**: For module support and generics
- **Docker**: For containerized testing (optional)
- **Docker Compose**: For service orchestration (optional)
- **SQLite**: For database testing

## Contributing

When adding new integration tests:

1. Add fixtures to the appropriate `fixtures/` subdirectory
2. Create helper functions in the appropriate `helpers/` file
3. Write tests in the appropriate test package directory
4. Update this README with new functionality
5. Verify infrastructure with `./verify-infrastructure.sh`

## Architecture

The integration test infrastructure is designed to:

- **Isolate tests**: Each test gets clean environment
- **Support multiple scenarios**: Different user/mailbox configurations  
- **Enable performance testing**: Large fixtures and stress scenarios
- **Provide realistic testing**: Real email formats and complex structures
- **Support Docker deployment**: Container-based testing for CI/CD
- **Maintain simplicity**: Easy-to-use helper functions and clear patterns
