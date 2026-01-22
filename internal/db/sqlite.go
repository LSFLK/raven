package db

import (
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"mime/quotedprintable"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// InitDB initializes the database with the new normalized schema
func InitDB(file string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", file)
	if err != nil {
		return nil, err
	}

	// Enable foreign key constraints
	if _, err = db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, err
	}

	// Create all tables
	if err = createDomainsTable(db); err != nil {
		return nil, fmt.Errorf("failed to create domains table: %v", err)
	}

	if err = createUsersTable(db); err != nil {
		return nil, fmt.Errorf("failed to create users table: %v", err)
	}

	if err = createRoleMailboxesTable(db); err != nil {
		return nil, fmt.Errorf("failed to create role_mailboxes table: %v", err)
	}

	if err = createUserRoleAssignmentsTable(db); err != nil {
		return nil, fmt.Errorf("failed to create user_role_assignments table: %v", err)
	}

	if err = createBlobsTable(db); err != nil {
		return nil, fmt.Errorf("failed to create blobs table: %v", err)
	}

	if err = createMailboxesTable(db); err != nil {
		return nil, fmt.Errorf("failed to create mailboxes table: %v", err)
	}

	if err = createAliasesTable(db); err != nil {
		return nil, fmt.Errorf("failed to create aliases table: %v", err)
	}

	if err = createMessagesTable(db); err != nil {
		return nil, fmt.Errorf("failed to create messages table: %v", err)
	}

	if err = createSubscriptionsTable(db); err != nil {
		return nil, fmt.Errorf("failed to create subscriptions table: %v", err)
	}

	if err = createAddressesTable(db); err != nil {
		return nil, fmt.Errorf("failed to create addresses table: %v", err)
	}

	if err = createMessagePartsTablePerUser(db); err != nil {
		return nil, fmt.Errorf("failed to create message_parts table: %v", err)
	}

	if err = createDeliveriesTable(db); err != nil {
		return nil, fmt.Errorf("failed to create deliveries table: %v", err)
	}

	if err = createMessageMailboxTable(db); err != nil {
		return nil, fmt.Errorf("failed to create message_mailbox table: %v", err)
	}

	if err = createMessageHeadersTable(db); err != nil {
		return nil, fmt.Errorf("failed to create message_headers table: %v", err)
	}

	if err = createOutboundQueueTable(db); err != nil {
		return nil, fmt.Errorf("failed to create outbound_queue table: %v", err)
	}

	// Create indexes
	if err = createIndexes(db); err != nil {
		return nil, fmt.Errorf("failed to create indexes: %v", err)
	}

	return db, nil
}

// Table creation functions

func createDomainsTable(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS domains (
		id INTEGER PRIMARY KEY,
		domain TEXT NOT NULL UNIQUE,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		enabled BOOLEAN DEFAULT TRUE
	);
	`
	_, err := db.Exec(schema)
	return err
}

func createUsersTable(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY,
		username TEXT NOT NULL,
		domain_id INTEGER NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		enabled BOOLEAN DEFAULT TRUE,
		password_initialized BOOLEAN DEFAULT FALSE,
		FOREIGN KEY (domain_id) REFERENCES domains(id),
		UNIQUE(username, domain_id)
	);
	`
	_, err := db.Exec(schema)
	return err
}

func createRoleMailboxesTable(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS role_mailboxes (
		id INTEGER PRIMARY KEY,
		email TEXT NOT NULL,
		domain_id INTEGER NOT NULL,
		description TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		enabled BOOLEAN DEFAULT TRUE,
		FOREIGN KEY (domain_id) REFERENCES domains(id),
		UNIQUE(email)
	);
	`
	_, err := db.Exec(schema)
	return err
}

func createUserRoleAssignmentsTable(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS user_role_assignments (
		id INTEGER PRIMARY KEY,
		user_id INTEGER NOT NULL,
		role_mailbox_id INTEGER NOT NULL,
		assigned_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		assigned_by INTEGER,
		is_active BOOLEAN DEFAULT TRUE,
		FOREIGN KEY (user_id) REFERENCES users(id),
		FOREIGN KEY (role_mailbox_id) REFERENCES role_mailboxes(id),
		FOREIGN KEY (assigned_by) REFERENCES users(id),
		UNIQUE(user_id, role_mailbox_id)
	);
	`
	_, err := db.Exec(schema)
	return err
}

func createBlobsTable(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS blobs (
		id INTEGER PRIMARY KEY,
		sha256_hash TEXT NOT NULL UNIQUE,
		size_bytes INTEGER NOT NULL,
		content TEXT,
		s3_blob_id TEXT,
		storage_type TEXT DEFAULT 'local',
		reference_count INTEGER DEFAULT 0,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := db.Exec(schema)
	return err
}

func createMailboxesTable(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS mailboxes (
		id INTEGER PRIMARY KEY,
		user_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		parent_id INTEGER,
		uid_validity INTEGER NOT NULL,
		uid_next INTEGER NOT NULL,
		special_use TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id),
		FOREIGN KEY (parent_id) REFERENCES mailboxes(id),
		UNIQUE(user_id, name)
	);
	`
	_, err := db.Exec(schema)
	return err
}

func createAliasesTable(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS aliases (
		id INTEGER PRIMARY KEY,
		alias TEXT NOT NULL,
		domain_id INTEGER NOT NULL,
		destination_user_id INTEGER NOT NULL,
		enabled BOOLEAN DEFAULT TRUE,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (domain_id) REFERENCES domains(id),
		FOREIGN KEY (destination_user_id) REFERENCES users(id),
		UNIQUE(alias, domain_id)
	);
	`
	_, err := db.Exec(schema)
	return err
}

