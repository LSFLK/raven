package parser

import (
	"bufio"
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"time"

	"go-imap/internal/db"
)

// Message represents a parsed email message (simple format for LMTP)
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

// ParsedMessage represents a parsed email message with full MIME structure
type ParsedMessage struct {
	MessageID   int64
	Subject     string
	From        []mail.Address
	To          []mail.Address
	Cc          []mail.Address
	Bcc         []mail.Address
	Date        time.Time
	InReplyTo   string
	References  string
	Headers     []MessageHeader
	Parts       []MessagePart
	RawMessage  string
	SizeBytes   int64
}

// MessageHeader represents a single email header
type MessageHeader struct {
	Name     string
	Value    string
	Sequence int
}

// MessagePart represents a single MIME part
type MessagePart struct {
	PartNumber              int
	ParentPartID            sql.NullInt64
	ContentType             string
	ContentDisposition      string
	ContentTransferEncoding string
	Charset                 string
	Filename                string
	BlobID                  sql.NullInt64
	TextContent             string
	SizeBytes               int64
}

// ParseMessage parses an email message from a reader (simple format for LMTP)
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

// ParseMIMEMessage parses a raw MIME message into structured components for database storage
func ParseMIMEMessage(rawMessage string) (*ParsedMessage, error) {
	parsed := &ParsedMessage{
		RawMessage: rawMessage,
		SizeBytes:  int64(len(rawMessage)),
	}

	// Parse the email using net/mail
	msg, err := mail.ReadMessage(strings.NewReader(rawMessage))
	if err != nil {
		return nil, fmt.Errorf("failed to parse message: %v", err)
	}

	// Extract ALL headers from the raw message to preserve order and multi-line headers
	parsed.Headers = extractAllHeaders(rawMessage)

	// Extract specific headers for backward compatibility
	parsed.Subject = msg.Header.Get("Subject")
	parsed.InReplyTo = msg.Header.Get("In-Reply-To")
	parsed.References = msg.Header.Get("References")

	// Parse date
	dateStr := msg.Header.Get("Date")
	if dateStr != "" {
		parsed.Date, _ = mail.ParseDate(dateStr)
	}
	if parsed.Date.IsZero() {
		parsed.Date = time.Now()
	}

	// Parse addresses
	if fromList, err := mail.ParseAddressList(msg.Header.Get("From")); err == nil && fromList != nil {
		for _, addr := range fromList {
			if addr != nil {
				parsed.From = append(parsed.From, *addr)
			}
		}
	}
	if toList, err := mail.ParseAddressList(msg.Header.Get("To")); err == nil && toList != nil {
		for _, addr := range toList {
			if addr != nil {
				parsed.To = append(parsed.To, *addr)
			}
		}
	}
	if ccList, err := mail.ParseAddressList(msg.Header.Get("Cc")); err == nil && ccList != nil {
		for _, addr := range ccList {
			if addr != nil {
				parsed.Cc = append(parsed.Cc, *addr)
			}
		}
	}
	if bccList, err := mail.ParseAddressList(msg.Header.Get("Bcc")); err == nil && bccList != nil {
		for _, addr := range bccList {
			if addr != nil {
				parsed.Bcc = append(parsed.Bcc, *addr)
			}
		}
	}

	// Parse MIME parts
	contentType := msg.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "text/plain; charset=us-ascii"
	}

	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = "text/plain"
		params = map[string]string{"charset": "us-ascii"}
	}

	// Handle multipart messages
	if strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		if boundary != "" {
			parsed.Parts, err = parseMultipart(msg.Body, boundary, 0, sql.NullInt64{})
			if err != nil {
				return nil, fmt.Errorf("failed to parse multipart: %v", err)
			}
		}
	} else {
		// Single part message
		bodyBytes, err := io.ReadAll(msg.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read body: %v", err)
		}

		charset := params["charset"]
		if charset == "" {
			charset = "us-ascii"
		}

		encoding := msg.Header.Get("Content-Transfer-Encoding")

		part := MessagePart{
			PartNumber:              1,
			ContentType:             mediaType,
			ContentTransferEncoding: encoding,
			Charset:                 charset,
			TextContent:             string(bodyBytes),
			SizeBytes:               int64(len(bodyBytes)),
		}

		parsed.Parts = append(parsed.Parts, part)
	}

	return parsed, nil
}

