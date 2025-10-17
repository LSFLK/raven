package server

import (
	"database/sql"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"

	"go-imap/internal/delivery/parser"
	"go-imap/internal/models"
)

func (s *IMAPServer) handleUID(conn net.Conn, tag string, parts []string, state *models.ClientState) {
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
	default:
		s.sendResponse(conn, fmt.Sprintf("%s BAD Unknown UID command: %s", tag, subCmd))
	}
}

func (s *IMAPServer) handleUIDFetch(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	if state.SelectedFolder == "" {
		s.sendResponse(conn, fmt.Sprintf("%s NO No folder selected", tag))
		return
	}

	if len(parts) < 5 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD UID FETCH requires sequence and items", tag))
		return
	}

	sequence := parts[3]
	items := strings.Join(parts[4:], " ")
	items = strings.Trim(items, "()")

	if state.SelectedMailboxID == 0 {
		s.sendResponse(conn, fmt.Sprintf("%s NO No mailbox selected", tag))
		return
	}

	var rows *sql.Rows
	var err error

	// Parse comma-separated UID sequences (e.g., "3:4,6:7" or "1,3,5:7")
	sequences := strings.Split(sequence, ",")
	var uidRanges []string
	var args []interface{}
	args = append(args, state.SelectedMailboxID)

	for _, seq := range sequences {
		seq = strings.TrimSpace(seq)
		
		if seq == "1:*" || seq == "*" {
			// Handle special case
			uidRanges = append(uidRanges, "1=1")
		} else if strings.Contains(seq, ":") {
			// Handle range (e.g., "3:7")
			r := strings.Split(seq, ":")
			if len(r) != 2 {
				s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid UID range format", tag))
				return
			}
			start, err1 := strconv.Atoi(r[0])
			end, err2 := strconv.Atoi(r[1])
			if err1 != nil || err2 != nil {
				s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid UID range", tag))
				return
			}
			if start > end {
				start, end = end, start
			}
			uidRanges = append(uidRanges, "(uid >= ? AND uid <= ?)")
			args = append(args, start, end)
		} else {
			// Handle single UID (e.g., "3")
			uid, parseErr := strconv.Atoi(seq)
			if parseErr != nil {
				s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid UID", tag))
				return
			}
			uidRanges = append(uidRanges, "uid = ?")
			args = append(args, uid)
		}
	}

	// Build the query with OR conditions for multiple ranges
	whereClause := strings.Join(uidRanges, " OR ")
	query := fmt.Sprintf("SELECT mm.uid, mm.message_id, mm.flags, ROW_NUMBER() OVER (ORDER BY mm.uid ASC) as seq FROM message_mailbox mm WHERE mm.mailbox_id = ? AND (%s) ORDER BY mm.uid ASC", whereClause)
	rows, err = s.db.Query(query, args...)

	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
		return
	}
	defer rows.Close()

	for rows.Next() {
		var uid, messageID, seqNum int64
		var flagsStr sql.NullString
		rows.Scan(&uid, &messageID, &flagsStr, &seqNum)

		flags := ""
		if flagsStr.Valid {
			flags = flagsStr.String
		}

		// Reconstruct message from database
		rawMsg, err := parser.ReconstructMessage(s.db, messageID)
		if err != nil {
			log.Printf("Failed to reconstruct message %d: %v", messageID, err)
			continue
		}

		if !strings.Contains(rawMsg, "\r\n") {
			rawMsg = strings.ReplaceAll(rawMsg, "\n", "\r\n")
		}

	itemsUpper := strings.ToUpper(items)
	var responseParts []string
	var literalData string // Store literal data separately

	if strings.Contains(itemsUpper, "UID") || true {
		responseParts = append(responseParts, fmt.Sprintf("UID %d", uid))
	}

	if strings.Contains(itemsUpper, "FLAGS") {
		flagsStr := "()"
		if flags != "" {
			flagsStr = fmt.Sprintf("(%s)", flags)
		}
		responseParts = append(responseParts, fmt.Sprintf("FLAGS %s", flagsStr))
	}

	if strings.Contains(itemsUpper, "RFC822.SIZE") {
		responseParts = append(responseParts, fmt.Sprintf("RFC822.SIZE %d", len(rawMsg)))
	}

	// Handle multiple body parts - process each separately
	// This allows handling requests like: BODY.PEEK[HEADER.FIELDS (...)] BODY.PEEK[TEXT]
	
	// Handle BODY.PEEK[HEADER.FIELDS (...)] - specific header fields
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
	}

	s.sendResponse(conn, fmt.Sprintf("%s OK UID FETCH completed", tag))
}

