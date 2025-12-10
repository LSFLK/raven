package conf

import (
	"testing"
)

// Unit test: Tests Config struct field assignment
func TestConfig_FieldAssignment(t *testing.T) {
	cfg := Config{
		Domain:        "example.com",
		AuthServerURL: "https://auth.example.com",
	}

	if cfg.Domain != "example.com" {
		t.Errorf("Expected domain 'example.com', got '%s'", cfg.Domain)
	}
	if cfg.AuthServerURL != "https://auth.example.com" {
		t.Errorf("Expected auth_server_url 'https://auth.example.com', got '%s'", cfg.AuthServerURL)
	}
}

// Unit test: Tests Config struct zero values
func TestConfig_ZeroValues(t *testing.T) {
	cfg := Config{}

	if cfg.Domain != "" {
		t.Errorf("Expected empty domain, got '%s'", cfg.Domain)
	}
	if cfg.AuthServerURL != "" {
		t.Errorf("Expected empty auth_server_url, got '%s'", cfg.AuthServerURL)
	}
}

// Unit test: Tests ParseConfig with valid complete YAML
func TestParseConfig_ValidCompleteYAML(t *testing.T) {
	yamlData := []byte(`domain: test.example.com
auth_server_url: https://auth.test.example.com
`)

	cfg, err := ParseConfig(yamlData)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if cfg == nil {
		t.Fatal("Expected config to be non-nil")
	}

	if cfg.Domain != "test.example.com" {
		t.Errorf("Expected domain 'test.example.com', got '%s'", cfg.Domain)
	}

	if cfg.AuthServerURL != "https://auth.test.example.com" {
		t.Errorf("Expected auth_server_url 'https://auth.test.example.com', got '%s'", cfg.AuthServerURL)
	}
}

// Unit test: Tests ParseConfig with invalid YAML syntax
func TestParseConfig_InvalidYAML(t *testing.T) {
	invalidYAML := []byte(`domain: test.example.com
auth_server_url: [invalid yaml structure
  missing closing bracket
`)

	_, err := ParseConfig(invalidYAML)
	if err == nil {
		t.Error("Expected error for invalid YAML, got nil")
	}
}

// Unit test: Tests ParseConfig with empty data
func TestParseConfig_EmptyData(t *testing.T) {
	emptyData := []byte("")

	cfg, err := ParseConfig(emptyData)
	if err != nil {
		t.Fatalf("Expected no error for empty data, got: %v", err)
	}

	if cfg == nil {
		t.Fatal("Expected config to be non-nil")
	}

	if cfg.Domain != "" {
		t.Errorf("Expected empty domain, got '%s'", cfg.Domain)
	}
	if cfg.AuthServerURL != "" {
		t.Errorf("Expected empty auth_server_url, got '%s'", cfg.AuthServerURL)
	}
}

// Unit test: Tests ParseConfig with partial configuration
func TestParseConfig_PartialConfig(t *testing.T) {
	yamlData := []byte(`domain: partial.example.com
`)

	cfg, err := ParseConfig(yamlData)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if cfg.Domain != "partial.example.com" {
		t.Errorf("Expected domain 'partial.example.com', got '%s'", cfg.Domain)
	}

	if cfg.AuthServerURL != "" {
		t.Errorf("Expected empty auth_server_url, got '%s'", cfg.AuthServerURL)
	}
}

// Unit test: Tests ParseConfig with only auth_server_url
func TestParseConfig_OnlyAuthServerURL(t *testing.T) {
	yamlData := []byte(`auth_server_url: https://auth.only.example.com
`)

	cfg, err := ParseConfig(yamlData)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if cfg.Domain != "" {
		t.Errorf("Expected empty domain, got '%s'", cfg.Domain)
	}

	if cfg.AuthServerURL != "https://auth.only.example.com" {
		t.Errorf("Expected auth_server_url 'https://auth.only.example.com', got '%s'", cfg.AuthServerURL)
	}
}

// Unit test: Tests ParseConfig with YAML comments
func TestParseConfig_WithComments(t *testing.T) {
	yamlData := []byte(`# This is a comment
domain: commented.example.com
# Another comment
auth_server_url: https://auth.commented.example.com
`)

	cfg, err := ParseConfig(yamlData)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if cfg.Domain != "commented.example.com" {
		t.Errorf("Expected domain 'commented.example.com', got '%s'", cfg.Domain)
	}

	if cfg.AuthServerURL != "https://auth.commented.example.com" {
		t.Errorf("Expected auth_server_url 'https://auth.commented.example.com', got '%s'", cfg.AuthServerURL)
	}
}

