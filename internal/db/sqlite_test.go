package db

import (
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := InitDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to initialize test database: %v", err)
	}
	return db
}

func TestInitDB(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
	}()

	db, err := InitDB(tmpFile.Name())
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer func() { _ = db.Close() }()

	if db == nil {
		t.Fatal("Expected non-nil database connection")
	}

	var fkEnabled int
	err = db.QueryRow("PRAGMA foreign_keys").Scan(&fkEnabled)
	if err != nil {
		t.Fatalf("Failed to check foreign keys: %v", err)
	}
	if fkEnabled != 1 {
		t.Error("Foreign keys should be enabled")
	}
}

func TestInitDB_InvalidPath(t *testing.T) {
	_, err := InitDB("/invalid/path/test.db")
	if err == nil {
		t.Error("Expected error for invalid database path")
	}
}

func TestCreateDomain(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainName := "example.com"
	id, err := CreateDomain(db, domainName)
	if err != nil {
		t.Fatalf("CreateDomain failed: %v", err)
	}

	if id == 0 {
		t.Error("Expected non-zero domain ID")
	}

	var retrievedDomain string
	var enabled bool
	err = db.QueryRow("SELECT domain, enabled FROM domains WHERE id = ?", id).Scan(&retrievedDomain, &enabled)
	if err != nil {
		t.Fatalf("Failed to retrieve domain: %v", err)
	}

	if retrievedDomain != domainName {
		t.Errorf("Expected domain %s, got %s", domainName, retrievedDomain)
	}

	if !enabled {
		t.Error("Expected domain to be enabled")
	}
}

func TestCreateDomain_Duplicate(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainName := "example.com"
	_, err := CreateDomain(db, domainName)
	if err != nil {
		t.Fatalf("First CreateDomain failed: %v", err)
	}

	_, err = CreateDomain(db, domainName)
	if err == nil {
		t.Error("Expected error when creating duplicate domain")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("Expected 'already exists' error, got: %v", err)
	}
}

func TestGetDomainByName(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainName := "example.com"
	createdID, err := CreateDomain(db, domainName)
	if err != nil {
		t.Fatalf("CreateDomain failed: %v", err)
	}

	retrievedID, err := GetDomainByName(db, domainName)
	if err != nil {
		t.Fatalf("GetDomainByName failed: %v", err)
	}

	if createdID != retrievedID {
		t.Errorf("Expected domain ID %d, got %d", createdID, retrievedID)
	}
}

func TestGetDomainByName_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	_, err := GetDomainByName(db, "nonexistent.com")
	if err == nil {
		t.Error("Expected error when getting non-existent domain")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}

func TestGetOrCreateDomain(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainName := "example.com"

	id1, err := GetOrCreateDomain(db, domainName)
	if err != nil {
		t.Fatalf("First GetOrCreateDomain failed: %v", err)
	}

	id2, err := GetOrCreateDomain(db, domainName)
	if err != nil {
		t.Fatalf("Second GetOrCreateDomain failed: %v", err)
	}

	if id1 != id2 {
		t.Errorf("Expected same domain ID, got %d and %d", id1, id2)
	}
}

func TestCreateUser(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, err := CreateDomain(db, "example.com")
	if err != nil {
		t.Fatalf("CreateDomain failed: %v", err)
	}

	username := "testuser"
	userID, err := CreateUser(db, username, domainID)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	if userID == 0 {
		t.Error("Expected non-zero user ID")
	}

	var retrievedUsername string
	var retrievedDomainID int64
	var enabled bool
	err = db.QueryRow("SELECT username, domain_id, enabled FROM users WHERE id = ?", userID).
		Scan(&retrievedUsername, &retrievedDomainID, &enabled)
	if err != nil {
		t.Fatalf("Failed to retrieve user: %v", err)
	}

	if retrievedUsername != username {
		t.Errorf("Expected username %s, got %s", username, retrievedUsername)
	}
	if retrievedDomainID != domainID {
		t.Errorf("Expected domain ID %d, got %d", domainID, retrievedDomainID)
	}
	if !enabled {
		t.Error("Expected user to be enabled")
	}
}

func TestCreateUser_Duplicate(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, err := CreateDomain(db, "example.com")
	if err != nil {
		t.Fatalf("CreateDomain failed: %v", err)
	}

	username := "testuser"
	_, err = CreateUser(db, username, domainID)
	if err != nil {
		t.Fatalf("First CreateUser failed: %v", err)
	}

	_, err = CreateUser(db, username, domainID)
	if err == nil {
		t.Error("Expected error when creating duplicate user")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("Expected 'already exists' error, got: %v", err)
	}
}

func TestGetUserByUsername(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	username := "testuser"
	createdID, err := CreateUser(db, username, domainID)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	retrievedID, err := GetUserByUsername(db, username, domainID)
	if err != nil {
		t.Fatalf("GetUserByUsername failed: %v", err)
	}

	if createdID != retrievedID {
		t.Errorf("Expected user ID %d, got %d", createdID, retrievedID)
	}
}

func TestGetUserByUsername_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")

	_, err := GetUserByUsername(db, "nonexistent", domainID)
	if err == nil {
		t.Error("Expected error when getting non-existent user")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}

func TestGetUserByEmail(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	username := "testuser"
	createdID, err := CreateUser(db, username, domainID)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	email := "testuser@example.com"
	retrievedID, err := GetUserByEmail(db, email)
	if err != nil {
		t.Fatalf("GetUserByEmail failed: %v", err)
	}

	if createdID != retrievedID {
		t.Errorf("Expected user ID %d, got %d", createdID, retrievedID)
	}
}

func TestGetUserByEmail_InvalidFormat(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	_, err := GetUserByEmail(db, "invalid-email")
	if err == nil {
		t.Error("Expected error for invalid email format")
	}
	if !strings.Contains(err.Error(), "invalid email format") {
		t.Errorf("Expected 'invalid email format' error, got: %v", err)
	}
}

func TestGetUserByEmail_NonExistentDomain(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	_, err := GetUserByEmail(db, "user@nonexistent.com")
	if err == nil {
		t.Error("Expected error for non-existent domain")
	}
}

func TestGetOrCreateUser(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	username := "testuser"

	id1, err := GetOrCreateUser(db, username, domainID)
	if err != nil {
		t.Fatalf("First GetOrCreateUser failed: %v", err)
	}

	id2, err := GetOrCreateUser(db, username, domainID)
	if err != nil {
		t.Fatalf("Second GetOrCreateUser failed: %v", err)
	}

	if id1 != id2 {
		t.Errorf("Expected same user ID, got %d and %d", id1, id2)
	}
}