func (s *IMAPServer) handleUIDSearch(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	if state.SelectedMailboxID == 0 {
		s.sendResponse(conn, fmt.Sprintf("%s NO No mailbox selected", tag))
		return
	}

	query := "SELECT uid FROM message_mailbox WHERE mailbox_id = ? ORDER BY uid ASC"
	rows, err := s.db.Query(query, state.SelectedMailboxID)
	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO Search failed", tag))
		return
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var uid int64
		rows.Scan(&uid)
		results = append(results, strconv.FormatInt(uid, 10))
	}

	s.sendResponse(conn, fmt.Sprintf("* SEARCH %s", strings.Join(results, " ")))
	s.sendResponse(conn, fmt.Sprintf("%s OK UID SEARCH completed", tag))
}

func (s *IMAPServer) handleUIDStore(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}
	if state.SelectedMailboxID == 0 {
		s.sendResponse(conn, fmt.Sprintf("%s NO No mailbox selected", tag))
		return
	}
	if len(parts) < 6 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD UID STORE requires sequence, operation, and flags", tag))
		return
	}
	sequence := parts[3]
	flagsStr := strings.Join(parts[5:], " ")
	flagsStr = strings.Trim(flagsStr, "()")

	if !strings.Contains(flagsStr, "\\Seen") {
		s.sendResponse(conn, fmt.Sprintf("%s BAD Only \\Seen flag supported", tag))
		return
	}

	// Parse comma-separated UID sequences (e.g., "3:4,6:7" or "1,3,5:7")
	sequences := strings.Split(sequence, ",")
	var uidRanges []string
	var args []interface{}
	args = append(args, state.SelectedMailboxID)

	for _, seq := range sequences {
		seq = strings.TrimSpace(seq)
		
		if seq == "1:*" || seq == "*" {
			// Handle special case
			uidRanges = append(uidRanges, "1=1")
		} else if strings.Contains(seq, ":") {
			// Handle range (e.g., "3:7")
			r := strings.Split(seq, ":")
			if len(r) != 2 {
				s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid UID range format", tag))
				return
			}
			start, err1 := strconv.Atoi(r[0])
			end, err2 := strconv.Atoi(r[1])
			if err1 != nil || err2 != nil {
				s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid UID range", tag))
				return
			}
			if start > end {
				start, end = end, start
			}
			uidRanges = append(uidRanges, "(uid >= ? AND uid <= ?)")
			args = append(args, start, end)
		} else {
			// Handle single UID (e.g., "3")
			uid, parseErr := strconv.Atoi(seq)
			if parseErr != nil {
				s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid UID", tag))
				return
			}
			uidRanges = append(uidRanges, "uid = ?")
			args = append(args, uid)
		}
	}

	// Build the query with OR conditions for multiple ranges
	whereClause := strings.Join(uidRanges, " OR ")
	query := fmt.Sprintf("UPDATE message_mailbox SET flags = CASE WHEN flags LIKE '%%\\Seen%%' THEN flags ELSE COALESCE(flags || ' ', '') || '\\Seen' END WHERE mailbox_id = ? AND (%s)", whereClause)
	_, err := s.db.Exec(query, args...)

	if err != nil {
		log.Printf("UID STORE error: %v", err)
		s.sendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
		return
	}

	s.sendResponse(conn, fmt.Sprintf("%s OK STORE completed", tag))
}
