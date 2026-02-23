package conf

import (
	"fmt"
	"os"
	"path/filepath"
	"raven/internal/blobstorage"

	"gopkg.in/yaml.v2"
)

// SASLScope defines where SASL authentication should be applied
type SASLScope string

const (
	// SASLScopeTCPOnly applies SASL authentication only on TCP connections
	SASLScopeTCPOnly SASLScope = "tcp_only"
	// SASLScopeUnixSocketOnly applies SASL authentication only on Unix domain sockets
	SASLScopeUnixSocketOnly SASLScope = "unix_socket_only"
	// SASLScopeAll applies SASL authentication on all connection types (default)
	SASLScopeAll SASLScope = "all"
)

type Config struct {
	Domain        string             `yaml:"domain"`
	AuthServerURL string             `yaml:"auth_server_url"`
	SASLScope     SASLScope          `yaml:"sasl_scope"`
	BlobStorage   blobstorage.Config `yaml:"blob_storage"`
}

func LoadConfig() (*Config, error) {
	var cfg Config

	// Try multiple possible paths
	configPaths := []string{
		"/etc/raven/raven.yaml",
		"./config/raven.yaml",
		"./raven.yaml",
		"config/raven.yaml",
	}

	var data []byte
	var err error
	for _, path := range configPaths {
		data, err = os.ReadFile(filepath.Clean(path))
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Set defaults
	cfg.SetDefaults()

	return &cfg, nil
}

// SetDefaults sets default values for configuration
func (c *Config) SetDefaults() {
	if c.SASLScope == "" {
		c.SASLScope = SASLScopeAll
	}
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Domain == "" {
		return fmt.Errorf("domain is required")
	}
	if c.AuthServerURL == "" {
		return fmt.Errorf("auth_server_url is required")
	}

	// Validate SASL scope
	switch c.SASLScope {
	case SASLScopeTCPOnly, SASLScopeUnixSocketOnly, SASLScopeAll:
		// Valid scope
	default:
		return fmt.Errorf("invalid sasl_scope: %s (must be 'tcp_only', 'unix_socket_only', or 'all')", c.SASLScope)
	}

	return nil
}
