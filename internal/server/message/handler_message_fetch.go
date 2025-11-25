package message

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

// HandleFetchForUIDs handles FETCH for a list of UIDs (used by UID FETCH command)
func HandleFetchForUIDs(deps ServerDeps, conn net.Conn, tag string, uids []int, items string, state *models.ClientState) {
	// Get appropriate database (user or role mailbox)
	targetDB, _, err := deps.GetSelectedDB(state)
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
		processFetchForMessage(deps, conn, messageID, int64(uid), seqNum, flags.String, items, state)
	}
}

func HandleFetch(deps ServerDeps, conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		deps.SendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	if state.SelectedMailboxID == 0 {
		deps.SendResponse(conn, fmt.Sprintf("%s NO No folder selected", tag))
		return
	}

	if len(parts) < 4 {
		deps.SendResponse(conn, fmt.Sprintf("%s BAD FETCH requires sequence and items", tag))
		return
	}

	// Get appropriate database (user or role mailbox)
	targetDB, _, err := deps.GetSelectedDB(state)
	if err != nil {
		deps.SendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
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
				deps.SendResponse(conn, fmt.Sprintf("%s BAD Invalid sequence number", tag))
				return
			}
		}
		if seqRange[1] == "*" {
			// Get max count for end using new schema
			end, _ = db.GetMessageCountPerUser(targetDB, state.SelectedMailboxID)
		} else {
			end, err = strconv.Atoi(seqRange[1])
			if err != nil || end < 1 {
				deps.SendResponse(conn, fmt.Sprintf("%s BAD Invalid sequence number", tag))
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
			deps.SendResponse(conn, fmt.Sprintf("%s BAD Invalid sequence number", tag))
			return
		}
		query := `SELECT mm.message_id, mm.uid, mm.flags
		          FROM message_mailbox mm
		          WHERE mm.mailbox_id = ?
		          ORDER BY mm.uid ASC LIMIT 1 OFFSET ?`
		rows, err = targetDB.Query(query, state.SelectedMailboxID, msgNum-1)
	}

	if err != nil {
		deps.SendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
		return
	}
	defer func() { _ = rows.Close() }()

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
		processFetchForMessage(deps, conn, messageID, uid, seqNum, flags, items, state)
		seqNum++
	}

	deps.SendResponse(conn, fmt.Sprintf("%s OK FETCH completed", tag))
}

