package mailbox

import (
	"database/sql"
	"fmt"
	"net"
	"strings"

	"raven/internal/blobstorage"
	"raven/internal/db"
	"raven/internal/models"
	"raven/internal/server/utils"
)

// ServerDeps defines the dependencies that mailbox handlers need from the server
type ServerDeps interface {
	SendResponse(conn net.Conn, response string)
	GetUserDB(userID int64) (*sql.DB, error)
	GetSharedDB() *sql.DB
	GetDBManager() *db.DBManager
	GetS3Storage() *blobstorage.S3BlobStorage
}

// ===== LIST =====

func HandleList(deps ServerDeps, conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		deps.SendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	// Parse arguments according to RFC 3501
	if len(parts) < 4 {
		deps.SendResponse(conn, fmt.Sprintf("%s BAD LIST command requires reference and mailbox arguments", tag))
		return
	}

	reference := utils.ParseQuotedString(parts[2])
	mailboxPattern := utils.ParseQuotedString(parts[3])

	// Handle special case: empty mailbox name to get hierarchy delimiter
	if mailboxPattern == "" {
		// Return hierarchy delimiter and root name
		hierarchyDelimiter := "/"
		rootName := reference
		if reference == "" {
			rootName = ""
		}
		deps.SendResponse(conn, fmt.Sprintf("* LIST (\\Noselect) \"%s\" \"%s\"", hierarchyDelimiter, rootName))
		deps.SendResponse(conn, fmt.Sprintf("%s OK LIST completed", tag))
		return
	}

	// Get user database
	userDB, err := deps.GetUserDB(state.UserID)
	if err != nil {
		deps.SendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
		return
	}

	// Get all mailboxes for the user
	mailboxes, err := db.GetUserMailboxesPerUser(userDB, state.UserID)
	if err != nil {
		deps.SendResponse(conn, fmt.Sprintf("%s NO LIST failure: can't list mailboxes", tag))
		return
	}

	// Apply reference and pattern matching
	matches := utils.FilterMailboxes(mailboxes, reference, mailboxPattern)

	// Return matching mailboxes
	for _, mailboxName := range matches {
		attrs := utils.GetMailboxAttributes(mailboxName)
		deps.SendResponse(conn, fmt.Sprintf("* LIST (%s) \"/\" \"%s\"", attrs, mailboxName))
	}

	// List role mailboxes if user has any assigned
	if len(state.RoleMailboxIDs) > 0 {
		sharedDB := deps.GetSharedDB()

		// Collect all role mailbox paths first
		var allRolePaths []string

		for _, roleMailboxID := range state.RoleMailboxIDs {
			// Get role mailbox email
			roleEmail, _, err := db.GetRoleMailboxByID(sharedDB, roleMailboxID)
			if err != nil {
				continue
			}

			// Get role mailbox database
			roleDB, err := deps.GetDBManager().GetRoleMailboxDB(roleMailboxID)
			if err != nil {
				continue
			}

			// Get mailboxes in the role mailbox (userID 0 for role mailboxes)
			roleMailboxes, err := db.GetUserMailboxesPerUser(roleDB, 0)
			if err != nil {
				continue
			}

			// Build full paths for role mailboxes
			rolePrefix := fmt.Sprintf("Roles/%s/", roleEmail)
			for _, mbx := range roleMailboxes {
				allRolePaths = append(allRolePaths, rolePrefix+mbx)
			}

			// Add the role folder itself
			allRolePaths = append(allRolePaths, fmt.Sprintf("Roles/%s", roleEmail))
		}

		// Add the top-level Roles folder
		allRolePaths = append(allRolePaths, "Roles")

		// Filter and list role paths
		roleMatches := utils.FilterMailboxes(allRolePaths, reference, mailboxPattern)
		for _, matchedPath := range roleMatches {
			// Skip if this doesn't start with "Roles" - prevents duplicate personal mailboxes
			if !strings.HasPrefix(matchedPath, "Roles") {
				continue
			}

			// Determine attributes based on the path
			if matchedPath == "Roles" {
				// Top-level Roles folder
				deps.SendResponse(conn, fmt.Sprintf("* LIST (\\Noselect \\HasChildren) \"/\" \"%s\"", matchedPath))
			} else if strings.Count(matchedPath, "/") == 1 {
				// Roles/email@domain - folder level
				deps.SendResponse(conn, fmt.Sprintf("* LIST (\\Noselect \\HasChildren) \"/\" \"%s\"", matchedPath))
			} else {
				// Actual mailbox: Roles/email@domain/INBOX
				parts := strings.Split(matchedPath, "/")
				mailboxName := parts[len(parts)-1]
				attrs := utils.GetMailboxAttributes(mailboxName)
				deps.SendResponse(conn, fmt.Sprintf("* LIST (%s) \"/\" \"%s\"", attrs, matchedPath))
			}
		}
	}

	deps.SendResponse(conn, fmt.Sprintf("%s OK LIST completed", tag))
}

