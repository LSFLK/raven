package server

import (
	"database/sql"
	"fmt"
	"net"
	"strings"

	"go-imap/internal/conf"
	"go-imap/internal/db"
	"go-imap/internal/models"
)

type IMAPServer struct {
	db       *sql.DB
	certPath string
	keyPath  string
}

func NewIMAPServer(database *sql.DB) *IMAPServer {
	return &IMAPServer{
		db:       database,
		certPath: "/certs/fullchain.pem",
		keyPath:  "/certs/privkey.pem",
	}
}

// SetTLSCertificates sets custom TLS certificate paths (useful for testing)
func (s *IMAPServer) SetTLSCertificates(certPath, keyPath string) {
	s.certPath = certPath
	s.keyPath = keyPath
}

func (s *IMAPServer) HandleConnection(conn net.Conn) {
	defer conn.Close()

	state := &models.ClientState{
		Authenticated: false,
		Conn:          conn,
	}

	// Greeting - advertise basic capabilities in greeting
	s.sendResponse(conn, "* OK [CAPABILITY IMAP4rev1 STARTTLS LOGINDISABLED UIDPLUS IDLE LITERAL+] SQLite IMAP server ready")

	handleClient(s, conn, state)
}

// Helper functions for new schema

// ensureUserAndMailboxes ensures user exists in database and has default mailboxes
func (s *IMAPServer) ensureUserAndMailboxes(username string, domain string) (int64, int64, error) {
	// Get or create domain
	domainID, err := db.GetOrCreateDomain(s.db, domain)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get/create domain: %v", err)
	}

	// Get or create user
	userID, err := db.GetOrCreateUser(s.db, username, domainID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get/create user: %v", err)
	}

	// Ensure default mailboxes exist
	defaultMailboxes := []struct {
		name       string
		specialUse string
	}{
		{"INBOX", "\\Inbox"},
		{"Sent", "\\Sent"},
		{"Drafts", "\\Drafts"},
		{"Trash", "\\Trash"},
	}

	for _, mbx := range defaultMailboxes {
		// Check if mailbox exists
		exists, _ := db.MailboxExists(s.db, userID, mbx.name)
		if !exists {
			// Create mailbox
			_, err := db.CreateMailbox(s.db, userID, mbx.name, mbx.specialUse)
			if err != nil && !strings.Contains(err.Error(), "already exists") {
				return 0, 0, fmt.Errorf("failed to create mailbox %s: %v", mbx.name, err)
			}
		}
	}

	return userID, domainID, nil
}

// getUserDomain extracts domain from username or uses default from config
func (s *IMAPServer) getUserDomain(username string) string {
	// If username contains @, extract domain
	if strings.Contains(username, "@") {
		parts := strings.Split(username, "@")
		if len(parts) == 2 {
			return parts[1]
		}
	}

	// Use domain from config
	cfg, err := conf.LoadConfig()
	if err == nil && cfg.Domain != "" {
		return cfg.Domain
	}

	// Fallback to localhost
	return "localhost"
}

// extractUsername removes domain from username if present
func (s *IMAPServer) extractUsername(email string) string {
	if strings.Contains(email, "@") {
		parts := strings.Split(email, "@")
		return parts[0]
	}
	return email
}