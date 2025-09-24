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
			s.handleCapability(conn, tag)
		case "LOGIN":
			s.handleLogin(conn, tag, parts, state)
		case "LIST":
			s.handleList(conn, tag, parts, state)
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
		case "NOOP":
			s.sendResponse(conn, fmt.Sprintf("%s OK NOOP completed", tag))
		case "LOGOUT":
			s.handleLogout(conn, tag)
			return
		case "STARTTLS":
			s.handleStartTLS(conn, tag)
			return
		default:
			s.sendResponse(conn, fmt.Sprintf("%s BAD Unknown command: %s", tag, cmd))
		}
	}
}

func (s *IMAPServer) sendResponse(conn net.Conn, response string) {
	fmt.Printf("Server: %s\n", response)
	conn.Write([]byte(response + "\r\n"))
}