package utils

import (
	"strings"
	"testing"
)

func TestBuildEnvelope_Complete(t *testing.T) {
	rawMsg := "Date: Mon, 7 Feb 1994 21:52:25 -0800\r\n" +
		"From: Fred Foobar <foobar@example.com>\r\n" +
		"Sender: sender@example.com\r\n" +
		"Reply-To: reply@example.com\r\n" +
		"To: mooch@example.com\r\n" +
		"Cc: cc@example.com\r\n" +
		"Bcc: bcc@example.com\r\n" +
		"In-Reply-To: <previous@example.com>\r\n" +
		"Message-ID: <B27397-0100000@example.com>\r\n" +
		"Subject: Test message\r\n" +
		"\r\n" +
		"Body content"

	envelope := BuildEnvelope(rawMsg)

	expectedFields := []string{
		"ENVELOPE",
		"Mon, 7 Feb 1994 21:52:25 -0800",
		"Test message",
		"foobar",
		"sender",
		"reply",
		"mooch",
		"cc",
		"bcc",
		"previous",
		"B27397-0100000",
	}

	for _, field := range expectedFields {
		if !strings.Contains(envelope, field) {
			t.Errorf("Expected envelope to contain '%s', got: %s", field, envelope)
		}
	}
}

func TestBuildEnvelope_MinimalHeaders(t *testing.T) {
	rawMsg := "Subject: Hello\r\n" +
		"From: user@example.com\r\n" +
		"\r\n" +
		"Body"

	envelope := BuildEnvelope(rawMsg)

	if !strings.Contains(envelope, "ENVELOPE") {
		t.Error("Expected ENVELOPE keyword")
	}
	if !strings.Contains(envelope, "Hello") {
		t.Error("Expected subject 'Hello'")
	}
	if !strings.Contains(envelope, "user") {
		t.Error("Expected from address mailbox 'user'")
	}
	if !strings.Contains(envelope, "example.com") {
		t.Error("Expected from address host 'example.com'")
	}
	if !strings.Contains(envelope, "NIL") {
		t.Error("Expected NIL for missing headers")
	}
}

func TestBuildEnvelope_SenderFallback(t *testing.T) {
	rawMsg := "From: user@example.com\r\n\r\nBody"
	envelope := BuildEnvelope(rawMsg)

	// Sender should default to from address (appears twice in envelope for from and sender)
	// Check for mailbox "user" which should appear twice
	count := strings.Count(envelope, "\"user\"")
	if count < 2 {
		t.Errorf("Expected mailbox 'user' to appear at least twice (from and sender), got %d occurrences", count)
	}
}

func TestBuildEnvelope_ReplyToFallback(t *testing.T) {
	rawMsg := "From: user@example.com\r\n\r\nBody"
	envelope := BuildEnvelope(rawMsg)

	// Reply-to should default to from address (appears three times: from, sender, reply-to)
	// Check for mailbox "user" which should appear three times
	count := strings.Count(envelope, "\"user\"")
	if count < 3 {
		t.Errorf("Expected mailbox 'user' to appear at least three times, got %d occurrences", count)
	}
}

func TestExtractHeader_Simple(t *testing.T) {
	rawMsg := "Subject: Test Subject\r\n\r\nBody"
	subject := ExtractHeader(rawMsg, "Subject")

	if subject != "Test Subject" {
		t.Errorf("Expected 'Test Subject', got '%s'", subject)
	}
}

func TestExtractHeader_CaseInsensitive(t *testing.T) {
	rawMsg := "subject: lowercase\r\n\r\nBody"
	subject := ExtractHeader(rawMsg, "Subject")

	if subject != "lowercase" {
		t.Errorf("Expected 'lowercase', got '%s'", subject)
	}
}

func TestExtractHeader_Continuation(t *testing.T) {
	rawMsg := "Subject: Line one\r\n" +
		" continuation line\r\n" +
		"\tanother continuation\r\n" +
		"\r\n" +
		"Body"

	subject := ExtractHeader(rawMsg, "Subject")

	if !strings.Contains(subject, "Line one") {
		t.Error("Expected first line in subject")
	}
	if !strings.Contains(subject, "continuation line") {
		t.Error("Expected continuation line")
	}
	if !strings.Contains(subject, "another continuation") {
		t.Error("Expected tab continuation")
	}
}

func TestExtractHeader_Missing(t *testing.T) {
	rawMsg := "Subject: Test\r\n\r\nBody"
	from := ExtractHeader(rawMsg, "From")

	if from != "" {
		t.Errorf("Expected empty string for missing header, got '%s'", from)
	}
}