// ===== LSUB =====

func HandleLsub(deps ServerDeps, conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		deps.SendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	// Parse arguments according to RFC 3501
	// LSUB requires reference name and mailbox name with possible wildcards
	if len(parts) < 4 {
		deps.SendResponse(conn, fmt.Sprintf("%s BAD LSUB command requires reference and mailbox arguments", tag))
		return
	}

	reference := utils.ParseQuotedString(parts[2])
	mailboxPattern := utils.ParseQuotedString(parts[3])

	// Handle special case: empty mailbox name to get hierarchy delimiter
	if mailboxPattern == "" {
		// Return hierarchy delimiter and root name
		hierarchyDelimiter := "/"
		rootName := reference
		if reference == "" {
			rootName = ""
		}
		deps.SendResponse(conn, fmt.Sprintf("* LSUB (\\Noselect) \"%s\" \"%s\"", hierarchyDelimiter, rootName))
		deps.SendResponse(conn, fmt.Sprintf("%s OK LSUB completed", tag))
		return
	}

	// Get user database
	userDB, err := deps.GetUserDB(state.UserID)
	if err != nil {
		deps.SendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
		return
	}

	// Get subscribed mailboxes from database
	subscriptions, err := db.GetUserSubscriptionsPerUser(userDB, state.UserID)
	if err != nil {
		fmt.Printf("Failed to get subscriptions for user %s: %v\n", state.Username, err)
		deps.SendResponse(conn, fmt.Sprintf("%s NO LSUB failure: can't list that reference or name", tag))
		return
	}

	// If no subscriptions exist, subscribe to default mailboxes
	if len(subscriptions) == 0 {
		defaultMailboxes := []string{"INBOX", "Sent", "Drafts", "Trash", "Spam"}
		for _, mailbox := range defaultMailboxes {
			_ = db.SubscribeToMailboxPerUser(userDB, state.UserID, mailbox)
		}
		subscriptions = defaultMailboxes
	}

	// Apply reference and pattern matching to subscriptions
	matches := utils.FilterMailboxes(subscriptions, reference, mailboxPattern)

	// RFC 3501 Special case: When using % wildcard, if "foo/bar" is subscribed
	// but "foo" is not, we must return "foo" with \Noselect attribute
	hierarchyDelimiter := "/"
	impliedParents := make(map[string]bool)

	// Collect all implied parent mailboxes when using % wildcard
	if strings.Contains(mailboxPattern, "%") {
		// Look at ALL subscriptions (not just matches) to find implied parents
		for _, mailbox := range subscriptions {
			// Check if this mailbox has hierarchy
			if strings.Contains(mailbox, hierarchyDelimiter) {
				mailboxParts := strings.Split(mailbox, hierarchyDelimiter)
				// Build up parent paths
				currentPath := ""
				for i := 0; i < len(mailboxParts)-1; i++ {
					if i > 0 {
						currentPath += hierarchyDelimiter
					}
					currentPath += mailboxParts[i]

					// If parent is not subscribed and matches the pattern, mark as implied
					if !utils.Contains(subscriptions, currentPath) {
						// Check if this implied parent matches the pattern
						canonicalPattern := utils.BuildCanonicalPattern(reference, mailboxPattern, hierarchyDelimiter)
						if utils.MatchesPattern(currentPath, canonicalPattern, hierarchyDelimiter) {
							impliedParents[currentPath] = true
						}
					}
				}
			}
		}
	}

	// Send implied parents with \Noselect first
	for parent := range impliedParents {
		deps.SendResponse(conn, fmt.Sprintf("* LSUB (\\Noselect) \"/\" \"%s\"", parent))
	}

	// Send actual subscribed mailboxes
	for _, mailboxName := range matches {
		attrs := utils.GetMailboxAttributes(mailboxName)
		deps.SendResponse(conn, fmt.Sprintf("* LSUB (%s) \"/\" \"%s\"", attrs, mailboxName))
	}

	// Include role mailboxes in LSUB (auto-subscribed)
	if len(state.RoleMailboxIDs) > 0 {
		sharedDB := deps.GetSharedDB()

		// Collect all role mailbox paths
		var allRolePaths []string

		for _, roleMailboxID := range state.RoleMailboxIDs {
			// Get role mailbox email
			roleEmail, _, err := db.GetRoleMailboxByID(sharedDB, roleMailboxID)
			if err != nil {
				continue
			}

			// Get role mailbox database
			roleDB, err := deps.GetDBManager().GetRoleMailboxDB(roleMailboxID)
			if err != nil {
				continue
			}

			// Get mailboxes in the role mailbox
			roleMailboxes, err := db.GetUserMailboxesPerUser(roleDB, 0)
			if err != nil {
				continue
			}

			// Build full paths for role mailboxes
			rolePrefix := fmt.Sprintf("Roles/%s/", roleEmail)
			for _, mbx := range roleMailboxes {
				allRolePaths = append(allRolePaths, rolePrefix+mbx)
			}

			// Add the role folder itself
			allRolePaths = append(allRolePaths, fmt.Sprintf("Roles/%s", roleEmail))
		}

		// Add the top-level Roles folder
		allRolePaths = append(allRolePaths, "Roles")

		// Filter and list role paths for LSUB
		roleMatches := utils.FilterMailboxes(allRolePaths, reference, mailboxPattern)
		for _, matchedPath := range roleMatches {
			// Skip if this doesn't start with "Roles"
			if !strings.HasPrefix(matchedPath, "Roles") {
				continue
			}

			// Determine attributes based on the path
			if matchedPath == "Roles" {
				// Top-level Roles folder
				deps.SendResponse(conn, fmt.Sprintf("* LSUB (\\Noselect \\HasChildren) \"/\" \"%s\"", matchedPath))
			} else if strings.Count(matchedPath, "/") == 1 {
				// Roles/email@domain - folder level
				deps.SendResponse(conn, fmt.Sprintf("* LSUB (\\Noselect \\HasChildren) \"/\" \"%s\"", matchedPath))
			} else {
				// Actual mailbox: Roles/email@domain/INBOX
				parts := strings.Split(matchedPath, "/")
				mailboxName := parts[len(parts)-1]
				attrs := utils.GetMailboxAttributes(mailboxName)
				deps.SendResponse(conn, fmt.Sprintf("* LSUB (%s) \"/\" \"%s\"", attrs, matchedPath))
			}
		}
	}

	deps.SendResponse(conn, fmt.Sprintf("%s OK LSUB completed", tag))
}

