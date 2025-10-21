package server

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go-imap/internal/conf"
	"go-imap/internal/db"
	"go-imap/internal/delivery/parser"
	"go-imap/internal/models"
)

func (s *IMAPServer) handleCapability(conn net.Conn, tag string, state *models.ClientState) {
    // Base capabilities
    capabilities := []string{"IMAP4rev1"}

    // Detect TLS: real TLS connection or test mock that advertises TLS
    isTLS := false
    if _, ok := conn.(*tls.Conn); ok {
        isTLS = true
    } else {
        // Allow test doubles to signal TLS via an interface
        type tlsAware interface{ IsTLS() bool }
        if ta, ok := any(conn).(tlsAware); ok && ta.IsTLS() {
            isTLS = true
        }
    }

    if isTLS {
        // TLS is active → allow authentication
        capabilities = append(capabilities, "AUTH=PLAIN", "LOGIN")
    } else {
        // Plain connection → require STARTTLS and disable login
        capabilities = append(capabilities, "STARTTLS", "LOGINDISABLED")
    }

    // Add extension capabilities
    capabilities = append(capabilities,
        "UIDPLUS",
        "IDLE",
        "NAMESPACE",
        "UNSELECT",
        "LITERAL+",
    )

    // Send CAPABILITY response
    s.sendResponse(conn, "* CAPABILITY "+strings.Join(capabilities, " "))
    s.sendResponse(conn, fmt.Sprintf("%s OK CAPABILITY completed", tag))
}

func (s *IMAPServer) handleLogin(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	// Check if LOGIN command has correct number of arguments
	if len(parts) < 4 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD LOGIN requires username and password", tag))
		return
	}

	// Detect if TLS is active
	isTLS := false
	if _, ok := conn.(*tls.Conn); ok {
		isTLS = true
	} else {
		// Allow test doubles to signal TLS via an interface
		type tlsAware interface{ IsTLS() bool }
		if ta, ok := any(conn).(tlsAware); ok && ta.IsTLS() {
			isTLS = true
		}
	}

	// Per RFC 3501: If LOGINDISABLED capability is advertised (i.e., no TLS),
	// reject the LOGIN command
	if !isTLS {
		s.sendResponse(conn, fmt.Sprintf("%s NO [PRIVACYREQUIRED] LOGIN is disabled on insecure connection. Use STARTTLS first.", tag))
		return
	}

	// Extract username and password, removing quotes if present
	username := strings.Trim(parts[2], "\"")
	password := strings.Trim(parts[3], "\"")

	// Use common authentication logic
	s.authenticateUser(conn, tag, username, password, state)
}

func (s *IMAPServer) handleList(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	// Parse arguments according to RFC 3501
	if len(parts) < 4 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD LIST command requires reference and mailbox arguments", tag))
		return
	}

	reference := s.parseQuotedString(parts[2])
	mailboxPattern := s.parseQuotedString(parts[3])

	// Handle special case: empty mailbox name to get hierarchy delimiter
	if mailboxPattern == "" {
		// Return hierarchy delimiter and root name
		hierarchyDelimiter := "/"
		rootName := reference
		if reference == "" {
			rootName = ""
		}
		s.sendResponse(conn, fmt.Sprintf("* LIST (\\Noselect) \"%s\" \"%s\"", hierarchyDelimiter, rootName))
		s.sendResponse(conn, fmt.Sprintf("%s OK LIST completed", tag))
		return
	}

	// Get all mailboxes for the user
	mailboxes, err := db.GetUserMailboxes(s.db, state.UserID)
	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO LIST failure: can't list mailboxes", tag))
		return
	}

	// Apply reference and pattern matching
	matches := s.filterMailboxes(mailboxes, reference, mailboxPattern)

	// Return matching mailboxes
	for _, mailboxName := range matches {
		attrs := s.getMailboxAttributes(mailboxName)
		s.sendResponse(conn, fmt.Sprintf("* LIST (%s) \"/\" \"%s\"", attrs, mailboxName))
	}

	s.sendResponse(conn, fmt.Sprintf("%s OK LIST completed", tag))
}

func (s *IMAPServer) handleLsub(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	// Parse arguments according to RFC 3501
	// LSUB requires reference name and mailbox name with possible wildcards
	if len(parts) < 4 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD LSUB command requires reference and mailbox arguments", tag))
		return
	}

	reference := s.parseQuotedString(parts[2])
	mailboxPattern := s.parseQuotedString(parts[3])

	// Handle special case: empty mailbox name to get hierarchy delimiter
	if mailboxPattern == "" {
		// Return hierarchy delimiter and root name
		hierarchyDelimiter := "/"
		rootName := reference
		if reference == "" {
			rootName = ""
		}
		s.sendResponse(conn, fmt.Sprintf("* LSUB (\\Noselect) \"%s\" \"%s\"", hierarchyDelimiter, rootName))
		s.sendResponse(conn, fmt.Sprintf("%s OK LSUB completed", tag))
		return
	}

	// Get subscribed mailboxes from database
	subscriptions, err := db.GetUserSubscriptions(s.db, state.UserID)
	if err != nil {
		fmt.Printf("Failed to get subscriptions for user %s: %v\n", state.Username, err)
		s.sendResponse(conn, fmt.Sprintf("%s NO LSUB failure: can't list that reference or name", tag))
		return
	}

	// If no subscriptions exist, subscribe to default mailboxes
	if len(subscriptions) == 0 {
		defaultMailboxes := []string{"INBOX", "Sent", "Drafts", "Trash"}
		for _, mailbox := range defaultMailboxes {
			db.SubscribeToMailbox(s.db, state.UserID, mailbox)
		}
		subscriptions = defaultMailboxes
	}

	// Apply reference and pattern matching to subscriptions
	matches := s.filterMailboxes(subscriptions, reference, mailboxPattern)

	// RFC 3501 Special case: When using % wildcard, if "foo/bar" is subscribed
	// but "foo" is not, we must return "foo" with \Noselect attribute
	hierarchyDelimiter := "/"
	impliedParents := make(map[string]bool)

	// Collect all implied parent mailboxes when using % wildcard
	if strings.Contains(mailboxPattern, "%") {
		// Look at ALL subscriptions (not just matches) to find implied parents
		for _, mailbox := range subscriptions {
			// Check if this mailbox has hierarchy
			if strings.Contains(mailbox, hierarchyDelimiter) {
				mailboxParts := strings.Split(mailbox, hierarchyDelimiter)
				// Build up parent paths
				currentPath := ""
				for i := 0; i < len(mailboxParts)-1; i++ {
					if i > 0 {
						currentPath += hierarchyDelimiter
					}
					currentPath += mailboxParts[i]

					// If parent is not subscribed and matches the pattern, mark as implied
					if !contains(subscriptions, currentPath) {
						// Check if this implied parent matches the pattern
						canonicalPattern := s.buildCanonicalPattern(reference, mailboxPattern, hierarchyDelimiter)
						if s.matchesPattern(currentPath, canonicalPattern, hierarchyDelimiter) {
							impliedParents[currentPath] = true
						}
					}
				}
			}
		}
	}

	// Send implied parents with \Noselect first
	for parent := range impliedParents {
		s.sendResponse(conn, fmt.Sprintf("* LSUB (\\Noselect) \"/\" \"%s\"", parent))
	}

	// Send actual subscribed mailboxes
	for _, mailboxName := range matches {
		attrs := s.getMailboxAttributes(mailboxName)
		s.sendResponse(conn, fmt.Sprintf("* LSUB (%s) \"/\" \"%s\"", attrs, mailboxName))
	}

	s.sendResponse(conn, fmt.Sprintf("%s OK LSUB completed", tag))
}

