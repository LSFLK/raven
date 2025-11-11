package utils

import (
	"fmt"
	"strings"
)

// BuildEnvelope builds an ENVELOPE response from a raw message
// ENVELOPE: (date subject from sender reply-to to cc bcc in-reply-to message-id)
func BuildEnvelope(rawMsg string) string {
	// Extract headers
	date := ExtractHeader(rawMsg, "Date")
	subject := ExtractHeader(rawMsg, "Subject")
	from := ExtractHeader(rawMsg, "From")
	sender := ExtractHeader(rawMsg, "Sender")
	replyTo := ExtractHeader(rawMsg, "Reply-To")
	to := ExtractHeader(rawMsg, "To")
	cc := ExtractHeader(rawMsg, "Cc")
	bcc := ExtractHeader(rawMsg, "Bcc")
	inReplyTo := ExtractHeader(rawMsg, "In-Reply-To")
	messageID := ExtractHeader(rawMsg, "Message-ID")

	// If sender is empty, use from
	if sender == "" {
		sender = from
	}
	// If reply-to is empty, use from
	if replyTo == "" {
		replyTo = from
	}

	// Build ENVELOPE structure
	envelope := fmt.Sprintf("ENVELOPE (%s %s %s %s %s %s %s %s %s %s)",
		QuoteOrNIL(date),
		QuoteOrNIL(subject),
		ParseAddressList(from),
		ParseAddressList(sender),
		ParseAddressList(replyTo),
		ParseAddressList(to),
		ParseAddressList(cc),
		ParseAddressList(bcc),
		QuoteOrNIL(inReplyTo),
		QuoteOrNIL(messageID),
	)

	return envelope
}

// ExtractHeader extracts a header value from a raw message
func ExtractHeader(rawMsg string, headerName string) string {
	lines := strings.Split(rawMsg, "\n")
	headerNameUpper := strings.ToUpper(headerName)
	var headerValue strings.Builder
	inHeader := false

	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			break // End of headers
		}

		// Check if continuation line
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
			if inHeader {
				headerValue.WriteString(" ")
				headerValue.WriteString(strings.TrimSpace(line))
			}
			continue
		}

		// New header line
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

// QuoteOrNIL quotes a string or returns NIL if empty
func QuoteOrNIL(str string) string {
	if str == "" {
		return "NIL"
	}
	// Escape quotes and backslashes in the string
	str = strings.ReplaceAll(str, "\\", "\\\\")
	str = strings.ReplaceAll(str, "\"", "\\\"")
	return fmt.Sprintf("\"%s\"", str)
}

// ParseAddressList parses an address header into IMAP address list format
// Address list: ((name route mailbox host) ...) or NIL
func ParseAddressList(addresses string) string {
	if addresses == "" {
		return "NIL"
	}

	// Simple parser - split by comma
	addrs := strings.Split(addresses, ",")
	var addrStructs []string

	for _, addr := range addrs {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}

		// Parse "Name <email@domain.com>" or just "email@domain.com"
		name := ""
		email := addr

		// Check for name part
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
		// route is always NIL in modern email
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
