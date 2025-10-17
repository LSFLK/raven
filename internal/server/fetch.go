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
		seqNum++
	}

	s.sendResponse(conn, fmt.Sprintf("%s OK FETCH completed", tag))
}

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
	rows, err := s.db.Query(query, state.SelectedMailboxID)
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
	matchingSeqNums := s.evaluateSearchCriteria(messages, criteria, charset)

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
func (s *IMAPServer) evaluateSearchCriteria(messages []messageInfo, criteria string, charset string) []int {
	var matchingSeqNums []int

	// Default to ALL if no criteria specified
	if strings.TrimSpace(criteria) == "" {
		criteria = "ALL"
	}

	// Parse criteria into tokens
	tokens := s.parseSearchTokens(criteria)

	// Evaluate each message
	for _, msg := range messages {
		if s.matchesSearchCriteria(msg, tokens, charset) {
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
func (s *IMAPServer) matchesSearchCriteria(msg messageInfo, tokens []string, charset string) bool {
	// Default to ALL - match everything
	if len(tokens) == 0 {
		return true
	}

	// Process tokens (AND logic by default)
	return s.evaluateTokens(msg, tokens, charset)
}

// evaluateTokens evaluates a list of search tokens
func (s *IMAPServer) evaluateTokens(msg messageInfo, tokens []string, charset string) bool {
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
			if s.evaluateTokens(msg, nextTokens, charset) {
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
			if !s.evaluateTokens(msg, key1Tokens, charset) && !s.evaluateTokens(msg, key2Tokens, charset) {
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
			if !s.matchesHeaderOrBody(msg, token, searchStr, charset) {
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
			if !s.matchesHeader(msg, fieldName, searchStr, charset) {
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
			if err != nil || !s.matchesSize(msg, size, true) {
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
			if err != nil || !s.matchesSize(msg, size, false) {
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
			if !s.matchesSentDate(msg, dateStr, token) {
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

func (s *IMAPServer) matchesHeaderOrBody(msg messageInfo, field string, searchStr string, charset string) bool {
	// Reconstruct message to search in headers/body
	rawMsg, err := parser.ReconstructMessage(s.db, msg.messageID)
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

func (s *IMAPServer) matchesHeader(msg messageInfo, fieldName string, searchStr string, charset string) bool {
	rawMsg, err := parser.ReconstructMessage(s.db, msg.messageID)
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

func (s *IMAPServer) matchesSize(msg messageInfo, size int, larger bool) bool {
	rawMsg, err := parser.ReconstructMessage(s.db, msg.messageID)
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

func (s *IMAPServer) matchesSentDate(msg messageInfo, dateStr string, comparison string) bool {
	// Get Date: header from message
	rawMsg, err := parser.ReconstructMessage(s.db, msg.messageID)
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
// For simple messages (non-multipart), this is a simplified implementation
func (s *IMAPServer) buildBodyStructure(rawMsg string) string {
	// Extract Content-Type header
	contentType := s.extractHeader(rawMsg, "Content-Type")
	if contentType == "" {
		contentType = "text/plain; charset=us-ascii"
	}

	// Parse content type
	mediaType := "text"
	mediaSubtype := "plain"
	if strings.Contains(contentType, "/") {
		parts := strings.SplitN(contentType, "/", 2)
		mediaType = strings.ToUpper(strings.TrimSpace(parts[0]))
		subParts := strings.Split(parts[1], ";")
		mediaSubtype = strings.ToUpper(strings.TrimSpace(subParts[0]))
	}

	// Get message size (body only)
	headerEnd := strings.Index(rawMsg, "\r\n\r\n")
	if headerEnd == -1 {
		headerEnd = strings.Index(rawMsg, "\n\n")
	}
	bodySize := 0
	if headerEnd != -1 {
		bodySize = len(rawMsg) - headerEnd - 4
	}

	// For non-multipart messages, return basic body structure
	// Format: (type subtype (params) id description encoding size)
	bodyStruct := fmt.Sprintf("BODYSTRUCTURE (%s %s NIL NIL NIL \"7BIT\" %d)",
		s.quoteOrNIL(mediaType),
		s.quoteOrNIL(mediaSubtype),
		bodySize,
	)

	return bodyStruct
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
