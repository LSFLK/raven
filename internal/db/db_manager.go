package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

// DBManager manages database connections for shared and per-user databases
type DBManager struct {
	basePath      string
	sharedDB      *sql.DB
	userDBCache   map[int64]*sql.DB
	roleDBCache   map[int64]*sql.DB
	cacheMutex    sync.RWMutex
}

// NewDBManager creates a new database manager
func NewDBManager(basePath string) (*DBManager, error) {
	// Create base directory if it doesn't exist
	if err := os.MkdirAll(basePath, 0750); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %v", err)
	}

	manager := &DBManager{
		basePath:    basePath,
		userDBCache: make(map[int64]*sql.DB),
		roleDBCache: make(map[int64]*sql.DB),
	}

	// Initialize shared database
	if err := manager.initSharedDB(); err != nil {
		return nil, fmt.Errorf("failed to initialize shared database: %v", err)
	}

	return manager, nil
}

// GetSharedDB returns the shared database connection
func (m *DBManager) GetSharedDB() *sql.DB {
	return m.sharedDB
}

// GetUserDB returns a database connection for a specific user
func (m *DBManager) GetUserDB(userID int64) (*sql.DB, error) {
	// Check cache first
	m.cacheMutex.RLock()
	if db, exists := m.userDBCache[userID]; exists {
		m.cacheMutex.RUnlock()
		return db, nil
	}
	m.cacheMutex.RUnlock()

	// Create or open user database
	m.cacheMutex.Lock()
	defer m.cacheMutex.Unlock()

	// Double-check after acquiring write lock
	if db, exists := m.userDBCache[userID]; exists {
		return db, nil
	}

	dbPath := m.getUserDBPath(userID)

	// Check if database file exists
	exists := false
	if _, err := os.Stat(dbPath); err == nil {
		exists = true
	}

	// Open database
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open user database: %v", err)
	}

	// Enable foreign key constraints
	if _, err = db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %v", err)
	}

	// Initialize schema if this is a new database
	if !exists {
		if err := m.initUserDB(db, userID); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("failed to initialize user database: %v", err)
		}
	}

	// Cache the connection
	m.userDBCache[userID] = db

	return db, nil
}

// GetRoleMailboxDB returns a database connection for a specific role mailbox
func (m *DBManager) GetRoleMailboxDB(roleMailboxID int64) (*sql.DB, error) {
	// Check cache first
	m.cacheMutex.RLock()
	if db, exists := m.roleDBCache[roleMailboxID]; exists {
		m.cacheMutex.RUnlock()
		return db, nil
	}
	m.cacheMutex.RUnlock()

	// Create or open role mailbox database
	m.cacheMutex.Lock()
	defer m.cacheMutex.Unlock()

	// Double-check after acquiring write lock
	if db, exists := m.roleDBCache[roleMailboxID]; exists {
		return db, nil
	}

	dbPath := m.getRoleMailboxDBPath(roleMailboxID)

	// Check if database file exists
	exists := false
	if _, err := os.Stat(dbPath); err == nil {
		exists = true
	}

	// Open database
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open role mailbox database: %v", err)
	}

	// Enable foreign key constraints
	if _, err = db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %v", err)
	}

	// Initialize schema if this is a new database (use userID 0 for role mailbox)
	if !exists {
		if err := m.initUserDB(db, 0); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("failed to initialize role mailbox database: %v", err)
		}
	}

	// Cache the connection
	m.roleDBCache[roleMailboxID] = db

	return db, nil
}

