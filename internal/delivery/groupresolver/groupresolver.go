package groupresolver

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	// assertionCacheBufferTime is the time buffer before JWT expiry to refresh the assertion
	assertionCacheBufferTime = 30 * time.Second
)

// AssertionCache stores cached assertions with their expiry time
type AssertionCache struct {
	assertion string
	expiresAt time.Time
}

// GroupResolver handles group resolution from IDP
type GroupResolver struct {
	baseURL        string
	applicationID  string
	assertionCache *AssertionCache
	mu             sync.RWMutex
	httpClient     *http.Client
	systemUsername string
	systemPassword string
	flowActionRef  string
}

// NewGroupResolver creates a new GroupResolver
func NewGroupResolver(baseURL, applicationID, systemUsername, systemPassword string) *GroupResolver {
	return &GroupResolver{
		baseURL:        baseURL,
		applicationID:  applicationID,
		systemUsername: systemUsername,
		systemPassword: systemPassword,
		httpClient:     buildHTTPClient(),
	}
}

// ResolveGroupMembers resolves all members of a group (recursively) and returns their email addresses.
// groupName should be the portion before "-group@", e.g. "engineering" from "engineering-group@domain.com".
// Returns slice of resolved email addresses, deduplicated
func (gr *GroupResolver) ResolveGroupMembers(groupName string) ([]string, error) {
	// Get system assertion
	assertion, err := gr.getOrFreshAssertion()
	if err != nil {
		return nil, fmt.Errorf("failed to obtain assertion for group resolution: %w", err)
	}

	// Look up group ID by name
	groupID, err := gr.lookupGroupIDByName(assertion, groupName)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup group '%s': %w", groupName, err)
	}

	// Resolve members recursively
	visited := make(map[string]struct{})
	entries := make(map[string]struct{}) // deduplicate resolved addresses
	var resolveMembers func(string) error

	resolveMembers = func(gid string) error {
		if _, seen := visited[gid]; seen {
			log.Printf("GroupResolver: cycle detected for group %s, skipping", gid)
			return nil
		}
		visited[gid] = struct{}{}

		members, err := gr.fetchGroupMembers(assertion, gid)
		if err != nil {
			return fmt.Errorf("failed to fetch members of group %s: %w", gid, err)
		}

		for _, member := range members {
			if member.Type == "user" {
				// Resolve user to email address
				email, err := gr.resolveUserEmail(member.ID, baseDomain)
				if err != nil {
					log.Printf("GroupResolver: failed to resolve user %s: %v, skipping", member.ID, err)
					continue
				}
				entries[email] = struct{}{}
			} else if member.Type == "group" {
				// Recursively resolve nested group
				if err := resolveMembers(member.ID); err != nil {
					log.Printf("GroupResolver: failed to resolve nested group %s: %v, continuing", member.ID, err)
					// Don't fail entirely, just skip this nested group
				}
			}
		}

		return nil
	}

	if err := resolveMembers(groupID); err != nil {
		return nil, err
	}

	// Convert map keys to slice
	result := make([]string, 0, len(entries))
	for email := range entries {
		result = append(result, email)
	}

	log.Printf("GroupResolver: resolved %d unique members for group %s", len(result), groupName)
	return result, nil
}

// getOrFreshAssertion returns a cached assertion or fetches a new one if expired
func (gr *GroupResolver) getOrFreshAssertion() (string, error) {
	gr.mu.RLock()
	if gr.assertionCache != nil && gr.assertionCache.assertion != "" {
		if time.Now().Add(assertionCacheBufferTime).Before(gr.assertionCache.expiresAt) {
			log.Printf("GroupResolver: using cached assertion (expires in %v)", time.Until(gr.assertionCache.expiresAt))
			assertion := gr.assertionCache.assertion
			gr.mu.RUnlock()
			return assertion, nil
		}
	}
	gr.mu.RUnlock()

	gr.mu.Lock()
	defer gr.mu.Unlock()

	// Double-check cache in case another goroutine refreshed while waiting for write lock
	if gr.assertionCache != nil && gr.assertionCache.assertion != "" {
		if time.Now().Add(assertionCacheBufferTime).Before(gr.assertionCache.expiresAt) {
			log.Printf("GroupResolver: using cached assertion (expires in %v)", time.Until(gr.assertionCache.expiresAt))
			return gr.assertionCache.assertion, nil
		}
	}

	// Fetch new assertion
	assertion, expiresAt, err := gr.fetchAssertion()
	if err != nil {
		return "", err
	}

	gr.assertionCache = &AssertionCache{
		assertion: assertion,
		expiresAt: expiresAt,
	}

	log.Printf("GroupResolver: obtained fresh assertion (expires in %v)", time.Until(expiresAt))
	return assertion, nil
}

