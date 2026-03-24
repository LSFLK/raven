package config

import (
	"log"
	"os"
	"strconv"
)

// Config holds all configuration for the socketmap service
type Config struct {
	Host                string
	Port                string
	ThunderHost         string
	ThunderPort         string
	CacheTTLSeconds     int
	TokenRefreshSeconds int
}

// Load reads configuration from environment variables
func Load() *Config {
	return &Config{
		Host:                getEnv("SOCKETMAP_HOST", "127.0.0.1"),
		Port:                getEnv("SOCKETMAP_PORT", "9100"),
		ThunderHost:         getEnv("THUNDER_HOST", "thunder-server"),
		ThunderPort:         getEnv("THUNDER_PORT", "8090"),
		CacheTTLSeconds:     getEnvInt("CACHE_TTL_SECONDS", 300),      // 5 minutes default
		TokenRefreshSeconds: getEnvInt("TOKEN_REFRESH_SECONDS", 3300), // 55 minutes default
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		} else {
			log.Printf("Warning: could not parse env var %s as int. Using default %d.", key, defaultValue)
		}
	}
	return defaultValue
}
