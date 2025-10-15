package delivery_test

import (
	"bufio"
	"strings"
	"testing"

	"go-imap/internal/delivery/parser"
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
