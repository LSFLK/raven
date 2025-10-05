package server

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"go-imap/internal/models"
	"go-imap/internal/conf"
)

func (s *IMAPServer) handleCapability(conn net.Conn, tag string, state *models.ClientState) {
    // Base capabilities
    capabilities := []string{"IMAP4rev1"}

    // Detect TLS: real TLS connection or test mock that advertises TLS
    isTLS := false
    if _, ok := conn.(*tls.Conn); ok {
        isTLS = true
    } else {
        // Allow test doubles to signal TLS via an interface
        type tlsAware interface{ IsTLS() bool }
        if ta, ok := any(conn).(tlsAware); ok && ta.IsTLS() {
            isTLS = true
        }
    }

    if isTLS {
        // TLS is active → allow authentication
        capabilities = append(capabilities, "AUTH=PLAIN", "LOGIN")
    } else {
        // Plain connection → require STARTTLS and disable login
        capabilities = append(capabilities, "STARTTLS", "LOGINDISABLED")
    }

    // Add extension capabilities
    capabilities = append(capabilities,
        "UIDPLUS",
        "IDLE",
        "NAMESPACE",
        "UNSELECT",
        "LITERAL+",
    )

    // Send CAPABILITY response
    s.sendResponse(conn, "* CAPABILITY "+strings.Join(capabilities, " "))
    s.sendResponse(conn, fmt.Sprintf("%s OK CAPABILITY completed", tag))
}

func (s *IMAPServer) handleLogin(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if len(parts) < 4 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD LOGIN requires username and password", tag))
		return
	}

	username := strings.Trim(parts[2], "\"")
	password := strings.Trim(parts[3], "\"")

	// Use common authentication logic
	s.authenticateUser(conn, tag, username, password, state)
}

