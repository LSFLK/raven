package helpers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// GetFixturesDir returns the path to the fixtures directory
func GetFixturesDir() string {
	// Get the current file's directory
	_, filename, _, _ := runtime.Caller(0)
	helpersDir := filepath.Dir(filename)
	testDir := filepath.Dir(helpersDir)
	fixturesDir := filepath.Join(testDir, "fixtures")
	return fixturesDir
}

// validateFixturePath ensures the file path is within the fixtures directory
// and doesn't contain any directory traversal attempts
func validateFixturePath(fixturesDir, filename string) (string, error) {
	// Clean the filename to remove any path traversal attempts
	cleanFilename := filepath.Clean(filename)

	// Check for directory traversal patterns
	if strings.Contains(cleanFilename, "..") || strings.HasPrefix(cleanFilename, "/") || strings.Contains(cleanFilename, "\\") {
		return "", fmt.Errorf("invalid filename: potential directory traversal detected in %s", filename)
	}

	// Build the full path
	fullPath := filepath.Join(fixturesDir, cleanFilename)

	// Ensure the resolved path is still within fixtures directory
	absFixturesDir, err := filepath.Abs(fixturesDir)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute fixtures directory: %v", err)
	}

	absFullPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute file path: %v", err)
	}

	if !strings.HasPrefix(absFullPath, absFixturesDir) {
		return "", fmt.Errorf("file path %s is outside fixtures directory %s", filename, fixturesDir)
	}

	return fullPath, nil
}

// LoadTestEmail loads a test email fixture by filename
func LoadTestEmail(t *testing.T, filename string) []byte {
	t.Helper()

	fixturesDir := GetFixturesDir()
	emailPath, err := validateFixturePath(fixturesDir, filename)
	if err != nil {
		t.Fatalf("Invalid fixture path for %s: %v", filename, err)
	}

	data, err := os.ReadFile(emailPath) // #nosec G304 - Path validated against directory traversal
	if err != nil {
		t.Fatalf("Failed to load test email %s: %v", filename, err)
	}

	t.Logf("Loaded test email: %s (%d bytes)", filename, len(data))
	return data
}

// LoadSimpleEmail loads the simple test email fixture
func LoadSimpleEmail(t *testing.T) []byte {
	return LoadTestEmail(t, "simple-email.eml")
}

// LoadMultipartEmail loads the multipart test email fixture
func LoadMultipartEmail(t *testing.T) []byte {
	return LoadTestEmail(t, "multipart-email.eml")
}

// LoadEmailWithAttachment loads the email with attachment fixture
func LoadEmailWithAttachment(t *testing.T) []byte {
	return LoadTestEmail(t, "email-with-attachment.eml")
}

// LoadHTMLEmail loads the HTML test email fixture
func LoadHTMLEmail(t *testing.T) []byte {
	return LoadTestEmail(t, "html-email.eml")
}

// LoadMultiRecipientEmail loads the multi-recipient test email fixture
func LoadMultiRecipientEmail(t *testing.T) []byte {
	return LoadTestEmail(t, "multi-recipient-email.eml")
}

// LoadUnicodeEmail loads the Unicode and emoji test email fixture
func LoadUnicodeEmail(t *testing.T) []byte {
	return LoadTestEmail(t, "unicode-email.eml")
}

// LoadLargeEmail loads the large test email fixture for performance testing
func LoadLargeEmail(t *testing.T) []byte {
	return LoadTestEmail(t, "large-email.eml")
}

// LoadTestUsers loads the test user configuration
func LoadTestUsers(t *testing.T) map[string]interface{} {
	t.Helper()

	fixturesDir := GetFixturesDir()
	configPath, err := validateFixturePath(fixturesDir, "test-users.json")
	if err != nil {
		t.Fatalf("Invalid fixture path for test-users.json: %v", err)
	}

	data, err := os.ReadFile(configPath) // #nosec G304 - Path validated against directory traversal
	if err != nil {
		t.Fatalf("Failed to load test users config: %v", err)
	}

	var config map[string]interface{}
	err = json.Unmarshal(data, &config)
	if err != nil {
		t.Fatalf("Failed to parse test users config: %v", err)
	}

	t.Logf("Loaded test users configuration with %d domains", len(config["domains"].([]interface{})))
	return config
}

// LoadMailboxStructures loads the mailbox structure configuration
func LoadMailboxStructures(t *testing.T) map[string]interface{} {
	t.Helper()

	fixturesDir := GetFixturesDir()
	configPath, err := validateFixturePath(fixturesDir, "mailbox-structures.json")
	if err != nil {
		t.Fatalf("Invalid fixture path for mailbox-structures.json: %v", err)
	}

	data, err := os.ReadFile(configPath) // #nosec G304 - Path validated against directory traversal
	if err != nil {
		t.Fatalf("Failed to load mailbox structures config: %v", err)
	}

	var config map[string]interface{}
	err = json.Unmarshal(data, &config)
	if err != nil {
		t.Fatalf("Failed to parse mailbox structures config: %v", err)
	}

	t.Logf("Loaded mailbox structures configuration")
	return config
}

// LoadTestConfig loads the test configuration YAML
func LoadTestConfig(t *testing.T) []byte {
	t.Helper()

	fixturesDir := GetFixturesDir()
	configPath, err := validateFixturePath(fixturesDir, "test-config.yaml")
	if err != nil {
		t.Fatalf("Invalid fixture path for test-config.yaml: %v", err)
	}

	data, err := os.ReadFile(configPath) // #nosec G304 - Path validated against directory traversal
	if err != nil {
		t.Fatalf("Failed to load test config: %v", err)
	}

	t.Logf("Loaded test configuration (%d bytes)", len(data))
	return data
}
