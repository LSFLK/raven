package parser

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/mail"
	"strings"
	"time"
)

// Message represents a parsed email message
type Message struct {
	From        string
	To          []string
	Subject     string
	Date        time.Time
	MessageID   string
	Headers     map[string]string
	Body        string
	RawMessage  string
	Size        int64
}

// ParseMessage parses an email message from a reader
func ParseMessage(r io.Reader) (*Message, error) {
	// Read entire message
	var buf bytes.Buffer
	tee := io.TeeReader(r, &buf)

	// Parse using net/mail
	msg, err := mail.ReadMessage(tee)
	if err != nil {
		return nil, fmt.Errorf("failed to parse message: %w", err)
	}

	// Read remaining data to ensure buf has complete message
	io.Copy(&buf, r)
	rawMessage := buf.String()

	// Extract headers
	headers := make(map[string]string)
	for key, values := range msg.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	// Extract From
	from := msg.Header.Get("From")
	if from == "" {
		return nil, fmt.Errorf("missing From header")
	}

	// Extract To (can be multiple)
	to := extractRecipients(msg.Header)
	if len(to) == 0 {
		return nil, fmt.Errorf("missing To/Cc/Bcc headers")
	}

	// Extract Subject
	subject := msg.Header.Get("Subject")

	// Extract Date
	dateStr := msg.Header.Get("Date")
	var msgDate time.Time
	if dateStr != "" {
		msgDate, err = mail.ParseDate(dateStr)
		if err != nil {
			// If date parsing fails, use current time
			msgDate = time.Now()
		}
	} else {
		msgDate = time.Now()
	}

	// Extract Message-ID
	messageID := msg.Header.Get("Message-Id")
	if messageID == "" {
		// Generate a message ID if not present
		messageID = generateMessageID()
	}

	// Read body
	bodyBytes, err := io.ReadAll(msg.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read message body: %w", err)
	}

	return &Message{
		From:       from,
		To:         to,
		Subject:    subject,
		Date:       msgDate,
		MessageID:  messageID,
		Headers:    headers,
		Body:       string(bodyBytes),
		RawMessage: rawMessage,
		Size:       int64(len(rawMessage)),
	}, nil
}

// ParseMessageFromBytes parses an email message from bytes
func ParseMessageFromBytes(data []byte) (*Message, error) {
	return ParseMessage(bytes.NewReader(data))
}

// extractRecipients extracts all recipient addresses from To, Cc, and Bcc headers
func extractRecipients(header mail.Header) []string {
	var recipients []string

	// Extract from To
	if to := header.Get("To"); to != "" {
		recipients = append(recipients, parseAddressList(to)...)
	}

	// Extract from Cc
	if cc := header.Get("Cc"); cc != "" {
		recipients = append(recipients, parseAddressList(cc)...)
	}

	// Extract from Bcc
	if bcc := header.Get("Bcc"); bcc != "" {
		recipients = append(recipients, parseAddressList(bcc)...)
	}

	return recipients
}

// parseAddressList parses a comma-separated list of email addresses
func parseAddressList(addrList string) []string {
	var result []string

	addresses, err := mail.ParseAddressList(addrList)
	if err != nil {
		// Fallback to simple split if parsing fails
		parts := strings.Split(addrList, ",")
		for _, part := range parts {
			addr := strings.TrimSpace(part)
			if addr != "" {
				result = append(result, addr)
			}
		}
		return result
	}

	for _, addr := range addresses {
		result = append(result, addr.Address)
	}

	return result
}

// generateMessageID generates a unique Message-ID
func generateMessageID() string {
	return fmt.Sprintf("<%d@raven-delivery>", time.Now().UnixNano())
}

// ValidateMessage performs basic validation on a message
func ValidateMessage(msg *Message, maxSize int64) error {
	if msg.From == "" {
		return fmt.Errorf("message missing From header")
	}

	if len(msg.To) == 0 {
		return fmt.Errorf("message missing recipients")
	}

	if msg.Size > maxSize {
		return fmt.Errorf("message size (%d bytes) exceeds maximum allowed size (%d bytes)", msg.Size, maxSize)
	}

	return nil
}

// ExtractEnvelopeRecipient extracts the email address from an envelope recipient
func ExtractEnvelopeRecipient(recipient string) (string, error) {
	// Handle various formats:
	// - user@domain.com
	// - <user@domain.com>
	// - "Name" <user@domain.com>

	recipient = strings.TrimSpace(recipient)

	// Check if already in simple format
	if !strings.Contains(recipient, "<") && !strings.Contains(recipient, ">") {
		if isValidEmail(recipient) {
			return recipient, nil
		}
		return "", fmt.Errorf("invalid email format: %s", recipient)
	}

	// Parse using net/mail
	addr, err := mail.ParseAddress(recipient)
	if err != nil {
		return "", fmt.Errorf("failed to parse recipient: %w", err)
	}

	return addr.Address, nil
}

// isValidEmail performs basic email validation
func isValidEmail(email string) bool {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}

	localPart := parts[0]
	domain := parts[1]

	if localPart == "" || domain == "" {
		return false
	}

	if !strings.Contains(domain, ".") {
		return false
	}

	return true
}

// ExtractLocalPart extracts the local part (username) from an email address
func ExtractLocalPart(email string) (string, error) {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid email format: %s", email)
	}
	return parts[0], nil
}

// ExtractDomain extracts the domain from an email address
func ExtractDomain(email string) (string, error) {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid email format: %s", email)
	}
	return parts[1], nil
}

// ReadDataCommand reads the message data from an LMTP DATA command
func ReadDataCommand(r *bufio.Reader, maxSize int64) ([]byte, error) {
	var buf bytes.Buffer
	var size int64

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("error reading data: %w", err)
		}

		// Check for end of data marker (single dot on a line)
		if line == ".\r\n" || line == ".\n" {
			break
		}

		// Handle dot-stuffing (RFC 2821 section 4.5.2)
		if strings.HasPrefix(line, "..") {
			line = line[1:] // Remove leading dot
		}

		// Write line to buffer
		n, err := buf.WriteString(line)
		if err != nil {
			return nil, fmt.Errorf("error writing to buffer: %w", err)
		}

		size += int64(n)

		// Check size limit
		if size > maxSize {
			return nil, fmt.Errorf("message size exceeds maximum allowed size (%d bytes)", maxSize)
		}
	}

	return buf.Bytes(), nil
}
