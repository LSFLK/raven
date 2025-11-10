package server_test

import (
	"fmt"
	"strings"
	"testing"

	"raven/internal/models"
	"raven/test/helpers"
)

// TestListCommand_RFC3501_Examples tests examples from RFC 3501 Section 6.3.8
func TestListCommand_RFC3501_Examples(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := helpers.SetupAuthenticatedState(t, server, "testuser")

	testCases := []struct {
		name              string
		reference         string
		mailbox           string
		expectedResponses []string
		description       string
	}{
		{
			name:      "Example1_EmptyArgs",
			reference: `""`,
			mailbox:   `""`,
			expectedResponses: []string{
				`* LIST (\Noselect) "/" ""`,
				`OK LIST Completed`,
			},
			description: "LIST \"\" \"\" - Get hierarchy delimiter",
		},
		{
			name:      "Example2_Reference",
			reference: `"#news.comp.mail.misc"`,
			mailbox:   `""`,
			expectedResponses: []string{
				`* LIST (\Noselect) "/" "#news.comp.mail.misc"`,
				`OK LIST Completed`,
			},
			description: "LIST with reference and empty mailbox",
		},
		{
			name:      "Example3_UserStaffJones",
			reference: `"/usr/staff/jones"`,
			mailbox:   `""`,
			expectedResponses: []string{
				`* LIST (\Noselect) "/" "/usr/staff/jones"`,
				`OK LIST Completed`,
			},
			description: "LIST with absolute path reference",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			conn.ClearWriteBuffer()
			server.HandleList(conn, "A001", []string{"A001", "LIST", tc.reference, tc.mailbox}, state)

			response := conn.GetWrittenData()

			// Check that we get the expected hierarchy delimiter response
			if !strings.Contains(response, `* LIST (\Noselect) "/"`) {
				t.Errorf("Expected hierarchy delimiter response, got: %s", response)
			}

			// Check completion
			if !strings.Contains(response, "A001 OK LIST completed") {
				t.Errorf("Expected OK completion, got: %s", response)
			}

			t.Logf("Test %s: %s\nResponse: %s", tc.name, tc.description, response)
		})
	}
}

// TestListCommand_WildcardEdgeCases tests edge cases with wildcard patterns
func TestListCommand_WildcardEdgeCases(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := helpers.SetupAuthenticatedState(t, server, "testuser")

	testCases := []struct {
		pattern          string
		shouldMatchINBOX bool
		shouldMatchSent  bool
		shouldMatchTrash bool
		shouldMatchDrafts bool
		description      string
	}{
		{"*", true, true, true, true, "Asterisk should match all"},
		{"%", true, true, true, true, "Percent should match all (no hierarchy)"},
		{"I*X", true, false, false, false, "I*X should match INBOX"},
		{"IN%OX", true, false, false, false, "IN%OX should match INBOX (no delimiters in INBOX)"},
		{"INBOX", true, false, false, false, "Exact match INBOX"},
		{"inbox", true, false, false, false, "Case insensitive INBOX"},
		{"*ent", false, true, false, false, "*ent should match Sent"},
		{"Se*", false, true, false, false, "Se* should match Sent"},
		{"S%", false, true, false, false, "S% should match Sent"},
		{"*ox", true, false, false, false, "*ox should only match INBOX"},
		{"Tr*", false, false, true, false, "Tr* should match Trash"},
		{"Dr*", false, false, false, true, "Dr* should match Drafts"},
		{"X*", false, false, false, false, "X* should match nothing"},
		{"", false, false, false, false, "Empty pattern (handled as hierarchy delimiter case)"},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Pattern_%s", tc.pattern), func(t *testing.T) {
			conn.ClearWriteBuffer()
			
			if tc.pattern == "" {
				// Empty pattern is special case - hierarchy delimiter
				server.HandleList(conn, "A001", []string{"A001", "LIST", `""`, `""`}, state)
				response := conn.GetWrittenData()
				if !strings.Contains(response, `\Noselect`) {
					t.Errorf("Empty pattern should return hierarchy delimiter, got: %s", response)
				}
				return
			}

			server.HandleList(conn, "A001", []string{"A001", "LIST", `""`, fmt.Sprintf(`"%s"`, tc.pattern)}, state)

			response := conn.GetWrittenData()

			// Check INBOX matching
			inboxFound := strings.Contains(response, `"INBOX"`)
			if inboxFound != tc.shouldMatchINBOX {
				t.Errorf("Pattern %s: INBOX match expected=%v, got=%v. Response: %s",
					tc.pattern, tc.shouldMatchINBOX, inboxFound, response)
			}

			// Check Sent matching  
			sentFound := strings.Contains(response, `"Sent"`)
			if sentFound != tc.shouldMatchSent {
				t.Errorf("Pattern %s: Sent match expected=%v, got=%v. Response: %s",
					tc.pattern, tc.shouldMatchSent, sentFound, response)
			}

			// Check Trash matching  
			trashFound := strings.Contains(response, `"Trash"`)
			if trashFound != tc.shouldMatchTrash {
				t.Errorf("Pattern %s: Trash match expected=%v, got=%v. Response: %s",
					tc.pattern, tc.shouldMatchTrash, trashFound, response)
			}

			// Check Drafts matching  
			draftsFound := strings.Contains(response, `"Drafts"`)
			if draftsFound != tc.shouldMatchDrafts {
				t.Errorf("Pattern %s: Drafts match expected=%v, got=%v. Response: %s",
					tc.pattern, tc.shouldMatchDrafts, draftsFound, response)
			}

			// All should complete successfully
			if !strings.Contains(response, "A001 OK LIST completed") {
				t.Errorf("Pattern %s should complete successfully, got: %s", tc.pattern, response)
			}
		})
	}
}

