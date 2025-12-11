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
	var count int
	err := sharedDB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table'").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query shared database: %v", err)
	}

	if count < 4 {
		t.Errorf("Expected at least 4 tables in shared database, got %d", count)
	}

	// Verify database file exists
	helpers.AssertDatabaseExists(t, dbManager.BasePath, "shared", 0)
}

// TestDBManagerToUserDB_SuccessFlow tests user database creation
func TestDBManagerToUserDB_SuccessFlow(t *testing.T) {
	// Setup
	dbManager := helpers.SetupTestDatabase(t)
	defer helpers.TeardownTestDatabase(t, dbManager)

	// Create test data
	testData := helpers.SeedTestData(t, dbManager.DBManager)

	// Verify user database was created
	helpers.AssertDatabaseExists(t, dbManager.BasePath, "user", testData.UserID)

	// Verify default mailboxes were created
	userDB, err := dbManager.GetUserDB(testData.UserID)
	if err != nil {
		t.Fatalf("Failed to get user database: %v", err)
	}

	// Check for default mailboxes
	mailboxes, err := db.GetUserMailboxesPerUser(userDB, testData.UserID)
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

	if testData.UserID == 0 {
		t.Error("Expected non-zero user ID")
	}
	if testData.DomainID == 0 {
		t.Error("Expected non-zero domain ID")
	}
	if testData.MailboxID == 0 {
		t.Error("Expected non-zero mailbox ID")
	}

	// Verify user exists in shared database
	sharedDB := dbManager.GetSharedDB()
	var username string
	err := sharedDB.QueryRow("SELECT username FROM users WHERE id = ?", testData.UserID).Scan(&username)
	if err != nil {
		t.Fatalf("Failed to query user: %v", err)
	}

	if username != "alice" {
		t.Errorf("Expected username 'alice', got '%s'", username)
	}

	// Verify domain exists
	var domainName string
	err = sharedDB.QueryRow("SELECT domain FROM domains WHERE id = ?", testData.DomainID).Scan(&domainName)
	if err != nil {
		t.Fatalf("Failed to query domain: %v", err)
	}

	if domainName != "example.com" {
		t.Errorf("Expected domain 'example.com', got '%s'", domainName)
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

	// Verify users have different IDs
	for i, user1 := range testUsers {
		for j, user2 := range testUsers {
			if i != j && user1.UserID == user2.UserID {
				t.Errorf("Users %d and %d have the same ID: %d", i, j, user1.UserID)
			}
		}
	}

	// Verify domains are shared correctly
	// alice and bob should share domain ID
	if testUsers[0].DomainID != testUsers[1].DomainID {
		t.Error("alice and bob should share the same domain ID")
	}

	// charlie should have different domain ID
	if testUsers[2].DomainID == testUsers[0].DomainID {
		t.Error("charlie should have a different domain ID")
	}

	// Added: verify each user has default mailboxes (cross-module consistency)
	for _, tu := range testUsers {
		userDB, err := dbManager.GetUserDB(tu.UserID)
		if err != nil {
			t.Fatalf("Failed to get user DB for user %d: %v", tu.UserID, err)
		}
		mboxes, err := db.GetUserMailboxesPerUser(userDB, tu.UserID)
		if err != nil {
			t.Fatalf("Failed to get mailboxes for user %d: %v", tu.UserID, err)
		}
		// expect default set
		expected := map[string]bool{"Drafts": true, "INBOX": true, "Sent": true, "Spam": true, "Trash": true}
		for _, m := range mboxes {
			delete(expected, m)
		}
		if len(expected) != 0 {
			t.Errorf("User %d missing default mailboxes: %v", tu.UserID, expected)
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
	customMailboxID := helpers.CreateTestMailbox(t, dbManager.DBManager, testData.UserID, "Work")

	if customMailboxID == 0 {
		t.Error("Expected non-zero mailbox ID")
	}

	// Verify mailbox was created
	userDB, err := dbManager.GetUserDB(testData.UserID)
	if err != nil {
		t.Fatalf("Failed to get user database: %v", err)
	}

	retrievedID, err := db.GetMailboxByNamePerUser(userDB, testData.UserID, "Work")
	if err != nil {
		t.Fatalf("Failed to get mailbox: %v", err)
	}

	if retrievedID != customMailboxID {
		t.Errorf("Expected mailbox ID %d, got %d", customMailboxID, retrievedID)
	}

	// Delete mailbox
	err = db.DeleteMailboxPerUser(userDB, testData.UserID, "Work")
	if err != nil {
		t.Fatalf("Failed to delete mailbox: %v", err)
	}

	// Verify mailbox was deleted
	exists, err := db.MailboxExistsPerUser(userDB, testData.UserID, "Work")
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
	userIDs := make([]int64, numUsers)

	for i := 0; i < numUsers; i++ {
		email := fmt.Sprintf("user%d@example.com", i)
		testData := helpers.CreateTestUser(t, dbManager.DBManager, email)
		userIDs[i] = testData.UserID
	}

	// Concurrently access user databases
	var wg sync.WaitGroup
	errors := make(chan error, numUsers)

	for i := 0; i < numUsers; i++ {
		wg.Add(1)
		go func(userID int64) {
			defer wg.Done()

			// Get user database
			userDB, err := dbManager.GetUserDB(userID)
			if err != nil {
				errors <- fmt.Errorf("failed to get user DB for user %d: %v", userID, err)
				return
			}

			// Perform database operations
			mailboxID := helpers.CreateTestMailbox(t, dbManager.DBManager, userID, "Concurrent")

			// Verify mailbox was created
			retrievedID, err := db.GetMailboxByNamePerUser(userDB, userID, "Concurrent")
			if err != nil {
				errors <- fmt.Errorf("failed to get mailbox for user %d: %v", userID, err)
				return
			}

			if retrievedID != mailboxID {
				errors <- fmt.Errorf("mailbox ID mismatch for user %d: expected %d, got %d", userID, mailboxID, retrievedID)
				return
			}
		}(userIDs[i])
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Error(err)
	}

	// Verify all user databases can still be accessed (test cache)
	for _, userID := range userIDs {
		userDB, err := dbManager.GetUserDB(userID)
		if err != nil {
			t.Errorf("Failed to get user DB after concurrent access: %v", err)
		}
		if userDB == nil {
			t.Errorf("Got nil user DB for user %d", userID)
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
	userDB, err := dbManager.GetUserDB(testData.UserID)
	if err != nil {
		t.Fatalf("Failed to get user database: %v", err)
	}

	// Verify database is functional
	mailboxID, err := db.GetMailboxByNamePerUser(userDB, testData.UserID, "INBOX")
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
	defer dbManager2.Close()

	// Verify we can access the same data
	userDB2, err := dbManager2.GetUserDB(testData.UserID)
	if err != nil {
		t.Fatalf("Failed to get user database after recovery: %v", err)
	}

	mailboxID2, err := db.GetMailboxByNamePerUser(userDB2, testData.UserID, "INBOX")
	if err != nil {
		t.Fatalf("Failed to get INBOX after recovery: %v", err)
	}

	if mailboxID2 != mailboxID {
		t.Errorf("Mailbox ID changed after recovery: expected %d, got %d", mailboxID, mailboxID2)
	}

	// Verify domain still exists in shared database
	sharedDB := dbManager2.GetSharedDB()
	var domainName string
	err = sharedDB.QueryRow("SELECT domain FROM domains WHERE id = ?", testData.DomainID).Scan(&domainName)
	if err != nil {
		t.Fatalf("Failed to query domain after recovery: %v", err)
	}

	if domainName != "example.com" {
		t.Errorf("Expected domain 'example.com', got '%s'", domainName)
	}
}

// TestDBManagerToUserDB_RollbackAndCommit tests transaction rollback functionality
func TestDBManagerToUserDB_RollbackAndCommit(t *testing.T) {
	// Setup
	dbManager := helpers.SetupTestDatabase(t)
	defer helpers.TeardownTestDatabase(t, dbManager)

	// Create test user
	testData := helpers.CreateTestUser(t, dbManager.DBManager, "alice@example.com")
	userDB, err := dbManager.GetUserDB(testData.UserID)
	if err != nil {
		t.Fatalf("Failed to get user database: %v", err)
	}

	// Get initial mailbox count
	initialMailboxes, err := db.GetUserMailboxesPerUser(userDB, testData.UserID)
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
		INSERT INTO mailboxes (user_id, name, uid_validity, uid_next, created_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, testData.UserID, "TransactionTest1", 1, 1)
	if err != nil {
		t.Fatalf("Failed to insert first mailbox: %v", err)
	}

	_, err = tx.Exec(`
		INSERT INTO mailboxes (user_id, name, uid_validity, uid_next, created_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, testData.UserID, "TransactionTest2", 1, 1)
	if err != nil {
		t.Fatalf("Failed to insert second mailbox: %v", err)
	}

	// Rollback the transaction
	err = tx.Rollback()
	if err != nil {
		t.Fatalf("Failed to rollback transaction: %v", err)
	}

	// Verify mailboxes were not created
	finalMailboxes, err := db.GetUserMailboxesPerUser(userDB, testData.UserID)
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
		INSERT INTO mailboxes (user_id, name, uid_validity, uid_next, created_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, testData.UserID, "CommitTest", 1, 1)
	if err != nil {
		t.Fatalf("Failed to insert mailbox: %v", err)
	}

	err = tx2.Commit()
	if err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	// Verify mailbox was created
	exists, err := db.MailboxExistsPerUser(userDB, testData.UserID, "CommitTest")
	if err != nil {
		t.Fatalf("Failed to check mailbox existence: %v", err)
	}

	if !exists {
		t.Error("Mailbox 'CommitTest' should exist after commit")
	}
}
