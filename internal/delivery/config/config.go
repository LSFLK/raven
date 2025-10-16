package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

// Config holds the delivery service configuration
type Config struct {
	LMTP     LMTPConfig     `yaml:"lmtp"`
	Database DatabaseConfig `yaml:"database"`
	Delivery DeliveryConfig `yaml:"delivery"`
	Logging  LoggingConfig  `yaml:"logging"`
}

// LMTPConfig holds LMTP server configuration
type LMTPConfig struct {
	UnixSocket  string `yaml:"unix_socket"`
	TCPAddress  string `yaml:"tcp_address"`
	MaxSize     int64  `yaml:"max_size"`      // Maximum message size in bytes
	Timeout     int    `yaml:"timeout"`       // Connection timeout in seconds
	Hostname    string `yaml:"hostname"`      // Server hostname for LHLO
	MaxRecipients int  `yaml:"max_recipients"` // Maximum recipients per transaction
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Path string `yaml:"path"`
}

// DeliveryConfig holds delivery-specific configuration
type DeliveryConfig struct {
	DefaultFolder    string   `yaml:"default_folder"`     // Default delivery folder (usually INBOX)
	QuotaEnabled     bool     `yaml:"quota_enabled"`      // Enable quota checking
	QuotaLimit       int64    `yaml:"quota_limit"`        // Quota limit in bytes
	AllowedDomains   []string `yaml:"allowed_domains"`    // List of allowed recipient domains
	RejectUnknownUser bool    `yaml:"reject_unknown_user"` // Reject messages for unknown users
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string `yaml:"level"`  // log level: debug, info, warn, error
	Format string `yaml:"format"` // log format: text, json
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		LMTP: LMTPConfig{
			UnixSocket:    "/var/run/raven/lmtp.sock",
			TCPAddress:    "127.0.0.1:24",
			MaxSize:       52428800, // 50MB
			Timeout:       300,      // 5 minutes
			Hostname:      "localhost",
			MaxRecipients: 100,
		},
		Database: DatabaseConfig{
			Path: "data/mails.db",
		},
		Delivery: DeliveryConfig{
			DefaultFolder:     "INBOX",
			QuotaEnabled:      false,
			QuotaLimit:        1073741824, // 1GB
			AllowedDomains:    []string{},
			RejectUnknownUser: false,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
		},
	}
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(path string) (*Config, error) {
	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate LMTP config
	if c.LMTP.UnixSocket == "" && c.LMTP.TCPAddress == "" {
		return fmt.Errorf("at least one of unix_socket or tcp_address must be specified")
	}

	if c.LMTP.MaxSize <= 0 {
		return fmt.Errorf("max_size must be positive")
	}

	if c.LMTP.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}

	if c.LMTP.MaxRecipients <= 0 {
		return fmt.Errorf("max_recipients must be positive")
	}

	// Validate database config
	if c.Database.Path == "" {
		return fmt.Errorf("database path cannot be empty")
	}

	// Validate delivery config
	if c.Delivery.DefaultFolder == "" {
		return fmt.Errorf("default_folder cannot be empty")
	}

	if c.Delivery.QuotaEnabled && c.Delivery.QuotaLimit <= 0 {
		return fmt.Errorf("quota_limit must be positive when quota is enabled")
	}

	// Validate logging config
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[c.Logging.Level] {
		return fmt.Errorf("invalid log level: %s", c.Logging.Level)
	}

	validFormats := map[string]bool{"text": true, "json": true}
	if !validFormats[c.Logging.Format] {
		return fmt.Errorf("invalid log format: %s", c.Logging.Format)
	}

	return nil
}