func createMessagesTable(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY,
		in_reply_to TEXT,
		references_header TEXT,
		subject TEXT,
		date TIMESTAMP,
		size_bytes INTEGER NOT NULL,
		received_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		thread_id INTEGER
	);
	`
	_, err := db.Exec(schema)
	return err
}

func createSubscriptionsTable(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS subscriptions (
		id INTEGER PRIMARY KEY,
		user_id INTEGER NOT NULL,
		mailbox_name TEXT NOT NULL,
		subscribed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id),
		UNIQUE(user_id, mailbox_name)
	);
	`
	_, err := db.Exec(schema)
	return err
}

func createAddressesTable(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS addresses (
		id INTEGER PRIMARY KEY,
		message_id INTEGER NOT NULL,
		address_type TEXT NOT NULL,
		name TEXT,
		email TEXT NOT NULL,
		sequence INTEGER NOT NULL,
		FOREIGN KEY (message_id) REFERENCES messages(id)
	);
	`
	_, err := db.Exec(schema)
	return err
}

// createMessagePartsTablePerUser creates the message_parts table for per-user databases
// without the foreign key to blobs (since blobs are in the shared database)
func createMessagePartsTablePerUser(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS message_parts (
		id INTEGER PRIMARY KEY,
		message_id INTEGER NOT NULL,
		part_number INTEGER NOT NULL,
		parent_part_id INTEGER,
		content_type TEXT NOT NULL,
		content_disposition TEXT,
		content_transfer_encoding TEXT,
		charset TEXT,
		filename TEXT,
		content_id TEXT,
		blob_id INTEGER,
		text_content TEXT,
		size_bytes INTEGER NOT NULL,
		FOREIGN KEY (message_id) REFERENCES messages(id),
		FOREIGN KEY (parent_part_id) REFERENCES message_parts(id)
	);
	`
	if _, err := db.Exec(schema); err != nil {
		return err
	}

	return nil
}

func createDeliveriesTable(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS deliveries (
		id INTEGER PRIMARY KEY,
		message_id INTEGER NOT NULL,
		recipient TEXT NOT NULL,
		sender TEXT NOT NULL,
		status TEXT NOT NULL,
		user_id INTEGER,
		delivered_at TIMESTAMP,
		smtp_response TEXT,
		FOREIGN KEY (message_id) REFERENCES messages(id),
		FOREIGN KEY (user_id) REFERENCES users(id)
	);
	`
	_, err := db.Exec(schema)
	return err
}

func createMessageMailboxTable(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS message_mailbox (
		id INTEGER PRIMARY KEY,
		message_id INTEGER NOT NULL,
		mailbox_id INTEGER NOT NULL,
		uid INTEGER NOT NULL,
		flags TEXT,
		internal_date TIMESTAMP NOT NULL,
		added_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (message_id) REFERENCES messages(id),
		FOREIGN KEY (mailbox_id) REFERENCES mailboxes(id),
		UNIQUE(mailbox_id, uid)
	);
	`
	_, err := db.Exec(schema)
	return err
}

func createMessageHeadersTable(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS message_headers (
		id INTEGER PRIMARY KEY,
		message_id INTEGER NOT NULL,
		header_name TEXT NOT NULL,
		header_value TEXT NOT NULL,
		sequence INTEGER NOT NULL,
		FOREIGN KEY (message_id) REFERENCES messages(id)
	);
	`
	_, err := db.Exec(schema)
	return err
}

func createOutboundQueueTable(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS outbound_queue (
		id INTEGER PRIMARY KEY,
		message_id INTEGER NOT NULL,
		sender TEXT NOT NULL,
		recipient TEXT NOT NULL,
		retry_count INTEGER DEFAULT 0,
		max_retries INTEGER DEFAULT 5,
		next_retry_at TIMESTAMP,
		status TEXT NOT NULL,
		last_error TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		sent_at TIMESTAMP,
		FOREIGN KEY (message_id) REFERENCES messages(id)
	);
	`
	_, err := db.Exec(schema)
	return err
}

func createIndexes(db *sql.DB) error {
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_users_username_domain ON users(username, domain_id)",
		"CREATE INDEX IF NOT EXISTS idx_users_domain ON users(domain_id)",
		"CREATE INDEX IF NOT EXISTS idx_role_mailboxes_domain ON role_mailboxes(domain_id)",
		"CREATE INDEX IF NOT EXISTS idx_role_mailboxes_email ON role_mailboxes(email)",
		"CREATE INDEX IF NOT EXISTS idx_role_assignments_user ON user_role_assignments(user_id)",
		"CREATE INDEX IF NOT EXISTS idx_role_assignments_role ON user_role_assignments(role_mailbox_id)",
		"CREATE INDEX IF NOT EXISTS idx_role_assignments_active ON user_role_assignments(is_active)",
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

// Domain management functions

func CreateDomain(db *sql.DB, domain string) (int64, error) {
	result, err := db.Exec("INSERT INTO domains (domain, enabled) VALUES (?, ?)", domain, true)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return 0, fmt.Errorf("domain already exists")
		}
		return 0, err
	}
	return result.LastInsertId()
}

