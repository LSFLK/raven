package models

import (
	"net"
	"testing"
)

func TestClientState_Initialization(t *testing.T) {
	// Test zero value initialization
	var state ClientState

	if state.Authenticated != false {
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
	if state.RoleMailboxIDs != nil {
		t.Error("Expected RoleMailboxIDs to be nil by default")
	}
	if state.SelectedRoleMailboxID != 0 {
		t.Error("Expected SelectedRoleMailboxID to be 0 by default")
	}
	if state.IsRoleMailbox != false {
		t.Error("Expected IsRoleMailbox to be false by default")
	}
}

func TestClientState_AuthenticationFields(t *testing.T) {
	state := ClientState{
		Authenticated: true,
		Username:      "testuser",
		UserID:        123,
		DomainID:      456,
	}

	if !state.Authenticated {
		t.Error("Expected Authenticated to be true")
	}
	if state.Username != "testuser" {
		t.Errorf("Expected Username 'testuser', got '%s'", state.Username)
	}
	if state.UserID != 123 {
		t.Errorf("Expected UserID 123, got %d", state.UserID)
	}
	if state.DomainID != 456 {
		t.Errorf("Expected DomainID 456, got %d", state.DomainID)
	}
}

func TestClientState_MailboxSelection(t *testing.T) {
	state := ClientState{
		SelectedFolder:    "INBOX",
		SelectedMailboxID: 789,
	}

	if state.SelectedFolder != "INBOX" {
		t.Errorf("Expected SelectedFolder 'INBOX', got '%s'", state.SelectedFolder)
	}
	if state.SelectedMailboxID != 789 {
		t.Errorf("Expected SelectedMailboxID 789, got %d", state.SelectedMailboxID)
	}
}

func TestClientState_MailboxStateTracking(t *testing.T) {
	state := ClientState{
		LastMessageCount: 50,
		LastRecentCount:  5,
		UIDValidity:      1234567890,
		UIDNext:          1001,
	}

	if state.LastMessageCount != 50 {
		t.Errorf("Expected LastMessageCount 50, got %d", state.LastMessageCount)
	}
	if state.LastRecentCount != 5 {
		t.Errorf("Expected LastRecentCount 5, got %d", state.LastRecentCount)
	}
	if state.UIDValidity != 1234567890 {
		t.Errorf("Expected UIDValidity 1234567890, got %d", state.UIDValidity)
	}
	if state.UIDNext != 1001 {
		t.Errorf("Expected UIDNext 1001, got %d", state.UIDNext)
	}
}

func TestClientState_RoleMailboxSupport(t *testing.T) {
	roleIDs := []int64{100, 200, 300}
	state := ClientState{
		RoleMailboxIDs:        roleIDs,
		SelectedRoleMailboxID: 200,
		IsRoleMailbox:         true,
	}

	if len(state.RoleMailboxIDs) != 3 {
		t.Errorf("Expected 3 role mailbox IDs, got %d", len(state.RoleMailboxIDs))
	}
	if state.SelectedRoleMailboxID != 200 {
		t.Errorf("Expected SelectedRoleMailboxID 200, got %d", state.SelectedRoleMailboxID)
	}
	if !state.IsRoleMailbox {
		t.Error("Expected IsRoleMailbox to be true")
	}
}

func TestClientState_ConnectionField(t *testing.T) {
	// Create a mock connection (pipe)
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	defer func() { _ = client.Close() }()

	state := ClientState{
		Conn: client,
	}

	if state.Conn == nil {
		t.Error("Expected Conn to be non-nil")
	}

	// Verify we can access the connection
	if state.Conn.LocalAddr() == nil {
		t.Error("Expected LocalAddr to be accessible")
	}
}

func TestClientState_CompleteState(t *testing.T) {
	// Test a fully populated ClientState
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	defer func() { _ = client.Close() }()

	state := ClientState{
		Authenticated:         true,
		SelectedFolder:        "Sent",
		SelectedMailboxID:     456,
		Conn:                  client,
		Username:              "john.doe",
		UserID:                789,
		DomainID:              123,
		LastMessageCount:      100,
		LastRecentCount:       10,
		UIDValidity:           9876543210,
		UIDNext:               2001,
		RoleMailboxIDs:        []int64{10, 20, 30},
		SelectedRoleMailboxID: 0,
		IsRoleMailbox:         false,
	}

	// Verify all fields
	if !state.Authenticated {
		t.Error("Expected Authenticated to be true")
	}
	if state.SelectedFolder != "Sent" {
		t.Error("Expected SelectedFolder to be 'Sent'")
	}
	if state.SelectedMailboxID != 456 {
		t.Error("Expected SelectedMailboxID to be 456")
	}
	if state.Username != "john.doe" {
		t.Error("Expected Username to be 'john.doe'")
	}
	if state.UserID != 789 {
		t.Error("Expected UserID to be 789")
	}
	if state.DomainID != 123 {
		t.Error("Expected DomainID to be 123")
	}
	if state.LastMessageCount != 100 {
		t.Error("Expected LastMessageCount to be 100")
	}
	if state.LastRecentCount != 10 {
		t.Error("Expected LastRecentCount to be 10")
	}
	if state.UIDValidity != 9876543210 {
		t.Error("Expected UIDValidity to be 9876543210")
	}
	if state.UIDNext != 2001 {
		t.Error("Expected UIDNext to be 2001")
	}
	if len(state.RoleMailboxIDs) != 3 {
		t.Error("Expected 3 role mailbox IDs")
	}
	if state.IsRoleMailbox {
		t.Error("Expected IsRoleMailbox to be false")
	}
}

func TestClientState_FieldModification(t *testing.T) {
	state := ClientState{}

	// Test that fields can be modified
	state.Authenticated = true
	state.Username = "alice"
	state.SelectedFolder = "Drafts"
	state.LastMessageCount = 25

	if !state.Authenticated {
		t.Error("Failed to modify Authenticated field")
	}
	if state.Username != "alice" {
		t.Error("Failed to modify Username field")
	}
	if state.SelectedFolder != "Drafts" {
		t.Error("Failed to modify SelectedFolder field")
	}
	if state.LastMessageCount != 25 {
		t.Error("Failed to modify LastMessageCount field")
	}
}

func TestClientState_EmptyRoleMailboxIDs(t *testing.T) {
	state := ClientState{
		RoleMailboxIDs: []int64{},
	}

	if state.RoleMailboxIDs == nil {
		t.Error("Expected RoleMailboxIDs to be non-nil empty slice")
	}
	if len(state.RoleMailboxIDs) != 0 {
		t.Errorf("Expected empty RoleMailboxIDs, got length %d", len(state.RoleMailboxIDs))
	}
}

func TestClientState_NilConnection(t *testing.T) {
	state := ClientState{
		Authenticated: true,
		Username:      "testuser",
		Conn:          nil,
	}

	if state.Conn != nil {
		t.Error("Expected Conn to be nil")
	}
}

func TestClientState_NegativeValues(t *testing.T) {
	// Test that negative values can be set (though they may not be valid in practice)
	state := ClientState{
		LastMessageCount: -1,
		LastRecentCount:  -1,
	}

	if state.LastMessageCount != -1 {
		t.Error("Expected LastMessageCount to be -1")
	}
	if state.LastRecentCount != -1 {
		t.Error("Expected LastRecentCount to be -1")
	}
}

func TestClientState_ZeroValueAfterReset(t *testing.T) {
	// Test resetting state to zero values
	state := ClientState{
		Authenticated:         true,
		Username:              "testuser",
		UserID:                123,
		SelectedFolder:        "INBOX",
		SelectedMailboxID:     456,
		LastMessageCount:      50,
		LastRecentCount:       5,
		UIDValidity:           9999,
		UIDNext:               100,
		RoleMailboxIDs:        []int64{1, 2, 3},
		SelectedRoleMailboxID: 2,
		IsRoleMailbox:         true,
	}

	// Verify state is populated before reset
	if !state.Authenticated {
		t.Fatal("Setup failed: expected Authenticated to be true before reset")
	}

	// Reset to zero value
	state = ClientState{}

	if state.Authenticated {
		t.Error("Expected Authenticated to be false after reset")
	}
	if state.Username != "" {
		t.Error("Expected Username to be empty after reset")
	}
	if state.SelectedFolder != "" {
		t.Error("Expected SelectedFolder to be empty after reset")
	}
	if state.LastMessageCount != 0 {
		t.Error("Expected LastMessageCount to be 0 after reset")
	}
	if state.RoleMailboxIDs != nil {
		t.Error("Expected RoleMailboxIDs to be nil after reset")
	}
}

func TestClientState_PartialState(t *testing.T) {
	// Test state with only some fields populated
	state := ClientState{
		Authenticated: true,
		Username:      "partialuser",
		// Other fields left at zero values
	}

	if !state.Authenticated {
		t.Error("Expected Authenticated to be true")
	}
	if state.Username != "partialuser" {
		t.Error("Expected Username to be 'partialuser'")
	}
	if state.SelectedFolder != "" {
		t.Error("Expected SelectedFolder to be empty")
	}
	if state.UserID != 0 {
		t.Error("Expected UserID to be 0")
	}
}

func TestClientState_LargeValues(t *testing.T) {
	// Test with large int64 values
	state := ClientState{
		UserID:      9223372036854775807, // Max int64
		DomainID:    9223372036854775806,
		UIDValidity: 9223372036854775805,
		UIDNext:     9223372036854775804,
	}

	if state.UserID != 9223372036854775807 {
		t.Error("Failed to store max int64 in UserID")
	}
	if state.DomainID != 9223372036854775806 {
		t.Error("Failed to store large int64 in DomainID")
	}
}

func TestClientState_StringFields(t *testing.T) {
	// Test string fields with various values
	testCases := []struct {
		username string
		folder   string
	}{
		{"simple", "INBOX"},
		{"user.with.dots", "Folder/Subfolder"},
		{"user@domain.com", "Sent Items"},
		{"", ""}, // Empty strings
		{"unicode_用户", "文件夹"},
	}

	for _, tc := range testCases {
		state := ClientState{
			Username:       tc.username,
			SelectedFolder: tc.folder,
		}

		if state.Username != tc.username {
			t.Errorf("Expected Username '%s', got '%s'", tc.username, state.Username)
		}
		if state.SelectedFolder != tc.folder {
			t.Errorf("Expected SelectedFolder '%s', got '%s'", tc.folder, state.SelectedFolder)
		}
	}
}

func TestClientState_RoleMailboxScenarios(t *testing.T) {
	testCases := []struct {
		name                  string
		roleIDs               []int64
		selectedRoleMailboxID int64
		isRoleMailbox         bool
	}{
		{
			name:                  "No role mailboxes",
			roleIDs:               nil,
			selectedRoleMailboxID: 0,
			isRoleMailbox:         false,
		},
		{
			name:                  "Single role mailbox",
			roleIDs:               []int64{100},
			selectedRoleMailboxID: 100,
			isRoleMailbox:         true,
		},
		{
			name:                  "Multiple role mailboxes",
			roleIDs:               []int64{100, 200, 300, 400},
			selectedRoleMailboxID: 200,
			isRoleMailbox:         true,
		},
		{
			name:                  "Role mailboxes but none selected",
			roleIDs:               []int64{100, 200},
			selectedRoleMailboxID: 0,
			isRoleMailbox:         false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			state := ClientState{
				RoleMailboxIDs:        tc.roleIDs,
				SelectedRoleMailboxID: tc.selectedRoleMailboxID,
				IsRoleMailbox:         tc.isRoleMailbox,
			}

			if len(state.RoleMailboxIDs) != len(tc.roleIDs) {
				t.Errorf("Expected %d role mailbox IDs, got %d", len(tc.roleIDs), len(state.RoleMailboxIDs))
			}
			if state.SelectedRoleMailboxID != tc.selectedRoleMailboxID {
				t.Errorf("Expected SelectedRoleMailboxID %d, got %d", tc.selectedRoleMailboxID, state.SelectedRoleMailboxID)
			}
			if state.IsRoleMailbox != tc.isRoleMailbox {
				t.Errorf("Expected IsRoleMailbox %v, got %v", tc.isRoleMailbox, state.IsRoleMailbox)
			}
		})
	}
}

