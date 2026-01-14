package parser_test

import (
	"bufio"
	"database/sql"
	"net/mail"
	"strings"
	"testing"
	"time"

	"raven/internal/db"
	"raven/internal/delivery/parser"

	_ "github.com/mattn/go-sqlite3"
)

func TestParseMessage(t *testing.T) {
	rawEmail := `From: sender@example.com
To: recipient@example.com
Subject: Test Message
Date: Mon, 01 Jan 2024 12:00:00 +0000
Message-Id: <test123@example.com>

This is a test message body.
`

	msg, err := parser.ParseMessageFromBytes([]byte(rawEmail))
	if err != nil {
		t.Fatalf("Failed to parse message: %v", err)
	}

	if msg.From != "sender@example.com" {
		t.Errorf("Expected From: sender@example.com, got: %s", msg.From)
	}

	if len(msg.To) == 0 || msg.To[0] != "recipient@example.com" {
		t.Errorf("Expected To: recipient@example.com, got: %v", msg.To)
	}

	if msg.Subject != "Test Message" {
		t.Errorf("Expected Subject: Test Message, got: %s", msg.Subject)
	}

	if msg.MessageID != "<test123@example.com>" {
		t.Errorf("Expected Message-Id: <test123@example.com>, got: %s", msg.MessageID)
	}

	if !strings.Contains(msg.Body, "This is a test message body") {
		t.Errorf("Body does not contain expected text")
	}
}

func TestParseMessageWithMultipleRecipients(t *testing.T) {
	rawEmail := `From: sender@example.com
To: recipient1@example.com, recipient2@example.com
Cc: cc@example.com
Subject: Test Message

Body
`

	msg, err := parser.ParseMessageFromBytes([]byte(rawEmail))
	if err != nil {
		t.Fatalf("Failed to parse message: %v", err)
	}

	if len(msg.To) != 3 {
		t.Errorf("Expected 3 recipients, got: %d", len(msg.To))
	}
}

