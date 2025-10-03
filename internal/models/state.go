package models

import "net"

type ClientState struct {
	Authenticated  bool
	SelectedFolder string
	Conn           net.Conn
	Username       string
	// Mailbox state tracking for NOOP and other commands
	LastMessageCount int // Last known message count in selected folder
	LastRecentCount  int // Last known recent (unseen) message count
}