// initSharedDB initializes the shared database
func (m *DBManager) initSharedDB() error {
	sharedPath := filepath.Join(m.basePath, "shared.db")

	db, err := sql.Open("sqlite3", sharedPath)
	if err != nil {
		return err
	}

	// Enable foreign key constraints
	if _, err = db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		_ = db.Close()
		return err
	}

	// Create shared tables
	if err := createDomainsTable(db); err != nil {
		_ = db.Close()
		return fmt.Errorf("failed to create domains table: %v", err)
	}

	if err := createUsersTable(db); err != nil {
		_ = db.Close()
		return fmt.Errorf("failed to create users table: %v", err)
	}

	if err := createRoleMailboxesTable(db); err != nil {
		_ = db.Close()
		return fmt.Errorf("failed to create role_mailboxes table: %v", err)
	}

	if err := createUserRoleAssignmentsTable(db); err != nil {
		_ = db.Close()
		return fmt.Errorf("failed to create user_role_assignments table: %v", err)
	}

	// Create indexes for shared tables
	if err := createSharedIndexes(db); err != nil {
		_ = db.Close()
		return fmt.Errorf("failed to create shared indexes: %v", err)
	}

	m.sharedDB = db
	return nil
}

// initUserDB initializes a per-user database
func (m *DBManager) initUserDB(db *sql.DB, userID int64) error {
	// Create user tables
	if err := createBlobsTable(db); err != nil {
		return fmt.Errorf("failed to create blobs table: %v", err)
	}

	if err := createMailboxesTablePerUser(db); err != nil {
		return fmt.Errorf("failed to create mailboxes table: %v", err)
	}

	if err := createAliasesTablePerUser(db); err != nil {
		return fmt.Errorf("failed to create aliases table: %v", err)
	}

	if err := createMessagesTable(db); err != nil {
		return fmt.Errorf("failed to create messages table: %v", err)
	}

	if err := createSubscriptionsTablePerUser(db); err != nil {
		return fmt.Errorf("failed to create subscriptions table: %v", err)
	}

	if err := createAddressesTable(db); err != nil {
		return fmt.Errorf("failed to create addresses table: %v", err)
	}

	if err := createMessagePartsTable(db); err != nil {
		return fmt.Errorf("failed to create message_parts table: %v", err)
	}

	if err := createDeliveriesTablePerUser(db); err != nil {
		return fmt.Errorf("failed to create deliveries table: %v", err)
	}

	if err := createMessageMailboxTable(db); err != nil {
		return fmt.Errorf("failed to create message_mailbox table: %v", err)
	}

	if err := createMessageHeadersTable(db); err != nil {
		return fmt.Errorf("failed to create message_headers table: %v", err)
	}

	if err := createOutboundQueueTablePerUser(db); err != nil {
		return fmt.Errorf("failed to create outbound_queue table: %v", err)
	}

	// Create indexes for user tables
	if err := createUserIndexes(db); err != nil {
		return fmt.Errorf("failed to create user indexes: %v", err)
	}

	// Create default mailboxes
	if err := createDefaultMailboxes(db, userID); err != nil {
		return fmt.Errorf("failed to create default mailboxes: %v", err)
	}

	return nil
}

// getUserDBPath returns the file path for a user's database
func (m *DBManager) getUserDBPath(userID int64) string {
	return filepath.Join(m.basePath, fmt.Sprintf("user_db_%d.db", userID))
}

// getRoleMailboxDBPath returns the file path for a role mailbox's database
func (m *DBManager) getRoleMailboxDBPath(roleMailboxID int64) string {
	return filepath.Join(m.basePath, fmt.Sprintf("role_db_%d.db", roleMailboxID))
}

// Close closes all database connections
func (m *DBManager) Close() error {
	var lastErr error

	// Close shared database
	if m.sharedDB != nil {
		if err := m.sharedDB.Close(); err != nil {
			lastErr = err
		}
	}

	// Close all user databases
	m.cacheMutex.Lock()
	defer m.cacheMutex.Unlock()

	for userID, db := range m.userDBCache {
		if err := db.Close(); err != nil {
			lastErr = err
		}
		delete(m.userDBCache, userID)
	}

	// Close all role mailbox databases
	for roleID, db := range m.roleDBCache {
		if err := db.Close(); err != nil {
			lastErr = err
		}
		delete(m.roleDBCache, roleID)
	}

	return lastErr
}

