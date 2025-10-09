package db

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

func InitDB(file string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", file)
	if err != nil {
		return nil, err
	}

	// Create a metadata table to track users
	metadataSchema := `
	CREATE TABLE IF NOT EXISTS user_metadata (
		username TEXT PRIMARY KEY,
		created_at TEXT DEFAULT CURRENT_TIMESTAMP
	);
	`
	if _, err = db.Exec(metadataSchema); err != nil {
		return nil, err
	}

	// Create mailboxes table
	if err = CreateMailboxTable(db); err != nil {
		return nil, err
	}

	return db, nil
}

// GetUserTableName returns the sanitized table name for a user
func GetUserTableName(username string) string {
	// Sanitize username to create a valid table name
	// Replace special characters with underscores
	sanitized := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return '_'
	}, username)
	return fmt.Sprintf("mails_%s", sanitized)
}

// CreateUserTable creates a dedicated table for a user if it doesn't exist
func CreateUserTable(db *sql.DB, username string) error {
	tableName := GetUserTableName(username)

	schema := fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		subject TEXT,
		sender TEXT,
		recipient TEXT,
		date_sent TEXT,
		raw_message TEXT,
		flags TEXT DEFAULT '',
		folder TEXT DEFAULT 'INBOX'
	);
	`, tableName)

	if _, err := db.Exec(schema); err != nil {
		return err
	}

	// Track user in metadata table
	_, err := db.Exec("INSERT OR IGNORE INTO user_metadata (username) VALUES (?)", username)
	if err != nil {
		return err
	}

	return nil
}

// EnsureUserTable ensures a user's table exists (creates if needed)
func EnsureUserTable(db *sql.DB, username string) error {
	return CreateUserTable(db, username)
}

// CreateMailboxTable creates a table to track mailboxes for all users
func CreateMailboxTable(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS mailboxes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT NOT NULL,
		name TEXT NOT NULL,
		hierarchy_separator TEXT DEFAULT '/',
		created_at TEXT DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(username, name)
	);
	`

	_, err := db.Exec(schema)
	return err
}

// EnsureMailboxTable ensures the mailbox table exists
func EnsureMailboxTable(db *sql.DB) error {
	return CreateMailboxTable(db)
}

// CreateMailbox creates a new mailbox for a user
func CreateMailbox(db *sql.DB, username, mailboxName string) error {
	// Validate mailbox name
	if mailboxName == "" {
		return fmt.Errorf("mailbox name cannot be empty")
	}

	// INBOX is case-insensitive and special
	if strings.ToUpper(mailboxName) == "INBOX" {
		return fmt.Errorf("cannot create INBOX - it already exists")
	}

	// Insert mailbox record
	_, err := db.Exec(`
		INSERT INTO mailboxes (username, name) 
		VALUES (?, ?)
	`, username, mailboxName)

	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return fmt.Errorf("mailbox already exists")
		}
		return err
	}

	return nil
}

// MailboxExists checks if a mailbox exists for a user
func MailboxExists(db *sql.DB, username, mailboxName string) (bool, error) {
	// INBOX always exists for authenticated users
	if strings.ToUpper(mailboxName) == "INBOX" {
		return true, nil
	}

	// Check default mailboxes
	defaultMailboxes := map[string]bool{
		"Sent":   true,
		"Drafts": true,
		"Trash":  true,
	}

	if defaultMailboxes[mailboxName] {
		return true, nil
	}

	// Check custom mailboxes in database
	var count int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM mailboxes 
		WHERE username = ? AND name = ?
	`, username, mailboxName).Scan(&count)

	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// GetUserMailboxes returns all mailboxes for a user
func GetUserMailboxes(db *sql.DB, username string) ([]string, error) {
	// Start with default mailboxes
	mailboxes := []string{"INBOX", "Sent", "Drafts", "Trash"}

	// Add custom mailboxes from database
	rows, err := db.Query(`
		SELECT name FROM mailboxes 
		WHERE username = ? 
		ORDER BY name
	`, username)

	if err != nil {
		return mailboxes, nil // Return defaults if query fails
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			mailboxes = append(mailboxes, name)
		}
	}

	return mailboxes, nil
}
