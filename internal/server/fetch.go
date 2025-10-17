package server

import (
	"database/sql"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"go-imap/internal/db"
	"go-imap/internal/delivery/parser"
	"go-imap/internal/models"
)

func (s *IMAPServer) handleSelect(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	if len(parts) < 3 {
		cmd := strings.ToUpper(parts[1])
		s.sendResponse(conn, fmt.Sprintf("%s BAD %s requires folder name", tag, cmd))
		return
	}

	folder := strings.Trim(parts[2], "\"")
	state.SelectedFolder = folder

	// Get mailbox ID using new schema
	mailboxID, err := db.GetMailboxByName(s.db, state.UserID, folder)
	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO [TRYCREATE] Mailbox does not exist", tag))
		return
	}

	state.SelectedMailboxID = mailboxID

	// Get mailbox info (UID validity and next UID)
	uidValidity, uidNext, err := db.GetMailboxInfo(s.db, mailboxID)
	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO Server error: cannot get mailbox info", tag))
		return
	}

	state.UIDValidity = uidValidity
	state.UIDNext = uidNext

	// Get message count using new schema
	count, err := db.GetMessageCount(s.db, mailboxID)
	if err != nil {
		count = 0
	}

	// Get unseen (recent) count using new schema
	recent, err := db.GetUnseenCount(s.db, mailboxID)
	if err != nil {
		recent = 0
	}

	// Get the first unseen message sequence number (RFC 3501 requirement)
	var unseenSeqNum int
	query := `
		SELECT seq_num FROM (
			SELECT ROW_NUMBER() OVER (ORDER BY uid ASC) as seq_num, flags
			FROM message_mailbox
			WHERE mailbox_id = ?
		) WHERE flags IS NULL OR flags NOT LIKE '%\Seen%'
		ORDER BY seq_num ASC
		LIMIT 1
	`
	err = s.db.QueryRow(query, mailboxID).Scan(&unseenSeqNum)
	hasUnseen := (err == nil && unseenSeqNum > 0)

	// Initialize state tracking for NOOP and other commands
	state.LastMessageCount = count
	state.LastRecentCount = recent

	// Determine if this is SELECT or EXAMINE
	cmd := strings.ToUpper(parts[1])
	isExamine := (cmd == "EXAMINE")

	// Send REQUIRED untagged responses in the correct order per RFC 3501
	// For SELECT: FLAGS, EXISTS, RECENT
	// For EXAMINE: EXISTS, RECENT, then FLAGS (per RFC 3501 example)
	if !isExamine {
		s.sendResponse(conn, "* FLAGS (\\Answered \\Flagged \\Deleted \\Seen \\Draft)")
	}
	s.sendResponse(conn, fmt.Sprintf("* %d EXISTS", count))
	s.sendResponse(conn, fmt.Sprintf("* %d RECENT", recent))

	// Send REQUIRED OK untagged responses
	if hasUnseen {
		s.sendResponse(conn, fmt.Sprintf("* OK [UNSEEN %d] Message %d is first unseen", unseenSeqNum, unseenSeqNum))
	}
	s.sendResponse(conn, fmt.Sprintf("* OK [UIDVALIDITY %d] UIDs valid", uidValidity))
	s.sendResponse(conn, fmt.Sprintf("* OK [UIDNEXT %d] Predicted next UID", uidNext))

	// FLAGS for EXAMINE comes after OK untagged responses
	if isExamine {
		s.sendResponse(conn, "* FLAGS (\\Answered \\Flagged \\Deleted \\Seen \\Draft)")
	}

	// PERMANENTFLAGS: Empty for EXAMINE (read-only), full for SELECT
	if isExamine {
		s.sendResponse(conn, "* OK [PERMANENTFLAGS ()] No permanent flags permitted")
	} else {
		s.sendResponse(conn, "* OK [PERMANENTFLAGS (\\Answered \\Flagged \\Deleted \\Seen \\Draft \\*)] Limited")
	}

	// Send tagged completion response
	if cmd == "SELECT" {
		s.sendResponse(conn, fmt.Sprintf("%s OK [READ-WRITE] SELECT completed", tag))
	} else {
		s.sendResponse(conn, fmt.Sprintf("%s OK [READ-ONLY] EXAMINE completed", tag))
	}
}

