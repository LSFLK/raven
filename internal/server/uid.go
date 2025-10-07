package server

import (
	"database/sql"
	"fmt"
	"net"
	"strconv"
	"strings"

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
	tableName := s.getUserTableName(state.Username)

	var rows *sql.Rows
	var err error

	// Parse comma-separated UID sequences (e.g., "3:4,6:7" or "1,3,5:7")
	sequences := strings.Split(sequence, ",")
	var uidRanges []string
	var args []interface{}
	args = append(args, state.SelectedFolder)

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
			uidRanges = append(uidRanges, "(id >= ? AND id <= ?)")
			args = append(args, start, end)
		} else {
			// Handle single UID (e.g., "3")
			uid, parseErr := strconv.Atoi(seq)
			if parseErr != nil {
				s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid UID", tag))
				return
			}
			uidRanges = append(uidRanges, "id = ?")
			args = append(args, uid)
		}
	}

	// Build the query with OR conditions for multiple ranges
	whereClause := strings.Join(uidRanges, " OR ")
	query := fmt.Sprintf("SELECT id, raw_message, flags, ROW_NUMBER() OVER (ORDER BY id ASC) as seq FROM %s WHERE folder = ? AND (%s) ORDER BY id ASC", tableName, whereClause)
	rows, err = s.db.Query(query, args...)

	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id, seqNum int
		var rawMsg, flags string
		rows.Scan(&id, &rawMsg, &flags, &seqNum)

		if !strings.Contains(rawMsg, "\r\n") {
			rawMsg = strings.ReplaceAll(rawMsg, "\n", "\r\n")
		}

	itemsUpper := strings.ToUpper(items)
	var responseParts []string
	var literalData string // Store literal data separately

	if strings.Contains(itemsUpper, "UID") || true {
		responseParts = append(responseParts, fmt.Sprintf("UID %d", id))
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

	// Handle BODY.PEEK[HEADER.FIELDS (...)] - specific header fields
	if strings.Contains(itemsUpper, "BODY.PEEK[HEADER.FIELDS") {
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
	} else if strings.Contains(itemsUpper, "BODY.PEEK[HEADER]") {
		// Handle BODY.PEEK[HEADER] - all headers
		headerEnd := strings.Index(rawMsg, "\r\n\r\n")
		headers := rawMsg
		if headerEnd != -1 {
			headers = rawMsg[:headerEnd+2] // include last CRLF
		}
		responseParts = append(responseParts, "BODY[HEADER]")
		literalData = fmt.Sprintf("{%d}\r\n%s", len(headers), headers)
	} else if strings.Contains(itemsUpper, "RFC822.HEADER") {
		// RFC822.HEADER - return only the header portion
		headerEnd := strings.Index(rawMsg, "\r\n\r\n")
		headers := rawMsg
		if headerEnd != -1 {
			headers = rawMsg[:headerEnd+2] // include last CRLF
		}
		responseParts = append(responseParts, "RFC822.HEADER")
		literalData = fmt.Sprintf("{%d}\r\n%s", len(headers), headers)
	} else if strings.Contains(itemsUpper, "BODY[]") || strings.Contains(itemsUpper, "BODY.PEEK[]") || strings.Contains(itemsUpper, "RFC822.TEXT") || (strings.Contains(itemsUpper, "RFC822") && !strings.Contains(itemsUpper, "RFC822.SIZE")) {
		// RFC822 = entire message (same as BODY[])
		// RFC822.TEXT = body text only (excluding headers)
		if strings.Contains(itemsUpper, "RFC822.TEXT") {
			// Return only the body (text after headers)
			headerEnd := strings.Index(rawMsg, "\r\n\r\n")
			body := ""
			if headerEnd != -1 {
				body = rawMsg[headerEnd+4:] // skip the double CRLF
			}
			responseParts = append(responseParts, "RFC822.TEXT")
			literalData = fmt.Sprintf("{%d}\r\n%s", len(body), body)
		} else {
			responseParts = append(responseParts, "BODY[]")
			literalData = fmt.Sprintf("{%d}\r\n%s", len(rawMsg), rawMsg)
		}
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

	if state.SelectedFolder == "" {
		s.sendResponse(conn, fmt.Sprintf("%s NO No folder selected", tag))
		return
	}

	tableName := s.getUserTableName(state.Username)
	query := fmt.Sprintf("SELECT id FROM %s WHERE folder = ? ORDER BY id ASC", tableName)
	rows, err := s.db.Query(query, state.SelectedFolder)
	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO Search failed", tag))
		return
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var uid int
		rows.Scan(&uid)
		results = append(results, strconv.Itoa(uid))
	}

	s.sendResponse(conn, fmt.Sprintf("* SEARCH %s", strings.Join(results, " ")))
	s.sendResponse(conn, fmt.Sprintf("%s OK UID SEARCH completed", tag))
}

func (s *IMAPServer) handleUIDStore(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}
	if state.SelectedFolder == "" {
		s.sendResponse(conn, fmt.Sprintf("%s NO No folder selected", tag))
		return
	}
	if len(parts) < 6 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD UID STORE requires sequence, operation, and flags", tag))
		return
	}
	sequence := parts[3]
	flagsStr := strings.Join(parts[5:], " ")
	flagsStr = strings.Trim(flagsStr, "()")
	tableName := s.getUserTableName(state.Username)

	if !strings.Contains(flagsStr, "\\Seen") {
		s.sendResponse(conn, fmt.Sprintf("%s BAD Only \\Seen flag supported", tag))
		return
	}

	// Parse comma-separated UID sequences (e.g., "3:4,6:7" or "1,3,5:7")
	sequences := strings.Split(sequence, ",")
	var uidRanges []string
	var args []interface{}
	args = append(args, state.SelectedFolder)

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
			uidRanges = append(uidRanges, "(id >= ? AND id <= ?)")
			args = append(args, start, end)
		} else {
			// Handle single UID (e.g., "3")
			uid, parseErr := strconv.Atoi(seq)
			if parseErr != nil {
				s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid UID", tag))
				return
			}
			uidRanges = append(uidRanges, "id = ?")
			args = append(args, uid)
		}
	}

	// Build the query with OR conditions for multiple ranges
	whereClause := strings.Join(uidRanges, " OR ")
	query := fmt.Sprintf("UPDATE %s SET flags = CASE WHEN flags LIKE '%%\\Seen%%' THEN flags ELSE flags || ' \\Seen' END WHERE folder = ? AND (%s)", tableName, whereClause)
	_, err := s.db.Exec(query, args...)

	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
		return
	}

	s.sendResponse(conn, fmt.Sprintf("%s OK STORE completed", tag))
}
