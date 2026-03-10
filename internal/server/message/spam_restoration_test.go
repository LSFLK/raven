package message_test

import (
	"database/sql"
	"fmt"
	"testing"

	"raven/internal/models"
	"raven/internal/server"
)

func TestSpamRestoration(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	database := server.GetDatabaseFromServer(srv)

	userID := server.CreateTestUser(t, database, "testuser")
	
	// Ensure Sent and Spam mailboxes exist
	server.CreateMailbox(t, database, "testuser", "Sent")
	server.CreateMailbox(t, database, "testuser", "Spam")
	
	sentMailboxID, _ := server.GetMailboxID(t, database, userID, "Sent")
	spamMailboxID, _ := server.GetMailboxID(t, database, userID, "Spam")

	// 1. Insert message into "Sent"
	msgID := server.InsertTestMail(t, database, "testuser", "Spam Test", "sender@test.com", "testuser@localhost", "Sent")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: sentMailboxID,
	}
	userDB := server.GetUserDBByID(t, database, state.UserID)

	// 2. Mark it as Junk
	srv.HandleStore(conn, "S001", []string{"S001", "STORE", "1", "+FLAGS", "(Junk)"}, state)

	// Verify it moved to Spam
	var currentMailboxID int64
	var prevMailboxID sql.NullInt64
	err := userDB.QueryRow("SELECT mailbox_id, previous_mailbox_id FROM message_mailbox WHERE message_id = ?", msgID).Scan(&currentMailboxID, &prevMailboxID)
	if err != nil {
		t.Fatalf("Failed to query message status: %v", err)
	}

	if currentMailboxID != spamMailboxID {
		t.Errorf("Expected message to be in Spam (%d), but it is in %d", spamMailboxID, currentMailboxID)
	}
	if !prevMailboxID.Valid || prevMailboxID.Int64 != sentMailboxID {
		t.Errorf("Expected previous_mailbox_id to be Sent (%d), but got %v", sentMailboxID, prevMailboxID)
	}

	// 3. Mark it as NonJunk from Spam
	state.SelectedMailboxID = spamMailboxID
	conn.Reset()
	srv.HandleStore(conn, "S002", []string{"S002", "STORE", "1", "+FLAGS", "(NonJunk)"}, state)

	// Verify it moved back to Sent
	err = userDB.QueryRow("SELECT mailbox_id FROM message_mailbox WHERE message_id = ?", msgID).Scan(&currentMailboxID)
	if err != nil {
		t.Fatalf("Failed to query message status after restoration: %v", err)
	}

	if currentMailboxID != sentMailboxID {
		t.Errorf("Expected message to be restored to Sent (%d), but it is in %d", sentMailboxID, currentMailboxID)
	}

	fmt.Println("Spam restoration test passed!")
}

func TestSpamDefaultRestoreToInbox(t *testing.T) {
	srv := server.SetupTestServerSimple(t)
	conn := server.NewMockConn()
	database := server.GetDatabaseFromServer(srv)

	userID := server.CreateTestUser(t, database, "testuser")
	server.CreateMailbox(t, database, "testuser", "Spam")
	
	spamMailboxID, _ := server.GetMailboxID(t, database, userID, "Spam")
	inboxID, _ := server.GetMailboxID(t, database, userID, "INBOX")

	// 1. Insert message directly into "Spam" without previous_mailbox_id
	msgID := server.InsertTestMail(t, database, "testuser", "Inbox Test", "sender@test.com", "testuser@localhost", "Spam")

	state := &models.ClientState{
		Authenticated:     true,
		UserID:            userID,
		Username:          "testuser",
		SelectedMailboxID: spamMailboxID,
	}
	userDB := server.GetUserDBByID(t, database, state.UserID)

	// 2. Mark it as NonJunk
	srv.HandleStore(conn, "S003", []string{"S003", "STORE", "1", "+FLAGS", "(NonJunk)"}, state)

	// Verify it moved to INBOX
	var currentMailboxID int64
	err := userDB.QueryRow("SELECT mailbox_id FROM message_mailbox WHERE message_id = ?", msgID).Scan(&currentMailboxID)
	if err != nil {
		t.Fatalf("Failed to query message status: %v", err)
	}

	if currentMailboxID != inboxID {
		t.Errorf("Expected message to move to INBOX (%d) by default, but it is in %d", inboxID, currentMailboxID)
	}

	fmt.Println("Spam default restoration to INBOX test passed!")
}
