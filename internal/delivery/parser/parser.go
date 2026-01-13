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

	"raven/internal/blobstorage"
	"raven/internal/db"
)

// Message represents a parsed email message (simple format for LMTP)
type Message struct {
	From       string
	To         []string
	Subject    string
	Date       time.Time
	MessageID  string
	Headers    map[string]string
	Body       string
	RawMessage string
	Size       int64
}

// ParsedMessage represents a parsed email message with full MIME structure
type ParsedMessage struct {
	MessageID  int64
	Subject    string
	From       []mail.Address
	To         []mail.Address
	Cc         []mail.Address
	Bcc        []mail.Address
	Date       time.Time
	InReplyTo  string
	References string
	Headers    []MessageHeader
	Parts      []MessagePart
	RawMessage string
	SizeBytes  int64
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
	ContentID               string
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
	_, _ = io.Copy(&buf, r)
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

	fmt.Printf("DEBUG ParseMIMEMessage: Content-Type='%s'\n", contentType)

	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		fmt.Printf("DEBUG ParseMIMEMessage: Failed to parse Content-Type: %v\n", err)
		mediaType = "text/plain"
		params = map[string]string{"charset": "us-ascii"}
	}

	fmt.Printf("DEBUG ParseMIMEMessage: mediaType='%s', boundary='%s'\n", mediaType, params["boundary"])

	// Handle multipart messages
	if strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		if boundary != "" {
			fmt.Printf("DEBUG ParseMIMEMessage: Parsing multipart with boundary='%s'\n", boundary)
			parsed.Parts, err = parseMultipart(msg.Body, boundary, 0, sql.NullInt64{})
			if err != nil {
				fmt.Printf("DEBUG ParseMIMEMessage: multipart parsing failed: %v\n", err)
				return nil, fmt.Errorf("failed to parse multipart: %v", err)
			}
			fmt.Printf("DEBUG ParseMIMEMessage: Successfully parsed %d parts\n", len(parsed.Parts))
		} else {
			fmt.Printf("DEBUG ParseMIMEMessage: multipart detected but no boundary!\n")
		}
	} else {
		// Single part message
		fmt.Printf("DEBUG ParseMIMEMessage: Single-part message (not multipart)\n")
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

	fmt.Printf("DEBUG parseMultipart: boundary='%s', depth=%d\n", boundary, depth)

	mr := multipart.NewReader(body, boundary)
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			fmt.Printf("DEBUG parseMultipart: EOF reached, found %d parts\n", len(parts))
			break
		}
		if err != nil {
			fmt.Printf("DEBUG parseMultipart: NextPart error: %v\n", err)
			return parts, err
		}

		// Read part content
		content, err := io.ReadAll(p)
		if err != nil {
			fmt.Printf("DEBUG parseMultipart: ReadAll error: %v\n", err)
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
		contentID := p.Header.Get("Content-ID")

		fmt.Printf("DEBUG parseMultipart: Part %d: type='%s', disposition='%s', filename='%s', size=%d\n",
			partNumber, mediaType, disposition, filename, len(content))

		part := MessagePart{
			PartNumber:              partNumber,
			ParentPartID:            parentPartID,
			ContentType:             mediaType,
			ContentDisposition:      disposition,
			ContentTransferEncoding: encoding,
			Charset:                 charset,
			Filename:                filename,
			ContentID:               contentID,
			TextContent:             string(content),
			SizeBytes:               int64(len(content)),
		}

		// If this is a multipart, recursively parse it
		if strings.HasPrefix(mediaType, "multipart/") && params["boundary"] != "" {
			fmt.Printf("DEBUG parseMultipart: Part %d is nested multipart with boundary='%s'\n", partNumber, params["boundary"])
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

// StoreMessageWithSharedDB stores a message with separate shared and user databases
func StoreMessageWithSharedDB(sharedDB *sql.DB, userDB *sql.DB, parsed *ParsedMessage) (int64, error) {
	// Create message record in user database
	messageID, err := db.CreateMessage(userDB, parsed.Subject, parsed.InReplyTo, parsed.References, parsed.Date, parsed.SizeBytes)
	if err != nil {
		return 0, fmt.Errorf("failed to create message: %v", err)
	}

	parsed.MessageID = messageID

	// Store all headers in user database
	for _, header := range parsed.Headers {
		if err := db.AddMessageHeader(userDB, messageID, header.Name, header.Value, header.Sequence); err != nil {
			return 0, fmt.Errorf("failed to store header %s: %v", header.Name, err)
		}
	}

	// Store addresses in user database
	if err := storeAddresses(userDB, messageID, "from", parsed.From); err != nil {
		return 0, fmt.Errorf("failed to store from addresses: %v", err)
	}
	if err := storeAddresses(userDB, messageID, "to", parsed.To); err != nil {
		return 0, fmt.Errorf("failed to store to addresses: %v", err)
	}
	if err := storeAddresses(userDB, messageID, "cc", parsed.Cc); err != nil {
		return 0, fmt.Errorf("failed to store cc addresses: %v", err)
	}
	if err := storeAddresses(userDB, messageID, "bcc", parsed.Bcc); err != nil {
		return 0, fmt.Errorf("failed to store bcc addresses: %v", err)
	}

	// Store message parts
	for _, part := range parsed.Parts {
		var blobID sql.NullInt64

		// Store large content or attachments in blobs (in shared database for deduplication)
		if len(part.TextContent) > 1024 || part.Filename != "" {
			id, err := db.StoreBlob(sharedDB, part.TextContent)
			if err == nil {
				blobID = sql.NullInt64{Valid: true, Int64: id}
				// Clear text content since it's in blob
				part.TextContent = ""
			}
		}

		_, err := db.AddMessagePart(
			userDB,
			messageID,
			part.PartNumber,
			part.ParentPartID,
			part.ContentType,
			part.ContentDisposition,
			part.ContentTransferEncoding,
			part.Charset,
			part.Filename,
			part.ContentID,
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

// StoreMessagePerUserWithSharedDB stores a message with separate shared and user databases
func StoreMessagePerUserWithSharedDB(sharedDB *sql.DB, userDB *sql.DB, parsed *ParsedMessage) (int64, error) {
	return StoreMessagePerUserWithSharedDBAndS3(sharedDB, userDB, parsed, nil)
}

// StoreMessagePerUserWithSharedDBAndS3 stores a message in a per-user database with optional S3 blob storage and shared blob deduplication
func StoreMessagePerUserWithSharedDBAndS3(sharedDB *sql.DB, userDB *sql.DB, parsed *ParsedMessage, s3Storage *blobstorage.S3BlobStorage) (int64, error) {
	// Create message record in user database
	messageID, err := db.CreateMessage(userDB, parsed.Subject, parsed.InReplyTo, parsed.References, parsed.Date, parsed.SizeBytes)
	if err != nil {
		return 0, fmt.Errorf("failed to create message: %v", err)
	}

	parsed.MessageID = messageID

	// Store all headers in user database
	for _, header := range parsed.Headers {
		if err := db.AddMessageHeader(userDB, messageID, header.Name, header.Value, header.Sequence); err != nil {
			return 0, fmt.Errorf("failed to store header %s: %v", header.Name, err)
		}
	}

	// Store addresses in user database
	if err := storeAddresses(userDB, messageID, "from", parsed.From); err != nil {
		return 0, fmt.Errorf("failed to store from addresses: %v", err)
	}
	if err := storeAddresses(userDB, messageID, "to", parsed.To); err != nil {
		return 0, fmt.Errorf("failed to store to addresses: %v", err)
	}
	if err := storeAddresses(userDB, messageID, "cc", parsed.Cc); err != nil {
		return 0, fmt.Errorf("failed to store cc addresses: %v", err)
	}
	if err := storeAddresses(userDB, messageID, "bcc", parsed.Bcc); err != nil {
		return 0, fmt.Errorf("failed to store bcc addresses: %v", err)
	}

	// Store message parts
	for _, part := range parsed.Parts {
		var blobID sql.NullInt64

		// Store large content or attachments in blobs (in shared database for cross-user deduplication)
		if len(part.TextContent) > 1024 || part.Filename != "" {
			var id int64

			// Use S3 storage if available and enabled
			if s3Storage != nil && s3Storage.IsEnabled() {
				s3BlobID, err := s3Storage.Store(part.TextContent)
				if err == nil {
					id, err = db.StoreBlobS3(sharedDB, part.TextContent, s3BlobID)
					if err == nil {
						blobID = sql.NullInt64{Valid: true, Int64: id}
						// Clear text content since it's in S3
						part.TextContent = ""
						fmt.Printf("Stored attachment in S3 with shared deduplication: %s (blob_id: %d, s3_id: %s)\n", part.Filename, id, s3BlobID)
					}
				} else {
					fmt.Printf("Failed to store in S3, falling back to local: %v\n", err)
					// Fall back to local storage
					id, err = db.StoreBlob(sharedDB, part.TextContent)
					if err == nil {
						blobID = sql.NullInt64{Valid: true, Int64: id}
						part.TextContent = ""
					}
				}
			} else {
				// Use local SQLite storage in shared database
				id, err = db.StoreBlob(sharedDB, part.TextContent)
				if err == nil {
					blobID = sql.NullInt64{Valid: true, Int64: id}
					part.TextContent = ""
				}
			}
		}

		_, err := db.AddMessagePart(
			userDB,
			messageID,
			part.PartNumber,
			part.ParentPartID,
			part.ContentType,
			part.ContentDisposition,
			part.ContentTransferEncoding,
			part.Charset,
			part.Filename,
			part.ContentID,
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

// ReconstructMessageWithSharedDB reconstructs the raw message with separate shared and user databases
func ReconstructMessageWithSharedDB(sharedDB *sql.DB, userDB *sql.DB, messageID int64) (string, error) {
	return ReconstructMessageWithSharedDBAndS3(sharedDB, userDB, messageID, nil)
}

// ReconstructMessageWithSharedDBAndS3 reconstructs the raw message from database parts with S3 support and shared blob storage
func ReconstructMessageWithSharedDBAndS3(sharedDB *sql.DB, userDB *sql.DB, messageID int64, s3Storage *blobstorage.S3BlobStorage) (string, error) {
	// Get message parts from user database
	parts, err := db.GetMessageParts(userDB, messageID)
	if err != nil {
		return "", fmt.Errorf("failed to get message parts: %v", err)
	}

	if len(parts) == 0 {
		return "", fmt.Errorf("no message parts found")
	}

	// Get ALL stored headers from user database
	headers, err := db.GetMessageHeaders(userDB, messageID)
	if err != nil {
		// If we can't get headers, try to reconstruct from addresses and metadata
		fmt.Printf("WARNING: Failed to get message headers for message %d: %v\n", messageID, err)
		headers = []map[string]string{}
	}

	fmt.Printf("DEBUG ReconstructMessage: Starting reconstruction with %d headers, %d parts\n", len(headers), len(parts))

	// For multipart messages, we need to determine this early to filter headers correctly
	isMultipart := len(parts) > 1

	// Check if we have stored Content-Type and MIME-Version headers
	// For multipart messages, we need to filter these out from stored headers
	// because we'll generate new ones with boundaries that match the reconstructed body
	hasStoredContentType := false
	filteredHeaders := []map[string]string{}
	for _, header := range headers {
		headerName := strings.ToLower(strings.TrimSpace(header["name"]))

		if headerName == "content-type" {
			hasStoredContentType = true
			// Skip Content-Type header for multipart messages - we'll generate it
			if isMultipart {
				fmt.Printf("DEBUG ReconstructMessage: Filtering out stored Content-Type header\n")
				continue
			}
		} else if headerName == "mime-version" {
			// Skip MIME-Version for multipart messages - we'll add it when needed
			if isMultipart {
				fmt.Printf("DEBUG ReconstructMessage: Filtering out stored MIME-Version header\n")
				continue
			}
		} else if headerName == "content-transfer-encoding" {
			// RFC 2045: multipart entities MUST NOT have a Content-Transfer-Encoding
			if isMultipart {
				fmt.Printf("DEBUG ReconstructMessage: Filtering out stored Content-Transfer-Encoding header for multipart\n")
				continue
			}
		}
		filteredHeaders = append(filteredHeaders, header)
	}

	// Use filtered headers for multipart messages
	if isMultipart {
		headers = filteredHeaders
		fmt.Printf("DEBUG ReconstructMessage: After filtering, %d headers remain\n", len(headers))
	}

	// Build message headers
	var buf bytes.Buffer

	// If we have stored headers, use them
	if len(headers) > 0 {
		// Write all headers in original order
		for _, header := range headers {
			buf.WriteString(fmt.Sprintf("%s: %s\r\n", header["name"], header["value"]))
		}
		fmt.Printf("DEBUG ReconstructMessage: Wrote %d stored headers\n", len(headers))
	} else {
		// Fallback: Build minimal headers from addresses and metadata for old messages
		fmt.Printf("WARNING: No stored headers found for message %d, using fallback\n", messageID)

		// Get addresses from user database
		fromAddrs, _ := db.GetMessageAddresses(userDB, messageID, "from")
		toAddrs, _ := db.GetMessageAddresses(userDB, messageID, "to")
		ccAddrs, _ := db.GetMessageAddresses(userDB, messageID, "cc")

		// Get message metadata from user database
		var subject string
		var date time.Time
		err = userDB.QueryRow("SELECT subject, date FROM messages WHERE id = ?", messageID).Scan(&subject, &date)
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

	// Use consistent boundaries for reconstructed messages
	boundaryAlt := "----=_Part_Alternative_" + fmt.Sprintf("%d", time.Now().UnixNano())
	boundaryMixed := "----=_Part_Mixed_" + fmt.Sprintf("%d", time.Now().UnixNano()+1)

	// For simple single-part messages
	if len(parts) == 1 {
		part := parts[0]

		// Only add Content-Type if not already in filtered headers
		if !hasStoredContentType {
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

		// Get content from blob (in shared database) or text_content (with S3 support)
		if blobID, ok := part["blob_id"].(int64); ok {
			content, err := db.GetBlob(sharedDB, blobID)
			if err == nil && content != "" {
				buf.WriteString(content)
			} else if s3Storage != nil && s3Storage.IsEnabled() {
				// Try to get from S3 storage
				if s3BlobID, storageType, err := db.GetBlobS3BlobID(sharedDB, blobID); err == nil && storageType == "s3" && s3BlobID != "" {
					if content, err := s3Storage.Retrieve(s3BlobID); err == nil {
						buf.WriteString(content)
					}
				}
			}
		} else if textContent, ok := part["text_content"].(string); ok {
			buf.WriteString(textContent)
		}
	} else {
		// Multipart message handling
		// Separate text parts from attachments
		var textParts []map[string]interface{}
		var attachments []map[string]interface{}

		fmt.Printf("DEBUG ReconstructMessage: Processing %d parts\n", len(parts))

		for _, part := range parts {
			contentType := part["content_type"].(string)
			disposition := ""
			if d, ok := part["content_disposition"].(string); ok {
				disposition = d
			}
			filename := ""
			if f, ok := part["filename"].(string); ok {
				filename = f
			}

			// Classify as attachment or text part
			isAttachment := false
			dispLower := strings.ToLower(strings.TrimSpace(disposition))
			// Never treat multipart containers as attachments
			// CRITICAL FIX: Skip multipart containers - they're just structure, not content
			if strings.HasPrefix(strings.ToLower(contentType), "multipart/") {
				fmt.Printf("DEBUG ReconstructMessage: Skipping multipart container: type='%s'\n", contentType)
				continue
			} else if strings.HasPrefix(dispLower, "attachment") {
				isAttachment = true
			} else if !strings.HasPrefix(strings.ToLower(contentType), "text/") && !strings.HasPrefix(dispLower, "inline") {
				// Non-text types are attachments unless explicitly inline
				isAttachment = true
			}

			fmt.Printf("DEBUG ReconstructMessage: Part type='%s', disposition='%s', filename='%s', isAttachment=%v\n",
				contentType, disposition, filename, isAttachment)

			if isAttachment {
				attachments = append(attachments, part)
			} else {
				textParts = append(textParts, part)
			}
		}

		fmt.Printf("DEBUG ReconstructMessage: Found %d text parts, %d attachments\n", len(textParts), len(attachments))

		// Determine if we have attachments
		hasAttachments := len(attachments) > 0

		// Check if we have both text/plain and text/html
		hasPlain := false
		hasHTML := false
		var htmlPart, plainPart map[string]interface{}

		for _, part := range textParts {
			contentType := part["content_type"].(string)
			switch contentType {
			case "text/plain":
				hasPlain = true
				plainPart = part
			case "text/html":
				hasHTML = true
				htmlPart = part
			}
		}

		// Build the message structure
		if hasAttachments {
			fmt.Printf("DEBUG ReconstructMessage: Building multipart/mixed message\n")
			// Always generate Content-Type header for multipart messages
			buf.WriteString("MIME-Version: 1.0\r\n")
			buf.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=\"%s\"\r\n", boundaryMixed))
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
				writePartContentWithS3(&buf, sharedDB, plainPart, s3Storage)

				// HTML version
				buf.WriteString(fmt.Sprintf("--%s\r\n", boundaryAlt))
				writePartHeaders(&buf, htmlPart)
				writePartContentWithS3(&buf, sharedDB, htmlPart, s3Storage)

				buf.WriteString(fmt.Sprintf("--%s--\r\n", boundaryAlt))
			} else if hasHTML {
				// HTML only
				buf.WriteString(fmt.Sprintf("--%s\r\n", boundaryMixed))
				writePartHeaders(&buf, htmlPart)
				writePartContentWithS3(&buf, sharedDB, htmlPart, s3Storage)
			} else if hasPlain {
				// Plain text only
				buf.WriteString(fmt.Sprintf("--%s\r\n", boundaryMixed))
				writePartHeaders(&buf, plainPart)
				writePartContentWithS3(&buf, sharedDB, plainPart, s3Storage)
			}

			// Attachments
			for _, attachment := range attachments {
				buf.WriteString(fmt.Sprintf("--%s\r\n", boundaryMixed))
				writePartHeaders(&buf, attachment)
				writePartContentWithS3(&buf, sharedDB, attachment, s3Storage)
			}

			buf.WriteString(fmt.Sprintf("--%s--\r\n", boundaryMixed))
		} else if hasPlain && hasHTML {
			// multipart/alternative (no attachments)
			buf.WriteString("MIME-Version: 1.0\r\n")
			buf.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n", boundaryAlt))
			buf.WriteString("\r\n")

			// Plain text version
			buf.WriteString(fmt.Sprintf("--%s\r\n", boundaryAlt))
			writePartHeaders(&buf, plainPart)
			writePartContentWithS3(&buf, sharedDB, plainPart, s3Storage)

			// HTML version
			buf.WriteString(fmt.Sprintf("--%s\r\n", boundaryAlt))
			writePartHeaders(&buf, htmlPart)
			writePartContentWithS3(&buf, sharedDB, htmlPart, s3Storage)

			buf.WriteString(fmt.Sprintf("--%s--\r\n", boundaryAlt))
		} else if len(textParts) == 1 {
			// Single text part, no multipart needed
			writePartHeaders(&buf, textParts[0])
			writePartContentWithS3(&buf, sharedDB, textParts[0], s3Storage)
		} else {
			// Fallback: multipart/mixed for all parts
			buf.WriteString("MIME-Version: 1.0\r\n")
			buf.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=\"%s\"\r\n", boundaryMixed))
			buf.WriteString("\r\n")

			for _, part := range parts {
				buf.WriteString(fmt.Sprintf("--%s\r\n", boundaryMixed))
				writePartHeaders(&buf, part)
				writePartContentWithS3(&buf, sharedDB, part, s3Storage)
			}

			buf.WriteString(fmt.Sprintf("--%s--\r\n", boundaryMixed))
		}
	}

	result := buf.String()
	// Debug: Show the first part of the reconstructed message (headers)
	headerEnd := strings.Index(result, "\r\n\r\n")
	if headerEnd == -1 {
		headerEnd = strings.Index(result, "\n\n")
	}
	if headerEnd > 0 && headerEnd < 1000 {
		fmt.Printf("DEBUG ReconstructMessage: Reconstructed headers:\n%s\n", result[:headerEnd])
	}
	fmt.Printf("DEBUG ReconstructMessage: Total message size: %d bytes\n", len(result))

	return result, nil
}

// writePartHeaders writes the MIME headers for a message part
func writePartHeaders(buf *bytes.Buffer, part map[string]interface{}) {
	contentType := part["content_type"].(string)

	// Content-Type
	if charset, ok := part["charset"].(string); ok && charset != "" {
		fmt.Fprintf(buf, "Content-Type: %s; charset=%s\r\n", contentType, charset)
	} else {
		fmt.Fprintf(buf, "Content-Type: %s\r\n", contentType)
	}

	// Content-Transfer-Encoding (default 7bit for text/*)
	if encoding, ok := part["content_transfer_encoding"].(string); ok && strings.TrimSpace(encoding) != "" {
		fmt.Fprintf(buf, "Content-Transfer-Encoding: %s\r\n", encoding)
	} else if strings.HasPrefix(strings.ToLower(contentType), "text/") {
		buf.WriteString("Content-Transfer-Encoding: 7bit\r\n")
	}

	// Content-ID
	if contentID, ok := part["content_id"].(string); ok && strings.TrimSpace(contentID) != "" {
		fmt.Fprintf(buf, "Content-ID: %s\r\n", contentID)
	}

	// Content-Disposition
	if disp, ok := part["content_disposition"].(string); ok && strings.TrimSpace(disp) != "" {
		lowerDisp := strings.ToLower(disp)
		buf.WriteString("Content-Disposition: ")
		buf.WriteString(disp)
		if filename, ok := part["filename"].(string); ok && strings.TrimSpace(filename) != "" && !strings.Contains(lowerDisp, "filename=") {
			fmt.Fprintf(buf, "; filename=\"%s\"", filename)
		}
		buf.WriteString("\r\n")
	} else {
		filename, hasFilename := part["filename"].(string)
		isText := strings.HasPrefix(strings.ToLower(contentType), "text/")
		// Only write Content-Disposition for non-text parts with a filename
		// Text parts and non-text parts without filename omit disposition
		if !isText && hasFilename && strings.TrimSpace(filename) != "" {
			fmt.Fprintf(buf, "Content-Disposition: attachment; filename=\"%s\"\r\n", filename)
		}
	}

	buf.WriteString("\r\n")
}

// writePartContentWithS3 writes the content of a message part with S3 support
func writePartContentWithS3(buf *bytes.Buffer, sharedDB *sql.DB, part map[string]interface{}, s3Storage *blobstorage.S3BlobStorage) {
	// Get content from blob (in shared database) or text_content
	var content string
	if blobID, ok := part["blob_id"].(int64); ok {
		// First try to get from local storage (in shared database)
		if c, err := db.GetBlob(sharedDB, blobID); err == nil && c != "" {
			content = c
		} else if s3Storage != nil && s3Storage.IsEnabled() {
			// Try to get from S3 storage
			if s3BlobID, storageType, err := db.GetBlobS3BlobID(sharedDB, blobID); err == nil && storageType == "s3" && s3BlobID != "" {
				if c, err := s3Storage.Retrieve(s3BlobID); err == nil {
					content = c
				} else {
					fmt.Printf("Failed to retrieve blob from S3: %v\n", err)
				}
			}
		}
	} else if textContent, ok := part["text_content"].(string); ok {
		content = textContent
	}

	// If base64 encoding, ensure proper 76 char wrapping per RFC 2045
	if enc, ok := part["content_transfer_encoding"].(string); ok && strings.EqualFold(strings.TrimSpace(enc), "base64") {
		// Detect if already wrapped (any line length <= 78 and multiple lines)
		lines := strings.Split(content, "\r\n")
		alreadyWrapped := true
		if len(lines) <= 1 {
			alreadyWrapped = false
		} else {
			for _, l := range lines {
				if len(l) > 0 && len(l) > 78 { // some lines too long
					alreadyWrapped = false
					break
				}
			}
		}
		if !alreadyWrapped {
			// Remove any existing whitespace/newlines and re-wrap
			raw := strings.ReplaceAll(content, "\r", "")
			raw = strings.ReplaceAll(raw, "\n", "")
			var wrapped strings.Builder
			for i := 0; i < len(raw); i += 76 {
				end := i + 76
				if end > len(raw) {
					end = len(raw)
				}
				wrapped.WriteString(raw[i:end])
				wrapped.WriteString("\r\n")
			}
			content = wrapped.String()
		}
	}

	buf.WriteString(content)
	if !strings.HasSuffix(content, "\r\n") {
		buf.WriteString("\r\n")
	}
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
