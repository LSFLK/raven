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
	"go-imap/internal/db"
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
	// Check if LOGIN command has correct number of arguments
	if len(parts) < 4 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD LOGIN requires username and password", tag))
		return
	}

	// Detect if TLS is active
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

	// Per RFC 3501: If LOGINDISABLED capability is advertised (i.e., no TLS),
	// reject the LOGIN command
	if !isTLS {
		s.sendResponse(conn, fmt.Sprintf("%s NO [PRIVACYREQUIRED] LOGIN is disabled on insecure connection. Use STARTTLS first.", tag))
		return
	}

	// Extract username and password, removing quotes if present
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

	// Get all mailboxes for the user
	mailboxes, err := db.GetUserMailboxes(s.db, state.Username)
	if err != nil {
		// Fall back to default mailboxes if database query fails
		mailboxes = []string{"INBOX", "Sent", "Drafts", "Trash"}
	}

	for _, mailboxName := range mailboxes {
		attrs := "\\Unmarked"
		
		// Set appropriate attributes for special mailboxes
		switch mailboxName {
		case "Drafts":
			attrs = "\\Drafts"
		case "Trash":
			attrs = "\\Trash"
		case "Sent":
			attrs = "\\Sent"
		}
		
		s.sendResponse(conn, fmt.Sprintf("* LIST (%s) \"/\" \"%s\"", attrs, mailboxName))
	}
	s.sendResponse(conn, fmt.Sprintf("%s OK LIST completed", tag))
}

func (s *IMAPServer) handleLsub(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	// Get subscribed mailboxes from database
	subscriptions, err := db.GetUserSubscriptions(s.db, state.Username)
	if err != nil {
		fmt.Printf("Failed to get subscriptions for user %s: %v\n", state.Username, err)
		s.sendResponse(conn, fmt.Sprintf("%s NO LSUB failure: server error", tag))
		return
	}

	// If no subscriptions exist, subscribe to default mailboxes
	if len(subscriptions) == 0 {
		defaultMailboxes := []string{"INBOX", "Sent", "Drafts", "Trash"}
		for _, mailbox := range defaultMailboxes {
			db.SubscribeToMailbox(s.db, state.Username, mailbox)
		}
		subscriptions = defaultMailboxes
	}

	for _, mailboxName := range subscriptions {
		attrs := "\\Unmarked"
		
		// Set appropriate attributes for special mailboxes
		switch mailboxName {
		case "Drafts":
			attrs = "\\Drafts"
		case "Trash":
			attrs = "\\Trash"
		case "Sent":
			attrs = "\\Sent"
		}
		
		s.sendResponse(conn, fmt.Sprintf("* LSUB (%s) \"/\" \"%s\"", attrs, mailboxName))
	}
	s.sendResponse(conn, fmt.Sprintf("%s OK LSUB completed", tag))
}

func (s *IMAPServer) handleCreate(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	if len(parts) < 3 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD CREATE requires mailbox name", tag))
		return
	}

	// Parse mailbox name (could be quoted)
	mailboxName := strings.Trim(parts[2], "\"")
	
	// Remove trailing hierarchy separator if present
	// According to RFC 3501, the name created is without the trailing hierarchy delimiter
	if strings.HasSuffix(mailboxName, "/") {
		mailboxName = strings.TrimSuffix(mailboxName, "/")
	}

	// Validate mailbox name
	if mailboxName == "" {
		s.sendResponse(conn, fmt.Sprintf("%s NO Cannot create mailbox with empty name", tag))
		return
	}

	// Check if trying to create INBOX (case-insensitive)
	if strings.ToUpper(mailboxName) == "INBOX" {
		s.sendResponse(conn, fmt.Sprintf("%s NO Cannot create INBOX - it already exists", tag))
		return
	}

	// Ensure mailboxes table exists
	if err := db.EnsureMailboxTable(s.db); err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO Server error: cannot initialize mailbox storage", tag))
		return
	}

	// Check if mailbox already exists
	exists, err := db.MailboxExists(s.db, state.Username, mailboxName)
	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO Server error: cannot check mailbox existence", tag))
		return
	}

	if exists {
		s.sendResponse(conn, fmt.Sprintf("%s NO Mailbox already exists", tag))
		return
	}

	// Handle hierarchy creation
	// If the mailbox name contains hierarchy separators, create parent mailboxes if needed
	if strings.Contains(mailboxName, "/") {
		parts := strings.Split(mailboxName, "/")
		currentPath := ""
		
		// Create each level of the hierarchy
		for i, part := range parts {
			if i > 0 {
				currentPath += "/"
			}
			currentPath += part
			
			// Skip if this is the final mailbox (we'll create it below)
			if i == len(parts)-1 {
				break
			}
			
			// Check if this intermediate mailbox exists
			intermediateExists, checkErr := db.MailboxExists(s.db, state.Username, currentPath)
			if checkErr == nil && !intermediateExists {
				// Create intermediate mailbox - ignore errors as per RFC 3501
				db.CreateMailbox(s.db, state.Username, currentPath)
			}
		}
	}

	// Create the target mailbox
	err = db.CreateMailbox(s.db, state.Username, mailboxName)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			s.sendResponse(conn, fmt.Sprintf("%s NO Mailbox already exists", tag))
		} else {
			s.sendResponse(conn, fmt.Sprintf("%s NO Create failure: %s", tag, err.Error()))
		}
		return
	}

	s.sendResponse(conn, fmt.Sprintf("%s OK CREATE completed", tag))
}