func (s *IMAPServer) handleFetch(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	if state.SelectedMailboxID == 0 {
		s.sendResponse(conn, fmt.Sprintf("%s NO No folder selected", tag))
		return
	}

	if len(parts) < 4 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD FETCH requires sequence and items", tag))
		return
	}

	sequence := parts[2]
	items := strings.Join(parts[3:], " ")
	items = strings.Trim(items, "()")

	var rows *sql.Rows
	var err error

	// Support for sequence ranges (e.g., 1:2, 2:4, 1:*, *)
	seqRange := strings.Split(sequence, ":")
	var start, end int
	var useRange bool

	if len(seqRange) == 2 {
		useRange = true
		if seqRange[0] == "*" {
			start = -1 // will handle below
		} else {
			start, err = strconv.Atoi(seqRange[0])
			if err != nil || start < 1 {
				s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid sequence number", tag))
				return
			}
		}
		if seqRange[1] == "*" {
			// Get max count for end using new schema
			end, _ = db.GetMessageCount(s.db, state.SelectedMailboxID)
		} else {
			end, err = strconv.Atoi(seqRange[1])
			if err != nil || end < 1 {
				s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid sequence number", tag))
				return
			}
		}
		if start == -1 {
			start = end
		}
		if end < start {
			end = start
		}
		// Query message_mailbox for messages in selected mailbox using new schema
		query := `SELECT mm.message_id, mm.uid, mm.flags
		          FROM message_mailbox mm
		          WHERE mm.mailbox_id = ?
		          ORDER BY mm.uid ASC LIMIT ? OFFSET ?`
		rows, err = s.db.Query(query, state.SelectedMailboxID, end-start+1, start-1)
	} else if sequence == "1:*" || sequence == "*" {
		query := `SELECT mm.message_id, mm.uid, mm.flags
		          FROM message_mailbox mm
		          WHERE mm.mailbox_id = ?
		          ORDER BY mm.uid ASC`
		rows, err = s.db.Query(query, state.SelectedMailboxID)
	} else {
		msgNum, parseErr := strconv.Atoi(sequence)
		if parseErr != nil {
			s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid sequence number", tag))
			return
		}
		query := `SELECT mm.message_id, mm.uid, mm.flags
		          FROM message_mailbox mm
		          WHERE mm.mailbox_id = ?
		          ORDER BY mm.uid ASC LIMIT 1 OFFSET ?`
		rows, err = s.db.Query(query, state.SelectedMailboxID, msgNum-1)
	}

	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
		return
	}
	defer rows.Close()

	seqNum := 1
	if useRange {
		seqNum = start
	}
	for rows.Next() {
		var messageID int64
		var uid int64
		var flagsStr sql.NullString
		if err := rows.Scan(&messageID, &uid, &flagsStr); err != nil {
			continue
		}

		flags := ""
		if flagsStr.Valid {
			flags = flagsStr.String
		}

		// Reconstruct message from new schema
		rawMsg, err := parser.ReconstructMessage(s.db, messageID)
		if err != nil {
			// If reconstruction fails, skip this message
			continue
		}

		if !strings.Contains(rawMsg, "\r\n") {
			rawMsg = strings.ReplaceAll(rawMsg, "\n", "\r\n")
		}

		itemsUpper := strings.ToUpper(items)
		responseParts := []string{}
		var literalData string // Store literal data separately

		if strings.Contains(itemsUpper, "UID") {
			responseParts = append(responseParts, fmt.Sprintf("UID %d", uid))
		}
		if strings.Contains(itemsUpper, "FLAGS") {
			if flags == "" {
				flags = "()"
			} else {
				flags = fmt.Sprintf("(%s)", flags)
			}
			responseParts = append(responseParts, fmt.Sprintf("FLAGS %s", flags))
		}
		if strings.Contains(itemsUpper, "INTERNALDATE") {
			var internalDate time.Time
			// Query message_mailbox for internal_date using new schema
			query := "SELECT internal_date FROM message_mailbox WHERE message_id = ? AND mailbox_id = ?"
			err := s.db.QueryRow(query, messageID, state.SelectedMailboxID).Scan(&internalDate)

			var dateStr string
			if err != nil || internalDate.IsZero() {
				dateStr = "01-Jan-1970 00:00:00 +0000"
			} else {
				// Format as RFC 3501: "02-Jan-2006 15:04:05 -0700"
				dateStr = internalDate.Format("02-Jan-2006 15:04:05 -0700")
			}
			responseParts = append(responseParts, fmt.Sprintf("INTERNALDATE \"%s\"", dateStr))
		}
		if strings.Contains(itemsUpper, "RFC822.SIZE") {
			responseParts = append(responseParts, fmt.Sprintf("RFC822.SIZE %d", len(rawMsg)))
		}
		
		// Handle multiple body parts - process each separately
		// Handle BODY.PEEK[HEADER.FIELDS (...)] or BODY[HEADER.FIELDS (...)] - specific header fields
		if strings.Contains(itemsUpper, "BODY.PEEK[HEADER.FIELDS") || strings.Contains(itemsUpper, "BODY[HEADER.FIELDS") {
			start := strings.Index(itemsUpper, "BODY.PEEK[HEADER.FIELDS")
			end := strings.Index(itemsUpper[start:], "]")
			
			// Extract requested header field names
			requestedHeaders := []string{"FROM", "TO", "CC", "BCC", "SUBJECT", "DATE", "MESSAGE-ID", "PRIORITY", "X-PRIORITY", "REFERENCES", "NEWSGROUPS", "IN-REPLY-TO", "CONTENT-TYPE", "REPLY-TO"}
			if start != -1 && end != -1 {
				fieldsStr := items[start+len("BODY.PEEK[HEADER.FIELDS ("):]
				closeParen := strings.Index(fieldsStr, ")")
				if closeParen != -1 {
					fieldsStr = fieldsStr[:closeParen]
					fields := strings.Fields(fieldsStr)
					if len(fields) > 0 {
						requestedHeaders = []string{}
						for _, f := range fields {
							requestedHeaders = append(requestedHeaders, strings.ToUpper(strings.TrimSpace(f)))
						}
					}
				}
			}
			
			// Extract only the requested headers from the message
			headersMap := map[string]string{}
			lines := strings.Split(rawMsg, "\r\n")
			currentHeader := ""
			for _, line := range lines {
				if line == "" {
					break // End of headers
				}
				// Check if this is a continuation line (starts with space or tab)
				if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
					if currentHeader != "" {
						headersMap[currentHeader] += "\r\n" + line
					}
					continue
				}
				// New header line
				colonIdx := strings.Index(line, ":")
				if colonIdx != -1 {
					headerName := strings.ToUpper(strings.TrimSpace(line[:colonIdx]))
					for _, h := range requestedHeaders {
						if headerName == h {
							currentHeader = h
							headersMap[h] = line
							break
						}
					}
				}
			}
			
			// Build response with requested headers in order
			var headerLines []string
			for _, h := range requestedHeaders {
				if val, ok := headersMap[h]; ok {
					headerLines = append(headerLines, val)
				}
			}
			headersStr := strings.Join(headerLines, "\r\n")
			if len(headersStr) > 0 {
				headersStr += "\r\n"
			}
			headersStr += "\r\n" // Final blank line
				// Match the exact format the client requested
		fieldList := strings.Join(requestedHeaders, " ")
		responseParts = append(responseParts, fmt.Sprintf("BODY[HEADER.FIELDS (%s)]", fieldList))
		literalData = fmt.Sprintf("{%d}\r\n%s", len(headersStr), headersStr)
	}
	
	// Handle BODY.PEEK[TEXT] or BODY[TEXT] - message body only (can be combined with other parts)
	if strings.Contains(itemsUpper, "BODY.PEEK[TEXT]") || strings.Contains(itemsUpper, "BODY[TEXT]") {
		headerEnd := strings.Index(rawMsg, "\r\n\r\n")
		body := ""
		if headerEnd != -1 {
			body = rawMsg[headerEnd+4:] // skip the double CRLF
		}
		
		// Check for partial fetch like BODY.PEEK[TEXT]<0.2048>
		partialStart := 0
		partialLength := len(body)
		if strings.Contains(itemsUpper, "<") && strings.Contains(itemsUpper, ">") {
			startIdx := strings.Index(itemsUpper, "<")
			endIdx := strings.Index(itemsUpper, ">")
			if startIdx != -1 && endIdx > startIdx {
				partialSpec := itemsUpper[startIdx+1:endIdx]
				fmt.Sscanf(partialSpec, "%d.%d", &partialStart, &partialLength)
				if partialStart < len(body) {
					endPos := partialStart + partialLength
					if endPos > len(body) {
						endPos = len(body)
					}
					body = body[partialStart:endPos]
				} else {
					body = ""
				}
			}
		}
		
		if literalData != "" {
			literalData += " "
		}
		responseParts = append(responseParts, "BODY[TEXT]")
		literalData += fmt.Sprintf("{%d}\r\n%s", len(body), body)
	}
	
	// Handle BODY.PEEK[HEADER] or BODY[HEADER] - all headers (check it's not HEADER.FIELDS)
	if (strings.Contains(itemsUpper, "BODY.PEEK[HEADER]") || strings.Contains(itemsUpper, "BODY[HEADER]")) && 
	   !strings.Contains(itemsUpper, "HEADER.FIELDS") {
		headerEnd := strings.Index(rawMsg, "\r\n\r\n")
		headers := rawMsg
		if headerEnd != -1 {
			headers = rawMsg[:headerEnd+2] // include last CRLF
		}
		if literalData != "" {
			literalData += " "
		}
		responseParts = append(responseParts, "BODY[HEADER]")
		literalData += fmt.Sprintf("{%d}\r\n%s", len(headers), headers)
	}
	
	// Handle RFC822.HEADER - return only the header portion
	if strings.Contains(itemsUpper, "RFC822.HEADER") {
		headerEnd := strings.Index(rawMsg, "\r\n\r\n")
		headers := rawMsg
		if headerEnd != -1 {
			headers = rawMsg[:headerEnd+2] // include last CRLF
		}
		if literalData != "" {
			literalData += " "
		}
		responseParts = append(responseParts, "RFC822.HEADER")
		literalData += fmt.Sprintf("{%d}\r\n%s", len(headers), headers)
	}
	
	// Handle RFC822.TEXT - body text only (excluding headers)
	if strings.Contains(itemsUpper, "RFC822.TEXT") {
		headerEnd := strings.Index(rawMsg, "\r\n\r\n")
		body := ""
		if headerEnd != -1 {
			body = rawMsg[headerEnd+4:] // skip the double CRLF
		}
		if literalData != "" {
			literalData += " "
		}
		responseParts = append(responseParts, "RFC822.TEXT")
		literalData += fmt.Sprintf("{%d}\r\n%s", len(body), body)
	}
	
	// Handle BODY[] / BODY.PEEK[] / RFC822 / RFC822.PEEK - full message
	if strings.Contains(itemsUpper, "BODY[]") || strings.Contains(itemsUpper, "BODY.PEEK[]") || 
	   strings.Contains(itemsUpper, "RFC822.PEEK") || 
	   (strings.Contains(itemsUpper, "RFC822") && !strings.Contains(itemsUpper, "RFC822.SIZE") && 
	    !strings.Contains(itemsUpper, "RFC822.HEADER") && !strings.Contains(itemsUpper, "RFC822.TEXT") && !strings.Contains(itemsUpper, "RFC822.PEEK")) {
		if literalData != "" {
			literalData += " "
		}
		responseParts = append(responseParts, "BODY[]")
		literalData += fmt.Sprintf("{%d}\r\n%s", len(rawMsg), rawMsg)
	}
	
		if len(responseParts) > 0 {
			responseStr := fmt.Sprintf("* %d FETCH (%s", seqNum, strings.Join(responseParts, " "))
			if literalData != "" {
				responseStr += " " + literalData + ")"
			} else {
				responseStr += ")"
			}
			s.sendResponse(conn, responseStr)
		} else {
			s.sendResponse(conn, fmt.Sprintf("* %d FETCH (FLAGS ())", seqNum))
		}
		seqNum++
	}

	s.sendResponse(conn, fmt.Sprintf("%s OK FETCH completed", tag))
}