func TestUserExists(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	username := "testuser"

	exists, err := UserExists(db, username, domainID)
	if err != nil {
		t.Fatalf("UserExists failed: %v", err)
	}
	if exists {
		t.Error("User should not exist yet")
	}

	_, err = CreateUser(db, username, domainID)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	exists, err = UserExists(db, username, domainID)
	if err != nil {
		t.Fatalf("UserExists failed after creating user: %v", err)
	}
	if !exists {
		t.Error("User should exist after creation")
	}
}

func TestCreateMailbox(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)

	mailboxID, err := CreateMailbox(db, userID, "INBOX", "\\Inbox")
	if err != nil {
		t.Fatalf("CreateMailbox failed: %v", err)
	}

	if mailboxID == 0 {
		t.Error("Expected non-zero mailbox ID")
	}

	var name, specialUse string
	var uidValidity, uidNext int64
	err = db.QueryRow("SELECT name, special_use, uid_validity, uid_next FROM mailboxes WHERE id = ?", mailboxID).
		Scan(&name, &specialUse, &uidValidity, &uidNext)
	if err != nil {
		t.Fatalf("Failed to retrieve mailbox: %v", err)
	}

	if name != "INBOX" {
		t.Errorf("Expected mailbox name INBOX, got %s", name)
	}
	if specialUse != "\\Inbox" {
		t.Errorf("Expected special use \\Inbox, got %s", specialUse)
	}
	if uidValidity == 0 {
		t.Error("Expected non-zero UID validity")
	}
	if uidNext != 1 {
		t.Errorf("Expected UID next to be 1, got %d", uidNext)
	}
}

func TestCreateMailbox_EmptyName(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)

	_, err := CreateMailbox(db, userID, "", "")
	if err == nil {
		t.Error("Expected error when creating mailbox with empty name")
	}
	if !strings.Contains(err.Error(), "cannot be empty") {
		t.Errorf("Expected 'cannot be empty' error, got: %v", err)
	}
}

func TestCreateMailbox_Duplicate(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)

	_, err := CreateMailbox(db, userID, "INBOX", "\\Inbox")
	if err != nil {
		t.Fatalf("First CreateMailbox failed: %v", err)
	}

	_, err = CreateMailbox(db, userID, "INBOX", "\\Inbox")
	if err == nil {
		t.Error("Expected error when creating duplicate mailbox")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("Expected 'already exists' error, got: %v", err)
	}
}

func TestGetMailboxByName(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)
	createdID, _ := CreateMailbox(db, userID, "INBOX", "\\Inbox")

	retrievedID, err := GetMailboxByName(db, userID, "INBOX")
	if err != nil {
		t.Fatalf("GetMailboxByName failed: %v", err)
	}

	if createdID != retrievedID {
		t.Errorf("Expected mailbox ID %d, got %d", createdID, retrievedID)
	}
}

func TestGetMailboxByName_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)

	_, err := GetMailboxByName(db, userID, "NonExistent")
	if err == nil {
		t.Error("Expected error when getting non-existent mailbox")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}

func TestGetMailboxInfo(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)
	mailboxID, _ := CreateMailbox(db, userID, "INBOX", "\\Inbox")

	uidValidity, uidNext, err := GetMailboxInfo(db, mailboxID)
	if err != nil {
		t.Fatalf("GetMailboxInfo failed: %v", err)
	}

	if uidValidity == 0 {
		t.Error("Expected non-zero UID validity")
	}
	if uidNext != 1 {
		t.Errorf("Expected UID next to be 1, got %d", uidNext)
	}
}

func TestIncrementUIDNext(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)
	mailboxID, _ := CreateMailbox(db, userID, "INBOX", "\\Inbox")

	uid1, err := IncrementUIDNext(db, mailboxID)
	if err != nil {
		t.Fatalf("First IncrementUIDNext failed: %v", err)
	}
	if uid1 != 1 {
		t.Errorf("Expected UID 1, got %d", uid1)
	}

	uid2, err := IncrementUIDNext(db, mailboxID)
	if err != nil {
		t.Fatalf("Second IncrementUIDNext failed: %v", err)
	}
	if uid2 != 2 {
		t.Errorf("Expected UID 2, got %d", uid2)
	}
}

func TestMailboxExists(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)

	exists, err := MailboxExists(db, userID, "INBOX")
	if err != nil {
		t.Fatalf("MailboxExists failed: %v", err)
	}
	if exists {
		t.Error("Mailbox should not exist yet")
	}

	_, err = CreateMailbox(db, userID, "INBOX", "\\Inbox")
	if err != nil {
		t.Fatalf("CreateMailbox failed: %v", err)
	}

	exists, err = MailboxExists(db, userID, "INBOX")
	if err != nil {
		t.Fatalf("MailboxExists failed after creating mailbox: %v", err)
	}
	if !exists {
		t.Error("Mailbox should exist after creation")
	}
}

func TestGetUserMailboxes(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)

	mailboxes := []string{"INBOX", "Sent", "Drafts", "Trash"}
	for _, name := range mailboxes {
		_, err := CreateMailbox(db, userID, name, "")
		if err != nil {
			t.Fatalf("Failed to create mailbox %s: %v", name, err)
		}
	}

	retrieved, err := GetUserMailboxes(db, userID)
	if err != nil {
		t.Fatalf("GetUserMailboxes failed: %v", err)
	}

	if len(retrieved) != len(mailboxes) {
		t.Errorf("Expected %d mailboxes, got %d", len(mailboxes), len(retrieved))
	}

	expected := []string{"Drafts", "INBOX", "Sent", "Trash"}
	for i, name := range expected {
		if retrieved[i] != name {
			t.Errorf("Expected mailbox %s at index %d, got %s", name, i, retrieved[i])
		}
	}
}

func TestDeleteMailbox(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)
	_, _ = CreateMailbox(db, userID, "INBOX", "\\Inbox")
	_, _ = CreateMailbox(db, userID, "TestFolder", "")

	err := DeleteMailbox(db, userID, "TestFolder")
	if err != nil {
		t.Fatalf("DeleteMailbox failed: %v", err)
	}

	exists, _ := MailboxExists(db, userID, "TestFolder")
	if exists {
		t.Error("Mailbox should not exist after deletion")
	}
}

