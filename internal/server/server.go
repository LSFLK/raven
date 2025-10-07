package server

import (
	"database/sql"
	"net"

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
	s.sendResponse(conn, "* OK [CAPABILITY IMAP4rev1 STARTTLS LOGINDISABLED UIDPLUS IDLE] SQLite IMAP server ready")

	handleClient(s, conn, state)
}

// getUserTableName returns the table name for a specific user
func (s *IMAPServer) getUserTableName(username string) string {
	return db.GetUserTableName(username)
}

// ensureUserTable ensures the user's table exists
func (s *IMAPServer) ensureUserTable(username string) error {
	return db.EnsureUserTable(s.db, username)
}