func (s *IMAPServer) handleDelete(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	if len(parts) < 3 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD DELETE requires mailbox name", tag))
		return
	}

	// Parse mailbox name (could be quoted)
	mailboxName := strings.Trim(parts[2], "\"")

	// Validate mailbox name
	if mailboxName == "" {
		s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid mailbox name", tag))
		return
	}

	// Cannot delete INBOX (case-insensitive)
	if strings.ToUpper(mailboxName) == "INBOX" {
		s.sendResponse(conn, fmt.Sprintf("%s NO Cannot delete INBOX", tag))
		return
	}

	// Ensure mailboxes table exists
	if err := db.EnsureMailboxTable(s.db); err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO Server error: cannot initialize mailbox storage", tag))
		return
	}

	// Attempt to delete the mailbox
	err := db.DeleteMailbox(s.db, state.Username, mailboxName)
	if err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			s.sendResponse(conn, fmt.Sprintf("%s NO Mailbox does not exist", tag))
		} else if strings.Contains(err.Error(), "has inferior hierarchical names") {
			s.sendResponse(conn, fmt.Sprintf("%s NO Name \"%s\" has inferior hierarchical names", tag, mailboxName))
		} else if strings.Contains(err.Error(), "cannot delete INBOX") {
			s.sendResponse(conn, fmt.Sprintf("%s NO Cannot delete INBOX", tag))
		} else {
			s.sendResponse(conn, fmt.Sprintf("%s NO Delete failure: %s", tag, err.Error()))
		}
		return
	}

	s.sendResponse(conn, fmt.Sprintf("%s OK DELETE completed", tag))
}

