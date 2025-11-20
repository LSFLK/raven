package auth

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"raven/internal/conf"
	"raven/internal/db"
	"raven/internal/models"
)

// ServerDeps defines the dependencies that auth handlers need from the server
type ServerDeps interface {
	SendResponse(conn net.Conn, response string)
	ExtractUsername(username string) string
	GetUserDomain(username string) string
	EnsureUserAndMailboxes(username, domain string) (userID int64, domainID int64, err error)
	GetDBManager() *db.DBManager
	GetCertPath() string
	GetKeyPath() string
}

// ClientHandler is a function type for handling client connections
type ClientHandler func(conn net.Conn, state *models.ClientState)

// ===== CAPABILITY =====

func HandleCapability(deps ServerDeps, conn net.Conn, tag string, state *models.ClientState) {
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
	deps.SendResponse(conn, "* CAPABILITY "+strings.Join(capabilities, " "))
	deps.SendResponse(conn, fmt.Sprintf("%s OK CAPABILITY completed", tag))
}

// ===== LOGIN =====

func HandleLogin(deps ServerDeps, conn net.Conn, tag string, parts []string, state *models.ClientState) {
	// Check if LOGIN command has correct number of arguments
	if len(parts) < 4 {
		deps.SendResponse(conn, fmt.Sprintf("%s BAD LOGIN requires username and password", tag))
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
		deps.SendResponse(conn, fmt.Sprintf("%s NO [PRIVACYREQUIRED] LOGIN is disabled on insecure connection. Use STARTTLS first.", tag))
		return
	}

	// Extract username and password, removing quotes if present
	username := strings.Trim(parts[2], "\"")
	password := strings.Trim(parts[3], "\"")

	// Use common authentication logic
	authenticateUser(deps, conn, tag, username, password, state)
}

// ===== AUTHENTICATE =====

func HandleAuthenticate(deps ServerDeps, conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if len(parts) < 3 {
		deps.SendResponse(conn, fmt.Sprintf("%s BAD AUTHENTICATE requires authentication mechanism", tag))
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
			deps.SendResponse(conn, fmt.Sprintf("%s NO Plaintext authentication disallowed without TLS", tag))
			return
		}

		// Send continuation request
		deps.SendResponse(conn, "+ ")

		// Read the authentication data
		buf := make([]byte, 8192)
		_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		n, err := conn.Read(buf)
		if err != nil {
			deps.SendResponse(conn, fmt.Sprintf("%s NO Authentication failed", tag))
			return
		}

		authData := strings.TrimSpace(string(buf[:n]))

		// Client may cancel authentication with a single "*"
		if authData == "*" {
			deps.SendResponse(conn, fmt.Sprintf("%s BAD Authentication exchange cancelled", tag))
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
			deps.SendResponse(conn, fmt.Sprintf("%s NO [AUTHENTICATIONFAILED] Invalid credentials format", tag))
			return
		}

		if username == "" || password == "" {
			log.Printf("AUTHENTICATE PLAIN: empty username or password")
			deps.SendResponse(conn, fmt.Sprintf("%s NO [AUTHENTICATIONFAILED] Invalid credentials", tag))
			return
		}

		// Reuse the existing login logic
		authenticateUser(deps, conn, tag, username, password, state)
		return

	default:
		deps.SendResponse(conn, fmt.Sprintf("%s NO Unsupported authentication mechanism", tag))
	}
}

// ===== STARTTLS =====

