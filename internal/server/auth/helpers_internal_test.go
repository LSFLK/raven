package auth

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"raven/internal/models"
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

func TestGetEnvOrDefault(t *testing.T) {
	t.Setenv("AUTH_TEST_ENV", "")
	if got := getEnvOrDefault("AUTH_TEST_ENV", "fallback"); got != "fallback" {
		t.Fatalf("getEnvOrDefault() = %q, want %q", got, "fallback")
	}

	t.Setenv("AUTH_TEST_ENV", " configured ")
	if got := getEnvOrDefault("AUTH_TEST_ENV", "fallback"); got != "configured" {
		t.Fatalf("getEnvOrDefault() = %q, want %q", got, "configured")
	}
}

func TestPostJSONAndGetJSON_ErrorAndSuccess(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"value":"done"}`))
		case "/empty":
			w.WriteHeader(http.StatusNoContent)
		case "/bad":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("bad request"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	var out struct {
		Value string `json:"value"`
	}
	if err := postJSON(srv.URL+"/ok", map[string]string{"k": "v"}, "assertion", &out); err != nil {
		t.Fatalf("postJSON() unexpected error: %v", err)
	}
	if out.Value != "done" {
		t.Fatalf("postJSON() decoded value = %q, want %q", out.Value, "done")
	}

	if err := postJSON(srv.URL+"/empty", map[string]string{"k": "v"}, "", nil); err != nil {
		t.Fatalf("postJSON() with nil out unexpected error: %v", err)
	}

	if err := postJSON(srv.URL+"/bad", map[string]string{"k": "v"}, "", &out); err == nil {
		t.Fatal("postJSON() expected error for non-2xx response")
	}

	if err := getJSON(srv.URL+"/ok", "", &out); err != nil {
		t.Fatalf("getJSON() unexpected error: %v", err)
	}
	if out.Value != "done" {
		t.Fatalf("getJSON() decoded value = %q, want %q", out.Value, "done")
	}

	if err := getJSON(srv.URL+"/bad", "", &out); err == nil {
		t.Fatal("getJSON() expected error for non-2xx response")
	}
}

func TestResolveDomainFromOrganizationUnit_AdditionalBranches(t *testing.T) {
	if got := resolveDomainFromOrganizationUnit("not a url", "ou-1", "user", "pass"); got != "" {
		t.Fatalf("expected empty result for invalid auth URL, got %q", got)
	}

	t.Setenv("THUNDER_DEVELOP_APP_ID", "app-1")
	t.Setenv("IDP_SYSTEM_USERNAME", "admin")
	t.Setenv("IDP_SYSTEM_PASSWORD", "admin")
	t.Setenv("IDP_FLOW_ACTION", "")
	t.Setenv("idp_flow_action", "")

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/flow/execute":
			defer func() { _ = r.Body.Close() }()
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte("invalid json"))
				return
			}

			if _, hasFlowType := payload["flowType"]; hasFlowType {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"flowId":"flow-1","data":{"actions":[{"ref":"action_1"}]}}`))
				return
			}

			inputs, _ := payload["inputs"].(map[string]any)
			username, _ := inputs["username"].(string)
			if username == "admin" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte("unauthorized"))
				return
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"assertion":"assertion-user"}`))
		case "/organization-units/ou-child":
			if r.Header.Get("Authorization") != "Bearer assertion-user" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"ou-child","handle":"silver","parent":"ou-root"}`))
		case "/organization-units/ou-root":
			if r.Header.Get("Authorization") != "Bearer assertion-user" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"ou-root","handle":"example.com","parent":null}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprintf(w, "not found: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	baseURL, err := extractBaseURL(srv.URL + "/auth/credentials/authenticate")
	if err != nil {
		t.Fatalf("extractBaseURL() unexpected error: %v", err)
	}
	if baseURL != srv.URL {
		t.Fatalf("extractBaseURL() = %q, want %q", baseURL, srv.URL)
	}

	if got := resolveDomainFromOrganizationUnit(srv.URL+"/auth/credentials/authenticate", "ou-child", "user2", "pass"); got != "silver.example.com" {
		t.Fatalf("resolveDomainFromOrganizationUnit() = %q, want %q", got, "silver.example.com")
	}
}

func TestHandleSSLConnection_LoadCertFailureClosesConn(t *testing.T) {
	conn := &sslTrackingConn{}

	called := false
	HandleSSLConnection(func(_ net.Conn, _ *models.ClientState) {
		called = true
	}, conn)

	if called {
		t.Fatal("expected client handler not to be called on cert load failure")
	}
	if !conn.closed {
		t.Fatal("expected connection to be closed on cert load failure")
	}
}

func TestFetchAssertionBranches(t *testing.T) {
	t.Run("returns empty when app id unavailable", func(t *testing.T) {
		t.Setenv("THUNDER_DEVELOP_APP_ID", "")
		t.Setenv("APPLICATION_ID", "")
		t.Setenv("applicationId", "")

		tmpDir := t.TempDir()
		oldWD, err := os.Getwd()
		if err != nil {
			t.Fatalf("failed to get cwd: %v", err)
		}
		if err := os.Chdir(tmpDir); err != nil {
			t.Fatalf("failed to chdir: %v", err)
		}
		defer func() { _ = os.Chdir(oldWD) }()

		if got := fetchAssertion("https://idp.example.com", "user", "pass"); got != "" {
			t.Fatalf("fetchAssertion() = %q, want empty", got)
		}
	})

	t.Run("returns empty when flow bootstrap fails", func(t *testing.T) {
		t.Setenv("THUNDER_DEVELOP_APP_ID", "app-1")

		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		if got := fetchAssertion(srv.URL, "user", "pass"); got != "" {
			t.Fatalf("fetchAssertion() = %q, want empty", got)
		}
	})

	t.Run("returns trimmed assertion", func(t *testing.T) {
		t.Setenv("THUNDER_DEVELOP_APP_ID", "app-1")

		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() { _ = r.Body.Close() }()
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			if _, hasFlowType := payload["flowType"]; hasFlowType {
				_, _ = w.Write([]byte(`{"flowId":"flow-x","data":{"actions":[{"ref":"action_9"}]}}`))
				return
			}

			_, _ = w.Write([]byte(`{"assertion":"  token-abc  "}`))
		}))
		defer srv.Close()

		if got := fetchAssertion(srv.URL, "user", "pass"); got != "token-abc" {
			t.Fatalf("fetchAssertion() = %q, want %q", got, "token-abc")
		}
	})
}

type sslTrackingConn struct {
	closed bool
}

func (c *sslTrackingConn) Read(_ []byte) (int, error)         { return 0, os.ErrClosed }
func (c *sslTrackingConn) Write(b []byte) (int, error)         { return len(b), nil }
func (c *sslTrackingConn) Close() error                        { c.closed = true; return nil }
func (c *sslTrackingConn) LocalAddr() net.Addr                 { return nil }
func (c *sslTrackingConn) RemoteAddr() net.Addr                { return nil }
func (c *sslTrackingConn) SetDeadline(_ time.Time) error       { return nil }
func (c *sslTrackingConn) SetReadDeadline(_ time.Time) error   { return nil }
func (c *sslTrackingConn) SetWriteDeadline(_ time.Time) error  { return nil }