// TestListCommand_HierarchyDelimiterVariations tests various hierarchy delimiter scenarios
func TestListCommand_HierarchyDelimiterVariations(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := helpers.SetupAuthenticatedState(t, server, "testuser")

	testCases := []struct {
		reference         string
		mailbox           string
		expectedRootName  string
		description       string
	}{
		{`""`, `""`, `""`, "Both empty - should return empty root"},
		{`"foo"`, `""`, `"foo"`, "Reference only - should return reference as root"},
		{`"foo/bar"`, `""`, `"foo/bar"`, "Reference with path - should return full reference"},
		{`"~/Mail/"`, `""`, `"~/Mail/"`, "Home directory reference"},
		{`"/absolute/path"`, `""`, `"/absolute/path"`, "Absolute path reference"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			conn.ClearWriteBuffer()
			server.HandleList(conn, "A001", []string{"A001", "LIST", tc.reference, tc.mailbox}, state)

			response := conn.GetWrittenData()
			lines := strings.Split(strings.TrimSpace(response), "\r\n")

			// Should have exactly 2 lines: LIST response and completion
			if len(lines) != 2 {
				t.Fatalf("Expected 2 lines, got %d: %v", len(lines), lines)
			}

			// Check that the response contains the expected root name
			expectedPattern := fmt.Sprintf(`* LIST (\Noselect) "/" %s`, tc.expectedRootName)
			if !strings.Contains(response, expectedPattern) {
				t.Errorf("Expected pattern %s, got: %s", expectedPattern, response)
			}

			// Check completion
			if !strings.Contains(lines[1], "A001 OK LIST completed") {
				t.Errorf("Expected completion, got: %s", lines[1])
			}
		})
	}
}