func TestDeleteMailbox_INBOX(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)
	_, _ = CreateMailbox(db, userID, "INBOX", "\\Inbox")

	err := DeleteMailbox(db, userID, "INBOX")
	if err == nil {
		t.Error("Expected error when deleting INBOX")
	}
	if !strings.Contains(err.Error(), "cannot delete INBOX") {
		t.Errorf("Expected 'cannot delete INBOX' error, got: %v", err)
	}
}

func TestDeleteMailbox_WithChildren(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)
	_, _ = CreateMailbox(db, userID, "Parent", "")
	_, _ = CreateMailbox(db, userID, "Parent/Child", "")

	err := DeleteMailbox(db, userID, "Parent")
	if err == nil {
		t.Error("Expected error when deleting mailbox with children")
	}
	if !strings.Contains(err.Error(), "inferior hierarchical names") {
		t.Errorf("Expected 'inferior hierarchical names' error, got: %v", err)
	}
}

func TestDeleteMailbox_DefaultMailbox(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)
	_, _ = CreateMailbox(db, userID, "Sent", "\\Sent")

	err := DeleteMailbox(db, userID, "Sent")
	if err == nil {
		t.Error("Expected error when deleting default mailbox")
	}
	if !strings.Contains(err.Error(), "cannot delete default mailbox") {
		t.Errorf("Expected 'cannot delete default mailbox' error, got: %v", err)
	}
}

func TestRenameMailbox(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)
	_, _ = CreateMailbox(db, userID, "OldName", "")

	err := RenameMailbox(db, userID, "OldName", "NewName")
	if err != nil {
		t.Fatalf("RenameMailbox failed: %v", err)
	}

	exists, _ := MailboxExists(db, userID, "OldName")
	if exists {
		t.Error("Old mailbox name should not exist")
	}

	exists, _ = MailboxExists(db, userID, "NewName")
	if !exists {
		t.Error("New mailbox name should exist")
	}
}

func TestRenameMailbox_ToINBOX(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)
	_, _ = CreateMailbox(db, userID, "TestFolder", "")

	err := RenameMailbox(db, userID, "TestFolder", "INBOX")
	if err == nil {
		t.Error("Expected error when renaming to INBOX")
	}
	if !strings.Contains(err.Error(), "cannot rename to INBOX") {
		t.Errorf("Expected 'cannot rename to INBOX' error, got: %v", err)
	}
}

func TestRenameMailbox_NonExistent(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)

	err := RenameMailbox(db, userID, "NonExistent", "NewName")
	if err == nil {
		t.Error("Expected error when renaming non-existent mailbox")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("Expected 'does not exist' error, got: %v", err)
	}
}

func TestRenameMailbox_AlreadyExists(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)
	_, _ = CreateMailbox(db, userID, "Mailbox1", "")
	_, _ = CreateMailbox(db, userID, "Mailbox2", "")

	err := RenameMailbox(db, userID, "Mailbox1", "Mailbox2")
	if err == nil {
		t.Error("Expected error when renaming to existing mailbox")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("Expected 'already exists' error, got: %v", err)
	}
}

func TestRenameMailbox_WithChildren(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)
	_, _ = CreateMailbox(db, userID, "Parent", "")
	_, _ = CreateMailbox(db, userID, "Parent/Child", "")

	err := RenameMailbox(db, userID, "Parent", "NewParent")
	if err != nil {
		t.Fatalf("RenameMailbox with children failed: %v", err)
	}

	exists, _ := MailboxExists(db, userID, "NewParent")
	if !exists {
		t.Error("New parent mailbox should exist")
	}

	exists, _ = MailboxExists(db, userID, "NewParent/Child")
	if !exists {
		t.Error("Renamed child mailbox should exist")
	}

	exists, _ = MailboxExists(db, userID, "Parent/Child")
	if exists {
		t.Error("Old child mailbox should not exist")
	}
}

func TestRenameMailbox_CreateIntermediateHierarchy(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)
	_, _ = CreateMailbox(db, userID, "Mailbox", "")

	err := RenameMailbox(db, userID, "Mailbox", "Parent/Child/GrandChild")
	if err != nil {
		t.Fatalf("RenameMailbox with hierarchy creation failed: %v", err)
	}

	exists, _ := MailboxExists(db, userID, "Parent")
	if !exists {
		t.Error("Intermediate parent mailbox should be created")
	}

	exists, _ = MailboxExists(db, userID, "Parent/Child")
	if !exists {
		t.Error("Intermediate child mailbox should be created")
	}

	exists, _ = MailboxExists(db, userID, "Parent/Child/GrandChild")
	if !exists {
		t.Error("Target mailbox should exist")
	}
}

func TestStoreBlob(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	content := "test blob content"
	blobID, err := StoreBlob(db, content)
	if err != nil {
		t.Fatalf("StoreBlob failed: %v", err)
	}

	if blobID == 0 {
		t.Error("Expected non-zero blob ID")
	}

	var retrievedContent string
	var refCount int
	err = db.QueryRow("SELECT content, reference_count FROM blobs WHERE id = ?", blobID).
		Scan(&retrievedContent, &refCount)
	if err != nil {
		t.Fatalf("Failed to retrieve blob: %v", err)
	}

	if retrievedContent != content {
		t.Errorf("Expected content %s, got %s", content, retrievedContent)
	}
	if refCount != 1 {
		t.Errorf("Expected reference count 1, got %d", refCount)
	}
}

func TestStoreBlob_Duplicate(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	content := "test blob content"
	blobID1, err := StoreBlob(db, content)
	if err != nil {
		t.Fatalf("First StoreBlob failed: %v", err)
	}

	blobID2, err := StoreBlob(db, content)
	if err != nil {
		t.Fatalf("Second StoreBlob failed: %v", err)
	}

	if blobID1 != blobID2 {
		t.Errorf("Expected same blob ID for duplicate content, got %d and %d", blobID1, blobID2)
	}

	var refCount int
	err = db.QueryRow("SELECT reference_count FROM blobs WHERE id = ?", blobID1).Scan(&refCount)
	if err != nil {
		t.Fatalf("Failed to retrieve reference count: %v", err)
	}

	if refCount != 2 {
		t.Errorf("Expected reference count 2, got %d", refCount)
	}
}

func TestGetBlob(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	content := "test blob content"
	blobID, _ := StoreBlob(db, content)

	retrievedContent, err := GetBlob(db, blobID)
	if err != nil {
		t.Fatalf("GetBlob failed: %v", err)
	}

	if retrievedContent != content {
		t.Errorf("Expected content %s, got %s", content, retrievedContent)
	}
}

