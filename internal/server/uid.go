package server

import (
	"database/sql"
	"fmt"
	"net"
	"strconv"
	"strings"

	"raven/internal/models"
)

// handleUID implements the UID command (RFC 3501 Section 6.4.8)
// Syntax: UID <command> <arguments>
// Supports: UID FETCH, UID SEARCH, UID STORE, UID COPY
func (s *IMAPServer) handleUID(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	if state.SelectedMailboxID == 0 {
		s.sendResponse(conn, fmt.Sprintf("%s NO No mailbox selected", tag))
		return
	}

	if len(parts) < 3 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD UID requires sub-command", tag))
		return
	}

	subCmd := strings.ToUpper(parts[2])
	switch subCmd {
	case "FETCH":
		s.handleUIDFetch(conn, tag, parts, state)
	case "SEARCH":
		s.handleUIDSearch(conn, tag, parts, state)
	case "STORE":
		s.handleUIDStore(conn, tag, parts, state)
	case "COPY":
		s.handleUIDCopy(conn, tag, parts, state)
	default:
		s.sendResponse(conn, fmt.Sprintf("%s BAD Unknown UID command: %s", tag, subCmd))
	}
}

// handleUIDFetch implements UID FETCH command
// Note: UID is always included in FETCH response, even if not requested
func (s *IMAPServer) handleUIDFetch(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if len(parts) < 5 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD UID FETCH requires UID sequence and items", tag))
		return
	}

	uidSequence := parts[3]
	items := strings.Join(parts[4:], " ")

	// Ensure UID is always in the items list
	itemsUpper := strings.ToUpper(items)
	if !strings.Contains(itemsUpper, "UID") {
		items = "UID " + items
	}

	// Get appropriate database (user or role mailbox)
	targetDB, _, err := s.getSelectedDB(state)
	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
		return
	}

	// Parse UID sequence set using the correct database
	uids := s.parseUIDSequenceSetWithDB(uidSequence, state.SelectedMailboxID, targetDB)
	if len(uids) == 0 {
		// Non-existent UIDs are ignored without error - just return OK
		s.sendResponse(conn, fmt.Sprintf("%s OK UID FETCH completed", tag))
		return
	}

	// Convert UIDs to a sequence set format that handleFetchForUIDs can use
	// For each UID, we need to fetch using the same logic as handleFetch
	s.handleFetchForUIDs(conn, tag, uids, items, state)

	s.sendResponse(conn, fmt.Sprintf("%s OK UID FETCH completed", tag))
}

// handleUIDSearch implements UID SEARCH command
// Returns UIDs instead of message sequence numbers
func (s *IMAPServer) handleUIDSearch(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if len(parts) < 4 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD UID SEARCH requires search criteria", tag))
		return
	}

	// Get appropriate database (user or role mailbox)
	targetDB, _, err := s.getSelectedDB(state)
	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
		return
	}

	// Get search criteria (everything after "UID SEARCH")
	searchCriteria := strings.Join(parts[3:], " ")

	// Query all messages in mailbox with UIDs
	rows, err := targetDB.Query(`
		SELECT mm.message_id, mm.uid, mm.flags, mm.internal_date,
			(SELECT COUNT(*) FROM message_mailbox mm2
			 WHERE mm2.mailbox_id = mm.mailbox_id AND mm2.uid <= mm.uid) as seq_num
		FROM message_mailbox mm
		WHERE mm.mailbox_id = ?
		ORDER BY mm.uid
	`, state.SelectedMailboxID)

	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO UID SEARCH failed: %v", tag, err))
		return
	}
	defer rows.Close()

	// Build message info structures
	type uidMessageInfo struct {
		messageID    int64
		uid          int
		seqNum       int
		flags        string
		internalDate string
	}

	var messages []uidMessageInfo
	for rows.Next() {
		var msg uidMessageInfo
		err := rows.Scan(&msg.messageID, &msg.uid, &msg.flags, &msg.internalDate, &msg.seqNum)
		if err != nil {
			continue
		}
		messages = append(messages, msg)
	}

	// Evaluate search criteria - returns matching UIDs
	var matchingUIDs []string
	criteriaUpper := strings.ToUpper(searchCriteria)

	if criteriaUpper == "ALL" {
		for _, msg := range messages {
			matchingUIDs = append(matchingUIDs, strconv.Itoa(msg.uid))
		}
	} else if strings.Contains(criteriaUpper, "UID ") {
		// Extract UID range
		parts := strings.Fields(searchCriteria)
		for i, part := range parts {
			if strings.ToUpper(part) == "UID" && i+1 < len(parts) {
				uidRange := parts[i+1]
				if strings.Contains(uidRange, ":") {
					rangeParts := strings.Split(uidRange, ":")
					if len(rangeParts) == 2 {
						start, _ := strconv.Atoi(rangeParts[0])
						end, _ := strconv.Atoi(rangeParts[1])
						for _, msg := range messages {
							if msg.uid >= start && msg.uid <= end {
								matchingUIDs = append(matchingUIDs, strconv.Itoa(msg.uid))
							}
						}
					}
				}
				break
			}
		}
	} else {
		// Default: return all UIDs
		for _, msg := range messages {
			matchingUIDs = append(matchingUIDs, strconv.Itoa(msg.uid))
		}
	}

	// Return matching UIDs
	s.sendResponse(conn, fmt.Sprintf("* SEARCH %s", strings.Join(matchingUIDs, " ")))
	s.sendResponse(conn, fmt.Sprintf("%s OK UID SEARCH completed", tag))
}

