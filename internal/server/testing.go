//go:build test
// +build test

package server

import (
	"net"

	"go-imap/internal/models"
)

// TestInterface provides access to internal methods for testing
// This interface should only be used in tests
type TestInterface struct {
	server *IMAPServer
}

// NewTestInterface creates a new test interface wrapper
// This function should only be used in tests
func NewTestInterface(server *IMAPServer) *TestInterface {
	return &TestInterface{server: server}
}

// HandleCapability exposes the capability handler for testing
func (t *TestInterface) HandleCapability(conn net.Conn, tag string, state *models.ClientState) {
	t.server.handleCapability(conn, tag, state)
}

// HandleLogin exposes the login handler for testing
func (t *TestInterface) HandleLogin(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	t.server.handleLogin(conn, tag, parts, state)
}

// HandleList exposes the list handler for testing
func (t *TestInterface) HandleList(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	t.server.handleList(conn, tag, parts, state)
}

// HandleCreate exposes the create handler for testing
func (t *TestInterface) HandleCreate(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	t.server.handleCreate(conn, tag, parts, state)
}

// HandleDelete exposes the delete handler for testing
func (t *TestInterface) HandleDelete(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	t.server.handleDelete(conn, tag, parts, state)
}

// HandleRename exposes the rename handler for testing
func (t *TestInterface) HandleRename(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	t.server.handleRename(conn, tag, parts, state)
}

// HandleLogout exposes the logout handler for testing
func (t *TestInterface) HandleLogout(conn net.Conn, tag string) {
	t.server.handleLogout(conn, tag)
}

// HandleIdle exposes the idle handler for testing
func (t *TestInterface) HandleIdle(conn net.Conn, tag string, state *models.ClientState) {
	t.server.handleIdle(conn, tag, state)
}

// HandleNamespace exposes the namespace handler for testing
func (t *TestInterface) HandleNamespace(conn net.Conn, tag string, state *models.ClientState) {
	t.server.handleNamespace(conn, tag, state)
}

// HandleUnselect exposes the unselect handler for testing
func (t *TestInterface) HandleUnselect(conn net.Conn, tag string, state *models.ClientState) {
	t.server.handleUnselect(conn, tag, state)
}

// HandleNoop exposes the noop handler for testing
func (t *TestInterface) HandleNoop(conn net.Conn, tag string, state *models.ClientState) {
	t.server.handleNoop(conn, tag, state)
}

// HandleCheck exposes the check handler for testing
func (t *TestInterface) HandleCheck(conn net.Conn, tag string, state *models.ClientState) {
	t.server.handleCheck(conn, tag, state)
}

// HandleClose exposes the close handler for testing
func (t *TestInterface) HandleClose(conn net.Conn, tag string, state *models.ClientState) {
	t.server.handleClose(conn, tag, state)
}

// HandleExpunge exposes the expunge handler for testing
func (t *TestInterface) HandleExpunge(conn net.Conn, tag string, state *models.ClientState) {
	t.server.handleExpunge(conn, tag, state)
}

// HandleSelect exposes the select handler for testing
func (t *TestInterface) HandleSelect(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	t.server.handleSelect(conn, tag, parts, state)
}

// HandleExamine exposes the examine handler for testing (uses same handler as SELECT)
func (t *TestInterface) HandleExamine(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	t.server.handleSelect(conn, tag, parts, state)
}

// HandleAppend exposes the append handler for testing
func (t *TestInterface) HandleAppend(conn net.Conn, tag string, parts []string, fullLine string, state *models.ClientState) {
	t.server.handleAppend(conn, tag, parts, fullLine, state)
}

// HandleAuthenticate exposes the authenticate handler for testing
func (t *TestInterface) HandleAuthenticate(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	t.server.handleAuthenticate(conn, tag, parts, state)
}

// HandleStartTLS exposes the STARTTLS handler for testing
func (t *TestInterface) HandleStartTLS(conn net.Conn, tag string, parts []string) {
	t.server.handleStartTLS(conn, tag, parts)
}

// HandleSubscribe exposes the subscribe handler for testing
func (t *TestInterface) HandleSubscribe(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	t.server.handleSubscribe(conn, tag, parts, state)
}

// HandleUnsubscribe exposes the unsubscribe handler for testing
func (t *TestInterface) HandleUnsubscribe(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	t.server.handleUnsubscribe(conn, tag, parts, state)
}

// HandleLsub exposes the lsub handler for testing
func (t *TestInterface) HandleLsub(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	t.server.handleLsub(conn, tag, parts, state)
}

// HandleStatus exposes the status handler for testing
func (t *TestInterface) HandleStatus(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	t.server.handleStatus(conn, tag, parts, state)
}

// HandleSearch exposes the search handler for testing
func (t *TestInterface) HandleSearch(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	t.server.handleSearch(conn, tag, parts, state)
}

// HandleFetch exposes the fetch handler for testing
func (t *TestInterface) HandleFetch(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	t.server.handleFetch(conn, tag, parts, state)
}

// HandleStore exposes the store handler for testing
func (t *TestInterface) HandleStore(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	t.server.handleStore(conn, tag, parts, state)
}

// HandleCopy exposes the copy handler for testing
func (t *TestInterface) HandleCopy(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	t.server.handleCopy(conn, tag, parts, state)
}

// HandleUID exposes the UID handler for testing
func (t *TestInterface) HandleUID(conn net.Conn, tag string, parts []string, state *models.ClientState) {
	t.server.handleUID(conn, tag, parts, state)
}

// SetTLSCertificates sets custom TLS certificate paths for testing
func (t *TestInterface) SetTLSCertificates(certPath, keyPath string) {
	t.server.SetTLSCertificates(certPath, keyPath)
}

// GetDBManager exposes the database manager for testing
func (t *TestInterface) GetDBManager() interface{} {
	return t.server.dbManager
}

// GetDB provides backward compatibility - returns the DBManager
// Note: Tests should migrate to using GetDBManager() and per-user databases
func (t *TestInterface) GetDB() interface{} {
	return t.server.dbManager
}

// SendResponse exposes the sendResponse method for testing
func (t *TestInterface) SendResponse(conn net.Conn, response string) {
	t.server.sendResponse(conn, response)
}

// HandleClientExported exposes handleClient for testing
func HandleClientExported(server *TestInterface, conn net.Conn) {
	handleClient(server.server, conn, &models.ClientState{})
}

// GetServer returns the underlying IMAPServer for compatibility
func (t *TestInterface) GetServer() *IMAPServer {
	return t.server
}