// parseMultipart recursively parses multipart MIME messages
func parseMultipart(body io.Reader, boundary string, depth int, parentPartID sql.NullInt64) ([]MessagePart, error) {
	var parts []MessagePart
	partNumber := 1

	mr := multipart.NewReader(body, boundary)
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return parts, err
		}

		// Read part content
		content, err := io.ReadAll(p)
		if err != nil {
			continue
		}

		// Parse content type
		contentType := p.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "text/plain; charset=us-ascii"
		}

		mediaType, params, err := mime.ParseMediaType(contentType)
		if err != nil {
			mediaType = "text/plain"
			params = map[string]string{"charset": "us-ascii"}
		}

		charset := params["charset"]
		encoding := p.Header.Get("Content-Transfer-Encoding")
		disposition := p.Header.Get("Content-Disposition")
		filename := p.FileName()

		part := MessagePart{
			PartNumber:              partNumber,
			ParentPartID:            parentPartID,
			ContentType:             mediaType,
			ContentDisposition:      disposition,
			ContentTransferEncoding: encoding,
			Charset:                 charset,
			Filename:                filename,
			TextContent:             string(content),
			SizeBytes:               int64(len(content)),
		}

		// If this is a multipart, recursively parse it
		if strings.HasPrefix(mediaType, "multipart/") && params["boundary"] != "" {
			// This part is a container, store it but don't store content
			part.TextContent = ""
			parts = append(parts, part)

			// Parse sub-parts (depth-first)
			subParts, err := parseMultipart(bytes.NewReader(content), params["boundary"], depth+1, sql.NullInt64{Valid: true, Int64: int64(partNumber)})
			if err == nil {
				parts = append(parts, subParts...)
			}
		} else {
			parts = append(parts, part)
		}

		partNumber++
	}

	return parts, nil
}

// StoreMessage stores a parsed message in the database
func StoreMessage(database *sql.DB, parsed *ParsedMessage) (int64, error) {
	// Create message record
	messageID, err := db.CreateMessage(database, parsed.Subject, parsed.InReplyTo, parsed.References, parsed.Date, parsed.SizeBytes)
	if err != nil {
		return 0, fmt.Errorf("failed to create message: %v", err)
	}

	parsed.MessageID = messageID

	// Store all headers
	for _, header := range parsed.Headers {
		if err := db.AddMessageHeader(database, messageID, header.Name, header.Value, header.Sequence); err != nil {
			return 0, fmt.Errorf("failed to store header %s: %v", header.Name, err)
		}
	}

	// Store addresses
	if err := storeAddresses(database, messageID, "from", parsed.From); err != nil {
		return 0, fmt.Errorf("failed to store from addresses: %v", err)
	}
	if err := storeAddresses(database, messageID, "to", parsed.To); err != nil {
		return 0, fmt.Errorf("failed to store to addresses: %v", err)
	}
	if err := storeAddresses(database, messageID, "cc", parsed.Cc); err != nil {
		return 0, fmt.Errorf("failed to store cc addresses: %v", err)
	}
	if err := storeAddresses(database, messageID, "bcc", parsed.Bcc); err != nil {
		return 0, fmt.Errorf("failed to store bcc addresses: %v", err)
	}

	// Store message parts
	for _, part := range parsed.Parts {
		var blobID sql.NullInt64

		// Store large content or attachments in blobs
		if len(part.TextContent) > 1024 || part.Filename != "" {
			id, err := db.StoreBlob(database, part.TextContent)
			if err == nil {
				blobID = sql.NullInt64{Valid: true, Int64: id}
				// Clear text content since it's in blob
				part.TextContent = ""
			}
		}

		_, err := db.AddMessagePart(
			database,
			messageID,
			part.PartNumber,
			part.ParentPartID,
			part.ContentType,
			part.ContentDisposition,
			part.ContentTransferEncoding,
			part.Charset,
			part.Filename,
			blobID,
			part.TextContent,
			part.SizeBytes,
		)
		if err != nil {
			return 0, fmt.Errorf("failed to store message part: %v", err)
		}
	}

	return messageID, nil
}