// handleUIDStore implements UID STORE command
// Updates flags for messages by UID
func (s *IMAPServer) handleUIDStore(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if len(parts) < 6 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD UID STORE requires UID sequence, operation, and flags", tag))
		return
	}

	// Get appropriate database (user or role mailbox)
	targetDB, _, err := s.getSelectedDB(state)
	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
		return
	}

	uidSequence := parts[3]
	dataItem := strings.ToUpper(parts[4])
	flagsParts := parts[5:]

	// Check for .SILENT suffix
	silent := strings.HasSuffix(dataItem, ".SILENT")
	if silent {
		dataItem = strings.TrimSuffix(dataItem, ".SILENT")
	}

	// Parse flags
	flagsStr := strings.Join(flagsParts, " ")
	flagsStr = strings.Trim(flagsStr, "()")
	newFlags := strings.Fields(flagsStr)

	// Validate data item
	if dataItem != "FLAGS" && dataItem != "+FLAGS" && dataItem != "-FLAGS" {
		s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid data item: %s", tag, dataItem))
		return
	}

	// Parse UID sequence set using the correct database
	uids := s.parseUIDSequenceSetWithDB(uidSequence, state.SelectedMailboxID, targetDB)
	if len(uids) == 0 {
		// Non-existent UIDs are ignored without error
		s.sendResponse(conn, fmt.Sprintf("%s OK UID STORE completed", tag))
		return
	}

	// Process each UID
	for _, uid := range uids {
		// Get current flags and sequence number
		var currentFlags string
		var seqNum int

		err := targetDB.QueryRow(`
			SELECT mm.flags,
				(SELECT COUNT(*) FROM message_mailbox mm2
				 WHERE mm2.mailbox_id = mm.mailbox_id AND mm2.uid <= mm.uid) as seq_num
			FROM message_mailbox mm
			WHERE mm.mailbox_id = ? AND mm.uid = ?
		`, state.SelectedMailboxID, uid).Scan(&currentFlags, &seqNum)

		if err != nil {
			// Non-existent UID is silently ignored
			continue
		}

		// Calculate new flags based on operation
		updatedFlags := s.calculateNewFlags(currentFlags, newFlags, dataItem)

		// Update flags in database
		_, err = targetDB.Exec(`
			UPDATE message_mailbox
			SET flags = ?
			WHERE mailbox_id = ? AND uid = ?
		`, updatedFlags, state.SelectedMailboxID, uid)

		if err != nil {
			s.sendResponse(conn, fmt.Sprintf("%s NO UID STORE failed: %v", tag, err))
			return
		}

		// Send untagged FETCH response unless .SILENT
		if !silent {
			flagsResponse := "()"
			if updatedFlags != "" {
				flagsResponse = fmt.Sprintf("(%s)", updatedFlags)
			}
			s.sendResponse(conn, fmt.Sprintf("* %d FETCH (FLAGS %s UID %d)", seqNum, flagsResponse, uid))
		}
	}

	s.sendResponse(conn, fmt.Sprintf("%s OK UID STORE completed", tag))
}