func (s *IMAPServer) handleSearch(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	if state.SelectedMailboxID == 0 {
		s.sendResponse(conn, fmt.Sprintf("%s NO No folder selected", tag))
		return
	}

	// Query message_mailbox using new schema
	query := `SELECT ROW_NUMBER() OVER (ORDER BY uid ASC) as seq
	          FROM message_mailbox
	          WHERE mailbox_id = ?`
	rows, err := s.db.Query(query, state.SelectedMailboxID)
	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO Search failed", tag))
		return
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var seq int
		rows.Scan(&seq)
		results = append(results, strconv.Itoa(seq))
	}

	s.sendResponse(conn, fmt.Sprintf("* SEARCH %s", strings.Join(results, " ")))
	s.sendResponse(conn, fmt.Sprintf("%s OK SEARCH completed", tag))
}

func (s *IMAPServer) handleStatus(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	if len(parts) < 4 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD STATUS requires mailbox name and status data items", tag))
		return
	}

	// Parse mailbox name (could be quoted)
	mailboxName := s.parseQuotedString(parts[2])

	// Validate mailbox name
	if mailboxName == "" {
		s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid mailbox name", tag))
		return
	}

	// Get mailbox ID using new schema
	mailboxID, err := db.GetMailboxByName(s.db, state.UserID, mailboxName)
	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO STATUS failure: no status for that name", tag))
		return
	}

	// Parse status data items - they are enclosed in parentheses
	// Example: STATUS mailbox (MESSAGES RECENT)
	// Build the full items string from remaining parts
	itemsStr := strings.Join(parts[3:], " ")

	// Remove parentheses if present
	itemsStr = strings.Trim(itemsStr, "()")
	itemsStr = strings.TrimSpace(itemsStr)

	if itemsStr == "" {
		s.sendResponse(conn, fmt.Sprintf("%s BAD STATUS requires status data items", tag))
		return
	}

	// Split items by whitespace
	requestedItems := strings.Fields(strings.ToUpper(itemsStr))

	if len(requestedItems) == 0 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD STATUS requires status data items", tag))
		return
	}

	// Initialize status values
	statusValues := make(map[string]int)

	// Query total message count using new schema
	messageCount, err := db.GetMessageCount(s.db, mailboxID)
	if err != nil {
		messageCount = 0
	}
	statusValues["MESSAGES"] = messageCount

	// Query recent/unseen count using new schema
	recentCount, err := db.GetUnseenCount(s.db, mailboxID)
	if err != nil {
		recentCount = 0
	}
	statusValues["RECENT"] = recentCount
	statusValues["UNSEEN"] = recentCount

	// Get mailbox info for UID values
	uidValidity, uidNext, err := db.GetMailboxInfo(s.db, mailboxID)
	if err == nil {
		statusValues["UIDNEXT"] = int(uidNext)
		statusValues["UIDVALIDITY"] = int(uidValidity)
	} else {
		statusValues["UIDNEXT"] = 1
		statusValues["UIDVALIDITY"] = 1
	}

	// Build response with only requested items
	var responseItems []string
	for _, item := range requestedItems {
		itemUpper := strings.ToUpper(item)
		if value, ok := statusValues[itemUpper]; ok {
			responseItems = append(responseItems, fmt.Sprintf("%s %d", itemUpper, value))
		} else {
			// Unknown status item - return BAD response
			s.sendResponse(conn, fmt.Sprintf("%s BAD Unknown status data item: %s", tag, item))
			return
		}
	}

	// Send STATUS response
	s.sendResponse(conn, fmt.Sprintf("* STATUS \"%s\" (%s)", mailboxName, strings.Join(responseItems, " ")))
	s.sendResponse(conn, fmt.Sprintf("%s OK STATUS completed", tag))
}
