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

	if sequence == "1:*" {
		query := fmt.Sprintf("SELECT id, raw_message, flags, ROW_NUMBER() OVER (ORDER BY id ASC) as seq FROM %s WHERE folder = ? ORDER BY id ASC", tableName)
		rows, err = s.db.Query(query, state.SelectedFolder)
	} else if strings.Contains(sequence, ":") {
		r := strings.Split(sequence, ":")
		if len(r) == 2 {
			start, err1 := strconv.Atoi(r[0])
			end, err2 := strconv.Atoi(r[1])
			if err1 != nil || err2 != nil || start > end {
				s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid UID range", tag))
				return
			}
			query := fmt.Sprintf("SELECT id, raw_message, flags, ROW_NUMBER() OVER (ORDER BY id ASC) as seq FROM %s WHERE folder = ? AND id >= ? AND id <= ? ORDER BY id ASC", tableName)
			rows, err = s.db.Query(query, state.SelectedFolder, start, end)
		} else {
			s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid UID range format", tag))
			return
		}
	} else {
		uid, parseErr := strconv.Atoi(sequence)
		if parseErr != nil {
			s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid UID", tag))
			return
		}
		query := fmt.Sprintf("SELECT id, raw_message, flags, ROW_NUMBER() OVER (ORDER BY id ASC) as seq FROM %s WHERE folder = ? AND id = ?", tableName)
		rows, err = s.db.Query(query, state.SelectedFolder, uid)
	}

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

		if strings.Contains(itemsUpper, "BODY.PEEK[HEADER.FIELDS") {
			start := strings.Index(itemsUpper, "BODY.PEEK[HEADER.FIELDS")
			end := strings.Index(itemsUpper[start:], "]")
			headers := []string{"FROM", "TO", "CC", "BCC", "SUBJECT", "DATE", "MESSAGE-ID", "PRIORITY", "X-PRIORITY", "REFERENCES", "NEWSGROUPS", "IN-REPLY-TO", "CONTENT-TYPE", "REPLY-TO"}
			if start != -1 && end != -1 {
				fieldsStr := items[start+len("BODY.PEEK[HEADER.FIELDS (") : start+end]
				fields := strings.FieldsFunc(fieldsStr, func(r rune) bool { return r == ' ' || r == ',' })
				if len(fields) > 0 {
					headers = []string{}
					for _, f := range fields {
						headers = append(headers, strings.ToUpper(strings.TrimSpace(f)))
					}
				}
			}
			headersMap := map[string]string{}
			lines := strings.Split(rawMsg, "\r\n")
			for _, line := range lines {
				for _, h := range headers {
					if strings.HasPrefix(strings.ToUpper(line), h+":") {
						headersMap[h] = line
					}
				}
			}
			var headerLines []string
			for _, h := range headers {
				if val, ok := headersMap[h]; ok {
					headerLines = append(headerLines, val)
				}
			}
			headersStr := strings.Join(headerLines, "\r\n") + "\r\n\r\n"
			responseParts = append(responseParts, fmt.Sprintf("BODY[HEADER] {%d}", len(headersStr)))
			s.sendResponse(conn, fmt.Sprintf("* %d FETCH (%s)", seqNum, strings.Join(responseParts, " ")))
			conn.Write([]byte(headersStr))
			s.sendResponse(conn, ")")
			continue
		}

		if strings.Contains(itemsUpper, "BODY[]") || strings.Contains(itemsUpper, "BODY.PEEK[]") || strings.Contains(itemsUpper, "RFC822") {
			responseParts = append(responseParts, fmt.Sprintf("BODY[] {%d}", len(rawMsg)))
			s.sendResponse(conn, fmt.Sprintf("* %d FETCH (%s)", seqNum, strings.Join(responseParts, " ")))
			conn.Write([]byte(rawMsg + "\r\n"))
			s.sendResponse(conn, ")")
		} else {
			s.sendResponse(conn, fmt.Sprintf("* %d FETCH (%s)", seqNum, strings.Join(responseParts, " ")))
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

	var err error
	if sequence == "1:*" {
		query := fmt.Sprintf("UPDATE %s SET flags = CASE WHEN flags LIKE '%%\\Seen%%' THEN flags ELSE flags || ' \\Seen' END WHERE folder = ?", tableName)
		_, err = s.db.Exec(query, state.SelectedFolder)
	} else if strings.Contains(sequence, ":") {
		r := strings.Split(sequence, ":")
		if len(r) == 2 {
			start, err1 := strconv.Atoi(r[0])
			end, err2 := strconv.Atoi(r[1])
			if err1 != nil || err2 != nil || start > end {
				s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid UID range", tag))
				return
			}
			query := fmt.Sprintf("UPDATE %s SET flags = CASE WHEN flags LIKE '%%\\Seen%%' THEN flags ELSE flags || ' \\Seen' END WHERE folder = ? AND id >= ? AND id <= ?", tableName)
			_, err = s.db.Exec(query, state.SelectedFolder, start, end)
		} else {
			s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid UID range format", tag))
			return
		}
	} else {
		uid, parseErr := strconv.Atoi(sequence)
		if parseErr != nil {
			s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid UID", tag))
			return
		}
		query := fmt.Sprintf("UPDATE %s SET flags = CASE WHEN flags LIKE '%%\\Seen%%' THEN flags ELSE flags || ' \\Seen' END WHERE folder = ? AND id = ?", tableName)
		_, err = s.db.Exec(query, state.SelectedFolder, uid)
	}

	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
		return
	}

	s.sendResponse(conn, fmt.Sprintf("%s OK STORE completed", tag))
}