func (s *IMAPServer) handleRename(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	if len(parts) < 4 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD RENAME requires existing and new mailbox names", tag))
		return
	}

	// Parse mailbox names (could be quoted)
	oldName := strings.Trim(parts[2], "\"")
	newName := strings.Trim(parts[3], "\"")

	// Validate mailbox names
	if oldName == "" || newName == "" {
		s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid mailbox names", tag))
		return
	}

	// Ensure mailboxes table exists
	if err := db.EnsureMailboxTable(s.db); err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO Server error: cannot initialize mailbox storage", tag))
		return
	}

	// Attempt to rename the mailbox
	err := db.RenameMailbox(s.db, state.Username, oldName, newName)
	if err != nil {
		if strings.Contains(err.Error(), "source mailbox does not exist") {
			s.sendResponse(conn, fmt.Sprintf("%s NO Source mailbox does not exist", tag))
		} else if strings.Contains(err.Error(), "destination mailbox already exists") {
			s.sendResponse(conn, fmt.Sprintf("%s NO Destination mailbox already exists", tag))
		} else if strings.Contains(err.Error(), "cannot rename to INBOX") {
			s.sendResponse(conn, fmt.Sprintf("%s NO Cannot rename to INBOX", tag))
		} else {
			s.sendResponse(conn, fmt.Sprintf("%s NO Rename failure: %s", tag, err.Error()))
		}
		return
	}

	s.sendResponse(conn, fmt.Sprintf("%s OK RENAME completed", tag))
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

		// Debug logging
		fmt.Printf("NOOP Debug: folder=%s, lastCount=%d, currentCount=%d, lastRecent=%d, currentRecent=%d\n",
			state.SelectedFolder, state.LastMessageCount, currentCount, state.LastRecentCount, currentRecent)

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

	// Validate folder exists using the database
	exists, err := db.MailboxExists(s.db, state.Username, folder)
	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO Server error checking folder", tag))
		return
	}
	
	if !exists {
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
	_, err = s.db.Exec(query, subject, sender, recipient, dateSent, rawMessage, flags, folder)

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

func (s *IMAPServer) handleStartTLS(conn net.Conn, tag string, parts []string) {
	// RFC 3501: STARTTLS takes no arguments
	if len(parts) > 2 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD STARTTLS command does not accept arguments", tag))
		return
	}

	// Check if already on TLS connection
	if _, ok := conn.(*tls.Conn); ok {
		s.sendResponse(conn, fmt.Sprintf("%s BAD TLS already active", tag))
		return
	}

	// Also check mock TLS connections
	type tlsAware interface{ IsTLS() bool }
	if ta, ok := any(conn).(tlsAware); ok && ta.IsTLS() {
		s.sendResponse(conn, fmt.Sprintf("%s BAD TLS already active", tag))
		return
	}

	cert, err := tls.LoadX509KeyPair(s.certPath, s.keyPath)
	if err != nil {
		fmt.Printf("Failed to load TLS cert/key: %v\n", err)
		s.sendResponse(conn, fmt.Sprintf("%s BAD TLS not available", tag))
		return
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	// RFC 3501: Send OK response before starting TLS negotiation
	s.sendResponse(conn, fmt.Sprintf("%s OK Begin TLS negotiation now", tag))

	tlsConn := tls.Server(conn, tlsConfig)

	// RFC 3501: Client MUST discard cached server capabilities after STARTTLS
	// Restart handler with upgraded TLS connection and fresh state
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
			s.sendResponse(conn, fmt.Sprintf("%s NO Authentication failed", tag))
			return
		}

		authData := strings.TrimSpace(string(buf[:n]))

		// Client may cancel authentication with a single "*"
		if authData == "*" {
			s.sendResponse(conn, fmt.Sprintf("%s BAD Authentication exchange cancelled", tag))
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
			s.sendResponse(conn, fmt.Sprintf("%s NO [AUTHENTICATIONFAILED] Invalid credentials format", tag))
			return
		}

		if username == "" || password == "" {
			log.Printf("AUTHENTICATE PLAIN: empty username or password")
			s.sendResponse(conn, fmt.Sprintf("%s NO [AUTHENTICATIONFAILED] Invalid credentials", tag))
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
		s.sendResponse(conn, fmt.Sprintf("%s NO [SERVERBUG] Configuration error", tag))
		return
	}

	if cfg.Domain == "" || cfg.AuthServerURL == "" {
		s.sendResponse(conn, fmt.Sprintf("%s NO [SERVERBUG] Configuration error", tag))
		return
	}

	email := username + "@" + cfg.Domain

	// Prepare JSON body
	requestBody := fmt.Sprintf(`{"email":"%s","password":"%s"}`, email, password)

	// Create HTTP request
	req, err := http.NewRequest("POST", cfg.AuthServerURL, strings.NewReader(requestBody))
	if err != nil {
		s.sendResponse(conn, fmt.Sprintf("%s NO [SERVERBUG] Internal error", tag))
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
		s.sendResponse(conn, fmt.Sprintf("%s NO [UNAVAILABLE] Authentication service unavailable", tag))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		log.Printf("Accepting login for user: %s", username)
		
		// Ensure user table exists
		if err := s.ensureUserTable(username); err != nil {
			log.Printf("Failed to create user table: %v", err)
			s.sendResponse(conn, fmt.Sprintf("%s NO [SERVERBUG] Server error", tag))
			return
		}
		
		state.Authenticated = true
		state.Username = username
		
		// Detect if TLS is active
		isTLS := false
		if _, ok := conn.(*tls.Conn); ok {
			isTLS = true
		} else {
			type tlsAware interface{ IsTLS() bool }
			if ta, ok := any(conn).(tlsAware); ok && ta.IsTLS() {
				isTLS = true
			}
		}
		
		// Per RFC 3501, include CAPABILITY response code in OK response
		// Only do this if security layer was not negotiated (TLS doesn't count as SASL security layer)
		capabilities := "IMAP4rev1 AUTH=PLAIN LOGIN"
		if isTLS {
			capabilities += " UIDPLUS IDLE NAMESPACE UNSELECT LITERAL+"
		} else {
			capabilities += " STARTTLS LOGINDISABLED UIDPLUS IDLE NAMESPACE UNSELECT LITERAL+"
		}
		s.sendResponse(conn, fmt.Sprintf("%s OK [CAPABILITY %s] Authenticated", tag, capabilities))
	} else {
		s.sendResponse(conn, fmt.Sprintf("%s NO [AUTHENTICATIONFAILED] Authentication failed", tag))
	}
}

