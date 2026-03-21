package handler

import (
	"fmt"
	"log"
	"strings"
	"time"

	"raven/internal/socketmap/cache"
	"raven/internal/socketmap/config"
)

// ResolveAlias checks if an alias exists and returns its destination
func ResolveAlias(address string, cfg *config.Config, cacheManager *cache.Cache) string {
	log.Printf("    ┌─ Alias Lookup ──────────────────")
	log.Printf("    │ Address: %s", address)

	// Check cache first (read lock)
	cacheKey := "alias:" + address
	entry, found := cacheManager.Get(cacheKey)

	now := time.Now()

	if found {
		// Cache hit - check if still valid
		if !cacheManager.IsExpired(entry) {
			log.Printf("    │ ✓ CACHE HIT (fresh)")
			log.Printf("    │ Destination: %s", entry.Data)
			log.Printf("    │ Expires: %s", entry.Expires.Format("15:04:05"))
			log.Printf("    └─────────────────────────────────")
			return entry.Data
		}

		// Cache expired - refresh
		cacheAge := now.Sub(entry.LastUpdate).Seconds()
		log.Printf("    │ ✓ CACHE HIT (stale)")
		log.Printf("    │ Age: %.0f seconds", cacheAge)
		log.Printf("    │ Refreshing from database...")
	} else {
		log.Printf("    │ ✗ CACHE MISS")
		log.Printf("    │ Querying alias database...")
	}

	// Query database for alias
	destination := checkAliasInTestDB(address)

	log.Printf("    │ Database result: destination=%s", destination)

	// Update cache (write lock)
	cacheManager.Set(cacheKey, cache.Entry{
		Exists:     destination != "",
		Data:       destination,
		Expires:    now.Add(cacheManager.GetTTL()),
		LastUpdate: now,
	})

	log.Printf("    │ Cached for %d seconds", cfg.CacheTTLSeconds)
	log.Printf("    └─────────────────────────────────")

	return destination
}

// checkAliasInTestDB queries the test alias database
func checkAliasInTestDB(address string) string {
	log.Printf("      ┌─ Test Alias DB Lookup ───────")
	log.Printf("      │ Checking: %s", address)

	// Parse address to get domain
	parts := strings.Split(strings.ToLower(address), "@")
	if len(parts) != 2 {
		log.Printf("      │ ✗ Invalid email format")
		log.Printf("      └──────────────────────────────")
		return ""
	}

	localPart := parts[0]
	domain := parts[1]

	// Only handle postmaster@domain → admin@domain
	if localPart == "postmaster" {
		destination := fmt.Sprintf("admin@%s", domain)
		log.Printf("      │ ✓ Postmaster alias: %s → %s", address, destination)
		log.Printf("      └──────────────────────────────")
		return destination
	}

	log.Printf("      │ ✗ No alias found (only postmaster@domain supported)")
	log.Printf("      └──────────────────────────────")
	return ""
}

// HandleVirtualAliasesMap handles the virtual-aliases map lookup
func HandleVirtualAliasesMap(address string, cfg *config.Config, cacheManager *cache.Cache) string {
	log.Printf("  │ Checking if alias exists...")
	destination := ResolveAlias(address, cfg, cacheManager)

	if destination != "" {
		log.Printf("  │ ✓ ALIAS FOUND: %s -> %s", address, destination)
		log.Printf("  │ Response: OK %s", destination)
		log.Printf("  └─────────────────────────────────────")
		return fmt.Sprintf("OK %s", destination)
	}

	log.Printf("  │ ✗ ALIAS NOT FOUND: %s", address)
	log.Printf("  │ Response: NOTFOUND")
	log.Printf("  └─────────────────────────────────────")
	return "NOTFOUND"
}
