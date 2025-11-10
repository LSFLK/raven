package storage

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"raven/internal/db"
	"raven/internal/delivery/parser"
)

// Storage handles message storage operations
type Storage struct {
	dbManager *db.DBManager
}

// NewStorage creates a new storage handler
func NewStorage(dbManager *db.DBManager) *Storage {
	return &Storage{
		dbManager: dbManager,
	}
}

// DeliverMessage stores a message for a recipient
func (s *Storage) DeliverMessage(recipient string, msg *parser.Message, folder string) error {
	// Extract username and domain from email address
	username, err := parser.ExtractLocalPart(recipient)
	if err != nil {
		return fmt.Errorf("failed to extract username: %w", err)
	}

	domain, err := parser.ExtractDomain(recipient)
	if err != nil {
		return fmt.Errorf("failed to extract domain: %w", err)
	}

	// Get shared database for domain and user operations
	sharedDB := s.dbManager.GetSharedDB()

	// Get or create domain
	domainID, err := db.GetOrCreateDomain(sharedDB, domain)
	if err != nil {
		return fmt.Errorf("failed to get/create domain: %w", err)
	}

	// Check if this is a role mailbox
	roleMailboxID, _, roleErr := db.GetRoleMailboxByEmail(sharedDB, recipient)

	var targetDB *sql.DB
	var targetUserID int64

	if roleErr == nil {
		// This is a role mailbox - deliver to role mailbox database
		targetDB, err = s.dbManager.GetRoleMailboxDB(roleMailboxID)
		if err != nil {
			return fmt.Errorf("failed to get role mailbox database: %w", err)
		}
		targetUserID = 0 // Role mailboxes use userID 0
		log.Printf("Delivering to role mailbox: %s (ID: %d)", recipient, roleMailboxID)
	} else {
		// Not a role mailbox - deliver to regular user mailbox
		// Get or create user
		userID, err := db.GetOrCreateUser(sharedDB, username, domainID)
		if err != nil {
			return fmt.Errorf("failed to get/create user: %w", err)
		}

		// Get user database
		targetDB, err = s.dbManager.GetUserDB(userID)
		if err != nil {
			return fmt.Errorf("failed to get user database: %w", err)
		}
		targetUserID = userID
	}

	// Get or create the target mailbox
	mailboxID, err := db.GetMailboxByNamePerUser(targetDB, targetUserID, folder)
	if err != nil {
		// Mailbox doesn't exist, create it
		mailboxID, err = db.CreateMailboxPerUser(targetDB, targetUserID, folder, "")
		if err != nil {
			return fmt.Errorf("failed to create mailbox: %w", err)
		}
	}

	// Parse the message into MIME structure
	parsed, err := parser.ParseMIMEMessage(msg.RawMessage)
	if err != nil {
		return fmt.Errorf("failed to parse message: %w", err)
	}

	// Store the message in the target database (user or role mailbox)
	messageID, err := parser.StoreMessagePerUser(targetDB, parsed)
	if err != nil {
		return fmt.Errorf("failed to store message: %w", err)
	}

	// Add the message to the mailbox
	internalDate := msg.Date
	if internalDate.IsZero() {
		internalDate = time.Now()
	}

	err = db.AddMessageToMailboxPerUser(targetDB, messageID, mailboxID, "", internalDate)
	if err != nil {
		return fmt.Errorf("failed to add message to mailbox: %w", err)
	}

	// Record delivery
	var userIDNull sql.NullInt64
	if targetUserID > 0 {
		userIDNull = sql.NullInt64{Valid: true, Int64: targetUserID}
	} else {
		userIDNull = sql.NullInt64{Valid: false}
	}
	err = db.RecordDeliveryPerUser(targetDB, messageID, recipient, msg.From, "delivered", userIDNull, "250 OK")
	if err != nil {
		// Log but don't fail - delivery tracking is not critical
		fmt.Printf("Warning: failed to record delivery: %v\n", err)
	}

	return nil
}

