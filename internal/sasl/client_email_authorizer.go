package sasl

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type clientEmailAuthorizer struct {
	allowedByClientID map[string]map[string]struct{}
}

func newClientEmailAuthorizerFromFile(path string) (*clientEmailAuthorizer, error) {
	cleanPath := filepath.Clean(strings.TrimSpace(path))
	if cleanPath == "" {
		return nil, fmt.Errorf("empty client email authorization file path")
	}

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read client email authorization file: %w", err)
	}

	var raw map[string][]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse client email authorization file: %w", err)
	}

	a := &clientEmailAuthorizer{allowedByClientID: make(map[string]map[string]struct{}, len(raw))}
	totalAllowedEmails := 0
	for clientID, emails := range raw {
		normalizedClientID := strings.TrimSpace(clientID)
		if normalizedClientID == "" {
			log.Printf("SASL OAUTHBEARER: skipping empty client_id in authorization config file=%q", cleanPath)
			continue
		}

		allowed := make(map[string]struct{}, len(emails))
		for _, email := range emails {
			normalizedEmail := strings.ToLower(strings.TrimSpace(email))
			if normalizedEmail == "" {
				log.Printf("SASL OAUTHBEARER: skipping empty email for client_id=%q in authorization config", normalizedClientID)
				continue
			}
			allowed[normalizedEmail] = struct{}{}
			totalAllowedEmails++
		}

		a.allowedByClientID[normalizedClientID] = allowed
	}

	log.Printf("SASL OAUTHBEARER: client email authorization config loaded file=%q clients=%d allowed_emails=%d", cleanPath, len(a.allowedByClientID), totalAllowedEmails)

	return a, nil
}

func (a *clientEmailAuthorizer) IsEmailAuthorized(clientID, email string) bool {
	if a == nil {
		return false
	}

	normalizedClientID := strings.TrimSpace(clientID)
	normalizedEmail := strings.ToLower(strings.TrimSpace(email))
	if normalizedClientID == "" || normalizedEmail == "" {
		return false
	}

	allowedEmails, ok := a.allowedByClientID[normalizedClientID]
	if !ok {
		return false
	}

	_, exists := allowedEmails[normalizedEmail]
	return exists
}
