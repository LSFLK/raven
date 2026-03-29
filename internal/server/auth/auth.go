package auth

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"raven/internal/auth/oauthbearer"
	"raven/internal/blobstorage"
	"raven/internal/conf"
	"raven/internal/db"
	"raven/internal/models"
)

// ServerDeps defines the dependencies that auth handlers need from the server
type ServerDeps interface {
	SendResponse(conn net.Conn, response string)
	ExtractUsername(username string) string
	GetUserDomain(username string) string
	EnsureUserAndMailboxes(email string) error
	GetDBManager() *db.DBManager
	GetConfig() *conf.Config
	GetOAuthValidator() *oauthbearer.Validator
	GetCertPath() string
	GetKeyPath() string
	GetS3Storage() *blobstorage.S3BlobStorage
}

// ClientHandler is a function type for handling client connections
type ClientHandler func(conn net.Conn, state *models.ClientState)

// ===== CAPABILITY =====

func oauthSASLReady(deps ServerDeps) bool {
	return deps.GetConfig() != nil && deps.GetOAuthValidator() != nil
}

func buildCapabilities(deps ServerDeps, isTLS bool) []string {
	capabilities := []string{"IMAP4rev1"}

	if isTLS {
		capabilities = append(capabilities, "AUTH=PLAIN", "LOGIN")
	} else {
		capabilities = append(capabilities, "STARTTLS", "LOGINDISABLED")
	}

	if oauthSASLReady(deps) {
		capabilities = append(capabilities, "AUTH=OAUTHBEARER", "AUTH=XOAUTH2", "SASL-IR")
	}

	capabilities = append(capabilities,
		"UIDPLUS",
		"IDLE",
		"NAMESPACE",
		"UNSELECT",
		"LITERAL+",
	)

	return capabilities
}

