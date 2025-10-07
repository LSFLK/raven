package server

import (
	"database/sql"
	"fmt"
	"net"
	"strconv"
	"strings"

	"go-imap/internal/models"
)

func (s *IMAPServer) handleSelect(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	if len(parts) < 3 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD SELECT requires folder name", tag))
		return
	}

	folder := strings.Trim(parts[2], "\"")
	state.SelectedFolder = folder
	tableName := s.getUserTableName(state.Username)

	var count int
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE folder = ?", tableName)
	err := s.db.QueryRow(query, folder).Scan(&count)
	if err != nil {
		count = 0
	}

	var recent int
	query = fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE folder = ? AND flags NOT LIKE '%%\\Seen%%'", tableName)
	err = s.db.QueryRow(query, folder).Scan(&recent)
	if err != nil {
		recent = 0
	}

	// Get the next UID (max ID + 1)
	var maxUID int
	query = fmt.Sprintf("SELECT COALESCE(MAX(id), 0) FROM %s WHERE folder = ?", tableName)
	err = s.db.QueryRow(query, folder).Scan(&maxUID)
	if err != nil {
		maxUID = 0
	}

	// Initialize state tracking for NOOP and other commands
	state.LastMessageCount = count
	state.LastRecentCount = recent

	s.sendResponse(conn, fmt.Sprintf("* %d EXISTS", count))
	s.sendResponse(conn, fmt.Sprintf("* %d RECENT", recent))
	s.sendResponse(conn, "* OK [UIDVALIDITY 1] UID validity status")
	s.sendResponse(conn, fmt.Sprintf("* OK [UIDNEXT %d] Predicted next UID", maxUID+1))
	s.sendResponse(conn, "* FLAGS (\\Answered \\Flagged \\Deleted \\Seen \\Draft)")
	s.sendResponse(conn, "* OK [PERMANENTFLAGS (\\Answered \\Flagged \\Deleted \\Seen \\Draft \\*)] Flags permitted")

	cmd := strings.ToUpper(parts[1])
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

	if state.SelectedFolder == "" {
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
	tableName := s.getUserTableName(state.Username)

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
			// Get max count for end
			query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE folder = ?", tableName)
			s.db.QueryRow(query, state.SelectedFolder).Scan(&end)
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
		query := fmt.Sprintf("SELECT id, raw_message, flags FROM %s WHERE folder = ? ORDER BY id ASC LIMIT ? OFFSET ?", tableName)
		rows, err = s.db.Query(query, state.SelectedFolder, end-start+1, start-1)
	} else if sequence == "1:*" || sequence == "*" {
		query := fmt.Sprintf("SELECT id, raw_message, flags FROM %s WHERE folder = ? ORDER BY id ASC", tableName)
		rows, err = s.db.Query(query, state.SelectedFolder)
	} else {
		msgNum, parseErr := strconv.Atoi(sequence)
		if parseErr != nil {
			s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid sequence number", tag))
			return
		}
		query := fmt.Sprintf("SELECT id, raw_message, flags FROM %s WHERE folder = ? ORDER BY id ASC LIMIT 1 OFFSET ?", tableName)
		rows, err = s.db.Query(query, state.SelectedFolder, msgNum-1)
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
		var id int
		var rawMsg, flags string
		rows.Scan(&id, &rawMsg, &flags)

		if !strings.Contains(rawMsg, "\r\n") {
			rawMsg = strings.ReplaceAll(rawMsg, "\n", "\r\n")
		}

		itemsUpper := strings.ToUpper(items)
		responseParts := []string{}
		var literalData string // Store literal data separately

		if strings.Contains(itemsUpper, "UID") {
			responseParts = append(responseParts, fmt.Sprintf("UID %d", id))
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
			var internalDate string
			query := fmt.Sprintf("SELECT date_sent FROM %s WHERE id = ?", tableName)
			s.db.QueryRow(query, id).Scan(&internalDate)
			if internalDate == "" {
				internalDate = "01-Jan-1970 00:00:00 +0000"
			}
			responseParts = append(responseParts, fmt.Sprintf("INTERNALDATE \"%s\"", internalDate))
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
	} else if strings.Contains(itemsUpper, "BODY[]") || strings.Contains(itemsUpper, "RFC822.TEXT") || (strings.Contains(itemsUpper, "RFC822") && !strings.Contains(itemsUpper, "RFC822.SIZE")) {
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
		seqNum++
	}

	s.sendResponse(conn, fmt.Sprintf("%s OK FETCH completed", tag))
}

func (s *IMAPServer) handleSearch(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	if state.SelectedFolder == "" {
		s.sendResponse(conn, fmt.Sprintf("%s NO No folder selected", tag))
		return
	}

	tableName := s.getUserTableName(state.Username)
	query := fmt.Sprintf("SELECT ROW_NUMBER() OVER (ORDER BY id ASC) as seq FROM %s WHERE folder = ?", tableName)
	rows, err := s.db.Query(query, state.SelectedFolder)
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
		s.sendResponse(conn, fmt.Sprintf("%s BAD STATUS requires folder and items", tag))
		return
	}

	folder := strings.Trim(parts[2], "\"")
	tableName := s.getUserTableName(state.Username)

	var count int
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE folder = ?", tableName)
	s.db.QueryRow(query, folder).Scan(&count)

	// Get the next UID (max ID + 1)
	var maxUID int
	query = fmt.Sprintf("SELECT COALESCE(MAX(id), 0) FROM %s WHERE folder = ?", tableName)
	s.db.QueryRow(query, folder).Scan(&maxUID)

	s.sendResponse(conn, fmt.Sprintf("* STATUS \"%s\" (MESSAGES %d RECENT 0 UIDNEXT %d UIDVALIDITY 1 UNSEEN 0)", folder, count, maxUID+1))
	s.sendResponse(conn, fmt.Sprintf("%s OK STATUS completed", tag))
}
