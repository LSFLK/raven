package uid

import (
	"database/sql"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"

	"raven/internal/db"
	"raven/internal/models"
	"raven/internal/server/message"
	"raven/internal/server/utils"
)

// ServerDeps defines the dependencies that UID handlers need from the server
type ServerDeps interface {
	SendResponse(conn net.Conn, response string)
	GetSelectedDB(state *models.ClientState) (*sql.DB, int64, error)
	GetUserDB(userID int64) (*sql.DB, error)
	GetSharedDB() *sql.DB
	GetDBManager() *db.DBManager
}

// ===== UID (Main Dispatcher) =====

// HandleUID implements the UID command (RFC 3501 Section 6.4.8)
// Syntax: UID <command> <arguments>
// Supports: UID FETCH, UID SEARCH, UID STORE, UID COPY
func HandleUID(deps ServerDeps, conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		deps.SendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	if state.SelectedMailboxID == 0 {
		deps.SendResponse(conn, fmt.Sprintf("%s NO No mailbox selected", tag))
		return
	}

	if len(parts) < 3 {
		deps.SendResponse(conn, fmt.Sprintf("%s BAD UID requires sub-command", tag))
		return
	}

	subCmd := strings.ToUpper(parts[2])
	switch subCmd {
	case "FETCH":
		handleUIDFetch(deps, conn, tag, parts, state)
	case "SEARCH":
		handleUIDSearch(deps, conn, tag, parts, state)
	case "STORE":
		handleUIDStore(deps, conn, tag, parts, state)
	case "COPY":
		handleUIDCopy(deps, conn, tag, parts, state)
	case "EXPUNGE":
		handleUIDExpunge(deps, conn, tag, parts, state)
	default:
		deps.SendResponse(conn, fmt.Sprintf("%s BAD Unknown UID command: %s", tag, subCmd))
	}
}

// ===== UID FETCH =====

