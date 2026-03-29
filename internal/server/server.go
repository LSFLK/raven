package server

import (
	"database/sql"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"raven/internal/auth/oauthbearer"
	"raven/internal/blobstorage"
	"raven/internal/conf"
	"raven/internal/db"
	"raven/internal/models"
	"raven/internal/server/auth"
)

type IMAPServer struct {
	dbManager *db.DBManager
	certPath  string
	keyPath   string
	s3Storage *blobstorage.S3BlobStorage
	cfg       *conf.Config
	oauthVal  *oauthbearer.Validator
}

func NewIMAPServer(dbManager *db.DBManager) *IMAPServer {
	server := &IMAPServer{
		dbManager: dbManager,
		certPath:  "/certs/fullchain.pem",
		keyPath:   "/certs/privkey.pem",
		s3Storage: nil,
	}

	server.initOAuthValidation()

	return server
}

// NewIMAPServerWithS3 creates a new IMAP server with S3 blob storage support
func NewIMAPServerWithS3(dbManager *db.DBManager, s3Storage *blobstorage.S3BlobStorage) *IMAPServer {
	server := &IMAPServer{
		dbManager: dbManager,
		certPath:  "/certs/fullchain.pem",
		keyPath:   "/certs/privkey.pem",
		s3Storage: s3Storage,
	}

	server.initOAuthValidation()

	return server
}

func (s *IMAPServer) initOAuthValidation() {
	cfg, err := conf.LoadConfig()
	if err != nil {
		log.Printf("IMAP OAUTHBEARER: config load skipped at init: %v", err)
		return
	}

	validator, err := oauthbearer.NewValidator(oauthbearer.Config{
		IssuerURL: cfg.OAuthIssuer,
		JWKSURL:   cfg.OAuthJWKSURL,
		Audiences: cfg.OAuthAudience,
		ClockSkew: time.Duration(cfg.OAuthSkewSec) * time.Second,
	})
	if err != nil {
		log.Printf("IMAP OAUTHBEARER: validator init skipped at startup: %v", err)
		return
	}

	s.cfg = cfg
	s.oauthVal = validator
}

func (s *IMAPServer) oauthSASLReady() bool {
	return s.cfg != nil && s.oauthVal != nil
}

func (s *IMAPServer) greetingCapabilities(isTLS bool) string {
	capabilities := []string{"IMAP4rev1"}

	if isTLS {
		capabilities = append(capabilities, "AUTH=PLAIN", "LOGIN")
	} else {
		capabilities = append(capabilities, "STARTTLS", "LOGINDISABLED")
	}

	if s.oauthSASLReady() {
		capabilities = append(capabilities, "AUTH=OAUTHBEARER", "AUTH=XOAUTH2", "SASL-IR")
	}

	capabilities = append(capabilities, "UIDPLUS", "IDLE", "LITERAL+")

	return strings.Join(capabilities, " ")
}

// GetConfig returns cached process configuration loaded at startup.
func (s *IMAPServer) GetConfig() *conf.Config {
	return s.cfg
}

// GetOAuthValidator returns the shared OAuth validator instance.
func (s *IMAPServer) GetOAuthValidator() *oauthbearer.Validator {
	return s.oauthVal
}

// SetS3Storage sets the S3 blob storage (useful for adding storage after creation)
func (s *IMAPServer) SetS3Storage(s3Storage *blobstorage.S3BlobStorage) {
	s.s3Storage = s3Storage
}

// GetS3Storage returns the S3 blob storage
func (s *IMAPServer) GetS3Storage() *blobstorage.S3BlobStorage {
	return s.s3Storage
}

// SetTLSCertificates sets custom TLS certificate paths (useful for testing)
func (s *IMAPServer) SetTLSCertificates(certPath, keyPath string) {
	s.certPath = certPath
	s.keyPath = keyPath
}