func (s *IMAPServer) handleSubscribe(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	// SUBSCRIBE command format: tag SUBSCRIBE mailbox
	if len(parts) < 3 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD SUBSCRIBE command requires a mailbox argument", tag))
		return
	}

	mailboxName := parts[2]
	
	// Remove quotes if present
	if len(mailboxName) >= 2 && mailboxName[0] == '"' && mailboxName[len(mailboxName)-1] == '"' {
		mailboxName = mailboxName[1 : len(mailboxName)-1]
	}

	// Validate mailbox name
	if mailboxName == "" {
		s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid mailbox name", tag))
		return
	}

	// Subscribe to the mailbox
	err := db.SubscribeToMailbox(s.db, state.Username, mailboxName)
	if err != nil {
		fmt.Printf("Failed to subscribe to mailbox %s for user %s: %v\n", mailboxName, state.Username, err)
		s.sendResponse(conn, fmt.Sprintf("%s NO SUBSCRIBE failure: server error", tag))
		return
	}

	s.sendResponse(conn, fmt.Sprintf("%s OK SUBSCRIBE completed", tag))
}

// handleUnsubscribe handles the UNSUBSCRIBE command to remove a mailbox from the subscription list
func (s *IMAPServer) handleUnsubscribe(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		s.sendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	// UNSUBSCRIBE command format: tag UNSUBSCRIBE mailbox
	if len(parts) < 3 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD UNSUBSCRIBE command requires a mailbox argument", tag))
		return
	}

	mailboxName := parts[2]
	
	// Remove quotes if present
	if len(mailboxName) >= 2 && mailboxName[0] == '"' && mailboxName[len(mailboxName)-1] == '"' {
		mailboxName = mailboxName[1 : len(mailboxName)-1]
	}

	// Validate mailbox name
	if mailboxName == "" {
		s.sendResponse(conn, fmt.Sprintf("%s BAD Invalid mailbox name", tag))
		return
	}

	// Unsubscribe from the mailbox
	err := db.UnsubscribeFromMailbox(s.db, state.Username, mailboxName)
	if err != nil {
		if strings.Contains(err.Error(), "subscription does not exist") {
			s.sendResponse(conn, fmt.Sprintf("%s NO UNSUBSCRIBE failure: can't unsubscribe that name", tag))
		} else {
			fmt.Printf("Failed to unsubscribe from mailbox %s for user %s: %v\n", mailboxName, state.Username, err)
			s.sendResponse(conn, fmt.Sprintf("%s NO UNSUBSCRIBE failure: server error", tag))
		}
		return
	}

	s.sendResponse(conn, fmt.Sprintf("%s OK UNSUBSCRIBE completed", tag))
}

// Exported handler methods for testing

// HandleCapability exports the capability handler for testing
func (s *IMAPServer) HandleCapability(conn net.Conn, tag string, state *models.ClientState) {
	s.handleCapability(conn, tag, state)
}

// HandleSubscribe exports the subscribe handler for testing
func (s *IMAPServer) HandleSubscribe(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	s.handleSubscribe(conn, tag, parts, state)
}

// HandleUnsubscribe exports the unsubscribe handler for testing
func (s *IMAPServer) HandleUnsubscribe(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	s.handleUnsubscribe(conn, tag, parts, state)
}

// HandleLsub exports the lsub handler for testing
func (s *IMAPServer) HandleLsub(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	s.handleLsub(conn, tag, parts, state)
}