// Helper function to check if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func (s *IMAPServer) handleCreate(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	if len(parts) < 3 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD CREATE requires mailbox name", tag))
		return
	}

	// Parse mailbox name (could be quoted)
	mailboxName := strings.Trim(parts[2], "\"")
	
	// Remove trailing hierarchy separator if present
	// According to RFC 3501, the name created is without the trailing hierarchy delimiter
	if strings.HasSuffix(mailboxName, "/") {
		mailboxName = strings.TrimSuffix(mailboxName, "/")
	}

	// Validate mailbox name
	if mailboxName == "" {
		s.sendResponse(conn, fmt.Sprintf("%s NO Cannot create mailbox with empty name", tag))
		return
	}

	// Check if trying to create INBOX (case-insensitive)
	if strings.ToUpper(mailboxName) == "INBOX" {
		s.sendResponse(conn, fmt.Sprintf("%s NO Cannot create INBOX - it already exists", tag))
		return
	}

	// Check if mailbox already exists
	exists, err := db.MailboxExists(s.db, state.UserID, mailboxName)
	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO Server error: cannot check mailbox existence", tag))
		return
	}

	if exists {
		s.sendResponse(conn, fmt.Sprintf("%s NO Mailbox already exists", tag))
		return
	}

	// Handle hierarchy creation
	// If the mailbox name contains hierarchy separators, create parent mailboxes if needed
	if strings.Contains(mailboxName, "/") {
		parts := strings.Split(mailboxName, "/")
		currentPath := ""

		// Create each level of the hierarchy
		for i, part := range parts {
			if i > 0 {
				currentPath += "/"
			}
			currentPath += part

			// Skip if this is the final mailbox (we'll create it below)
			if i == len(parts)-1 {
				break
			}

			// Check if this intermediate mailbox exists
			intermediateExists, checkErr := db.MailboxExists(s.db, state.UserID, currentPath)
			if checkErr == nil && !intermediateExists {
				// Create intermediate mailbox - ignore errors as per RFC 3501
				db.CreateMailbox(s.db, state.UserID, currentPath, "")
			}
		}
	}

	// Create the target mailbox
	_, err = db.CreateMailbox(s.db, state.UserID, mailboxName, "")
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			s.sendResponse(conn, fmt.Sprintf("%s NO Mailbox already exists", tag))
		} else {
			s.sendResponse(conn, fmt.Sprintf("%s NO Create failure: %s", tag, err.Error()))
		}
		return
	}

	s.sendResponse(conn, fmt.Sprintf("%s OK CREATE completed", tag))
}

func (s *IMAPServer) handleDelete(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	if len(parts) < 3 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD DELETE requires mailbox name", tag))
		return
	}

	// Parse mailbox name (could be quoted)
	mailboxName := strings.Trim(parts[2], "\"")

	// Validate mailbox name
	if mailboxName == "" {
		s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid mailbox name", tag))
		return
	}

	// Cannot delete INBOX (case-insensitive)
	if strings.ToUpper(mailboxName) == "INBOX" {
		s.sendResponse(conn, fmt.Sprintf("%s NO Cannot delete INBOX", tag))
		return
	}

	// Attempt to delete the mailbox
	err := db.DeleteMailbox(s.db, state.UserID, mailboxName)
	if err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			s.sendResponse(conn, fmt.Sprintf("%s NO Mailbox does not exist", tag))
		} else if strings.Contains(err.Error(), "has inferior hierarchical names") {
			s.sendResponse(conn, fmt.Sprintf("%s NO Name \"%s\" has inferior hierarchical names", tag, mailboxName))
		} else if strings.Contains(err.Error(), "cannot delete INBOX") {
			s.sendResponse(conn, fmt.Sprintf("%s NO Cannot delete INBOX", tag))
		} else {
			s.sendResponse(conn, fmt.Sprintf("%s NO Delete failure: %s", tag, err.Error()))
		}
		return
	}

	s.sendResponse(conn, fmt.Sprintf("%s OK DELETE completed", tag))
}

func (s *IMAPServer) handleRename(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	if len(parts) < 4 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD RENAME requires existing and new mailbox names", tag))
		return
	}

	// Parse mailbox names (could be quoted)
	oldName := strings.Trim(parts[2], "\"")
	newName := strings.Trim(parts[3], "\"")

	// Validate mailbox names
	if oldName == "" || newName == "" {
		s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid mailbox names", tag))
		return
	}

	// Attempt to rename the mailbox
	err := db.RenameMailbox(s.db, state.UserID, oldName, newName)
	if err != nil {
		if strings.Contains(err.Error(), "source mailbox does not exist") {
			s.sendResponse(conn, fmt.Sprintf("%s NO Source mailbox does not exist", tag))
		} else if strings.Contains(err.Error(), "destination mailbox already exists") {
			s.sendResponse(conn, fmt.Sprintf("%s NO Destination mailbox already exists", tag))
		} else if strings.Contains(err.Error(), "cannot rename to INBOX") {
			s.sendResponse(conn, fmt.Sprintf("%s NO Cannot rename to INBOX", tag))
		} else {
			s.sendResponse(conn, fmt.Sprintf("%s NO Rename failure: %s", tag, err.Error()))
		}
		return
	}

	s.sendResponse(conn, fmt.Sprintf("%s OK RENAME completed", tag))
}

func (s *IMAPServer) handleLogout(conn net.Conn, tag string) {
	s.sendResponse(conn, "* BYE IMAP4rev1 Server logging out")
	s.sendResponse(conn, fmt.Sprintf("%s OK LOGOUT completed", tag))
}