// storeAddresses stores email addresses in the database
func storeAddresses(database *sql.DB, messageID int64, addressType string, addresses []mail.Address) error {
	for i, addr := range addresses {
		err := db.AddAddress(database, messageID, addressType, addr.Name, addr.Address, i)
		if err != nil {
			return err
		}
	}
	return nil
}

// ReconstructMessage reconstructs the raw message from database parts
func ReconstructMessage(database *sql.DB, messageID int64) (string, error) {
	// Get message parts
	parts, err := db.GetMessageParts(database, messageID)
	if err != nil {
		return "", fmt.Errorf("failed to get message parts: %v", err)
	}

	if len(parts) == 0 {
		return "", fmt.Errorf("no message parts found")
	}

	// Get ALL stored headers
	headers, err := db.GetMessageHeaders(database, messageID)
	if err != nil {
		// If we can't get headers, try to reconstruct from addresses and metadata
		fmt.Printf("WARNING: Failed to get message headers for message %d: %v\n", messageID, err)
		headers = []map[string]string{}
	}

	// Build message headers
	var buf bytes.Buffer

	// If we have stored headers, use them
	if len(headers) > 0 {
		// Write all headers in original order
		for _, header := range headers {
			buf.WriteString(fmt.Sprintf("%s: %s\r\n", header["name"], header["value"]))
		}
	} else {
		// Fallback: Build minimal headers from addresses and metadata for old messages
		fmt.Printf("WARNING: No stored headers found for message %d, using fallback\n", messageID)

		// Get addresses
		fromAddrs, _ := db.GetMessageAddresses(database, messageID, "from")
		toAddrs, _ := db.GetMessageAddresses(database, messageID, "to")
		ccAddrs, _ := db.GetMessageAddresses(database, messageID, "cc")

		// Get message metadata
		var subject string
		var date time.Time
		err = database.QueryRow("SELECT subject, date FROM messages WHERE id = ?", messageID).Scan(&subject, &date)
		if err != nil {
			return "", fmt.Errorf("failed to get message metadata: %v", err)
		}

		if len(fromAddrs) > 0 {
			buf.WriteString(fmt.Sprintf("From: %s\r\n", strings.Join(fromAddrs, ", ")))
		}
		if len(toAddrs) > 0 {
			buf.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(toAddrs, ", ")))
		}
		if len(ccAddrs) > 0 {
			buf.WriteString(fmt.Sprintf("Cc: %s\r\n", strings.Join(ccAddrs, ", ")))
		}

		buf.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
		buf.WriteString(fmt.Sprintf("Date: %s\r\n", date.Format(time.RFC1123Z)))
	}

	// Check if we need to add Content-Type headers (for backward compatibility with old messages)
	// New messages will have these headers already stored in message_headers
	hasContentType := false
	var storedContentType string
	for _, header := range headers {
		if strings.EqualFold(header["name"], "Content-Type") {
			hasContentType = true
			storedContentType = header["value"]
			break
		}
	}

	// Extract boundaries from stored Content-Type header if present
	boundaryAlt := "boundary-alt"
	boundaryMixed := "boundary-mixed"
	if storedContentType != "" {
		// Try to extract boundary from Content-Type
		if idx := strings.Index(storedContentType, "boundary="); idx != -1 {
			boundaryStr := storedContentType[idx+9:] // Skip "boundary="
			// Remove quotes if present
			boundaryStr = strings.Trim(boundaryStr, "\"")
			// Get only the boundary value (up to semicolon or end)
			if semiIdx := strings.Index(boundaryStr, ";"); semiIdx != -1 {
				boundaryStr = boundaryStr[:semiIdx]
			}
			boundaryStr = strings.TrimSpace(boundaryStr)

			// Determine which type of boundary this is based on Content-Type
			if strings.Contains(storedContentType, "multipart/alternative") {
				boundaryAlt = boundaryStr
			} else if strings.Contains(storedContentType, "multipart/mixed") {
				boundaryMixed = boundaryStr
			}
		}
	}

	// For simple single-part messages
	if len(parts) == 1 {
		part := parts[0]

		// Only add Content-Type if not already in headers
		if !hasContentType {
			contentType := part["content_type"].(string)
			if charset, ok := part["charset"].(string); ok && charset != "" {
				buf.WriteString(fmt.Sprintf("Content-Type: %s; charset=%s\r\n", contentType, charset))
			} else {
				buf.WriteString(fmt.Sprintf("Content-Type: %s\r\n", contentType))
			}

			if encoding, ok := part["content_transfer_encoding"].(string); ok && encoding != "" {
				buf.WriteString(fmt.Sprintf("Content-Transfer-Encoding: %s\r\n", encoding))
			}
		}

		buf.WriteString("\r\n")

		// Get content from blob or text_content
		if blobID, ok := part["blob_id"].(int64); ok {
			content, err := db.GetBlob(database, blobID)
			if err == nil {
				buf.WriteString(content)
			}
		} else if textContent, ok := part["text_content"].(string); ok {
			buf.WriteString(textContent)
		}
	} else {
		// Multipart message handling
		// Separate text parts from attachments
		var textParts []map[string]interface{}
		var attachments []map[string]interface{}

		for _, part := range parts {
			contentType := part["content_type"].(string)
			disposition := ""
			if d, ok := part["content_disposition"].(string); ok {
				disposition = d
			}

			// Classify as attachment or text part
			isAttachment := false
			if disposition == "attachment" || disposition == "inline" {
				isAttachment = true
			} else if !strings.HasPrefix(contentType, "text/") {
				// Non-text types are likely attachments (images, etc.)
				isAttachment = true
			}

			if isAttachment {
				attachments = append(attachments, part)
			} else {
				textParts = append(textParts, part)
			}
		}

		// Determine if we have attachments
		hasAttachments := len(attachments) > 0

		// Check if we have both text/plain and text/html
		hasPlain := false
		hasHTML := false
		var htmlPart, plainPart map[string]interface{}

		for _, part := range textParts {
			contentType := part["content_type"].(string)
			if contentType == "text/plain" {
				hasPlain = true
				plainPart = part
			} else if contentType == "text/html" {
				hasHTML = true
				htmlPart = part
			}
		}

		// Build the message structure
		if hasAttachments {
			// Only add Content-Type if not already in headers
			if !hasContentType {
				buf.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=\"%s\"\r\n", boundaryMixed))
			}
			buf.WriteString("\r\n")

			// First part: the message body
			if hasPlain && hasHTML {
				// multipart/alternative for text versions
				buf.WriteString(fmt.Sprintf("--%s\r\n", boundaryMixed))
				buf.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n", boundaryAlt))
				buf.WriteString("\r\n")

				// Plain text version
				buf.WriteString(fmt.Sprintf("--%s\r\n", boundaryAlt))
				writePartHeaders(&buf, plainPart)
				writePartContent(&buf, database, plainPart)

				// HTML version
				buf.WriteString(fmt.Sprintf("--%s\r\n", boundaryAlt))
				writePartHeaders(&buf, htmlPart)
				writePartContent(&buf, database, htmlPart)

				buf.WriteString(fmt.Sprintf("--%s--\r\n", boundaryAlt))
			} else if hasHTML {
				// HTML only
				buf.WriteString(fmt.Sprintf("--%s\r\n", boundaryMixed))
				writePartHeaders(&buf, htmlPart)
				writePartContent(&buf, database, htmlPart)
			} else if hasPlain {
				// Plain text only
				buf.WriteString(fmt.Sprintf("--%s\r\n", boundaryMixed))
				writePartHeaders(&buf, plainPart)
				writePartContent(&buf, database, plainPart)
			}

			// Attachments
			for _, attachment := range attachments {
				buf.WriteString(fmt.Sprintf("--%s\r\n", boundaryMixed))
				writePartHeaders(&buf, attachment)
				writePartContent(&buf, database, attachment)
			}

			buf.WriteString(fmt.Sprintf("--%s--\r\n", boundaryMixed))
		} else if hasPlain && hasHTML {
			// multipart/alternative (no attachments)
			if !hasContentType {
				buf.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n", boundaryAlt))
			}
			buf.WriteString("\r\n")

			// Plain text version
			buf.WriteString(fmt.Sprintf("--%s\r\n", boundaryAlt))
			writePartHeaders(&buf, plainPart)
			writePartContent(&buf, database, plainPart)

			// HTML version
			buf.WriteString(fmt.Sprintf("--%s\r\n", boundaryAlt))
			writePartHeaders(&buf, htmlPart)
			writePartContent(&buf, database, htmlPart)

			buf.WriteString(fmt.Sprintf("--%s--\r\n", boundaryAlt))
		} else if len(textParts) == 1 {
			// Single text part, no multipart needed
			writePartHeaders(&buf, textParts[0])
			writePartContent(&buf, database, textParts[0])
		} else {
			// Fallback: multipart/mixed for all parts
			if !hasContentType {
				buf.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=\"%s\"\r\n", boundaryMixed))
			}
			buf.WriteString("\r\n")

			for _, part := range parts {
				buf.WriteString(fmt.Sprintf("--%s\r\n", boundaryMixed))
				writePartHeaders(&buf, part)
				writePartContent(&buf, database, part)
			}

			buf.WriteString(fmt.Sprintf("--%s--\r\n", boundaryMixed))
		}
	}

	return buf.String(), nil
}

