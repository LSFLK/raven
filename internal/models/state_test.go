package models

import (
	"net"
	"testing"
)

func TestClientState_Initialization(t *testing.T) {
	var state ClientState

	if state.Authenticated {
		t.Error("Expected Authenticated to be false by default")
	}
	if state.SelectedFolder != "" {
		t.Error("Expected SelectedFolder to be empty by default")
	}
	if state.SelectedMailboxID != 0 {
		t.Error("Expected SelectedMailboxID to be 0 by default")
	}
	if state.Username != "" {
		t.Error("Expected Username to be empty by default")
	}
	if state.Email != "" {
		t.Error("Expected Email to be empty by default")
	}
	if state.UserID != 0 {
		t.Error("Expected UserID to be 0 by default")
	}
	if state.DomainID != 0 {
		t.Error("Expected DomainID to be 0 by default")
	}
	if state.LastMessageCount != 0 {
		t.Error("Expected LastMessageCount to be 0 by default")
	}
	if state.LastRecentCount != 0 {
		t.Error("Expected LastRecentCount to be 0 by default")
	}
	if state.UIDValidity != 0 {
		t.Error("Expected UIDValidity to be 0 by default")
	}
	if state.UIDNext != 0 {
		t.Error("Expected UIDNext to be 0 by default")
	}
}

func TestClientState_ConnectionField(t *testing.T) {
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	defer func() { _ = client.Close() }()

	state := ClientState{Conn: client}
	if state.Conn == nil {
		t.Error("Expected Conn to be non-nil")
	}
}

func TestClientState_FieldModification(t *testing.T) {
	state := ClientState{}
	state.Authenticated = true
	state.Username = "alice"
	state.Email = "alice@example.com"
	state.SelectedFolder = "Drafts"
	state.LastMessageCount = 25
	state.UIDNext = 102

	if !state.Authenticated {
		t.Error("Failed to modify Authenticated field")
	}
	if state.Username != "alice" {
		t.Error("Failed to modify Username field")
	}
	if state.Email != "alice@example.com" {
		t.Error("Failed to modify Email field")
	}
	if state.SelectedFolder != "Drafts" {
		t.Error("Failed to modify SelectedFolder field")
	}
	if state.LastMessageCount != 25 {
		t.Error("Failed to modify LastMessageCount field")
	}
	if state.UIDNext != 102 {
		t.Error("Failed to modify UIDNext field")
	}
}