// handleUIDCopy implements UID COPY command
// Copies messages by UID to destination mailbox
func (s *IMAPServer) handleUIDCopy(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if len(parts) < 5 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD UID COPY requires UID sequence and destination mailbox", tag))
		return
	}

	// Get appropriate database (user or role mailbox)
	targetDB, targetUserID, err := s.getSelectedDB(state)
	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
		return
	}

	uidSequence := parts[3]
	destMailbox := strings.Trim(strings.Join(parts[4:], " "), "\"")

	// Parse UID sequence set using the correct database
	uids := s.parseUIDSequenceSetWithDB(uidSequence, state.SelectedMailboxID, targetDB)
	if len(uids) == 0 {
		// Non-existent UIDs are ignored without error
		s.sendResponse(conn, fmt.Sprintf("%s OK UID COPY completed", tag))
		return
	}

	// Check if destination mailbox exists
	var destMailboxID int64
	err = targetDB.QueryRow(`
		SELECT id FROM mailboxes
		WHERE name = ? AND user_id = ?
	`, destMailbox, targetUserID).Scan(&destMailboxID)

	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO [TRYCREATE] Destination mailbox does not exist", tag))
		return
	}

	// Begin transaction
	tx, err := targetDB.Begin()
	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO UID COPY failed: %v", tag, err))
		return
	}
	defer tx.Rollback()

	// Get next UID for destination mailbox
	var nextUID int64
	err = tx.QueryRow(`
		SELECT COALESCE(MAX(uid), 0) + 1
		FROM message_mailbox
		WHERE mailbox_id = ?
	`, destMailboxID).Scan(&nextUID)

	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO UID COPY failed: %v", tag, err))
		return
	}

	// Copy each message by UID
	for _, uid := range uids {
		var messageID int64
		var flags, internalDate string

		err = tx.QueryRow(`
			SELECT message_id, flags, internal_date
			FROM message_mailbox
			WHERE mailbox_id = ? AND uid = ?
		`, state.SelectedMailboxID, uid).Scan(&messageID, &flags, &internalDate)

		if err != nil {
			// Non-existent UID is silently ignored
			continue
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
			s.sendResponse(conn, fmt.Sprintf("%s NO UID COPY failed: %v", tag, err))
			return
		}

		nextUID++
	}

	// Commit transaction
	err = tx.Commit()
	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO UID COPY failed: %v", tag, err))
		return
	}

	s.sendResponse(conn, fmt.Sprintf("%s OK UID COPY completed", tag))
}

// parseUIDSequenceSet parses a UID sequence set and returns list of UIDs
// Handles: single (443), ranges (100:200), star (*), ranges with star (559:*)
func (s *IMAPServer) parseUIDSequenceSet(sequenceSet string, mailboxID int64, userID int64) []int {
	var uids []int

	// Get user database
	userDB, err := s.getUserDB(userID)
	if err != nil {
		return uids
	}

	return s.parseUIDSequenceSetWithDB(sequenceSet, mailboxID, userDB)
}

// parseUIDSequenceSetWithDB parses a UID sequence set using a provided database connection
// Handles: single (443), ranges (100:200), star (*), ranges with star (559:*)
func (s *IMAPServer) parseUIDSequenceSetWithDB(sequenceSet string, mailboxID int64, db *sql.DB) []int {
	var uids []int

	// Get highest UID in mailbox for * handling
	var maxUID int
	err := db.QueryRow(`
		SELECT COALESCE(MAX(uid), 0)
		FROM message_mailbox
		WHERE mailbox_id = ?
	`, mailboxID).Scan(&maxUID)

	if err != nil || maxUID == 0 {
		return uids
	}

	// Split by comma for multiple sequences
	parts := strings.Split(sequenceSet, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)

		if part == "*" {
			// * means highest UID
			uids = append(uids, maxUID)
		} else if strings.Contains(part, ":") {
			// Range
			rangeParts := strings.Split(part, ":")
			if len(rangeParts) != 2 {
				continue
			}

			start := 0
			end := 0

			if rangeParts[0] == "*" {
				start = maxUID
			} else {
				start, _ = strconv.Atoi(rangeParts[0])
			}

			if rangeParts[1] == "*" {
				end = maxUID
			} else {
				end, _ = strconv.Atoi(rangeParts[1])
			}

			// Ensure start <= end (RFC 3501: contents of range independent of order)
			if start > end {
				start, end = end, start
			}

			// Get all UIDs in range
			rows, err := db.Query(`
				SELECT uid
				FROM message_mailbox
				WHERE mailbox_id = ? AND uid >= ? AND uid <= ?
				ORDER BY uid
			`, mailboxID, start, end)

			if err != nil {
				continue
			}

			for rows.Next() {
				var uid int
				rows.Scan(&uid)
				uids = append(uids, uid)
			}
			rows.Close()
		} else {
			// Single UID
			uid, err := strconv.Atoi(part)
			if err != nil {
				continue
			}

			// Check if UID exists
			var count int
			db.QueryRow(`
				SELECT COUNT(*)
				FROM message_mailbox
				WHERE mailbox_id = ? AND uid = ?
			`, mailboxID, uid).Scan(&count)

			if count > 0 {
				uids = append(uids, uid)
			}
		}
	}

	return uids
}