func TestValidateMessage(t *testing.T) {
	tests := []struct {
		name      string
		msg       *parser.Message
		maxSize   int64
		expectErr bool
	}{
		{
			name: "Valid message",
			msg: &parser.Message{
				From: "sender@example.com",
				To:   []string{"recipient@example.com"},
				Size: 100,
			},
			maxSize:   1000,
			expectErr: false,
		},
		{
			name: "Missing From",
			msg: &parser.Message{
				To:   []string{"recipient@example.com"},
				Size: 100,
			},
			maxSize:   1000,
			expectErr: true,
		},
		{
			name: "Missing recipients",
			msg: &parser.Message{
				From: "sender@example.com",
				To:   []string{},
				Size: 100,
			},
			maxSize:   1000,
			expectErr: true,
		},
		{
			name: "Size exceeds limit",
			msg: &parser.Message{
				From: "sender@example.com",
				To:   []string{"recipient@example.com"},
				Size: 2000,
			},
			maxSize:   1000,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parser.ValidateMessage(tt.msg, tt.maxSize)
			if tt.expectErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestExtractEnvelopeRecipient(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  string
		expectErr bool
	}{
		{
			name:      "Simple format",
			input:     "user@example.com",
			expected:  "user@example.com",
			expectErr: false,
		},
		{
			name:      "Angle brackets",
			input:     "<user@example.com>",
			expected:  "user@example.com",
			expectErr: false,
		},
		{
			name:      "With display name",
			input:     `"John Doe" <user@example.com>`,
			expected:  "user@example.com",
			expectErr: false,
		},
		{
			name:      "Invalid email",
			input:     "invalid",
			expected:  "",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parser.ExtractEnvelopeRecipient(tt.input)
			if tt.expectErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestReadDataCommand(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		maxSize   int64
		expected  string
		expectErr bool
	}{
		{
			name:      "Simple message",
			input:     "Line 1\r\nLine 2\r\n.\r\n",
			maxSize:   1000,
			expected:  "Line 1\r\nLine 2\r\n",
			expectErr: false,
		},
		{
			name:      "Dot stuffing",
			input:     "..Line 1\r\n.\r\n",
			maxSize:   1000,
			expected:  ".Line 1\r\n",
			expectErr: false,
		},
		{
			name:      "Size exceeded",
			input:     "This is a long message\r\n.\r\n",
			maxSize:   5,
			expected:  "",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bufio.NewReader(strings.NewReader(tt.input))
			result, err := parser.ReadDataCommand(reader, tt.maxSize)

			if tt.expectErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
			if !tt.expectErr && string(result) != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, string(result))
			}
		})
	}
}

func TestExtractLocalPart(t *testing.T) {
	tests := []struct {
		name      string
		email     string
		expected  string
		expectErr bool
	}{
		{
			name:      "Valid email",
			email:     "user@example.com",
			expected:  "user",
			expectErr: false,
		},
		{
			name:      "Invalid email - no @",
			email:     "invalid",
			expected:  "",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parser.ExtractLocalPart(tt.email)
			if tt.expectErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestExtractDomain(t *testing.T) {
	tests := []struct {
		name      string
		email     string
		expected  string
		expectErr bool
	}{
		{
			name:      "Valid email",
			email:     "user@example.com",
			expected:  "example.com",
			expectErr: false,
		},
		{
			name:      "Invalid email - no @",
			email:     "invalid",
			expected:  "",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parser.ExtractDomain(tt.email)
			if tt.expectErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestParseMIMEMessage_SinglePart(t *testing.T) {
	tests := []struct {
		name        string
		rawMessage  string
		expectError bool
		checkFunc   func(*testing.T, *parser.ParsedMessage)
	}{
		{
			name: "Simple text/plain message",
			rawMessage: `From: sender@example.com
To: recipient@example.com
Subject: Test Subject
Date: Mon, 01 Jan 2024 12:00:00 +0000
Content-Type: text/plain; charset=utf-8

This is a plain text message.`,
			expectError: false,
			checkFunc: func(t *testing.T, msg *parser.ParsedMessage) {
				if msg.Subject != "Test Subject" {
					t.Errorf("Expected subject 'Test Subject', got '%s'", msg.Subject)
				}
				if len(msg.From) == 0 || msg.From[0].Address != "sender@example.com" {
					t.Errorf("Expected from 'sender@example.com', got %v", msg.From)
				}
				if len(msg.To) == 0 || msg.To[0].Address != "recipient@example.com" {
					t.Errorf("Expected to 'recipient@example.com', got %v", msg.To)
				}
				if len(msg.Parts) != 1 {
					t.Errorf("Expected 1 part, got %d", len(msg.Parts))
				}
				if len(msg.Parts) > 0 && msg.Parts[0].ContentType != "text/plain" {
					t.Errorf("Expected content type 'text/plain', got '%s'", msg.Parts[0].ContentType)
				}
			},
		},
		{
			name: "Message with no Content-Type (defaults to text/plain)",
			rawMessage: `From: sender@example.com
To: recipient@example.com
Subject: Default Content Type

Plain message body without explicit content type.`,
			expectError: false,
			checkFunc: func(t *testing.T, msg *parser.ParsedMessage) {
				if len(msg.Parts) != 1 {
					t.Errorf("Expected 1 part, got %d", len(msg.Parts))
				}
				if len(msg.Parts) > 0 && msg.Parts[0].ContentType != "text/plain" {
					t.Errorf("Expected default content type 'text/plain', got '%s'", msg.Parts[0].ContentType)
				}
			},
		},
		{
			name: "Message with multiple address types",
			rawMessage: `From: sender@example.com
To: to1@example.com, to2@example.com
Cc: cc@example.com
Bcc: bcc@example.com
Subject: Multiple Recipients

Body text.`,
			expectError: false,
			checkFunc: func(t *testing.T, msg *parser.ParsedMessage) {
				if len(msg.To) != 2 {
					t.Errorf("Expected 2 To addresses, got %d", len(msg.To))
				}
				if len(msg.Cc) != 1 {
					t.Errorf("Expected 1 Cc address, got %d", len(msg.Cc))
				}
				if len(msg.Bcc) != 1 {
					t.Errorf("Expected 1 Bcc address, got %d", len(msg.Bcc))
				}
			},
		},
		{
			name: "Message with In-Reply-To and References",
			rawMessage: `From: sender@example.com
To: recipient@example.com
Subject: Re: Previous Message
In-Reply-To: <msg123@example.com>
References: <msg123@example.com>
Date: Mon, 01 Jan 2024 12:00:00 +0000

Reply body.`,
			expectError: false,
			checkFunc: func(t *testing.T, msg *parser.ParsedMessage) {
				if msg.InReplyTo != "<msg123@example.com>" {
					t.Errorf("Expected InReplyTo '<msg123@example.com>', got '%s'", msg.InReplyTo)
				}
				if msg.References != "<msg123@example.com>" {
					t.Errorf("Expected References '<msg123@example.com>', got '%s'", msg.References)
				}
			},
		},
		{
			name: "Message with invalid date defaults to now",
			rawMessage: `From: sender@example.com
To: recipient@example.com
Subject: Invalid Date
Date: Not a valid date

Body.`,
			expectError: false,
			checkFunc: func(t *testing.T, msg *parser.ParsedMessage) {
				if msg.Date.IsZero() {
					t.Error("Expected date to be set to current time for invalid date")
				}
			},
		},
		{
			name: "Invalid message format",
			rawMessage: `This is not a valid email message
without proper headers`,
			expectError: true,
			checkFunc:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := parser.ParseMIMEMessage(tt.rawMessage)
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
			if !tt.expectError && tt.checkFunc != nil && msg != nil {
				tt.checkFunc(t, msg)
			}
		})
	}
}

func TestParseMIMEMessage_Multipart(t *testing.T) {
	tests := []struct {
		name        string
		rawMessage  string
		expectError bool
		checkFunc   func(*testing.T, *parser.ParsedMessage)
	}{
		{
			name: "Multipart/alternative with text and HTML",
			rawMessage: `From: sender@example.com
To: recipient@example.com
Subject: Multipart Test
Content-Type: multipart/alternative; boundary="boundary123"
MIME-Version: 1.0

--boundary123
Content-Type: text/plain; charset=utf-8

Plain text version.
--boundary123
Content-Type: text/html; charset=utf-8

<html><body>HTML version.</body></html>
--boundary123--`,
			expectError: false,
			checkFunc: func(t *testing.T, msg *parser.ParsedMessage) {
				if len(msg.Parts) != 2 {
					t.Errorf("Expected 2 parts, got %d", len(msg.Parts))
				}
				if len(msg.Parts) >= 2 {
					if msg.Parts[0].ContentType != "text/plain" {
						t.Errorf("Expected first part to be text/plain, got %s", msg.Parts[0].ContentType)
					}
					if msg.Parts[1].ContentType != "text/html" {
						t.Errorf("Expected second part to be text/html, got %s", msg.Parts[1].ContentType)
					}
				}
			},
		},
		{
			name: "Multipart/mixed with attachment",
			rawMessage: `From: sender@example.com
To: recipient@example.com
Subject: Message with Attachment
Content-Type: multipart/mixed; boundary="boundary456"
MIME-Version: 1.0

--boundary456
Content-Type: text/plain; charset=utf-8

Message body.
--boundary456
Content-Type: application/pdf; name="document.pdf"
Content-Disposition: attachment; filename="document.pdf"

Binary content here
--boundary456--`,
			expectError: false,
			checkFunc: func(t *testing.T, msg *parser.ParsedMessage) {
				if len(msg.Parts) != 2 {
					t.Errorf("Expected 2 parts, got %d", len(msg.Parts))
				}
				if len(msg.Parts) >= 2 {
					if msg.Parts[1].ContentType != "application/pdf" {
						t.Errorf("Expected attachment to be application/pdf, got %s", msg.Parts[1].ContentType)
					}
					if msg.Parts[1].Filename != "document.pdf" {
						t.Errorf("Expected filename 'document.pdf', got '%s'", msg.Parts[1].Filename)
					}
				}
			},
		},
		{
			name: "Multipart with no boundary",
			rawMessage: `From: sender@example.com
To: recipient@example.com
Subject: Malformed Multipart
Content-Type: multipart/mixed

Body without boundary.`,
			expectError: false,
			checkFunc: func(t *testing.T, msg *parser.ParsedMessage) {
				// When multipart has no boundary, code skips multipart parsing
				// This is expected behavior - the message is still parsed
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := parser.ParseMIMEMessage(tt.rawMessage)
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
			if !tt.expectError && tt.checkFunc != nil && msg != nil {
				tt.checkFunc(t, msg)
			}
		})
	}
}

func TestExtractAllHeaders(t *testing.T) {
	rawMessage := `From: sender@example.com
To: recipient1@example.com,
 recipient2@example.com
Subject: Test Subject
X-Custom-Header: Custom Value
Date: Mon, 01 Jan 2024 12:00:00 +0000

Body starts here.`

	msg, err := parser.ParseMIMEMessage(rawMessage)
	if err != nil {
		t.Fatalf("Failed to parse message: %v", err)
	}

	if len(msg.Headers) == 0 {
		t.Error("Expected headers to be extracted")
	}

	foundFrom := false
	foundTo := false
	foundSubject := false
	for _, header := range msg.Headers {
		if header.Name == "From" {
			foundFrom = true
			if header.Value != "sender@example.com" {
				t.Errorf("Expected From header value 'sender@example.com', got '%s'", header.Value)
			}
		}
		if header.Name == "To" {
			foundTo = true
		}
		if header.Name == "Subject" {
			foundSubject = true
			if header.Value != "Test Subject" {
				t.Errorf("Expected Subject 'Test Subject', got '%s'", header.Value)
			}
		}
	}

	if !foundFrom || !foundTo || !foundSubject {
		t.Error("Expected to find From, To, and Subject headers")
	}
}

func TestIsValidEmail(t *testing.T) {
	tests := []struct {
		name  string
		email string
		want  bool
	}{
		{"Valid email", "user@example.com", true},
		{"Valid email with subdomain", "user@mail.example.com", true},
		{"Missing @", "userexample.com", false},
		{"Missing local part", "@example.com", false},
		{"Missing domain", "user@", false},
		{"No dot in domain", "user@example", false},
		{"Empty string", "", false},
		{"Multiple @", "user@@example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We need to test isValidEmail indirectly through ExtractEnvelopeRecipient
			_, err := parser.ExtractEnvelopeRecipient(tt.email)
			got := err == nil
			if got != tt.want {
				t.Errorf("isValidEmail(%q) = %v, want %v", tt.email, got, tt.want)
			}
		})
	}
}

func TestParseAddressList(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"Single address", "user@example.com", 1},
		{"Multiple addresses", "user1@example.com, user2@example.com", 2},
		{"With display names", "\"John Doe\" <john@example.com>, \"Jane Doe\" <jane@example.com>", 2},
		{"Invalid format fallback", "invalid1, invalid2", 2},
		{"Empty string", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test indirectly through ParseMessage
			rawEmail := "From: sender@example.com\nTo: " + tt.input + "\n\nBody"
			msg, err := parser.ParseMessageFromBytes([]byte(rawEmail))
			if tt.expected > 0 && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if err == nil && len(msg.To) != tt.expected {
				t.Errorf("Expected %d recipients, got %d", tt.expected, len(msg.To))
			}
		})
	}
}

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to initialize test database: %v", err)
	}
	return database
}

