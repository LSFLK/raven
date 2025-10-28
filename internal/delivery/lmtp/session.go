package lmtp

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"go-imap/internal/delivery/config"
	"go-imap/internal/delivery/parser"
	"go-imap/internal/delivery/storage"
)

// Session represents an LMTP session
type Session struct {
	conn       net.Conn
	reader     *bufio.Reader
	writer     *bufio.Writer
	storage    *storage.Storage
	config     *config.Config
	mailFrom   string
	recipients []string
	helo       string
}

// NewSession creates a new LMTP session
func NewSession(conn net.Conn, stor *storage.Storage, cfg *config.Config) *Session {
	return &Session{
		conn:       conn,
		reader:     bufio.NewReader(conn),
		writer:     bufio.NewWriter(conn),
		storage:    stor,
		config:     cfg,
		recipients: make([]string, 0),
	}
}

// Handle handles the LMTP session
func (s *Session) Handle() error {
	// Set connection timeout
	if s.config.LMTP.Timeout > 0 {
		timeout := time.Duration(s.config.LMTP.Timeout) * time.Second
		s.conn.SetDeadline(time.Now().Add(timeout))
	}

	// Send greeting
	log.Printf("Sending greeting to %s", s.conn.RemoteAddr())
	if err := s.sendResponse(220, "%s LMTP Service ready", s.config.LMTP.Hostname); err != nil {
		log.Printf("Failed to send greeting to %s: %v", s.conn.RemoteAddr(), err)
		return err
	}
	log.Printf("Greeting sent successfully to %s, waiting for client command...", s.conn.RemoteAddr())

	// Process commands
	for {
		log.Printf("Waiting to read from %s...", s.conn.RemoteAddr())
		line, err := s.reader.ReadString('\n')
		if err != nil {
			log.Printf("Read failed from %s: %v (connection likely closed by client)", s.conn.RemoteAddr(), err)
			return fmt.Errorf("read error: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		log.Printf("C: %s", line)

		// Parse command
		parts := strings.SplitN(line, " ", 2)
		cmd := strings.ToUpper(parts[0])
		args := ""
		if len(parts) > 1 {
			args = parts[1]
		}

		// Handle command
		if err := s.handleCommand(cmd, args); err != nil {
			log.Printf("Command error: %v", err)
			if strings.Contains(err.Error(), "QUIT") {
				return nil
			}
		}

		// Reset timeout after each command
		if s.config.LMTP.Timeout > 0 {
			timeout := time.Duration(s.config.LMTP.Timeout) * time.Second
			s.conn.SetDeadline(time.Now().Add(timeout))
		}
	}
}

// handleCommand handles a single LMTP command
func (s *Session) handleCommand(cmd, args string) error {
	switch cmd {
	case "LHLO":
		return s.handleLHLO(args)
	case "MAIL":
		return s.handleMAIL(args)
	case "RCPT":
		return s.handleRCPT(args)
	case "DATA":
		return s.handleDATA()
	case "RSET":
		return s.handleRSET()
	case "NOOP":
		return s.handleNOOP()
	case "QUIT":
		return s.handleQUIT()
	case "VRFY":
		return s.handleVRFY(args)
	case "HELP":
		return s.handleHELP()
	default:
		return s.sendResponse(500, "Command not recognized")
	}
}

// handleLHLO handles the LHLO command
func (s *Session) handleLHLO(args string) error {
	if args == "" {
		return s.sendResponse(501, "LHLO requires domain address")
	}

	s.helo = args

	// Send multiline response with capabilities
	responses := []string{
		fmt.Sprintf("250-%s", s.config.LMTP.Hostname),
		"250-PIPELINING",
		"250-ENHANCEDSTATUSCODES",
		fmt.Sprintf("250-SIZE %d", s.config.LMTP.MaxSize),
		"250 8BITMIME",
	}

	for _, resp := range responses {
		if err := s.sendRawResponse(resp); err != nil {
			return err
		}
	}

	return nil
}

// handleMAIL handles the MAIL FROM command
func (s *Session) handleMAIL(args string) error {
	if s.helo == "" {
		return s.sendResponse(503, "Please send LHLO first")
	}

	if s.mailFrom != "" {
		return s.sendResponse(503, "Sender already specified")
	}

	// Parse MAIL FROM:<address>
	from, err := s.parseMailFrom(args)
	if err != nil {
		return s.sendResponse(501, "Invalid MAIL FROM syntax: %v", err)
	}

	s.mailFrom = from
	return s.sendResponse(250, "2.1.0 Sender OK")
}

// handleRCPT handles the RCPT TO command
func (s *Session) handleRCPT(args string) error {
	if s.mailFrom == "" {
		return s.sendResponse(503, "Please send MAIL FROM first")
	}

	if len(s.recipients) >= s.config.LMTP.MaxRecipients {
		return s.sendResponse(452, "Too many recipients")
	}

	// Parse RCPT TO:<address>
	to, err := s.parseRcptTo(args)
	if err != nil {
		return s.sendResponse(501, "Invalid RCPT TO syntax: %v", err)
	}

	// Validate recipient domain if configured
	if len(s.config.Delivery.AllowedDomains) > 0 {
		domain, err := parser.ExtractDomain(to)
		if err != nil {
			return s.sendResponse(550, "5.1.1 Invalid recipient address")
		}

		allowed := false
		for _, allowedDomain := range s.config.Delivery.AllowedDomains {
			if domain == allowedDomain {
				allowed = true
				break
			}
		}

		if !allowed {
			return s.sendResponse(550, "5.7.1 Relay not permitted")
		}
	}

	// Check if user exists (if configured)
	if s.config.Delivery.RejectUnknownUser {
		exists, err := s.storage.CheckRecipientExists(to)
		if err != nil {
			log.Printf("Error checking recipient: %v", err)
			return s.sendResponse(450, "4.3.0 Temporary failure")
		}
		if !exists {
			return s.sendResponse(550, "5.1.1 User does not exist")
		}
	}

	s.recipients = append(s.recipients, to)
	return s.sendResponse(250, "2.1.5 Recipient OK")
}

// handleDATA handles the DATA command
func (s *Session) handleDATA() error {
	if s.mailFrom == "" {
		return s.sendResponse(503, "Please send MAIL FROM first")
	}

	if len(s.recipients) == 0 {
		return s.sendResponse(503, "Please send RCPT TO first")
	}

	// Send intermediate response
	if err := s.sendResponse(354, "Start mail input; end with <CRLF>.<CRLF>"); err != nil {
		return err
	}

	// Read message data
	data, err := parser.ReadDataCommand(s.reader, s.config.LMTP.MaxSize)
	if err != nil {
		log.Printf("Error reading message data: %v", err)
		return s.sendResponse(554, "Error reading message: %v", err)
	}

	// Parse message
	msg, err := parser.ParseMessageFromBytes(data)
	if err != nil {
		log.Printf("Error parsing message: %v", err)
		return s.sendResponse(554, "Error parsing message: %v", err)
	}

	// Validate message
	if err := parser.ValidateMessage(msg, s.config.LMTP.MaxSize); err != nil {
		log.Printf("Message validation failed: %v", err)
		return s.sendResponse(554, "Message validation failed: %v", err)
	}

	// Check quota for each recipient (if enabled)
	if s.config.Delivery.QuotaEnabled {
		for _, recipient := range s.recipients {
			username, err := parser.ExtractLocalPart(recipient)
			if err != nil {
				continue
			}

			if err := s.storage.CheckQuota(username, msg.Size, s.config.Delivery.QuotaLimit); err != nil {
				log.Printf("Quota check failed for %s: %v", recipient, err)
				// Continue with other recipients
			}
		}
	}

	// Deliver to each recipient (LMTP requires per-recipient response)
	folder := s.config.Delivery.DefaultFolder
	results := s.storage.DeliverToMultipleRecipients(s.recipients, msg, folder)

	// Send per-recipient responses
	for _, recipient := range s.recipients {
		if err := results[recipient]; err != nil {
			log.Printf("Delivery failed for %s: %v", recipient, err)
			s.sendResponse(550, "5.3.0 Delivery failed for <%s>: %v", recipient, err)
		} else {
			log.Printf("Message delivered successfully to %s", recipient)
			s.sendResponse(250, "2.0.0 Message accepted for delivery to <%s>", recipient)
		}
	}

	// Reset session state
	s.mailFrom = ""
	s.recipients = make([]string, 0)

	return nil
}

// handleRSET handles the RSET command
func (s *Session) handleRSET() error {
	s.mailFrom = ""
	s.recipients = make([]string, 0)
	return s.sendResponse(250, "Reset state")
}

// handleNOOP handles the NOOP command
func (s *Session) handleNOOP() error {
	return s.sendResponse(250, "OK")
}

// handleQUIT handles the QUIT command
func (s *Session) handleQUIT() error {
	s.sendResponse(221, "Bye")
	return fmt.Errorf("QUIT")
}

// handleVRFY handles the VRFY command
func (s *Session) handleVRFY(args string) error {
	// VRFY is typically disabled for security reasons
	return s.sendResponse(252, "Cannot VRFY user, but will accept message")
}

// handleHELP handles the HELP command
func (s *Session) handleHELP() error {
	return s.sendResponse(214, "Commands: LHLO MAIL RCPT DATA RSET NOOP QUIT")
}

// parseMailFrom parses the MAIL FROM command arguments
func (s *Session) parseMailFrom(args string) (string, error) {
	// Expected format: FROM:<address> or FROM: <address>
	args = strings.TrimSpace(args)

	if !strings.HasPrefix(strings.ToUpper(args), "FROM:") {
		return "", fmt.Errorf("expected FROM:")
	}

	args = strings.TrimPrefix(args, "FROM:")
	args = strings.TrimPrefix(args, "from:")
	args = strings.TrimSpace(args)

	// Remove angle brackets if present
	args = strings.TrimPrefix(args, "<")
	args = strings.TrimSuffix(args, ">")

	// Handle SIZE parameter and other ESMTP parameters
	parts := strings.Fields(args)
	if len(parts) > 0 {
		return parts[0], nil
	}

	return args, nil
}

// parseRcptTo parses the RCPT TO command arguments
func (s *Session) parseRcptTo(args string) (string, error) {
	// Expected format: TO:<address> or TO: <address>
	args = strings.TrimSpace(args)

	if !strings.HasPrefix(strings.ToUpper(args), "TO:") {
		return "", fmt.Errorf("expected TO:")
	}

	args = strings.TrimPrefix(args, "TO:")
	args = strings.TrimPrefix(args, "to:")
	args = strings.TrimSpace(args)

	// Remove angle brackets if present
	args = strings.TrimPrefix(args, "<")
	args = strings.TrimSuffix(args, ">")

	return args, nil
}

// sendResponse sends a formatted response
func (s *Session) sendResponse(code int, format string, args ...interface{}) error {
	message := fmt.Sprintf(format, args...)
	response := fmt.Sprintf("%d %s\r\n", code, message)
	return s.sendRawResponse(response)
}

// sendRawResponse sends a raw response
func (s *Session) sendRawResponse(response string) error {
	if !strings.HasSuffix(response, "\r\n") {
		response += "\r\n"
	}

	log.Printf("S: %s", strings.TrimSpace(response))

	_, err := s.writer.WriteString(response)
	if err != nil {
		return err
	}

	return s.writer.Flush()
}