// ===== CREATE =====

func HandleCreate(deps ServerDeps, conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		deps.SendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	if len(parts) < 3 {
		deps.SendResponse(conn, fmt.Sprintf("%s BAD CREATE requires mailbox name", tag))
		return
	}

	// Parse mailbox name (could be quoted)
	mailboxName := strings.Trim(parts[2], "\"")

	// Remove trailing hierarchy separator if present
	// According to RFC 3501, the name created is without the trailing hierarchy delimiter
	mailboxName = strings.TrimSuffix(mailboxName, "/")

	// Validate mailbox name
	if mailboxName == "" {
		deps.SendResponse(conn, fmt.Sprintf("%s NO Cannot create mailbox with empty name", tag))
		return
	}

	// Check if trying to create INBOX (case-insensitive)
	if strings.ToUpper(mailboxName) == "INBOX" {
		deps.SendResponse(conn, fmt.Sprintf("%s NO Cannot create INBOX - it already exists", tag))
		return
	}

	// Get user database
	userDB, err := deps.GetUserDB(state.UserID)
	if err != nil {
		deps.SendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
		return
	}

	// Check if mailbox already exists
	exists, err := db.MailboxExistsPerUser(userDB, state.UserID, mailboxName)
	if err != nil {
		deps.SendResponse(conn, fmt.Sprintf("%s NO Server error: cannot check mailbox existence", tag))
		return
	}

	if exists {
		deps.SendResponse(conn, fmt.Sprintf("%s NO Mailbox already exists", tag))
		return
	}

	// Handle hierarchy creation
	// If the mailbox name contains hierarchy separators, create parent mailboxes if needed
	if strings.Contains(mailboxName, "/") {
		parts := strings.Split(mailboxName, "/")
		currentPath := ""

		// Create each level of the hierarchy
		for i, part := range parts {
			if i > 0 {
				currentPath += "/"
			}
			currentPath += part

			// Skip if this is the final mailbox (we'll create it below)
			if i == len(parts)-1 {
				break
			}

			// Check if this intermediate mailbox exists
			intermediateExists, checkErr := db.MailboxExistsPerUser(userDB, state.UserID, currentPath)
			if checkErr == nil && !intermediateExists {
				// Create intermediate mailbox - ignore errors as per RFC 3501
				_, _ = db.CreateMailboxPerUser(userDB, state.UserID, currentPath, "")
			}
		}
	}

	// Create the target mailbox
	_, err = db.CreateMailboxPerUser(userDB, state.UserID, mailboxName, "")
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			deps.SendResponse(conn, fmt.Sprintf("%s NO Mailbox already exists", tag))
		} else {
			deps.SendResponse(conn, fmt.Sprintf("%s NO Create failure: %s", tag, err.Error()))
		}
		return
	}

	deps.SendResponse(conn, fmt.Sprintf("%s OK CREATE completed", tag))
}

