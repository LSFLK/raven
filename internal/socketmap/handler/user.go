package handler

import (
	"fmt"
	"log"
	"strings"
	"time"

	"raven/internal/socketmap/config"
	"raven/internal/socketmap/cache"
	"raven/internal/socketmap/thunder"
)

// UserExists checks if a user exists in Thunder IDP
func UserExists(email string, cfg *config.Config, cacheManager *cache.Cache) bool {
	log.Printf("    ┌─ User Lookup ───────────────────")
	log.Printf("    │ Email: %s", email)
	
	// Check cache first (read lock)
	cacheKey := "user:" + email
	entry, found := cacheManager.Get(cacheKey)
	
	now := time.Now()
	
	if found {
		// Cache hit - check if still valid
		if !cacheManager.IsExpired(entry) {
			log.Printf("    │ ✓ CACHE HIT (fresh)")
			log.Printf("    │ Cached result: exists=%v", entry.Exists)
			log.Printf("    │ Expires: %s", entry.Expires.Format("15:04:05"))
			log.Printf("    └─────────────────────────────────")
			return entry.Exists
		}
		
		// Cache expired - check if we should refresh
		cacheAge := now.Sub(entry.LastUpdate).Seconds()
		log.Printf("    │ ✓ CACHE HIT (stale)")
		log.Printf("    │ Age: %.0f seconds", cacheAge)
		log.Printf("    │ Refreshing from IDP...")
	} else {
		log.Printf("    │ ✗ CACHE MISS")
		log.Printf("    │ Querying IDP...")
	}

	// Query Thunder IDP for user validation first.
	exists, err := thunder.ValidateUser(email, cfg.ThunderHost, cfg.ThunderPort, cfg.TokenRefreshSeconds)
	if err != nil {
		log.Printf("    │ ⚠ User lookup failed: %v", err)
		exists = false
	}

	// Treat group addresses as mailbox identities in user-exists map.
	if !exists && strings.Contains(email, "@") {
		groupExists, groupErr := thunder.ValidateGroupAddress(email, cfg.ThunderHost, cfg.ThunderPort, cfg.TokenRefreshSeconds)
		if groupErr != nil {
			log.Printf("    │ ⚠ Group lookup failed: %v", groupErr)
		} else if groupExists {
			log.Printf("    │ ✓ Group found; treating as existing user")
			exists = true
		}
	}

	if !exists {
		log.Printf("    │ User/group not found - Thunder unavailable or no match")
	}

	log.Printf("    │ IDP result: exists=%v", exists)
	
	// Only cache positive results (exists=true)
	if exists {
		cacheManager.Set(cacheKey, cache.Entry{
			Exists:     true,
			Expires:    now.Add(cacheManager.GetTTL()),
			LastUpdate: now,
		})
		log.Printf("    │ ✓ Cached positive result for %d seconds", cfg.CacheTTLSeconds)
	} else {
		log.Printf("    │ ℹ Negative result NOT cached (will query IDP next time)")
	}
	
	log.Printf("    └─────────────────────────────────")

	return exists
}

// HandleUserExistsMap handles the user-exists map lookup
func HandleUserExistsMap(key string, cfg *config.Config, cacheManager *cache.Cache) string {
	log.Printf("  │ Checking if user exists...")
	exists := UserExists(key, cfg, cacheManager)

	if exists {
		// For virtual_mailbox_maps, Postfix expects a mailbox pathname
		mailboxPath := key
		
		log.Printf("  │ ✓ USER FOUND: %s", key)
		log.Printf("  │ Response: OK %s", mailboxPath)
		log.Printf("  └─────────────────────────────────────")
		return fmt.Sprintf("OK %s", mailboxPath)
	}

	log.Printf("  │ ✗ USER NOT FOUND: %s", key)
	log.Printf("  │ Response: NOTFOUND")
	log.Printf("  └─────────────────────────────────────")
	return "NOTFOUND"
}