// createDefaultMailboxes creates default mailboxes for a new user
func createDefaultMailboxes(db *sql.DB, userID int64) error {
	defaultMailboxes := []struct {
		name        string
		specialUse  string
	}{
		{"INBOX", "\\Inbox"},
		{"Sent", "\\Sent"},
		{"Drafts", "\\Drafts"},
		{"Trash", "\\Trash"},
	}

	for _, mbx := range defaultMailboxes {
		_, err := CreateMailboxPerUser(db, userID, mbx.name, mbx.specialUse)
		if err != nil {
			return fmt.Errorf("failed to create mailbox %s: %v", mbx.name, err)
		}
	}

	return nil
}

// createSharedIndexes creates indexes for shared database tables
func createSharedIndexes(db *sql.DB) error {
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_users_username_domain ON users(username, domain_id)",
		"CREATE INDEX IF NOT EXISTS idx_users_domain ON users(domain_id)",
		"CREATE INDEX IF NOT EXISTS idx_role_mailboxes_domain ON role_mailboxes(domain_id)",
		"CREATE INDEX IF NOT EXISTS idx_role_mailboxes_email ON role_mailboxes(email)",
		"CREATE INDEX IF NOT EXISTS idx_role_assignments_user ON user_role_assignments(user_id)",
		"CREATE INDEX IF NOT EXISTS idx_role_assignments_role ON user_role_assignments(role_mailbox_id)",
		"CREATE INDEX IF NOT EXISTS idx_role_assignments_active ON user_role_assignments(is_active)",
	}

	for _, idx := range indexes {
		if _, err := db.Exec(idx); err != nil {
			return fmt.Errorf("failed to create index: %v", err)
		}
	}

	return nil
}

// createUserIndexes creates indexes for per-user database tables
func createUserIndexes(db *sql.DB) error {
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_mailboxes_user ON mailboxes(user_id)",
		"CREATE INDEX IF NOT EXISTS idx_mailboxes_parent ON mailboxes(parent_id)",
		"CREATE INDEX IF NOT EXISTS idx_messages_date ON messages(date)",
		"CREATE INDEX IF NOT EXISTS idx_messages_thread ON messages(thread_id)",
		"CREATE INDEX IF NOT EXISTS idx_addresses_message ON addresses(message_id)",
		"CREATE INDEX IF NOT EXISTS idx_addresses_email ON addresses(email)",
		"CREATE INDEX IF NOT EXISTS idx_message_parts_message ON message_parts(message_id)",
		"CREATE INDEX IF NOT EXISTS idx_message_parts_blob ON message_parts(blob_id)",
		"CREATE INDEX IF NOT EXISTS idx_message_mailbox_mailbox ON message_mailbox(mailbox_id)",
		"CREATE INDEX IF NOT EXISTS idx_message_mailbox_message ON message_mailbox(message_id)",
		"CREATE INDEX IF NOT EXISTS idx_message_mailbox_uid ON message_mailbox(mailbox_id, uid)",
		"CREATE INDEX IF NOT EXISTS idx_message_headers_message ON message_headers(message_id)",
		"CREATE INDEX IF NOT EXISTS idx_blobs_hash ON blobs(sha256_hash)",
		"CREATE INDEX IF NOT EXISTS idx_deliveries_message ON deliveries(message_id)",
		"CREATE INDEX IF NOT EXISTS idx_deliveries_status ON deliveries(status)",
		"CREATE INDEX IF NOT EXISTS idx_outbound_status ON outbound_queue(status, next_retry_at)",
		"CREATE INDEX IF NOT EXISTS idx_subscriptions_user ON subscriptions(user_id)",
	}

	for _, idx := range indexes {
		if _, err := db.Exec(idx); err != nil {
			return fmt.Errorf("failed to create index: %v", err)
		}
	}

	return nil
}