func TestStoreMessage(t *testing.T) {
	database := setupTestDB(t)
	defer func() { _ = database.Close() }()

	rawMessage := `From: sender@example.com
To: recipient1@example.com, recipient2@example.com
Cc: cc@example.com
Subject: Test Message for Storage
Date: Mon, 01 Jan 2024 12:00:00 +0000
Content-Type: text/plain; charset=utf-8

This is the message body.`

	parsed, err := parser.ParseMIMEMessage(rawMessage)
	if err != nil {
		t.Fatalf("Failed to parse message: %v", err)
	}

	messageID, err := parser.StoreMessageWithSharedDB(database, database, parsed)
	if err != nil {
		t.Fatalf("Failed to store message: %v", err)
	}

	if messageID == 0 {
		t.Error("Expected non-zero message ID")
	}

	var subject string
	err = database.QueryRow("SELECT subject FROM messages WHERE id = ?", messageID).Scan(&subject)
	if err != nil {
		t.Fatalf("Failed to retrieve message: %v", err)
	}

	if subject != "Test Message for Storage" {
		t.Errorf("Expected subject 'Test Message for Storage', got '%s'", subject)
	}

	var headerCount int
	err = database.QueryRow("SELECT COUNT(*) FROM message_headers WHERE message_id = ?", messageID).Scan(&headerCount)
	if err != nil {
		t.Fatalf("Failed to count headers: %v", err)
	}

	if headerCount == 0 {
		t.Error("Expected at least one header to be stored")
	}

	var addressCount int
	err = database.QueryRow("SELECT COUNT(*) FROM addresses WHERE message_id = ?", messageID).Scan(&addressCount)
	if err != nil {
		t.Fatalf("Failed to count addresses: %v", err)
	}

	if addressCount < 3 {
		t.Errorf("Expected at least 3 addresses (2 To + 1 Cc), got %d", addressCount)
	}

	var partCount int
	err = database.QueryRow("SELECT COUNT(*) FROM message_parts WHERE message_id = ?", messageID).Scan(&partCount)
	if err != nil {
		t.Fatalf("Failed to count parts: %v", err)
	}

	if partCount != 1 {
		t.Errorf("Expected 1 message part, got %d", partCount)
	}
}