// ===== DELETE =====

func HandleDelete(deps ServerDeps, conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		deps.SendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	if len(parts) < 3 {
		deps.SendResponse(conn, fmt.Sprintf("%s BAD DELETE requires mailbox name", tag))
		return
	}

	// Parse mailbox name (could be quoted)
	mailboxName := strings.Trim(parts[2], "\"")

	// Validate mailbox name
	if mailboxName == "" {
		deps.SendResponse(conn, fmt.Sprintf("%s BAD Invalid mailbox name", tag))
		return
	}

	// Cannot delete INBOX (case-insensitive)
	if strings.ToUpper(mailboxName) == "INBOX" {
		deps.SendResponse(conn, fmt.Sprintf("%s NO Cannot delete INBOX", tag))
		return
	}

	// Get user database
	userDB, err := deps.GetUserDB(state.UserID)
	if err != nil {
		deps.SendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
		return
	}

	// Attempt to delete the mailbox
	err = db.DeleteMailboxPerUser(userDB, state.UserID, mailboxName)
	if err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			deps.SendResponse(conn, fmt.Sprintf("%s NO Mailbox does not exist", tag))
		} else if strings.Contains(err.Error(), "has inferior hierarchical names") {
			deps.SendResponse(conn, fmt.Sprintf("%s NO Name \"%s\" has inferior hierarchical names", tag, mailboxName))
		} else if strings.Contains(err.Error(), "cannot delete INBOX") {
			deps.SendResponse(conn, fmt.Sprintf("%s NO Cannot delete INBOX", tag))
		} else {
			deps.SendResponse(conn, fmt.Sprintf("%s NO Delete failure: %s", tag, err.Error()))
		}
		return
	}

	deps.SendResponse(conn, fmt.Sprintf("%s OK DELETE completed", tag))
}

// ===== RENAME =====

