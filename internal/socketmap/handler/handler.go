package handler

import (
	"log"
	"strings"

	"raven/internal/socketmap/cache"
	"raven/internal/socketmap/config"
)

// ProcessRequest processes a socketmap request and returns the response
func ProcessRequest(line string, cfg *config.Config, cacheManager *cache.Cache) string {
	log.Printf("  ┌─ Processing Request ─────────────────")
	log.Printf("  │ Raw input: %q", line)

	parts := strings.Fields(line)
	log.Printf("  │ Split into %d parts: %v", len(parts), parts)

	// Postfix socketmap protocol sends: <mapname> <key>
	if len(parts) != 2 {
		log.Printf("  │ ⚠ INVALID REQUEST FORMAT")
		log.Printf("  │ Expected: <mapname> <key>")
		log.Printf("  │ Got: %d parts", len(parts))
		log.Printf("  └─────────────────────────────────────")
		return "PERM invalid request format"
	}

	mapname := parts[0]
	key := parts[1]

	log.Printf("  │ Map:     %q", mapname)
	log.Printf("  │ Key:     %q", key)

	// Route to appropriate handler based on map name
	switch mapname {
	case "user-exists":
		return HandleUserExistsMap(key, cfg, cacheManager)
	case "virtual-domains":
		return HandleVirtualDomainsMap(key, cfg, cacheManager)
	case "virtual-aliases":
		return HandleVirtualAliasesMap(key, cfg, cacheManager)
	default:
		log.Printf("  │ ⚠ UNKNOWN MAP")
		log.Printf("  │ Supported maps: user-exists, virtual-domains, virtual-aliases")
		log.Printf("  │ Got: %q", mapname)
		log.Printf("  └─────────────────────────────────────")
		return "NOTFOUND"
	}
}