func TestStoreMessage_Multipart(t *testing.T) {
	database := setupTestDB(t)
	defer func() { _ = database.Close() }()

	rawMessage := `From: sender@example.com
To: recipient@example.com
Subject: Multipart Message
Content-Type: multipart/alternative; boundary="boundary123"
MIME-Version: 1.0

--boundary123
Content-Type: text/plain; charset=utf-8

Plain text version.
--boundary123
Content-Type: text/html; charset=utf-8

<html><body>HTML version.</body></html>
--boundary123--`

	parsed, err := parser.ParseMIMEMessage(rawMessage)
	if err != nil {
		t.Fatalf("Failed to parse message: %v", err)
	}

	messageID, err := parser.StoreMessageWithSharedDB(database, database, parsed)
	if err != nil {
		t.Fatalf("Failed to store message: %v", err)
	}

	var partCount int
	err = database.QueryRow("SELECT COUNT(*) FROM message_parts WHERE message_id = ?", messageID).Scan(&partCount)
	if err != nil {
		t.Fatalf("Failed to count parts: %v", err)
	}

	if partCount != 2 {
		t.Errorf("Expected 2 message parts, got %d", partCount)
	}
}

func TestStoreMessage_LargeContent(t *testing.T) {
	database := setupTestDB(t)
	defer func() { _ = database.Close() }()

	largeBody := strings.Repeat("This is a large message body. ", 100)
	rawMessage := "From: sender@example.com\nTo: recipient@example.com\nSubject: Large Message\n\n" + largeBody

	parsed, err := parser.ParseMIMEMessage(rawMessage)
	if err != nil {
		t.Fatalf("Failed to parse message: %v", err)
	}

	messageID, err := parser.StoreMessageWithSharedDB(database, database, parsed)
	if err != nil {
		t.Fatalf("Failed to store message: %v", err)
	}

	var blobCount int
	err = database.QueryRow("SELECT COUNT(*) FROM blobs WHERE id IN (SELECT blob_id FROM message_parts WHERE message_id = ?)", messageID).Scan(&blobCount)
	if err != nil {
		t.Fatalf("Failed to count blobs: %v", err)
	}

	if blobCount == 0 {
		t.Error("Expected large content to be stored in blob")
	}
}