func HandleRename(deps ServerDeps, conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		deps.SendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	if len(parts) < 4 {
		deps.SendResponse(conn, fmt.Sprintf("%s BAD RENAME requires existing and new mailbox names", tag))
		return
	}

	// Parse mailbox names (could be quoted)
	oldName := strings.Trim(parts[2], "\"")
	newName := strings.Trim(parts[3], "\"")

	// Validate mailbox names
	if oldName == "" || newName == "" {
		deps.SendResponse(conn, fmt.Sprintf("%s BAD Invalid mailbox names", tag))
		return
	}

	// Get user database
	userDB, err := deps.GetUserDB(state.UserID)
	if err != nil {
		deps.SendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
		return
	}

	// Attempt to rename the mailbox
	err = db.RenameMailboxPerUser(userDB, state.UserID, oldName, newName)
	if err != nil {
		if strings.Contains(err.Error(), "source mailbox does not exist") {
			deps.SendResponse(conn, fmt.Sprintf("%s NO Source mailbox does not exist", tag))
		} else if strings.Contains(err.Error(), "destination mailbox already exists") {
			deps.SendResponse(conn, fmt.Sprintf("%s NO Destination mailbox already exists", tag))
		} else if strings.Contains(err.Error(), "cannot rename to INBOX") {
			deps.SendResponse(conn, fmt.Sprintf("%s NO Cannot rename to INBOX", tag))
		} else {
			deps.SendResponse(conn, fmt.Sprintf("%s NO Rename failure: %s", tag, err.Error()))
		}
		return
	}

	deps.SendResponse(conn, fmt.Sprintf("%s OK RENAME completed", tag))
}

// ===== SUBSCRIBE =====

func HandleSubscribe(deps ServerDeps, conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		deps.SendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	// SUBSCRIBE command format: tag SUBSCRIBE mailbox
	if len(parts) < 3 {
		deps.SendResponse(conn, fmt.Sprintf("%s BAD SUBSCRIBE command requires a mailbox argument", tag))
		return
	}

	mailboxName := parts[2]

	// Remove quotes if present
	if len(mailboxName) >= 2 && mailboxName[0] == '"' && mailboxName[len(mailboxName)-1] == '"' {
		mailboxName = mailboxName[1 : len(mailboxName)-1]
	}

	// Validate mailbox name
	if mailboxName == "" {
		deps.SendResponse(conn, fmt.Sprintf("%s BAD Invalid mailbox name", tag))
		return
	}

	// Get user database
	userDB, err := deps.GetUserDB(state.UserID)
	if err != nil {
		deps.SendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
		return
	}

	// Subscribe to the mailbox
	err = db.SubscribeToMailboxPerUser(userDB, state.UserID, mailboxName)
	if err != nil {
		fmt.Printf("Failed to subscribe to mailbox %s for user %s: %v\n", mailboxName, state.Username, err)
		deps.SendResponse(conn, fmt.Sprintf("%s NO SUBSCRIBE failure: server error", tag))
		return
	}

	deps.SendResponse(conn, fmt.Sprintf("%s OK SUBSCRIBE completed", tag))
}

// ===== UNSUBSCRIBE =====

func HandleUnsubscribe(deps ServerDeps, conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		deps.SendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	if len(parts) < 3 {
		deps.SendResponse(conn, fmt.Sprintf("%s BAD UNSUBSCRIBE requires mailbox name", tag))
		return
	}

	mailboxName := parts[2]

	// Remove quotes if present
	if len(mailboxName) >= 2 && mailboxName[0] == '"' && mailboxName[len(mailboxName)-1] == '"' {
		mailboxName = mailboxName[1 : len(mailboxName)-1]
	}

	// Validate mailbox name
	if mailboxName == "" {
		deps.SendResponse(conn, fmt.Sprintf("%s BAD Invalid mailbox name", tag))
		return
	}

	// Get user database
	userDB, err := deps.GetUserDB(state.UserID)
	if err != nil {
		deps.SendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
		return
	}

	// Unsubscribe from the mailbox
	err = db.UnsubscribeFromMailboxPerUser(userDB, state.UserID, mailboxName)
	if err != nil {
		if strings.Contains(err.Error(), "subscription does not exist") {
			deps.SendResponse(conn, fmt.Sprintf("%s NO UNSUBSCRIBE failure: can't unsubscribe that name", tag))
		} else {
			fmt.Printf("Failed to unsubscribe from mailbox %s for user %s: %v\n", mailboxName, state.Username, err)
			deps.SendResponse(conn, fmt.Sprintf("%s NO UNSUBSCRIBE failure: server error", tag))
		}
		return
	}

	deps.SendResponse(conn, fmt.Sprintf("%s OK UNSUBSCRIBE completed", tag))
}