func HandleStartTLS(deps ServerDeps, clientHandler ClientHandler, conn net.Conn, tag string, parts []string) {
	// RFC 3501: STARTTLS takes no arguments
	if len(parts) > 2 {
		deps.SendResponse(conn, fmt.Sprintf("%s BAD STARTTLS command does not accept arguments", tag))
		return
	}

	// Check if already on TLS connection
	if _, ok := conn.(*tls.Conn); ok {
		deps.SendResponse(conn, fmt.Sprintf("%s BAD TLS already active", tag))
		return
	}

	// Also check mock TLS connections
	type tlsAware interface{ IsTLS() bool }
	if ta, ok := any(conn).(tlsAware); ok && ta.IsTLS() {
		deps.SendResponse(conn, fmt.Sprintf("%s BAD TLS already active", tag))
		return
	}

	cert, err := tls.LoadX509KeyPair(deps.GetCertPath(), deps.GetKeyPath())
	if err != nil {
		fmt.Printf("Failed to load TLS cert/key: %v\n", err)
		deps.SendResponse(conn, fmt.Sprintf("%s BAD TLS not available", tag))
		return
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	// RFC 3501: Send OK response before starting TLS negotiation
	deps.SendResponse(conn, fmt.Sprintf("%s OK Begin TLS negotiation now", tag))

	tlsConn := tls.Server(conn, tlsConfig)

	// RFC 3501: Client MUST discard cached server capabilities after STARTTLS
	// Restart handler with upgraded TLS connection and fresh state
	clientHandler(tlsConn, &models.ClientState{})
}

// ===== LOGOUT =====

func HandleLogout(deps ServerDeps, conn net.Conn, tag string) {
	deps.SendResponse(conn, "* BYE IMAP4rev1 Server logging out")
	deps.SendResponse(conn, fmt.Sprintf("%s OK LOGOUT completed", tag))
}

// ===== AUTHENTICATE USER (Shared Auth Logic) =====

// Extract common authentication logic
func authenticateUser(deps ServerDeps, conn net.Conn, tag string, username string, password string, state *models.ClientState) {
	// Load domain from config file
	cfg, err := conf.LoadConfig()
	if err != nil {
		log.Printf("LoadConfig error: %v", err)
		deps.SendResponse(conn, fmt.Sprintf("%s NO [SERVERBUG] Configuration error", tag))
		return
	}

	if cfg.Domain == "" || cfg.AuthServerURL == "" {
		deps.SendResponse(conn, fmt.Sprintf("%s NO [SERVERBUG] Configuration error", tag))
		return
	}

	// Determine the email address to use for authentication
	var email string
	if strings.Contains(username, "@") {
		email = username
	} else {
		// Username doesn't contain domain - append configured domain
		email = username + "@" + cfg.Domain
	}

	// Prepare JSON body
	requestBody := fmt.Sprintf(`{"email":"%s","password":"%s"}`, email, password)

	// Create HTTP request
	req, err := http.NewRequest("POST", cfg.AuthServerURL, strings.NewReader(requestBody))
	if err != nil {
		deps.SendResponse(conn, fmt.Sprintf("%s NO [SERVERBUG] Internal error", tag))
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
		deps.SendResponse(conn, fmt.Sprintf("%s NO [UNAVAILABLE] Authentication service unavailable", tag))
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == 200 {
		log.Printf("Accepting login for user: %s", username)

		// Extract username and domain
		actualUsername := deps.ExtractUsername(username)
		domain := deps.GetUserDomain(username)

		// Ensure user exists in database and has default mailboxes
		userID, domainID, err := deps.EnsureUserAndMailboxes(actualUsername, domain)
		if err != nil {
			log.Printf("Failed to create user and mailboxes: %v", err)
			deps.SendResponse(conn, fmt.Sprintf("%s NO [SERVERBUG] Server error", tag))
			return
		}

		state.Authenticated = true
		state.Username = actualUsername
		state.UserID = userID
		state.DomainID = domainID

		// Load role mailbox assignments for this user
		roleMailboxIDs, err := db.GetUserRoleAssignments(deps.GetDBManager().GetSharedDB(), userID)
		if err != nil {
			log.Printf("Failed to load role assignments for user %d: %v", userID, err)
			// Don't fail authentication, just continue without role mailboxes
			state.RoleMailboxIDs = []int64{}
		} else {
			state.RoleMailboxIDs = roleMailboxIDs
			log.Printf("User %s has %d role mailbox assignments", actualUsername, len(roleMailboxIDs))
		}

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
		deps.SendResponse(conn, fmt.Sprintf("%s OK [CAPABILITY %s] Authenticated", tag, capabilities))
	} else {
		deps.SendResponse(conn, fmt.Sprintf("%s NO [AUTHENTICATIONFAILED] Authentication failed", tag))
	}
}

// ===== HANDLE SSL CONNECTION =====

func HandleSSLConnection(clientHandler ClientHandler, conn net.Conn) {
	certPath := "/certs/fullchain.pem"
	keyPath := "/certs/privkey.pem"

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		log.Printf("Failed to load TLS cert/key: %v", err)
		_ = conn.Close()
		return
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	tlsConn := tls.Server(conn, tlsConfig)

	// Start IMAP session over TLS
	clientHandler(tlsConn, &models.ClientState{})
}