// Unit test: Tests ParseConfig with special characters in values
func TestParseConfig_SpecialCharacters(t *testing.T) {
	yamlData := []byte(`domain: "test-domain.example.com"
auth_server_url: "https://auth.example.com:8443/api/v1"
`)

	cfg, err := ParseConfig(yamlData)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if cfg.Domain != "test-domain.example.com" {
		t.Errorf("Expected domain 'test-domain.example.com', got '%s'", cfg.Domain)
	}

	if cfg.AuthServerURL != "https://auth.example.com:8443/api/v1" {
		t.Errorf("Expected auth_server_url 'https://auth.example.com:8443/api/v1', got '%s'", cfg.AuthServerURL)
	}
}

// Unit test: Tests ParseConfig with extra whitespace
func TestParseConfig_WhitespaceHandling(t *testing.T) {
	yamlData := []byte(`
domain:   whitespace.example.com
auth_server_url:   https://auth.whitespace.example.com

`)

	cfg, err := ParseConfig(yamlData)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// YAML should trim whitespace
	if cfg.Domain != "whitespace.example.com" {
		t.Errorf("Expected domain 'whitespace.example.com', got '%s'", cfg.Domain)
	}

	if cfg.AuthServerURL != "https://auth.whitespace.example.com" {
		t.Errorf("Expected auth_server_url 'https://auth.whitespace.example.com', got '%s'", cfg.AuthServerURL)
	}
}

// Unit test: Tests ParseConfig with case-sensitive YAML keys
func TestParseConfig_CaseSensitiveKeys(t *testing.T) {
	// Use uppercase keys (should not match lowercase struct tags)
	yamlData := []byte(`Domain: uppercase.example.com
Auth_Server_URL: https://auth.uppercase.example.com
`)

	cfg, err := ParseConfig(yamlData)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Keys with wrong case should not populate fields
	if cfg.Domain != "" {
		t.Errorf("Expected empty domain (case mismatch), got '%s'", cfg.Domain)
	}

	if cfg.AuthServerURL != "" {
		t.Errorf("Expected empty auth_server_url (case mismatch), got '%s'", cfg.AuthServerURL)
	}
}

// Unit test: Tests ParseConfig with correct lowercase keys
func TestParseConfig_CorrectCaseKeys(t *testing.T) {
	yamlData := []byte(`domain: correct.example.com
auth_server_url: https://auth.correct.example.com
`)

	cfg, err := ParseConfig(yamlData)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if cfg.Domain != "correct.example.com" {
		t.Errorf("Expected domain 'correct.example.com', got '%s'", cfg.Domain)
	}

	if cfg.AuthServerURL != "https://auth.correct.example.com" {
		t.Errorf("Expected auth_server_url 'https://auth.correct.example.com', got '%s'", cfg.AuthServerURL)
	}
}

// Unit test: Tests ParseConfig with malformed YAML (unclosed quotes)
func TestParseConfig_MalformedYAML_UnclosedQuotes(t *testing.T) {
	yamlData := []byte(`domain: "unclosed.example.com
auth_server_url: https://auth.example.com
`)

	_, err := ParseConfig(yamlData)
	if err == nil {
		t.Error("Expected error for malformed YAML with unclosed quotes, got nil")
	}
}

// Unit test: Tests ParseConfig with malformed YAML (invalid indentation)
func TestParseConfig_MalformedYAML_InvalidIndentation(t *testing.T) {
	yamlData := []byte(`domain: test.example.com
  auth_server_url: https://auth.example.com
`)

	_, err := ParseConfig(yamlData)
	if err == nil {
		t.Error("Expected error for malformed YAML with invalid indentation, got nil")
	}
}

// Unit test: Tests ParseConfig with nil data
func TestParseConfig_NilData(t *testing.T) {
	var nilData []byte = nil

	cfg, err := ParseConfig(nilData)
	if err != nil {
		t.Fatalf("Expected no error for nil data, got: %v", err)
	}

	if cfg == nil {
		t.Fatal("Expected config to be non-nil")
	}

	if cfg.Domain != "" {
		t.Errorf("Expected empty domain, got '%s'", cfg.Domain)
	}
	if cfg.AuthServerURL != "" {
		t.Errorf("Expected empty auth_server_url, got '%s'", cfg.AuthServerURL)
	}
}