// processFetchForMessage processes a single message for FETCH/UID FETCH
func processFetchForMessage(deps ServerDeps, conn net.Conn, messageID, uid int64, seqNum int, flags, items string, state *models.ClientState) {
	// Get appropriate database (user or role mailbox)
	targetDB, _, err := deps.GetSelectedDB(state)
	if err != nil {
		return
	}

	// Lazy-load the full reconstructed message only when needed
	var rawMsg string
	var rawMsgErr error
	loadRawMsg := func() string {
		if rawMsg == "" && rawMsgErr == nil {
			rawMsg, rawMsgErr = parser.ReconstructMessage(targetDB, messageID)
			if rawMsgErr != nil {
				return ""
			}
			if !strings.Contains(rawMsg, "\r\n") {
				rawMsg = strings.ReplaceAll(rawMsg, "\n", "\r\n")
			}
		}
		return rawMsg
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
			msg := loadRawMsg()
			responseParts = append(responseParts, fmt.Sprintf("RFC822.SIZE %d", len(msg)))
		}

		// Handle ENVELOPE
		if strings.Contains(itemsUpper, "ENVELOPE") {
			msg := loadRawMsg()
			envelope := response.BuildEnvelope(msg)
			responseParts = append(responseParts, envelope)
		}

		// Handle BODYSTRUCTURE
		if strings.Contains(itemsUpper, "BODYSTRUCTURE") {
			msg := loadRawMsg()
			bodyStructure := response.BuildBodyStructure(msg)
			responseParts = append(responseParts, bodyStructure)
		}

		// Handle BODY (non-extensible BODYSTRUCTURE)
		if strings.Contains(itemsUpper, "BODY") && !strings.Contains(itemsUpper, "BODY[") && !strings.Contains(itemsUpper, "BODY.PEEK") && !strings.Contains(itemsUpper, "BODYSTRUCTURE") {
			// BODY is the non-extensible form of BODYSTRUCTURE
			msg := loadRawMsg()
			bodyStructure := response.BuildBodyStructure(msg)
			// Replace BODYSTRUCTURE with BODY in the response
			bodyStructure = strings.Replace(bodyStructure, "BODYSTRUCTURE", "BODY", 1)
			responseParts = append(responseParts, bodyStructure)
		}

		// Handle numeric BODY sections like BODY.PEEK[1], BODY[2], BODY[1.MIME] with optional partial ranges
		if strings.Contains(itemsUpper, "BODY[") || strings.Contains(itemsUpper, "BODY.PEEK[") {
			// Lazy-load parts for this message if needed
			var parts []map[string]interface{}
			loadParts := func() {
				if parts == nil {
					p, err := db.GetMessageParts(targetDB, messageID)
					if err == nil {
						parts = p
					}
				}
			}

			orig := items
			upper := itemsUpper
			pos := 0
			for {
				idxPeek := strings.Index(upper[pos:], "BODY.PEEK[")
				idxBody := strings.Index(upper[pos:], "BODY[")
				if idxPeek == -1 && idxBody == -1 {
					break
				}
				offset := pos
				prefix := "BODY["
				if idxPeek != -1 && (idxBody == -1 || idxPeek < idxBody) {
					offset += idxPeek
					prefix = "BODY.PEEK["
				} else {
					offset += idxBody
				}

				// Find closing bracket
				start := offset + len(prefix)
				end := strings.Index(upper[start:], "]")
				if end == -1 {
					break
				}
				end = start + end

				sectionSpec := orig[start:end] // preserve original case/format for echo
				sectionUpper := strings.ToUpper(sectionSpec)

				// Only handle numeric sections here; others handled elsewhere
				if len(sectionSpec) > 0 && sectionSpec[0] >= '0' && sectionSpec[0] <= '9' {
					// Determine if .MIME requested
					wantMIME := false
					partNumStr := sectionSpec
					if strings.Contains(sectionUpper, ".MIME") {
						wantMIME = true
						partNumStr = sectionSpec[:strings.Index(sectionUpper, ".MIME")]
					}
					// Parse part number
					pn, err := strconv.Atoi(partNumStr)
					if err == nil && pn > 0 {
						loadParts()
						// Map IMAP part number to database part
						target := mapIMAPPartToDBPart(parts, pn)

						payload := ""
						if target != nil {
							// Check if this is a multipart container (has no body)
							contentType, _ := target["content_type"].(string)
							isMultipart := strings.HasPrefix(contentType, "multipart/")

							if wantMIME {
								// Build MIME headers for the part
								hdr := buildMIMEHeadersForPart(target)
								payload = hdr
							} else if isMultipart {
								// For multipart containers, extract from the full reconstructed message
								fullMsg := loadRawMsg()
								payload = extractBodySection(fullMsg, pn)
							} else {
								// Part body only - for non-multipart parts
								if blobID, ok := target["blob_id"].(int64); ok {
									if content, err := db.GetBlob(targetDB, blobID); err == nil {
										payload = content
									}
								} else if textContent, ok := target["text_content"].(string); ok {
									payload = textContent
								}
							}
						}

						// Check for partial spec immediately following the closing bracket
						partialStartPos := -1
						after := end + 1
						if after < len(upper) && upper[after] == '<' {
							close := strings.Index(upper[after:], ">")
							if close != -1 {
								rangeSpec := upper[after+1 : after+close]
								var startPos, length int
								if _, err := fmt.Sscanf(rangeSpec, "%d.%d", &startPos, &length); err == nil {
									partialStartPos = startPos
									if startPos < len(payload) {
										endPos := startPos + length
										if endPos > len(payload) {
											endPos = len(payload)
										}
										payload = payload[startPos:endPos]
									} else {
										payload = ""
									}
								}
								// Advance parser position past the range
								end = after + close
							}
						}

						// Append response
						if payload == "" {
							responseParts = append(responseParts, fmt.Sprintf("BODY[%s] NIL", sectionSpec))
						} else {
							if literalData != "" {
								literalData += " "
							}
							// Include partial start position in response if this was a partial fetch
							if partialStartPos >= 0 {
								responseParts = append(responseParts, fmt.Sprintf("BODY[%s]<%d>", sectionSpec, partialStartPos))
							} else {
								responseParts = append(responseParts, fmt.Sprintf("BODY[%s]", sectionSpec))
							}
							literalData += fmt.Sprintf("{%d}\r\n%s", len(payload), payload)
						}
					}
				}

				// Move past this section for next search
				pos = end + 1
			}
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
			msg := loadRawMsg()
			headersMap := map[string]string{}
			lines := strings.Split(msg, "\r\n")
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
		msg := loadRawMsg()
		headerEnd := strings.Index(msg, "\r\n\r\n")
		body := ""
		if headerEnd != -1 {
			body = msg[headerEnd+4:] // skip the double CRLF
		}

		// Check for partial fetch like BODY.PEEK[TEXT]<0.2048>
		partialStart := 0
		partialLength := len(body)
		if strings.Contains(itemsUpper, "<") && strings.Contains(itemsUpper, ">") {
			startIdx := strings.Index(itemsUpper, "<")
			endIdx := strings.Index(itemsUpper, ">")
			if startIdx != -1 && endIdx > startIdx {
				partialSpec := itemsUpper[startIdx+1:endIdx]
				_, _ = fmt.Sscanf(partialSpec, "%d.%d", &partialStart, &partialLength)
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
		msg := loadRawMsg()
		headerEnd := strings.Index(msg, "\r\n\r\n")
		headers := msg
		if headerEnd != -1 {
			headers = msg[:headerEnd+2] // include last CRLF
		}
		if literalData != "" {
			literalData += " "
		}
		responseParts = append(responseParts, "BODY[HEADER]")
		literalData += fmt.Sprintf("{%d}\r\n%s", len(headers), headers)
	}

	// Handle RFC822.HEADER - return only the header portion
	if strings.Contains(itemsUpper, "RFC822.HEADER") {
		msg := loadRawMsg()
		headerEnd := strings.Index(msg, "\r\n\r\n")
		headers := msg
		if headerEnd != -1 {
			headers = msg[:headerEnd+2] // include last CRLF
		}
		if literalData != "" {
			literalData += " "
		}
		responseParts = append(responseParts, "RFC822.HEADER")
		literalData += fmt.Sprintf("{%d}\r\n%s", len(headers), headers)
	}

	// Handle RFC822.TEXT - body text only (excluding headers)
	if strings.Contains(itemsUpper, "RFC822.TEXT") {
		msg := loadRawMsg()
		headerEnd := strings.Index(msg, "\r\n\r\n")
		body := ""
		if headerEnd != -1 {
			body = msg[headerEnd+4:] // skip the double CRLF
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
		msg := loadRawMsg()
		if literalData != "" {
			literalData += " "
		}
		responseParts = append(responseParts, "BODY[]")
		literalData += fmt.Sprintf("{%d}\r\n%s", len(msg), msg)
	}

	if len(responseParts) > 0 {
		responseStr := fmt.Sprintf("* %d FETCH (%s", seqNum, strings.Join(responseParts, " "))
		if literalData != "" {
			responseStr += " " + literalData + ")"
		} else {
			responseStr += ")"
		}
		deps.SendResponse(conn, responseStr)
	} else {
		deps.SendResponse(conn, fmt.Sprintf("* %d FETCH (FLAGS ())", seqNum))
	}
}

// extractBodySection extracts a specific MIME body section from a reconstructed message
func extractBodySection(fullMessage string, partNum int) string {
	// Parse the message to find the main content-type and boundary
	lines := strings.Split(fullMessage, "\r\n")

	// Find the main Content-Type header
	var mainBoundary string
	headerEnd := -1
	for i, line := range lines {
		if line == "" {
			headerEnd = i
			break
		}
		if strings.HasPrefix(strings.ToUpper(line), "CONTENT-TYPE:") {
			// Extract boundary from Content-Type header
			ctLine := line
			// Handle multi-line headers
			for j := i + 1; j < len(lines); j++ {
				if len(lines[j]) > 0 && (lines[j][0] == ' ' || lines[j][0] == '\t') {
					ctLine += lines[j]
				} else {
					break
				}
			}
			// Parse boundary
			if idx := strings.Index(strings.ToLower(ctLine), "boundary="); idx != -1 {
				boundaryPart := ctLine[idx+9:]
				if boundaryPart[0] == '"' {
					// Quoted boundary
					endQuote := strings.Index(boundaryPart[1:], "\"")
					if endQuote != -1 {
						mainBoundary = boundaryPart[1 : endQuote+1]
					}
				} else {
					// Unquoted boundary (ends at semicolon or end of line)
					endIdx := strings.IndexAny(boundaryPart, "; \r\n")
					if endIdx != -1 {
						mainBoundary = boundaryPart[:endIdx]
					} else {
						mainBoundary = strings.TrimSpace(boundaryPart)
					}
				}
			}
		}
	}

	if mainBoundary == "" || headerEnd == -1 {
		return ""
	}

	// Find the Nth part (1-indexed)
	body := strings.Join(lines[headerEnd+1:], "\r\n")
	delimiter := "--" + mainBoundary
	parts := strings.Split(body, delimiter)

	// parts[0] is the preamble (before first boundary)
	// parts[1..n] are the actual MIME parts
	// parts[n+1] would be after the closing boundary

	if partNum <= 0 || partNum >= len(parts) {
		return ""
	}

	// Get the requested part (parts array is 0-indexed, but partNum is 1-indexed)
	// parts[0] = preamble, parts[1] = part 1, parts[2] = part 2, etc.
	partContent := parts[partNum]

	// Remove the closing boundary marker if present (--boundary--)
	if strings.HasPrefix(strings.TrimSpace(partContent), "--") {
		return ""
	}

	// Trim leading CRLF that comes after the boundary marker
	partContent = strings.TrimPrefix(partContent, "\r\n")
	partContent = strings.TrimPrefix(partContent, "\n")

	// The part content includes headers and body. We need to extract just the body.
	partLines := strings.Split(partContent, "\r\n")
	partHeaderEnd := -1
	for i, line := range partLines {
		if line == "" {
			partHeaderEnd = i
			break
		}
	}

	if partHeaderEnd == -1 {
		return ""
	}

	// Extract the body (everything after the blank line)
	partBody := strings.Join(partLines[partHeaderEnd+1:], "\r\n")

	// Trim any trailing content that belongs to the outer message
	// Remove trailing newlines and any closing boundaries
	partBody = strings.TrimRight(partBody, "\r\n")

	// Check if there's a closing boundary from parent at the end and remove it
	lastLines := strings.Split(partBody, "\r\n")
	if len(lastLines) > 0 {
		lastLine := lastLines[len(lastLines)-1]
		// If last line is a boundary marker from a different part, remove it
		if strings.HasPrefix(lastLine, "--") && !strings.Contains(partBody[:len(partBody)-len(lastLine)], lastLine[:min(len(lastLine), 20)]) {
			partBody = strings.Join(lastLines[:len(lastLines)-1], "\r\n")
		}
	}

	return partBody
}

// mapIMAPPartToDBPart maps an IMAP part number to the corresponding database part
// IMAP numbering: top-level parts are 1, 2, 3..., nested parts are 1.1, 1.2, etc.
// Database numbering: sequential (1, 2, 3, 4...) with parent_part_id relationships
//
// For a message like:
//   multipart/mixed
//     1: multipart/alternative (container)
//        - text/plain
//        - text/html
//     2: image/png
//
// IMAP part 1 = the multipart/alternative container (should reconstruct the entire section)
// IMAP part 2 = the image/png
func mapIMAPPartToDBPart(parts []map[string]interface{}, imapPartNum int) map[string]interface{} {
	// First, identify which parts are top-level (no parent)
	topLevelParts := []map[string]interface{}{}
	for _, p := range parts {
		parentID, hasParent := p["parent_part_id"]
		if !hasParent || parentID == nil {
			topLevelParts = append(topLevelParts, p)
		}
	}

	// In IMAP, top-level parts are numbered sequentially (including multipart containers)
	// Part 1, Part 2, Part 3, etc.
	if imapPartNum > 0 && imapPartNum <= len(topLevelParts) {
		return topLevelParts[imapPartNum-1]
	}

	return nil
}

// buildMIMEHeadersForPart reconstructs MIME headers for a specific part
func buildMIMEHeadersForPart(part map[string]interface{}) string {
	var b strings.Builder
	contentType := part["content_type"].(string)
	if charset, ok := part["charset"].(string); ok && strings.TrimSpace(charset) != "" {
		b.WriteString(fmt.Sprintf("Content-Type: %s; charset=%s\r\n", contentType, charset))
	} else {
		b.WriteString(fmt.Sprintf("Content-Type: %s\r\n", contentType))
	}
	// Note: filename handling is done in Content-Disposition below
	// Many clients accept name= on Content-Type or only in Content-Disposition
	_ = part["filename"] // checked but handled elsewhere
	if encoding, ok := part["content_transfer_encoding"].(string); ok && strings.TrimSpace(encoding) != "" {
		b.WriteString(fmt.Sprintf("Content-Transfer-Encoding: %s\r\n", encoding))
	}
	if contentID, ok := part["content_id"].(string); ok && strings.TrimSpace(contentID) != "" {
		b.WriteString(fmt.Sprintf("Content-ID: %s\r\n", contentID))
	}
	if disp, ok := part["content_disposition"].(string); ok && strings.TrimSpace(disp) != "" {
		b.WriteString(fmt.Sprintf("Content-Disposition: %s", disp))
		if filename, ok := part["filename"].(string); ok && strings.TrimSpace(filename) != "" {
			b.WriteString(fmt.Sprintf("; filename=\"%s\"", filename))
		}
		b.WriteString("\r\n")
	}
	b.WriteString("\r\n")
	return b.String()
}

