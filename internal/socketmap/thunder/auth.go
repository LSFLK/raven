package thunder

import (
	"fmt"
	"log"
	"sync"
	"time"

	"raven/internal/idp"
)

var (
	thunderAuth      *Auth
	thunderAuthMutex sync.RWMutex
)

// Authenticate obtains a system access token from Thunder for raven's service account.
//
// tokenRefreshSeconds caps how long the token is cached. Thunder's own expiry is honoured as
// well and whichever comes first wins, so a short-lived token is never held past its validity.
func Authenticate(host, port string, tokenRefreshSeconds int) (*Auth, error) {
	log.Printf("  ┌─ Thunder Authentication ─────────")

	baseURL := fmt.Sprintf("https://%s:%s", host, port)

	token, expiresAt, err := idp.SystemToken(baseURL)
	if err != nil {
		log.Printf("  │ ✗ Failed to obtain system token: %v", err)
		log.Printf("  │")
		log.Printf("  │ Raven authenticates as a service account. Check that:")
		log.Printf("  │ 1. IDP_CLIENT_ID and IDP_CLIENT_SECRET are both set")
		log.Printf("  │ 2. That client exists in Thunder as a machine-to-machine application")
		log.Printf("  │ 3. A role granting the 'system' permission is assigned to it")
		log.Printf("  └───────────────────────────────────")
		return nil, fmt.Errorf("failed to obtain system token: %w", err)
	}

	refreshAt := time.Now().Add(time.Duration(tokenRefreshSeconds) * time.Second)
	if expiresAt.Before(refreshAt) {
		refreshAt = expiresAt
	}

	log.Printf("  │ ✓ Authentication successful (expires in %v)", time.Until(expiresAt).Round(time.Second))
	log.Printf("  └───────────────────────────────────")

	return &Auth{
		BearerToken: token,
		ExpiresAt:   refreshAt,
		LastRefresh: time.Now(),
	}, nil
}

// GetAuth returns a valid Thunder auth token, refreshing if needed
func GetAuth(host, port string, tokenRefreshSeconds int) (*Auth, error) {
	thunderAuthMutex.RLock()
	auth := thunderAuth
	thunderAuthMutex.RUnlock()

	// Check if we have a valid token
	if auth != nil && time.Now().Before(auth.ExpiresAt) {
		return auth, nil
	}

	// Need to authenticate or refresh
	thunderAuthMutex.Lock()
	defer thunderAuthMutex.Unlock()

	// Double-check after acquiring write lock
	if thunderAuth != nil && time.Now().Before(thunderAuth.ExpiresAt) {
		return thunderAuth, nil
	}

	// Authenticate
	newAuth, err := Authenticate(host, port, tokenRefreshSeconds)
	if err != nil {
		return nil, err
	}

	thunderAuth = newAuth
	return thunderAuth, nil
}

// SetAuth sets the global auth state (for initialization)
func SetAuth(auth *Auth) {
	thunderAuthMutex.Lock()
	defer thunderAuthMutex.Unlock()
	thunderAuth = auth
}
