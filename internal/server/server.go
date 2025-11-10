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
	dbManager *db.DBManager
	certPath  string
	keyPath   string
}

func NewIMAPServer(dbManager *db.DBManager) *IMAPServer {
	return &IMAPServer{
		dbManager: dbManager,
		certPath:  "/certs/fullchain.pem",
		keyPath:   "/certs/privkey.pem",
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
	sharedDB := s.dbManager.GetSharedDB()

	// Get or create domain
	domainID, err := db.GetOrCreateDomain(sharedDB, domain)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get/create domain: %v", err)
	}

	// Get or create user
	userID, err := db.GetOrCreateUser(sharedDB, username, domainID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get/create user: %v", err)
	}

	// Get user database (this will create default mailboxes if it's a new user)
	_, err = s.dbManager.GetUserDB(userID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to initialize user database: %v", err)
	}

	return userID, domainID, nil
}

// getUserDB returns the database connection for a user
func (s *IMAPServer) getUserDB(userID int64) (*sql.DB, error) {
	return s.dbManager.GetUserDB(userID)
}

// getSelectedDB returns the appropriate database based on client state
// If a role mailbox is selected, returns the role mailbox database
// Otherwise returns the user's database
func (s *IMAPServer) getSelectedDB(state *models.ClientState) (*sql.DB, int64, error) {
	if state.IsRoleMailbox {
		roleDB, err := s.dbManager.GetRoleMailboxDB(state.SelectedRoleMailboxID)
		return roleDB, 0, err // userID is 0 for role mailboxes
	}
	userDB, err := s.dbManager.GetUserDB(state.UserID)
	return userDB, state.UserID, err
}

// getSharedDB returns the shared database connection
func (s *IMAPServer) getSharedDB() *sql.DB {
	return s.dbManager.GetSharedDB()
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