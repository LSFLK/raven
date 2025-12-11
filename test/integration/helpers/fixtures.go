package helpers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// GetFixturesDir returns the path to the fixtures directory
func GetFixturesDir() string {
	// Get the current file's directory
	_, filename, _, _ := runtime.Caller(0)
	helpersDir := filepath.Dir(filename)
	integrationDir := filepath.Dir(helpersDir)
	fixturesDir := filepath.Join(integrationDir, "fixtures")
	return fixturesDir
}

// LoadTestEmail loads a test email fixture by filename
func LoadTestEmail(t *testing.T, filename string) []byte {
	t.Helper()

	fixturesDir := GetFixturesDir()
	emailPath := filepath.Join(fixturesDir, filename)

	data, err := os.ReadFile(emailPath)
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
	configPath := filepath.Join(fixturesDir, "test-users.json")

	data, err := os.ReadFile(configPath)
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
	configPath := filepath.Join(fixturesDir, "mailbox-structures.json")

	data, err := os.ReadFile(configPath)
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
	configPath := filepath.Join(fixturesDir, "test-config.yaml")

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to load test config: %v", err)
	}

	t.Logf("Loaded test configuration (%d bytes)", len(data))
	return data
}
