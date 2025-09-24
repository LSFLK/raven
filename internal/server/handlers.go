package server

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"

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
	s.sendResponse(conn, "+ idling")
	s.sendResponse(conn, fmt.Sprintf("%s OK IDLE completed", tag))
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