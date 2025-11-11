package server

import (
	"fmt"
	"net"
	"strings"
	"time"

	"raven/internal/db"
	"raven/internal/models"
)

// ===== NOOP =====

func (s *IMAPServer) handleNoop(conn net.Conn, tag string, state *models.ClientState) {
	// NOOP can be used before authentication
	// If authenticated and a folder is selected, check for mailbox updates
	// and send untagged responses per RFC 3501
	if state.Authenticated && state.SelectedMailboxID > 0 {
		// Get user database
		userDB, err := s.GetUserDB(state.UserID)
		if err != nil {
			s.sendResponse(conn, fmt.Sprintf("%s OK NOOP completed", tag))
			return
		}

		// Get current mailbox state using new schema
		currentCount, err := db.GetMessageCountPerUser(userDB, state.SelectedMailboxID)
		if err != nil {
			// If there's a database error, just complete normally
			s.sendResponse(conn, fmt.Sprintf("%s OK NOOP completed", tag))
			return
		}

		currentRecent, err := db.GetUnseenCountPerUser(userDB, state.SelectedMailboxID)
		if err != nil {
			currentRecent = 0
		}

		// Debug logging
		fmt.Printf("NOOP Debug: mailbox_id=%d, lastCount=%d, currentCount=%d, lastRecent=%d, currentRecent=%d\n",
			state.SelectedMailboxID, state.LastMessageCount, currentCount, state.LastRecentCount, currentRecent)

		// Check for new messages (EXISTS response)
		if currentCount > state.LastMessageCount {
			s.sendResponse(conn, fmt.Sprintf("* %d EXISTS", currentCount))

			// Calculate new recent messages
			newRecent := currentCount - state.LastMessageCount
			if newRecent > 0 {
				s.sendResponse(conn, fmt.Sprintf("* %d RECENT", newRecent))
			}
		}

		// Check for expunged (deleted) messages
		if currentCount < state.LastMessageCount {
			// Send EXPUNGE for each deleted message
			// Note: In a real implementation, you'd track which specific messages
			// were expunged. Here we send generic expunge notifications.
			for i := state.LastMessageCount; i > currentCount; i-- {
				s.sendResponse(conn, fmt.Sprintf("* %d EXPUNGE", i))
			}
		}

		// Check for flag changes (simplified - just report recent count changes)
		if currentRecent != state.LastRecentCount && currentCount == state.LastMessageCount {
			// Messages exist but recent count changed (flags were modified)
			// In a full implementation, you'd send FETCH responses with updated flags
			// For now, we send an informational message
			if currentRecent > 0 {
				s.sendResponse(conn, fmt.Sprintf("* %d RECENT", currentRecent))
			}
		}

		// Update state tracking
		state.LastMessageCount = currentCount
		state.LastRecentCount = currentRecent
	}

	// Always complete successfully per RFC 3501
	s.sendResponse(conn, fmt.Sprintf("%s OK NOOP completed", tag))
}

// ===== IDLE =====

func (s *IMAPServer) handleIdle(conn net.Conn, tag string, state *models.ClientState) {
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	if state.SelectedMailboxID == 0 {
		s.sendResponse(conn, fmt.Sprintf("%s NO No folder selected", tag))
		return
	}

	// Tell client we're entering idle mode
	s.sendResponse(conn, "+ idling")

	buf := make([]byte, 4096)

	// Get user database
	userDB, err := s.GetUserDB(state.UserID)
	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
		return
	}

	// Track previous state of the folder using new schema
	prevCount, _ := db.GetMessageCountPerUser(userDB, state.SelectedMailboxID)
	prevUnseen, _ := db.GetUnseenCountPerUser(userDB, state.SelectedMailboxID)

	for {
		// Poll every 500ms for changes to ensure responsive notifications
		time.Sleep(500 * time.Millisecond)

		// Check current mailbox state using new schema
		count, _ := db.GetMessageCountPerUser(userDB, state.SelectedMailboxID)
		unseen, _ := db.GetUnseenCountPerUser(userDB, state.SelectedMailboxID)

		// Notify about new messages
		if count > prevCount {
			s.sendResponse(conn, fmt.Sprintf("* %d EXISTS", count))
			newRecent := count - prevCount
			if newRecent > 0 {
				s.sendResponse(conn, fmt.Sprintf("* %d RECENT", newRecent))
			}
		}

		// Notify about expunged (deleted) messages
		if count < prevCount {
			for i := prevCount; i > count; i-- {
				s.sendResponse(conn, fmt.Sprintf("* %d EXPUNGE", i))
			}
		}

		// Notify about unseen count change
		if unseen != prevUnseen {
			s.sendResponse(conn, fmt.Sprintf("* OK [UNSEEN %d] Message %d is first unseen", unseen, unseen))
		}

		// Update cached values
		prevCount = count
		prevUnseen = unseen

		// Check if client sent DONE (non-blocking read)
		conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		n, err := conn.Read(buf)
		if err == nil && strings.TrimSpace(strings.ToUpper(string(buf[:n]))) == "DONE" {
			s.sendResponse(conn, fmt.Sprintf("%s OK IDLE terminated", tag))
			return
		}
	}
}

// ===== NAMESPACE =====

func (s *IMAPServer) handleNamespace(conn net.Conn, tag string, state *models.ClientState) {
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	// Send namespace response - simple single personal namespace
	s.sendResponse(conn, `* NAMESPACE (("" "/")) NIL NIL`)
	s.sendResponse(conn, fmt.Sprintf("%s OK NAMESPACE completed", tag))
}