func TestGetBlob_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	_, err := GetBlob(db, 99999)
	if err == nil {
		t.Error("Expected error when getting non-existent blob")
	}
}

func TestDecrementBlobReference(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	content := "test blob content"
	blobID, _ := StoreBlob(db, content)
	_, _ = StoreBlob(db, content)

	err := DecrementBlobReference(db, blobID)
	if err != nil {
		t.Fatalf("DecrementBlobReference failed: %v", err)
	}

	var refCount int
	err = db.QueryRow("SELECT reference_count FROM blobs WHERE id = ?", blobID).Scan(&refCount)
	if err != nil {
		t.Fatalf("Failed to retrieve reference count: %v", err)
	}

	if refCount != 1 {
		t.Errorf("Expected reference count 1, got %d", refCount)
	}

	err = DecrementBlobReference(db, blobID)
	if err != nil {
		t.Fatalf("Second DecrementBlobReference failed: %v", err)
	}

	err = db.QueryRow("SELECT reference_count FROM blobs WHERE id = ?", blobID).Scan(&refCount)
	if err != sql.ErrNoRows {
		t.Error("Expected blob to be deleted when reference count reaches 0")
	}
}

func TestCreateMessage(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	subject := "Test Message"
	inReplyTo := "<parent@example.com>"
	references := "<ref1@example.com> <ref2@example.com>"
	date := time.Now()
	sizeBytes := int64(1024)

	messageID, err := CreateMessage(db, subject, inReplyTo, references, date, sizeBytes)
	if err != nil {
		t.Fatalf("CreateMessage failed: %v", err)
	}

	if messageID == 0 {
		t.Error("Expected non-zero message ID")
	}

	var retrievedSubject, retrievedInReplyTo, retrievedReferences string
	var retrievedDate time.Time
	var retrievedSize int64
	err = db.QueryRow("SELECT subject, in_reply_to, references_header, date, size_bytes FROM messages WHERE id = ?", messageID).
		Scan(&retrievedSubject, &retrievedInReplyTo, &retrievedReferences, &retrievedDate, &retrievedSize)
	if err != nil {
		t.Fatalf("Failed to retrieve message: %v", err)
	}

	if retrievedSubject != subject {
		t.Errorf("Expected subject %s, got %s", subject, retrievedSubject)
	}
	if retrievedInReplyTo != inReplyTo {
		t.Errorf("Expected in_reply_to %s, got %s", inReplyTo, retrievedInReplyTo)
	}
	if retrievedReferences != references {
		t.Errorf("Expected references %s, got %s", references, retrievedReferences)
	}
	if retrievedSize != sizeBytes {
		t.Errorf("Expected size %d, got %d", sizeBytes, retrievedSize)
	}
}

func TestAddMessageToMailbox(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)
	mailboxID, _ := CreateMailbox(db, userID, "INBOX", "\\Inbox")
	messageID, _ := CreateMessage(db, "Test", "", "", time.Now(), 100)

	err := AddMessageToMailbox(db, messageID, mailboxID, "\\Seen", time.Now())
	if err != nil {
		t.Fatalf("AddMessageToMailbox failed: %v", err)
	}

	var flags string
	var uid int64
	err = db.QueryRow("SELECT flags, uid FROM message_mailbox WHERE message_id = ? AND mailbox_id = ?", messageID, mailboxID).
		Scan(&flags, &uid)
	if err != nil {
		t.Fatalf("Failed to retrieve message_mailbox entry: %v", err)
	}

	if flags != "\\Seen" {
		t.Errorf("Expected flags \\Seen, got %s", flags)
	}
	if uid != 1 {
		t.Errorf("Expected UID 1, got %d", uid)
	}
}

func TestGetMessagesByMailbox(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)
	mailboxID, _ := CreateMailbox(db, userID, "INBOX", "\\Inbox")

	messageIDs := make([]int64, 3)
	for i := 0; i < 3; i++ {
		msgID, _ := CreateMessage(db, "Test", "", "", time.Now(), 100)
		messageIDs[i] = msgID
		_ = AddMessageToMailbox(db, msgID, mailboxID, "", time.Now())
	}

	retrieved, err := GetMessagesByMailbox(db, mailboxID)
	if err != nil {
		t.Fatalf("GetMessagesByMailbox failed: %v", err)
	}

	if len(retrieved) != len(messageIDs) {
		t.Errorf("Expected %d messages, got %d", len(messageIDs), len(retrieved))
	}

	for i, id := range messageIDs {
		if retrieved[i] != id {
			t.Errorf("Expected message ID %d at index %d, got %d", id, i, retrieved[i])
		}
	}
}

func TestGetMessageCount(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)
	mailboxID, _ := CreateMailbox(db, userID, "INBOX", "\\Inbox")

	for i := 0; i < 5; i++ {
		msgID, _ := CreateMessage(db, "Test", "", "", time.Now(), 100)
		_ = AddMessageToMailbox(db, msgID, mailboxID, "", time.Now())
	}

	count, err := GetMessageCount(db, mailboxID)
	if err != nil {
		t.Fatalf("GetMessageCount failed: %v", err)
	}

	if count != 5 {
		t.Errorf("Expected message count 5, got %d", count)
	}
}

func TestGetUnseenCount(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)
	mailboxID, _ := CreateMailbox(db, userID, "INBOX", "\\Inbox")

	for i := 0; i < 3; i++ {
		msgID, _ := CreateMessage(db, "Test", "", "", time.Now(), 100)
		_ = AddMessageToMailbox(db, msgID, mailboxID, "\\Seen", time.Now())
	}

	for i := 0; i < 2; i++ {
		msgID, _ := CreateMessage(db, "Test", "", "", time.Now(), 100)
		_ = AddMessageToMailbox(db, msgID, mailboxID, "", time.Now())
	}

	count, err := GetUnseenCount(db, mailboxID)
	if err != nil {
		t.Fatalf("GetUnseenCount failed: %v", err)
	}

	if count != 2 {
		t.Errorf("Expected unseen count 2, got %d", count)
	}
}

func TestUpdateMessageFlags(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)
	mailboxID, _ := CreateMailbox(db, userID, "INBOX", "\\Inbox")
	messageID, _ := CreateMessage(db, "Test", "", "", time.Now(), 100)
	_ = AddMessageToMailbox(db, messageID, mailboxID, "", time.Now())

	err := UpdateMessageFlags(db, mailboxID, messageID, "\\Seen \\Flagged")
	if err != nil {
		t.Fatalf("UpdateMessageFlags failed: %v", err)
	}

	flags, err := GetMessageFlags(db, mailboxID, messageID)
	if err != nil {
		t.Fatalf("GetMessageFlags failed: %v", err)
	}

	if flags != "\\Seen \\Flagged" {
		t.Errorf("Expected flags '\\Seen \\Flagged', got %s", flags)
	}
}