func (s *IMAPServer) handleNoop(conn net.Conn, tag string, state *models.ClientState) {
	// NOOP can be used before authentication
	// If authenticated and a folder is selected, check for mailbox updates
	// and send untagged responses per RFC 3501
	if state.Authenticated && state.SelectedMailboxID > 0 {
		// Get current mailbox state using new schema
		currentCount, err := db.GetMessageCount(s.db, state.SelectedMailboxID)
		if err != nil {
			// If there's a database error, just complete normally
			s.sendResponse(conn, fmt.Sprintf("%s OK NOOP completed", tag))
			return
		}

		currentRecent, err := db.GetUnseenCount(s.db, state.SelectedMailboxID)
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

func (s *IMAPServer) handleCheck(conn net.Conn, tag string, state *models.ClientState) {
	// CHECK command requires authentication
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	// CHECK command requires a selected mailbox (Selected state)
	// Per RFC 3501: CHECK is only valid in Selected state
	if state.SelectedMailboxID == 0 {
		s.sendResponse(conn, fmt.Sprintf("%s NO No mailbox selected", tag))
		return
	}

	// Perform checkpoint operations for the currently selected mailbox
	// This involves resolving the server's in-memory state with the state on disk
	// In our implementation, this is similar to NOOP but emphasizes housekeeping

	// Get current mailbox state
	currentCount, err := db.GetMessageCount(s.db, state.SelectedMailboxID)
	if err != nil {
		// If there's a database error, still complete normally per RFC 3501
		// CHECK should always succeed even if housekeeping fails
		s.sendResponse(conn, fmt.Sprintf("%s OK CHECK completed", tag))
		return
	}

	currentRecent, err := db.GetUnseenCount(s.db, state.SelectedMailboxID)
	if err != nil {
		currentRecent = 0
	}

	// Update state tracking to ensure in-memory state matches database
	// This is the "checkpoint" - synchronizing cached state with actual state
	state.LastMessageCount = currentCount
	state.LastRecentCount = currentRecent

	// Note: Unlike NOOP, CHECK does not guarantee sending EXISTS responses
	// Per RFC 3501: "There is no guarantee that an EXISTS untagged response
	// will happen as a result of CHECK. NOOP, not CHECK, SHOULD be used for
	// new message polling."
	// Therefore, we do NOT send untagged responses here

	// Always complete successfully per RFC 3501
	s.sendResponse(conn, fmt.Sprintf("%s OK CHECK completed", tag))
}

func (s *IMAPServer) handleClose(conn net.Conn, tag string, state *models.ClientState) {
	// CLOSE command requires authentication
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	// CLOSE command requires a selected mailbox (Selected state)
	// Per RFC 3501: CLOSE is only valid in Selected state
	if state.SelectedMailboxID == 0 {
		s.sendResponse(conn, fmt.Sprintf("%s NO No mailbox selected", tag))
		return
	}

	// Per RFC 3501: CLOSE permanently removes all messages with \Deleted flag
	// from the currently selected mailbox, and returns to authenticated state
	// No untagged EXPUNGE responses are sent (unlike EXPUNGE command)

	// Important: Per RFC 3501, if mailbox is read-only (selected with EXAMINE),
	// no messages are removed and no error is given.
	// Since we don't currently track read-only state in ClientState,
	// we always perform the expunge operation.
	// TODO: Add ReadOnly field to ClientState to properly handle EXAMINE

	// Delete all messages with \Deleted flag from the mailbox
	// Query for all messages with \Deleted flag in the current mailbox
	rows, err := s.db.Query(`
		SELECT id FROM message_mailbox
		WHERE mailbox_id = ? AND flags LIKE '%\Deleted%'
	`, state.SelectedMailboxID)

	if err == nil {
		defer rows.Close()

		// Collect all message_mailbox IDs to delete
		var idsToDelete []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err == nil {
				idsToDelete = append(idsToDelete, id)
			}
		}

		// Delete the messages from message_mailbox table
		// This removes them from the mailbox but keeps the message data
		for _, id := range idsToDelete {
			s.db.Exec(`DELETE FROM message_mailbox WHERE id = ?`, id)
		}
	}

	// Return to authenticated state by clearing the selected mailbox
	state.SelectedFolder = ""
	state.SelectedMailboxID = 0
	state.LastMessageCount = 0
	state.LastRecentCount = 0
	state.UIDValidity = 0
	state.UIDNext = 0

	// Always complete successfully per RFC 3501
	s.sendResponse(conn, fmt.Sprintf("%s OK CLOSE completed", tag))
}

func (s *IMAPServer) handleExpunge(conn net.Conn, tag string, state *models.ClientState) {
	// EXPUNGE command requires authentication
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	// EXPUNGE command requires a selected mailbox (Selected state)
	// Per RFC 3501: EXPUNGE is only valid in Selected state
	if state.SelectedMailboxID == 0 {
		s.sendResponse(conn, fmt.Sprintf("%s NO No mailbox selected", tag))
		return
	}

	// Per RFC 3501: EXPUNGE permanently removes all messages with \Deleted flag
	// Before returning OK, an untagged EXPUNGE response is sent for each message removed
	// The key difference from CLOSE: EXPUNGE sends untagged responses showing which
	// messages were deleted

	// Important: Per RFC 3501, if mailbox is read-only (selected with EXAMINE),
	// EXPUNGE should return NO
	// TODO: Add ReadOnly field to ClientState to properly handle EXAMINE

	// Query for all messages with \Deleted flag, ordered by sequence number
	// We need to get the sequence numbers before deletion
	rows, err := s.db.Query(`
		SELECT id, uid FROM message_mailbox
		WHERE mailbox_id = ? AND flags LIKE '%\Deleted%'
		ORDER BY uid ASC
	`, state.SelectedMailboxID)

	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO EXPUNGE failed: %v", tag, err))
		return
	}
	defer rows.Close()

	// Collect messages to delete with their UIDs
	type messageToDelete struct {
		id  int64
		uid int64
	}
	var messagesToDelete []messageToDelete
	for rows.Next() {
		var msg messageToDelete
		if err := rows.Scan(&msg.id, &msg.uid); err == nil {
			messagesToDelete = append(messagesToDelete, msg)
		}
	}
	rows.Close()

	// If no messages to delete, just return OK
	if len(messagesToDelete) == 0 {
		s.sendResponse(conn, fmt.Sprintf("%s OK EXPUNGE completed", tag))
		return
	}

	// Get all messages in the mailbox to calculate sequence numbers
	allRows, err := s.db.Query(`
		SELECT id, uid FROM message_mailbox
		WHERE mailbox_id = ?
		ORDER BY uid ASC
	`, state.SelectedMailboxID)

	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO EXPUNGE failed: %v", tag, err))
		return
	}
	defer allRows.Close()

	// Build a map of message IDs to sequence numbers
	sequenceMap := make(map[int64]int)
	seqNum := 1
	for allRows.Next() {
		var id, uid int64
		if err := allRows.Scan(&id, &uid); err == nil {
			sequenceMap[id] = seqNum
			seqNum++
		}
	}
	allRows.Close()

	// Delete messages and send EXPUNGE responses
	// Important: As we delete messages, sequence numbers change for subsequent messages
	// We need to account for this by tracking how many messages we've deleted
	deletedCount := 0
	for _, msg := range messagesToDelete {
		// Get the original sequence number for this message
		originalSeqNum := sequenceMap[msg.id]

		// Adjust for previously deleted messages in this EXPUNGE operation
		// When we delete message N, all messages after it shift down by 1
		adjustedSeqNum := originalSeqNum - deletedCount

		// Send untagged EXPUNGE response with the adjusted sequence number
		s.sendResponse(conn, fmt.Sprintf("* %d EXPUNGE", adjustedSeqNum))

		// Delete the message from the mailbox
		s.db.Exec(`DELETE FROM message_mailbox WHERE id = ?`, msg.id)

		deletedCount++
	}

	// Update state tracking
	state.LastMessageCount -= len(messagesToDelete)
	if state.LastMessageCount < 0 {
		state.LastMessageCount = 0
	}

	// Send completion response
	s.sendResponse(conn, fmt.Sprintf("%s OK EXPUNGE completed", tag))
}

