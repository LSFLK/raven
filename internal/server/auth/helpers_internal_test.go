package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResolveMailboxEmailPriority(t *testing.T) {
	tests := []struct {
		name          string
		loginIdentity string
		authID        string
		domain        string
		want          string
	}{
		{
			name:          "login email wins",
			loginIdentity: " user@example.com ",
			authID:        "id@example.net",
			domain:        "silver.example.com",
			want:          "user@example.com",
		},
		{
			name:          "auth id email used",
			loginIdentity: "user2",
			authID:        "user2@silver.example.com",
			domain:        "example.com",
			want:          "user2@silver.example.com",
		},
		{
			name:          "derived domain fallback",
			loginIdentity: "user2",
			authID:        "019cf0a6-114a",
			domain:        "silver.example.com",
			want:          "user2@silver.example.com",
		},
		{
			name:          "empty when nothing resolvable",
			loginIdentity: "",
			authID:        "019cf0a6-114a",
			domain:        "",
			want:          "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveMailboxEmail(tc.loginIdentity, tc.authID, tc.domain)
			if got != tc.want {
				t.Fatalf("resolveMailboxEmail() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNormalizeEmail(t *testing.T) {
	if got := normalizeEmail(" user@example.com. "); got != "user@example.com" {
		t.Fatalf("normalizeEmail() = %q, want %q", got, "user@example.com")
	}

	for _, input := range []string{"", "user", "@example.com", "user@", "a@b@c"} {
		if got := normalizeEmail(input); got != "" {
			t.Fatalf("normalizeEmail(%q) = %q, want empty", input, got)
		}
	}
}

func TestExtractBaseURL(t *testing.T) {
	base, err := extractBaseURL("https://example.com/auth/credentials/authenticate")
	if err != nil {
		t.Fatalf("extractBaseURL() unexpected error: %v", err)
	}
	if base != "https://example.com" {
		t.Fatalf("extractBaseURL() = %q, want %q", base, "https://example.com")
	}

	if _, err := extractBaseURL("not a url"); err == nil {
		t.Fatal("extractBaseURL() expected error for invalid URL")
	}
}

func TestStartAuthenticationFlowExecuteBehavior(t *testing.T) {
	t.Setenv("IDP_FLOW_ACTION", "")
	t.Setenv("idp_flow_action", "")

	t.Run("execute returns action ref", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/flow/execute" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"flowId":"f-1","data":{"actions":[{"ref":"action_123"}]}}`))
		}))
		defer srv.Close()

		flowID, action := startAuthenticationFlow(srv.URL, "app-1")
		if flowID != "f-1" || action != "action_123" {
			t.Fatalf("startAuthenticationFlow() = (%q,%q), want (%q,%q)", flowID, action, "f-1", "action_123")
		}
	})

	t.Run("returns empty when execute fails", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		flowID, action := startAuthenticationFlow(srv.URL, "app-1")
		if flowID != "" || action != "" {
			t.Fatalf("startAuthenticationFlow() = (%q,%q), want empty results", flowID, action)
		}
	})
}

func TestResolveOrganizationUnitDomainAndCycle(t *testing.T) {
	t.Run("hierarchy builds subdomain", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			switch r.URL.Path {
			case "/organization-units/ou-child":
				_, _ = w.Write([]byte(`{"id":"ou-child","handle":"silver","parent":"ou-root"}`))
			case "/organization-units/ou-root":
				_, _ = w.Write([]byte(`{"id":"ou-root","handle":"example.com","parent":null}`))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		domain, err := resolveOrganizationUnitDomain(srv.URL, "ou-child", "token")
		if err != nil {
			t.Fatalf("resolveOrganizationUnitDomain() unexpected error: %v", err)
		}
		if domain != "silver.example.com" {
			t.Fatalf("domain = %q, want %q", domain, "silver.example.com")
		}
	})

	t.Run("cycle detected", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"ou-a","handle":"a","parent":"ou-a"}`))
		}))
		defer srv.Close()

		_, err := resolveOrganizationUnitDomain(srv.URL, "ou-a", "")
		if err == nil || !strings.Contains(err.Error(), "cycle detected") {
			t.Fatalf("expected cycle detected error, got: %v", err)
		}
	})
}

func TestBuildAuthHTTPClient(t *testing.T) {
	client := buildAuthHTTPClient()
	if client == nil || client.Transport == nil {
		t.Fatal("buildAuthHTTPClient() returned nil client or transport")
	}
}
