package models

import "net"

type ClientState struct {
	Authenticated      bool
	SelectedFolder     string
	SelectedMailboxID  int64  // Database ID of selected mailbox
	Conn               net.Conn
	Username           string // Local part of the email address (before @)
	Email              string // Full email address from IDP (username@domain)
	UserID             int64  // Deprecated: kept for backward-compatible tests
	DomainID           int64  // Deprecated: kept for backward-compatible tests
	// Mailbox state tracking for NOOP and other commands
	LastMessageCount   int    // Last known message count in selected folder
	LastRecentCount    int    // Last known recent (unseen) message count
	UIDValidity        int64  // UID validity for selected mailbox
	UIDNext            int64  // Next UID for selected mailbox
}