func TestClientState_MailboxStateUpdates(t *testing.T) {
	state := ClientState{
		LastMessageCount: 10,
		LastRecentCount:  2,
		UIDValidity:      1000,
		UIDNext:          100,
	}

	// Simulate message count changes
	state.LastMessageCount = 15
	state.LastRecentCount = 5

	if state.LastMessageCount != 15 {
		t.Errorf("Expected LastMessageCount to be updated to 15, got %d", state.LastMessageCount)
	}
	if state.LastRecentCount != 5 {
		t.Errorf("Expected LastRecentCount to be updated to 5, got %d", state.LastRecentCount)
	}

	// Simulate UID changes
	state.UIDNext = 150

	if state.UIDNext != 150 {
		t.Errorf("Expected UIDNext to be updated to 150, got %d", state.UIDNext)
	}
}

func TestClientState_PointerBehavior(t *testing.T) {
	// Test that ClientState can be used with pointers
	state := &ClientState{
		Authenticated: true,
		Username:      "pointeruser",
	}

	if !state.Authenticated {
		t.Error("Expected Authenticated to be true via pointer")
	}
	if state.Username != "pointeruser" {
		t.Error("Expected Username to be 'pointeruser' via pointer")
	}

	// Modify via pointer
	state.SelectedFolder = "INBOX"
	if state.SelectedFolder != "INBOX" {
		t.Error("Failed to modify via pointer")
	}
}