func TestGetMessageFlags(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)
	mailboxID, _ := CreateMailbox(db, userID, "INBOX", "\\Inbox")
	messageID, _ := CreateMessage(db, "Test", "", "", time.Now(), 100)
	_ = AddMessageToMailbox(db, messageID, mailboxID, "\\Seen", time.Now())

	flags, err := GetMessageFlags(db, mailboxID, messageID)
	if err != nil {
		t.Fatalf("GetMessageFlags failed: %v", err)
	}

	if flags != "\\Seen" {
		t.Errorf("Expected flags \\Seen, got %s", flags)
	}
}

func TestAddAddress(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	messageID, _ := CreateMessage(db, "Test", "", "", time.Now(), 100)

	err := AddAddress(db, messageID, "from", "John Doe", "john@example.com", 0)
	if err != nil {
		t.Fatalf("AddAddress failed: %v", err)
	}

	var name, email string
	err = db.QueryRow("SELECT name, email FROM addresses WHERE message_id = ?", messageID).
		Scan(&name, &email)
	if err != nil {
		t.Fatalf("Failed to retrieve address: %v", err)
	}

	if name != "John Doe" {
		t.Errorf("Expected name 'John Doe', got %s", name)
	}
	if email != "john@example.com" {
		t.Errorf("Expected email 'john@example.com', got %s", email)
	}
}

func TestGetMessageAddresses(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	messageID, _ := CreateMessage(db, "Test", "", "", time.Now(), 100)

	_ = AddAddress(db, messageID, "from", "John Doe", "john@example.com", 0)
	_ = AddAddress(db, messageID, "to", "Jane Smith", "jane@example.com", 1)
	_ = AddAddress(db, messageID, "to", "", "bob@example.com", 2)

	addresses, err := GetMessageAddresses(db, messageID, "to")
	if err != nil {
		t.Fatalf("GetMessageAddresses failed: %v", err)
	}

	if len(addresses) != 2 {
		t.Errorf("Expected 2 addresses, got %d", len(addresses))
	}

	if addresses[0] != "Jane Smith <jane@example.com>" {
		t.Errorf("Expected 'Jane Smith <jane@example.com>', got %s", addresses[0])
	}
	if addresses[1] != "bob@example.com" {
		t.Errorf("Expected 'bob@example.com', got %s", addresses[1])
	}
}

func TestAddMessagePart(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	messageID, _ := CreateMessage(db, "Test", "", "", time.Now(), 100)
	blobID, _ := StoreBlob(db, "part content")

	partID, err := AddMessagePart(db, messageID, 1, sql.NullInt64{}, "text/plain", "", "7bit", "utf-8", "", "", sql.NullInt64{Int64: blobID, Valid: true}, "", 12)
	if err != nil {
		t.Fatalf("AddMessagePart failed: %v", err)
	}

	if partID == 0 {
		t.Error("Expected non-zero part ID")
	}

	var contentType, charset string
	var partBlobID int64
	err = db.QueryRow("SELECT content_type, charset, blob_id FROM message_parts WHERE id = ?", partID).
		Scan(&contentType, &charset, &partBlobID)
	if err != nil {
		t.Fatalf("Failed to retrieve message part: %v", err)
	}

	if contentType != "text/plain" {
		t.Errorf("Expected content type 'text/plain', got %s", contentType)
	}
	if charset != "utf-8" {
		t.Errorf("Expected charset 'utf-8', got %s", charset)
	}
	if partBlobID != blobID {
		t.Errorf("Expected blob ID %d, got %d", blobID, partBlobID)
	}
}

func TestGetMessageParts(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	messageID, _ := CreateMessage(db, "Test", "", "", time.Now(), 100)
	blobID, _ := StoreBlob(db, "part content")

	_, _ = AddMessagePart(db, messageID, 1, sql.NullInt64{}, "text/plain", "", "7bit", "utf-8", "", "", sql.NullInt64{Int64: blobID, Valid: true}, "", 12)
	_, _ = AddMessagePart(db, messageID, 2, sql.NullInt64{}, "text/html", "", "7bit", "utf-8", "", "", sql.NullInt64{}, "HTML content", 12)

	parts, err := GetMessageParts(db, messageID)
	if err != nil {
		t.Fatalf("GetMessageParts failed: %v", err)
	}

	if len(parts) != 2 {
		t.Errorf("Expected 2 parts, got %d", len(parts))
	}

	if parts[0]["content_type"] != "text/plain" {
		t.Errorf("Expected first part content type 'text/plain', got %s", parts[0]["content_type"])
	}
	if parts[1]["content_type"] != "text/html" {
		t.Errorf("Expected second part content type 'text/html', got %s", parts[1]["content_type"])
	}
}

func TestSubscribeToMailbox(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)

	err := SubscribeToMailbox(db, userID, "INBOX")
	if err != nil {
		t.Fatalf("SubscribeToMailbox failed: %v", err)
	}

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM subscriptions WHERE user_id = ? AND mailbox_name = ?", userID, "INBOX").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to verify subscription: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected 1 subscription, got %d", count)
	}
}

func TestSubscribeToMailbox_Duplicate(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)

	_ = SubscribeToMailbox(db, userID, "INBOX")
	err := SubscribeToMailbox(db, userID, "INBOX")
	if err != nil {
		t.Error("Duplicate subscription should not fail due to INSERT OR IGNORE")
	}

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM subscriptions WHERE user_id = ? AND mailbox_name = ?", userID, "INBOX").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to verify subscription count: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected 1 subscription after duplicate insert, got %d", count)
	}
}

func TestUnsubscribeFromMailbox(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)
	_ = SubscribeToMailbox(db, userID, "INBOX")

	err := UnsubscribeFromMailbox(db, userID, "INBOX")
	if err != nil {
		t.Fatalf("UnsubscribeFromMailbox failed: %v", err)
	}

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM subscriptions WHERE user_id = ? AND mailbox_name = ?", userID, "INBOX").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to verify unsubscription: %v", err)
	}

	if count != 0 {
		t.Errorf("Expected 0 subscriptions after unsubscribe, got %d", count)
	}
}