// fetchAssertion performs the IDP authentication flow to get an assertion
func (gr *GroupResolver) fetchAssertion() (string, time.Time, error) {
	// Step 1: Start authentication flow
	flowID, actionRef, err := gr.startAuthenticationFlow()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to start authentication flow: %w", err)
	}

	if actionRef != "" {
		gr.flowActionRef = actionRef
	}
	if gr.flowActionRef == "" {
		gr.flowActionRef = "action_001"
	}

	// Step 2: Execute flow with credentials
	payload := map[string]any{
		"flowId": flowID,
		"inputs": map[string]string{
			"username":              gr.systemUsername,
			"password":              gr.systemPassword,
			"requested_permissions": "system",
		},
		"action": gr.flowActionRef,
	}

	var result struct {
		Assertion string `json:"assertion"`
	}

	if err := gr.postJSON(gr.baseURL+"/flow/execute", payload, "", &result); err != nil {
		return "", time.Time{}, fmt.Errorf("flow execute failed: %w", err)
	}

	assertion := strings.TrimSpace(result.Assertion)
	if assertion == "" {
		return "", time.Time{}, fmt.Errorf("no assertion returned from IDP")
	}

	// Decode JWT to get expiry
	expiresAt, err := extractJWTExpiry(assertion)
	if err != nil {
		// If we can't decode, assume 1 hour expiry
		log.Printf("GroupResolver: could not decode JWT expiry, assuming 1 hour: %v", err)
		expiresAt = time.Now().Add(1 * time.Hour)
	}

	return assertion, expiresAt, nil
}

// startAuthenticationFlow initiates the authentication flow with the IDP
func (gr *GroupResolver) startAuthenticationFlow() (string, string, error) {
	type executeFlowResponse struct {
		FlowID string `json:"flowId"`
		Data   struct {
			Actions []struct {
				Ref string `json:"ref"`
			} `json:"actions"`
		} `json:"data"`
	}

	var result executeFlowResponse
	err := gr.postJSON(gr.baseURL+"/flow/execute", map[string]string{
		"applicationId": gr.applicationID,
		"flowType":      "AUTHENTICATION",
	}, "", &result)

	if err != nil {
		return "", "", err
	}

	if result.FlowID == "" {
		return "", "", fmt.Errorf("no flowId returned from IDP")
	}

	actionRef := ""
	if len(result.Data.Actions) > 0 {
		actionRef = strings.TrimSpace(result.Data.Actions[0].Ref)
	}

	return result.FlowID, actionRef, nil
}

// lookupGroupIDByName looks up a group ID by its name
func (gr *GroupResolver) lookupGroupIDByName(assertion, groupName string) (string, error) {
	type GroupResponse struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}

	type GroupsListResponse struct {
		Groups []GroupResponse `json:"groups"`
	}

	var result GroupsListResponse
	if err := gr.getJSON(gr.baseURL+"/groups", assertion, &result); err != nil {
		return "", fmt.Errorf("failed to fetch groups list: %w", err)
	}

	for _, group := range result.Groups {
		if group.Name == groupName {
			return group.ID, nil
		}
	}

	return "", fmt.Errorf("group '%s' not found", groupName)
}

// fetchGroupMembers fetches the members of a group
func (gr *GroupResolver) fetchGroupMembers(assertion, groupID string) ([]Member, error) {
	type MembersResponse struct {
		Members []Member `json:"members"`
	}

	var result MembersResponse
	endpoint := fmt.Sprintf("%s/groups/%s/members", gr.baseURL, groupID)
	if err := gr.getJSON(endpoint, assertion, &result); err != nil {
		return nil, err
	}

	return result.Members, nil
}

// resolveUserEmail resolves a user's UUID to their email address using domain
func (gr *GroupResolver) resolveUserEmail(userID, baseDomain string) (string, error) {
	// The user ID is typically a username or similar identifier
	// We construct the email as username@domain
	username := userID
	if baseDomain == "" {
		return "", fmt.Errorf("base domain is required to resolve user email")
	}
	return username + "@" + baseDomain, nil
}

// Member represents a group member
type Member struct {
	ID   string `json:"id"`
	Type string `json:"type"` // "user" or "group"
}

// postJSON performs a POST request with JSON payload
func (gr *GroupResolver) postJSON(endpoint string, payload any, assertion string, out any) error {
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

	resp, err := gr.httpClient.Do(req)
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

// getJSON performs a GET request with JSON response
func (gr *GroupResolver) getJSON(endpoint, assertion string, out any) error {
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return err
	}

	if assertion != "" {
		req.Header.Set("Authorization", "Bearer "+assertion)
	}

	resp, err := gr.httpClient.Do(req)
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

// buildHTTPClient creates an HTTP client with TLS configuration
func buildHTTPClient() *http.Client {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true, // #nosec G402 -- Required for internal auth server communication
	}

	return &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsConfig},
		Timeout:   10 * time.Second,
	}
}

// extractJWTExpiry extracts the expiry time from a JWT token
func extractJWTExpiry(token string) (time.Time, error) {
	// JWT format: header.payload.signature
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, fmt.Errorf("invalid JWT format")
	}

	// Decode payload (base64url encoded)
	payload := parts[1]

	// Decode the base64url payload
	decoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to decode JWT payload: %w", err)
	}

	var claims struct {
		Exp int64 `json:"exp"`
	}

	if err := json.Unmarshal(decoded, &claims); err != nil {
		return time.Time{}, fmt.Errorf("failed to parse JWT claims: %w", err)
	}

	if claims.Exp == 0 {
		return time.Time{}, fmt.Errorf("no exp claim in JWT")
	}

	return time.Unix(claims.Exp, 0), nil
}