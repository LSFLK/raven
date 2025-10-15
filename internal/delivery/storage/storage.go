package storage

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"go-imap/internal/db"
	"go-imap/internal/delivery/parser"
)

// Storage handles message storage operations
type Storage struct {
	db *sql.DB
}

// NewStorage creates a new storage handler
func NewStorage(database *sql.DB) *Storage {
	return &Storage{
		db: database,
	}
}

// DeliverMessage stores a message for a recipient
func (s *Storage) DeliverMessage(recipient string, msg *parser.Message, folder string) error {
	// Extract username from email address
	username, err := parser.ExtractLocalPart(recipient)
	if err != nil {
		return fmt.Errorf("failed to extract username: %w", err)
	}

	// Ensure user table exists
	if err := db.EnsureUserTable(s.db, username); err != nil {
		return fmt.Errorf("failed to ensure user table: %w", err)
	}

	// If folder is not INBOX, ensure it exists
	if strings.ToUpper(folder) != "INBOX" {
		exists, err := db.MailboxExists(s.db, username, folder)
		if err != nil {
			return fmt.Errorf("failed to check mailbox existence: %w", err)
		}

		// Create folder if it doesn't exist (except for default folders)
		if !exists {
			if err := db.CreateMailbox(s.db, username, folder); err != nil {
				// Ignore "already exists" errors
				if !strings.Contains(err.Error(), "already exists") {
					return fmt.Errorf("failed to create mailbox: %w", err)
				}
			}
		}
	}

	// Insert message into user's table
	tableName := db.GetUserTableName(username)
	query := fmt.Sprintf(`
		INSERT INTO %s (subject, sender, recipient, date_sent, raw_message, flags, folder)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, tableName)

	_, err = s.db.Exec(
		query,
		msg.Subject,
		msg.From,
		recipient,
		msg.Date.Format(time.RFC3339),
		msg.RawMessage,
		"", // No flags initially
		folder,
	)

	if err != nil {
		return fmt.Errorf("failed to insert message: %w", err)
	}

	return nil
}

// DeliverToMultipleRecipients delivers a message to multiple recipients
func (s *Storage) DeliverToMultipleRecipients(recipients []string, msg *parser.Message, folder string) map[string]error {
	results := make(map[string]error)

	for _, recipient := range recipients {
		err := s.DeliverMessage(recipient, msg, folder)
		if err != nil {
			results[recipient] = err
		} else {
			results[recipient] = nil
		}
	}

	return results
}

// CheckUserExists checks if a user exists in the system
func (s *Storage) CheckUserExists(username string) (bool, error) {
	var count int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM user_metadata WHERE username = ?",
		username,
	).Scan(&count)

	if err != nil {
		// If table doesn't exist, user doesn't exist
		if strings.Contains(err.Error(), "no such table") {
			return false, nil
		}
		return false, err
	}

	return count > 0, nil
}

// CheckRecipientExists checks if a recipient email address is valid for delivery
func (s *Storage) CheckRecipientExists(recipient string) (bool, error) {
	username, err := parser.ExtractLocalPart(recipient)
	if err != nil {
		return false, err
	}

	return s.CheckUserExists(username)
}

// GetUserQuota retrieves the current quota usage for a user
func (s *Storage) GetUserQuota(username string) (int64, error) {
	tableName := db.GetUserTableName(username)

	// Check if user table exists
	var tableExists int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type='table' AND name=?
	`, tableName).Scan(&tableExists)

	if err != nil {
		return 0, err
	}

	if tableExists == 0 {
		return 0, nil // No messages yet
	}

	// Calculate total size of all messages
	query := fmt.Sprintf(`
		SELECT COALESCE(SUM(LENGTH(raw_message)), 0)
		FROM %s
	`, tableName)

	var totalSize int64
	err = s.db.QueryRow(query).Scan(&totalSize)
	if err != nil {
		return 0, fmt.Errorf("failed to calculate quota: %w", err)
	}

	return totalSize, nil
}

// CheckQuota checks if a user has enough quota for a message
func (s *Storage) CheckQuota(username string, messageSize int64, quotaLimit int64) error {
	currentUsage, err := s.GetUserQuota(username)
	if err != nil {
		return fmt.Errorf("failed to get quota: %w", err)
	}

	if currentUsage+messageSize > quotaLimit {
		return fmt.Errorf("quota exceeded: current=%d, limit=%d, message=%d",
			currentUsage, quotaLimit, messageSize)
	}

	return nil
}

// GetMessageCount returns the total number of messages for a user
func (s *Storage) GetMessageCount(username string) (int, error) {
	tableName := db.GetUserTableName(username)

	// Check if user table exists
	var tableExists int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type='table' AND name=?
	`, tableName).Scan(&tableExists)

	if err != nil {
		return 0, err
	}

	if tableExists == 0 {
		return 0, nil
	}

	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)
	var count int
	err = s.db.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

// GetMessageCountInFolder returns the number of messages in a specific folder
func (s *Storage) GetMessageCountInFolder(username string, folder string) (int, error) {
	tableName := db.GetUserTableName(username)

	// Check if user table exists
	var tableExists int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type='table' AND name=?
	`, tableName).Scan(&tableExists)

	if err != nil {
		return 0, err
	}

	if tableExists == 0 {
		return 0, nil
	}

	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE folder = ?", tableName)
	var count int
	err = s.db.QueryRow(query, folder).Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

// CreateUserIfNotExists creates a user if they don't exist
func (s *Storage) CreateUserIfNotExists(username string) error {
	return db.EnsureUserTable(s.db, username)
}
