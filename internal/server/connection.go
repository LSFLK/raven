package server

import (
	"fmt"
	"net"
	"strings"
	"time"

	"go-imap/internal/models"
)

func handleClient(s *IMAPServer, conn net.Conn, state *models.ClientState) {
	buf := make([]byte, 4096)
	for {
		conn.SetReadDeadline(time.Now().Add(30 * time.Minute))
		n, err := conn.Read(buf)
		if err != nil {
			return
		}

		line := strings.TrimSpace(string(buf[:n]))
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
			s.handleCapability(conn, tag, state)
		case "LOGIN":
			s.handleLogin(conn, tag, parts, state)
		case "AUTHENTICATE":
			s.handleAuthenticate(conn, tag, parts, state)
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
			s.handleAppend(conn, tag, parts, line, state)
		case "NOOP":
			s.handleNoop(conn, tag, state)
		case "CHECK":
			s.handleCheck(conn, tag, state)
		case "CLOSE":
			s.handleClose(conn, tag, state)
		case "SUBSCRIBE":
			s.handleSubscribe(conn, tag, parts, state)
		case "UNSUBSCRIBE":
			s.handleUnsubscribe(conn, tag, parts, state)
		case "LOGOUT":
			s.handleLogout(conn, tag)
			return
		case "STARTTLS":
			s.handleStartTLS(conn, tag, parts)
			return
		default:
			s.sendResponse(conn, fmt.Sprintf("%s BAD Unknown command: %s", tag, cmd))
		}
	}
}

func (s *IMAPServer) sendResponse(conn net.Conn, response string) {
	// Avoid logging full message bodies
	if strings.Contains(response, "BODY[] ") || strings.Contains(response, "BODY[HEADER] ") || strings.Contains(response, "RFC822.HEADER ") || strings.Contains(response, "RFC822.TEXT ") {
		// Log only the first line/metadata, mask the body
		if idx := strings.Index(response, "{"); idx != -1 {
			endIdx := strings.Index(response[idx:], "}\r\n")
			if endIdx != -1 {
				endIdx += idx + 3 // include }\r\n
				fmt.Printf("Server: %s [BODY OMITTED]\n", response[:endIdx])
			} else {
				fmt.Printf("Server: %s [BODY OMITTED]\n", response[:idx])
			}
			conn.Write([]byte(response + "\r\n"))
			return
		}
	}
	fmt.Printf("Server: %s\n", response)
	conn.Write([]byte(response + "\r\n"))
}