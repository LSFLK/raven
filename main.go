package main

import (
	"database/sql"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const (
	DB_FILE   = "mails.db"
	SERVER_IP = "0.0.0.0:143"
)

type IMAPServer struct {
	db *sql.DB
}

type ClientState struct {
	authenticated  bool
	selectedFolder string
	conn           net.Conn
	username       string
}

// ============================
// Database initialization
// ============================

func (s *IMAPServer) initDB() error {
	var err error
	s.db, err = sql.Open("sqlite3", DB_FILE)
	if err != nil {
		return err
	}

	schema := `
	CREATE TABLE IF NOT EXISTS mails (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		subject TEXT,
		sender TEXT,
		recipient TEXT,
		date_sent TEXT,
		raw_message TEXT,
		flags TEXT DEFAULT '',
		folder TEXT DEFAULT 'INBOX'
	);

	CREATE TABLE IF NOT EXISTS folders (
		name TEXT PRIMARY KEY,
		delimiter TEXT DEFAULT '/',
		attributes TEXT DEFAULT ''
	);

	INSERT OR IGNORE INTO folders (name) VALUES
		('INBOX'),
		('Sent'),
		('Drafts'),
		('Trash');
	`
	if _, err = s.db.Exec(schema); err != nil {
		return err
	}

	// Insert test mails if DB is empty
	var count int
	err = s.db.QueryRow("SELECT COUNT(*) FROM mails").Scan(&count)
	if err != nil {
		return err
	}

	if count == 0 {
		testMails := []struct {
			subject, sender, recipient, body string
		}{
			{
				"Welcome to SQLite IMAP",
				"admin@example.com",
				"user@example.com",
				"From: admin@example.com\r\n" +
					"To: user@example.com\r\n" +
					"Subject: Welcome to SQLite IMAP\r\n" +
					"Date: " + time.Now().Format(time.RFC1123Z) + "\r\n" +
					"\r\n" +
					"Hello user@example.com,\r\nThis is your first test mail!\r\n",
			},
			{
				"Meeting Reminder",
				"boss@example.com",
				"user@example.com",
				"From: boss@example.com\r\n" +
					"To: user@example.com\r\n" +
					"Subject: Meeting Reminder\r\n" +
					"Date: " + time.Now().Add(-2*time.Hour).Format(time.RFC1123Z) + "\r\n" +
					"\r\n" +
					"Don’t forget our meeting at 3PM today.\r\n",
			},
		}

		for _, m := range testMails {
			_, err = s.db.Exec(
				"INSERT INTO mails (subject, sender, recipient, date_sent, raw_message, folder) VALUES (?, ?, ?, ?, ?, ?)",
				m.subject, m.sender, m.recipient, time.Now().Format(time.RFC3339), m.body, "INBOX",
			)
			if err != nil {
				return err
			}
		}
		log.Println("Inserted sample mails for user@example.com")
	}

	return nil
}

// ============================
// IMAP handling
// ============================

func (s *IMAPServer) handleConnection(conn net.Conn) {
	defer conn.Close()

	state := &ClientState{
		authenticated: false,
		conn:          conn,
	}

	// Greeting
	s.sendResponse(conn, "* OK [CAPABILITY IMAP4rev1 UIDPLUS IDLE] SQLite IMAP server ready")

	buf := make([]byte, 4096)
	for {
		conn.SetReadDeadline(time.Now().Add(30 * time.Minute))
		n, err := conn.Read(buf)
		if err != nil {
			return
		}

		line := strings.TrimSpace(string(buf[:n]))
		if line == "" {
			continue
		}

		fmt.Printf("Client: %s\n", line)
		parts := strings.Fields(line)
		if len(parts) < 2 {
			s.sendResponse(conn, "* BAD Invalid command format")
			continue
		}

		tag := parts[0]
		cmd := strings.ToUpper(parts[1])

		switch cmd {
		case "CAPABILITY":
			s.handleCapability(conn, tag)
		case "LOGIN":
			s.handleLogin(conn, tag, parts, state)
		case "LIST":
			s.handleList(conn, tag, parts, state)
		case "SELECT", "EXAMINE":
			s.handleSelect(conn, tag, parts, state)
		case "FETCH":
			s.handleFetch(conn, tag, parts, state)
		case "SEARCH":
			s.handleSearch(conn, tag, parts, state)
		case "STATUS":
			s.handleStatus(conn, tag, parts, state)
		case "UID":
			s.handleUID(conn, tag, parts, state)
		case "IDLE":
			s.handleIdle(conn, tag, state)
		case "NOOP":
			s.sendResponse(conn, fmt.Sprintf("%s OK NOOP completed", tag))
		case "LOGOUT":
			s.handleLogout(conn, tag)
			return
		default:
			s.sendResponse(conn, fmt.Sprintf("%s BAD Unknown command: %s", tag, cmd))
		}
	}
}

func (s *IMAPServer) sendResponse(conn net.Conn, response string) {
	fmt.Printf("Server: %s\n", response)
	conn.Write([]byte(response + "\r\n"))
}

func (s *IMAPServer) handleCapability(conn net.Conn, tag string) {
	s.sendResponse(conn, "* CAPABILITY IMAP4rev1 LOGIN IDLE")
	s.sendResponse(conn, fmt.Sprintf("%s OK CAPABILITY completed", tag))
}

// Accept any username/password combination
func (s *IMAPServer) handleLogin(conn net.Conn, tag string, parts []string, state *ClientState) {
	if len(parts) < 4 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD LOGIN requires username and password", tag))
		return
	}

	username := strings.Trim(parts[2], "\"")

	log.Printf("Accepting login for user: %s", username)
	state.authenticated = true
	state.username = username
	s.sendResponse(conn, fmt.Sprintf("%s OK LOGIN completed", tag))
}

