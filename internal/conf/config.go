package conf

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"raven/internal/blobstorage"
	"regexp"
	"strings"

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

// GetApplicationID retrieves the application ID from environment variables or thunder container logs
// It tries the following sources in order:
// 1. THUNDER_DEVELOP_APP_ID environment variable
// 2. APPLICATION_ID environment variable
// 3. applicationId environment variable
// 4. Extracted from thunder-setup container logs as fallback
func GetApplicationID() (string, error) {
	// Try THUNDER_DEVELOP_APP_ID first (socketmap develop environment)
	if appID := strings.TrimSpace(os.Getenv("THUNDER_DEVELOP_APP_ID")); appID != "" {
		log.Printf("Using Application ID from THUNDER_DEVELOP_APP_ID environment variable")
		return appID, nil
	}

	// Try APPLICATION_ID
	if appID := strings.TrimSpace(os.Getenv("APPLICATION_ID")); appID != "" {
		log.Printf("Using Application ID from APPLICATION_ID environment variable")
		return appID, nil
	}

	// Try applicationId
	if appID := strings.TrimSpace(os.Getenv("applicationId")); appID != "" {
		log.Printf("Using Application ID from applicationId environment variable")
		return appID, nil
	}

	// Fall back to extracting from thunder container logs
	log.Printf("Application ID not set in environment variables, attempting to extract from thunder-setup container logs...")
	appID, err := extractApplicationIDFromThunderLogs()
	if err != nil {
		return "", fmt.Errorf("failed to get Application ID: %w", err)
	}

	return appID, nil
}

// extractApplicationIDFromThunderLogs extracts the Application ID from thunder-setup container logs
func extractApplicationIDFromThunderLogs() (string, error) {
	log.Printf("Extracting Application ID from thunder-setup container logs...")

	// Execute: docker logs thunder-setup 2>&1
	cmd := exec.Command("docker", "logs", "thunder-setup")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if docker command doesn't exist
		if strings.Contains(err.Error(), "executable file not found") {
			return "", fmt.Errorf("docker command not available in PATH")
		}
		// Docker command exists but failed - might be permission issue
		log.Printf("⚠ Warning: docker logs command failed: %v", err)
		log.Printf("This might be due to:")
		log.Printf("  - thunder-setup container doesn't exist")
		log.Printf("  - No permission to access Docker")
		log.Printf("  - Running inside a container without Docker socket")
		return "", fmt.Errorf("docker logs failed: %w", err)
	}

	// Search for "DEVELOP_APP_ID:" in logs
	// Log format: [INFO] DEVELOP_APP_ID: 019cdc47-3537-74ee-951e-3f50e48786ab
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		// Look for line containing DEVELOP_APP_ID (case-insensitive)
		if strings.Contains(line, "DEVELOP_APP_ID") || strings.Contains(line, "develop_app_id") {
			// Extract UUID pattern: [a-f0-9-]{36}
			re := regexp.MustCompile(`[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}`)
			match := re.FindString(line)
			if match != "" {
				log.Printf("✓ Application ID extracted from thunder logs: %s", match)
				return match, nil
			}
		}
	}

	return "", fmt.Errorf("Application ID not found in thunder-setup container logs")
}
