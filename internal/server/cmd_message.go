package server

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net"
	"strconv"
	"strings"
	"time"

	"raven/internal/db"
	"raven/internal/delivery/parser"
	"raven/internal/models"
	"raven/internal/server/utils"
)

// ===== FETCH =====

// handleFetchForUIDs handles FETCH for a list of UIDs (used by UID FETCH command)
func (s *IMAPServer) handleFetchForUIDs(conn net.Conn, tag string, uids []int, items string, state *models.ClientState) {
	// Get appropriate database (user or role mailbox)
	targetDB, _, err := s.GetSelectedDB(state)
	if err != nil {
		return
	}

	for _, uid := range uids {
		// Get message details by UID
		var messageID int64
		var seqNum int
		var flags sql.NullString

		err := targetDB.QueryRow(`
			SELECT mm.message_id, mm.flags,
				(SELECT COUNT(*) FROM message_mailbox mm2
				 WHERE mm2.mailbox_id = mm.mailbox_id AND mm2.uid <= mm.uid) as seq_num
			FROM message_mailbox mm
			WHERE mm.mailbox_id = ? AND mm.uid = ?
		`, state.SelectedMailboxID, uid).Scan(&messageID, &flags, &seqNum)

		if err != nil {
			// Non-existent UID is silently ignored
			continue
		}

		// Process this message using the same logic as handleFetch
		s.processFetchForMessage(conn, messageID, int64(uid), seqNum, flags.String, items, state)
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

	// Get appropriate database (user or role mailbox)
	targetDB, _, err := s.GetSelectedDB(state)
	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
		return
	}

	sequence := parts[2]
	items := strings.Join(parts[3:], " ")

	// Handle FETCH macros: ALL, FAST, FULL
	itemsUpper := strings.ToUpper(strings.TrimSpace(items))
	switch itemsUpper {
	case "ALL":
		items = "FLAGS INTERNALDATE RFC822.SIZE ENVELOPE"
	case "FAST":
		items = "FLAGS INTERNALDATE RFC822.SIZE"
	case "FULL":
		items = "FLAGS INTERNALDATE RFC822.SIZE ENVELOPE BODY"
	default:
		// Remove parentheses if present
		items = strings.Trim(items, "()")
	}

	var rows *sql.Rows

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
			end, _ = db.GetMessageCountPerUser(targetDB, state.SelectedMailboxID)
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
		rows, err = targetDB.Query(query, state.SelectedMailboxID, end-start+1, start-1)
	} else if sequence == "1:*" || sequence == "*" {
		query := `SELECT mm.message_id, mm.uid, mm.flags
		          FROM message_mailbox mm
		          WHERE mm.mailbox_id = ?
		          ORDER BY mm.uid ASC`
		rows, err = targetDB.Query(query, state.SelectedMailboxID)
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
		rows, err = targetDB.Query(query, state.SelectedMailboxID, msgNum-1)
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

		// Process this message
		s.processFetchForMessage(conn, messageID, uid, seqNum, flags, items, state)
		seqNum++
	}

	s.sendResponse(conn, fmt.Sprintf("%s OK FETCH completed", tag))
}