func TestStoreMessagePerUser(t *testing.T) {
	database := setupTestDB(t)
	defer func() { _ = database.Close() }()

	rawMessage := `From: sender@example.com
To: user@example.com
Subject: Per-User Message
Date: Mon, 01 Jan 2024 12:00:00 +0000

Message body for user.`

	parsed, err := parser.ParseMIMEMessage(rawMessage)
	if err != nil {
		t.Fatalf("Failed to parse message: %v", err)
	}

	messageID, err := parser.StoreMessagePerUserWithSharedDB(database, database, parsed)
	if err != nil {
		t.Fatalf("Failed to store message per user: %v", err)
	}

	if messageID == 0 {
		t.Error("Expected non-zero message ID")
	}

	var subject string
	err = database.QueryRow("SELECT subject FROM messages WHERE id = ?", messageID).Scan(&subject)
	if err != nil {
		t.Fatalf("Failed to retrieve message: %v", err)
	}

	if subject != "Per-User Message" {
		t.Errorf("Expected subject 'Per-User Message', got '%s'", subject)
	}
}

func TestReconstructMessage_SinglePart(t *testing.T) {
	database := setupTestDB(t)
	defer func() { _ = database.Close() }()

	rawMessage := `From: sender@example.com
To: recipient@example.com
Subject: Simple Message
Date: Mon, 01 Jan 2024 12:00:00 +0000
Content-Type: text/plain; charset=utf-8

Simple message body.`

	parsed, err := parser.ParseMIMEMessage(rawMessage)
	if err != nil {
		t.Fatalf("Failed to parse message: %v", err)
	}

	messageID, err := parser.StoreMessageWithSharedDB(database, database, parsed)
	if err != nil {
		t.Fatalf("Failed to store message: %v", err)
	}

	reconstructed, err := parser.ReconstructMessageWithSharedDB(database, database, messageID)
	if err != nil {
		t.Fatalf("Failed to reconstruct message: %v", err)
	}

	if !strings.Contains(reconstructed, "Simple message body") {
		t.Error("Reconstructed message missing expected body content")
	}

	if !strings.Contains(reconstructed, "From: sender@example.com") {
		t.Error("Reconstructed message missing From header")
	}

	if !strings.Contains(reconstructed, "Subject: Simple Message") {
		t.Error("Reconstructed message missing Subject header")
	}
}