func TestUnsubscribeFromMailbox_NonExistent(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)

	err := UnsubscribeFromMailbox(db, userID, "INBOX")
	if err == nil {
		t.Error("Expected error when unsubscribing from non-existent subscription")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("Expected 'does not exist' error, got: %v", err)
	}
}

func TestGetUserSubscriptions(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)

	mailboxes := []string{"Drafts", "INBOX", "Sent"}
	for _, name := range mailboxes {
		_ = SubscribeToMailbox(db, userID, name)
	}

	subscriptions, err := GetUserSubscriptions(db, userID)
	if err != nil {
		t.Fatalf("GetUserSubscriptions failed: %v", err)
	}

	if len(subscriptions) != len(mailboxes) {
		t.Errorf("Expected %d subscriptions, got %d", len(mailboxes), len(subscriptions))
	}

	expected := []string{"Drafts", "INBOX", "Sent"}
	for i, name := range expected {
		if subscriptions[i] != name {
			t.Errorf("Expected subscription %s at index %d, got %s", name, i, subscriptions[i])
		}
	}
}

func TestIsMailboxSubscribed(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)

	subscribed, err := IsMailboxSubscribed(db, userID, "INBOX")
	if err != nil {
		t.Fatalf("IsMailboxSubscribed failed: %v", err)
	}
	if subscribed {
		t.Error("Mailbox should not be subscribed yet")
	}

	_ = SubscribeToMailbox(db, userID, "INBOX")

	subscribed, err = IsMailboxSubscribed(db, userID, "INBOX")
	if err != nil {
		t.Fatalf("IsMailboxSubscribed failed after subscribing: %v", err)
	}
	if !subscribed {
		t.Error("Mailbox should be subscribed")
	}
}

func TestRecordDelivery(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)
	messageID, _ := CreateMessage(db, "Test", "", "", time.Now(), 100)

	err := RecordDelivery(db, messageID, "recipient@example.com", "sender@example.com", "delivered", sql.NullInt64{Int64: userID, Valid: true}, "250 OK")
	if err != nil {
		t.Fatalf("RecordDelivery failed: %v", err)
	}

	var recipient, sender, status, smtpResponse string
	err = db.QueryRow("SELECT recipient, sender, status, smtp_response FROM deliveries WHERE message_id = ?", messageID).
		Scan(&recipient, &sender, &status, &smtpResponse)
	if err != nil {
		t.Fatalf("Failed to retrieve delivery: %v", err)
	}

	if recipient != "recipient@example.com" {
		t.Errorf("Expected recipient 'recipient@example.com', got %s", recipient)
	}
	if sender != "sender@example.com" {
		t.Errorf("Expected sender 'sender@example.com', got %s", sender)
	}
	if status != "delivered" {
		t.Errorf("Expected status 'delivered', got %s", status)
	}
	if smtpResponse != "250 OK" {
		t.Errorf("Expected SMTP response '250 OK', got %s", smtpResponse)
	}
}

func TestQueueOutboundMessage(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	messageID, _ := CreateMessage(db, "Test", "", "", time.Now(), 100)

	err := QueueOutboundMessage(db, messageID, "sender@example.com", "recipient@example.com", 5)
	if err != nil {
		t.Fatalf("QueueOutboundMessage failed: %v", err)
	}

	var sender, recipient, status string
	var maxRetries int
	err = db.QueryRow("SELECT sender, recipient, status, max_retries FROM outbound_queue WHERE message_id = ?", messageID).
		Scan(&sender, &recipient, &status, &maxRetries)
	if err != nil {
		t.Fatalf("Failed to retrieve outbound message: %v", err)
	}

	if sender != "sender@example.com" {
		t.Errorf("Expected sender 'sender@example.com', got %s", sender)
	}
	if recipient != "recipient@example.com" {
		t.Errorf("Expected recipient 'recipient@example.com', got %s", recipient)
	}
	if status != "pending" {
		t.Errorf("Expected status 'pending', got %s", status)
	}
	if maxRetries != 5 {
		t.Errorf("Expected max retries 5, got %d", maxRetries)
	}
}

func TestGetPendingOutboundMessages(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	messageIDs := make([]int64, 3)
	for i := range 3 {
		msgID, _ := CreateMessage(db, "Test", "", "", time.Now(), 100)
		messageIDs[i] = msgID
		_ = QueueOutboundMessage(db, msgID, "sender@example.com", "recipient@example.com", 5)
	}

	messages, err := GetPendingOutboundMessages(db, 10)
	if err != nil {
		t.Fatalf("GetPendingOutboundMessages failed: %v", err)
	}

	if len(messages) != len(messageIDs) {
		t.Errorf("Expected %d pending messages, got %d", len(messageIDs), len(messages))
	}

	for i, msg := range messages {
		if msg["message_id"] != messageIDs[i] {
			t.Errorf("Expected message ID %d at index %d, got %d", messageIDs[i], i, msg["message_id"])
		}
	}
}

func TestUpdateOutboundStatus(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	messageID, _ := CreateMessage(db, "Test", "", "", time.Now(), 100)
	_ = QueueOutboundMessage(db, messageID, "sender@example.com", "recipient@example.com", 5)

	var queueID int64
	err := db.QueryRow("SELECT id FROM outbound_queue WHERE message_id = ?", messageID).Scan(&queueID)
	if err != nil {
		t.Fatalf("Failed to get queue ID: %v", err)
	}

	err = UpdateOutboundStatus(db, queueID, "sent", "")
	if err != nil {
		t.Fatalf("UpdateOutboundStatus failed: %v", err)
	}

	var status string
	err = db.QueryRow("SELECT status FROM outbound_queue WHERE id = ?", queueID).Scan(&status)
	if err != nil {
		t.Fatalf("Failed to retrieve status: %v", err)
	}

	if status != "sent" {
		t.Errorf("Expected status 'sent', got %s", status)
	}
}

func TestRetryOutboundMessage(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	messageID, _ := CreateMessage(db, "Test", "", "", time.Now(), 100)
	_ = QueueOutboundMessage(db, messageID, "sender@example.com", "recipient@example.com", 5)

	var queueID int64
	err := db.QueryRow("SELECT id FROM outbound_queue WHERE message_id = ?", messageID).Scan(&queueID)
	if err != nil {
		t.Fatalf("Failed to get queue ID: %v", err)
	}

	delay := 5 * time.Minute
	err = RetryOutboundMessage(db, queueID, delay)
	if err != nil {
		t.Fatalf("RetryOutboundMessage failed: %v", err)
	}

	var retryCount int
	err = db.QueryRow("SELECT retry_count FROM outbound_queue WHERE id = ?", queueID).Scan(&retryCount)
	if err != nil {
		t.Fatalf("Failed to retrieve retry count: %v", err)
	}

	if retryCount != 1 {
		t.Errorf("Expected retry count 1, got %d", retryCount)
	}
}