// processFetchForMessage processes a single message for FETCH/UID FETCH
func (s *IMAPServer) processFetchForMessage(conn net.Conn, messageID, uid int64, seqNum int, flags, items string, state *models.ClientState) {
	// Get appropriate database (user or role mailbox)
	targetDB, _, err := s.GetSelectedDB(state)
	if err != nil {
		return
	}

	// Reconstruct message from new schema
	rawMsg, err := parser.ReconstructMessage(targetDB, messageID)
	if err != nil {
		// If reconstruction fails, skip this message
		return
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
			err := targetDB.QueryRow(query, messageID, state.SelectedMailboxID).Scan(&internalDate)

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

		// Handle ENVELOPE
		if strings.Contains(itemsUpper, "ENVELOPE") {
			envelope := s.buildEnvelope(rawMsg)
			responseParts = append(responseParts, envelope)
		}

		// Handle BODYSTRUCTURE
		if strings.Contains(itemsUpper, "BODYSTRUCTURE") {
			bodyStructure := s.buildBodyStructure(rawMsg)
			responseParts = append(responseParts, bodyStructure)
		}

		// Handle BODY (non-extensible BODYSTRUCTURE)
		if strings.Contains(itemsUpper, "BODY") && !strings.Contains(itemsUpper, "BODY[") && !strings.Contains(itemsUpper, "BODY.PEEK") && !strings.Contains(itemsUpper, "BODYSTRUCTURE") {
			// BODY is the non-extensible form of BODYSTRUCTURE
			bodyStructure := s.buildBodyStructure(rawMsg)
			// Replace BODYSTRUCTURE with BODY in the response
			bodyStructure = strings.Replace(bodyStructure, "BODYSTRUCTURE", "BODY", 1)
			responseParts = append(responseParts, bodyStructure)
		}

		// Handle multiple body parts - process each separately
		// Handle BODY.PEEK[HEADER.FIELDS (...)] or BODY[HEADER.FIELDS (...)] - specific header fields
		if strings.Contains(itemsUpper, "BODY.PEEK[HEADER.FIELDS") || strings.Contains(itemsUpper, "BODY[HEADER.FIELDS") {
			start := strings.Index(itemsUpper, "BODY.PEEK[HEADER.FIELDS")
			if start == -1 {
				start = strings.Index(itemsUpper, "BODY[HEADER.FIELDS")
			}

			// Extract requested header field names
			requestedHeaders := []string{"FROM", "TO", "CC", "BCC", "SUBJECT", "DATE", "MESSAGE-ID", "PRIORITY", "X-PRIORITY", "REFERENCES", "NEWSGROUPS", "IN-REPLY-TO", "CONTENT-TYPE", "REPLY-TO"}
			if start != -1 {
				isPeek := strings.Contains(itemsUpper, "BODY.PEEK[HEADER.FIELDS")
				prefixLen := len("BODY[HEADER.FIELDS (")
				if isPeek {
					prefixLen = len("BODY.PEEK[HEADER.FIELDS (")
				}

				fieldsStr := items[start+prefixLen:]
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
}

// ===== SEARCH =====

// messageInfo holds metadata about a message for search operations
type messageInfo struct {
	messageID    int64
	uid          int64
	flags        string
	internalDate time.Time
	seqNum       int
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

	// Get appropriate database (user or role mailbox)
	targetDB, targetUserID, err := s.GetSelectedDB(state)
	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
		return
	}

	// Parse search criteria
	if len(parts) < 3 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD SEARCH requires search criteria", tag))
		return
	}

	// Check for CHARSET specification
	searchStart := 2
	charset := "US-ASCII"
	if len(parts) > 3 && strings.ToUpper(parts[2]) == "CHARSET" {
		charset = strings.ToUpper(parts[3])
		searchStart = 4

		// RFC 3501: US-ASCII MUST be supported, other charsets MAY be supported
		if charset != "US-ASCII" && charset != "UTF-8" {
			// Return tagged NO with BADCHARSET response code
			s.sendResponse(conn, fmt.Sprintf("%s NO [BADCHARSET (US-ASCII UTF-8)] Charset not supported", tag))
			return
		}
	}

	if searchStart >= len(parts) {
		s.sendResponse(conn, fmt.Sprintf("%s BAD SEARCH requires search criteria", tag))
		return
	}

	// Get all messages in the mailbox with their metadata
	query := `
		SELECT mm.message_id, mm.uid, mm.flags, mm.internal_date,
		       ROW_NUMBER() OVER (ORDER BY mm.uid ASC) as seq_num
		FROM message_mailbox mm
		WHERE mm.mailbox_id = ?
		ORDER BY mm.uid ASC
	`
	rows, err := targetDB.Query(query, state.SelectedMailboxID)
	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO Search failed: %v", tag, err))
		return
	}
	defer rows.Close()

	// Build list of messages with metadata
	var messages []messageInfo

	for rows.Next() {
		var msg messageInfo
		var flagsStr sql.NullString
		var internalDate sql.NullTime
		if err := rows.Scan(&msg.messageID, &msg.uid, &flagsStr, &internalDate, &msg.seqNum); err != nil {
			continue
		}
		if flagsStr.Valid {
			msg.flags = flagsStr.String
		}
		if internalDate.Valid {
			msg.internalDate = internalDate.Time
		}
		messages = append(messages, msg)
	}

	// Parse and evaluate search criteria
	criteria := strings.Join(parts[searchStart:], " ")
	matchingSeqNums := s.evaluateSearchCriteria(messages, criteria, charset, targetUserID)

	// Build response
	if len(matchingSeqNums) > 0 {
		var results []string
		for _, seq := range matchingSeqNums {
			results = append(results, strconv.Itoa(seq))
		}
		s.sendResponse(conn, fmt.Sprintf("* SEARCH %s", strings.Join(results, " ")))
	} else {
		s.sendResponse(conn, "* SEARCH")
	}
	s.sendResponse(conn, fmt.Sprintf("%s OK SEARCH completed", tag))
}

// evaluateSearchCriteria evaluates search criteria against messages
func (s *IMAPServer) evaluateSearchCriteria(messages []messageInfo, criteria string, charset string, userID int64) []int {
	var matchingSeqNums []int

	// Default to ALL if no criteria specified
	if strings.TrimSpace(criteria) == "" {
		criteria = "ALL"
	}

	// Parse criteria into tokens
	tokens := s.parseSearchTokens(criteria)

	// Evaluate each message
	for _, msg := range messages {
		if s.matchesSearchCriteria(msg, tokens, charset, userID) {
			matchingSeqNums = append(matchingSeqNums, msg.seqNum)
		}
	}

	return matchingSeqNums
}

// parseSearchTokens tokenizes search criteria
func (s *IMAPServer) parseSearchTokens(criteria string) []string {
	var tokens []string
	var current strings.Builder
	inQuotes := false
	inParens := 0

	for i := 0; i < len(criteria); i++ {
		ch := criteria[i]

		switch ch {
		case '"':
			inQuotes = !inQuotes
			current.WriteByte(ch)
		case '(':
			if !inQuotes {
				inParens++
			}
			current.WriteByte(ch)
		case ')':
			if !inQuotes {
				inParens--
			}
			current.WriteByte(ch)
		case ' ', '\t':
			if inQuotes || inParens > 0 {
				current.WriteByte(ch)
			} else if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(ch)
		}
	}

	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}

// matchesSearchCriteria checks if a message matches the search criteria
func (s *IMAPServer) matchesSearchCriteria(msg messageInfo, tokens []string, charset string, userID int64) bool {
	// Default to ALL - match everything
	if len(tokens) == 0 {
		return true
	}

	// Process tokens (AND logic by default)
	return s.evaluateTokens(msg, tokens, charset, userID)
}

