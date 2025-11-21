package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewDBManager(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "db_manager_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	manager, err := NewDBManager(tmpDir)
	if err != nil {
		t.Fatalf("NewDBManager failed: %v", err)
	}
	defer func() { _ = manager.Close() }()

	if manager == nil {
		t.Fatal("Expected non-nil DBManager")
	}

	if manager.sharedDB == nil {
		t.Error("Expected non-nil shared database")
	}

	if manager.basePath != tmpDir {
		t.Errorf("Expected base path %s, got %s", tmpDir, manager.basePath)
	}
}

func TestNewDBManager_InvalidPath(t *testing.T) {
	_, err := NewDBManager("/invalid/nonexistent/path")
	if err == nil {
		t.Error("Expected error for invalid path")
	}
}

func TestGetSharedDB(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "db_manager_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	manager, err := NewDBManager(tmpDir)
	if err != nil {
		t.Fatalf("NewDBManager failed: %v", err)
	}
	defer func() { _ = manager.Close() }()

	sharedDB := manager.GetSharedDB()
	if sharedDB == nil {
		t.Fatal("Expected non-nil shared database")
	}

	var count int
	err = sharedDB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='domains'").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query domains table: %v", err)
	}

	if count != 1 {
		t.Error("Expected domains table to exist in shared database")
	}
}

func TestGetUserDB(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "db_manager_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	manager, err := NewDBManager(tmpDir)
	if err != nil {
		t.Fatalf("NewDBManager failed: %v", err)
	}
	defer func() { _ = manager.Close() }()

	userID := int64(123)
	userDB, err := manager.GetUserDB(userID)
	if err != nil {
		t.Fatalf("GetUserDB failed: %v", err)
	}

	if userDB == nil {
		t.Fatal("Expected non-nil user database")
	}

	dbPath := filepath.Join(tmpDir, "user_db_123.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Expected user database file to be created")
	}

	var count int
	err = userDB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='mailboxes'").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query mailboxes table: %v", err)
	}

	if count != 1 {
		t.Error("Expected mailboxes table to exist in user database")
	}
}

func TestGetUserDB_Caching(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "db_manager_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	manager, err := NewDBManager(tmpDir)
	if err != nil {
		t.Fatalf("NewDBManager failed: %v", err)
	}
	defer func() { _ = manager.Close() }()

	userID := int64(123)
	userDB1, err := manager.GetUserDB(userID)
	if err != nil {
		t.Fatalf("First GetUserDB failed: %v", err)
	}

	userDB2, err := manager.GetUserDB(userID)
	if err != nil {
		t.Fatalf("Second GetUserDB failed: %v", err)
	}

	if userDB1 != userDB2 {
		t.Error("Expected same database connection from cache")
	}

	if len(manager.userDBCache) != 1 {
		t.Errorf("Expected 1 cached user database, got %d", len(manager.userDBCache))
	}
}

func TestGetUserDB_DefaultMailboxes(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "db_manager_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	manager, err := NewDBManager(tmpDir)
	if err != nil {
		t.Fatalf("NewDBManager failed: %v", err)
	}
	defer func() { _ = manager.Close() }()

	userID := int64(123)
	userDB, err := manager.GetUserDB(userID)
	if err != nil {
		t.Fatalf("GetUserDB failed: %v", err)
	}

	expectedMailboxes := []string{"INBOX", "Sent", "Drafts", "Trash"}
	for _, mailboxName := range expectedMailboxes {
		var count int
		err = userDB.QueryRow("SELECT COUNT(*) FROM mailboxes WHERE name = ?", mailboxName).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to query mailbox %s: %v", mailboxName, err)
		}

		if count != 1 {
			t.Errorf("Expected mailbox %s to be created by default", mailboxName)
		}
	}
}

func TestGetRoleMailboxDB(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "db_manager_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	manager, err := NewDBManager(tmpDir)
	if err != nil {
		t.Fatalf("NewDBManager failed: %v", err)
	}
	defer func() { _ = manager.Close() }()

	roleID := int64(456)
	roleDB, err := manager.GetRoleMailboxDB(roleID)
	if err != nil {
		t.Fatalf("GetRoleMailboxDB failed: %v", err)
	}

	if roleDB == nil {
		t.Fatal("Expected non-nil role mailbox database")
	}

	dbPath := filepath.Join(tmpDir, "role_db_456.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Expected role mailbox database file to be created")
	}

	var count int
	err = roleDB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='mailboxes'").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query mailboxes table: %v", err)
	}

	if count != 1 {
		t.Error("Expected mailboxes table to exist in role mailbox database")
	}
}