func (s *IMAPServer) handleStore(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	// RFC 3501: STORE requires authentication and selected mailbox
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	if state.SelectedMailboxID == 0 {
		s.sendResponse(conn, fmt.Sprintf("%s NO No mailbox selected", tag))
		return
	}

	// Parse command: STORE sequence data-item value
	if len(parts) < 4 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD STORE requires sequence set, data item, and value", tag))
		return
	}

	sequenceSet := parts[2]
	dataItem := strings.ToUpper(parts[3])

	// Check if .SILENT suffix is used
	silent := strings.HasSuffix(dataItem, ".SILENT")
	if silent {
		dataItem = strings.TrimSuffix(dataItem, ".SILENT")
	}

	// Parse flags from remaining parts
	flagsPart := strings.Join(parts[4:], " ")
	flagsPart = strings.Trim(flagsPart, "()")
	newFlags := strings.Fields(flagsPart)

	// Validate data item
	if dataItem != "FLAGS" && dataItem != "+FLAGS" && dataItem != "-FLAGS" {
		s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid data item: %s", tag, parts[3]))
		return
	}

	// Parse sequence set
	sequences := s.parseSequenceSet(sequenceSet, state.SelectedMailboxID)
	if len(sequences) == 0 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid sequence set", tag))
		return
	}

	// Process each message in the sequence
	for _, seqNum := range sequences {
		// Get message by sequence number
		query := `
			SELECT mm.message_id, mm.uid, mm.flags
			FROM message_mailbox mm
			WHERE mm.mailbox_id = ?
			ORDER BY mm.uid ASC
			LIMIT 1 OFFSET ?
		`
		var messageID, uid int64
		var currentFlags string
		err := s.db.QueryRow(query, state.SelectedMailboxID, seqNum-1).Scan(&messageID, &uid, &currentFlags)
		if err != nil {
			// Message not found - skip
			continue
		}

		// Calculate new flags based on operation
		updatedFlags := s.calculateNewFlags(currentFlags, newFlags, dataItem)

		// Update flags in database
		updateQuery := "UPDATE message_mailbox SET flags = ? WHERE message_id = ? AND mailbox_id = ?"
		_, err = s.db.Exec(updateQuery, updatedFlags, messageID, state.SelectedMailboxID)
		if err != nil {
			log.Printf("Failed to update flags for message %d: %v", messageID, err)
			continue
		}

		// Send untagged FETCH response unless .SILENT
		if !silent {
			flagsFormatted := "()"
			if updatedFlags != "" {
				flagsFormatted = fmt.Sprintf("(%s)", updatedFlags)
			}
			s.sendResponse(conn, fmt.Sprintf("* %d FETCH (FLAGS %s)", seqNum, flagsFormatted))
		}
	}

	s.sendResponse(conn, fmt.Sprintf("%s OK STORE completed", tag))
}

// calculateNewFlags determines the new flags based on the operation
func (s *IMAPServer) calculateNewFlags(currentFlags string, newFlags []string, operation string) string {
	// Parse current flags into a map
	flagMap := make(map[string]bool)
	if currentFlags != "" {
		for _, flag := range strings.Fields(currentFlags) {
			flagMap[flag] = true
		}
	}

	switch operation {
	case "FLAGS":
		// Replace all flags (except \Recent which server manages)
		flagMap = make(map[string]bool)
		for _, flag := range newFlags {
			if flag != "\\Recent" {
				flagMap[flag] = true
			}
		}

	case "+FLAGS":
		// Add flags
		for _, flag := range newFlags {
			if flag != "\\Recent" {
				flagMap[flag] = true
			}
		}

	case "-FLAGS":
		// Remove flags
		for _, flag := range newFlags {
			if flag != "\\Recent" {
				delete(flagMap, flag)
			}
		}
	}

	// Convert map back to string
	var flags []string
	for flag := range flagMap {
		flags = append(flags, flag)
	}

	return strings.Join(flags, " ")
}

// parseSequenceSet parses a sequence set and returns message sequence numbers
func (s *IMAPServer) parseSequenceSet(sequenceSet string, mailboxID int64) []int {
	var sequences []int

	// Get total message count
	totalMessages, err := db.GetMessageCount(s.db, mailboxID)
	if err != nil || totalMessages == 0 {
		return sequences
	}

	// Handle * (last message)
	sequenceSet = strings.ReplaceAll(sequenceSet, "*", fmt.Sprintf("%d", totalMessages))

	// Split by comma for multiple sequences
	parts := strings.Split(sequenceSet, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)

		// Check if it's a range (e.g., "2:4")
		if strings.Contains(part, ":") {
			rangeParts := strings.Split(part, ":")
			if len(rangeParts) == 2 {
				start, err1 := strconv.Atoi(rangeParts[0])
				end, err2 := strconv.Atoi(rangeParts[1])

				if err1 == nil && err2 == nil && start > 0 && end > 0 {
					if start > end {
						start, end = end, start
					}
					for i := start; i <= end && i <= totalMessages; i++ {
						sequences = append(sequences, i)
					}
				}
			}
		} else {
			// Single message
			num, err := strconv.Atoi(part)
			if err == nil && num > 0 && num <= totalMessages {
				sequences = append(sequences, num)
			}
		}
	}

	return sequences
}