// evaluateTokens evaluates a list of search tokens
func (s *IMAPServer) evaluateTokens(msg messageInfo, tokens []string, charset string, userID int64) bool {
	i := 0
	for i < len(tokens) {
		token := strings.ToUpper(tokens[i])

		// Handle sequence set (numbers and ranges)
		if s.isSequenceSet(token) {
			if !s.matchesSequenceSet(msg.seqNum, token) {
				return false
			}
			i++
			continue
		}

		switch token {
		case "ALL":
			// Matches all messages
			i++

		case "ANSWERED":
			if !strings.Contains(msg.flags, "\\Answered") {
				return false
			}
			i++

		case "DELETED":
			if !strings.Contains(msg.flags, "\\Deleted") {
				return false
			}
			i++

		case "DRAFT":
			if !strings.Contains(msg.flags, "\\Draft") {
				return false
			}
			i++

		case "FLAGGED":
			if !strings.Contains(msg.flags, "\\Flagged") {
				return false
			}
			i++

		case "NEW":
			// NEW = RECENT UNSEEN
			if !strings.Contains(msg.flags, "\\Recent") || strings.Contains(msg.flags, "\\Seen") {
				return false
			}
			i++

		case "OLD":
			// OLD = NOT RECENT
			if strings.Contains(msg.flags, "\\Recent") {
				return false
			}
			i++

		case "RECENT":
			if !strings.Contains(msg.flags, "\\Recent") {
				return false
			}
			i++

		case "SEEN":
			if !strings.Contains(msg.flags, "\\Seen") {
				return false
			}
			i++

		case "UNANSWERED":
			if strings.Contains(msg.flags, "\\Answered") {
				return false
			}
			i++

		case "UNDELETED":
			if strings.Contains(msg.flags, "\\Deleted") {
				return false
			}
			i++

		case "UNDRAFT":
			if strings.Contains(msg.flags, "\\Draft") {
				return false
			}
			i++

		case "UNFLAGGED":
			if strings.Contains(msg.flags, "\\Flagged") {
				return false
			}
			i++

		case "UNSEEN":
			if strings.Contains(msg.flags, "\\Seen") {
				return false
			}
			i++

		case "NOT":
			// NOT <search-key>
			if i+1 >= len(tokens) {
				return false
			}
			i++
			// Evaluate next token and negate result
			nextTokens := []string{tokens[i]}
			// Handle NOT with arguments (e.g., NOT FROM "Smith")
			if i+1 < len(tokens) && s.requiresArgument(strings.ToUpper(tokens[i])) {
				i++
				nextTokens = append(nextTokens, tokens[i])
			}
			if s.evaluateTokens(msg, nextTokens, charset, userID) {
				return false
			}
			i++

		case "OR":
			// OR <search-key1> <search-key2>
			if i+2 >= len(tokens) {
				return false
			}
			i++
			key1Tokens := []string{tokens[i]}
			if i+1 < len(tokens) && s.requiresArgument(strings.ToUpper(tokens[i])) {
				i++
				key1Tokens = append(key1Tokens, tokens[i])
			}
			i++
			key2Tokens := []string{tokens[i]}
			if i+1 < len(tokens) && s.requiresArgument(strings.ToUpper(tokens[i])) {
				i++
				key2Tokens = append(key2Tokens, tokens[i])
			}
			if !s.evaluateTokens(msg, key1Tokens, charset, userID) && !s.evaluateTokens(msg, key2Tokens, charset, userID) {
				return false
			}
			i++

		case "BCC", "CC", "FROM", "SUBJECT", "TO", "BODY", "TEXT":
			// These require a string argument
			if i+1 >= len(tokens) {
				return false
			}
			i++
			searchStr := s.unquote(tokens[i])
			if !s.matchesHeaderOrBody(msg, token, searchStr, charset, userID) {
				return false
			}
			i++

		case "HEADER":
			// HEADER <field-name> <string>
			if i+2 >= len(tokens) {
				return false
			}
			i++
			fieldName := s.unquote(tokens[i])
			i++
			searchStr := s.unquote(tokens[i])
			if !s.matchesHeader(msg, fieldName, searchStr, charset, userID) {
				return false
			}
			i++

		case "KEYWORD":
			// KEYWORD <flag>
			if i+1 >= len(tokens) {
				return false
			}
			i++
			keyword := s.unquote(tokens[i])
			if !strings.Contains(msg.flags, keyword) {
				return false
			}
			i++

		case "UNKEYWORD":
			// UNKEYWORD <flag>
			if i+1 >= len(tokens) {
				return false
			}
			i++
			keyword := s.unquote(tokens[i])
			if strings.Contains(msg.flags, keyword) {
				return false
			}
			i++

		case "LARGER":
			// LARGER <n>
			if i+1 >= len(tokens) {
				return false
			}
			i++
			size, err := strconv.Atoi(tokens[i])
			if err != nil || !s.matchesSize(msg, size, true, userID) {
				return false
			}
			i++

		case "SMALLER":
			// SMALLER <n>
			if i+1 >= len(tokens) {
				return false
			}
			i++
			size, err := strconv.Atoi(tokens[i])
			if err != nil || !s.matchesSize(msg, size, false, userID) {
				return false
			}
			i++

		case "UID":
			// UID <sequence set>
			if i+1 >= len(tokens) {
				return false
			}
			i++
			if !s.matchesUIDSet(int(msg.uid), tokens[i]) {
				return false
			}
			i++

		case "BEFORE", "ON", "SINCE":
			// Date-based searches on internal date
			if i+1 >= len(tokens) {
				return false
			}
			i++
			dateStr := s.unquote(tokens[i])
			if !s.matchesDate(msg.internalDate, dateStr, token) {
				return false
			}
			i++

		case "SENTBEFORE", "SENTON", "SENTSINCE":
			// Date-based searches on Date: header
			if i+1 >= len(tokens) {
				return false
			}
			i++
			dateStr := s.unquote(tokens[i])
			if !s.matchesSentDate(msg, dateStr, token, userID) {
				return false
			}
			i++

		default:
			// Unknown search key - skip it
			i++
		}
	}

	return true
}

// Helper functions for search criteria evaluation

func (s *IMAPServer) isSequenceSet(token string) bool {
	// Check if token looks like a sequence number or range (e.g., "1", "2:4", "1:*", "*")
	if token == "*" {
		return true
	}
	for _, ch := range token {
		if ch != ':' && ch != '*' && (ch < '0' || ch > '9') {
			return false
		}
	}
	return len(token) > 0 && (token[0] >= '0' && token[0] <= '9' || token[0] == '*')
}

func (s *IMAPServer) matchesSequenceSet(seqNum int, set string) bool {
	// Handle single number
	if !strings.Contains(set, ":") && set != "*" {
		num, err := strconv.Atoi(set)
		return err == nil && num == seqNum
	}

	// Handle * (highest sequence number) - for now, just return true
	if set == "*" {
		return true
	}

	// Handle range
	parts := strings.Split(set, ":")
	if len(parts) != 2 {
		return false
	}

	start, end := 0, 0
	if parts[0] == "*" {
		start = seqNum // Will match if seqNum is the highest
	} else {
		start, _ = strconv.Atoi(parts[0])
	}

	if parts[1] == "*" {
		end = 999999 // Effectively infinity
	} else {
		end, _ = strconv.Atoi(parts[1])
	}

	return seqNum >= start && seqNum <= end
}