func TestGetRoleMailboxDB_Caching(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "db_manager_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	manager, err := NewDBManager(tmpDir)
	if err != nil {
		t.Fatalf("NewDBManager failed: %v", err)
	}
	defer func() { _ = manager.Close() }()

	roleID := int64(456)
	roleDB1, err := manager.GetRoleMailboxDB(roleID)
	if err != nil {
		t.Fatalf("First GetRoleMailboxDB failed: %v", err)
	}

	roleDB2, err := manager.GetRoleMailboxDB(roleID)
	if err != nil {
		t.Fatalf("Second GetRoleMailboxDB failed: %v", err)
	}

	if roleDB1 != roleDB2 {
		t.Error("Expected same database connection from cache")
	}

	if len(manager.roleDBCache) != 1 {
		t.Errorf("Expected 1 cached role mailbox database, got %d", len(manager.roleDBCache))
	}
}

func TestClose(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "db_manager_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	manager, err := NewDBManager(tmpDir)
	if err != nil {
		t.Fatalf("NewDBManager failed: %v", err)
	}

	_, _ = manager.GetUserDB(1)
	_, _ = manager.GetUserDB(2)
	_, _ = manager.GetRoleMailboxDB(1)

	err = manager.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if len(manager.userDBCache) != 0 {
		t.Errorf("Expected empty user DB cache after close, got %d entries", len(manager.userDBCache))
	}

	if len(manager.roleDBCache) != 0 {
		t.Errorf("Expected empty role DB cache after close, got %d entries", len(manager.roleDBCache))
	}
}

func TestClose_MultipleUserDBs(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "db_manager_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	manager, err := NewDBManager(tmpDir)
	if err != nil {
		t.Fatalf("NewDBManager failed: %v", err)
	}

	for i := int64(1); i <= 5; i++ {
		_, err := manager.GetUserDB(i)
		if err != nil {
			t.Fatalf("GetUserDB(%d) failed: %v", i, err)
		}
	}

	if len(manager.userDBCache) != 5 {
		t.Errorf("Expected 5 cached user databases, got %d", len(manager.userDBCache))
	}

	err = manager.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if len(manager.userDBCache) != 0 {
		t.Errorf("Expected empty user DB cache after close, got %d entries", len(manager.userDBCache))
	}
}

func TestSharedDBTables(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "db_manager_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	manager, err := NewDBManager(tmpDir)
	if err != nil {
		t.Fatalf("NewDBManager failed: %v", err)
	}
	defer func() { _ = manager.Close() }()

	sharedDB := manager.GetSharedDB()

	expectedTables := []string{"domains", "users", "role_mailboxes", "user_role_assignments"}
	for _, tableName := range expectedTables {
		var count int
		err = sharedDB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", tableName).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to query table %s: %v", tableName, err)
		}

		if count != 1 {
			t.Errorf("Expected table %s to exist in shared database", tableName)
		}
	}
}

func TestUserDBTables(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "db_manager_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	manager, err := NewDBManager(tmpDir)
	if err != nil {
		t.Fatalf("NewDBManager failed: %v", err)
	}
	defer func() { _ = manager.Close() }()

	userDB, err := manager.GetUserDB(1)
	if err != nil {
		t.Fatalf("GetUserDB failed: %v", err)
	}

	expectedTables := []string{
		"blobs", "mailboxes", "aliases", "messages",
		"subscriptions", "addresses", "message_parts",
		"deliveries", "message_mailbox", "message_headers",
		"outbound_queue",
	}
	for _, tableName := range expectedTables {
		var count int
		err = userDB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", tableName).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to query table %s: %v", tableName, err)
		}

		if count != 1 {
			t.Errorf("Expected table %s to exist in user database", tableName)
		}
	}
}