func TestReconstructMessage_Multipart(t *testing.T) {
	database := setupTestDB(t)
	defer func() { _ = database.Close() }()

	rawMessage := `From: sender@example.com
To: recipient@example.com
Subject: Multipart Message
Content-Type: multipart/alternative; boundary="boundary123"
MIME-Version: 1.0

--boundary123
Content-Type: text/plain; charset=utf-8

Plain text version.
--boundary123
Content-Type: text/html; charset=utf-8

<html><body>HTML version.</body></html>
--boundary123--`

	parsed, err := parser.ParseMIMEMessage(rawMessage)
	if err != nil {
		t.Fatalf("Failed to parse message: %v", err)
	}

	messageID, err := parser.StoreMessageWithSharedDB(database, database, parsed)
	if err != nil {
		t.Fatalf("Failed to store message: %v", err)
	}

	reconstructed, err := parser.ReconstructMessageWithSharedDB(database, database, messageID)
	if err != nil {
		t.Fatalf("Failed to reconstruct message: %v", err)
	}

	if !strings.Contains(reconstructed, "multipart/alternative") && !strings.Contains(reconstructed, "multipart/mixed") {
		t.Error("Reconstructed message missing multipart content type")
	}

	if !strings.Contains(reconstructed, "Plain text version") {
		t.Error("Reconstructed message missing plain text part")
	}

	if !strings.Contains(reconstructed, "HTML version") {
		t.Error("Reconstructed message missing HTML part")
	}
}

func TestReconstructMessage_WithAttachment(t *testing.T) {
	database := setupTestDB(t)
	defer func() { _ = database.Close() }()

	rawMessage := `From: sender@example.com
To: recipient@example.com
Subject: Message with Attachment
Content-Type: multipart/mixed; boundary="boundary456"
MIME-Version: 1.0

--boundary456
Content-Type: text/plain; charset=utf-8

Message body.
--boundary456
Content-Type: application/pdf; name="document.pdf"
Content-Disposition: attachment; filename="document.pdf"

Binary content
--boundary456--`

	parsed, err := parser.ParseMIMEMessage(rawMessage)
	if err != nil {
		t.Fatalf("Failed to parse message: %v", err)
	}

	messageID, err := parser.StoreMessageWithSharedDB(database, database, parsed)
	if err != nil {
		t.Fatalf("Failed to store message: %v", err)
	}

	reconstructed, err := parser.ReconstructMessageWithSharedDB(database, database, messageID)
	if err != nil {
		t.Fatalf("Failed to reconstruct message: %v", err)
	}

	if !strings.Contains(reconstructed, "multipart/mixed") {
		t.Error("Reconstructed message should be multipart/mixed")
	}

	if !strings.Contains(reconstructed, "application/pdf") {
		t.Error("Reconstructed message missing PDF attachment content type")
	}

	if !strings.Contains(reconstructed, "document.pdf") {
		t.Error("Reconstructed message missing attachment filename")
	}
}