func (s *IMAPServer) matchesUIDSet(uid int, set string) bool {
	// Similar to sequence set but for UIDs
	return s.matchesSequenceSet(uid, set)
}

func (s *IMAPServer) matchesHeaderOrBody(msg messageInfo, field string, searchStr string, charset string, userID int64) bool {
	// Get user database
	userDB, err := s.GetUserDB(userID)
	if err != nil {
		return false
	}

	// Reconstruct message to search in headers/body
	rawMsg, err := parser.ReconstructMessage(userDB, msg.messageID)
	if err != nil {
		return false
	}

	searchStrUpper := strings.ToUpper(searchStr)

	switch field {
	case "FROM":
		return s.headerContains(rawMsg, "From", searchStrUpper)
	case "TO":
		return s.headerContains(rawMsg, "To", searchStrUpper)
	case "CC":
		return s.headerContains(rawMsg, "Cc", searchStrUpper)
	case "BCC":
		return s.headerContains(rawMsg, "Bcc", searchStrUpper)
	case "SUBJECT":
		return s.headerContains(rawMsg, "Subject", searchStrUpper)
	case "BODY":
		// Search only in message body
		headerEnd := strings.Index(rawMsg, "\r\n\r\n")
		if headerEnd == -1 {
			headerEnd = strings.Index(rawMsg, "\n\n")
		}
		if headerEnd != -1 {
			body := rawMsg[headerEnd:]
			return strings.Contains(strings.ToUpper(body), searchStrUpper)
		}
		return false
	case "TEXT":
		// Search in entire message (headers + body)
		return strings.Contains(strings.ToUpper(rawMsg), searchStrUpper)
	}

	return false
}

func (s *IMAPServer) matchesHeader(msg messageInfo, fieldName string, searchStr string, charset string, userID int64) bool {
	// Get user database
	userDB, err := s.GetUserDB(userID)
	if err != nil {
		return false
	}

	rawMsg, err := parser.ReconstructMessage(userDB, msg.messageID)
	if err != nil {
		return false
	}

	// Special case: empty search string matches any message with that header
	if searchStr == "" {
		return s.hasHeader(rawMsg, fieldName)
	}

	return s.headerContains(rawMsg, fieldName, strings.ToUpper(searchStr))
}

func (s *IMAPServer) hasHeader(rawMsg string, fieldName string) bool {
	lines := strings.Split(rawMsg, "\n")
	fieldNameUpper := strings.ToUpper(fieldName)

	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			break // End of headers
		}
		if strings.HasPrefix(strings.ToUpper(line), fieldNameUpper+":") {
			return true
		}
	}
	return false
}

func (s *IMAPServer) headerContains(rawMsg string, fieldName string, searchStr string) bool {
	lines := strings.Split(rawMsg, "\n")
	fieldNameUpper := strings.ToUpper(fieldName)
	searchStrUpper := strings.ToUpper(searchStr)

	inTargetHeader := false
	var headerValue strings.Builder

	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			break // End of headers
		}

		// Check if this is a continuation line (starts with space or tab)
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
			if inTargetHeader {
				headerValue.WriteString(" ")
				headerValue.WriteString(strings.TrimSpace(line))
			}
			continue
		}

		// New header line
		if strings.HasPrefix(strings.ToUpper(line), fieldNameUpper+":") {
			inTargetHeader = true
			colonIdx := strings.Index(line, ":")
			if colonIdx != -1 {
				headerValue.WriteString(strings.TrimSpace(line[colonIdx+1:]))
			}
		} else {
			inTargetHeader = false
		}
	}

	return strings.Contains(strings.ToUpper(headerValue.String()), searchStrUpper)
}

func (s *IMAPServer) matchesSize(msg messageInfo, size int, larger bool, userID int64) bool {
	// Get user database
	userDB, err := s.GetUserDB(userID)
	if err != nil {
		return false
	}

	rawMsg, err := parser.ReconstructMessage(userDB, msg.messageID)
	if err != nil {
		return false
	}

	msgSize := len(rawMsg)
	if larger {
		return msgSize > size
	}
	return msgSize < size
}

func (s *IMAPServer) matchesDate(internalDate time.Time, dateStr string, comparison string) bool {
	// Parse RFC 3501 date format: "1-Feb-1994" or "01-Feb-1994"
	targetDate, err := s.parseIMAPDate(dateStr)
	if err != nil {
		return false
	}

	// Compare dates (disregarding time and timezone)
	msgDate := time.Date(internalDate.Year(), internalDate.Month(), internalDate.Day(), 0, 0, 0, 0, time.UTC)
	targetDate = time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 0, 0, 0, 0, time.UTC)

	switch comparison {
	case "BEFORE":
		return msgDate.Before(targetDate)
	case "ON":
		return msgDate.Equal(targetDate)
	case "SINCE":
		return msgDate.Equal(targetDate) || msgDate.After(targetDate)
	}

	return false
}

func (s *IMAPServer) matchesSentDate(msg messageInfo, dateStr string, comparison string, userID int64) bool {
	// Get user database
	userDB, err := s.GetUserDB(userID)
	if err != nil {
		return false
	}

	// Get Date: header from message
	rawMsg, err := parser.ReconstructMessage(userDB, msg.messageID)
	if err != nil {
		return false
	}

	lines := strings.Split(rawMsg, "\n")
	var dateHeader string

	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToUpper(line), "DATE:") {
			colonIdx := strings.Index(line, ":")
			if colonIdx != -1 {
				dateHeader = strings.TrimSpace(line[colonIdx+1:])
			}
			break
		}
	}

	if dateHeader == "" {
		return false
	}

	// Parse the Date: header (RFC 2822 format)
	sentDate, err := time.Parse(time.RFC1123Z, dateHeader)
	if err != nil {
		// Try RFC1123
		sentDate, err = time.Parse(time.RFC1123, dateHeader)
		if err != nil {
			return false
		}
	}

	// Use the date matching logic
	comparisonType := strings.TrimPrefix(comparison, "SENT")
	return s.matchesDate(sentDate, dateStr, comparisonType)
}