// TestListCommand_ReferencePatternCombination tests combination of reference and pattern
func TestListCommand_ReferencePatternCombination(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := helpers.SetupAuthenticatedState(t, server, "testuser")

	testCases := []struct {
		reference       string
		pattern         string
		expectedMatches int
		description     string
	}{
		{`""`, `"*"`, 4, "Empty reference with * should match all 4 mailboxes"},
		{`""`, `"INBOX"`, 1, "Empty reference with INBOX should match 1"},
		{`"INBOX"`, `"*"`, 0, "INBOX reference with * should match none (no sub-mailboxes)"},
		{`"Mail/"`, `"*"`, 0, "Mail/ reference should match none of our flat mailboxes"},
		{`""`, `"I*"`, 1, "Empty reference with I* should match INBOX"},
		{`""`, `"S*"`, 1, "Empty reference with S* should match Sent"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			conn.ClearWriteBuffer()
			server.HandleList(conn, "A001", []string{"A001", "LIST", tc.reference, tc.pattern}, state)

			response := conn.GetWrittenData()
			lines := strings.Split(strings.TrimSpace(response), "\r\n")

			// Count LIST responses (exclude completion line)
			listLines := 0
			for _, line := range lines {
				if strings.HasPrefix(line, "* LIST ") {
					listLines++
				}
			}

			if listLines != tc.expectedMatches {
				t.Errorf("Reference %s Pattern %s: expected %d matches, got %d. Response: %s",
					tc.reference, tc.pattern, tc.expectedMatches, listLines, response)
			}

			// Check completion
			if !strings.Contains(response, "A001 OK LIST completed") {
				t.Errorf("Expected completion, got: %s", response)
			}
		})
	}
}

// TestListCommand_ErrorConditions tests various error conditions
func TestListCommand_ErrorConditions(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()

	// Test unauthenticated state
	t.Run("Unauthenticated", func(t *testing.T) {
		state := &models.ClientState{Authenticated: false}
		conn.ClearWriteBuffer()
		server.HandleList(conn, "A001", []string{"A001", "LIST", `""`, `"*"`}, state)

		response := conn.GetWrittenData()
		if !strings.Contains(response, "A001 NO Please authenticate first") {
			t.Errorf("Expected authentication error, got: %s", response)
		}
	})

	// Test insufficient arguments
	t.Run("InsufficientArgs", func(t *testing.T) {
		state := &models.ClientState{Authenticated: true, Username: "testuser"}
		conn.ClearWriteBuffer()
		server.HandleList(conn, "A001", []string{"A001", "LIST", `""`}, state)

		response := conn.GetWrittenData()
		if !strings.Contains(response, "A001 BAD LIST command requires reference and mailbox arguments") {
			t.Errorf("Expected BAD response, got: %s", response)
		}
	})

	// Test minimal valid arguments
	t.Run("MinimalValid", func(t *testing.T) {
		state := &models.ClientState{Authenticated: true, Username: "testuser"}
		conn.ClearWriteBuffer()
		server.HandleList(conn, "A001", []string{"A001", "LIST", `""`, `""`}, state)

		response := conn.GetWrittenData()
		if strings.Contains(response, "BAD") || strings.Contains(response, "NO") {
			t.Errorf("Minimal valid arguments should succeed, got: %s", response)
		}
		if !strings.Contains(response, "A001 OK LIST completed") {
			t.Errorf("Expected OK completion, got: %s", response)
		}
	})
}

// TestListCommand_SpecialCharacters tests LIST with special characters in patterns
func TestListCommand_SpecialCharacters(t *testing.T) {
	server := helpers.SetupTestServerSimple(t)
	conn := helpers.NewMockConn()
	state := helpers.SetupAuthenticatedState(t, server, "testuser")

	testCases := []struct {
		pattern     string
		description string
	}{
		{`"*"`, "Single asterisk"},
		{`"%"`, "Single percent"},
		{`"**"`, "Double asterisk"},
		{`"%%"`, "Double percent"},
		{`"*%"`, "Asterisk and percent"},
		{`"%*"`, "Percent and asterisk"},
		{`"I*B*X"`, "Multiple wildcards"},
		{`"*I*N*B*O*X*"`, "Alternating wildcards"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			conn.ClearWriteBuffer()
			server.HandleList(conn, "A001", []string{"A001", "LIST", `""`, tc.pattern}, state)

			response := conn.GetWrittenData()

			// Should not error
			if strings.Contains(response, "BAD") || strings.Contains(response, "NO") {
				t.Errorf("Pattern %s should not error, got: %s", tc.pattern, response)
			}

			// Should complete
			if !strings.Contains(response, "A001 OK LIST completed") {
				t.Errorf("Pattern %s should complete, got: %s", tc.pattern, response)
			}

			t.Logf("Pattern %s: %s", tc.pattern, response)
		})
	}
}