func TestAddMessageHeader(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	messageID, _ := CreateMessage(db, "Test", "", "", time.Now(), 100)

	err := AddMessageHeader(db, messageID, "X-Custom-Header", "Custom Value", 0)
	if err != nil {
		t.Fatalf("AddMessageHeader failed: %v", err)
	}

	var headerName, headerValue string
	err = db.QueryRow("SELECT header_name, header_value FROM message_headers WHERE message_id = ?", messageID).
		Scan(&headerName, &headerValue)
	if err != nil {
		t.Fatalf("Failed to retrieve header: %v", err)
	}

	if headerName != "X-Custom-Header" {
		t.Errorf("Expected header name 'X-Custom-Header', got %s", headerName)
	}
	if headerValue != "Custom Value" {
		t.Errorf("Expected header value 'Custom Value', got %s", headerValue)
	}
}

func TestGetMessageHeaders(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	messageID, _ := CreateMessage(db, "Test", "", "", time.Now(), 100)

	_ = AddMessageHeader(db, messageID, "From", "sender@example.com", 0)
	_ = AddMessageHeader(db, messageID, "To", "recipient@example.com", 1)
	_ = AddMessageHeader(db, messageID, "Subject", "Test Subject", 2)

	headers, err := GetMessageHeaders(db, messageID)
	if err != nil {
		t.Fatalf("GetMessageHeaders failed: %v", err)
	}

	if len(headers) != 3 {
		t.Errorf("Expected 3 headers, got %d", len(headers))
	}

	expectedHeaders := []struct {
		name  string
		value string
	}{
		{"From", "sender@example.com"},
		{"To", "recipient@example.com"},
		{"Subject", "Test Subject"},
	}

	for i, expected := range expectedHeaders {
		if headers[i]["name"] != expected.name {
			t.Errorf("Expected header name %s at index %d, got %s", expected.name, i, headers[i]["name"])
		}
		if headers[i]["value"] != expected.value {
			t.Errorf("Expected header value %s at index %d, got %s", expected.value, i, headers[i]["value"])
		}
	}
}

func TestCreateRoleMailbox(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")

	roleID, err := CreateRoleMailbox(db, "support@example.com", domainID, "Support mailbox")
	if err != nil {
		t.Fatalf("CreateRoleMailbox failed: %v", err)
	}

	if roleID == 0 {
		t.Error("Expected non-zero role mailbox ID")
	}

	var email, description string
	var enabled bool
	err = db.QueryRow("SELECT email, description, enabled FROM role_mailboxes WHERE id = ?", roleID).
		Scan(&email, &description, &enabled)
	if err != nil {
		t.Fatalf("Failed to retrieve role mailbox: %v", err)
	}

	if email != "support@example.com" {
		t.Errorf("Expected email 'support@example.com', got %s", email)
	}
	if description != "Support mailbox" {
		t.Errorf("Expected description 'Support mailbox', got %s", description)
	}
	if !enabled {
		t.Error("Expected role mailbox to be enabled")
	}
}

func TestCreateRoleMailbox_Duplicate(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")

	_, err := CreateRoleMailbox(db, "support@example.com", domainID, "Support mailbox")
	if err != nil {
		t.Fatalf("First CreateRoleMailbox failed: %v", err)
	}

	_, err = CreateRoleMailbox(db, "support@example.com", domainID, "Support mailbox")
	if err == nil {
		t.Error("Expected error when creating duplicate role mailbox")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("Expected 'already exists' error, got: %v", err)
	}
}

func TestGetRoleMailboxByEmail(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	createdID, _ := CreateRoleMailbox(db, "support@example.com", domainID, "Support mailbox")

	retrievedID, retrievedDomainID, err := GetRoleMailboxByEmail(db, "support@example.com")
	if err != nil {
		t.Fatalf("GetRoleMailboxByEmail failed: %v", err)
	}

	if retrievedID != createdID {
		t.Errorf("Expected role mailbox ID %d, got %d", createdID, retrievedID)
	}
	if retrievedDomainID != domainID {
		t.Errorf("Expected domain ID %d, got %d", domainID, retrievedDomainID)
	}
}

func TestGetRoleMailboxByEmail_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	_, _, err := GetRoleMailboxByEmail(db, "nonexistent@example.com")
	if err == nil {
		t.Error("Expected error when getting non-existent role mailbox")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}

func TestGetRoleMailboxByID(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	roleID, _ := CreateRoleMailbox(db, "support@example.com", domainID, "Support mailbox")

	email, retrievedDomainID, err := GetRoleMailboxByID(db, roleID)
	if err != nil {
		t.Fatalf("GetRoleMailboxByID failed: %v", err)
	}

	if email != "support@example.com" {
		t.Errorf("Expected email 'support@example.com', got %s", email)
	}
	if retrievedDomainID != domainID {
		t.Errorf("Expected domain ID %d, got %d", domainID, retrievedDomainID)
	}
}

func TestGetRoleMailboxByID_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	_, _, err := GetRoleMailboxByID(db, 99999)
	if err == nil {
		t.Error("Expected error when getting non-existent role mailbox by ID")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}

func TestGetOrCreateRoleMailbox(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")

	id1, err := GetOrCreateRoleMailbox(db, "support@example.com", domainID, "Support mailbox")
	if err != nil {
		t.Fatalf("First GetOrCreateRoleMailbox failed: %v", err)
	}

	id2, err := GetOrCreateRoleMailbox(db, "support@example.com", domainID, "Support mailbox")
	if err != nil {
		t.Fatalf("Second GetOrCreateRoleMailbox failed: %v", err)
	}

	if id1 != id2 {
		t.Errorf("Expected same role mailbox ID, got %d and %d", id1, id2)
	}
}

func TestRoleMailboxExists(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")

	exists, err := RoleMailboxExists(db, "support@example.com")
	if err != nil {
		t.Fatalf("RoleMailboxExists failed: %v", err)
	}
	if exists {
		t.Error("Role mailbox should not exist yet")
	}

	_, _ = CreateRoleMailbox(db, "support@example.com", domainID, "Support mailbox")

	exists, err = RoleMailboxExists(db, "support@example.com")
	if err != nil {
		t.Fatalf("RoleMailboxExists failed after creating role mailbox: %v", err)
	}
	if !exists {
		t.Error("Role mailbox should exist after creation")
	}
}