func (s *IMAPServer) parseIMAPDate(dateStr string) (time.Time, error) {
	// RFC 3501 date format: "1-Feb-1994" or "01-Feb-1994"
	// Try both formats
	t, err := time.Parse("2-Jan-2006", dateStr)
	if err != nil {
		t, err = time.Parse("02-Jan-2006", dateStr)
	}
	return t, err
}

func (s *IMAPServer) unquote(str string) string {
	str = strings.TrimSpace(str)
	if len(str) >= 2 && str[0] == '"' && str[len(str)-1] == '"' {
		return str[1 : len(str)-1]
	}
	return str
}

func (s *IMAPServer) requiresArgument(token string) bool {
	switch token {
	case "BCC", "CC", "FROM", "SUBJECT", "TO", "BODY", "TEXT",
		"KEYWORD", "UNKEYWORD", "LARGER", "SMALLER", "UID",
		"BEFORE", "ON", "SINCE", "SENTBEFORE", "SENTON", "SENTSINCE":
		return true
	case "HEADER":
		return true // Actually requires 2 arguments, but handle separately
	}
	return false
}

// ===== STORE =====

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

	// Get user database
	userDB, err := s.GetUserDB(state.UserID)
	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
		return
	}

	// Parse sequence set
	sequences := utils.ParseSequenceSetWithDB(sequenceSet, state.SelectedMailboxID, userDB)
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
		err := userDB.QueryRow(query, state.SelectedMailboxID, seqNum-1).Scan(&messageID, &uid, &currentFlags)
		if err != nil {
			// Message not found - skip
			continue
		}

		// Calculate new flags based on operation
		updatedFlags := s.calculateNewFlags(currentFlags, newFlags, dataItem)

		// Update flags in database
		updateQuery := "UPDATE message_mailbox SET flags = ? WHERE message_id = ? AND mailbox_id = ?"
		_, err = userDB.Exec(updateQuery, updatedFlags, messageID, state.SelectedMailboxID)
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

// ===== COPY =====

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

	// Get user database
	userDB, err := s.GetUserDB(state.UserID)
	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
		return
	}

	// Parse sequence set
	sequences := utils.ParseSequenceSetWithDB(sequenceSet, state.SelectedMailboxID, userDB)
	if len(sequences) == 0 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid sequence set", tag))
		return
	}

	// Check if destination mailbox exists
	var destMailboxID int64
	err = userDB.QueryRow(`
		SELECT id FROM mailboxes
		WHERE name = ? AND user_id = ?
	`, destMailbox, state.UserID).Scan(&destMailboxID)

	if err != nil {
		// Destination mailbox doesn't exist - return NO with [TRYCREATE]
		s.sendResponse(conn, fmt.Sprintf("%s NO [TRYCREATE] Destination mailbox does not exist", tag))
		return
	}

	// Begin transaction to ensure atomicity
	tx, err := userDB.Begin()
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

// ===== APPEND =====

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

	// Get user database
	userDB, err := s.GetUserDB(state.UserID)
	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
		return
	}

	// Parse folder name (could be quoted)
	folder := strings.Trim(parts[2], "\"")

	// Validate folder exists using the database with new schema
	mailboxID, err := db.GetMailboxByNamePerUser(userDB, state.UserID, folder)
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

	// Look for literal size indicator {size} or {size+}
	literalStartIdx := strings.Index(fullLine, "{")
	literalEndIdx := strings.Index(fullLine, "}")

	if literalStartIdx == -1 || literalEndIdx == -1 || literalStartIdx > literalEndIdx {
		s.sendResponse(conn, fmt.Sprintf("%s BAD APPEND requires message size", tag))
		return
	}

	// Extract the size and check for LITERAL+ (RFC 4466)
	sizeStr := fullLine[literalStartIdx+1 : literalEndIdx]
	isLiteralPlus := strings.HasSuffix(sizeStr, "+")
	if isLiteralPlus {
		sizeStr = strings.TrimSuffix(sizeStr, "+")
	}

	var messageSize int
	fmt.Sscanf(sizeStr, "%d", &messageSize)

	if messageSize <= 0 || messageSize > 50*1024*1024 { // Max 50MB
		s.sendResponse(conn, fmt.Sprintf("%s NO Message size invalid or too large", tag))
		return
	}

	// Send continuation response only for synchronizing literals
	// RFC 4466: LITERAL+ ({size+}) means client sends data immediately without waiting
	if !isLiteralPlus {
		s.sendResponse(conn, "+ Ready for literal data")
	}

	// Read the message data using io.ReadFull to ensure we read exactly messageSize bytes
	messageData := make([]byte, messageSize)

	conn.SetReadDeadline(time.Now().Add(5 * time.Minute))
	log.Printf("APPEND expecting %d bytes literal", messageSize)

	n, err := io.ReadFull(conn, messageData)
	if err != nil {
		log.Printf("Error reading message data: expected %d bytes, read %d bytes, error: %v", messageSize, n, err)
		s.sendResponse(conn, fmt.Sprintf("%s NO Failed to read message data", tag))
		return
	}

	log.Printf("APPEND successfully read %d bytes", n)

	// Read and discard the trailing CRLF after the literal data
	// RFC 3501: The client sends CRLF after the literal data
	// Use a short timeout to avoid delays
	crlfBuf := make([]byte, 2)
	conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	n, err = conn.Read(crlfBuf)
	if err != nil {
		log.Printf("Warning: Failed to read trailing CRLF after literal data: %v", err)
		// Continue anyway - some clients might not send it
	} else if n > 0 && !(crlfBuf[0] == '\r' || crlfBuf[0] == '\n') {
		log.Printf("Warning: Expected CRLF after literal data, got: %v", crlfBuf[:n])
		// Continue anyway - be lenient with protocol violations
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
	messageID, err := parser.StoreMessagePerUser(userDB, parsed)
	if err != nil {
		log.Printf("Failed to store message: %v", err)
		s.sendResponse(conn, fmt.Sprintf("%s NO [SERVERBUG] Failed to save message", tag))
		return
	}

	// Add message to mailbox
	internalDate := time.Now()
	err = db.AddMessageToMailboxPerUser(userDB, messageID, mailboxID, flags, internalDate)
	if err != nil {
		log.Printf("Failed to add message to mailbox: %v", err)
		s.sendResponse(conn, fmt.Sprintf("%s NO [SERVERBUG] Failed to add message to mailbox", tag))
		return
	}

	// Get UID validity for APPENDUID response
	uidValidity, _, err := db.GetMailboxInfoPerUser(userDB, mailboxID)
	if err != nil {
		uidValidity = 1
	}

	// Get the UID assigned to the message
	var newUID int64
	query := "SELECT uid FROM message_mailbox WHERE message_id = ? AND mailbox_id = ?"
	err = userDB.QueryRow(query, messageID, mailboxID).Scan(&newUID)
	if err != nil {
		log.Printf("Failed to get new UID: %v", err)
		newUID = 1
	}

	log.Printf("Message appended to folder '%s' with UID %d", folder, newUID)

	// Send success response with APPENDUID (RFC 4315 - UIDPLUS extension)
	s.sendResponse(conn, fmt.Sprintf("%s OK [APPENDUID %d %d] APPEND completed", tag, uidValidity, newUID))
}

