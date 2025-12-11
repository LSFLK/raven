package helpers

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"raven/internal/db"
)

// TestData contains test fixtures for integration tests
type TestData struct {
	DomainID  int64
	UserID    int64
	MailboxID int64
	MessageID int64
}

// TestDBManager wraps a DBManager with its base path for cleanup
type TestDBManager struct {
	*db.DBManager
	BasePath string
}

// SetupTestDatabase creates a temporary database manager for testing
// Returns a fully initialized DBManager with test data directory
func SetupTestDatabase(t *testing.T) *TestDBManager {
	t.Helper()

	// Create temporary directory for test databases
	tmpDir, err := os.MkdirTemp("", "raven_integration_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	// Initialize database manager
	dbManager, err := db.NewDBManager(tmpDir)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("Failed to initialize database manager: %v", err)
	}

	t.Logf("Test database created at: %s", tmpDir)
	return &TestDBManager{
		DBManager: dbManager,
		BasePath:  tmpDir,
	}
}

// TeardownTestDatabase cleans up the test database and removes temporary files
func TeardownTestDatabase(t *testing.T, dbManager *TestDBManager) {
	t.Helper()

	if dbManager == nil {
		return
	}

	// Close all database connections
	if err := dbManager.Close(); err != nil {
		t.Logf("Warning: Error closing database manager: %v", err)
	}

	// Remove temporary directory
	if dbManager.BasePath != "" {
		if err := os.RemoveAll(dbManager.BasePath); err != nil {
			t.Logf("Warning: Failed to remove temp directory %s: %v", dbManager.BasePath, err)
		} else {
			t.Logf("Test database cleaned up: %s", dbManager.BasePath)
		}
	}
}

// SeedTestData populates the database with test fixtures
// Creates a test domain, user, mailbox, and optionally a test message
func SeedTestData(t *testing.T, dbManager *db.DBManager) TestData {
	t.Helper()

	sharedDB := dbManager.GetSharedDB()

	// Create test domain
	domainID, err := db.CreateDomain(sharedDB, "example.com")
	if err != nil {
		t.Fatalf("Failed to create test domain: %v", err)
	}

	// Create test user
	userID, err := db.CreateUser(sharedDB, "testuser", domainID)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Get user database
	userDB, err := dbManager.GetUserDB(userID)
	if err != nil {
		t.Fatalf("Failed to get user database: %v", err)
	}

	// Get INBOX mailbox ID (created by default)
	mailboxID, err := db.GetMailboxByNamePerUser(userDB, userID, "INBOX")
	if err != nil {
		t.Fatalf("Failed to get INBOX mailbox: %v", err)
	}

	t.Logf("Test data seeded: domain=%d, user=%d, mailbox=%d", domainID, userID, mailboxID)

	return TestData{
		DomainID:  domainID,
		UserID:    userID,
		MailboxID: mailboxID,
		MessageID: 0, // No message created by default
	}
}

// CreateTestUser creates a test user with the given email address
func CreateTestUser(t *testing.T, dbManager *db.DBManager, email string) TestData {
	t.Helper()

	// Parse email to extract username and domain
	username, domain := parseEmail(email)
	if username == "" || domain == "" {
		t.Fatalf("Invalid email format: %s", email)
	}

	sharedDB := dbManager.GetSharedDB()

	// Create or get domain
	domainID, err := db.GetOrCreateDomain(sharedDB, domain)
	if err != nil {
		t.Fatalf("Failed to create/get domain %s: %v", domain, err)
	}

	// Create or get user
	userID, err := db.GetOrCreateUser(sharedDB, username, domainID)
	if err != nil {
		t.Fatalf("Failed to create/get user %s: %v", username, err)
	}

	// Get user database
	userDB, err := dbManager.GetUserDB(userID)
	if err != nil {
		t.Fatalf("Failed to get user database: %v", err)
	}

	// Get INBOX mailbox ID
	mailboxID, err := db.GetMailboxByNamePerUser(userDB, userID, "INBOX")
	if err != nil {
		t.Fatalf("Failed to get INBOX mailbox: %v", err)
	}

	t.Logf("Test user created: %s (domain=%d, user=%d)", email, domainID, userID)

	return TestData{
		DomainID:  domainID,
		UserID:    userID,
		MailboxID: mailboxID,
		MessageID: 0,
	}
}

// GetUserMessages retrieves all message IDs for a user's INBOX
func GetUserMessages(t *testing.T, dbManager *db.DBManager, userID int64) []int64 {
	t.Helper()

	userDB, err := dbManager.GetUserDB(userID)
	if err != nil {
		t.Fatalf("Failed to get user database: %v", err)
	}

	mailboxID, err := db.GetMailboxByNamePerUser(userDB, userID, "INBOX")
	if err != nil {
		t.Fatalf("Failed to get INBOX mailbox: %v", err)
	}

	messages, err := db.GetMessagesByMailboxPerUser(userDB, mailboxID)
	if err != nil {
		t.Fatalf("Failed to get messages: %v", err)
	}

	return messages
}

// CreateTestMailbox creates a mailbox for a user
func CreateTestMailbox(t *testing.T, dbManager *db.DBManager, userID int64, mailboxName string) int64 {
	t.Helper()

	userDB, err := dbManager.GetUserDB(userID)
	if err != nil {
		t.Fatalf("Failed to get user database: %v", err)
	}

	mailboxID, err := db.CreateMailboxPerUser(userDB, userID, mailboxName, "")
	if err != nil {
		t.Fatalf("Failed to create mailbox %s: %v", mailboxName, err)
	}

	t.Logf("Test mailbox created: %s (id=%d)", mailboxName, mailboxID)
	return mailboxID
}

// AssertDatabaseExists verifies that a database file exists at the expected path
func AssertDatabaseExists(t *testing.T, basePath string, dbType string, id int64) {
	t.Helper()

	var dbPath string
	switch dbType {
	case "shared":
		dbPath = filepath.Join(basePath, "shared.db")
	case "user":
		dbPath = filepath.Join(basePath, fmt.Sprintf("user_db_%d.db", id))
	case "role":
		dbPath = filepath.Join(basePath, fmt.Sprintf("role_db_%d.db", id))
	default:
		t.Fatalf("Unknown database type: %s", dbType)
	}

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Errorf("Expected database file to exist: %s", dbPath)
	}
}

// parseEmail splits an email address into username and domain
func parseEmail(email string) (username, domain string) {
	for i := 0; i < len(email); i++ {
		if email[i] == '@' {
			return email[:i], email[i+1:]
		}
	}
	return "", ""
}

// CreateTestMessage creates a simple message in the given user's database and returns its ID.
func CreateTestMessage(t *testing.T, userDB *sql.DB, raw string) int64 {
	t.Helper()
	msgID, err := db.CreateMessage(userDB, raw, "", "", time.Now(), int64(len(raw)))
	if err != nil {
		t.Fatalf("Failed to create test message: %v", err)
	}
	return msgID
}

// LinkMessageToMailbox links an existing message to a mailbox, assigning next UID and flags.
func LinkMessageToMailbox(t *testing.T, userDB *sql.DB, messageID, mailboxID int64) {
	t.Helper()
	if err := db.AddMessageToMailboxPerUser(userDB, messageID, mailboxID, "", time.Now()); err != nil {
		t.Fatalf("Failed to link message %d to mailbox %d: %v", messageID, mailboxID, err)
	}
}