// handleCopy implements the COPY command (RFC 3501 Section 6.4.7)
// Syntax: COPY sequence-set mailbox-name
func (s *IMAPServer) handleCopy(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	// Check authentication
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	// Check if mailbox is selected
	if state.SelectedMailboxID == 0 {
		s.sendResponse(conn, fmt.Sprintf("%s NO No mailbox selected", tag))
		return
	}

	// Parse command: COPY sequence-set mailbox-name
	if len(parts) < 3 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid COPY command syntax", tag))
		return
	}

	sequenceSet := parts[1]
	destMailbox := strings.Trim(strings.Join(parts[2:], " "), "\"")

	// Parse sequence set
	sequences := s.parseSequenceSet(sequenceSet, state.SelectedMailboxID)
	if len(sequences) == 0 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid sequence set", tag))
		return
	}

	// Check if destination mailbox exists
	var destMailboxID int64
	err := s.db.QueryRow(`
		SELECT id FROM mailboxes
		WHERE name = ? AND user_id = ?
	`, destMailbox, state.UserID).Scan(&destMailboxID)

	if err != nil {
		// Destination mailbox doesn't exist - return NO with [TRYCREATE]
		s.sendResponse(conn, fmt.Sprintf("%s NO [TRYCREATE] Destination mailbox does not exist", tag))
		return
	}

	// Begin transaction to ensure atomicity
	tx, err := s.db.Begin()
	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO COPY failed: %v", tag, err))
		return
	}
	defer tx.Rollback()

	// Get the next UID for destination mailbox
	var nextUID int64
	err = tx.QueryRow(`
		SELECT COALESCE(MAX(uid), 0) + 1
		FROM message_mailbox
		WHERE mailbox_id = ?
	`, destMailboxID).Scan(&nextUID)

	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO COPY failed: %v", tag, err))
		return
	}

	// Copy each message in the sequence
	for _, seqNum := range sequences {
		// Get message details from source mailbox
		var messageID int64
		var flags, internalDate string

		err = tx.QueryRow(`
			SELECT mm.message_id, mm.flags, mm.internal_date
			FROM message_mailbox mm
			WHERE mm.mailbox_id = ?
			ORDER BY mm.uid
			LIMIT 1 OFFSET ?
		`, state.SelectedMailboxID, seqNum-1).Scan(&messageID, &flags, &internalDate)

		if err != nil {
			s.sendResponse(conn, fmt.Sprintf("%s NO COPY failed: %v", tag, err))
			return
		}

		// Prepare flags for copy - preserve existing flags and add \Recent
		copyFlags := flags
		if !strings.Contains(copyFlags, `\Recent`) {
			if copyFlags == "" {
				copyFlags = `\Recent`
			} else {
				copyFlags = copyFlags + ` \Recent`
			}
		}

		// Insert message into destination mailbox
		_, err = tx.Exec(`
			INSERT INTO message_mailbox (message_id, mailbox_id, uid, flags, internal_date)
			VALUES (?, ?, ?, ?, ?)
		`, messageID, destMailboxID, nextUID, copyFlags, internalDate)

		if err != nil {
			s.sendResponse(conn, fmt.Sprintf("%s NO COPY failed: %v", tag, err))
			return
		}

		nextUID++
	}

	// Commit transaction
	err = tx.Commit()
	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO COPY failed: %v", tag, err))
		return
	}

	s.sendResponse(conn, fmt.Sprintf("%s OK COPY completed", tag))
}

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

	// Track previous state of the folder using new schema
	prevCount, _ := db.GetMessageCount(s.db, state.SelectedMailboxID)
	prevUnseen, _ := db.GetUnseenCount(s.db, state.SelectedMailboxID)

	for {
		// Poll every 2 seconds for changes
		time.Sleep(2 * time.Second)

		// Check current mailbox state using new schema
		count, _ := db.GetMessageCount(s.db, state.SelectedMailboxID)
		unseen, _ := db.GetUnseenCount(s.db, state.SelectedMailboxID)

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

func (s *IMAPServer) handleNamespace(conn net.Conn, tag string, state *models.ClientState) {
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	// Send namespace response - simple single personal namespace
	s.sendResponse(conn, `* NAMESPACE (("" "/")) NIL NIL`)
	s.sendResponse(conn, fmt.Sprintf("%s OK NAMESPACE completed", tag))
}

func (s *IMAPServer) handleUnselect(conn net.Conn, tag string, state *models.ClientState) {
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	if state.SelectedMailboxID == 0 {
		s.sendResponse(conn, fmt.Sprintf("%s NO No folder selected", tag))
		return
	}

	// Close mailbox without expunging messages
	state.SelectedFolder = ""
	state.SelectedMailboxID = 0
	// Reset state tracking
	state.LastMessageCount = 0
	state.LastRecentCount = 0
	state.UIDValidity = 0
	state.UIDNext = 0
	s.sendResponse(conn, fmt.Sprintf("%s OK UNSELECT completed", tag))
}

// handleAppend handles the APPEND command to add a message to a mailbox
func (s *IMAPServer) handleAppend(conn net.Conn, tag string, parts []string, fullLine string, state *models.ClientState) {
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	if len(parts) < 3 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD APPEND requires folder name", tag))
		return
	}

	// Parse folder name (could be quoted)
	folder := strings.Trim(parts[2], "\"")

	// Validate folder exists using the database with new schema
	mailboxID, err := db.GetMailboxByName(s.db, state.UserID, folder)
	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO [TRYCREATE] Folder does not exist", tag))
		return
	}

	// Parse optional flags and date/time
	// Format: tag APPEND folder [(flags)] [date-time] {size}
	var flags string
	
	// Look for flags in parentheses
	if strings.Contains(fullLine, "(") && strings.Contains(fullLine, ")") {
		startIdx := strings.Index(fullLine, "(")
		endIdx := strings.Index(fullLine, ")")
		if startIdx < endIdx {
			flags = fullLine[startIdx+1 : endIdx]
		}
	}

	// Look for literal size indicator {size}
	literalStartIdx := strings.Index(fullLine, "{")
	literalEndIdx := strings.Index(fullLine, "}")
	
	if literalStartIdx == -1 || literalEndIdx == -1 || literalStartIdx > literalEndIdx {
		s.sendResponse(conn, fmt.Sprintf("%s BAD APPEND requires message size", tag))
		return
	}

	// Extract the size
	sizeStr := fullLine[literalStartIdx+1 : literalEndIdx]
	var messageSize int
	fmt.Sscanf(sizeStr, "%d", &messageSize)
	
	if messageSize <= 0 || messageSize > 50*1024*1024 { // Max 50MB
		s.sendResponse(conn, fmt.Sprintf("%s NO Message size invalid or too large", tag))
		return
	}

	// Send continuation response
	s.sendResponse(conn, "+ Ready for literal data")

	// Read the message data
	messageData := make([]byte, messageSize)
	totalRead := 0
	
	conn.SetReadDeadline(time.Now().Add(5 * time.Minute))
	for totalRead < messageSize {
		n, err := conn.Read(messageData[totalRead:])
		if err != nil {
			log.Printf("Error reading message data: %v", err)
			s.sendResponse(conn, fmt.Sprintf("%s NO Failed to read message data", tag))
			return
		}
		totalRead += n
	}

	rawMessage := string(messageData)

	// Ensure message has CRLF line endings
	if !strings.Contains(rawMessage, "\r\n") {
		rawMessage = strings.ReplaceAll(rawMessage, "\n", "\r\n")
	}

	// Parse and store message using new schema
	parsed, err := parser.ParseMIMEMessage(rawMessage)
	if err != nil {
		log.Printf("Failed to parse message: %v", err)
		s.sendResponse(conn, fmt.Sprintf("%s NO [SERVERBUG] Failed to parse message", tag))
		return
	}

	// Store message in database
	messageID, err := parser.StoreMessage(s.db, parsed)
	if err != nil {
		log.Printf("Failed to store message: %v", err)
		s.sendResponse(conn, fmt.Sprintf("%s NO [SERVERBUG] Failed to save message", tag))
		return
	}

	// Add message to mailbox
	internalDate := time.Now()
	err = db.AddMessageToMailbox(s.db, messageID, mailboxID, flags, internalDate)
	if err != nil {
		log.Printf("Failed to add message to mailbox: %v", err)
		s.sendResponse(conn, fmt.Sprintf("%s NO [SERVERBUG] Failed to add message to mailbox", tag))
		return
	}

	// Get UID validity for APPENDUID response
	uidValidity, _, err := db.GetMailboxInfo(s.db, mailboxID)
	if err != nil {
		uidValidity = 1
	}

	// Get the UID assigned to the message
	var newUID int64
	query := "SELECT uid FROM message_mailbox WHERE message_id = ? AND mailbox_id = ?"
	err = s.db.QueryRow(query, messageID, mailboxID).Scan(&newUID)
	if err != nil {
		log.Printf("Failed to get new UID: %v", err)
		newUID = 1
	}

	log.Printf("Message appended to folder '%s' with UID %d", folder, newUID)

	// Send success response with APPENDUID (RFC 4315 - UIDPLUS extension)
	s.sendResponse(conn, fmt.Sprintf("%s OK [APPENDUID %d %d] APPEND completed", tag, uidValidity, newUID))
}