// ===== EXPUNGE =====

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

	// Get user database
	userDB, err := s.GetUserDB(state.UserID)
	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
		return
	}

	// Query for all messages with \Deleted flag, ordered by sequence number
	// We need to get the sequence numbers before deletion
	rows, err := userDB.Query(`
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
	allRows, err := userDB.Query(`
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
		userDB.Exec(`DELETE FROM message_mailbox WHERE id = ?`, msg.id)

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

// ===== CHECK =====

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

	// Get user database
	userDB, err := s.GetUserDB(state.UserID)
	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s OK CHECK completed", tag))
		return
	}

	// Perform checkpoint operations for the currently selected mailbox
	// This involves resolving the server's in-memory state with the state on disk
	// In our implementation, this is similar to NOOP but emphasizes housekeeping

	// Get current mailbox state
	currentCount, err := db.GetMessageCountPerUser(userDB, state.SelectedMailboxID)
	if err != nil {
		// If there's a database error, still complete normally per RFC 3501
		// CHECK should always succeed even if housekeeping fails
		s.sendResponse(conn, fmt.Sprintf("%s OK CHECK completed", tag))
		return
	}

	currentRecent, err := db.GetUnseenCountPerUser(userDB, state.SelectedMailboxID)
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

// ===== EXPORTED METHODS FOR TESTING =====

// HandleCheck exports the check handler for testing
func (s *IMAPServer) HandleCheck(conn net.Conn, tag string, state *models.ClientState) {
	s.handleCheck(conn, tag, state)
}

// ===== HELPER METHODS FOR FETCH =====

// buildEnvelope builds an ENVELOPE structure from a message
// ENVELOPE: (date subject from sender reply-to to cc bcc in-reply-to message-id)
func (s *IMAPServer) buildEnvelope(rawMsg string) string {
	// Extract headers
	date := s.extractHeader(rawMsg, "Date")
	subject := s.extractHeader(rawMsg, "Subject")
	from := s.extractHeader(rawMsg, "From")
	sender := s.extractHeader(rawMsg, "Sender")
	replyTo := s.extractHeader(rawMsg, "Reply-To")
	to := s.extractHeader(rawMsg, "To")
	cc := s.extractHeader(rawMsg, "Cc")
	bcc := s.extractHeader(rawMsg, "Bcc")
	inReplyTo := s.extractHeader(rawMsg, "In-Reply-To")
	messageID := s.extractHeader(rawMsg, "Message-ID")

	// If sender is empty, use from
	if sender == "" {
		sender = from
	}
	// If reply-to is empty, use from
	if replyTo == "" {
		replyTo = from
	}

	// Build ENVELOPE structure
	envelope := fmt.Sprintf("ENVELOPE (%s %s %s %s %s %s %s %s %s %s)",
		s.quoteOrNIL(date),
		s.quoteOrNIL(subject),
		s.parseAddressList(from),
		s.parseAddressList(sender),
		s.parseAddressList(replyTo),
		s.parseAddressList(to),
		s.parseAddressList(cc),
		s.parseAddressList(bcc),
		s.quoteOrNIL(inReplyTo),
		s.quoteOrNIL(messageID),
	)

	return envelope
}

// extractHeader extracts a header value from a raw message
func (s *IMAPServer) extractHeader(rawMsg string, headerName string) string {
	lines := strings.Split(rawMsg, "\n")
	headerNameUpper := strings.ToUpper(headerName)
	var headerValue strings.Builder
	inHeader := false

	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			break // End of headers
		}

		// Check if continuation line
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
			if inHeader {
				headerValue.WriteString(" ")
				headerValue.WriteString(strings.TrimSpace(line))
			}
			continue
		}

		// New header line
		colonIdx := strings.Index(line, ":")
		if colonIdx != -1 {
			currentHeader := strings.TrimSpace(line[:colonIdx])
			if strings.ToUpper(currentHeader) == headerNameUpper {
				inHeader = true
				headerValue.WriteString(strings.TrimSpace(line[colonIdx+1:]))
			} else {
				inHeader = false
			}
		}
	}

	return headerValue.String()
}

// quoteOrNIL quotes a string or returns NIL if empty
func (s *IMAPServer) quoteOrNIL(str string) string {
	if str == "" {
		return "NIL"
	}
	// Escape quotes and backslashes in the string
	str = strings.ReplaceAll(str, "\\", "\\\\")
	str = strings.ReplaceAll(str, "\"", "\\\"")
	return fmt.Sprintf("\"%s\"", str)
}