func (s *IMAPServer) handleList(conn net.Conn, tag string, parts []string, state *ClientState) {
	if !state.authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	// List folders
	folders := []struct{ name, attrs string }{
		{"INBOX", ""},
		{"Sent", ""},
		{"Drafts", "\\Drafts"},
		{"Trash", "\\Trash"},
	}

	for _, folder := range folders {
		attrs := folder.attrs
		if attrs == "" {
			attrs = "\\Unmarked"
		}
		s.sendResponse(conn, fmt.Sprintf("* LIST (%s) \"/\" \"%s\"", attrs, folder.name))
	}
	s.sendResponse(conn, fmt.Sprintf("%s OK LIST completed", tag))
}

func (s *IMAPServer) handleSelect(conn net.Conn, tag string, parts []string, state *ClientState) {
	if !state.authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	if len(parts) < 3 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD SELECT requires folder name", tag))
		return
	}

	folder := strings.Trim(parts[2], "\"")
	state.selectedFolder = folder

	// Get message count
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM mails WHERE folder = ?", folder).Scan(&count)
	if err != nil {
		count = 0
	}

	// Get recent count (messages without \Seen flag)
	var recent int
	err = s.db.QueryRow("SELECT COUNT(*) FROM mails WHERE folder = ? AND flags NOT LIKE '%\\Seen%'", folder).Scan(&recent)
	if err != nil {
		recent = 0
	}

	// Send required untagged responses
	s.sendResponse(conn, fmt.Sprintf("* %d EXISTS", count))
	s.sendResponse(conn, fmt.Sprintf("* %d RECENT", recent))
	s.sendResponse(conn, "* OK [UIDVALIDITY 1] UID validity status")
	s.sendResponse(conn, fmt.Sprintf("* OK [UIDNEXT %d] Predicted next UID", count+1))
	s.sendResponse(conn, "* FLAGS (\\Answered \\Flagged \\Deleted \\Seen \\Draft)")
	s.sendResponse(conn, "* OK [PERMANENTFLAGS (\\Answered \\Flagged \\Deleted \\Seen \\Draft \\*)] Flags permitted")

	cmd := strings.ToUpper(parts[1])
	if cmd == "SELECT" {
		s.sendResponse(conn, fmt.Sprintf("%s OK [READ-WRITE] SELECT completed", tag))
	} else {
		s.sendResponse(conn, fmt.Sprintf("%s OK [READ-ONLY] EXAMINE completed", tag))
	}
}

