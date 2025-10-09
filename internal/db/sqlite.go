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

// DeleteMailbox deletes a mailbox for a user according to RFC 3501 rules
func DeleteMailbox(db *sql.DB, username, mailboxName string) error {
	// Validate mailbox name
	if mailboxName == "" {
		return fmt.Errorf("mailbox name cannot be empty")
	}

	// Cannot delete INBOX (case-insensitive)
	if strings.ToUpper(mailboxName) == "INBOX" {
		return fmt.Errorf("cannot delete INBOX")
	}

	// Check if mailbox exists
	exists, err := MailboxExists(db, username, mailboxName)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("mailbox does not exist")
	}

	// Check for inferior hierarchical names
	var count int
	err = db.QueryRow(`
		SELECT COUNT(*) FROM mailboxes 
		WHERE username = ? AND name LIKE ? AND name != ?
	`, username, mailboxName+"/%", mailboxName).Scan(&count)

	if err != nil {
		return err
	}

	// If mailbox has inferior hierarchical names
	if count > 0 {
		// RFC 3501: Mailboxes with \Noselect attribute can be deleted even if they have inferior names.
		// However, our implementation does not support mailbox attributes (including \Noselect).
		// Therefore, mailboxes with inferior hierarchical names cannot be deleted, regardless of attributes.
		return fmt.Errorf("name \"%s\" has inferior hierarchical names (mailbox attributes such as \\Noselect are not supported)", mailboxName)
	}

	// Delete all messages from the mailbox first
	tableName := GetUserTableName(username)
	
	// Check if user table exists first
	var tableExists int
	err = db.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master 
		WHERE type='table' AND name=?
	`, tableName).Scan(&tableExists)
	
	if err != nil {
		return fmt.Errorf("failed to check user table: %v", err)
	}
	
	// Only delete messages if user table exists
	if tableExists > 0 {
		_, err = db.Exec(fmt.Sprintf(
			"DELETE FROM %s WHERE folder = ?", tableName,
		), mailboxName)
		if err != nil {
			return fmt.Errorf("failed to delete messages: %v", err)
		}
	}

	// Delete the mailbox record from mailboxes table
	result, err := db.Exec(`
		DELETE FROM mailboxes 
		WHERE username = ? AND name = ?
	`, username, mailboxName)

	if err != nil {
		return err
	}

	// Check if any rows were affected
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return fmt.Errorf("mailbox does not exist")
	}

	return nil
}

// RenameMailbox renames a mailbox for a user according to RFC 3501 rules
func RenameMailbox(db *sql.DB, username, oldName, newName string) error {
	// Validate mailbox names
	if oldName == "" || newName == "" {
		return fmt.Errorf("mailbox names cannot be empty")
	}

	// Handle INBOX renaming (special case)
	if strings.ToUpper(oldName) == "INBOX" {
		return renameInbox(db, username, newName)
	}

	// Cannot rename TO INBOX
	if strings.ToUpper(newName) == "INBOX" {
		return fmt.Errorf("cannot rename to INBOX")
	}

	// Check if source mailbox exists
	exists, err := MailboxExists(db, username, oldName)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("source mailbox does not exist")
	}

	// Check if destination mailbox already exists
	destExists, err := MailboxExists(db, username, newName)
	if err != nil {
		return err
	}
	if destExists {
		return fmt.Errorf("destination mailbox already exists")
	}

	// Handle hierarchy creation for destination
	if strings.Contains(newName, "/") {
		err := createSuperiorHierarchy(db, username, newName)
		if err != nil {
			return fmt.Errorf("failed to create superior hierarchy: %v", err)
		}
	}

	// Rename the mailbox and all its inferior hierarchical names
	err = renameMailboxHierarchy(db, username, oldName, newName)
	if err != nil {
		return err
	}

	return nil
}

// renameInbox handles the special case of renaming INBOX
func renameInbox(db *sql.DB, username, newName string) error {
	// Check if destination mailbox already exists
	destExists, err := MailboxExists(db, username, newName)
	if err != nil {
		return err
	}
	if destExists {
		return fmt.Errorf("destination mailbox already exists")
	}

	// Handle hierarchy creation for destination
	if strings.Contains(newName, "/") {
		err := createSuperiorHierarchy(db, username, newName)
		if err != nil {
			return fmt.Errorf("failed to create superior hierarchy: %v", err)
		}
	}

	// Create the destination mailbox
	err = CreateMailbox(db, username, newName)
	if err != nil {
		return fmt.Errorf("failed to create destination mailbox: %v", err)
	}

	// Move all messages from INBOX to the new mailbox
	tableName := GetUserTableName(username)
	
	// Check if user table exists
	var tableExists int
	err = db.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master 
		WHERE type='table' AND name=?
	`, tableName).Scan(&tableExists)
	
	if err != nil {
		return fmt.Errorf("failed to check user table: %v", err)
	}
	
	// Only move messages if user table exists
	if tableExists > 0 {
		_, err = db.Exec(fmt.Sprintf(
			"UPDATE %s SET folder = ? WHERE folder = 'INBOX'", tableName,
		), newName)
		if err != nil {
			return fmt.Errorf("failed to move messages: %v", err)
		}
	}

	// INBOX itself remains empty but continues to exist
	// (inferior hierarchical names of INBOX are unaffected)
	
	return nil
}

