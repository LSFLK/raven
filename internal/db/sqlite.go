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
