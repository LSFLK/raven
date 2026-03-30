package db_test

import (
	"fmt"
	"sync"
	"testing"

	"raven/internal/db"
	"raven/test/helpers"
)

// TestDBManagerToSharedDB_SuccessFlow tests database manager with real file system
func TestDBManagerToSharedDB_SuccessFlow(t *testing.T) {
	// Setup
	dbManager := helpers.SetupTestDatabase(t)
	defer helpers.TeardownTestDatabase(t, dbManager)

	// Verify shared database exists
	sharedDB := dbManager.GetSharedDB()
	if sharedDB == nil {
		t.Fatal("Expected non-nil shared database")
	}

	// Verify we can query shared database
	rows, err := sharedDB.Query("SELECT name FROM sqlite_master WHERE type='table'")
	if err != nil {
		t.Fatalf("Failed to query shared database tables: %v", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			t.Errorf("Failed to close rows: %v", closeErr)
		}
	}()

	tables := make(map[string]bool)
	for rows.Next() {
		var name string
		if scanErr := rows.Scan(&name); scanErr != nil {
			t.Fatalf("Failed to scan shared database table name: %v", scanErr)
		}
		tables[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("Error iterating shared database tables: %v", err)
	}

	requiredTables := []string{"blobs"}
	for _, table := range requiredTables {
		if !tables[table] {
			t.Errorf("Expected shared table %q to exist", table)
		}
	}

	// Verify database file exists
	helpers.AssertDatabaseExists(t, dbManager.BasePath, "shared", "")
}

// TestDBManagerToUserDB_SuccessFlow tests user database creation
func TestDBManagerToUserDB_SuccessFlow(t *testing.T) {
	// Setup
	dbManager := helpers.SetupTestDatabase(t)
	defer helpers.TeardownTestDatabase(t, dbManager)

	// Create test data
	testData := helpers.SeedTestData(t, dbManager.DBManager)

	// Verify user database was created
	helpers.AssertDatabaseExists(t, dbManager.BasePath, "user", testData.Email)

	// Verify default mailboxes were created
	userDB, err := dbManager.GetUserDB(testData.Email)
	if err != nil {
		t.Fatalf("Failed to get user database: %v", err)
	}

	// Check for default mailboxes
	mailboxes, err := db.GetUserMailboxesPerUser(userDB)
	if err != nil {
		t.Fatalf("Failed to get user mailboxes: %v", err)
	}

	expectedMailboxes := []string{"Drafts", "INBOX", "Sent", "Spam", "Trash"}
	if len(mailboxes) != len(expectedMailboxes) {
		t.Errorf("Expected %d mailboxes, got %d", len(expectedMailboxes), len(mailboxes))
	}

	// Verify specific mailboxes exist
	for _, expected := range expectedMailboxes {
		found := false
		for _, actual := range mailboxes {
			if actual == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected mailbox '%s' not found", expected)
		}
	}
}

// TestDBManagerToSharedDB_CreateUserFlow tests complete user creation workflow
func TestDBManagerToSharedDB_CreateUserFlow(t *testing.T) {
	// Setup
	dbManager := helpers.SetupTestDatabase(t)
	defer helpers.TeardownTestDatabase(t, dbManager)

	// Create test user
	testData := helpers.CreateTestUser(t, dbManager.DBManager, "alice@example.com")

	if testData.Email == "" {
		t.Error("Expected non-empty email")
	}
	if testData.MailboxID == 0 {
		t.Error("Expected non-zero mailbox ID")
	}
}

// TestDBManagerToSharedDB_MultipleUsersAndDomains tests multiple user creation
func TestDBManagerToSharedDB_MultipleUsersAndDomains(t *testing.T) {
	// Setup
	dbManager := helpers.SetupTestDatabase(t)
	defer helpers.TeardownTestDatabase(t, dbManager)

	// Create multiple test users
	users := []string{
		"alice@example.com",
		"bob@example.com",
		"charlie@different.com",
	}

	var testUsers []helpers.TestData
	for _, email := range users {
		userData := helpers.CreateTestUser(t, dbManager.DBManager, email)
		testUsers = append(testUsers, userData)
	}

	// Verify all users were created
	if len(testUsers) != len(users) {
		t.Fatalf("Expected %d users, got %d", len(users), len(testUsers))
	}

	// Added: verify each user has default mailboxes (cross-module consistency)
	for _, tu := range testUsers {
		userDB, err := dbManager.GetUserDB(tu.Email)
		if err != nil {
			t.Fatalf("Failed to get user DB for user %s: %v", tu.Email, err)
		}
		mboxes, err := db.GetUserMailboxesPerUser(userDB)
		if err != nil {
			t.Fatalf("Failed to get mailboxes for user %s: %v", tu.Email, err)
		}
		// expect default set
		expected := map[string]bool{"Drafts": true, "INBOX": true, "Sent": true, "Spam": true, "Trash": true}
		for _, m := range mboxes {
			delete(expected, m)
		}
		if len(expected) != 0 {
			t.Errorf("User %s missing default mailboxes: %v", tu.Email, expected)
		}
	}
}

// TestDBManagerToUserDB_MailboxCRUD tests mailbox CRUD operations
func TestDBManagerToUserDB_MailboxCRUD(t *testing.T) {
	// Setup
	dbManager := helpers.SetupTestDatabase(t)
	defer helpers.TeardownTestDatabase(t, dbManager)

	testData := helpers.CreateTestUser(t, dbManager.DBManager, "alice@example.com")

	// Create custom mailbox
	customMailboxID := helpers.CreateTestMailbox(t, dbManager.DBManager, testData.Email, "Work")

	if customMailboxID == 0 {
		t.Error("Expected non-zero mailbox ID")
	}

	// Verify mailbox was created
	userDB, err := dbManager.GetUserDB(testData.Email)
	if err != nil {
		t.Fatalf("Failed to get user database: %v", err)
	}

	retrievedID, err := db.GetMailboxByNamePerUser(userDB, "Work")
	if err != nil {
		t.Fatalf("Failed to get mailbox: %v", err)
	}

	if retrievedID != customMailboxID {
		t.Errorf("Expected mailbox ID %d, got %d", customMailboxID, retrievedID)
	}

	// Delete mailbox
	err = db.DeleteMailboxPerUser(userDB, "Work")
	if err != nil {
		t.Fatalf("Failed to delete mailbox: %v", err)
	}

	// Verify mailbox was deleted
	exists, err := db.MailboxExistsPerUser(userDB, "Work")
	if err != nil {
		t.Fatalf("Failed to check mailbox existence: %v", err)
	}

	if exists {
		t.Error("Mailbox should not exist after deletion")
	}
}

// TestDBManagerToUserDB_Concurrency tests concurrent access to multiple user databases
func TestDBManagerToUserDB_Concurrency(t *testing.T) {
	// Setup
	dbManager := helpers.SetupTestDatabase(t)
	defer helpers.TeardownTestDatabase(t, dbManager)

	// Create multiple users
	numUsers := 10
	emails := make([]string, numUsers)

	for i := 0; i < numUsers; i++ {
		email := fmt.Sprintf("user%d@example.com", i)
		_ = helpers.CreateTestUser(t, dbManager.DBManager, email)
		emails[i] = email
	}

	// Concurrently access user databases
	var wg sync.WaitGroup
	errors := make(chan error, numUsers)

	for i := 0; i < numUsers; i++ {
		wg.Add(1)
		go func(email string) {
			defer wg.Done()

			// Get user database
			userDB, err := dbManager.GetUserDB(email)
			if err != nil {
				errors <- fmt.Errorf("failed to get user DB for %s: %v", email, err)
				return
			}

			// Perform database operations
			mailboxID := helpers.CreateTestMailbox(t, dbManager.DBManager, email, "Concurrent")

			// Verify mailbox was created
			retrievedID, err := db.GetMailboxByNamePerUser(userDB, "Concurrent")
			if err != nil {
				errors <- fmt.Errorf("failed to get mailbox for %s: %v", email, err)
				return
			}

			if retrievedID != mailboxID {
				errors <- fmt.Errorf("mailbox ID mismatch for %s: expected %d, got %d", email, mailboxID, retrievedID)
				return
			}
		}(emails[i])
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Error(err)
	}

	// Verify all user databases can still be accessed (test cache)
	for _, email := range emails {
		userDB, err := dbManager.GetUserDB(email)
		if err != nil {
			t.Errorf("Failed to get user DB after concurrent access: %v", err)
		}
		if userDB == nil {
			t.Errorf("Got nil user DB for %s", email)
		}
	}
}

// TestDBManagerToSharedDB_Recovery tests database recovery from corruption or errors
func TestDBManagerToSharedDB_Recovery(t *testing.T) {
	// Setup
	dbManager := helpers.SetupTestDatabase(t)
	defer helpers.TeardownTestDatabase(t, dbManager)

	// Create test user
	testData := helpers.CreateTestUser(t, dbManager.DBManager, "alice@example.com")

	// Get user database to ensure it's created
	userDB, err := dbManager.GetUserDB(testData.Email)
	if err != nil {
		t.Fatalf("Failed to get user database: %v", err)
	}

	// Verify database is functional
	mailboxID, err := db.GetMailboxByNamePerUser(userDB, "INBOX")
	if err != nil {
		t.Fatalf("Failed to get INBOX: %v", err)
	}
	if mailboxID == 0 {
		t.Fatal("Expected non-zero mailbox ID")
	}

	// Close the database manager
	err = dbManager.Close()
	if err != nil {
		t.Fatalf("Failed to close database manager: %v", err)
	}

	// Reopen the database manager (recovery scenario)
	dbManager2, err := db.NewDBManager(dbManager.BasePath)
	if err != nil {
		t.Fatalf("Failed to reopen database manager: %v", err)
	}
	defer func(dbManager2 *db.DBManager) {
		err := dbManager2.Close()
		if err != nil {
			// Ignore close errors in defer cleanup
			_ = err
		}
	}(dbManager2)

	// Verify we can access the same data
	userDB2, err := dbManager2.GetUserDB(testData.Email)
	if err != nil {
		t.Fatalf("Failed to get user database after recovery: %v", err)
	}

	mailboxID2, err := db.GetMailboxByNamePerUser(userDB2, "INBOX")
	if err != nil {
		t.Fatalf("Failed to get INBOX after recovery: %v", err)
	}

	if mailboxID2 != mailboxID {
		t.Errorf("Mailbox ID changed after recovery: expected %d, got %d", mailboxID, mailboxID2)
	}

	// Verify domain still exists in shared database
}

// TestDBManagerToUserDB_RollbackAndCommit tests transaction rollback functionality
func TestDBManagerToUserDB_RollbackAndCommit(t *testing.T) {
	// Setup
	dbManager := helpers.SetupTestDatabase(t)
	defer helpers.TeardownTestDatabase(t, dbManager)

	// Create test user
	testData := helpers.CreateTestUser(t, dbManager.DBManager, "alice@example.com")
	userDB, err := dbManager.GetUserDB(testData.Email)
	if err != nil {
		t.Fatalf("Failed to get user database: %v", err)
	}

	// Get initial mailbox count
	initialMailboxes, err := db.GetUserMailboxesPerUser(userDB)
	if err != nil {
		t.Fatalf("Failed to get initial mailboxes: %v", err)
	}
	initialCount := len(initialMailboxes)

	// Start a transaction
	tx, err := userDB.Begin()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	// Create mailboxes within transaction
	_, err = tx.Exec(`
		INSERT INTO mailboxes (name, uid_validity, uid_next, created_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
	`, "TransactionTest1", 1, 1)
	if err != nil {
		t.Fatalf("Failed to insert first mailbox: %v", err)
	}

	_, err = tx.Exec(`
		INSERT INTO mailboxes (name, uid_validity, uid_next, created_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
	`, "TransactionTest2", 1, 1)
	if err != nil {
		t.Fatalf("Failed to insert second mailbox: %v", err)
	}

	// Rollback the transaction
	err = tx.Rollback()
	if err != nil {
		t.Fatalf("Failed to rollback transaction: %v", err)
	}

	// Verify mailboxes were not created
	finalMailboxes, err := db.GetUserMailboxesPerUser(userDB)
	if err != nil {
		t.Fatalf("Failed to get final mailboxes: %v", err)
	}

	if len(finalMailboxes) != initialCount {
		t.Errorf("Expected %d mailboxes after rollback, got %d", initialCount, len(finalMailboxes))
	}

	// Verify specific mailboxes don't exist
	for _, mbx := range finalMailboxes {
		if mbx == "TransactionTest1" || mbx == "TransactionTest2" {
			t.Errorf("Mailbox '%s' should not exist after rollback", mbx)
		}
	}

	// Test successful commit
	tx2, err := userDB.Begin()
	if err != nil {
		t.Fatalf("Failed to begin second transaction: %v", err)
	}

	_, err = tx2.Exec(`
		INSERT INTO mailboxes (name, uid_validity, uid_next, created_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
	`, "CommitTest", 1, 1)
	if err != nil {
		t.Fatalf("Failed to insert mailbox: %v", err)
	}

	err = tx2.Commit()
	if err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	// Verify mailbox was created
	exists, err := db.MailboxExistsPerUser(userDB, "CommitTest")
	if err != nil {
		t.Fatalf("Failed to check mailbox existence: %v", err)
	}

	if !exists {
		t.Error("Mailbox 'CommitTest' should exist after commit")
	}
}