func GetDomainByName(db *sql.DB, domain string) (int64, error) {
	var id int64
	err := db.QueryRow("SELECT id FROM domains WHERE domain = ? AND enabled = ?", domain, true).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("domain not found")
	}
	return id, err
}

func GetOrCreateDomain(db *sql.DB, domain string) (int64, error) {
	id, err := GetDomainByName(db, domain)
	if err == nil {
		return id, nil
	}
	return CreateDomain(db, domain)
}

// User management functions

func CreateUser(db *sql.DB, username string, domainID int64) (int64, error) {
	// By default create users with password_initialized = false so admin-provisioned accounts
	// cannot login until they initialize their password. Callers that create "trusted"
	// accounts for automated systems can explicitly set the flag with SetPasswordInitialized.
	result, err := db.Exec("INSERT INTO users (username, domain_id, enabled, password_initialized) VALUES (?, ?, ?, ?)", username, domainID, true, false)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return 0, fmt.Errorf("user already exists")
		}
		return 0, err
	}
	return result.LastInsertId()
}

func GetUserByUsername(db *sql.DB, username string, domainID int64) (int64, error) {
	var id int64
	err := db.QueryRow("SELECT id FROM users WHERE username = ? AND domain_id = ? AND enabled = ?", username, domainID, true).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("user not found")
	}
	return id, err
}

func GetUserByEmail(db *sql.DB, email string) (int64, error) {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid email format")
	}
	username, domain := parts[0], parts[1]

	domainID, err := GetDomainByName(db, domain)
	if err != nil {
		return 0, err
	}

	return GetUserByUsername(db, username, domainID)
}

func GetOrCreateUser(db *sql.DB, username string, domainID int64) (int64, error) {
	id, err := GetUserByUsername(db, username, domainID)
	if err == nil {
		return id, nil
	}
	return CreateUser(db, username, domainID)
}

func UserExists(db *sql.DB, username string, domainID int64) (bool, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM users WHERE username = ? AND domain_id = ? AND enabled = ?", username, domainID, true).Scan(&count)
	return count > 0, err
}

// Mailbox management functions

func CreateMailbox(db *sql.DB, userID int64, name string, specialUse string) (int64, error) {
	// Validate mailbox name
	if name == "" {
		return 0, fmt.Errorf("mailbox name cannot be empty")
	}

	// Generate UID validity (Unix timestamp)
	uidValidity := time.Now().Unix()

	// Insert mailbox record
	result, err := db.Exec(`
		INSERT INTO mailboxes (user_id, name, uid_validity, uid_next, special_use)
		VALUES (?, ?, ?, ?, ?)
	`, userID, name, uidValidity, 1, specialUse)

	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return 0, fmt.Errorf("mailbox already exists")
		}
		return 0, err
	}

	return result.LastInsertId()
}

func GetMailboxByName(db *sql.DB, userID int64, name string) (int64, error) {
	var id int64
	err := db.QueryRow("SELECT id FROM mailboxes WHERE user_id = ? AND name = ?", userID, name).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("mailbox not found")
	}
	return id, err
}

func GetMailboxInfo(db *sql.DB, mailboxID int64) (uidValidity, uidNext int64, err error) {
	err = db.QueryRow("SELECT uid_validity, uid_next FROM mailboxes WHERE id = ?", mailboxID).Scan(&uidValidity, &uidNext)
	return
}

func IncrementUIDNext(db *sql.DB, mailboxID int64) (int64, error) {
	var currentUID int64
	err := db.QueryRow("SELECT uid_next FROM mailboxes WHERE id = ?", mailboxID).Scan(&currentUID)
	if err != nil {
		return 0, err
	}

	newUID := currentUID
	_, err = db.Exec("UPDATE mailboxes SET uid_next = uid_next + 1 WHERE id = ?", mailboxID)
	return newUID, err
}

func MailboxExists(db *sql.DB, userID int64, mailboxName string) (bool, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM mailboxes WHERE user_id = ? AND name = ?", userID, mailboxName).Scan(&count)
	return count > 0, err
}

func GetUserMailboxes(db *sql.DB, userID int64) ([]string, error) {
	rows, err := db.Query("SELECT name FROM mailboxes WHERE user_id = ? ORDER BY name", userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var mailboxes []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			mailboxes = append(mailboxes, name)
		}
	}

	return mailboxes, rows.Err()
}

