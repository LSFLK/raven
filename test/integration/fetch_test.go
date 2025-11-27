package integration_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"raven/internal/db"
	"raven/internal/server"
)

func TestFetchCommand_Success(t *testing.T) {
	// 1. Setup
	ss, cleanup := server.SetupTestServer(t)
	defer cleanup()

	conn := server.NewMockTLSConn()
	username := "testuser@example.com"
	state := server.SetupAuthenticatedState(t, ss, username)

	// 2. Create a mailbox and a message
	dbManager := ss.GetDBManager().(*db.DBManager)
	userDB, err := dbManager.GetUserDB(state.UserID)
	if err != nil {
		t.Fatalf("Failed to get user database: %v", err)
	}
	mailboxName := "INBOX"
	_, err = db.CreateMailboxPerUser(userDB, state.UserID, mailboxName, "")
	if err != nil {
		t.Fatalf("Failed to create mailbox: %v", err)
	}

	subject := "Test Subject"
	body := "This is the body of the test message."
	fullMessage := fmt.Sprintf("Subject: %s\r\n\r\n%s", subject, body)
	messageID, err := db.CreateMessage(userDB, subject, "", "", time.Now(), int64(len(fullMessage)))
	if err != nil {
		t.Fatalf("Failed to create message: %v", err)
	}

	mailboxID, err := db.GetMailboxByNamePerUser(userDB, state.UserID, mailboxName)
	if err != nil {
		t.Fatalf("Failed to get mailbox id: %v", err)
	}

	err = db.AddMessageToMailboxPerUser(userDB, messageID, mailboxID, `\Seen`, time.Now())
	if err != nil {
		t.Fatalf("Failed to add message to mailbox: %v", err)
	}

	// 3. Select the mailbox
	state.SelectedFolder = mailboxName

	// 4. Run FETCH command
	ss.HandleFetch(conn, "A001", []string{"A001", "FETCH", "1", "(BODY[])"}, state)

	// 5. Assert
	response := conn.GetWrittenData()
	t.Logf("Response from server: %s", response)

	if !strings.Contains(response, "* 1 FETCH (BODY[]") {
		t.Errorf("Expected FETCH response for message 1, but did not find it")
	}

	if !strings.Contains(response, subject) {
		t.Errorf("Expected response to contain subject '%s', but it did not", subject)
	}

	if !strings.Contains(response, "A001 OK FETCH completed") {
		t.Errorf("Expected FETCH OK response, but did not find it")
	}
}
