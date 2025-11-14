package server

import (
	"database/sql"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"raven/internal/db"
	"raven/internal/delivery/parser"
	"raven/internal/models"
	"raven/internal/server/response"
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
			envelope := response.BuildEnvelope(rawMsg)
			responseParts = append(responseParts, envelope)
		}

		// Handle BODYSTRUCTURE
		if strings.Contains(itemsUpper, "BODYSTRUCTURE") {
			bodyStructure := response.BuildBodyStructure(rawMsg)
			responseParts = append(responseParts, bodyStructure)
		}

		// Handle BODY (non-extensible BODYSTRUCTURE)
		if strings.Contains(itemsUpper, "BODY") && !strings.Contains(itemsUpper, "BODY[") && !strings.Contains(itemsUpper, "BODY.PEEK") && !strings.Contains(itemsUpper, "BODYSTRUCTURE") {
			// BODY is the non-extensible form of BODYSTRUCTURE
			bodyStructure := response.BuildBodyStructure(rawMsg)
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