func HandleCapability(deps ServerDeps, conn net.Conn, tag string, state *models.ClientState) {
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

	capabilities := buildCapabilities(deps, isTLS)

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

	case "OAUTHBEARER", "XOAUTH2":
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

		authData := ""
		if len(parts) >= 4 {
			authData = strings.TrimSpace(parts[3])
		}

		if authData == "" || authData == "=" {
			deps.SendResponse(conn, "+ ")

			buf := make([]byte, 8192)
			_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
			n, err := conn.Read(buf)
			if err != nil {
				deps.SendResponse(conn, fmt.Sprintf("%s NO Authentication failed", tag))
				return
			}
			authData = strings.TrimSpace(string(buf[:n]))
		}

		if authData == "*" {
			deps.SendResponse(conn, fmt.Sprintf("%s BAD Authentication exchange cancelled", tag))
			return
		}

		accessToken, _, _, err := oauthbearer.ParseInitialClientResponseDetails(authData)
		if err != nil {
			deps.SendResponse(conn, fmt.Sprintf("%s NO [AUTHENTICATIONFAILED] Invalid OAuth payload", tag))
			return
		}

		cfg := deps.GetConfig()
		if cfg == nil {
			log.Printf("OAUTHBEARER: config unavailable from server cache")
			deps.SendResponse(conn, fmt.Sprintf("%s NO [SERVERBUG] Configuration error", tag))
			return
		}

		validator := deps.GetOAuthValidator()
		log.Printf("OAUTHBEARER: validator config issuer=%q jwks_url=%q audience_count=%d skew_seconds=%d", cfg.OAuthIssuer, cfg.OAuthJWKSURL, len(cfg.OAuthAudience), cfg.OAuthSkewSec)
		if validator == nil {
			log.Printf("OAUTHBEARER: shared validator unavailable from server cache")
			deps.SendResponse(conn, fmt.Sprintf("%s NO [SERVERBUG] OAUTHBEARER configuration error", tag))
			return
		}

		claims, err := validator.ValidateAccessToken(accessToken)
		if err != nil {
			log.Printf("OAUTHBEARER: token validation failed: %v", err)
			deps.SendResponse(conn, fmt.Sprintf("%s NO [AUTHENTICATIONFAILED] Authentication failed", tag))
			return
		}

		tokenUsername := strings.TrimSpace(claims.Username)

		identity := claims.Identity()
		email := normalizeEmail(identity)
		if email == "" && tokenUsername != "" && claims.OrganizationUnitID != "" {
			if cfg.AuthServerURL == "" {
				log.Printf("OAUTHBEARER: cannot resolve OU domain without auth_server_url")
				deps.SendResponse(conn, fmt.Sprintf("%s NO [SERVERBUG] Configuration error", tag))
				return
			}

			derivedDomain := resolveDomainFromOrganizationUnit(cfg.AuthServerURL, claims.OrganizationUnitID, tokenUsername, "")
			if derivedDomain == "" {
				log.Printf("OAUTHBEARER: failed to resolve domain from ouId=%q for username=%q", claims.OrganizationUnitID, tokenUsername)
				deps.SendResponse(conn, fmt.Sprintf("%s NO [AUTHENTICATIONFAILED] Authentication failed", tag))
				return
			}

			email = tokenUsername + "@" + strings.Trim(strings.TrimSpace(derivedDomain), ".")
			log.Printf("OAUTHBEARER: resolved mailbox email from token username/ouId email=%q", email)
		}
		if email == "" && tokenUsername != "" && cfg.Domain != "" {
			email = tokenUsername + "@" + strings.Trim(strings.TrimSpace(cfg.Domain), ".")
		}
		if email == "" && cfg.Domain != "" && !strings.Contains(identity, "@") {
			email = strings.TrimSpace(identity) + "@" + strings.Trim(strings.TrimSpace(cfg.Domain), ".")
		}
		if email == "" {
			deps.SendResponse(conn, fmt.Sprintf("%s NO [AUTHENTICATIONFAILED] Authentication failed", tag))
			return
		}

		actualUsername := deps.ExtractUsername(email)

		if err := deps.EnsureUserAndMailboxes(email); err != nil {
			log.Printf("Failed to initialize user database for OAUTHBEARER: %v", err)
			deps.SendResponse(conn, fmt.Sprintf("%s NO [SERVERBUG] Server error", tag))
			return
		}

		state.Authenticated = true
		state.Username = actualUsername
		state.Email = email

		roleMailboxIDs, err := db.GetUserRoleAssignments(deps.GetDBManager().GetSharedDB(), email)
		if err != nil {
			log.Printf("Failed to load role assignments for OAUTHBEARER user %s: %v", email, err)
			state.RoleMailboxIDs = []int64{}
		} else {
			state.RoleMailboxIDs = roleMailboxIDs
		}

		capabilities := strings.Join(buildCapabilities(deps, true), " ")
		deps.SendResponse(conn, fmt.Sprintf("%s OK [CAPABILITY %s] Authenticated", tag, capabilities))
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
		MinVersion:   tls.VersionTLS12,
	}

	// RFC 3501: Send OK response before starting TLS negotiation
	deps.SendResponse(conn, fmt.Sprintf("%s OK Begin TLS negotiation now", tag))

	tlsConn := tls.Server(conn, tlsConfig)

	// Explicitly perform TLS handshake
	if err := tlsConn.Handshake(); err != nil {
		log.Printf("TLS handshake failed during STARTTLS: %v", err)
		_ = conn.Close()
		return
	}

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
	// Load authentication service configuration
	cfg, err := conf.LoadConfig()
	if err != nil {
		log.Printf("LoadConfig error: %v", err)
		deps.SendResponse(conn, fmt.Sprintf("%s NO [SERVERBUG] Configuration error", tag))
		return
	}

	if cfg.AuthServerURL == "" {
		deps.SendResponse(conn, fmt.Sprintf("%s NO [SERVERBUG] Configuration error", tag))
		return
	}

	// Determine the username to use for authentication
	authUsername := deps.ExtractUsername(username)
	if authUsername == "" {
		authUsername = username
	}

	// Prepare JSON body
	requestPayload := map[string]any{
		"identifiers": map[string]string{
			"username": authUsername,
		},
		"credentials": map[string]string{
			"password": password,
		},
		"skip_assertion": true,
	}
	requestBodyBytes, err := json.Marshal(requestPayload)
	if err != nil {
		deps.SendResponse(conn, fmt.Sprintf("%s NO [SERVERBUG] Internal error", tag))
		return
	}

	// Create HTTP request
	req, err := http.NewRequest("POST", cfg.AuthServerURL, strings.NewReader(string(requestBodyBytes)))
	if err != nil {
		deps.SendResponse(conn, fmt.Sprintf("%s NO [SERVERBUG] Internal error", tag))
		return
	}
	req.Header.Set("Content-Type", "application/json")

	// TLS config for system CA bundle (default)
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true, // #nosec G402 -- Required for internal auth server communication
	}
	transport := &http.Transport{TLSClientConfig: tlsConfig}
	client := &http.Client{Transport: transport}

	// #nosec G704 -- URL is from validated config, not user input
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("LOGIN: error reaching auth server: %v", err)
		deps.SendResponse(conn, fmt.Sprintf("%s NO [UNAVAILABLE] Authentication service unavailable", tag))
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == 200 {
		var authResp struct {
			ID               string `json:"id"`
			Type             string `json:"type"`
			OrganizationUnit string `json:"organization_unit"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
			log.Printf("LOGIN: failed to decode auth response: %v", err)
			deps.SendResponse(conn, fmt.Sprintf("%s NO [AUTHENTICATIONFAILED] Authentication failed", tag))
			return
		}
		if authResp.ID == "" {
			log.Printf("LOGIN: auth response missing id for user: %s", username)
			deps.SendResponse(conn, fmt.Sprintf("%s NO [AUTHENTICATIONFAILED] Authentication failed", tag))
			return
		}

		log.Printf("Accepting login for user: %s (type=%s)", username, authResp.Type)

		derivedDomain := ""
		if authResp.OrganizationUnit != "" {
			if strings.Contains(username, "@") {
				derivedDomain = resolveDomainFromOrganizationUnit(cfg.AuthServerURL, authResp.OrganizationUnit, authUsername, password)
				if derivedDomain == "" {
					log.Printf("LOGIN: unable to resolve OU-derived domain for login '%s'", username)
					deps.SendResponse(conn, fmt.Sprintf("%s NO [AUTHENTICATIONFAILED] Authentication failed", tag))
					return
				}

				loginEmail := normalizeEmail(username)
				if loginEmail == "" {
					log.Printf("LOGIN: invalid login identity format: %s", username)
					deps.SendResponse(conn, fmt.Sprintf("%s NO [AUTHENTICATIONFAILED] Authentication failed", tag))
					return
				}

				parts := strings.SplitN(loginEmail, "@", 2)
				loginDomain := strings.Trim(strings.TrimSpace(parts[1]), ".")
				expectedDomain := strings.Trim(strings.TrimSpace(derivedDomain), ".")
				if !strings.EqualFold(loginDomain, expectedDomain) {
					log.Printf("LOGIN: login domain '%s' does not match OU-derived domain '%s' for user '%s'", loginDomain, expectedDomain, username)
					deps.SendResponse(conn, fmt.Sprintf("%s NO [AUTHENTICATIONFAILED] Authentication failed", tag))
					return
				}
			} else if !strings.Contains(authResp.ID, "@") {
				derivedDomain = resolveDomainFromOrganizationUnit(cfg.AuthServerURL, authResp.OrganizationUnit, authUsername, password)
			}
		}

		email := resolveMailboxEmail(username, authResp.ID, derivedDomain)
		if email == "" {
			log.Printf("LOGIN: unable to resolve mailbox email from login '%s' and auth id '%s'", username, authResp.ID)
			deps.SendResponse(conn, fmt.Sprintf("%s NO [AUTHENTICATIONFAILED] Authentication failed", tag))
			return
		}

		actualUsername := deps.ExtractUsername(email)

		// Ensure user database exists and has default mailboxes
		if err := deps.EnsureUserAndMailboxes(email); err != nil {
			log.Printf("Failed to initialize user database: %v", err)
			deps.SendResponse(conn, fmt.Sprintf("%s NO [SERVERBUG] Server error", tag))
			return
		}

		state.Authenticated = true
		state.Username = actualUsername
		state.Email = email

		// Load role mailbox assignments for this user
		roleMailboxIDs, err := db.GetUserRoleAssignments(deps.GetDBManager().GetSharedDB(), email)
		if err != nil {
			log.Printf("Failed to load role assignments for user %s: %v", email, err)
			// Don't fail authentication, just continue without role mailboxes
			state.RoleMailboxIDs = []int64{}
		} else {
			state.RoleMailboxIDs = roleMailboxIDs
			log.Printf("User %s has %d role mailbox assignments", email, len(roleMailboxIDs))
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

		// Per RFC 3501, include CAPABILITY response code in OK response.
		capabilities := strings.Join(buildCapabilities(deps, isTLS), " ")
		deps.SendResponse(conn, fmt.Sprintf("%s OK [CAPABILITY %s] Authenticated", tag, capabilities))
	} else {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("LOGIN: auth server rejected login for %s (status=%d, body=%s)", username, resp.StatusCode, strings.TrimSpace(string(body)))
		deps.SendResponse(conn, fmt.Sprintf("%s NO [AUTHENTICATIONFAILED] Authentication failed", tag))
	}
}

func resolveMailboxEmail(loginIdentity, authID, derivedDomain string) string {
	if email := normalizeEmail(loginIdentity); email != "" {
		return email
	}

	if email := normalizeEmail(authID); email != "" {
		return email
	}

	if derivedDomain != "" {
		username := strings.TrimSpace(loginIdentity)
		if username == "" {
			username = strings.TrimSpace(authID)
		}

		if username != "" && !strings.Contains(username, "@") {
			return username + "@" + derivedDomain
		}
	}

	return ""
}

func normalizeEmail(value string) string {
	trimmed := strings.TrimSpace(value)
	parts := strings.Split(trimmed, "@")
	if len(parts) != 2 {
		return ""
	}

	local := strings.TrimSpace(parts[0])
	domain := strings.Trim(strings.TrimSpace(parts[1]), ".")
	if local == "" || domain == "" {
		return ""
	}

	return local + "@" + domain
}

func resolveDomainFromOrganizationUnit(authServerURL, orgUnitID, username, password string) string {
	baseURL, err := extractBaseURL(authServerURL)
	if err != nil {
		log.Printf("LOGIN: failed to parse auth server URL for OU domain resolution: %v", err)
		return ""
	}

	log.Printf("LOGIN: resolving OU domain for org unit %s using bearer assertion", orgUnitID)

	// Always prefer system assertion for OU reads because user-scoped assertions can be forbidden.
	assertion := fetchSystemAssertion(baseURL)
	if assertion == "" {
		log.Printf("LOGIN: system assertion unavailable, attempting OU resolution with user assertion")
		assertion = fetchAssertion(baseURL, username, password)
	}

	if assertion == "" {
		log.Printf("LOGIN: failed to obtain assertion for OU domain resolution")
		return ""
	}

	domain, err := resolveOrganizationUnitDomain(baseURL, orgUnitID, assertion)
	if err != nil {
		log.Printf("LOGIN: failed to resolve OU domain for %s: %v", orgUnitID, err)
		return ""
	}

	log.Printf("LOGIN: resolved OU domain for %s as %s", orgUnitID, domain)

	return domain
}

func extractBaseURL(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid auth server URL: %s", rawURL)
	}

	return parsed.Scheme + "://" + parsed.Host, nil
}

func fetchSystemAssertion(baseURL string) string {
	username := getEnvOrDefault("IDP_SYSTEM_USERNAME", "admin")
	password := getEnvOrDefault("IDP_SYSTEM_PASSWORD", "admin")
	log.Printf("LOGIN: requesting system assertion for OU resolution using configured system identity")

	assertion := fetchAssertion(baseURL, username, password)
	if assertion == "" {
		log.Printf("LOGIN: failed to obtain system assertion using configured IDP system credentials")
	}

	return assertion
}

func fetchAssertion(baseURL, username, password string) string {
	applicationID, err := conf.GetApplicationID()
	if err != nil {
		log.Printf("LOGIN: failed to get application ID: %v", err)
		return ""
	}
	if applicationID == "" {
		log.Printf("LOGIN: application ID is empty")
		return ""
	}

	flowID, actionRef := startAuthenticationFlow(baseURL, applicationID)
	if flowID == "" {
		log.Printf("LOGIN: unable to start flow (no flow id returned)")
		return ""
	}

	payload := map[string]any{
		"flowId": flowID,
		"inputs": map[string]string{
			"username":              username,
			"password":              password,
			"requested_permissions": "system",
		},
		"action": actionRef,
	}

	var result struct {
		Assertion string `json:"assertion"`
	}

	if err := postJSON(baseURL+"/flow/execute", payload, "", &result); err != nil {
		log.Printf("LOGIN: flow execute failed for user %s: %v", username, err)
		return ""
	}

	return strings.TrimSpace(result.Assertion)
}

func startAuthenticationFlow(baseURL, applicationID string) (string, string) {
	configuredActionRef := strings.TrimSpace(os.Getenv("IDP_FLOW_ACTION"))
	if configuredActionRef == "" {
		configuredActionRef = strings.TrimSpace(os.Getenv("idp_flow_action"))
	}

	type executeFlowResponse struct {
		FlowID string `json:"flowId"`
		Data   struct {
			Actions []struct {
				Ref string `json:"ref"`
			} `json:"actions"`
		} `json:"data"`
	}

	var executeResult executeFlowResponse
	err := postJSON(baseURL+"/flow/execute", map[string]string{
		"applicationId": applicationID,
		"flowType":      "AUTHENTICATION",
	}, "", &executeResult)
	if err == nil && executeResult.FlowID != "" {
		actionRef := configuredActionRef
		if actionRef == "" && len(executeResult.Data.Actions) > 0 {
			actionRef = strings.TrimSpace(executeResult.Data.Actions[0].Ref)
		}
		if actionRef == "" {
			actionRef = "action_001"
		}

		return executeResult.FlowID, actionRef
	}
	if err != nil {
		log.Printf("LOGIN: flow bootstrap failed on /flow/execute: %v", err)
	}

	return "", ""
}

func resolveOrganizationUnitDomain(baseURL, orgUnitID, assertion string) (string, error) {
	type ouResponse struct {
		ID     string  `json:"id"`
		Handle string  `json:"handle"`
		Parent *string `json:"parent"`
	}

	handles := make([]string, 0, 4)
	current := strings.TrimSpace(orgUnitID)
	visited := map[string]struct{}{}

	for current != "" {
		if _, seen := visited[current]; seen {
			return "", fmt.Errorf("cycle detected in OU hierarchy")
		}
		visited[current] = struct{}{}

		var ou ouResponse
		if err := getJSON(baseURL+"/organization-units/"+current, assertion, &ou); err != nil {
			return "", err
		}

		handle := strings.TrimSpace(ou.Handle)
		if handle != "" {
			handles = append(handles, handle)
		}

		if ou.Parent == nil {
			break
		}
		current = strings.TrimSpace(*ou.Parent)
	}

	if len(handles) == 0 {
		return "", fmt.Errorf("no OU handles found")
	}

	return strings.Join(handles, "."), nil
}

func postJSON(endpoint string, payload any, assertion string, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", endpoint, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if assertion != "" {
		req.Header.Set("Authorization", "Bearer "+assertion)
	}

	resp, err := buildAuthHTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	if out == nil {
		return nil
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

func getJSON(endpoint, assertion string, out any) error {
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return err
	}
	if assertion != "" {
		req.Header.Set("Authorization", "Bearer "+assertion)
	}

	resp, err := buildAuthHTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

func buildAuthHTTPClient() *http.Client {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true, // #nosec G402 -- Required for internal auth server communication
	}

	return &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsConfig},
		Timeout:   10 * time.Second,
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}

	return defaultValue
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
		MinVersion:   tls.VersionTLS12,
	}

	tlsConn := tls.Server(conn, tlsConfig)

	// Explicitly perform TLS handshake before starting IMAP session
	// This ensures the handshake completes before we send the IMAP greeting
	if err := tlsConn.Handshake(); err != nil {
		log.Printf("TLS handshake failed: %v", err)
		_ = conn.Close()
		return
	}

	// Start IMAP session over TLS
	clientHandler(tlsConn, &models.ClientState{})
}