// renameMailboxHierarchy renames a mailbox and all its inferior hierarchical names
func renameMailboxHierarchy(db *sql.DB, username, oldName, newName string) error {
	// Start a transaction
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Get all mailboxes that need to be renamed (the mailbox itself and its inferiors)
	var mailboxesToRename []struct {
		oldMailbox string
		newMailbox string
	}

	// Add the main mailbox
	mailboxesToRename = append(mailboxesToRename, struct {
		oldMailbox string
		newMailbox string
	}{oldName, newName})

	// Get all inferior hierarchical names
	rows, err := tx.Query(`
		SELECT name FROM mailboxes 
		WHERE username = ? AND name LIKE ? AND name != ?
		ORDER BY name
	`, username, oldName+"/%", oldName)

	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var inferiorName string
		if err := rows.Scan(&inferiorName); err != nil {
			return err
		}
		
		// Calculate new name by replacing the prefix
		newInferiorName := newName + inferiorName[len(oldName):]
		mailboxesToRename = append(mailboxesToRename, struct {
			oldMailbox string
			newMailbox string
		}{inferiorName, newInferiorName})
	}

	// Rename all mailboxes in the database
	for _, rename := range mailboxesToRename {
		_, err = tx.Exec(`
			UPDATE mailboxes 
			SET name = ? 
			WHERE username = ? AND name = ?
		`, rename.newMailbox, username, rename.oldMailbox)
		
		if err != nil {
			return err
		}
	}

	// Update folder names in user's message table
	tableName := GetUserTableName(username)
	
	// Check if user table exists
	var tableExists int
	err = tx.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master 
		WHERE type='table' AND name=?
	`, tableName).Scan(&tableExists)
	
	if err != nil {
		return err
	}
	
	// Only update messages if user table exists
	if tableExists > 0 {
		for _, rename := range mailboxesToRename {
			_, err = tx.Exec(fmt.Sprintf(
				"UPDATE %s SET folder = ? WHERE folder = ?", tableName,
			), rename.newMailbox, rename.oldMailbox)
			
			if err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

// createSuperiorHierarchy creates any superior hierarchical names needed
func createSuperiorHierarchy(db *sql.DB, username, mailboxName string) error {
	if !strings.Contains(mailboxName, "/") {
		return nil // No hierarchy to create
	}

	parts := strings.Split(mailboxName, "/")
	currentPath := ""
	
	// Create each level of the hierarchy (except the final mailbox)
	for i, part := range parts {
		if i > 0 {
			currentPath += "/"
		}
		currentPath += part
		
		// Skip if this is the final mailbox (caller will create it)
		if i == len(parts)-1 {
			break
		}
		
		// Check if this intermediate mailbox exists
		exists, err := MailboxExists(db, username, currentPath)
		if err != nil {
			return err
		}
		
		if !exists {
			// Create intermediate mailbox - ignore "already exists" errors
			err = CreateMailbox(db, username, currentPath)
			if err != nil && !strings.Contains(err.Error(), "already exists") {
				return err
			}
		}
	}
	
	return nil
}
