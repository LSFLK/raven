package sasl

import (
	"os"
	"path/filepath"
	"testing"

	"raven/internal/auth/oauthbearer"
)

func TestClientEmailAuthorizer_LoadAndAuthorize(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "client-email-authz.json")

	content := `{
  "client_id_1": ["email1@example.com", "email2@example.com"],
  "client_id_2": ["email3@example.com"]
}`
	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	authorizer, err := newClientEmailAuthorizerFromFile(configPath)
	if err != nil {
		t.Fatalf("failed to load authorizer: %v", err)
	}

	if !authorizer.IsEmailAuthorized("client_id_1", "email1@example.com") {
		t.Fatalf("expected email1@example.com to be authorized for client_id_1")
	}

	if !authorizer.IsEmailAuthorized("client_id_1", "EMAIL2@example.com") {
		t.Fatalf("expected case-insensitive email match for client_id_1")
	}

	if authorizer.IsEmailAuthorized("client_id_1", "email3@example.com") {
		t.Fatalf("did not expect email3@example.com to be authorized for client_id_1")
	}

	if authorizer.IsEmailAuthorized("missing_client", "email1@example.com") {
		t.Fatalf("did not expect authorization for unknown client_id")
	}
}

func TestAuthorizeOAuthClientCredentialsSender(t *testing.T) {
	authorizer := &clientEmailAuthorizer{
		allowedByClientID: map[string]map[string]struct{}{
			"client-a": {
				"sender@example.com": {},
			},
		},
	}

	s := &Server{
		domain:                    "example.com",
		oauthClientEmailAuthorizer: authorizer,
	}

	if err := s.authorizeOAuthClientCredentialsSender(oauthbearer.Claims{GrantType: "password"}, "", ""); err != nil {
		t.Fatalf("expected non-client_credentials grant to skip check, got error: %v", err)
	}

	if err := s.authorizeOAuthClientCredentialsSender(oauthbearer.Claims{GrantType: "client_credentials"}, "", "sender"); err == nil {
		t.Fatalf("expected error when client_id is missing")
	}

	if err := s.authorizeOAuthClientCredentialsSender(oauthbearer.Claims{GrantType: "client_credentials", ClientID: "client-a"}, "", ""); err == nil {
		t.Fatalf("expected error when sender email is missing")
	}

	s.oauthClientEmailAuthorizer = nil
	if err := s.authorizeOAuthClientCredentialsSender(oauthbearer.Claims{GrantType: "client_credentials", ClientID: "client-a"}, "", "sender@example.com"); err == nil {
		t.Fatalf("expected error when authorizer config is missing")
	}

	s.oauthClientEmailAuthorizer = authorizer
	if err := s.authorizeOAuthClientCredentialsSender(oauthbearer.Claims{GrantType: "client_credentials", ClientID: "client-a"}, "", "not-allowed@example.com"); err == nil {
		t.Fatalf("expected error for unauthorized sender email")
	}

	if err := s.authorizeOAuthClientCredentialsSender(oauthbearer.Claims{GrantType: "client_credentials", ClientID: "client-a"}, "", "sender@example.com"); err != nil {
		t.Fatalf("expected sender@example.com to be authorized: %v", err)
	}

	if err := s.authorizeOAuthClientCredentialsSender(oauthbearer.Claims{GrantType: "client_credentials", ClientID: "client-a"}, "", "sender"); err != nil {
		t.Fatalf("expected sender to be normalized with default domain and authorized: %v", err)
	}
}