// parseAddressList parses an address header into IMAP address list format
// Address list: ((name route mailbox host) ...) or NIL
func (s *IMAPServer) parseAddressList(addresses string) string {
	if addresses == "" {
		return "NIL"
	}

	// Simple parser - split by comma
	addrs := strings.Split(addresses, ",")
	var addrStructs []string

	for _, addr := range addrs {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}

		// Parse "Name <email@domain.com>" or just "email@domain.com"
		name := ""
		email := addr

		// Check for name part
		if strings.Contains(addr, "<") && strings.Contains(addr, ">") {
			start := strings.Index(addr, "<")
			end := strings.Index(addr, ">")
			name = strings.TrimSpace(addr[:start])
			email = addr[start+1 : end]
			// Remove quotes from name if present
			name = strings.Trim(name, "\"")
		}

		// Parse email into mailbox@host
		mailbox := email
		host := ""
		if strings.Contains(email, "@") {
			parts := strings.SplitN(email, "@", 2)
			mailbox = parts[0]
			host = parts[1]
		}

		// Build address structure: (name route mailbox host)
		// route is always NIL in modern email
		addrStruct := fmt.Sprintf("(%s NIL %s %s)",
			s.quoteOrNIL(name),
			s.quoteOrNIL(mailbox),
			s.quoteOrNIL(host),
		)
		addrStructs = append(addrStructs, addrStruct)
	}

	if len(addrStructs) == 0 {
		return "NIL"
	}

	return "(" + strings.Join(addrStructs, " ") + ")"
}

// buildBodyStructure builds a BODYSTRUCTURE response
func (s *IMAPServer) buildBodyStructure(rawMsg string) string {
	// Extract Content-Type header
	contentType := s.extractHeader(rawMsg, "Content-Type")
	if contentType == "" {
		contentType = "text/plain; charset=us-ascii"
	}

	// Parse content type and parameters
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = "text/plain"
		params = map[string]string{"charset": "us-ascii"}
	}

	// Split media type into type and subtype
	typeParts := strings.SplitN(mediaType, "/", 2)
	mainType := "TEXT"
	subType := "PLAIN"
	if len(typeParts) == 2 {
		mainType = strings.ToUpper(typeParts[0])
		subType = strings.ToUpper(typeParts[1])
	}

	// Handle multipart messages
	if strings.HasPrefix(strings.ToLower(mediaType), "multipart/") {
		boundary := params["boundary"]
		if boundary != "" {
			return s.buildMultipartBodyStructure(rawMsg, mainType, subType, boundary)
		}
	}

	// For non-multipart messages, return basic body structure
	// Get message body
	headerEnd := strings.Index(rawMsg, "\r\n\r\n")
	if headerEnd == -1 {
		headerEnd = strings.Index(rawMsg, "\n\n")
	}
	body := ""
	if headerEnd != -1 {
		body = rawMsg[headerEnd+4:]
	}

	// Get encoding
	encoding := s.extractHeader(rawMsg, "Content-Transfer-Encoding")
	if encoding == "" {
		encoding = "7BIT"
	}
	encoding = strings.ToUpper(encoding)

	// Build parameters list
	paramList := s.buildParamList(params)

	// Get Content-ID and Content-Description
	contentID := s.extractHeader(rawMsg, "Content-ID")
	contentDesc := s.extractHeader(rawMsg, "Content-Description")

	// Count lines for text types
	lines := 0
	if mainType == "TEXT" {
		lines = strings.Count(body, "\n")
	}

	// Format: (type subtype (params) id description encoding size [lines for text])
	if mainType == "TEXT" {
		return fmt.Sprintf("BODYSTRUCTURE (%s %s %s %s %s %s %d %d)",
			s.quoteOrNIL(mainType),
			s.quoteOrNIL(subType),
			paramList,
			s.quoteOrNIL(contentID),
			s.quoteOrNIL(contentDesc),
			s.quoteOrNIL(encoding),
			len(body),
			lines,
		)
	}

	return fmt.Sprintf("BODYSTRUCTURE (%s %s %s %s %s %s %d)",
		s.quoteOrNIL(mainType),
		s.quoteOrNIL(subType),
		paramList,
		s.quoteOrNIL(contentID),
		s.quoteOrNIL(contentDesc),
		s.quoteOrNIL(encoding),
		len(body),
	)
}

// buildMultipartBodyStructure builds BODYSTRUCTURE for multipart messages
func (s *IMAPServer) buildMultipartBodyStructure(rawMsg, mainType, subType, boundary string) string {
	// Get the body part (after headers)
	headerEnd := strings.Index(rawMsg, "\r\n\r\n")
	if headerEnd == -1 {
		headerEnd = strings.Index(rawMsg, "\n\n")
		if headerEnd == -1 {
			// No headers found, return fallback
			return s.buildFallbackBodyStructure(mainType, subType)
		}
		headerEnd += 2
	} else {
		headerEnd += 4
	}
	body := rawMsg[headerEnd:]

	// Normalize line endings for multipart parsing
	if !strings.Contains(body, "\r\n") {
		body = strings.ReplaceAll(body, "\n", "\r\n")
	}

	// Parse multipart body
	var parts []string
	mr := multipart.NewReader(strings.NewReader(body), boundary)

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Log error but try to continue parsing (might be a boundary issue)
			// Fall back to manual parsing if multipart.Reader fails completely
			if len(parts) == 0 {
				return s.buildFallbackMultipartBodyStructure(rawMsg, mainType, subType, boundary)
			}
			break
		}

		// Read part content
		partContent, err := io.ReadAll(part)
		if err != nil {
			continue
		}

		// Build part headers
		var partHeaders strings.Builder
		for key, values := range part.Header {
			for _, value := range values {
				partHeaders.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
			}
		}
		partHeaders.WriteString("\r\n")
		fullPart := partHeaders.String() + string(partContent)

		// Build BODYSTRUCTURE for this part
		partStruct := s.buildPartStructure(fullPart)
		parts = append(parts, partStruct)
	}

	if len(parts) == 0 {
		// Fallback to manual parsing if multipart.Reader failed
		return s.buildFallbackMultipartBodyStructure(rawMsg, mainType, subType, boundary)
	}

	// Multipart BODYSTRUCTURE format: BODYSTRUCTURE ((part1)(part2)... subtype)
	// Note: Each part is already a complete structure without BODYSTRUCTURE keyword
	return fmt.Sprintf("BODYSTRUCTURE (%s %s)", strings.Join(parts, " "), s.quoteOrNIL(subType))
}

