package server

import (
	"database/sql"
	"net"

	"go-imap/internal/models"
)

type IMAPServer struct {
	db *sql.DB
}

func NewIMAPServer(db *sql.DB) *IMAPServer {
	return &IMAPServer{db: db}
}

func (s *IMAPServer) HandleConnection(conn net.Conn) {
	defer conn.Close()

	state := &models.ClientState{
		Authenticated: false,
		Conn:          conn,
	}

	// Greeting
	s.sendResponse(conn, "* OK [CAPABILITY IMAP4rev1 STARTTLS UIDPLUS IDLE] SQLite IMAP server ready")

	handleClient(s, conn, state)
}