package thunder

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"
	"sync"
)

// sanitizeDomainPart validates and sanitizes a domain part to prevent path traversal attacks
func sanitizeDomainPart(part string) error {
	// Check for empty parts
	if part == "" {
		return fmt.Errorf("empty domain part")
	}

	// Check for path traversal characters
	if strings.Contains(part, "..") || strings.Contains(part, "/") || strings.Contains(part, "\\") {
		return fmt.Errorf("invalid characters in domain part: %s", part)
	}

	// Validate that domain part contains only valid characters (alphanumeric, hyphens, underscores)
	for _, char := range part {
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_') {
			return fmt.Errorf("invalid character in domain part: %c", char)
		}
	}

	return nil
}

// buildOUPath constructs an OU path from a domain, validating all parts
func buildOUPath(domain string) (string, error) {
	// Parse domain into parts
	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid domain format: minimum 2 parts required")
	}

	// Validate all domain parts to prevent path traversal attacks
	for _, part := range parts {
		if err := sanitizeDomainPart(part); err != nil {
			return "", fmt.Errorf("invalid domain part: %v", err)
		}
	}

	// Build OU path
	// So we need to reverse the subdomain parts
	var ouPath string
	if len(parts) == 2 {
		ouPath = domain
	} else {
		rootDomain := strings.Join(parts[len(parts)-2:], ".")
		subdomains := parts[:len(parts)-2]

		// Reverse the subdomain parts
		slices.Reverse(subdomains)

		ouPath = rootDomain + "/" + strings.Join(subdomains, "/")
	}

	return ouPath, nil
}

// Cache for OU ID lookups (domain -> OU ID mapping)
var (
	ouCache      = make(map[string]string)
	ouCacheMutex sync.RWMutex
)

// ValidateDomain checks if a domain exists in Thunder IDP
func ValidateDomain(domain, host, port string, tokenRefreshSeconds int) (bool, error) {
	log.Printf("      ┌─ Thunder Domain Validation ──")
	log.Printf("      │ Domain: %s", domain)

	// Get authentication token
	auth, err := GetAuth(host, port, tokenRefreshSeconds)
	if err != nil {
		log.Printf("      │ ⚠ Auth failed: %v", err)
		log.Printf("      └──────────────────────────────")
		return false, err
	}

	// Build OU path
	ouPath, err := buildOUPath(domain)
	if err != nil {
		log.Printf("      │ ✗ Invalid domain: %v", err)
		log.Printf("      └──────────────────────────────")
		return false, nil
	}

	log.Printf("      │ OU Path: %s", ouPath)

	// Query Thunder API
	client := GetHTTPClient()
	url := fmt.Sprintf("https://%s:%s/organization-units/tree/%s", host, port, ouPath)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("      │ ✗ Failed to create request: %v", err)
		log.Printf("      └──────────────────────────────")
		return false, err
	}

	req.Header.Set("Authorization", "Bearer "+auth.BearerToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("      │ ✗ Request failed: %v", err)
		log.Printf("      └──────────────────────────────")
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		log.Printf("      │ ✗ Domain not found in Thunder")
		log.Printf("      └──────────────────────────────")
		return false, nil
	}

	if resp.StatusCode != 200 {
		log.Printf("      │ ⚠ Unexpected status: %d", resp.StatusCode)
		log.Printf("      └──────────────────────────────")
		return false, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var ouResp OrgUnitResponse
	if err := json.NewDecoder(resp.Body).Decode(&ouResp); err != nil {
		log.Printf("      │ ✗ Failed to parse response: %v", err)
		log.Printf("      └──────────────────────────────")
		return false, err
	}

	log.Printf("      │ ✓ Domain found in Thunder")
	log.Printf("      │ OU ID: %s", ouResp.ID)
	log.Printf("      │ OU Name: %s", ouResp.Name)
	log.Printf("      └──────────────────────────────")

	// Cache the OU ID for future user lookups
	ouCacheMutex.Lock()
	ouCache[domain] = ouResp.ID
	ouCacheMutex.Unlock()

	return true, nil
}

// GetOrgUnitIDForDomain retrieves the OU ID for a domain from Thunder or cache
func GetOrgUnitIDForDomain(domain, host, port string, tokenRefreshSeconds int) (string, error) {
	// Check cache first
	ouCacheMutex.RLock()
	ouID, found := ouCache[domain]
	ouCacheMutex.RUnlock()

	if found {
		return ouID, nil
	}

	// Need to query Thunder to get OU ID
	log.Printf("      │ Fetching OU ID for domain: %s", domain)

	// Get authentication token
	auth, err := GetAuth(host, port, tokenRefreshSeconds)
	if err != nil {
		return "", err
	}

	// Build OU path
	ouPath, err := buildOUPath(domain)
	if err != nil {
		return "", err
	}

	// Query Thunder API for OU
	client := GetHTTPClient()
	url := fmt.Sprintf("https://%s:%s/organization-units/tree/%s", host, port, ouPath)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+auth.BearerToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("OU not found or error: %d", resp.StatusCode)
	}

	var ouResp OrgUnitResponse
	if err := json.NewDecoder(resp.Body).Decode(&ouResp); err != nil {
		return "", err
	}

	// Cache the result
	ouCacheMutex.Lock()
	ouCache[domain] = ouResp.ID
	ouCacheMutex.Unlock()

	log.Printf("      │ ✓ OU ID cached: %s", ouResp.ID)

	return ouResp.ID, nil
}
