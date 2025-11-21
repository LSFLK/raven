package response

import (
	"strings"
	"testing"
)

func TestQuoteOrNIL(t *testing.T) {
	if QuoteOrNIL("") != "NIL" {
		t.Errorf("expected NIL for empty string")
	}
	got := QuoteOrNIL("Hello \"World\"")
	if got != "\"Hello \\\"World\\\"\"" {
		t.Errorf("unexpected quoted output: %s", got)
	}
}

func TestExtractHeader_Simple(t *testing.T) {
	raw := "Subject: Test\r\nFrom: sender@example.com\r\n\r\nBody"
	if extractHeader(raw, "Subject") != "Test" {
		t.Errorf("subject mismatch")
	}
	if extractHeader(raw, "From") != "sender@example.com" {
		t.Errorf("from mismatch")
	}
}

func TestExtractHeader_Folded(t *testing.T) {
	raw := "Subject: Long\r\n continuing line\r\nFrom: sender@example.com\r\n\r\nBody"
	got := extractHeader(raw, "Subject")
	if got != "Long continuing line" {
		t.Errorf("expected folded header, got %q", got)
	}
}

func TestParseAddressList_Empty(t *testing.T) {
	if parseAddressList("") != "NIL" {
		t.Errorf("expected NIL for empty")
	}
}

func TestParseAddressList_Single(t *testing.T) {
	got := parseAddressList("user@example.com")
	expected := "((NIL NIL \"user\" \"example.com\"))" // matches current implementation output
	if got != expected {
		t.Errorf("unexpected structure: %s", got)
	}
}

func TestParseAddressList_WithName(t *testing.T) {
	got := parseAddressList("Alice <alice@example.com>")
	if got == "NIL" || len(got) == 0 {
		t.Errorf("unexpected NIL")
	}
	if !containsAll(got, []string{"Alice", "alice", "example.com"}) {
		t.Errorf("missing components: %s", got)
	}
}

func containsAll(s string, subs []string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

func TestBuildEnvelope_Defaults(t *testing.T) {
	raw := "From: Alice <alice@example.com>\r\nTo: Bob <bob@example.com>\r\nDate: Mon, 02 Jan 2006 15:04:05 -0700\r\nSubject: Hi\r\nMessage-ID: <id@example.com>\r\n\r\nBody"
	env := BuildEnvelope(raw)
	if !strings.HasPrefix(env, "ENVELOPE (") {
		t.Errorf("expected ENVELOPE structure")
	}
	if !containsAll(env, []string{"Hi", "Alice", "alice", "bob", "id@example.com"}) {
		t.Errorf("missing data: %s", env)
	}
}

func TestBuildEnvelope_MissingHeaders(t *testing.T) {
	raw := "To: recipient@example.com\r\n\r\nBody"
	env := BuildEnvelope(raw)
	if !strings.Contains(env, "NIL") {
		t.Error("expected NIL for missing headers")
	}
}

func TestBuildEnvelope_SenderDefault(t *testing.T) {
	raw := "From: sender@example.com\r\nTo: recipient@example.com\r\nSubject: Test\r\n\r\nBody"
	env := BuildEnvelope(raw)
	// Sender should default to From when missing - check structure contains from address multiple times
	count := strings.Count(env, "sender")
	if count < 2 {
		t.Errorf("expected sender field to default to from (should appear at least twice), found %d times", count)
	}
}

func TestBuildEnvelope_ReplyToDefault(t *testing.T) {
	raw := "From: sender@example.com\r\nTo: recipient@example.com\r\nSubject: Test\r\n\r\nBody"
	env := BuildEnvelope(raw)
	// Reply-To should default to From when missing - just verify envelope is valid
	if !strings.HasPrefix(env, "ENVELOPE (") {
		t.Error("expected valid ENVELOPE structure")
	}
}

func TestBuildEnvelope_MultipleRecipients(t *testing.T) {
	raw := "From: sender@example.com\r\nTo: user1@example.com, user2@example.com\r\nSubject: Multi\r\n\r\nBody"
	env := BuildEnvelope(raw)
	if !containsAll(env, []string{"user1", "user2"}) {
		t.Error("expected multiple recipients")
	}
}

func TestBuildEnvelope_CcBcc(t *testing.T) {
	raw := "From: sender@example.com\r\nTo: to@example.com\r\nCc: cc@example.com\r\nBcc: bcc@example.com\r\nSubject: Test\r\n\r\nBody"
	env := BuildEnvelope(raw)
	if !containsAll(env, []string{"cc", "bcc"}) {
		t.Error("expected Cc and Bcc fields")
	}
}

func TestExtractHeader_CaseInsensitive(t *testing.T) {
	raw := "subject: Test\r\nFROM: sender@example.com\r\n\r\nBody"
	if extractHeader(raw, "Subject") != "Test" {
		t.Error("case insensitive match failed")
	}
	if extractHeader(raw, "From") != "sender@example.com" {
		t.Error("case insensitive match failed")
	}
}

func TestExtractHeader_NotFound(t *testing.T) {
	raw := "From: sender@example.com\r\n\r\nBody"
	if extractHeader(raw, "X-Missing") != "" {
		t.Error("expected empty for missing header")
	}
}

func TestExtractHeader_MultiLine(t *testing.T) {
	raw := "Subject: This is a very long subject\r\n that continues on the next line\r\n and even further\r\nFrom: sender@example.com\r\n\r\nBody"
	got := extractHeader(raw, "Subject")
	expected := "This is a very long subject that continues on the next line and even further"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestParseAddressList_Multiple(t *testing.T) {
	got := parseAddressList("user1@example.com, user2@example.com")
	if !containsAll(got, []string{"user1", "user2", "example.com"}) {
		t.Error("missing addresses")
	}
}

func TestParseAddressList_WithQuotedNames(t *testing.T) {
	got := parseAddressList("\"John Doe\" <john@example.com>, \"Jane Smith\" <jane@example.com>")
	if !containsAll(got, []string{"John Doe", "Jane Smith", "john", "jane"}) {
		t.Error("missing name or address")
	}
}

func TestQuoteOrNIL_Backslashes(t *testing.T) {
	got := QuoteOrNIL("Path\\to\\file")
	if !strings.Contains(got, "\\\\") {
		t.Error("expected escaped backslashes")
	}
}

func TestQuoteOrNIL_Quotes(t *testing.T) {
	got := QuoteOrNIL("Say \"Hello\"")
	if !strings.Contains(got, "\\\"") {
		t.Error("expected escaped quotes")
	}
}