// writePartHeaders writes the MIME headers for a message part
func writePartHeaders(buf *bytes.Buffer, part map[string]interface{}) {
	contentType := part["content_type"].(string)

	// Write Content-Type with charset if available
	if charset, ok := part["charset"].(string); ok && charset != "" {
		buf.WriteString(fmt.Sprintf("Content-Type: %s; charset=%s", contentType, charset))
	} else {
		buf.WriteString(fmt.Sprintf("Content-Type: %s", contentType))
	}

	// Add filename to Content-Type if present (for attachments)
	if filename, ok := part["filename"].(string); ok && filename != "" {
		buf.WriteString(fmt.Sprintf("; name=\"%s\"", filename))
	}
	buf.WriteString("\r\n")

	// Write Content-Transfer-Encoding
	if encoding, ok := part["content_transfer_encoding"].(string); ok && encoding != "" {
		buf.WriteString(fmt.Sprintf("Content-Transfer-Encoding: %s\r\n", encoding))
	}

	// Write Content-Disposition if present
	if disposition, ok := part["content_disposition"].(string); ok && disposition != "" {
		buf.WriteString(fmt.Sprintf("Content-Disposition: %s", disposition))
		if filename, ok := part["filename"].(string); ok && filename != "" {
			buf.WriteString(fmt.Sprintf("; filename=\"%s\"", filename))
		}
		buf.WriteString("\r\n")
	}

	buf.WriteString("\r\n")
}