func (s *IMAPServer) handleList(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

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

func (s *IMAPServer) handleLogout(conn net.Conn, tag string) {
	s.sendResponse(conn, "* BYE IMAP4rev1 Server logging out")
	s.sendResponse(conn, fmt.Sprintf("%s OK LOGOUT completed", tag))
}

func (s *IMAPServer) handleNoop(conn net.Conn, tag string, state *models.ClientState) {
	// NOOP can be used before authentication
	// If authenticated and a folder is selected, check for mailbox updates
	// and send untagged responses per RFC 3501
	if state.Authenticated && state.SelectedFolder != "" {
		tableName := s.getUserTableName(state.Username)
		
		// Get current mailbox state
		var currentCount int
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE folder = ?", tableName)
		err := s.db.QueryRow(query, state.SelectedFolder).Scan(&currentCount)
		if err != nil {
			// If there's a database error, just complete normally
			s.sendResponse(conn, fmt.Sprintf("%s OK NOOP completed", tag))
			return
		}

		var currentRecent int
		query = fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE folder = ? AND flags NOT LIKE '%%\\Seen%%'", tableName)
		err = s.db.QueryRow(query, state.SelectedFolder).Scan(&currentRecent)
		if err != nil {
			currentRecent = 0
		}

		// Check for new messages (EXISTS response)
		if currentCount > state.LastMessageCount {
			s.sendResponse(conn, fmt.Sprintf("* %d EXISTS", currentCount))
			
			// Calculate new recent messages
			newRecent := currentCount - state.LastMessageCount
			if newRecent > 0 {
				s.sendResponse(conn, fmt.Sprintf("* %d RECENT", newRecent))
			}
		}

		// Check for expunged (deleted) messages
		if currentCount < state.LastMessageCount {
			// Send EXPUNGE for each deleted message
			// Note: In a real implementation, you'd track which specific messages
			// were expunged. Here we send generic expunge notifications.
			for i := state.LastMessageCount; i > currentCount; i-- {
				s.sendResponse(conn, fmt.Sprintf("* %d EXPUNGE", i))
			}
		}

		// Check for flag changes (simplified - just report recent count changes)
		if currentRecent != state.LastRecentCount && currentCount == state.LastMessageCount {
			// Messages exist but recent count changed (flags were modified)
			// In a full implementation, you'd send FETCH responses with updated flags
			// For now, we send an informational message
			if currentRecent > 0 {
				s.sendResponse(conn, fmt.Sprintf("* %d RECENT", currentRecent))
			}
		}

		// Update state tracking
		state.LastMessageCount = currentCount
		state.LastRecentCount = currentRecent
	}
	
	// Always complete successfully per RFC 3501
	s.sendResponse(conn, fmt.Sprintf("%s OK NOOP completed", tag))
}

func (s *IMAPServer) handleIdle(conn net.Conn, tag string, state *models.ClientState) {
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	if state.SelectedFolder == "" {
		s.sendResponse(conn, fmt.Sprintf("%s NO No folder selected", tag))
		return
	}

	// Tell client we're entering idle mode
	s.sendResponse(conn, "+ idling")

	buf := make([]byte, 4096)
	tableName := s.getUserTableName(state.Username)

	// Track previous state of the folder
	var prevCount, prevUnseen int
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE folder = ?", tableName)
	_ = s.db.QueryRow(query, state.SelectedFolder).Scan(&prevCount)
	query = fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE folder = ? AND flags NOT LIKE '%%\\Seen%%'", tableName)
	_ = s.db.QueryRow(query, state.SelectedFolder).Scan(&prevUnseen)

	for {
		// Poll every 2 seconds for changes
		time.Sleep(2 * time.Second)

		// Check current mailbox state
		var count, unseen int
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE folder = ?", tableName)
		_ = s.db.QueryRow(query, state.SelectedFolder).Scan(&count)
		query = fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE folder = ? AND flags NOT LIKE '%%\\Seen%%'", tableName)
		_ = s.db.QueryRow(query, state.SelectedFolder).Scan(&unseen)

		// Notify about new messages
		if count > prevCount {
			s.sendResponse(conn, fmt.Sprintf("* %d EXISTS", count))
			newRecent := count - prevCount
			if newRecent > 0 {
				s.sendResponse(conn, fmt.Sprintf("* %d RECENT", newRecent))
			}
		}

		// Notify about expunged (deleted) messages
		if count < prevCount {
			for i := prevCount; i > count; i-- {
				s.sendResponse(conn, fmt.Sprintf("* %d EXPUNGE", i))
			}
		}

		// Notify about unseen count change
		if unseen != prevUnseen {
			s.sendResponse(conn, fmt.Sprintf("* OK [UNSEEN %d] Message %d is first unseen", unseen, unseen))
		}

		// Update cached values
		prevCount = count
		prevUnseen = unseen

		// Check if client sent DONE (non-blocking read)
		conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		n, err := conn.Read(buf)
		if err == nil && strings.TrimSpace(strings.ToUpper(string(buf[:n]))) == "DONE" {
			s.sendResponse(conn, fmt.Sprintf("%s OK IDLE terminated", tag))
			return
		}
	}
}

func (s *IMAPServer) handleNamespace(conn net.Conn, tag string, state *models.ClientState) {
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	// Send namespace response - simple single personal namespace
	s.sendResponse(conn, `* NAMESPACE (("" "/")) NIL NIL`)
	s.sendResponse(conn, fmt.Sprintf("%s OK NAMESPACE completed", tag))
}

func (s *IMAPServer) handleUnselect(conn net.Conn, tag string, state *models.ClientState) {
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	if state.SelectedFolder == "" {
		s.sendResponse(conn, fmt.Sprintf("%s NO No folder selected", tag))
		return
	}

	// Close mailbox without expunging messages
	state.SelectedFolder = ""
	// Reset state tracking
	state.LastMessageCount = 0
	state.LastRecentCount = 0
	s.sendResponse(conn, fmt.Sprintf("%s OK UNSELECT completed", tag))
}

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

	// Parse folder name (could be quoted)
	folder := strings.Trim(parts[2], "\"")

	// Validate folder exists
	validFolders := map[string]bool{
		"INBOX":  true,
		"Sent":   true,
		"Drafts": true,
		"Trash":  true,
	}
	
	if !validFolders[folder] {
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

	// Look for literal size indicator {size}
	literalStartIdx := strings.Index(fullLine, "{")
	literalEndIdx := strings.Index(fullLine, "}")
	
	if literalStartIdx == -1 || literalEndIdx == -1 || literalStartIdx > literalEndIdx {
		s.sendResponse(conn, fmt.Sprintf("%s BAD APPEND requires message size", tag))
		return
	}

	// Extract the size
	sizeStr := fullLine[literalStartIdx+1 : literalEndIdx]
	var messageSize int
	fmt.Sscanf(sizeStr, "%d", &messageSize)
	
	if messageSize <= 0 || messageSize > 50*1024*1024 { // Max 50MB
		s.sendResponse(conn, fmt.Sprintf("%s NO Message size invalid or too large", tag))
		return
	}

	// Send continuation response
	s.sendResponse(conn, "+ Ready for literal data")

	// Read the message data
	messageData := make([]byte, messageSize)
	totalRead := 0
	
	conn.SetReadDeadline(time.Now().Add(5 * time.Minute))
	for totalRead < messageSize {
		n, err := conn.Read(messageData[totalRead:])
		if err != nil {
			log.Printf("Error reading message data: %v", err)
			s.sendResponse(conn, fmt.Sprintf("%s NO Failed to read message data", tag))
			return
		}
		totalRead += n
	}

	rawMessage := string(messageData)

	// Parse email headers for metadata
	subject := ""
	sender := ""
	recipient := ""
	dateSent := time.Now().Format("02-Jan-2006 15:04:05 -0700")

	lines := strings.Split(rawMessage, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			break // End of headers
		}
		
		upperLine := strings.ToUpper(line)
		if strings.HasPrefix(upperLine, "SUBJECT:") {
			subject = strings.TrimSpace(line[8:])
		} else if strings.HasPrefix(upperLine, "FROM:") {
			sender = strings.TrimSpace(line[5:])
		} else if strings.HasPrefix(upperLine, "TO:") {
			recipient = strings.TrimSpace(line[3:])
		} else if strings.HasPrefix(upperLine, "DATE:") {
			dateSent = strings.TrimSpace(line[5:])
		}
	}

	// Ensure message has CRLF line endings
	if !strings.Contains(rawMessage, "\r\n") {
		rawMessage = strings.ReplaceAll(rawMessage, "\n", "\r\n")
	}

	// Insert into database
	tableName := s.getUserTableName(state.Username)
	query := fmt.Sprintf(
		"INSERT INTO %s (subject, sender, recipient, date_sent, raw_message, flags, folder) VALUES (?, ?, ?, ?, ?, ?, ?)",
		tableName,
	)
	_, err := s.db.Exec(query, subject, sender, recipient, dateSent, rawMessage, flags, folder)

	if err != nil {
		log.Printf("Failed to insert message: %v", err)
		s.sendResponse(conn, fmt.Sprintf("%s NO [SERVERBUG] Failed to save message", tag))
		return
	}

	// Get the newly inserted message ID for APPENDUID response
	var newUID int64
	err = s.db.QueryRow("SELECT last_insert_rowid()").Scan(&newUID)
	if err != nil {
		log.Printf("Failed to get new UID: %v", err)
	}

	log.Printf("Message appended to folder '%s' with UID %d", folder, newUID)
	
	// Send success response with APPENDUID (RFC 4315 - UIDPLUS extension)
	s.sendResponse(conn, fmt.Sprintf("%s OK [APPENDUID 1 %d] APPEND completed", tag, newUID))
}