// handleUIDFetch implements UID FETCH command
// Note: UID is always included in FETCH response, even if not requested
func handleUIDFetch(deps ServerDeps, conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if len(parts) < 5 {
		deps.SendResponse(conn, fmt.Sprintf("%s BAD UID FETCH requires UID sequence and items", tag))
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
	targetDB, _, err := deps.GetSelectedDB(state)
	if err != nil {
		deps.SendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
		return
	}

	// Parse UID sequence set using the correct database
	uids := utils.ParseUIDSequenceSetWithDB(uidSequence, state.SelectedMailboxID, targetDB)
	if len(uids) == 0 {
		// Non-existent UIDs are ignored without error - just return OK
		deps.SendResponse(conn, fmt.Sprintf("%s OK UID FETCH completed", tag))
		return
	}

	// Convert UIDs to a sequence set format that HandleFetchForUIDs can use
	// For each UID, we need to fetch using the same logic as handleFetch
	message.HandleFetchForUIDs(deps, conn, tag, uids, items, state)

	deps.SendResponse(conn, fmt.Sprintf("%s OK UID FETCH completed", tag))
}

// ===== UID SEARCH =====

// handleUIDSearch implements UID SEARCH command
// Returns UIDs instead of message sequence numbers
func handleUIDSearch(deps ServerDeps, conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if len(parts) < 4 {
		deps.SendResponse(conn, fmt.Sprintf("%s BAD UID SEARCH requires search criteria", tag))
		return
	}

	// Get appropriate database (user or role mailbox)
	targetDB, _, err := deps.GetSelectedDB(state)
	if err != nil {
		deps.SendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
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
		deps.SendResponse(conn, fmt.Sprintf("%s NO UID SEARCH failed: %v", tag, err))
		return
	}
	defer func() { _ = rows.Close() }()

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
	deps.SendResponse(conn, fmt.Sprintf("* SEARCH %s", strings.Join(matchingUIDs, " ")))
	deps.SendResponse(conn, fmt.Sprintf("%s OK UID SEARCH completed", tag))
}

// ===== UID STORE =====

// handleUIDStore implements UID STORE command
// Updates flags for messages by UID
func handleUIDStore(deps ServerDeps, conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if len(parts) < 6 {
		deps.SendResponse(conn, fmt.Sprintf("%s BAD UID STORE requires UID sequence, operation, and flags", tag))
		return
	}

	// Get appropriate database (user or role mailbox)
	targetDB, _, err := deps.GetSelectedDB(state)
	if err != nil {
		deps.SendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
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
		deps.SendResponse(conn, fmt.Sprintf("%s BAD Invalid data item: %s", tag, dataItem))
		return
	}

	// Parse UID sequence set using the correct database
	uids := utils.ParseUIDSequenceSetWithDB(uidSequence, state.SelectedMailboxID, targetDB)
	if len(uids) == 0 {
		// Non-existent UIDs are ignored without error
		deps.SendResponse(conn, fmt.Sprintf("%s OK UID STORE completed", tag))
		return
	}

	// Process each UID
	for _, uid := range uids {
		// Get current flags, sequence number, message ID, and internal date
		var currentFlags string
		var seqNum int
		var messageID int64
		var internalDate string

		err := targetDB.QueryRow(`
			SELECT mm.message_id, mm.flags, mm.internal_date,
				(SELECT COUNT(*) FROM message_mailbox mm2
				 WHERE mm2.mailbox_id = mm.mailbox_id AND mm2.uid <= mm.uid) as seq_num
			FROM message_mailbox mm
			WHERE mm.mailbox_id = ? AND mm.uid = ?
		`, state.SelectedMailboxID, uid).Scan(&messageID, &currentFlags, &internalDate, &seqNum)

		if err != nil {
			// Non-existent UID is silently ignored
			continue
		}

		// Calculate new flags based on operation
		updatedFlags := message.CalculateNewFlags(currentFlags, newFlags, dataItem)

		// Check if Junk or NonJunk flags were added
		currentFlagsSet := parseFlagsToSet(currentFlags)
		updatedFlagsSet := parseFlagsToSet(updatedFlags)

		junkAdded := !currentFlagsSet["Junk"] && updatedFlagsSet["Junk"]
		nonJunkAdded := !currentFlagsSet["NonJunk"] && updatedFlagsSet["NonJunk"]

		// Auto-move messages based on Junk/NonJunk flags
		// These flags are mutually exclusive - ensure only one is set
		if junkAdded {
			// Remove NonJunk flag if present (mutually exclusive with Junk)
			cleanedFlags := removeFlagFromSet(updatedFlagsSet, "NonJunk")
			cleanedFlagsStr := flagSetToString(cleanedFlags)

			// Move to Spam folder
			err = message.MoveMessageToMailbox(targetDB, messageID, state.SelectedMailboxID, "Spam", state.UserID, cleanedFlagsStr, internalDate)
			if err != nil {
				log.Printf("Failed to move message %d to Spam: %v", messageID, err)
			} else {
				log.Printf("Auto-moved message %d to Spam folder (Junk flag added)", messageID)
				// Send EXPUNGE notification to tell client the message is gone from this mailbox
				if !silent {
					deps.SendResponse(conn, fmt.Sprintf("* %d EXPUNGE", seqNum))
				}
				// Message was moved - don't send FETCH response since it's no longer in this mailbox
				continue
			}
		} else if nonJunkAdded {
			// Remove Junk flag if present (mutually exclusive with NonJunk)
			cleanedFlags := removeFlagFromSet(updatedFlagsSet, "Junk")
			cleanedFlagsStr := flagSetToString(cleanedFlags)

			// Move to INBOX
			err = message.MoveMessageToMailbox(targetDB, messageID, state.SelectedMailboxID, "INBOX", state.UserID, cleanedFlagsStr, internalDate)
			if err != nil {
				log.Printf("Failed to move message %d to INBOX: %v", messageID, err)
			} else {
				log.Printf("Auto-moved message %d to INBOX (NonJunk flag added)", messageID)
				// Send EXPUNGE notification to tell client the message is gone from this mailbox
				if !silent {
					deps.SendResponse(conn, fmt.Sprintf("* %d EXPUNGE", seqNum))
				}
				// Message was moved - don't send FETCH response since it's no longer in this mailbox
				continue
			}
		}

		// Update flags in database (only if message wasn't moved)
		_, err = targetDB.Exec(`
			UPDATE message_mailbox
			SET flags = ?
			WHERE mailbox_id = ? AND uid = ?
		`, updatedFlags, state.SelectedMailboxID, uid)

		if err != nil {
			deps.SendResponse(conn, fmt.Sprintf("%s NO UID STORE failed: %v", tag, err))
			return
		}

		// Send untagged FETCH response unless .SILENT
		if !silent {
			flagsResponse := "()"
			if updatedFlags != "" {
				flagsResponse = fmt.Sprintf("(%s)", updatedFlags)
			}
			deps.SendResponse(conn, fmt.Sprintf("* %d FETCH (FLAGS %s UID %d)", seqNum, flagsResponse, uid))
		}
	}

	deps.SendResponse(conn, fmt.Sprintf("%s OK UID STORE completed", tag))
}

// ===== UID COPY =====

// handleUIDCopy implements UID COPY command
// Copies messages by UID to destination mailbox
func handleUIDCopy(deps ServerDeps, conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if len(parts) < 5 {
		deps.SendResponse(conn, fmt.Sprintf("%s BAD UID COPY requires UID sequence and destination mailbox", tag))
		return
	}

	// Get appropriate database (user or role mailbox)
	targetDB, targetUserID, err := deps.GetSelectedDB(state)
	if err != nil {
		deps.SendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
		return
	}

	uidSequence := parts[3]
	destMailbox := strings.Trim(strings.Join(parts[4:], " "), "\"")

	// Parse UID sequence set using the correct database
	uids := utils.ParseUIDSequenceSetWithDB(uidSequence, state.SelectedMailboxID, targetDB)
	if len(uids) == 0 {
		// Non-existent UIDs are ignored without error
		deps.SendResponse(conn, fmt.Sprintf("%s OK UID COPY completed", tag))
		return
	}

	// Check if destination mailbox exists
	var destMailboxID int64
	err = targetDB.QueryRow(`
		SELECT id FROM mailboxes
		WHERE name = ? AND user_id = ?
	`, destMailbox, targetUserID).Scan(&destMailboxID)

	if err != nil {
		deps.SendResponse(conn, fmt.Sprintf("%s NO [TRYCREATE] Destination mailbox does not exist", tag))
		return
	}

	// Begin transaction
	tx, err := targetDB.Begin()
	if err != nil {
		deps.SendResponse(conn, fmt.Sprintf("%s NO UID COPY failed: %v", tag, err))
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Get next UID for destination mailbox
	var nextUID int64
	err = tx.QueryRow(`
		SELECT COALESCE(MAX(uid), 0) + 1
		FROM message_mailbox
		WHERE mailbox_id = ?
	`, destMailboxID).Scan(&nextUID)

	if err != nil {
		deps.SendResponse(conn, fmt.Sprintf("%s NO UID COPY failed: %v", tag, err))
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
			deps.SendResponse(conn, fmt.Sprintf("%s NO UID COPY failed: %v", tag, err))
			return
		}

		nextUID++
	}

	// Commit transaction
	err = tx.Commit()
	if err != nil {
		deps.SendResponse(conn, fmt.Sprintf("%s NO UID COPY failed: %v", tag, err))
		return
	}

	deps.SendResponse(conn, fmt.Sprintf("%s OK UID COPY completed", tag))
}

// parseFlagsToSet converts a space-separated flags string into a set (map)
func parseFlagsToSet(flags string) map[string]bool {
	flagSet := make(map[string]bool)
	if flags != "" {
		for _, flag := range strings.Fields(flags) {
			flagSet[flag] = true
		}
	}
	return flagSet
}

// removeFlagFromSet removes a specific flag from the flag set
func removeFlagFromSet(flagSet map[string]bool, flagToRemove string) map[string]bool {
	newSet := make(map[string]bool)
	for flag, val := range flagSet {
		if flag != flagToRemove {
			newSet[flag] = val
		}
	}
	return newSet
}

// flagSetToString converts a flag set back to a space-separated string
func flagSetToString(flagSet map[string]bool) string {
	var flags []string
	for flag := range flagSet {
		flags = append(flags, flag)
	}
	return strings.Join(flags, " ")
}

// ===== UID EXPUNGE =====

// handleUIDExpunge implements UID EXPUNGE command (RFC 4315 - UIDPLUS extension)
// Syntax: UID EXPUNGE <uid-set>
// Permanently removes messages in the UID set that have the \Deleted flag
func handleUIDExpunge(deps ServerDeps, conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if len(parts) < 4 {
		deps.SendResponse(conn, fmt.Sprintf("%s BAD UID EXPUNGE requires UID sequence", tag))
		return
	}

	// Get appropriate database (user or role mailbox)
	targetDB, _, err := deps.GetSelectedDB(state)
	if err != nil {
		deps.SendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
		return
	}

	// Parse UID sequence set
	uidSequence := parts[3]
	uids := utils.ParseUIDSequenceSetWithDB(uidSequence, state.SelectedMailboxID, targetDB)
	if len(uids) == 0 {
		// Non-existent UIDs are ignored without error - just return OK
		deps.SendResponse(conn, fmt.Sprintf("%s OK UID EXPUNGE completed", tag))
		return
	}

	// Query for messages that are both in the UID set AND have \Deleted flag
	// Build a parameterized query with placeholders for the IN clause
	placeholders := make([]string, len(uids))
	args := make([]any, 0, len(uids)+1)

	args = append(args, state.SelectedMailboxID)

	for i, uid := range uids {
		placeholders[i] = "?"
		args = append(args, uid)
	}

	// #nosec G202 -- placeholder list is generated internally and contains only "?" placeholders
	query := `
		SELECT id, uid FROM message_mailbox
		WHERE mailbox_id = ? AND uid IN (` + strings.Join(placeholders, ",") + `)
		AND flags LIKE '%\Deleted%'
		ORDER BY uid ASC
	`

	rows, err := targetDB.Query(query, args...)
	if err != nil {
		deps.SendResponse(conn, fmt.Sprintf("%s NO UID EXPUNGE failed: %v", tag, err))
		return
	}
	defer func() { _ = rows.Close() }()

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
	_ = rows.Close()

	// If no messages to delete, just return OK
	if len(messagesToDelete) == 0 {
		deps.SendResponse(conn, fmt.Sprintf("%s OK UID EXPUNGE completed", tag))
		return
	}

	// Get all messages in the mailbox to calculate sequence numbers
	allRows, err := targetDB.Query(`
		SELECT id, uid FROM message_mailbox
		WHERE mailbox_id = ?
		ORDER BY uid ASC
	`, state.SelectedMailboxID)

	if err != nil {
		deps.SendResponse(conn, fmt.Sprintf("%s NO UID EXPUNGE failed: %v", tag, err))
		return
	}
	defer func() { _ = allRows.Close() }()

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
	_ = allRows.Close()

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
		deps.SendResponse(conn, fmt.Sprintf("* %d EXPUNGE", adjustedSeqNum))

		// Delete the message from the mailbox
		_, err = targetDB.Exec(`DELETE FROM message_mailbox WHERE id = ?`, msg.id)
		if err != nil {
			log.Printf("Failed to delete message %d (UID %d): %v", msg.id, msg.uid, err)
		}

		deletedCount++
	}

	// Update state tracking
	state.LastMessageCount -= len(messagesToDelete)
	if state.LastMessageCount < 0 {
		state.LastMessageCount = 0
	}

	// Send completion response
	deps.SendResponse(conn, fmt.Sprintf("%s OK UID EXPUNGE completed", tag))
}