func TestSharedDBIndexes(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "db_manager_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	manager, err := NewDBManager(tmpDir)
	if err != nil {
		t.Fatalf("NewDBManager failed: %v", err)
	}
	defer func() { _ = manager.Close() }()

	sharedDB := manager.GetSharedDB()

	expectedIndexes := []string{
		"idx_users_username_domain",
		"idx_users_domain",
		"idx_role_mailboxes_domain",
		"idx_role_mailboxes_email",
		"idx_role_assignments_user",
		"idx_role_assignments_role",
		"idx_role_assignments_active",
	}

	for _, indexName := range expectedIndexes {
		var count int
		err = sharedDB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?", indexName).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to query index %s: %v", indexName, err)
		}

		if count != 1 {
			t.Errorf("Expected index %s to exist in shared database", indexName)
		}
	}
}

func TestUserDBIndexes(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "db_manager_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	manager, err := NewDBManager(tmpDir)
	if err != nil {
		t.Fatalf("NewDBManager failed: %v", err)
	}
	defer func() { _ = manager.Close() }()

	userDB, err := manager.GetUserDB(1)
	if err != nil {
		t.Fatalf("GetUserDB failed: %v", err)
	}

	expectedIndexes := []string{
		"idx_mailboxes_user",
		"idx_mailboxes_parent",
		"idx_messages_date",
		"idx_messages_thread",
		"idx_addresses_message",
		"idx_addresses_email",
		"idx_message_parts_message",
		"idx_message_parts_blob",
		"idx_message_mailbox_mailbox",
		"idx_message_mailbox_message",
		"idx_message_mailbox_uid",
		"idx_message_headers_message",
		"idx_blobs_hash",
		"idx_deliveries_message",
		"idx_deliveries_status",
		"idx_outbound_status",
		"idx_subscriptions_user",
	}

	for _, indexName := range expectedIndexes {
		var count int
		err = userDB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?", indexName).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to query index %s: %v", indexName, err)
		}

		if count != 1 {
			t.Errorf("Expected index %s to exist in user database", indexName)
		}
	}
}

func TestGetUserDB_ExistingDatabase(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "db_manager_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	manager1, err := NewDBManager(tmpDir)
	if err != nil {
		t.Fatalf("First NewDBManager failed: %v", err)
	}

	userID := int64(123)
	userDB1, err := manager1.GetUserDB(userID)
	if err != nil {
		t.Fatalf("First GetUserDB failed: %v", err)
	}

	_, err = CreateMailboxPerUser(userDB1, userID, "CustomMailbox", "")
	if err != nil {
		t.Fatalf("Failed to create custom mailbox: %v", err)
	}

	_ = manager1.Close()

	manager2, err := NewDBManager(tmpDir)
	if err != nil {
		t.Fatalf("Second NewDBManager failed: %v", err)
	}
	defer func() { _ = manager2.Close() }()

	userDB2, err := manager2.GetUserDB(userID)
	if err != nil {
		t.Fatalf("Second GetUserDB failed: %v", err)
	}

	var count int
	err = userDB2.QueryRow("SELECT COUNT(*) FROM mailboxes WHERE name = ?", "CustomMailbox").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query custom mailbox: %v", err)
	}

	if count != 1 {
		t.Error("Expected custom mailbox to exist in reopened database")
	}
}

func TestForeignKeyConstraints(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "db_manager_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	manager, err := NewDBManager(tmpDir)
	if err != nil {
		t.Fatalf("NewDBManager failed: %v", err)
	}
	defer func() { _ = manager.Close() }()

	sharedDB := manager.GetSharedDB()

	var fkEnabled int
	err = sharedDB.QueryRow("PRAGMA foreign_keys").Scan(&fkEnabled)
	if err != nil {
		t.Fatalf("Failed to check foreign keys on shared DB: %v", err)
	}

	if fkEnabled != 1 {
		t.Error("Expected foreign keys to be enabled on shared database")
	}

	userDB, err := manager.GetUserDB(1)
	if err != nil {
		t.Fatalf("GetUserDB failed: %v", err)
	}

	err = userDB.QueryRow("PRAGMA foreign_keys").Scan(&fkEnabled)
	if err != nil {
		t.Fatalf("Failed to check foreign keys on user DB: %v", err)
	}

	if fkEnabled != 1 {
		t.Error("Expected foreign keys to be enabled on user database")
	}
}