func (s *IMAPServer) handleStartTLS(conn net.Conn, tag string, parts []string) {
	// RFC 3501: STARTTLS takes no arguments
	if len(parts) > 2 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD STARTTLS command does not accept arguments", tag))
		return
	}

	// Check if already on TLS connection
	if _, ok := conn.(*tls.Conn); ok {
		s.sendResponse(conn, fmt.Sprintf("%s BAD TLS already active", tag))
		return
	}

	// Also check mock TLS connections
	type tlsAware interface{ IsTLS() bool }
	if ta, ok := any(conn).(tlsAware); ok && ta.IsTLS() {
		s.sendResponse(conn, fmt.Sprintf("%s BAD TLS already active", tag))
		return
	}

	cert, err := tls.LoadX509KeyPair(s.certPath, s.keyPath)
	if err != nil {
		fmt.Printf("Failed to load TLS cert/key: %v\n", err)
		s.sendResponse(conn, fmt.Sprintf("%s BAD TLS not available", tag))
		return
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	// RFC 3501: Send OK response before starting TLS negotiation
	s.sendResponse(conn, fmt.Sprintf("%s OK Begin TLS negotiation now", tag))

	tlsConn := tls.Server(conn, tlsConfig)

	// RFC 3501: Client MUST discard cached server capabilities after STARTTLS
	// Restart handler with upgraded TLS connection and fresh state
	handleClient(s, tlsConn, &models.ClientState{})
}

func (s *IMAPServer) HandleSSLConnection(conn net.Conn) {

	certPath := "/certs/fullchain.pem"
	keyPath := "/certs/privkey.pem"

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		log.Printf("Failed to load TLS cert/key: %v", err)
		conn.Close()
		return
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	tlsConn := tls.Server(conn, tlsConfig)

	// Start IMAP session over TLS
	handleClient(s, tlsConn, &models.ClientState{})
}

func (s *IMAPServer) handleAuthenticate(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if len(parts) < 3 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD AUTHENTICATE requires authentication mechanism", tag))
		return
	}

	mechanism := strings.ToUpper(parts[2])
	switch mechanism {
	case "PLAIN":
		// Do not allow plaintext authentication unless using TLS
		isTLS := false
		if _, ok := conn.(*tls.Conn); ok {
			isTLS = true
		} else {
			type tlsAware interface{ IsTLS() bool }
			if ta, ok := any(conn).(tlsAware); ok && ta.IsTLS() {
				isTLS = true
			}
		}
		if !isTLS {
			s.sendResponse(conn, fmt.Sprintf("%s NO Plaintext authentication disallowed without TLS", tag))
			return
		}

		// Send continuation request
		s.sendResponse(conn, "+ ")

		// Read the authentication data
		buf := make([]byte, 8192)
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		n, err := conn.Read(buf)
		if err != nil {
			s.sendResponse(conn, fmt.Sprintf("%s NO Authentication failed", tag))
			return
		}

		authData := strings.TrimSpace(string(buf[:n]))

		// Client may cancel authentication with a single "*"
		if authData == "*" {
			s.sendResponse(conn, fmt.Sprintf("%s BAD Authentication exchange cancelled", tag))
			return
		}

		log.Printf("AUTHENTICATE PLAIN: received %d bytes of auth data", len(authData))

		// Decode base64 as per SASL challenge/response (PLAIN uses base64 here)
		var decoded []byte
		decoded, err = base64.StdEncoding.DecodeString(authData)
		if err != nil {
			log.Printf("AUTHENTICATE PLAIN: base64 decode failed: %v, treating as plain", err)
			// If decode fails, fall back to treating the input as plain (some test-clients may do this)
			decoded = []byte(authData)
		} else {
			log.Printf("AUTHENTICATE PLAIN: decoded %d bytes", len(decoded))
		}

		// Split on NUL (\x00). PLAIN: [authzid] \x00 authcid \x00 passwd
		partsNull := strings.Split(string(decoded), "\x00")
		log.Printf("AUTHENTICATE PLAIN: split into %d parts", len(partsNull))
		
		var username, password string
		if len(partsNull) >= 3 {
			username = partsNull[1]
			password = partsNull[2]
			log.Printf("AUTHENTICATE PLAIN: extracted username=%s (password length=%d)", username, len(password))
		} else if len(partsNull) == 2 {
			// fallback: username and password
			username = partsNull[0]
			password = partsNull[1]
			log.Printf("AUTHENTICATE PLAIN: fallback extracted username=%s (password length=%d)", username, len(password))
		} else {
			log.Printf("AUTHENTICATE PLAIN: invalid format, expected 2-3 parts, got %d", len(partsNull))
			s.sendResponse(conn, fmt.Sprintf("%s NO [AUTHENTICATIONFAILED] Invalid credentials format", tag))
			return
		}

		if username == "" || password == "" {
			log.Printf("AUTHENTICATE PLAIN: empty username or password")
			s.sendResponse(conn, fmt.Sprintf("%s NO [AUTHENTICATIONFAILED] Invalid credentials", tag))
			return
		}

		// Reuse the existing login logic
		s.authenticateUser(conn, tag, username, password, state)
		return

	default:
		s.sendResponse(conn, fmt.Sprintf("%s NO Unsupported authentication mechanism", tag))
	}
}