// writePartContent writes the content of a message part
func writePartContent(buf *bytes.Buffer, database *sql.DB, part map[string]interface{}) {
	// Get content from blob or text_content
	if blobID, ok := part["blob_id"].(int64); ok {
		content, err := db.GetBlob(database, blobID)
		if err == nil {
			buf.WriteString(content)
		}
	} else if textContent, ok := part["text_content"].(string); ok {
		buf.WriteString(textContent)
	}
	buf.WriteString("\r\n")
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

// extractAllHeaders extracts all headers from raw message preserving order and multi-line values
func extractAllHeaders(rawMessage string) []MessageHeader {
	var headers []MessageHeader
	lines := strings.Split(rawMessage, "\n")
	sequence := 0
	var currentHeaderName string
	var currentHeaderValue strings.Builder

	for _, line := range lines {
		line = strings.TrimRight(line, "\r")

		// Empty line marks end of headers
		if line == "" {
			// Save last header if exists
			if currentHeaderName != "" {
				headers = append(headers, MessageHeader{
					Name:     currentHeaderName,
					Value:    currentHeaderValue.String(),
					Sequence: sequence,
				})
				sequence++
			}
			break
		}

		// Check if this is a continuation line (starts with space or tab)
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
			if currentHeaderName != "" {
				// Append to current header value preserving the line break
				currentHeaderValue.WriteString("\r\n")
				currentHeaderValue.WriteString(line)
			}
			continue
		}

		// New header line - save previous header if exists
		if currentHeaderName != "" {
			headers = append(headers, MessageHeader{
				Name:     currentHeaderName,
				Value:    currentHeaderValue.String(),
				Sequence: sequence,
			})
			sequence++
			currentHeaderValue.Reset()
		}

		// Parse new header
		colonIdx := strings.Index(line, ":")
		if colonIdx != -1 {
			currentHeaderName = strings.TrimSpace(line[:colonIdx])
			currentHeaderValue.WriteString(strings.TrimSpace(line[colonIdx+1:]))
		} else {
			// Malformed header, skip it
			currentHeaderName = ""
		}
	}

	return headers
}
