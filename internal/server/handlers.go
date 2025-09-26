package server

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"go-imap/internal/models"
)

func (s *IMAPServer) handleCapability(conn net.Conn, tag string) {
    s.sendResponse(conn, "* CAPABILITY IMAP4rev1 STARTTLS LOGIN IDLE")
    s.sendResponse(conn, fmt.Sprintf("%s OK CAPABILITY completed", tag))
}

func (s *IMAPServer) handleLogin(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if len(parts) < 4 {
		s.sendResponse(conn, fmt.Sprintf("%s BAD LOGIN requires username and password", tag))
		return
	}

	username := strings.Trim(parts[2], "\"")
	password := strings.Trim(parts[3], "\"")

	email := username + "@openmail.lk"

	// Prepare JSON body
	requestBody := fmt.Sprintf(`{"email":"%s","password":"%s"}`, email, password)

	// Create HTTP request
	req, err := http.NewRequest("POST", "https://thunder-server:8090/users/authenticate", strings.NewReader(requestBody))
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
		state.Authenticated = true
		state.Username = username
		s.sendResponse(conn, fmt.Sprintf("%s OK LOGIN completed", tag))
	} else {
		s.sendResponse(conn, fmt.Sprintf("%s BAD LOGIN authentication failed", tag))
	}
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
	s.sendResponse(conn, "* BYE SQLite IMAP server logging out")
	s.sendResponse(conn, fmt.Sprintf("%s OK LOGOUT completed", tag))
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

	// Tell client we’re entering idle mode
	s.sendResponse(conn, "+ idling")

	buf := make([]byte, 4096)

	// Track previous state of the folder
	var prevCount, prevUnseen int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM mails WHERE folder = ?", state.SelectedFolder).Scan(&prevCount)
	_ = s.db.QueryRow("SELECT COUNT(*) FROM mails WHERE folder = ? AND flags NOT LIKE '%\\Seen%'", state.SelectedFolder).Scan(&prevUnseen)

	for {
		// Poll every 2 seconds for changes
		time.Sleep(2 * time.Second)

		// Check current mailbox state
		var count, unseen int
		_ = s.db.QueryRow("SELECT COUNT(*) FROM mails WHERE folder = ?", state.SelectedFolder).Scan(&count)
		_ = s.db.QueryRow("SELECT COUNT(*) FROM mails WHERE folder = ? AND flags NOT LIKE '%\\Seen%'", state.SelectedFolder).Scan(&unseen)

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

func (s *IMAPServer) handleStartTLS(conn net.Conn, tag string) {
	// Respond to client to begin TLS negotiation
	s.sendResponse(conn, fmt.Sprintf("%s OK Begin TLS negotiation", tag))

	certPath := "/etc/letsencrypt/live/openmail.lk/fullchain.pem"
	keyPath := "/etc/letsencrypt/live/openmail.lk/privkey.pem"

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