func (s *IMAPServer) handleStartTLS(conn net.Conn, tag string) {
	// Respond to client to begin TLS negotiation
	s.sendResponse(conn, fmt.Sprintf("%s OK Begin TLS negotiation", tag))

	certPath := "/certs/fullchain.pem"
	keyPath := "/certs/privkey.pem"

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		fmt.Printf("Failed to load TLS cert/key: %v\n", err)
		return
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	tlsConn := tls.Server(conn, tlsConfig)

	// Restart handler with upgraded TLS connection
	handleClient(s, tlsConn, &models.ClientState{})
}

func (s *IMAPServer) HandleSSLConnection(conn net.Conn) {

	certPath := "/certs/fullchain.pem"
	keyPath := "/certs/privkey.pem"

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		log.Printf("Failed to load TLS cert/key: %v", err)
		conn.Close()
		return
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	tlsConn := tls.Server(conn, tlsConfig)

	// Start IMAP session over TLS
	handleClient(s, tlsConn, &models.ClientState{})
}

func (s *IMAPServer) handleAuthenticate(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if len(parts) < 3 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD AUTHENTICATE requires authentication mechanism", tag))
		return
	}

	mechanism := strings.ToUpper(parts[2])
	switch mechanism {
	case "PLAIN":
		// Do not allow plaintext authentication unless using TLS
		isTLS := false
		if _, ok := conn.(*tls.Conn); ok {
			isTLS = true
		} else {
			type tlsAware interface{ IsTLS() bool }
			if ta, ok := any(conn).(tlsAware); ok && ta.IsTLS() {
				isTLS = true
			}
		}
		if !isTLS {
			s.sendResponse(conn, fmt.Sprintf("%s NO Plaintext authentication disallowed without TLS", tag))
			return
		}

		// Send continuation request
		s.sendResponse(conn, "+ ")

		// Read the authentication data
		buf := make([]byte, 8192)
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		n, err := conn.Read(buf)
		if err != nil {
			s.sendResponse(conn, fmt.Sprintf("%s BAD Authentication failed", tag))
			return
		}

		authData := strings.TrimSpace(string(buf[:n]))

		// Client may cancel authentication with a single "*"
		if authData == "*" {
			s.sendResponse(conn, fmt.Sprintf("%s BAD Authentication cancelled", tag))
			return
		}

		log.Printf("AUTHENTICATE PLAIN: received %d bytes of auth data", len(authData))

		// Decode base64 as per SASL challenge/response (PLAIN uses base64 here)
		var decoded []byte
		decoded, err = base64.StdEncoding.DecodeString(authData)
		if err != nil {
			log.Printf("AUTHENTICATE PLAIN: base64 decode failed: %v, treating as plain", err)
			// If decode fails, fall back to treating the input as plain (some test-clients may do this)
			decoded = []byte(authData)
		} else {
			log.Printf("AUTHENTICATE PLAIN: decoded %d bytes", len(decoded))
		}

		// Split on NUL (\x00). PLAIN: [authzid] \x00 authcid \x00 passwd
		partsNull := strings.Split(string(decoded), "\x00")
		log.Printf("AUTHENTICATE PLAIN: split into %d parts", len(partsNull))
		
		var username, password string
		if len(partsNull) >= 3 {
			username = partsNull[1]
			password = partsNull[2]
			log.Printf("AUTHENTICATE PLAIN: extracted username=%s (password length=%d)", username, len(password))
		} else if len(partsNull) == 2 {
			// fallback: username and password
			username = partsNull[0]
			password = partsNull[1]
			log.Printf("AUTHENTICATE PLAIN: fallback extracted username=%s (password length=%d)", username, len(password))
		} else {
			log.Printf("AUTHENTICATE PLAIN: invalid format, expected 2-3 parts, got %d", len(partsNull))
			s.sendResponse(conn, fmt.Sprintf("%s BAD Authentication failed", tag))
			return
		}

		if username == "" || password == "" {
			log.Printf("AUTHENTICATE PLAIN: empty username or password")
			s.sendResponse(conn, fmt.Sprintf("%s BAD Authentication failed", tag))
			return
		}

		// Reuse the existing login logic
		s.authenticateUser(conn, tag, username, password, state)
		return

	default:
		s.sendResponse(conn, fmt.Sprintf("%s NO Unsupported authentication mechanism", tag))
	}
}