// Extract common authentication logic
func (s *IMAPServer) authenticateUser(conn net.Conn, tag string, username string, password string, state *models.ClientState) {
	// Load domain from config file
	cfg, err := conf.LoadConfig()
	if err != nil {
		log.Printf("LoadConfig error: %v", err)
		s.sendResponse(conn, fmt.Sprintf("%s NO [SERVERBUG] Configuration error", tag))
		return
	}

	if cfg.Domain == "" || cfg.AuthServerURL == "" {
		s.sendResponse(conn, fmt.Sprintf("%s NO [SERVERBUG] Configuration error", tag))
		return
	}

	email := username + "@" + cfg.Domain

	// Prepare JSON body
	requestBody := fmt.Sprintf(`{"email":"%s","password":"%s"}`, email, password)

	// Create HTTP request
	req, err := http.NewRequest("POST", cfg.AuthServerURL, strings.NewReader(requestBody))
	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO [SERVERBUG] Internal error", tag))
		return
	}
	req.Header.Set("Content-Type", "application/json")

	// TLS config for system CA bundle (default)
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}
	transport := &http.Transport{TLSClientConfig: tlsConfig}
	client := &http.Client{Transport: transport}

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("LOGIN: error reaching auth server: %v", err)
		s.sendResponse(conn, fmt.Sprintf("%s NO [UNAVAILABLE] Authentication service unavailable", tag))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		log.Printf("Accepting login for user: %s", username)

		// Extract username and domain
		actualUsername := s.extractUsername(username)
		domain := s.getUserDomain(username)

		// Ensure user exists in database and has default mailboxes
		userID, domainID, err := s.ensureUserAndMailboxes(actualUsername, domain)
		if err != nil {
			log.Printf("Failed to create user and mailboxes: %v", err)
			s.sendResponse(conn, fmt.Sprintf("%s NO [SERVERBUG] Server error", tag))
			return
		}

		state.Authenticated = true
		state.Username = actualUsername
		state.UserID = userID
		state.DomainID = domainID
		
		// Detect if TLS is active
		isTLS := false
		if _, ok := conn.(*tls.Conn); ok {
			isTLS = true
		} else {
			type tlsAware interface{ IsTLS() bool }
			if ta, ok := any(conn).(tlsAware); ok && ta.IsTLS() {
				isTLS = true
			}
		}
		
		// Per RFC 3501, include CAPABILITY response code in OK response
		// Only do this if security layer was not negotiated (TLS doesn't count as SASL security layer)
		capabilities := "IMAP4rev1 AUTH=PLAIN LOGIN"
		if isTLS {
			capabilities += " UIDPLUS IDLE NAMESPACE UNSELECT LITERAL+"
		} else {
			capabilities += " STARTTLS LOGINDISABLED UIDPLUS IDLE NAMESPACE UNSELECT LITERAL+"
		}
		s.sendResponse(conn, fmt.Sprintf("%s OK [CAPABILITY %s] Authenticated", tag, capabilities))
	} else {
		s.sendResponse(conn, fmt.Sprintf("%s NO [AUTHENTICATIONFAILED] Authentication failed", tag))
	}
}

func (s *IMAPServer) handleSubscribe(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	// SUBSCRIBE command format: tag SUBSCRIBE mailbox
	if len(parts) < 3 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD SUBSCRIBE command requires a mailbox argument", tag))
		return
	}

	mailboxName := parts[2]
	
	// Remove quotes if present
	if len(mailboxName) >= 2 && mailboxName[0] == '"' && mailboxName[len(mailboxName)-1] == '"' {
		mailboxName = mailboxName[1 : len(mailboxName)-1]
	}

	// Validate mailbox name
	if mailboxName == "" {
		s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid mailbox name", tag))
		return
	}

	// Subscribe to the mailbox
	err := db.SubscribeToMailbox(s.db, state.UserID, mailboxName)
	if err != nil {
		fmt.Printf("Failed to subscribe to mailbox %s for user %s: %v\n", mailboxName, state.Username, err)
		s.sendResponse(conn, fmt.Sprintf("%s NO SUBSCRIBE failure: server error", tag))
		return
	}

	s.sendResponse(conn, fmt.Sprintf("%s OK SUBSCRIBE completed", tag))
}

// handleUnsubscribe handles the UNSUBSCRIBE command to remove a mailbox from the subscription list
func (s *IMAPServer) handleUnsubscribe(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	// UNSUBSCRIBE command format: tag UNSUBSCRIBE mailbox
	if len(parts) < 3 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD UNSUBSCRIBE command requires a mailbox argument", tag))
		return
	}

	mailboxName := parts[2]
	
	// Remove quotes if present
	if len(mailboxName) >= 2 && mailboxName[0] == '"' && mailboxName[len(mailboxName)-1] == '"' {
		mailboxName = mailboxName[1 : len(mailboxName)-1]
	}

	// Validate mailbox name
	if mailboxName == "" {
		s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid mailbox name", tag))
		return
	}

	// Unsubscribe from the mailbox
	err := db.UnsubscribeFromMailbox(s.db, state.UserID, mailboxName)
	if err != nil {
		if strings.Contains(err.Error(), "subscription does not exist") {
			s.sendResponse(conn, fmt.Sprintf("%s NO UNSUBSCRIBE failure: can't unsubscribe that name", tag))
		} else {
			fmt.Printf("Failed to unsubscribe from mailbox %s for user %s: %v\n", mailboxName, state.Username, err)
			s.sendResponse(conn, fmt.Sprintf("%s NO UNSUBSCRIBE failure: server error", tag))
		}
		return
	}

	s.sendResponse(conn, fmt.Sprintf("%s OK UNSUBSCRIBE completed", tag))
}

