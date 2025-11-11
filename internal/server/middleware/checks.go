package middleware

import (
	"database/sql"
	"fmt"
	"net"

	"raven/internal/models"
)

// ServerInterface defines methods needed from IMAPServer for middleware
type ServerInterface interface {
	SendResponse(conn net.Conn, response string)
	GetUserDB(userID int64) (*sql.DB, error)
	GetSelectedDB(state *models.ClientState) (*sql.DB, int64, error)
	GetSharedDB() *sql.DB
}

// HandlerFunc is the standard handler function signature
type HandlerFunc func(conn net.Conn, tag string, parts []string, state *models.ClientState)

// RequireAuth ensures the client is authenticated before proceeding
func RequireAuth(server ServerInterface, handler HandlerFunc) HandlerFunc {
	return func(conn net.Conn, tag string, parts []string, state *models.ClientState) {
		if !state.Authenticated {
			server.SendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
			return
		}
		handler(conn, tag, parts, state)
	}
}

// RequireMailboxSelected ensures a mailbox is selected before proceeding
func RequireMailboxSelected(server ServerInterface, handler HandlerFunc) HandlerFunc {
	return func(conn net.Conn, tag string, parts []string, state *models.ClientState) {
		if state.SelectedMailboxID == 0 {
			server.SendResponse(conn, fmt.Sprintf("%s NO No folder selected", tag))
			return
		}
		handler(conn, tag, parts, state)
	}
}

// RequireAuthAndMailbox combines authentication and mailbox selection checks
func RequireAuthAndMailbox(server ServerInterface, handler HandlerFunc) HandlerFunc {
	return RequireAuth(server, RequireMailboxSelected(server, handler))
}

// ValidateMinArgs ensures the command has the minimum required number of arguments
func ValidateMinArgs(server ServerInterface, minArgs int, errorMsg string, handler HandlerFunc) HandlerFunc {
	return func(conn net.Conn, tag string, parts []string, state *models.ClientState) {
		if len(parts) < minArgs {
			server.SendResponse(conn, fmt.Sprintf("%s BAD %s", tag, errorMsg))
			return
		}
		handler(conn, tag, parts, state)
	}
}

// WithDB provides database access to the handler, checking for errors
type HandlerWithDBFunc func(conn net.Conn, tag string, parts []string, state *models.ClientState, db *sql.DB)

// WithSelectedDB wraps a handler to provide access to the selected mailbox database
func WithSelectedDB(server ServerInterface, handler HandlerWithDBFunc) HandlerFunc {
	return func(conn net.Conn, tag string, parts []string, state *models.ClientState) {
		targetDB, _, err := server.GetSelectedDB(state)
		if err != nil {
			server.SendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
			return
		}
		handler(conn, tag, parts, state, targetDB)
	}
}

// WithUserDB wraps a handler to provide access to the user database
func WithUserDB(server ServerInterface, handler HandlerWithDBFunc) HandlerFunc {
	return func(conn net.Conn, tag string, parts []string, state *models.ClientState) {
		userDB, err := server.GetUserDB(state.UserID)
		if err != nil {
			server.SendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
			return
		}
		handler(conn, tag, parts, state, userDB)
	}
}