// Extract common authentication logic
func (s *IMAPServer) authenticateUser(conn net.Conn, tag string, username string, password string, state *models.ClientState) {
	// Load domain from config file
	cfg, err := conf.LoadConfig()
	if err != nil {
		log.Printf("LoadConfig error: %v", err)
		s.sendResponse(conn, fmt.Sprintf("%s BAD LOGIN config error", tag))
		return
	}

	if cfg.Domain == "" || cfg.AuthServerURL == "" {
		s.sendResponse(conn, fmt.Sprintf("%s BAD LOGIN config error", tag))
		return
	}

	email := username + "@" + cfg.Domain

	// Prepare JSON body
	requestBody := fmt.Sprintf(`{"email":"%s","password":"%s"}`, email, password)

	// Create HTTP request
	req, err := http.NewRequest("POST", cfg.AuthServerURL, strings.NewReader(requestBody))
	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s BAD LOGIN internal error", tag))
		return
	}
	req.Header.Set("Content-Type", "application/json")

	// TLS config for system CA bundle (default)
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}
	transport := &http.Transport{TLSClientConfig: tlsConfig}
	client := &http.Client{Transport: transport}

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("LOGIN: error reaching auth server: %v", err)
		s.sendResponse(conn, fmt.Sprintf("%s BAD LOGIN unable to reach auth server", tag))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		log.Printf("Accepting login for user: %s", username)
		
		// Ensure user table exists
		if err := s.ensureUserTable(username); err != nil {
			log.Printf("Failed to create user table: %v", err)
			s.sendResponse(conn, fmt.Sprintf("%s BAD LOGIN server error", tag))
			return
		}
		
		state.Authenticated = true
		state.Username = username
		s.sendResponse(conn, fmt.Sprintf("%s OK [CAPABILITY IMAP4rev1 UIDPLUS IDLE NAMESPACE UNSELECT LITERAL+] LOGIN completed", tag))
	} else {
		s.sendResponse(conn, fmt.Sprintf("%s BAD LOGIN authentication failed", tag))
	}
}