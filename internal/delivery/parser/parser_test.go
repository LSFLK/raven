package parser_test

import (
	"bufio"
	"bytes"
	"database/sql"
	"strings"
	"testing"

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

	msg, err := parser.ParseMessage(bytes.NewReader([]byte(rawEmail)))
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

	msg, err := parser.ParseMessage(bytes.NewReader([]byte(rawEmail)))
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
				if len(msg.Parts) != 3 {
					t.Errorf("Expected 3 parts (root multipart/alternative + 2 content parts), got %d", len(msg.Parts))
				}
				if len(msg.Parts) >= 3 {
					if msg.Parts[0].ContentType != "multipart/alternative" {
						t.Errorf("Expected first part to be multipart/alternative (root container), got %s", msg.Parts[0].ContentType)
					}
					if msg.Parts[1].ContentType != "text/plain" {
						t.Errorf("Expected second part to be text/plain, got %s", msg.Parts[1].ContentType)
					}
					if msg.Parts[2].ContentType != "text/html" {
						t.Errorf("Expected third part to be text/html, got %s", msg.Parts[2].ContentType)
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
				if len(msg.Parts) != 3 {
					t.Errorf("Expected 3 parts (root multipart/mixed + 2 content parts), got %d", len(msg.Parts))
				}
				if len(msg.Parts) >= 3 {
					if msg.Parts[0].ContentType != "multipart/mixed" {
						t.Errorf("Expected first part to be multipart/mixed (root container), got %s", msg.Parts[0].ContentType)
					}
					if msg.Parts[2].ContentType != "application/pdf" {
						t.Errorf("Expected third part (attachment) to be application/pdf, got %s", msg.Parts[2].ContentType)
					}
					if msg.Parts[2].Filename != "document.pdf" {
						t.Errorf("Expected filename 'document.pdf', got '%s'", msg.Parts[2].Filename)
					}
				}
			}},
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
			msg, err := parser.ParseMessage(bytes.NewReader([]byte(rawEmail)))
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

	messageID, err := parser.StoreMessagePerUserWithSharedDBAndS3(database, database, parsed, nil)
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

	messageID, err := parser.StoreMessagePerUserWithSharedDBAndS3(database, database, parsed, nil)
	if err != nil {
		t.Fatalf("Failed to store message: %v", err)
	}

	var partCount int
	err = database.QueryRow("SELECT COUNT(*) FROM message_parts WHERE message_id = ?", messageID).Scan(&partCount)
	if err != nil {
		t.Fatalf("Failed to count parts: %v", err)
	}

	if partCount != 3 {
		t.Errorf("Expected 3 message parts (1 multipart/alternative container + 2 content parts), got %d", partCount)
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

	messageID, err := parser.StoreMessagePerUserWithSharedDBAndS3(database, database, parsed, nil)
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

	messageID, err := parser.StoreMessagePerUserWithSharedDBAndS3(database, database, parsed, nil)
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

	messageID, err := parser.StoreMessagePerUserWithSharedDBAndS3(database, database, parsed, nil)
	if err != nil {
		t.Fatalf("Failed to store message: %v", err)
	}

	reconstructed, err := parser.ReconstructMessageWithSharedDBAndS3(database, database, messageID, nil)
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

	messageID, err := parser.StoreMessagePerUserWithSharedDBAndS3(database, database, parsed, nil)
	if err != nil {
		t.Fatalf("Failed to store message: %v", err)
	}

	reconstructed, err := parser.ReconstructMessageWithSharedDBAndS3(database, database, messageID, nil)
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

	messageID, err := parser.StoreMessagePerUserWithSharedDBAndS3(database, database, parsed, nil)
	if err != nil {
		t.Fatalf("Failed to store message: %v", err)
	}

	reconstructed, err := parser.ReconstructMessageWithSharedDBAndS3(database, database, messageID, nil)
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

	_, err := parser.ReconstructMessageWithSharedDBAndS3(database, database, 99999, nil)
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

	messageID, err := parser.StoreMessagePerUserWithSharedDBAndS3(database, database, parsed, nil)
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

	messageID, err := parser.StoreMessagePerUserWithSharedDBAndS3(database, database, parsed, nil)
	if err != nil {
		t.Fatalf("Failed to store message: %v", err)
	}

	reconstructed, err := parser.ReconstructMessageWithSharedDBAndS3(database, database, messageID, nil)
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

func TestReconstructMessage_WithMultipartRelated(t *testing.T) {
	database := setupTestDB(t)
	defer func() { _ = database.Close() }()

	// Message with multipart/alternative containing text/plain and multipart/related (with HTML + inline image)
	rawMessage := `From: sender@example.com
To: recipient@example.com
Subject: Message with Inline Image
Content-Type: multipart/alternative; boundary="boundary-alt"
MIME-Version: 1.0

--boundary-alt
Content-Type: text/plain; charset=UTF-8

Hi user,
--boundary-alt
Content-Type: multipart/related; boundary="boundary-rel"

--boundary-rel
Content-Type: text/html; charset=UTF-8

<!DOCTYPE html>
<html>
<head>
<meta http-equiv="content-type" content="text/html; charset=UTF-8">
</head>
<body>
<p>Hi user,</p>
<p><img src="cid:image123@example.com" alt=""></p>
</body>
</html>
--boundary-rel
Content-Type: image/png; name="image.png"
Content-Disposition: inline; filename="image.png"
Content-ID: <image123@example.com>
Content-Transfer-Encoding: base64

iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==
--boundary-rel--
--boundary-alt--`

	parsed, err := parser.ParseMIMEMessage(rawMessage)
	if err != nil {
		t.Fatalf("Failed to parse message: %v", err)
	}

	// Verify parsing detected all parts including multipart/related container
	// Expected: multipart/alternative (root), text/plain, multipart/related, text/html, image/png
	if len(parsed.Parts) != 5 {
		t.Fatalf("Expected 5 parts (multipart/alternative root + text/plain + multipart/related + text/html + image/png), got %d", len(parsed.Parts))
	}

	messageID, err := parser.StoreMessagePerUserWithSharedDBAndS3(database, database, parsed, nil)
	if err != nil {
		t.Fatalf("Failed to store message: %v", err)
	}

	reconstructed, err := parser.ReconstructMessageWithSharedDBAndS3(database, database, messageID, nil)
	if err != nil {
		t.Fatalf("Failed to reconstruct message: %v", err)
	}

	// Verify the reconstructed message has proper structure
	if !strings.Contains(reconstructed, "multipart/alternative") {
		t.Error("Reconstructed message missing multipart/alternative")
	}

	if !strings.Contains(reconstructed, "multipart/related") {
		t.Error("Reconstructed message missing multipart/related section")
	}

	if !strings.Contains(reconstructed, "text/html") {
		t.Error("Reconstructed message missing HTML part")
	}

	if !strings.Contains(reconstructed, "image/png") {
		t.Error("Reconstructed message missing inline image")
	}

	// Verify structure: should have text/plain, then multipart/related (containing html + image)
	// The multipart/related should come after text/plain in the alternative
	plainIdx := strings.Index(reconstructed, "text/plain")
	relatedIdx := strings.Index(reconstructed, "multipart/related")
	htmlIdx := strings.Index(reconstructed, "text/html")
	imageIdx := strings.Index(reconstructed, "image/png")

	if plainIdx == -1 || relatedIdx == -1 || htmlIdx == -1 || imageIdx == -1 {
		t.Fatal("Missing expected content types in reconstructed message")
	}

	// multipart/related should come after text/plain
	if relatedIdx < plainIdx {
		t.Error("multipart/related should come after text/plain in alternative")
	}

	// HTML and image should come after multipart/related header
	if htmlIdx < relatedIdx {
		t.Error("HTML part should be inside multipart/related section")
	}

	if imageIdx < relatedIdx {
		t.Error("Image should be inside multipart/related section")
	}

	t.Logf("Reconstructed message structure verified successfully")
}

// TestReconstructMessage_WithMultipleInlineImages tests reconstruction with multiple inline images
func TestReconstructMessage_WithMultipleInlineImages(t *testing.T) {
	database := setupTestDB(t)
	defer func() { _ = database.Close() }()

	rawMessage := `From: sender@example.com
To: recipient@example.com
Subject: Test Multiple Inline Images
MIME-Version: 1.0
Content-Type: multipart/alternative; boundary="alt-boundary"

--alt-boundary
Content-Type: text/plain; charset=UTF-8

Plain text version with two images referenced
--alt-boundary
Content-Type: multipart/related; boundary="rel-boundary"

--rel-boundary
Content-Type: text/html; charset=UTF-8

<html><body><p>Image 1: <img src="cid:image1@test.com"></p><p>Image 2: <img src="cid:image2@test.com"></p></body></html>
--rel-boundary
Content-Type: image/png
Content-ID: <image1@test.com>
Content-Disposition: inline; filename="image1.png"
Content-Transfer-Encoding: base64

iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==
--rel-boundary
Content-Type: image/png
Content-ID: <image2@test.com>
Content-Disposition: inline; filename="image2.png"
Content-Transfer-Encoding: base64

iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8DwHwAFBQIAX8jx0gAAAABJRU5ErkJggg==
--rel-boundary--
--alt-boundary--`

	parsed, err := parser.ParseMIMEMessage(rawMessage)
	if err != nil {
		t.Fatalf("Failed to parse message: %v", err)
	}

	// Verify parsing found all 6 parts: multipart/alternative (root), text/plain, multipart/related container, text/html, image1, image2
	if len(parsed.Parts) != 6 {
		t.Fatalf("Expected 6 parts (multipart/alternative + text/plain + multipart/related + text/html + 2x image/png), got %d", len(parsed.Parts))
	}

	messageID, err := parser.StoreMessagePerUserWithSharedDBAndS3(database, database, parsed, nil)
	if err != nil {
		t.Fatalf("Failed to store message: %v", err)
	}

	reconstructed, err := parser.ReconstructMessageWithSharedDBAndS3(database, database, messageID, nil)
	if err != nil {
		t.Fatalf("Failed to reconstruct message: %v", err)
	}

	// Verify the reconstructed message has proper structure
	if !strings.Contains(reconstructed, "multipart/alternative") {
		t.Error("Reconstructed message missing multipart/alternative")
	}

	if !strings.Contains(reconstructed, "multipart/related") {
		t.Error("Reconstructed message missing multipart/related section")
	}

	if !strings.Contains(reconstructed, "text/html") {
		t.Error("Reconstructed message missing HTML part")
	}

	// Check for BOTH inline images
	if !strings.Contains(reconstructed, "Content-ID: <image1@test.com>") {
		t.Error("Reconstructed message missing first inline image (image1@test.com)")
	}

	if !strings.Contains(reconstructed, "Content-ID: <image2@test.com>") {
		t.Error("Reconstructed message missing second inline image (image2@test.com)")
	}

	if !strings.Contains(reconstructed, `filename="image1.png"`) {
		t.Error("Reconstructed message missing filename for first image")
	}

	if !strings.Contains(reconstructed, `filename="image2.png"`) {
		t.Error("Reconstructed message missing filename for second image")
	}

	// Verify both images are in the multipart/related section
	relatedIdx := strings.Index(reconstructed, "multipart/related")
	image1Idx := strings.Index(reconstructed, "image1@test.com")
	image2Idx := strings.Index(reconstructed, "image2@test.com")

	if relatedIdx == -1 || image1Idx == -1 || image2Idx == -1 {
		t.Fatal("Missing required content types or Content-IDs")
	}

	if image1Idx < relatedIdx {
		t.Error("Image 1 should be inside multipart/related section")
	}

	if image2Idx < relatedIdx {
		t.Error("Image 2 should be inside multipart/related section")
	}

	t.Logf("Successfully reconstructed message with %d inline images", 2)
}

// TestReconstructMessage_MultipartRelatedHTMLNotFirst tests that when HTML is not first
// in a multipart/related block, it gets reordered to be first (the root part per RFC 2387)
func TestReconstructMessage_MultipartRelatedHTMLNotFirst(t *testing.T) {
	database := setupTestDB(t)
	defer func() { _ = database.Close() }()

	// Create a message where the IMAGE comes BEFORE the HTML in multipart/related
	// This is the scenario that causes Thunderbird to show only the image
	// We nest it in multipart/alternative to match the real-world structure
	rawMessage := `From: sender@example.com
To: recipient@example.com
Subject: HTML Not First Test
MIME-Version: 1.0
Content-Type: multipart/alternative; boundary="alt-boundary"

--alt-boundary
Content-Type: text/plain; charset=UTF-8

Plain text version
--alt-boundary
Content-Type: multipart/related; boundary="rel-boundary"

--rel-boundary
Content-Type: image/png; name="logo.png"
Content-Transfer-Encoding: base64
Content-ID: <logo@example.com>
Content-Disposition: inline; filename="logo.png"

iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==
--rel-boundary
Content-Type: text/html; charset=UTF-8

<html><body><h1>Welcome</h1><img src="cid:logo@example.com"/></body></html>
--rel-boundary--
--alt-boundary--
`

	msg, err := parser.ParseMIMEMessage(rawMessage)
	if err != nil {
		t.Fatalf("Failed to parse message: %v", err)
	}

	messageID, err := parser.StoreMessagePerUserWithSharedDBAndS3(database, database, msg, nil)
	if err != nil {
		t.Fatalf("Failed to store message: %v", err)
	}

	reconstructed, err := parser.ReconstructMessageWithSharedDBAndS3(database, database, messageID, nil)
	if err != nil {
		t.Fatalf("Failed to reconstruct message: %v", err)
	}

	// Verify the reconstructed message has multipart/related
	if !strings.Contains(reconstructed, "multipart/related") {
		t.Error("Reconstructed message missing multipart/related")
	}

	// Find the positions of HTML and image parts in the reconstructed message
	// We need to find them AFTER the multipart/related boundary
	relatedBoundaryPos := strings.Index(reconstructed, "multipart/related")
	if relatedBoundaryPos == -1 {
		t.Fatal("Could not find multipart/related in reconstructed message")
	}

	// Get the part of the message after the multipart/related header
	afterRelated := reconstructed[relatedBoundaryPos:]
	
	htmlPos := strings.Index(afterRelated, "text/html")
	imagePos := strings.Index(afterRelated, "image/png")

	if htmlPos == -1 {
		t.Fatal("HTML part not found in reconstructed message")
	}
	if imagePos == -1 {
		t.Fatal("Image part not found in reconstructed message")
	}

	// The critical check: HTML should come BEFORE the image in the reconstructed message
	// This ensures that RFC 2387-compliant clients will use HTML as the root, not the image
	if htmlPos > imagePos {
		t.Errorf("HTML part should come BEFORE image in multipart/related (HTML at %d, image at %d)", htmlPos, imagePos)
		t.Logf("This means Thunderbird will incorrectly show the image as the main content instead of the HTML")
	} else {
		t.Logf("✓ HTML part correctly comes first in multipart/related (HTML at %d, image at %d)", htmlPos, imagePos)
	}

	// Also verify that both parts are present
	if !strings.Contains(reconstructed, "<html>") {
		t.Error("HTML content missing from reconstruction")
	}
	if !strings.Contains(reconstructed, "Content-ID: <logo@example.com>") {
		t.Error("Image Content-ID missing from reconstruction")
	}
}

func TestParseGmailInlineImageStructure(t *testing.T) {
	database := setupTestDB(t)
	defer func() { _ = database.Close() }()

	// This is the structure Gmail uses for inline images:
	// multipart/related (root)
	//   ├── multipart/alternative (child)
	//   │     ├── text/plain
	//   │     └── text/html
	//   └── image/png (inline attachment with Content-ID)
	
	rawMessage := `From: sender@example.com
To: recipient@example.com
Subject: Gmail Inline Image Test
MIME-Version: 1.0
Content-Type: multipart/related; boundary="boundary-related"

--boundary-related
Content-Type: multipart/alternative; boundary="boundary-alt"

--boundary-alt
Content-Type: text/plain; charset="UTF-8"

Hi user,
[image: logo.png]

--boundary-alt
Content-Type: text/html; charset="UTF-8"

<div dir=3D"ltr">Hi user,<div><img src=3D"cid:logo@example.com" alt=3D"Screens=
hot 2026-01-04 at 11.33.43.png" width=3D"562" height=3D"556"><br></div></di=
v><br><div class=3D"gmail_quote gmail_quote_container"><div dir=3D"ltr" cla=
ss=3D"gmail_attr">On Tue, 20 Jan 2026 at 12:42, Aravinda H.W.K. &lt;<a href=
=3D"mailto:user1@aravindahwk.org">user1@aravindahwk.org</a>&gt; wrote:<br><=
/div><blockquote class=3D"gmail_quote" style=3D"margin:0px 0px 0px 0.8ex;bo=
rder-left:1px solid rgb(204,204,204);padding-left:1ex">Hi user,<br>
</blockquote></div>

--boundary-alt--

--boundary-related
Content-Type: image/png; name="logo.png"
Content-Disposition: attachment; filename="logo.png"
Content-Transfer-Encoding: base64
X-Attachment-Id: logo123
Content-ID: <logo@example.com>

iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==
--boundary-related--
`

	parsed, err := parser.ParseMIMEMessage(rawMessage)
	if err != nil {
		t.Fatalf("Failed to parse Gmail message: %v", err)
	}

	// Verify the structure: should have 5 parts
	// 1. multipart/related (root container)
	// 2. multipart/alternative (container, child of multipart/related)
	// 3. text/plain (child of multipart/alternative)
	// 4. text/html (child of multipart/alternative)
	// 5. image/png (child of multipart/related)
	
	if len(parsed.Parts) != 5 {
		t.Fatalf("Expected 5 parts, got %d", len(parsed.Parts))
		for i, part := range parsed.Parts {
			t.Logf("  Part %d: %s (parent: %v)", i, part.ContentType, part.ParentPartID)
		}
	}

	// Verify root: multipart/related
	if parsed.Parts[0].ContentType != "multipart/related" {
		t.Errorf("Part 0 should be multipart/related, got %s", parsed.Parts[0].ContentType)
	}
	if parsed.Parts[0].ParentPartID.Valid {
		t.Errorf("Part 0 (root) should have no parent")
	}

	// Verify part 1: multipart/alternative (child of part 0)
	if parsed.Parts[1].ContentType != "multipart/alternative" {
		t.Errorf("Part 1 should be multipart/alternative, got %s", parsed.Parts[1].ContentType)
	}
	if !parsed.Parts[1].ParentPartID.Valid || parsed.Parts[1].ParentPartID.Int64 != 0 {
		t.Errorf("Part 1 should have parent index 0, got %v", parsed.Parts[1].ParentPartID)
	}

	// Verify part 2: text/plain (child of part 1)
	if parsed.Parts[2].ContentType != "text/plain" {
		t.Errorf("Part 2 should be text/plain, got %s", parsed.Parts[2].ContentType)
	}

	// Verify part 3: text/html (child of part 1)
	if parsed.Parts[3].ContentType != "text/html" {
		t.Errorf("Part 3 should be text/html, got %s", parsed.Parts[3].ContentType)
	}

	// Verify part 4: image/png (child of part 0, the multipart/related root)
	if parsed.Parts[4].ContentType != "image/png" {
		t.Errorf("Part 4 should be image/png, got %s", parsed.Parts[4].ContentType)
	}
	if !parsed.Parts[4].ParentPartID.Valid || parsed.Parts[4].ParentPartID.Int64 != 0 {
		t.Errorf("Part 4 (image) should be child of part 0 (multipart/related), got parent %v", parsed.Parts[4].ParentPartID)
	}
	if parsed.Parts[4].ContentID != "<logo@example.com>" {
		t.Errorf("Part 4 should have Content-ID <logo@example.com>, got %s", parsed.Parts[4].ContentID)
	}

	// Store the message
	messageID, err := parser.StoreMessagePerUserWithSharedDBAndS3(database, database, parsed, nil)
	if err != nil {
		t.Fatalf("Failed to store message: %v", err)
	}

	// Reconstruct and verify
	reconstructed, err := parser.ReconstructMessageWithSharedDBAndS3(database, database, messageID, nil)
	if err != nil {
		t.Fatalf("Failed to reconstruct message: %v", err)
	}

	// The reconstructed message should maintain the multipart/related structure
	if !strings.Contains(reconstructed, "multipart/related") {
		t.Error("Reconstructed message missing multipart/related")
	}

	if !strings.Contains(reconstructed, "multipart/alternative") {
		t.Error("Reconstructed message missing multipart/alternative (should be nested in multipart/related)")
	}

	if !strings.Contains(reconstructed, "Content-ID: <logo@example.com>") {
		t.Error("Reconstructed message missing Content-ID for inline image")
	}

	// Check for cid reference (may be quoted-printable encoded as =3D)
	if !strings.Contains(reconstructed, `src="cid:logo@example.com"`) && !strings.Contains(reconstructed, `src=3D"cid:logo@example.com"`) {
		t.Error("Reconstructed message missing cid reference in HTML")
	}

	t.Logf("Successfully parsed and reconstructed Gmail multipart/related structure with %d parts", len(parsed.Parts))
}

// TestReconstructedMultipartRelatedOrder verifies that multipart/related has correct part order
func TestReconstructedMultipartRelatedOrder(t *testing.T) {
	database := setupTestDB(t)
	defer func() { _ = database.Close() }()

	// Gmail structure: multipart/related with multipart/alternative BEFORE image
	rawMessage := `From: sender@example.com
To: recipient@example.com
Subject: Order Test
Content-Type: multipart/related; boundary="boundary-rel"

--boundary-rel
Content-Type: multipart/alternative; boundary="boundary-alt"

--boundary-alt
Content-Type: text/plain

Text body

--boundary-alt
Content-Type: text/html

<html><body>HTML body <img src="cid:image123"></body></html>
--boundary-alt--
--boundary-rel
Content-Type: image/png
Content-ID: <image123>

imagedata
--boundary-rel--
`

	parsed, err := parser.ParseMIMEMessage(rawMessage)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	messageID, err := parser.StoreMessagePerUserWithSharedDBAndS3(database, database, parsed, nil)
	if err != nil {
		t.Fatalf("Failed to store: %v", err)
	}

	reconstructed, err := parser.ReconstructMessageWithSharedDBAndS3(database, database, messageID, nil)
	if err != nil {
		t.Fatalf("Failed to reconstruct: %v", err)
	}

	// Find positions of key parts in the reconstructed message
	relatedPos := strings.Index(reconstructed, "multipart/related")
	alternativePos := strings.Index(reconstructed, "multipart/alternative")
	imagePos := strings.Index(reconstructed, "Content-ID: <image123>")
	textPos := strings.Index(reconstructed, "Text body")
	htmlPos := strings.Index(reconstructed, "HTML body")

	if relatedPos == -1 {
		t.Fatal("Missing multipart/related")
	}
	if alternativePos == -1 {
		t.Fatal("Missing multipart/alternative")
	}
	if imagePos == -1 {
		t.Fatal("Missing image with Content-ID")
	}

	// CRITICAL: multipart/alternative must come BEFORE image in multipart/related
	// This ensures the body (alternative) is the root, not the image
	if alternativePos < relatedPos {
		t.Error("multipart/alternative should be nested inside multipart/related")
	}
	
	if imagePos < alternativePos {
		t.Errorf("Image should come AFTER multipart/alternative in multipart/related")
		t.Errorf("  multipart/alternative at position %d", alternativePos)
		t.Errorf("  image at position %d", imagePos)
		t.Error("This will cause mail clients to show only the image instead of the HTML body!")
	} else {
		t.Logf("✓ Correct order: multipart/alternative (%d) before image (%d)", alternativePos, imagePos)
	}

	// Verify text content is present
	if textPos == -1 || htmlPos == -1 {
		t.Error("Missing text or HTML content")
	}

	// The structure should be:
	// multipart/related
	//   multipart/alternative  <- FIRST (root)
	//     text/plain
	//     text/html
	//   image/png             <- SECOND (resource)
	t.Logf("Reconstructed structure order verified successfully")
}