func TestExtractHeader_MultipleHeaders(t *testing.T) {
	rawMsg := "From: first@example.com\r\n" +
		"Subject: Test\r\n" +
		"To: second@example.com\r\n" +
		"\r\n" +
		"Body"

	from := ExtractHeader(rawMsg, "From")
	subject := ExtractHeader(rawMsg, "Subject")
	to := ExtractHeader(rawMsg, "To")

	if from != "first@example.com" {
		t.Errorf("Expected 'first@example.com', got '%s'", from)
	}
	if subject != "Test" {
		t.Errorf("Expected 'Test', got '%s'", subject)
	}
	if to != "second@example.com" {
		t.Errorf("Expected 'second@example.com', got '%s'", to)
	}
}

func TestExtractHeader_StopsAtBody(t *testing.T) {
	rawMsg := "Subject: Real Header\r\n" +
		"\r\n" +
		"Subject: Not a header"

	subject := ExtractHeader(rawMsg, "Subject")

	if subject != "Real Header" {
		t.Errorf("Expected 'Real Header', got '%s'", subject)
	}
}

func TestQuoteOrNIL_NonEmpty(t *testing.T) {
	result := QuoteOrNIL("test string")
	expected := `"test string"`

	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestQuoteOrNIL_Empty(t *testing.T) {
	result := QuoteOrNIL("")
	expected := "NIL"

	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestQuoteOrNIL_WithQuotes(t *testing.T) {
	result := QuoteOrNIL(`test "quoted" string`)
	expected := `"test \"quoted\" string"`

	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestQuoteOrNIL_WithBackslash(t *testing.T) {
	result := QuoteOrNIL(`test\string`)
	expected := `"test\\string"`

	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestQuoteOrNIL_WithBoth(t *testing.T) {
	result := QuoteOrNIL(`test\"string`)
	expected := `"test\\\"string"`

	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestParseAddressList_Empty(t *testing.T) {
	result := ParseAddressList("")

	if result != "NIL" {
		t.Errorf("Expected NIL for empty address, got %s", result)
	}
}

func TestParseAddressList_SimpleEmail(t *testing.T) {
	result := ParseAddressList("user@example.com")

	if !strings.Contains(result, "user") {
		t.Error("Expected mailbox 'user'")
	}
	if !strings.Contains(result, "example.com") {
		t.Error("Expected host 'example.com'")
	}
	if !strings.Contains(result, "NIL") {
		t.Error("Expected NIL for empty name")
	}
}

func TestParseAddressList_WithName(t *testing.T) {
	result := ParseAddressList("John Doe <john@example.com>")

	if !strings.Contains(result, "John Doe") {
		t.Error("Expected name 'John Doe'")
	}
	if !strings.Contains(result, "john") {
		t.Error("Expected mailbox 'john'")
	}
	if !strings.Contains(result, "example.com") {
		t.Error("Expected host 'example.com'")
	}
}

func TestParseAddressList_QuotedName(t *testing.T) {
	result := ParseAddressList(`"Jane Smith" <jane@example.com>`)

	if !strings.Contains(result, "Jane Smith") {
		t.Error("Expected name 'Jane Smith'")
	}
	if !strings.Contains(result, "jane") {
		t.Error("Expected mailbox 'jane'")
	}
}

func TestParseAddressList_Multiple(t *testing.T) {
	result := ParseAddressList("user1@example.com, User Two <user2@example.com>")

	if !strings.Contains(result, "user1") {
		t.Error("Expected first mailbox")
	}
	if !strings.Contains(result, "user2") {
		t.Error("Expected second mailbox")
	}
	if !strings.Contains(result, "User Two") {
		t.Error("Expected second user name")
	}
}

func TestParseAddressList_NoAtSign(t *testing.T) {
	result := ParseAddressList("localuser")

	if !strings.Contains(result, "localuser") {
		t.Error("Expected mailbox 'localuser'")
	}
}

func TestParseAddressList_EmptyAfterSplit(t *testing.T) {
	result := ParseAddressList("user@example.com, , ")

	// Should ignore empty entries and return single address
	if !strings.Contains(result, "user") {
		t.Error("Expected valid address to be parsed")
	}
}

func TestParseAddressList_RouteIsAlwaysNIL(t *testing.T) {
	result := ParseAddressList("user@example.com")

	// Check that route field is NIL (appears after name field)
	parts := strings.Split(result, "NIL")
	if len(parts) < 2 {
		t.Error("Expected at least one NIL for route field")
	}
}
