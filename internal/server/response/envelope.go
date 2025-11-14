package response

import (
	"fmt"
	"strings"
)

// BuildEnvelope builds an ENVELOPE structure from a raw message
// ENVELOPE format: (date subject from sender reply-to to cc bcc in-reply-to message-id)
// This follows RFC 3501 Section 7.4.2 ENVELOPE structure
func BuildEnvelope(rawMsg string) string {
	// Extract all required headers
	date := extractHeader(rawMsg, "Date")
	subject := extractHeader(rawMsg, "Subject")
	from := extractHeader(rawMsg, "From")
	sender := extractHeader(rawMsg, "Sender")
	replyTo := extractHeader(rawMsg, "Reply-To")
	to := extractHeader(rawMsg, "To")
	cc := extractHeader(rawMsg, "Cc")
	bcc := extractHeader(rawMsg, "Bcc")
	inReplyTo := extractHeader(rawMsg, "In-Reply-To")
	messageID := extractHeader(rawMsg, "Message-ID")

	// Apply RFC defaults
	// If sender is empty, use from address
	if sender == "" {
		sender = from
	}
	// If reply-to is empty, use from address
	if replyTo == "" {
		replyTo = from
	}

	// Build ENVELOPE structure according to RFC 3501
	envelope := fmt.Sprintf("ENVELOPE (%s %s %s %s %s %s %s %s %s %s)",
		QuoteOrNIL(date),
		QuoteOrNIL(subject),
		parseAddressList(from),
		parseAddressList(sender),
		parseAddressList(replyTo),
		parseAddressList(to),
		parseAddressList(cc),
		parseAddressList(bcc),
		QuoteOrNIL(inReplyTo),
		QuoteOrNIL(messageID),
	)

	return envelope
}

// extractHeader extracts a header value from a raw message
// Handles header folding (continuation lines) per RFC 2822
func extractHeader(rawMsg string, headerName string) string {
	lines := strings.Split(rawMsg, "\n")
	headerNameUpper := strings.ToUpper(headerName)
	var headerValue strings.Builder
	inHeader := false

	for _, line := range lines {
		line = strings.TrimRight(line, "\r")

		// Empty line marks end of headers
		if line == "" {
			break
		}

		// Check if this is a continuation line (starts with space or tab)
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
			if inHeader {
				headerValue.WriteString(" ")
				headerValue.WriteString(strings.TrimSpace(line))
			}
			continue
		}

		// New header line - parse header name
		colonIdx := strings.Index(line, ":")
		if colonIdx != -1 {
			currentHeader := strings.TrimSpace(line[:colonIdx])
			if strings.ToUpper(currentHeader) == headerNameUpper {
				inHeader = true
				headerValue.WriteString(strings.TrimSpace(line[colonIdx+1:]))
			} else {
				inHeader = false
			}
		}
	}

	return headerValue.String()
}

// QuoteOrNIL quotes a string for IMAP response or returns NIL if empty
// Exported for reuse in other response builders
func QuoteOrNIL(str string) string {
	if str == "" {
		return "NIL"
	}
	// Escape special characters per IMAP spec
	str = strings.ReplaceAll(str, "\\", "\\\\")
	str = strings.ReplaceAll(str, "\"", "\\\"")
	return fmt.Sprintf("\"%s\"", str)
}

// parseAddressList parses an address header into IMAP address list format
// Address list format: ((name route mailbox host) (name route mailbox host) ...) or NIL
// Per RFC 3501 Section 7.4.2
func parseAddressList(addresses string) string {
	if addresses == "" {
		return "NIL"
	}

	// Simple parser - split by comma for multiple addresses
	addrs := strings.Split(addresses, ",")
	var addrStructs []string

	for _, addr := range addrs {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}

		// Parse address formats:
		// "Name <email@domain.com>" or just "email@domain.com"
		name := ""
		email := addr

		// Extract name part if present
		if strings.Contains(addr, "<") && strings.Contains(addr, ">") {
			start := strings.Index(addr, "<")
			end := strings.Index(addr, ">")
			name = strings.TrimSpace(addr[:start])
			email = addr[start+1 : end]
			// Remove quotes from name if present
			name = strings.Trim(name, "\"")
		}

		// Parse email into mailbox@host
		mailbox := email
		host := ""
		if strings.Contains(email, "@") {
			parts := strings.SplitN(email, "@", 2)
			mailbox = parts[0]
			host = parts[1]
		}

		// Build address structure: (name route mailbox host)
		// Note: route is always NIL in modern email (obsolete per RFC 2822)
		addrStruct := fmt.Sprintf("(%s NIL %s %s)",
			QuoteOrNIL(name),
			QuoteOrNIL(mailbox),
			QuoteOrNIL(host),
		)
		addrStructs = append(addrStructs, addrStruct)
	}

	if len(addrStructs) == 0 {
		return "NIL"
	}

	return "(" + strings.Join(addrStructs, " ") + ")"
}
