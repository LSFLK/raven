package models

import "net"

type ClientState struct {
	Authenticated      bool
	SelectedFolder     string
	SelectedMailboxID  int64  // Database ID of selected mailbox
	Conn               net.Conn
	Username           string
	UserID             int64  // Database ID of authenticated user
	DomainID           int64  // Database ID of user's domain
	// Mailbox state tracking for NOOP and other commands
	LastMessageCount   int    // Last known message count in selected folder
	LastRecentCount    int    // Last known recent (unseen) message count
	UIDValidity        int64  // UID validity for selected mailbox
	UIDNext            int64  // Next UID for selected mailbox
	// Role mailbox support
	RoleMailboxIDs     []int64  // Database IDs of role mailboxes assigned to this user
	SelectedRoleMailboxID int64 // Database ID of selected role mailbox (0 if not a role mailbox)
	IsRoleMailbox      bool     // True if currently browsing a role mailbox
}
