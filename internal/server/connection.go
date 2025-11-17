package server

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"raven/internal/models"
	"raven/internal/server/auth"
)

// HandleClient handles IMAP client connections (exported for auth package)
func HandleClient(s *IMAPServer, conn net.Conn, state *models.ClientState) {
	handleClient(s, conn, state)
}

func handleClient(s *IMAPServer, conn net.Conn, state *models.ClientState) {
	// Use buffered reader to properly handle command lines and literal data
	reader := bufio.NewReader(conn)

	for {
		conn.SetReadDeadline(time.Now().Add(30 * time.Minute))

		// Read one line at a time (until CRLF or LF)
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				return
			}
			if line == "" {
				return
			}
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fmt.Printf("Client: %s\n", line)
		parts := strings.Fields(line)
		if len(parts) < 2 {
			s.sendResponse(conn, "* BAD Invalid command format")
			continue
		}

		tag := parts[0]
		cmd := strings.ToUpper(parts[1])

		switch cmd {
		case "CAPABILITY":
			auth.HandleCapability(s, conn, tag, state)
		case "LOGIN":
			auth.HandleLogin(s, conn, tag, parts, state)
		case "AUTHENTICATE":
			auth.HandleAuthenticate(s, conn, tag, parts, state)
		case "LIST":
			s.handleList(conn, tag, parts, state)
		case "LSUB":
			s.handleLsub(conn, tag, parts, state)
		case "CREATE":
			s.handleCreate(conn, tag, parts, state)
		case "DELETE":
			s.handleDelete(conn, tag, parts, state)
		case "RENAME":
			s.handleRename(conn, tag, parts, state)
		case "SELECT", "EXAMINE":
			s.handleSelect(conn, tag, parts, state)
		case "FETCH":
			s.handleFetch(conn, tag, parts, state)
		case "SEARCH":
			s.handleSearch(conn, tag, parts, state)
		case "STORE":
			s.handleStore(conn, tag, parts, state)
		case "COPY":
			s.handleCopy(conn, tag, parts, state)
		case "STATUS":
			s.handleStatus(conn, tag, parts, state)
		case "UID":
			s.handleUID(conn, tag, parts, state)
		case "IDLE":
			s.handleIdle(conn, tag, state)
		case "NAMESPACE":
			s.handleNamespace(conn, tag, state)
		case "UNSELECT":
			s.handleUnselect(conn, tag, state)
		case "APPEND":
			s.handleAppendWithReader(reader, conn, tag, parts, line, state)
		case "NOOP":
			s.handleNoop(conn, tag, state)
		case "CHECK":
			s.handleCheck(conn, tag, state)
		case "CLOSE":
			s.handleClose(conn, tag, state)
		case "EXPUNGE":
			s.handleExpunge(conn, tag, state)
		case "SUBSCRIBE":
			s.handleSubscribe(conn, tag, parts, state)
		case "UNSUBSCRIBE":
			s.handleUnsubscribe(conn, tag, parts, state)
		case "LOGOUT":
			auth.HandleLogout(s, conn, tag)
			return
		case "STARTTLS":
			// Create a client handler wrapper for the auth package
			clientHandler := func(conn net.Conn, state *models.ClientState) {
				handleClient(s, conn, state)
			}
			auth.HandleStartTLS(s, clientHandler, conn, tag, parts)
			return
		default:
			s.sendResponse(conn, fmt.Sprintf("%s BAD Unknown command: %s", tag, cmd))
		}
	}
}

// SendResponse sends a response to the client (exported for auth package)
func (s *IMAPServer) SendResponse(conn net.Conn, response string) {
	s.sendResponse(conn, response)
}

func (s *IMAPServer) sendResponse(conn net.Conn, response string) {
	// Sanitize response for logging to avoid printing large message bodies
	logResponse := s.sanitizeResponseForLogging(response)
	fmt.Printf("Server: %s\n", logResponse)
	conn.Write([]byte(response + "\r\n"))
}

// sanitizeResponseForLogging removes or masks large message bodies from responses
func (s *IMAPServer) sanitizeResponseForLogging(response string) string {
	// Check for FETCH responses that contain message bodies
	// This includes BODY[], BODY[HEADER], BODY[TEXT], RFC822, etc.
	if strings.Contains(response, "FETCH (") &&
	   (strings.Contains(response, "BODY") ||
	    strings.Contains(response, "RFC822")) {

		// Find the literal string marker {number}
		idx := strings.Index(response, "{")
		if idx != -1 {
			// Find the closing brace
			closeIdx := strings.Index(response[idx:], "}")
			if closeIdx != -1 {
				closeIdx += idx
				// Extract the literal size
				literalSizeStr := response[idx+1 : closeIdx]

				// Check if this is a large literal (likely contains encoded data)
				// If size > 100 bytes, it's probably message content we want to mask
				var literalSize int
				if _, err := fmt.Sscanf(literalSizeStr, "%d", &literalSize); err == nil && literalSize > 100 {
					// Return everything up to and including the literal size marker
					// followed by a mask indicator
					return response[:closeIdx+1] + "\r\n[MESSAGE CONTENT OMITTED - " + literalSizeStr + " bytes]"
				}
			}
		}
	}

	// If response is very long (>2000 chars), truncate it to prevent console spam
	if len(response) > 2000 {
		return response[:2000] + "... [TRUNCATED - " + fmt.Sprintf("%d", len(response)) + " total bytes]"
	}

	return response
}