func DeleteMailbox(db *sql.DB, userID int64, mailboxName string) error {
	// Cannot delete INBOX
	if strings.ToUpper(mailboxName) == "INBOX" {
		return fmt.Errorf("cannot delete INBOX")
	}

	mailboxID, err := GetMailboxByName(db, userID, mailboxName)
	if err != nil {
		return fmt.Errorf("mailbox does not exist")
	}

	// Check for child mailboxes (both by parent_id and by hierarchical naming)
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM mailboxes WHERE parent_id = ?", mailboxID).Scan(&count)
	if err != nil {
		return err
	}

	if count > 0 {
		return fmt.Errorf("mailbox has inferior hierarchical names")
	}

	// Also check for hierarchical children by naming convention (mailboxName/*)
	hierarchyPattern := mailboxName + "/%"
	err = db.QueryRow("SELECT COUNT(*) FROM mailboxes WHERE user_id = ? AND name LIKE ?", userID, hierarchyPattern).Scan(&count)
	if err != nil {
		return err
	}

	if count > 0 {
		return fmt.Errorf("mailbox has inferior hierarchical names")
	}

	// Prevent deletion of default mailboxes (except via special operations)
	defaultMailboxes := []string{"Sent", "Drafts", "Trash"}
	for _, defaultMbx := range defaultMailboxes {
		if strings.EqualFold(mailboxName, defaultMbx) {
			return fmt.Errorf("cannot delete default mailbox %s", mailboxName)
		}
	}

	// Start transaction
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Delete message_mailbox entries
	_, err = tx.Exec("DELETE FROM message_mailbox WHERE mailbox_id = ?", mailboxID)
	if err != nil {
		return err
	}

	// Delete mailbox
	_, err = tx.Exec("DELETE FROM mailboxes WHERE id = ?", mailboxID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func RenameMailbox(db *sql.DB, userID int64, oldName, newName string) error {
	// Cannot rename TO INBOX
	if strings.ToUpper(newName) == "INBOX" {
		return fmt.Errorf("cannot rename to INBOX")
	}

	// Handle INBOX renaming (special case)
	if strings.ToUpper(oldName) == "INBOX" {
		return renameInbox(db, userID, newName)
	}

	// Check if source mailbox exists
	mailboxID, err := GetMailboxByName(db, userID, oldName)
	if err != nil {
		return fmt.Errorf("source mailbox does not exist")
	}

	// Check if destination mailbox already exists
	exists, err := MailboxExists(db, userID, newName)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("destination mailbox already exists")
	}

	// Create intermediate hierarchies if needed (RFC 3501 requirement)
	// For example, renaming to "baz/rag/zowie" should create "baz" and "baz/rag" if they don't exist
	if strings.Contains(newName, "/") {
		parts := strings.Split(newName, "/")
		for i := 0; i < len(parts)-1; i++ {
			parentPath := strings.Join(parts[:i+1], "/")
			exists, err := MailboxExists(db, userID, parentPath)
			if err != nil {
				return err
			}
			if !exists {
				// Create parent mailbox with no special use (it's just a hierarchy placeholder)
				_, err = CreateMailbox(db, userID, parentPath, "")
				if err != nil && !strings.Contains(err.Error(), "already exists") {
					return fmt.Errorf("failed to create parent hierarchy %s: %v", parentPath, err)
				}
			}
		}
	}

	// Start transaction
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Rename the mailbox
	_, err = tx.Exec("UPDATE mailboxes SET name = ? WHERE id = ?", newName, mailboxID)
	if err != nil {
		return err
	}

	// Rename all hierarchical children (mailboxes whose names start with "oldName/")
	// For example, renaming "foo" to "zap" should also rename "foo/bar" to "zap/bar"
	hierarchyPattern := oldName + "/%"
	rows, err := tx.Query("SELECT id, name FROM mailboxes WHERE user_id = ? AND name LIKE ?", userID, hierarchyPattern)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	type mailboxUpdate struct {
		id      int64
		newName string
	}
	var updates []mailboxUpdate

	for rows.Next() {
		var id int64
		var childName string
		if err := rows.Scan(&id, &childName); err != nil {
			return err
		}
		// Replace the old prefix with the new prefix
		newChildName := newName + childName[len(oldName):]
		updates = append(updates, mailboxUpdate{id: id, newName: newChildName})
	}
	_ = rows.Close()

	// Apply all updates
	for _, update := range updates {
		_, err = tx.Exec("UPDATE mailboxes SET name = ? WHERE id = ?", update.newName, update.id)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func renameInbox(db *sql.DB, userID int64, newName string) error {
	// Check if destination mailbox already exists
	exists, err := MailboxExists(db, userID, newName)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("destination mailbox already exists")
	}

	// Get INBOX mailbox ID
	inboxID, err := GetMailboxByName(db, userID, "INBOX")
	if err != nil {
		return err
	}

	// Create new mailbox
	newMailboxID, err := CreateMailbox(db, userID, newName, "")
	if err != nil {
		return err
	}

	// Move all messages from INBOX to new mailbox
	_, err = db.Exec(`
		UPDATE message_mailbox
		SET mailbox_id = ?
		WHERE mailbox_id = ?
	`, newMailboxID, inboxID)

	return err
}

// Blob management functions

// decodeContentForHashing decodes MIME content based on Content-Transfer-Encoding
// to normalize it before hashing. This ensures that the same binary content with
// different encodings (e.g., base64 with different line breaks) produces the same hash.
func decodeContentForHashing(content string, encoding string) ([]byte, error) {
	// Normalize encoding string (case-insensitive, trim whitespace)
	encoding = strings.ToLower(strings.TrimSpace(encoding))

	switch encoding {
	case "base64":
		// Decode base64 - this removes line breaks and other encoding artifacts
		decoded, err := base64.StdEncoding.DecodeString(content)
		if err != nil {
			return nil, fmt.Errorf("failed to decode base64: %w", err)
		}
		return decoded, nil

	case "quoted-printable":
		// Decode quoted-printable
		reader := quotedprintable.NewReader(strings.NewReader(content))
		decoded, err := io.ReadAll(reader)
		if err != nil {
			return nil, fmt.Errorf("failed to decode quoted-printable: %w", err)
		}
		return decoded, nil

	case "7bit", "8bit", "binary", "":
		// No decoding needed for these encodings - content is already in its final form
		return []byte(content), nil

	default:
		// Unknown encoding - treat as-is to avoid breaking existing functionality
		// Log a warning in production code if needed
		return []byte(content), nil
	}
}

// StoreBlobWithEncoding stores a blob with proper deduplication based on decoded content
func StoreBlobWithEncoding(db *sql.DB, content string, encoding string) (int64, error) {
	// Decode content before hashing to ensure same binary content produces same hash
	// regardless of encoding differences (e.g., base64 with different line breaks)
	decodedContent, err := decodeContentForHashing(content, encoding)
	if err != nil {
		// If decoding fails, fall back to hashing the original content
		// This maintains backward compatibility
		decodedContent = []byte(content)
	}

	// Calculate SHA256 hash of decoded (binary) content
	hash := sha256.Sum256(decodedContent)
	hashStr := hex.EncodeToString(hash[:])

	// Check if blob already exists (by hash of decoded content)
	var blobID int64
	err = db.QueryRow("SELECT id FROM blobs WHERE sha256_hash = ?", hashStr).Scan(&blobID)
	if err == nil {
		// Blob exists, increment reference count
		_, err = db.Exec("UPDATE blobs SET reference_count = reference_count + 1 WHERE id = ?", blobID)
		return blobID, err
	}

	// Create new blob (local storage) - store original encoded content
	result, err := db.Exec(`
		INSERT INTO blobs (sha256_hash, size_bytes, content, storage_type, reference_count)
		VALUES (?, ?, ?, 'local', ?)
	`, hashStr, len(content), content, 1)
	if err != nil {
		return 0, err
	}

	return result.LastInsertId()
}

// StoreBlobS3WithEncoding stores a blob reference with S3 blob ID with proper deduplication based on decoded content
func StoreBlobS3WithEncoding(db *sql.DB, content string, s3BlobID string, encoding string) (int64, error) {
	// Decode content before hashing to ensure same binary content produces same hash
	// regardless of encoding differences (e.g., base64 with different line breaks)
	decodedContent, err := decodeContentForHashing(content, encoding)
	if err != nil {
		// If decoding fails, fall back to hashing the original content
		// This maintains backward compatibility
		decodedContent = []byte(content)
	}

	// Calculate SHA256 hash of decoded (binary) content
	hash := sha256.Sum256(decodedContent)
	hashStr := hex.EncodeToString(hash[:])

	// Check if blob already exists (by hash of decoded content)
	var blobID int64
	err = db.QueryRow("SELECT id FROM blobs WHERE sha256_hash = ?", hashStr).Scan(&blobID)
	if err == nil {
		// Blob exists, increment reference count
		_, err = db.Exec("UPDATE blobs SET reference_count = reference_count + 1 WHERE id = ?", blobID)
		return blobID, err
	}

	// Create new blob (S3 storage) - store hash of decoded content but reference to encoded S3 object
	result, err := db.Exec(`
		INSERT INTO blobs (sha256_hash, size_bytes, s3_blob_id, storage_type, reference_count)
		VALUES (?, ?, ?, 's3', ?)
	`, hashStr, len(content), s3BlobID, 1)
	if err != nil {
		return 0, err
	}

	return result.LastInsertId()
}

func GetBlob(db *sql.DB, blobID int64) (string, error) {
	var content sql.NullString
	var storageType string
	err := db.QueryRow("SELECT content, storage_type FROM blobs WHERE id = ?", blobID).Scan(&content, &storageType)
	if err != nil {
		return "", err
	}

	if storageType == "local" && content.Valid {
		return content.String, nil
	}

	// For S3 storage, return empty string - caller should use GetBlobS3BlobID
	return "", nil
}

// GetBlobS3BlobID retrieves the S3 blob ID for a given blob
func GetBlobS3BlobID(db *sql.DB, blobID int64) (string, string, error) {
	var s3BlobID sql.NullString
	var storageType string
	err := db.QueryRow("SELECT s3_blob_id, storage_type FROM blobs WHERE id = ?", blobID).Scan(&s3BlobID, &storageType)
	if err != nil {
		return "", "", err
	}

	if storageType == "s3" && s3BlobID.Valid {
		return s3BlobID.String, storageType, nil
	}

	return "", storageType, nil
}

func DecrementBlobReference(db *sql.DB, blobID int64) error {
	// Decrement reference count
	_, err := db.Exec("UPDATE blobs SET reference_count = reference_count - 1 WHERE id = ? AND reference_count > 0", blobID)
	if err != nil {
		return err
	}

	// Check if we should delete the blob
	var refCount int
	err = db.QueryRow("SELECT reference_count FROM blobs WHERE id = ?", blobID).Scan(&refCount)
	if err != nil {
		return err
	}

	if refCount <= 0 {
		_, err = db.Exec("DELETE FROM blobs WHERE id = ?", blobID)
	}

	return err
}

// Message management functions

func CreateMessage(db *sql.DB, subject, inReplyTo, references string, date time.Time, sizeBytes int64) (int64, error) {
	result, err := db.Exec(`
		INSERT INTO messages (subject, in_reply_to, references_header, date, size_bytes)
		VALUES (?, ?, ?, ?, ?)
	`, subject, inReplyTo, references, date, sizeBytes)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func AddMessageToMailbox(db *sql.DB, messageID, mailboxID int64, flags string, internalDate time.Time) error {
	// Get next UID for this mailbox
	uid, err := IncrementUIDNext(db, mailboxID)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
		INSERT INTO message_mailbox (message_id, mailbox_id, uid, flags, internal_date)
		VALUES (?, ?, ?, ?, ?)
	`, messageID, mailboxID, uid, flags, internalDate)
	return err
}

func GetMessagesByMailbox(db *sql.DB, mailboxID int64) ([]int64, error) {
	rows, err := db.Query(`
		SELECT message_id FROM message_mailbox
		WHERE mailbox_id = ?
		ORDER BY uid ASC
	`, mailboxID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var messageIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			messageIDs = append(messageIDs, id)
		}
	}

	return messageIDs, rows.Err()
}

func GetMessageCount(db *sql.DB, mailboxID int64) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM message_mailbox WHERE mailbox_id = ?", mailboxID).Scan(&count)
	return count, err
}

func GetUnseenCount(db *sql.DB, mailboxID int64) (int, error) {
	var count int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM message_mailbox
		WHERE mailbox_id = ? AND (flags IS NULL OR flags NOT LIKE '%\Seen%')
	`, mailboxID).Scan(&count)
	return count, err
}

func UpdateMessageFlags(db *sql.DB, mailboxID, messageID int64, flags string) error {
	_, err := db.Exec(`
		UPDATE message_mailbox
		SET flags = ?
		WHERE mailbox_id = ? AND message_id = ?
	`, flags, mailboxID, messageID)
	return err
}

func GetMessageFlags(db *sql.DB, mailboxID, messageID int64) (string, error) {
	var flags sql.NullString
	err := db.QueryRow(`
		SELECT flags FROM message_mailbox
		WHERE mailbox_id = ? AND message_id = ?
	`, mailboxID, messageID).Scan(&flags)
	if err != nil {
		return "", err
	}
	return flags.String, nil
}

// Address management functions

func AddAddress(db *sql.DB, messageID int64, addressType, name, email string, sequence int) error {
	_, err := db.Exec(`
		INSERT INTO addresses (message_id, address_type, name, email, sequence)
		VALUES (?, ?, ?, ?, ?)
	`, messageID, addressType, name, email, sequence)
	return err
}

func GetMessageAddresses(db *sql.DB, messageID int64, addressType string) ([]string, error) {
	rows, err := db.Query(`
		SELECT name, email FROM addresses
		WHERE message_id = ? AND address_type = ?
		ORDER BY sequence
	`, messageID, addressType)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var addresses []string
	for rows.Next() {
		var name, email string
		if err := rows.Scan(&name, &email); err == nil {
			if name != "" {
				addresses = append(addresses, fmt.Sprintf("%s <%s>", name, email))
			} else {
				addresses = append(addresses, email)
			}
		}
	}

	return addresses, rows.Err()
}

// Message part management functions

func AddMessagePart(db *sql.DB, messageID int64, partNumber int, parentPartID sql.NullInt64, contentType, contentDisposition, contentTransferEncoding, charset, filename, contentID string, blobID sql.NullInt64, textContent string, sizeBytes int64) (int64, error) {
	result, err := db.Exec(`
		INSERT INTO message_parts (
			message_id, part_number, parent_part_id, content_type,
			content_disposition, content_transfer_encoding, charset,
			filename, content_id, blob_id, text_content, size_bytes
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, messageID, partNumber, parentPartID, contentType, contentDisposition, contentTransferEncoding, charset, filename, contentID, blobID, textContent, sizeBytes)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func GetMessageParts(db *sql.DB, messageID int64) ([]map[string]interface{}, error) {
	rows, err := db.Query(`
		SELECT id, part_number, parent_part_id, content_type, content_disposition,
		       content_transfer_encoding, charset, filename, content_id, blob_id, text_content, size_bytes
		FROM message_parts
		WHERE message_id = ?
		ORDER BY id
	`, messageID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var parts []map[string]interface{}
	for rows.Next() {
		var (
			id, partNumber, sizeBytes                                int64
			parentPartID, blobID                                     sql.NullInt64
			contentType, contentDisposition, contentTransferEncoding string
			charset, filename, contentID, textContent                sql.NullString
		)

		err := rows.Scan(&id, &partNumber, &parentPartID, &contentType, &contentDisposition,
			&contentTransferEncoding, &charset, &filename, &contentID, &blobID, &textContent, &sizeBytes)
		if err != nil {
			continue
		}

		part := map[string]interface{}{
			"id":                        id,
			"part_number":               partNumber,
			"content_type":              contentType,
			"content_disposition":       contentDisposition,
			"content_transfer_encoding": contentTransferEncoding,
			"size_bytes":                sizeBytes,
		}

		if parentPartID.Valid {
			part["parent_part_id"] = parentPartID.Int64
		}
		if charset.Valid {
			part["charset"] = charset.String
		}
		if filename.Valid {
			part["filename"] = filename.String
		}
		if contentID.Valid {
			part["content_id"] = contentID.String
		}
		if blobID.Valid {
			part["blob_id"] = blobID.Int64
		}
		if textContent.Valid {
			part["text_content"] = textContent.String
		}

		parts = append(parts, part)
	}

	return parts, rows.Err()
}

// Subscription management functions

func SubscribeToMailbox(db *sql.DB, userID int64, mailboxName string) error {
	_, err := db.Exec(`
		INSERT OR IGNORE INTO subscriptions (user_id, mailbox_name)
		VALUES (?, ?)
	`, userID, mailboxName)
	return err
}

func UnsubscribeFromMailbox(db *sql.DB, userID int64, mailboxName string) error {
	result, err := db.Exec(`
		DELETE FROM subscriptions
		WHERE user_id = ? AND mailbox_name = ?
	`, userID, mailboxName)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return fmt.Errorf("subscription does not exist")
	}

	return nil
}

func GetUserSubscriptions(db *sql.DB, userID int64) ([]string, error) {
	rows, err := db.Query(`
		SELECT mailbox_name
		FROM subscriptions
		WHERE user_id = ?
		ORDER BY mailbox_name
	`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var subscriptions []string
	for rows.Next() {
		var mailboxName string
		if err := rows.Scan(&mailboxName); err == nil {
			subscriptions = append(subscriptions, mailboxName)
		}
	}

	return subscriptions, rows.Err()
}

func IsMailboxSubscribed(db *sql.DB, userID int64, mailboxName string) (bool, error) {
	var count int
	err := db.QueryRow(`
		SELECT COUNT(*)
		FROM subscriptions
		WHERE user_id = ? AND mailbox_name = ?
	`, userID, mailboxName).Scan(&count)
	return count > 0, err
}

// Delivery management functions

func RecordDelivery(db *sql.DB, messageID int64, recipient, sender, status string, userID sql.NullInt64, smtpResponse string) error {
	_, err := db.Exec(`
		INSERT INTO deliveries (message_id, recipient, sender, status, user_id, delivered_at, smtp_response)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, messageID, recipient, sender, status, userID, time.Now(), smtpResponse)
	return err
}

// Outbound queue management functions

func QueueOutboundMessage(db *sql.DB, messageID int64, sender, recipient string, maxRetries int) error {
	_, err := db.Exec(`
		INSERT INTO outbound_queue (message_id, sender, recipient, max_retries, status, next_retry_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, messageID, sender, recipient, maxRetries, "pending", time.Now())
	return err
}

func GetPendingOutboundMessages(db *sql.DB, limit int) ([]map[string]interface{}, error) {
	rows, err := db.Query(`
		SELECT id, message_id, sender, recipient, retry_count, next_retry_at
		FROM outbound_queue
		WHERE status = 'pending' AND next_retry_at <= ?
		ORDER BY next_retry_at
		LIMIT ?
	`, time.Now(), limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var messages []map[string]interface{}
	for rows.Next() {
		var id, messageID, retryCount int64
		var sender, recipient string
		var nextRetryAt time.Time

		if err := rows.Scan(&id, &messageID, &sender, &recipient, &retryCount, &nextRetryAt); err == nil {
			messages = append(messages, map[string]interface{}{
				"id":            id,
				"message_id":    messageID,
				"sender":        sender,
				"recipient":     recipient,
				"retry_count":   retryCount,
				"next_retry_at": nextRetryAt,
			})
		}
	}

	return messages, rows.Err()
}

func UpdateOutboundStatus(db *sql.DB, queueID int64, status, lastError string) error {
	_, err := db.Exec(`
		UPDATE outbound_queue
		SET status = ?, last_error = ?, sent_at = ?
		WHERE id = ?
	`, status, lastError, time.Now(), queueID)
	return err
}

func RetryOutboundMessage(db *sql.DB, queueID int64, nextRetryDelay time.Duration) error {
	_, err := db.Exec(`
		UPDATE outbound_queue
		SET retry_count = retry_count + 1, next_retry_at = ?
		WHERE id = ?
	`, time.Now().Add(nextRetryDelay), queueID)
	return err
}

// Message header management functions

func AddMessageHeader(db *sql.DB, messageID int64, headerName, headerValue string, sequence int) error {
	_, err := db.Exec(`
		INSERT INTO message_headers (message_id, header_name, header_value, sequence)
		VALUES (?, ?, ?, ?)
	`, messageID, headerName, headerValue, sequence)
	return err
}

func GetMessageHeaders(db *sql.DB, messageID int64) ([]map[string]string, error) {
	rows, err := db.Query(`
		SELECT header_name, header_value
		FROM message_headers
		WHERE message_id = ?
		ORDER BY sequence
	`, messageID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var headers []map[string]string
	for rows.Next() {
		var name, value string
		if err := rows.Scan(&name, &value); err == nil {
			headers = append(headers, map[string]string{
				"name":  name,
				"value": value,
			})
		}
	}

	return headers, rows.Err()
}

// Role mailbox management functions

func CreateRoleMailbox(db *sql.DB, email string, domainID int64, description string) (int64, error) {
	result, err := db.Exec(`
		INSERT INTO role_mailboxes (email, domain_id, description, enabled)
		VALUES (?, ?, ?, ?)
	`, email, domainID, description, true)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return 0, fmt.Errorf("role mailbox already exists")
		}
		return 0, err
	}
	return result.LastInsertId()
}

func GetRoleMailboxByEmail(db *sql.DB, email string) (int64, int64, error) {
	var id, domainID int64
	err := db.QueryRow(`
		SELECT id, domain_id FROM role_mailboxes
		WHERE email = ? AND enabled = ?
	`, email, true).Scan(&id, &domainID)
	if err == sql.ErrNoRows {
		return 0, 0, fmt.Errorf("role mailbox not found")
	}
	return id, domainID, err
}

func GetRoleMailboxByID(db *sql.DB, roleMailboxID int64) (string, int64, error) {
	var email string
	var domainID int64
	err := db.QueryRow(`
		SELECT email, domain_id FROM role_mailboxes
		WHERE id = ? AND enabled = ?
	`, roleMailboxID, true).Scan(&email, &domainID)
	if err == sql.ErrNoRows {
		return "", 0, fmt.Errorf("role mailbox not found")
	}
	return email, domainID, err
}

func GetOrCreateRoleMailbox(db *sql.DB, email string, domainID int64, description string) (int64, error) {
	id, _, err := GetRoleMailboxByEmail(db, email)
	if err == nil {
		return id, nil
	}
	return CreateRoleMailbox(db, email, domainID, description)
}

func RoleMailboxExists(db *sql.DB, email string) (bool, error) {
	var count int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM role_mailboxes
		WHERE email = ? AND enabled = ?
	`, email, true).Scan(&count)
	return count > 0, err
}

// User role assignment management functions

func AssignUserToRoleMailbox(db *sql.DB, userID, roleMailboxID, assignedBy int64) error {
	// Start transaction to ensure consistency
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Deactivate any existing active assignments for this role mailbox
	_, err = tx.Exec(`
		UPDATE user_role_assignments
		SET is_active = FALSE
		WHERE role_mailbox_id = ? AND is_active = TRUE
	`, roleMailboxID)
	if err != nil {
		return err
	}

	// Insert or update the assignment
	_, err = tx.Exec(`
		INSERT INTO user_role_assignments (user_id, role_mailbox_id, assigned_by, is_active)
		VALUES (?, ?, ?, TRUE)
		ON CONFLICT(user_id, role_mailbox_id)
		DO UPDATE SET is_active = TRUE, assigned_at = CURRENT_TIMESTAMP, assigned_by = ?
	`, userID, roleMailboxID, assignedBy, assignedBy)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func UnassignUserFromRoleMailbox(db *sql.DB, userID, roleMailboxID int64) error {
	_, err := db.Exec(`
		UPDATE user_role_assignments
		SET is_active = FALSE
		WHERE user_id = ? AND role_mailbox_id = ?
	`, userID, roleMailboxID)
	return err
}

func GetUserRoleAssignments(db *sql.DB, userID int64) ([]int64, error) {
	rows, err := db.Query(`
		SELECT role_mailbox_id
		FROM user_role_assignments
		WHERE user_id = ? AND is_active = TRUE
	`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var roleMailboxIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			roleMailboxIDs = append(roleMailboxIDs, id)
		}
	}

	return roleMailboxIDs, rows.Err()
}

func GetRoleMailboxAssignedUser(db *sql.DB, roleMailboxID int64) (int64, error) {
	var userID int64
	err := db.QueryRow(`
		SELECT user_id
		FROM user_role_assignments
		WHERE role_mailbox_id = ? AND is_active = TRUE
		LIMIT 1
	`, roleMailboxID).Scan(&userID)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("no active user assigned to this role mailbox")
	}
	return userID, err
}

func IsUserAssignedToRoleMailbox(db *sql.DB, userID, roleMailboxID int64) (bool, error) {
	var count int
	err := db.QueryRow(`
		SELECT COUNT(*)
		FROM user_role_assignments
		WHERE user_id = ? AND role_mailbox_id = ? AND is_active = TRUE
	`, userID, roleMailboxID).Scan(&count)
	return count > 0, err
}

// New helper functions for password_initialized
func IsPasswordInitialized(db *sql.DB, userID int64) (bool, error) {
	var initialized bool
	err := db.QueryRow("SELECT password_initialized FROM users WHERE id = ?", userID).Scan(&initialized)
	if err != nil {
		return false, err
	}
	return initialized, nil
}

func SetPasswordInitialized(db *sql.DB, userID int64, initialized bool) error {
	_, err := db.Exec("UPDATE users SET password_initialized = ? WHERE id = ?", initialized, userID)
	return err
}

// GetOrCreateUserInitialized ensures the user exists; if creating a new user, mark password_initialized = true.
func GetOrCreateUserInitialized(db *sql.DB, username string, domainID int64) (int64, error) {
	id, err := GetUserByUsername(db, username, domainID)
	if err == nil {
		return id, nil
	}
	// Try to insert with password_initialized = true
	result, err := db.Exec("INSERT INTO users (username, domain_id, enabled, password_initialized) VALUES (?, ?, ?, ?)", username, domainID, true, true)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			// Race: another process created the user; select again
			return GetUserByUsername(db, username, domainID)
		}
		return 0, err
	}
	return result.LastInsertId()
}

// createDefaultMailboxes creates default mailboxes for a new user.
// Kept here with other schema helpers so migrations and initialization stay together.
func createDefaultMailboxes(db *sql.DB, userID int64) error {
	defaultMailboxes := []struct {
		name       string
		specialUse string
	}{
		{"INBOX", "\\Inbox"},
		{"Sent", "\\Sent"},
		{"Drafts", "\\Drafts"},
		{"Trash", "\\Trash"},
		{"Spam", "\\Junk"},
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
		"CREATE INDEX IF NOT EXISTS idx_blobs_hash ON blobs(sha256_hash)",
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