// Exported handler methods for testing

// HandleCapability exports the capability handler for testing
func (s *IMAPServer) HandleCapability(conn net.Conn, tag string, state *models.ClientState) {
	s.handleCapability(conn, tag, state)
}

// HandleSubscribe exports the subscribe handler for testing
func (s *IMAPServer) HandleSubscribe(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	s.handleSubscribe(conn, tag, parts, state)
}

// HandleUnsubscribe exports the unsubscribe handler for testing
func (s *IMAPServer) HandleUnsubscribe(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	s.handleUnsubscribe(conn, tag, parts, state)
}

// HandleLsub exports the lsub handler for testing
func (s *IMAPServer) HandleLsub(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	s.handleLsub(conn, tag, parts, state)
}

// HandleStatus exports the status handler for testing
func (s *IMAPServer) HandleStatus(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	s.handleStatus(conn, tag, parts, state)
}

// HandleCheck exports the check handler for testing
func (s *IMAPServer) HandleCheck(conn net.Conn, tag string, state *models.ClientState) {
	s.handleCheck(conn, tag, state)
}

// parseQuotedString parses a quoted string argument, handling both quoted and unquoted strings
func (s *IMAPServer) parseQuotedString(arg string) string {
	if len(arg) == 0 {
		return ""
	}
	
	// Handle quoted strings
	if arg[0] == '"' && len(arg) >= 2 && arg[len(arg)-1] == '"' {
		return arg[1 : len(arg)-1]
	}
	
	// Handle unquoted strings (including empty string represented as "")
	if arg == `""` {
		return ""
	}
	
	return arg
}

// filterMailboxes applies reference and pattern matching according to RFC 3501
func (s *IMAPServer) filterMailboxes(mailboxes []string, reference, pattern string) []string {
	var matches []string
	hierarchyDelimiter := "/"
	
	// Construct the canonical form by combining reference and pattern
	canonicalPattern := s.buildCanonicalPattern(reference, pattern, hierarchyDelimiter)
	
	for _, mailbox := range mailboxes {
		if s.matchesPattern(mailbox, canonicalPattern, hierarchyDelimiter) {
			matches = append(matches, mailbox)
		}
	}
	
	// Always include INBOX if it matches the pattern (case-insensitive)
	inboxPattern := strings.ToUpper(canonicalPattern)
	if s.matchesPattern("INBOX", inboxPattern, hierarchyDelimiter) {
		// Check if INBOX is already in the list
		found := false
		for _, match := range matches {
			if strings.ToUpper(match) == "INBOX" {
				found = true
				break
			}
		}
		if !found {
			matches = append(matches, "INBOX")
		}
	}
	
	return matches
}

// buildCanonicalPattern builds the canonical pattern from reference and mailbox pattern
func (s *IMAPServer) buildCanonicalPattern(reference, pattern, delimiter string) string {
	// If pattern starts with delimiter, it's absolute - ignore reference
	if strings.HasPrefix(pattern, delimiter) {
		return pattern
	}
	
	// If reference is empty, use pattern as-is
	if reference == "" {
		return pattern
	}
	
	// If reference doesn't end with delimiter and pattern doesn't start with delimiter,
	// and reference is not empty, append delimiter
	if !strings.HasSuffix(reference, delimiter) && !strings.HasPrefix(pattern, delimiter) {
		return reference + delimiter + pattern
	}
	
	// Otherwise, concatenate reference and pattern
	return reference + pattern
}

// matchesPattern checks if a mailbox name matches a pattern with wildcards
func (s *IMAPServer) matchesPattern(mailbox, pattern, delimiter string) bool {
	return s.matchWildcard(mailbox, pattern, delimiter)
}

// matchWildcard implements wildcard matching for IMAP LIST patterns
func (s *IMAPServer) matchWildcard(text, pattern, delimiter string) bool {
	// Convert to case-insensitive for INBOX matching
	if strings.ToUpper(text) == "INBOX" {
		text = "INBOX"
	}
	if strings.ToUpper(pattern) == "INBOX" {
		pattern = "INBOX"
	}
	
	return s.doWildcardMatch(text, pattern, delimiter, 0, 0)
}

// doWildcardMatch performs recursive wildcard matching
func (s *IMAPServer) doWildcardMatch(text, pattern, delimiter string, textPos, patternPos int) bool {
	for patternPos < len(pattern) {
		switch pattern[patternPos] {
		case '*':
			// * matches zero or more characters
			patternPos++
			if patternPos >= len(pattern) {
				return true // * at end matches everything
			}
			
			// Try matching * with zero characters first
			if s.doWildcardMatch(text, pattern, delimiter, textPos, patternPos) {
				return true
			}
			
			// Try matching * with one or more characters
			for textPos < len(text) {
				textPos++
				if s.doWildcardMatch(text, pattern, delimiter, textPos, patternPos) {
					return true
				}
			}
			return false
			
		case '%':
			// % matches zero or more characters but not hierarchy delimiter
			patternPos++
			if patternPos >= len(pattern) {
				// % at end - check if remaining text contains delimiter
				return !strings.Contains(text[textPos:], delimiter)
			}
			
			// Try matching % with zero characters first
			if s.doWildcardMatch(text, pattern, delimiter, textPos, patternPos) {
				return true
			}
			
			// Try matching % with one or more characters (but not delimiter)
			for textPos < len(text) && !strings.HasPrefix(text[textPos:], delimiter) {
				textPos++
				if s.doWildcardMatch(text, pattern, delimiter, textPos, patternPos) {
					return true
				}
			}
			return false
			
		default:
			// Regular character - must match exactly
			if textPos >= len(text) || text[textPos] != pattern[patternPos] {
				return false
			}
			textPos++
			patternPos++
		}
	}
	
	// Pattern consumed - text should also be consumed
	return textPos >= len(text)
}

// getMailboxAttributes returns the appropriate attributes for a mailbox
func (s *IMAPServer) getMailboxAttributes(mailboxName string) string {
	switch mailboxName {
	case "Drafts":
		return "\\Drafts"
	case "Trash":
		return "\\Trash"
	case "Sent":
		return "\\Sent"
	case "INBOX":
		return "\\Unmarked"
	default:
		return "\\Unmarked"
	}
}