func TestAssignUserToRoleMailbox(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)
	assignedByUserID, _ := CreateUser(db, "admin", domainID)
	roleID, _ := CreateRoleMailbox(db, "support@example.com", domainID, "Support mailbox")

	err := AssignUserToRoleMailbox(db, userID, roleID, assignedByUserID)
	if err != nil {
		t.Fatalf("AssignUserToRoleMailbox failed: %v", err)
	}

	var assignedUserID int64
	var isActive bool
	err = db.QueryRow("SELECT user_id, is_active FROM user_role_assignments WHERE role_mailbox_id = ?", roleID).
		Scan(&assignedUserID, &isActive)
	if err != nil {
		t.Fatalf("Failed to retrieve assignment: %v", err)
	}

	if assignedUserID != userID {
		t.Errorf("Expected user ID %d, got %d", userID, assignedUserID)
	}
	if !isActive {
		t.Error("Expected assignment to be active")
	}
}

func TestAssignUserToRoleMailbox_Reassign(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	user1ID, _ := CreateUser(db, "user1", domainID)
	user2ID, _ := CreateUser(db, "user2", domainID)
	adminID, _ := CreateUser(db, "admin", domainID)
	roleID, _ := CreateRoleMailbox(db, "support@example.com", domainID, "Support mailbox")

	_ = AssignUserToRoleMailbox(db, user1ID, roleID, adminID)
	err := AssignUserToRoleMailbox(db, user2ID, roleID, adminID)
	if err != nil {
		t.Fatalf("Reassigning role mailbox failed: %v", err)
	}

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM user_role_assignments WHERE role_mailbox_id = ? AND is_active = TRUE", roleID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count active assignments: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected 1 active assignment after reassignment, got %d", count)
	}

	var activeUserID int64
	err = db.QueryRow("SELECT user_id FROM user_role_assignments WHERE role_mailbox_id = ? AND is_active = TRUE", roleID).Scan(&activeUserID)
	if err != nil {
		t.Fatalf("Failed to retrieve active user: %v", err)
	}

	if activeUserID != user2ID {
		t.Errorf("Expected active user ID %d, got %d", user2ID, activeUserID)
	}
}

func TestUnassignUserFromRoleMailbox(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)
	adminID, _ := CreateUser(db, "admin", domainID)
	roleID, _ := CreateRoleMailbox(db, "support@example.com", domainID, "Support mailbox")

	_ = AssignUserToRoleMailbox(db, userID, roleID, adminID)

	err := UnassignUserFromRoleMailbox(db, userID, roleID)
	if err != nil {
		t.Fatalf("UnassignUserFromRoleMailbox failed: %v", err)
	}

	var isActive bool
	err = db.QueryRow("SELECT is_active FROM user_role_assignments WHERE user_id = ? AND role_mailbox_id = ?", userID, roleID).Scan(&isActive)
	if err != nil {
		t.Fatalf("Failed to retrieve assignment: %v", err)
	}

	if isActive {
		t.Error("Expected assignment to be inactive after unassignment")
	}
}

func TestGetUserRoleAssignments(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)
	adminID, _ := CreateUser(db, "admin", domainID)

	role1ID, _ := CreateRoleMailbox(db, "support@example.com", domainID, "Support mailbox")
	role2ID, _ := CreateRoleMailbox(db, "sales@example.com", domainID, "Sales mailbox")

	_ = AssignUserToRoleMailbox(db, userID, role1ID, adminID)
	_ = AssignUserToRoleMailbox(db, userID, role2ID, adminID)

	roleIDs, err := GetUserRoleAssignments(db, userID)
	if err != nil {
		t.Fatalf("GetUserRoleAssignments failed: %v", err)
	}

	if len(roleIDs) != 2 {
		t.Errorf("Expected 2 role assignments, got %d", len(roleIDs))
	}

	expectedRoleIDs := []int64{role1ID, role2ID}
	for i, expected := range expectedRoleIDs {
		if roleIDs[i] != expected {
			t.Errorf("Expected role ID %d at index %d, got %d", expected, i, roleIDs[i])
		}
	}
}

func TestGetRoleMailboxAssignedUser(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)
	adminID, _ := CreateUser(db, "admin", domainID)
	roleID, _ := CreateRoleMailbox(db, "support@example.com", domainID, "Support mailbox")

	_ = AssignUserToRoleMailbox(db, userID, roleID, adminID)

	assignedUserID, err := GetRoleMailboxAssignedUser(db, roleID)
	if err != nil {
		t.Fatalf("GetRoleMailboxAssignedUser failed: %v", err)
	}

	if assignedUserID != userID {
		t.Errorf("Expected user ID %d, got %d", userID, assignedUserID)
	}
}

func TestGetRoleMailboxAssignedUser_NoAssignment(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	roleID, _ := CreateRoleMailbox(db, "support@example.com", domainID, "Support mailbox")

	_, err := GetRoleMailboxAssignedUser(db, roleID)
	if err == nil {
		t.Error("Expected error when getting assigned user for unassigned role mailbox")
	}
	if !strings.Contains(err.Error(), "no active user assigned") {
		t.Errorf("Expected 'no active user assigned' error, got: %v", err)
	}
}

func TestIsUserAssignedToRoleMailbox(t *testing.T) {
	db := setupTestDB(t)
	defer func() { _ = db.Close() }()

	domainID, _ := CreateDomain(db, "example.com")
	userID, _ := CreateUser(db, "testuser", domainID)
	adminID, _ := CreateUser(db, "admin", domainID)
	roleID, _ := CreateRoleMailbox(db, "support@example.com", domainID, "Support mailbox")

	assigned, err := IsUserAssignedToRoleMailbox(db, userID, roleID)
	if err != nil {
		t.Fatalf("IsUserAssignedToRoleMailbox failed: %v", err)
	}
	if assigned {
		t.Error("User should not be assigned yet")
	}

	_ = AssignUserToRoleMailbox(db, userID, roleID, adminID)

	assigned, err = IsUserAssignedToRoleMailbox(db, userID, roleID)
	if err != nil {
		t.Fatalf("IsUserAssignedToRoleMailbox failed after assignment: %v", err)
	}
	if !assigned {
		t.Error("User should be assigned after assignment")
	}
}