func (s *IMAPServer) handleUID(conn net.Conn, tag string, parts []string, state *ClientState) {
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

func (s *IMAPServer) handleFetch(conn net.Conn, tag string, parts []string, state *ClientState) {
	if !state.authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	if state.selectedFolder == "" {
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

	// Parse sequence (simplified - handle 1:* and individual numbers)
	var rows *sql.Rows
	var err error

	if sequence == "1:*" {
		rows, err = s.db.Query("SELECT id, raw_message, flags FROM mails WHERE folder = ? ORDER BY id ASC", state.selectedFolder)
	} else {
		// Handle individual message numbers
		msgNum, parseErr := strconv.Atoi(sequence)
		if parseErr != nil {
			s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid sequence number", tag))
			return
		}
		rows, err = s.db.Query("SELECT id, raw_message, flags FROM mails WHERE folder = ? ORDER BY id ASC LIMIT 1 OFFSET ?", state.selectedFolder, msgNum-1)
	}

	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
		return
	}
	defer rows.Close()

	seqNum := 1
	for rows.Next() {
		var id int
		var rawMsg, flags string
		rows.Scan(&id, &rawMsg, &flags)

		// Ensure proper CRLF line endings
		if !strings.Contains(rawMsg, "\r\n") {
			rawMsg = strings.ReplaceAll(rawMsg, "\n", "\r\n")
		}

		// Handle different FETCH items
		itemsUpper := strings.ToUpper(items)
		if strings.Contains(itemsUpper, "BODY[]") || strings.Contains(itemsUpper, "RFC822") {
			// Return full message
			s.sendResponse(conn, fmt.Sprintf("* %d FETCH (BODY[] {%d}", seqNum, len(rawMsg)))
			conn.Write([]byte(rawMsg + "\r\n"))
			s.sendResponse(conn, ")")
		} else if strings.Contains(itemsUpper, "FLAGS") {
			// Return flags
			if flags == "" {
				flags = "()"
			} else {
				flags = fmt.Sprintf("(%s)", flags)
			}
			s.sendResponse(conn, fmt.Sprintf("* %d FETCH (FLAGS %s)", seqNum, flags))
		} else {
			// Default response
			s.sendResponse(conn, fmt.Sprintf("* %d FETCH (FLAGS ())", seqNum))
		}
		seqNum++
	}

	s.sendResponse(conn, fmt.Sprintf("%s OK FETCH completed", tag))
}

func (s *IMAPServer) handleUIDFetch(conn net.Conn, tag string, parts []string, state *ClientState) {
	if !state.authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	if state.selectedFolder == "" {
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

	// Parse UID sequence
	var rows *sql.Rows
	var err error

	if sequence == "1:*" {
		rows, err = s.db.Query("SELECT id, raw_message, flags, ROW_NUMBER() OVER (ORDER BY id ASC) as seq FROM mails WHERE folder = ? ORDER BY id ASC", state.selectedFolder)
	} else if strings.Contains(sequence, ":") {
		// Handle UID range, e.g., 1:2
		parts := strings.Split(sequence, ":")
		if len(parts) == 2 {
			start, err1 := strconv.Atoi(parts[0])
			end, err2 := strconv.Atoi(parts[1])
			if err1 != nil || err2 != nil || start > end {
				s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid UID range", tag))
				return
			}
			rows, err = s.db.Query("SELECT id, raw_message, flags, ROW_NUMBER() OVER (ORDER BY id ASC) as seq FROM mails WHERE folder = ? AND id >= ? AND id <= ? ORDER BY id ASC", state.selectedFolder, start, end)
		} else {
			s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid UID range format", tag))
			return
		}
	} else {
		// Handle individual UID numbers
		uid, parseErr := strconv.Atoi(sequence)
		if parseErr != nil {
			s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid UID", tag))
			return
		}
		rows, err = s.db.Query("SELECT id, raw_message, flags, ROW_NUMBER() OVER (ORDER BY id ASC) as seq FROM mails WHERE folder = ? AND id = ?", state.selectedFolder, uid)
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

		// Ensure proper CRLF
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
			// Extract requested header fields
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
			// Parse rawMsg headers
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

		if strings.Contains(itemsUpper, "BODY[]") || strings.Contains(itemsUpper, "RFC822") {
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

func (s *IMAPServer) handleUIDSearch(conn net.Conn, tag string, parts []string, state *ClientState) {
	if !state.authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	if state.selectedFolder == "" {
		s.sendResponse(conn, fmt.Sprintf("%s NO No folder selected", tag))
		return
	}

	// Simple UID search - return all UIDs
	rows, err := s.db.Query("SELECT id FROM mails WHERE folder = ? ORDER BY id ASC", state.selectedFolder)
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

func (s *IMAPServer) handleSearch(conn net.Conn, tag string, parts []string, state *ClientState) {
	if !state.authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	if state.selectedFolder == "" {
		s.sendResponse(conn, fmt.Sprintf("%s NO No folder selected", tag))
		return
	}

	// Simple search - return all message numbers
	rows, err := s.db.Query("SELECT ROW_NUMBER() OVER (ORDER BY id ASC) as seq FROM mails WHERE folder = ?", state.selectedFolder)
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

func (s *IMAPServer) handleStatus(conn net.Conn, tag string, parts []string, state *ClientState) {
	if !state.authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	if len(parts) < 4 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD STATUS requires folder and items", tag))
		return
	}

	folder := strings.Trim(parts[2], "\"")

	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM mails WHERE folder = ?", folder).Scan(&count)

	s.sendResponse(conn, fmt.Sprintf("* STATUS \"%s\" (MESSAGES %d RECENT 0 UIDNEXT %d UIDVALIDITY 1 UNSEEN 0)", folder, count, count+1))
	s.sendResponse(conn, fmt.Sprintf("%s OK STATUS completed", tag))
}

func (s *IMAPServer) handleLogout(conn net.Conn, tag string) {
	s.sendResponse(conn, "* BYE SQLite IMAP server logging out")
	s.sendResponse(conn, fmt.Sprintf("%s OK LOGOUT completed", tag))
}

func (s *IMAPServer) handleIdle(conn net.Conn, tag string, state *ClientState) {
	if !state.authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	// Simple IDLE implementation - just respond that we're ready and wait for DONE
	s.sendResponse(conn, "+ idling")

	// In a real implementation, we would wait for "DONE" command
	// For simplicity, we'll just immediately return OK
	s.sendResponse(conn, fmt.Sprintf("%s OK IDLE completed", tag))
}

func (s *IMAPServer) handleUIDStore(conn net.Conn, tag string, parts []string, state *ClientState) {
	if !state.authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}
	if state.selectedFolder == "" {
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

	// Only support adding \Seen for now
	if !strings.Contains(flagsStr, "\\Seen") {
		s.sendResponse(conn, fmt.Sprintf("%s BAD Only \\Seen flag supported", tag))
		return
	}

	var err error
	if sequence == "1:*" {
		_, err = s.db.Exec("UPDATE mails SET flags = CASE WHEN flags LIKE '%\\Seen%' THEN flags ELSE flags || ' \\Seen' END WHERE folder = ?", state.selectedFolder)
	} else if strings.Contains(sequence, ":") {
		parts := strings.Split(sequence, ":")
		if len(parts) == 2 {
			start, err1 := strconv.Atoi(parts[0])
			end, err2 := strconv.Atoi(parts[1])
			if err1 != nil || err2 != nil || start > end {
				s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid UID range", tag))
				return
			}
			_, err = s.db.Exec("UPDATE mails SET flags = CASE WHEN flags LIKE '%\\Seen%' THEN flags ELSE flags || ' \\Seen' END WHERE folder = ? AND id >= ? AND id <= ?", state.selectedFolder, start, end)
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
		_, err = s.db.Exec("UPDATE mails SET flags = CASE WHEN flags LIKE '%\\Seen%' THEN flags ELSE flags || ' \\Seen' END WHERE folder = ? AND id = ?", state.selectedFolder, uid)
	}

	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
		return
	}

	s.sendResponse(conn, fmt.Sprintf("%s OK STORE completed", tag))
}

func main() {
	log.Println("Starting SQLite IMAP server (no-auth mode)...")

	server := &IMAPServer{}

	if err := server.initDB(); err != nil {
		log.Fatal("Failed to initialize database:", err)
	}

	ln, err := net.Listen("tcp", SERVER_IP)
	if err != nil {
		log.Fatal("Failed to start TCP listener:", err)
	}
	defer ln.Close()

	log.Printf("SQLite IMAP server running on %s", SERVER_IP)
	log.Println("Configure your email client with:")
	log.Println("  Server: localhost (or container IP)")
	log.Println("  Port: 143")
	log.Println("  Security: None")
	log.Println("  Username: anything")
	log.Println("  Password: anything")

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println("Accept error:", err)
			continue
		}

		log.Printf("New connection from: %s", conn.RemoteAddr())
		go server.handleConnection(conn)
	}
}