// Unit test: Tests ParseConfig with URL containing query parameters
func TestParseConfig_URLWithQueryParams(t *testing.T) {
	yamlData := []byte(`domain: query.example.com
auth_server_url: https://auth.example.com/login?redirect=true&timeout=30
`)

	cfg, err := ParseConfig(yamlData)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	expectedURL := "https://auth.example.com/login?redirect=true&timeout=30"
	if cfg.AuthServerURL != expectedURL {
		t.Errorf("Expected auth_server_url '%s', got '%s'", expectedURL, cfg.AuthServerURL)
	}
}

// Unit test: Tests ParseConfig with multiline YAML string (using folded style)
func TestParseConfig_MultilineString(t *testing.T) {
	yamlData := []byte(`domain: >
  multi.example.com
auth_server_url: https://auth.example.com
`)

	cfg, err := ParseConfig(yamlData)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Folded style should convert to single line with trailing newline removed
	if cfg.Domain != "multi.example.com\n" {
		t.Errorf("Expected domain 'multi.example.com\\n', got '%s'", cfg.Domain)
	}
}

// Unit test: Tests ParseConfig with extra unknown fields (should be ignored)
func TestParseConfig_ExtraFields(t *testing.T) {
	yamlData := []byte(`domain: extra.example.com
auth_server_url: https://auth.example.com
unknown_field: this should be ignored
another_unknown: also ignored
`)

	cfg, err := ParseConfig(yamlData)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if cfg.Domain != "extra.example.com" {
		t.Errorf("Expected domain 'extra.example.com', got '%s'", cfg.Domain)
	}

	if cfg.AuthServerURL != "https://auth.example.com" {
		t.Errorf("Expected auth_server_url 'https://auth.example.com', got '%s'", cfg.AuthServerURL)
	}
}

// Unit test: Tests ParseConfig with numeric values treated as strings
func TestParseConfig_NumericAsString(t *testing.T) {
	yamlData := []byte(`domain: "12345"
auth_server_url: "67890"
`)

	cfg, err := ParseConfig(yamlData)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if cfg.Domain != "12345" {
		t.Errorf("Expected domain '12345', got '%s'", cfg.Domain)
	}

	if cfg.AuthServerURL != "67890" {
		t.Errorf("Expected auth_server_url '67890', got '%s'", cfg.AuthServerURL)
	}
}

// Unit test: Tests ParseConfig with boolean-like values treated as strings
func TestParseConfig_BooleanAsString(t *testing.T) {
	yamlData := []byte(`domain: "true"
auth_server_url: "false"
`)

	cfg, err := ParseConfig(yamlData)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if cfg.Domain != "true" {
		t.Errorf("Expected domain 'true', got '%s'", cfg.Domain)
	}

	if cfg.AuthServerURL != "false" {
		t.Errorf("Expected auth_server_url 'false', got '%s'", cfg.AuthServerURL)
	}
}

// Unit test: Tests ParseConfig with unicode characters
func TestParseConfig_UnicodeCharacters(t *testing.T) {
	yamlData := []byte(`domain: "域名.example.com"
auth_server_url: "https://认证.example.com/路径"
`)

	cfg, err := ParseConfig(yamlData)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if cfg.Domain != "域名.example.com" {
		t.Errorf("Expected domain '域名.example.com', got '%s'", cfg.Domain)
	}

	if cfg.AuthServerURL != "https://认证.example.com/路径" {
		t.Errorf("Expected auth_server_url 'https://认证.example.com/路径', got '%s'", cfg.AuthServerURL)
	}
}

// Unit test: Tests ParseConfig with escaped characters
func TestParseConfig_EscapedCharacters(t *testing.T) {
	yamlData := []byte(`domain: "test\nexample.com"
auth_server_url: "https://auth.example.com/path\twith\ttabs"
`)

	cfg, err := ParseConfig(yamlData)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	expectedDomain := "test\nexample.com"
	if cfg.Domain != expectedDomain {
		t.Errorf("Expected domain '%s', got '%s'", expectedDomain, cfg.Domain)
	}

	expectedURL := "https://auth.example.com/path\twith\ttabs"
	if cfg.AuthServerURL != expectedURL {
		t.Errorf("Expected auth_server_url '%s', got '%s'", expectedURL, cfg.AuthServerURL)
	}
}