// ensureDefaultMailboxes creates default mailboxes if they don't exist
// Note: This is now handled automatically when creating a new user database
func (s *Storage) ensureDefaultMailboxes(userID int64) {
	// Default mailboxes are created automatically by DBManager.GetUserDB()
	// when initializing a new user database, so this is now a no-op
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
	sharedDB := s.dbManager.GetSharedDB()
	var count int
	err := sharedDB.QueryRow(
		"SELECT COUNT(*) FROM users WHERE username = ?",
		username,
	).Scan(&count)

	if err != nil {
		if err == sql.ErrNoRows {
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

	// In multi-domain mode, we should also check the domain
	// For now, just check if the username exists in any domain
	return s.CheckUserExists(username)
}

// GetUserQuota retrieves the current quota usage for a user
func (s *Storage) GetUserQuota(username string) (int64, error) {
	sharedDB := s.dbManager.GetSharedDB()

	// Get user ID (from any domain - we'll sum across all)
	var userID int64
	err := sharedDB.QueryRow("SELECT id FROM users WHERE username = ? LIMIT 1", username).Scan(&userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil // User doesn't exist yet
		}
		return 0, err
	}

	// Get user database
	userDB, err := s.dbManager.GetUserDB(userID)
	if err != nil {
		return 0, fmt.Errorf("failed to get user database: %w", err)
	}

	// Calculate total size of all messages for this user
	// This sums up the size of all messages in all mailboxes for this user
	query := `
		SELECT COALESCE(SUM(m.size_bytes), 0)
		FROM messages m
		JOIN message_mailbox mm ON m.id = mm.message_id
		JOIN mailboxes mb ON mm.mailbox_id = mb.id
		WHERE mb.user_id = ?
	`

	var totalSize int64
	err = userDB.QueryRow(query, userID).Scan(&totalSize)
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
	sharedDB := s.dbManager.GetSharedDB()

	// Get user ID (from any domain)
	var userID int64
	err := sharedDB.QueryRow("SELECT id FROM users WHERE username = ? LIMIT 1", username).Scan(&userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}

	// Get user database
	userDB, err := s.dbManager.GetUserDB(userID)
	if err != nil {
		return 0, fmt.Errorf("failed to get user database: %w", err)
	}

	// Count messages in all mailboxes for this user
	query := `
		SELECT COUNT(DISTINCT mm.message_id)
		FROM message_mailbox mm
		JOIN mailboxes mb ON mm.mailbox_id = mb.id
		WHERE mb.user_id = ?
	`

	var count int
	err = userDB.QueryRow(query, userID).Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

// GetMessageCountInFolder returns the number of messages in a specific folder
func (s *Storage) GetMessageCountInFolder(username string, folder string) (int, error) {
	sharedDB := s.dbManager.GetSharedDB()

	// Get user ID (from any domain)
	var userID int64
	err := sharedDB.QueryRow("SELECT id FROM users WHERE username = ? LIMIT 1", username).Scan(&userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}

	// Get user database
	userDB, err := s.dbManager.GetUserDB(userID)
	if err != nil {
		return 0, fmt.Errorf("failed to get user database: %w", err)
	}

	// Get mailbox ID
	mailboxID, err := db.GetMailboxByNamePerUser(userDB, userID, folder)
	if err != nil {
		return 0, nil // Mailbox doesn't exist
	}

	// Count messages in the mailbox
	count, err := db.GetMessageCountPerUser(userDB, mailboxID)
	if err != nil {
		return 0, err
	}

	return count, nil
}

// CreateUserIfNotExists creates a user if they don't exist
func (s *Storage) CreateUserIfNotExists(username string) error {
	sharedDB := s.dbManager.GetSharedDB()

	// Use "localhost" as default domain if none specified
	domain := "localhost"
	if strings.Contains(username, "@") {
		parts := strings.Split(username, "@")
		username = parts[0]
		domain = parts[1]
	}

	// Get or create domain
	domainID, err := db.GetOrCreateDomain(sharedDB, domain)
	if err != nil {
		return fmt.Errorf("failed to get/create domain: %w", err)
	}

	// Get or create user
	userID, err := db.GetOrCreateUser(sharedDB, username, domainID)
	if err != nil {
		return fmt.Errorf("failed to get/create user: %w", err)
	}

	// Initialize user database (will create default mailboxes)
	_, err = s.dbManager.GetUserDB(userID)
	if err != nil {
		return fmt.Errorf("failed to initialize user database: %w", err)
	}

	return nil
}