// buildPartStructure builds BODYSTRUCTURE for a single MIME part
func (s *IMAPServer) buildPartStructure(partMsg string) string {
	// Extract Content-Type
	contentType := s.extractHeader(partMsg, "Content-Type")
	if contentType == "" {
		contentType = "text/plain; charset=us-ascii"
	}

	// Parse media type
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = "text/plain"
		params = map[string]string{"charset": "us-ascii"}
	}

	typeParts := strings.SplitN(mediaType, "/", 2)
	mainType := "TEXT"
	subType := "PLAIN"
	if len(typeParts) == 2 {
		mainType = strings.ToUpper(typeParts[0])
		subType = strings.ToUpper(typeParts[1])
	}

	// Get encoding
	encoding := s.extractHeader(partMsg, "Content-Transfer-Encoding")
	if encoding == "" {
		encoding = "7BIT"
	}
	encoding = strings.ToUpper(encoding)

	// Get body
	headerEnd := strings.Index(partMsg, "\r\n\r\n")
	if headerEnd == -1 {
		headerEnd = strings.Index(partMsg, "\n\n")
		if headerEnd == -1 {
			headerEnd = 0
		} else {
			headerEnd += 2
		}
	} else {
		headerEnd += 4
	}
	body := ""
	if headerEnd < len(partMsg) {
		body = partMsg[headerEnd:]
	}

	// Build parameters
	paramList := s.buildParamList(params)

	// Get Content-ID and Content-Description
	contentID := s.extractHeader(partMsg, "Content-ID")
	contentDesc := s.extractHeader(partMsg, "Content-Description")

	// Get Content-Disposition and filename
	disposition := s.extractHeader(partMsg, "Content-Disposition")
	var dispList string
	if disposition != "" {
		dispType, dispParams, _ := mime.ParseMediaType(disposition)
		dispParamList := s.buildParamList(dispParams)
		dispList = fmt.Sprintf("(%s %s)", s.quoteOrNIL(strings.ToUpper(dispType)), dispParamList)
	} else {
		dispList = "NIL"
	}

	// Count lines for text types
	lines := 0
	if mainType == "TEXT" {
		lines = strings.Count(body, "\n")
		return fmt.Sprintf("(%s %s %s %s %s %s %d %d NIL %s NIL)",
			s.quoteOrNIL(mainType),
			s.quoteOrNIL(subType),
			paramList,
			s.quoteOrNIL(contentID),
			s.quoteOrNIL(contentDesc),
			s.quoteOrNIL(encoding),
			len(body),
			lines,
			dispList,
		)
	}

	return fmt.Sprintf("(%s %s %s %s %s %s %d NIL %s NIL)",
		s.quoteOrNIL(mainType),
		s.quoteOrNIL(subType),
		paramList,
		s.quoteOrNIL(contentID),
		s.quoteOrNIL(contentDesc),
		s.quoteOrNIL(encoding),
		len(body),
		dispList,
	)
}

// buildParamList builds parameter list for BODYSTRUCTURE
func (s *IMAPServer) buildParamList(params map[string]string) string {
	if len(params) == 0 {
		return "NIL"
	}

	var paramPairs []string
	for key, value := range params {
		paramPairs = append(paramPairs, fmt.Sprintf("%s %s",
			s.quoteOrNIL(strings.ToUpper(key)),
			s.quoteOrNIL(value)))
	}

	return fmt.Sprintf("(%s)", strings.Join(paramPairs, " "))
}

// buildFallbackBodyStructure builds a simple fallback BODYSTRUCTURE
func (s *IMAPServer) buildFallbackBodyStructure(mainType, subType string) string {
	return fmt.Sprintf("BODYSTRUCTURE (%s %s NIL NIL NIL \"7BIT\" 0)",
		s.quoteOrNIL(mainType), s.quoteOrNIL(subType))
}

// buildFallbackMultipartBodyStructure manually parses multipart messages when multipart.Reader fails
func (s *IMAPServer) buildFallbackMultipartBodyStructure(rawMsg, mainType, subType, boundary string) string {
	// Get the body part (after headers)
	headerEnd := strings.Index(rawMsg, "\r\n\r\n")
	if headerEnd == -1 {
		headerEnd = strings.Index(rawMsg, "\n\n")
		if headerEnd == -1 {
			return s.buildFallbackBodyStructure(mainType, subType)
		}
		headerEnd += 2
	} else {
		headerEnd += 4
	}
	body := rawMsg[headerEnd:]

	// Normalize line endings
	if !strings.Contains(body, "\r\n") {
		body = strings.ReplaceAll(body, "\n", "\r\n")
	}

	// Manually split by boundary
	boundaryMarker := "--" + boundary
	closeBoundary := "--" + boundary + "--"

	// Split the body into parts
	partSections := strings.Split(body, boundaryMarker)
	var parts []string

	for i, section := range partSections {
		// Skip the preamble (before first boundary) and epilogue (after final boundary)
		if i == 0 || strings.TrimSpace(section) == "" {
			continue
		}

		// Check if this is the closing boundary
		if strings.HasPrefix(strings.TrimSpace(section), "--") {
			break
		}

		// Remove the closing boundary if present
		section = strings.TrimPrefix(section, "\r\n")
		section = strings.TrimPrefix(section, "\n")

		// Check if this section ends with the closing boundary
		if idx := strings.Index(section, closeBoundary); idx != -1 {
			section = section[:idx]
		}

		// Parse this part's structure
		if strings.TrimSpace(section) != "" {
			partStruct := s.buildPartStructure(section)
			parts = append(parts, partStruct)
		}
	}

	if len(parts) == 0 {
		// Still no parts found, return simple fallback
		return s.buildFallbackBodyStructure(mainType, subType)
	}

	// Return the multipart structure
	return fmt.Sprintf("BODYSTRUCTURE (%s %s)", strings.Join(parts, " "), s.quoteOrNIL(subType))
}