// ===== STATUS =====

func HandleStatus(deps ServerDeps, conn net.Conn, tag string, parts []string, state *models.ClientState) {
	if !state.Authenticated {
		deps.SendResponse(conn, fmt.Sprintf("%s NO Please authenticate first", tag))
		return
	}

	if len(parts) < 4 {
		deps.SendResponse(conn, fmt.Sprintf("%s BAD STATUS requires mailbox name and status data items", tag))
		return
	}

	// Get user database
	userDB, err := deps.GetUserDB(state.UserID)
	if err != nil {
		deps.SendResponse(conn, fmt.Sprintf("%s NO Database error", tag))
		return
	}

	// Parse mailbox name (could be quoted)
	mailboxName := utils.ParseQuotedString(parts[2])

	// Validate mailbox name
	if mailboxName == "" {
		deps.SendResponse(conn, fmt.Sprintf("%s BAD Invalid mailbox name", tag))
		return
	}

	// Get mailbox ID using new schema
	mailboxID, err := db.GetMailboxByNamePerUser(userDB, state.UserID, mailboxName)
	if err != nil {
		deps.SendResponse(conn, fmt.Sprintf("%s NO STATUS failure: no status for that name", tag))
		return
	}

	// Parse status data items - they are enclosed in parentheses
	// Example: STATUS mailbox (MESSAGES RECENT)
	// Build the full items string from remaining parts
	itemsStr := strings.Join(parts[3:], " ")

	// Remove parentheses if present
	itemsStr = strings.Trim(itemsStr, "()")
	itemsStr = strings.TrimSpace(itemsStr)

	if itemsStr == "" {
		deps.SendResponse(conn, fmt.Sprintf("%s BAD STATUS requires status data items", tag))
		return
	}

	// Split items by whitespace
	requestedItems := strings.Fields(strings.ToUpper(itemsStr))

	if len(requestedItems) == 0 {
		deps.SendResponse(conn, fmt.Sprintf("%s BAD STATUS requires status data items", tag))
		return
	}

	// Initialize status values
	statusValues := make(map[string]int)

	// Query total message count using new schema
	messageCount, err := db.GetMessageCountPerUser(userDB, mailboxID)
	if err != nil {
		messageCount = 0
	}
	statusValues["MESSAGES"] = messageCount

	// Query recent/unseen count using new schema
	recentCount, err := db.GetUnseenCountPerUser(userDB, mailboxID)
	if err != nil {
		recentCount = 0
	}
	statusValues["RECENT"] = recentCount
	statusValues["UNSEEN"] = recentCount

	// Get mailbox info for UID values
	uidValidity, uidNext, err := db.GetMailboxInfoPerUser(userDB, mailboxID)
	if err == nil {
		statusValues["UIDNEXT"] = int(uidNext)
		statusValues["UIDVALIDITY"] = int(uidValidity)
	} else {
		statusValues["UIDNEXT"] = 1
		statusValues["UIDVALIDITY"] = 1
	}

	// Build response with only requested items
	var responseItems []string
	for _, item := range requestedItems {
		itemUpper := strings.ToUpper(item)
		if value, ok := statusValues[itemUpper]; ok {
			responseItems = append(responseItems, fmt.Sprintf("%s %d", itemUpper, value))
		} else {
			// Unknown status item - return BAD response
			deps.SendResponse(conn, fmt.Sprintf("%s BAD Unknown status data item: %s", tag, item))
			return
		}
	}

	// Send STATUS response
	deps.SendResponse(conn, fmt.Sprintf("* STATUS \"%s\" (%s)", mailboxName, strings.Join(responseItems, " ")))
	deps.SendResponse(conn, fmt.Sprintf("%s OK STATUS completed", tag))
}