func TestReconstructMessage_NoPartsError(t *testing.T) {
	database := setupTestDB(t)
	defer func() { _ = database.Close() }()

	_, err := parser.ReconstructMessageWithSharedDB(database, database, 99999)
	if err == nil {
		t.Error("Expected error when reconstructing non-existent message")
	}
}

func TestStoreMessage_WithBlobStorage(t *testing.T) {
	database := setupTestDB(t)
	defer func() { _ = database.Close() }()

	attachmentContent := strings.Repeat("A", 2000)
	rawMessage := `From: sender@example.com
To: recipient@example.com
Subject: Message with Large Attachment
Content-Type: multipart/mixed; boundary="boundary789"
MIME-Version: 1.0

--boundary789
Content-Type: text/plain; charset=utf-8

Short body.
--boundary789
Content-Type: application/octet-stream; name="large.bin"
Content-Disposition: attachment; filename="large.bin"

` + attachmentContent + `
--boundary789--`

	parsed, err := parser.ParseMIMEMessage(rawMessage)
	if err != nil {
		t.Fatalf("Failed to parse message: %v", err)
	}

	messageID, err := parser.StoreMessageWithSharedDB(database, database, parsed)
	if err != nil {
		t.Fatalf("Failed to store message: %v", err)
	}

	var blobID sql.NullInt64
	err = database.QueryRow("SELECT blob_id FROM message_parts WHERE message_id = ? AND filename = ?", messageID, "large.bin").Scan(&blobID)
	if err != nil {
		t.Fatalf("Failed to retrieve blob ID: %v", err)
	}

	if !blobID.Valid {
		t.Error("Expected attachment to be stored in blob")
	}
}

func TestStoreAndReconstruct_ComplexMessage(t *testing.T) {
	database := setupTestDB(t)
	defer func() { _ = database.Close() }()

	originalMessage := `From: John Doe <john@example.com>
To: jane@example.com, bob@example.com
Cc: admin@example.com
Subject: Complex Test Message
Date: Mon, 01 Jan 2024 12:00:00 +0000
In-Reply-To: <previous@example.com>
References: <previous@example.com>
Content-Type: multipart/alternative; boundary="alt123"
MIME-Version: 1.0

--alt123
Content-Type: text/plain; charset=utf-8

This is the plain text version of the message.
--alt123
Content-Type: text/html; charset=utf-8

<html><body><p>This is the HTML version of the message.</p></body></html>
--alt123--`

	parsed, err := parser.ParseMIMEMessage(originalMessage)
	if err != nil {
		t.Fatalf("Failed to parse message: %v", err)
	}

	if parsed.Subject != "Complex Test Message" {
		t.Errorf("Expected subject 'Complex Test Message', got '%s'", parsed.Subject)
	}

	if parsed.InReplyTo != "<previous@example.com>" {
		t.Errorf("Expected InReplyTo '<previous@example.com>', got '%s'", parsed.InReplyTo)
	}

	messageID, err := parser.StoreMessageWithSharedDB(database, database, parsed)
	if err != nil {
		t.Fatalf("Failed to store message: %v", err)
	}

	reconstructed, err := parser.ReconstructMessageWithSharedDB(database, database, messageID)
	if err != nil {
		t.Fatalf("Failed to reconstruct message: %v", err)
	}

	if !strings.Contains(reconstructed, "Complex Test Message") {
		t.Error("Reconstructed message missing subject")
	}

	if !strings.Contains(reconstructed, "john@example.com") {
		t.Error("Reconstructed message missing from address")
	}

	if !strings.Contains(reconstructed, "plain text version") {
		t.Error("Reconstructed message missing plain text part")
	}

	if !strings.Contains(reconstructed, "HTML version") {
		t.Error("Reconstructed message missing HTML part")
	}
}

func TestStoreMessage_ErrorHandling(t *testing.T) {
	database := setupTestDB(t)
	defer func() { _ = database.Close() }()

	_ = database.Close()

	parsed := &parser.ParsedMessage{
		Subject: "Test",
		Date:    time.Now(),
		From:    []mail.Address{{Address: "test@example.com"}},
	}

	_, err := parser.StoreMessageWithSharedDB(database, database, parsed)
	if err == nil {
		t.Error("Expected error when storing to closed database")
	}
}