func (s *IMAPServer) HandleConnection(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	state := &models.ClientState{
		Authenticated: false,
		Conn:          conn,
	}

	// Greeting - advertise basic capabilities in greeting
	s.sendResponse(conn, fmt.Sprintf("* OK [CAPABILITY %s] SQLite IMAP server ready", s.greetingCapabilities(false)))

	handleClient(s, conn, state)
}

// ===== Helper functions for new schema =====

// EnsureUserAndMailboxes ensures user database exists and has default mailboxes (exported for commands)
func (s *IMAPServer) EnsureUserAndMailboxes(email string) error {
	// Get user database (this will create default mailboxes if it's a new user)
	_, err := s.dbManager.GetUserDB(email)
	if err != nil {
		return fmt.Errorf("failed to initialize user database: %v", err)
	}
	return nil
}

// GetUserDB returns the database connection for a user (exported for commands)
func (s *IMAPServer) GetUserDB(email string) (*sql.DB, error) {
	return s.dbManager.GetUserDB(email)
}

// GetSelectedDB returns the appropriate database based on client state (exported for commands)
// If a role mailbox is selected, returns the role mailbox database
// Otherwise returns the user's database
func (s *IMAPServer) GetSelectedDB(state *models.ClientState) (*sql.DB, error) {
	if state.IsRoleMailbox {
		roleDB, err := s.dbManager.GetRoleMailboxDB(state.SelectedRoleMailboxID)
		return roleDB, err
	}
	email := resolveStateEmail(state)
	userDB, err := s.dbManager.GetUserDB(email)
	return userDB, err
}

func resolveStateEmail(state *models.ClientState) string {
	if state.Email != "" {
		return state.Email
	}
	if state.Username == "" {
		if state.UserID > 0 {
			if email := getTestUserEmail(state.UserID); email != "" {
				return email
			}
			if email := getSingleTestUserEmail(); email != "" {
				return email
			}
			return fmt.Sprintf("user-%d@localhost", state.UserID)
		}
		return ""
	}
	if strings.Contains(state.Username, "@") {
		return state.Username
	}
	return state.Username + "@localhost"
}

// GetSharedDB returns the shared database connection (exported for commands)
func (s *IMAPServer) GetSharedDB() *sql.DB {
	return s.dbManager.GetSharedDB()
}

// GetDBManager returns the database manager (exported for commands)
func (s *IMAPServer) GetDBManager() *db.DBManager {
	return s.dbManager
}

// GetCertPath returns the TLS certificate path (exported for commands)
func (s *IMAPServer) GetCertPath() string {
	return s.certPath
}

// GetKeyPath returns the TLS key path (exported for commands)
func (s *IMAPServer) GetKeyPath() string {
	return s.keyPath
}

// GetUserDomain extracts domain only from an explicit email value (exported for commands)
func (s *IMAPServer) GetUserDomain(username string) string {
	// If username contains @, extract domain
	if strings.Contains(username, "@") {
		parts := strings.Split(username, "@")
		if len(parts) == 2 {
			return strings.Trim(parts[1], ".")
		}
	}

	return ""
}

// ExtractUsername removes domain from username if present (exported for commands)
func (s *IMAPServer) ExtractUsername(email string) string {
	if strings.Contains(email, "@") {
		parts := strings.Split(email, "@")
		return parts[0]
	}
	return email
}

// HandleSSLConnection handles SSL/TLS connections (delegates to auth package)
func (s *IMAPServer) HandleSSLConnection(conn net.Conn) {
	clientHandler := func(conn net.Conn, state *models.ClientState) {
		// Send greeting for SSL/TLS connections
		// TLS is active, so AUTH=PLAIN and LOGIN are allowed (no STARTTLS needed)
		s.sendResponse(conn, fmt.Sprintf("* OK [CAPABILITY %s] SQLite IMAP server ready", s.greetingCapabilities(true)))
		handleClient(s, conn, state)
	}
	auth.HandleSSLConnection(clientHandler, conn)
}
