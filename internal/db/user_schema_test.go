package db

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func setupTestDBPerUser(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	if _, err = db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("Failed to enable foreign keys: %v", err)
	}

	if err := createMailboxesTablePerUser(db); err != nil {
		t.Fatalf("Failed to create mailboxes table: %v", err)
	}
	if err := createAliasesTablePerUser(db); err != nil {
		t.Fatalf("Failed to create aliases table: %v", err)
	}
	if err := createSubscriptionsTablePerUser(db); err != nil {
		t.Fatalf("Failed to create subscriptions table: %v", err)
	}
	if err := createDeliveriesTablePerUser(db); err != nil {
		t.Fatalf("Failed to create deliveries table: %v", err)
	}
	if err := createMessagesTable(db); err != nil {
		t.Fatalf("Failed to create messages table: %v", err)
	}
	if err := createMessageMailboxTable(db); err != nil {
		t.Fatalf("Failed to create message_mailbox table: %v", err)
	}
	if err := createOutboundQueueTablePerUser(db); err != nil {
		t.Fatalf("Failed to create outbound_queue table: %v", err)
	}

	return db
}

func TestCreateMailboxPerUser(t *testing.T) {
	db := setupTestDBPerUser(t)
	defer func() { _ = db.Close() }()

	mailboxID, err := CreateMailboxPerUser(db, "INBOX", "\\Inbox")
	if err != nil {
		t.Fatalf("CreateMailboxPerUser failed: %v", err)
	}

	if mailboxID == 0 {
		t.Error("Expected non-zero mailbox ID")
	}

	var name, specialUse string
	var uidValidity, uidNext int64
	err = db.QueryRow("SELECT name, special_use, uid_validity, uid_next FROM mailboxes WHERE id = ?", mailboxID).
		Scan(&name, &specialUse, &uidValidity, &uidNext)
	if err != nil {
		t.Fatalf("Failed to retrieve mailbox: %v", err)
	}

	if name != "INBOX" {
		t.Errorf("Expected mailbox name INBOX, got %s", name)
	}
	if specialUse != "\\Inbox" {
		t.Errorf("Expected special use \\Inbox, got %s", specialUse)
	}
	if uidValidity == 0 {
		t.Error("Expected non-zero UID validity")
	}
	if uidNext != 1 {
		t.Errorf("Expected UID next to be 1, got %d", uidNext)
	}
}

func TestCreateMailboxPerUser_EmptyName(t *testing.T) {
	db := setupTestDBPerUser(t)
	defer func() { _ = db.Close() }()

	_, err := CreateMailboxPerUser(db, "", "")
	if err == nil {
		t.Error("Expected error when creating mailbox with empty name")
	}
	if !strings.Contains(err.Error(), "cannot be empty") {
		t.Errorf("Expected 'cannot be empty' error, got: %v", err)
	}
}

func TestCreateMailboxPerUser_Duplicate(t *testing.T) {
	db := setupTestDBPerUser(t)
	defer func() { _ = db.Close() }()

	_, err := CreateMailboxPerUser(db, "INBOX", "\\Inbox")
	if err != nil {
		t.Fatalf("First CreateMailboxPerUser failed: %v", err)
	}

	_, err = CreateMailboxPerUser(db, "INBOX", "\\Inbox")
	if err == nil {
		t.Error("Expected error when creating duplicate mailbox")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("Expected 'already exists' error, got: %v", err)
	}
}

func TestGetMailboxByNamePerUser(t *testing.T) {
	db := setupTestDBPerUser(t)
	defer func() { _ = db.Close() }()

	createdID, _ := CreateMailboxPerUser(db, "INBOX", "\\Inbox")

	retrievedID, err := GetMailboxByNamePerUser(db, "INBOX")
	if err != nil {
		t.Fatalf("GetMailboxByNamePerUser failed: %v", err)
	}

	if createdID != retrievedID {
		t.Errorf("Expected mailbox ID %d, got %d", createdID, retrievedID)
	}
}

func TestGetMailboxByNamePerUser_INBOXCaseInsensitive(t *testing.T) {
	db := setupTestDBPerUser(t)
	defer func() { _ = db.Close() }()

	createdID, _ := CreateMailboxPerUser(db, "INBOX", "\\Inbox")

	retrievedID, err := GetMailboxByNamePerUser(db, "Inbox")
	if err != nil {
		t.Fatalf("GetMailboxByNamePerUser failed for mixed-case Inbox: %v", err)
	}

	if createdID != retrievedID {
		t.Errorf("Expected mailbox ID %d, got %d", createdID, retrievedID)
	}
}

func TestGetMailboxByNamePerUser_NotFound(t *testing.T) {
	db := setupTestDBPerUser(t)
	defer func() { _ = db.Close() }()

	_, err := GetMailboxByNamePerUser(db, "NonExistent")
	if err == nil {
		t.Error("Expected error when getting non-existent mailbox")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}

func TestGetMailboxInfoPerUser(t *testing.T) {
	db := setupTestDBPerUser(t)
	defer func() { _ = db.Close() }()

	mailboxID, _ := CreateMailboxPerUser(db, "INBOX", "\\Inbox")

	uidValidity, uidNext, err := GetMailboxInfoPerUser(db, mailboxID)
	if err != nil {
		t.Fatalf("GetMailboxInfoPerUser failed: %v", err)
	}

	if uidValidity == 0 {
		t.Error("Expected non-zero UID validity")
	}
	if uidNext != 1 {
		t.Errorf("Expected UID next to be 1, got %d", uidNext)
	}
}

func TestIncrementUIDNextPerUser(t *testing.T) {
	db := setupTestDBPerUser(t)
	defer func() { _ = db.Close() }()

	mailboxID, _ := CreateMailboxPerUser(db, "INBOX", "\\Inbox")

	uid1, err := IncrementUIDNextPerUser(db, mailboxID)
	if err != nil {
		t.Fatalf("First IncrementUIDNextPerUser failed: %v", err)
	}
	if uid1 != 1 {
		t.Errorf("Expected UID 1, got %d", uid1)
	}

	uid2, err := IncrementUIDNextPerUser(db, mailboxID)
	if err != nil {
		t.Fatalf("Second IncrementUIDNextPerUser failed: %v", err)
	}
	if uid2 != 2 {
		t.Errorf("Expected UID 2, got %d", uid2)
	}
}

func TestMailboxExistsPerUser(t *testing.T) {
	db := setupTestDBPerUser(t)
	defer func() { _ = db.Close() }()

	exists, err := MailboxExistsPerUser(db, "INBOX")
	if err != nil {
		t.Fatalf("MailboxExistsPerUser failed: %v", err)
	}
	if exists {
		t.Error("Mailbox should not exist yet")
	}

	_, _ = CreateMailboxPerUser(db, "INBOX", "\\Inbox")

	exists, err = MailboxExistsPerUser(db, "INBOX")
	if err != nil {
		t.Fatalf("MailboxExistsPerUser failed after creating mailbox: %v", err)
	}
	if !exists {
		t.Error("Mailbox should exist after creation")
	}
}

func TestGetUserMailboxesPerUser(t *testing.T) {
	db := setupTestDBPerUser(t)
	defer func() { _ = db.Close() }()

	mailboxes := []string{"Drafts", "INBOX", "Sent", "Trash"}
	for _, name := range mailboxes {
		_, err := CreateMailboxPerUser(db, name, "")
		if err != nil {
			t.Fatalf("Failed to create mailbox %s: %v", name, err)
		}
	}

	retrieved, err := GetUserMailboxesPerUser(db)
	if err != nil {
		t.Fatalf("GetUserMailboxesPerUser failed: %v", err)
	}

	if len(retrieved) != len(mailboxes) {
		t.Errorf("Expected %d mailboxes, got %d", len(mailboxes), len(retrieved))
	}

	for i, name := range mailboxes {
		if retrieved[i] != name {
			t.Errorf("Expected mailbox %s at index %d, got %s", name, i, retrieved[i])
		}
	}
}

func TestDeleteMailboxPerUser(t *testing.T) {
	db := setupTestDBPerUser(t)
	defer func() { _ = db.Close() }()

	_, _ = CreateMailboxPerUser(db, "INBOX", "\\Inbox")
	_, _ = CreateMailboxPerUser(db, "TestFolder", "")

	err := DeleteMailboxPerUser(db, "TestFolder")
	if err != nil {
		t.Fatalf("DeleteMailboxPerUser failed: %v", err)
	}

	exists, _ := MailboxExistsPerUser(db, "TestFolder")
	if exists {
		t.Error("Mailbox should not exist after deletion")
	}
}

func TestDeleteMailboxPerUser_INBOX(t *testing.T) {
	db := setupTestDBPerUser(t)
	defer func() { _ = db.Close() }()

	_, _ = CreateMailboxPerUser(db, "INBOX", "\\Inbox")

	err := DeleteMailboxPerUser(db, "INBOX")
	if err == nil {
		t.Error("Expected error when deleting INBOX")
	}
	if !strings.Contains(err.Error(), "cannot delete INBOX") {
		t.Errorf("Expected 'cannot delete INBOX' error, got: %v", err)
	}
}

func TestDeleteMailboxPerUser_WithChildren(t *testing.T) {
	db := setupTestDBPerUser(t)
	defer func() { _ = db.Close() }()

	_, _ = CreateMailboxPerUser(db, "Parent", "")
	_, _ = CreateMailboxPerUser(db, "Parent/Child", "")

	err := DeleteMailboxPerUser(db, "Parent")
	if err == nil {
		t.Error("Expected error when deleting mailbox with children")
	}
	if !strings.Contains(err.Error(), "inferior hierarchical names") {
		t.Errorf("Expected 'inferior hierarchical names' error, got: %v", err)
	}
}

func TestRenameMailboxPerUser(t *testing.T) {
	db := setupTestDBPerUser(t)
	defer func() { _ = db.Close() }()

	_, _ = CreateMailboxPerUser(db, "OldName", "")

	err := RenameMailboxPerUser(db, "OldName", "NewName")
	if err != nil {
		t.Fatalf("RenameMailboxPerUser failed: %v", err)
	}

	exists, _ := MailboxExistsPerUser(db, "OldName")
	if exists {
		t.Error("Old mailbox name should not exist")
	}

	exists, _ = MailboxExistsPerUser(db, "NewName")
	if !exists {
		t.Error("New mailbox name should exist")
	}
}

func TestRenameMailboxPerUser_ToINBOX(t *testing.T) {
	db := setupTestDBPerUser(t)
	defer func() { _ = db.Close() }()

	_, _ = CreateMailboxPerUser(db, "TestFolder", "")

	err := RenameMailboxPerUser(db, "TestFolder", "INBOX")
	if err == nil {
		t.Error("Expected error when renaming to INBOX")
	}
	if !strings.Contains(err.Error(), "cannot rename to INBOX") {
		t.Errorf("Expected 'cannot rename to INBOX' error, got: %v", err)
	}
}

func TestRenameMailboxPerUser_WithChildren(t *testing.T) {
	db := setupTestDBPerUser(t)
	defer func() { _ = db.Close() }()

	_, _ = CreateMailboxPerUser(db, "Parent", "")
	_, _ = CreateMailboxPerUser(db, "Parent/Child", "")

	err := RenameMailboxPerUser(db, "Parent", "NewParent")
	if err != nil {
		t.Fatalf("RenameMailboxPerUser with children failed: %v", err)
	}

	exists, _ := MailboxExistsPerUser(db, "NewParent")
	if !exists {
		t.Error("New parent mailbox should exist")
	}

	exists, _ = MailboxExistsPerUser(db, "NewParent/Child")
	if !exists {
		t.Error("Renamed child mailbox should exist")
	}
}

func TestAddMessageToMailboxPerUser(t *testing.T) {
	db := setupTestDBPerUser(t)
	defer func() { _ = db.Close() }()

	mailboxID, _ := CreateMailboxPerUser(db, "INBOX", "\\Inbox")
	messageID, _ := CreateMessage(db, "Test", "", "", time.Now(), 100)

	err := AddMessageToMailboxPerUser(db, messageID, mailboxID, "\\Seen", time.Now())
	if err != nil {
		t.Fatalf("AddMessageToMailboxPerUser failed: %v", err)
	}

	var flags string
	var uid int64
	err = db.QueryRow("SELECT flags, uid FROM message_mailbox WHERE message_id = ? AND mailbox_id = ?", messageID, mailboxID).
		Scan(&flags, &uid)
	if err != nil {
		t.Fatalf("Failed to retrieve message_mailbox entry: %v", err)
	}

	if flags != "\\Seen" {
		t.Errorf("Expected flags \\Seen, got %s", flags)
	}
	if uid != 1 {
		t.Errorf("Expected UID 1, got %d", uid)
	}
}

func TestGetMessagesByMailboxPerUser(t *testing.T) {
	db := setupTestDBPerUser(t)
	defer func() { _ = db.Close() }()

	mailboxID, _ := CreateMailboxPerUser(db, "INBOX", "\\Inbox")

	messageIDs := make([]int64, 3)
	for i := range 3 {
		msgID, _ := CreateMessage(db, "Test", "", "", time.Now(), 100)
		messageIDs[i] = msgID
		_ = AddMessageToMailboxPerUser(db, msgID, mailboxID, "", time.Now())
	}

	retrieved, err := GetMessagesByMailboxPerUser(db, mailboxID)
	if err != nil {
		t.Fatalf("GetMessagesByMailboxPerUser failed: %v", err)
	}

	if len(retrieved) != len(messageIDs) {
		t.Errorf("Expected %d messages, got %d", len(messageIDs), len(retrieved))
	}
}

func TestGetMessageCountPerUser(t *testing.T) {
	db := setupTestDBPerUser(t)
	defer func() { _ = db.Close() }()

	mailboxID, _ := CreateMailboxPerUser(db, "INBOX", "\\Inbox")

	for range 5 {
		msgID, _ := CreateMessage(db, "Test", "", "", time.Now(), 100)
		_ = AddMessageToMailboxPerUser(db, msgID, mailboxID, "", time.Now())
	}

	count, err := GetMessageCountPerUser(db, mailboxID)
	if err != nil {
		t.Fatalf("GetMessageCountPerUser failed: %v", err)
	}

	if count != 5 {
		t.Errorf("Expected message count 5, got %d", count)
	}
}

func TestGetUnseenCountPerUser(t *testing.T) {
	db := setupTestDBPerUser(t)
	defer func() { _ = db.Close() }()

	mailboxID, _ := CreateMailboxPerUser(db, "INBOX", "\\Inbox")

	for range 3 {
		msgID, _ := CreateMessage(db, "Test", "", "", time.Now(), 100)
		_ = AddMessageToMailboxPerUser(db, msgID, mailboxID, "\\Seen", time.Now())
	}

	for range 2 {
		msgID, _ := CreateMessage(db, "Test", "", "", time.Now(), 100)
		_ = AddMessageToMailboxPerUser(db, msgID, mailboxID, "", time.Now())
	}

	count, err := GetUnseenCountPerUser(db, mailboxID)
	if err != nil {
		t.Fatalf("GetUnseenCountPerUser failed: %v", err)
	}

	if count != 2 {
		t.Errorf("Expected unseen count 2, got %d", count)
	}
}

func TestGetRecentCountPerUser(t *testing.T) {
	db := setupTestDBPerUser(t)
	defer func() { _ = db.Close() }()

	mailboxID, _ := CreateMailboxPerUser(db, "INBOX", "\\Inbox")

	for range 2 {
		msgID, _ := CreateMessage(db, "Recent", "", "", time.Now(), 100)
		_ = AddMessageToMailboxPerUser(db, msgID, mailboxID, "\\Recent", time.Now())
	}

	msgID, _ := CreateMessage(db, "Seen", "", "", time.Now(), 100)
	_ = AddMessageToMailboxPerUser(db, msgID, mailboxID, "\\Seen", time.Now())

	count, err := GetRecentCountPerUser(db, mailboxID)
	if err != nil {
		t.Fatalf("GetRecentCountPerUser failed: %v", err)
	}

	if count != 2 {
		t.Errorf("Expected recent count 2, got %d", count)
	}
}

func TestUpdateMessageFlagsPerUser(t *testing.T) {
	db := setupTestDBPerUser(t)
	defer func() { _ = db.Close() }()

	mailboxID, _ := CreateMailboxPerUser(db, "INBOX", "\\Inbox")
	messageID, _ := CreateMessage(db, "Test", "", "", time.Now(), 100)
	_ = AddMessageToMailboxPerUser(db, messageID, mailboxID, "", time.Now())

	err := UpdateMessageFlagsPerUser(db, mailboxID, messageID, "\\Seen \\Flagged")
	if err != nil {
		t.Fatalf("UpdateMessageFlagsPerUser failed: %v", err)
	}

	flags, err := GetMessageFlagsPerUser(db, mailboxID, messageID)
	if err != nil {
		t.Fatalf("GetMessageFlagsPerUser failed: %v", err)
	}

	if flags != "\\Seen \\Flagged" {
		t.Errorf("Expected flags '\\Seen \\Flagged', got %s", flags)
	}
}

func TestGetMessageFlagsPerUser(t *testing.T) {
	db := setupTestDBPerUser(t)
	defer func() { _ = db.Close() }()

	mailboxID, _ := CreateMailboxPerUser(db, "INBOX", "\\Inbox")
	messageID, _ := CreateMessage(db, "Test", "", "", time.Now(), 100)
	_ = AddMessageToMailboxPerUser(db, messageID, mailboxID, "\\Seen", time.Now())

	flags, err := GetMessageFlagsPerUser(db, mailboxID, messageID)
	if err != nil {
		t.Fatalf("GetMessageFlagsPerUser failed: %v", err)
	}

	if flags != "\\Seen" {
		t.Errorf("Expected flags \\Seen, got %s", flags)
	}
}

func TestSubscribeToMailboxPerUser(t *testing.T) {
	db := setupTestDBPerUser(t)
	defer func() { _ = db.Close() }()

	err := SubscribeToMailboxPerUser(db, "INBOX")
	if err != nil {
		t.Fatalf("SubscribeToMailboxPerUser failed: %v", err)
	}

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM subscriptions WHERE mailbox_name = ?", "INBOX").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to verify subscription: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected 1 subscription, got %d", count)
	}
}

func TestUnsubscribeFromMailboxPerUser(t *testing.T) {
	db := setupTestDBPerUser(t)
	defer func() { _ = db.Close() }()

	_ = SubscribeToMailboxPerUser(db, "INBOX")

	err := UnsubscribeFromMailboxPerUser(db, "INBOX")
	if err != nil {
		t.Fatalf("UnsubscribeFromMailboxPerUser failed: %v", err)
	}

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM subscriptions WHERE mailbox_name = ?", "INBOX").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to verify unsubscription: %v", err)
	}

	if count != 0 {
		t.Errorf("Expected 0 subscriptions after unsubscribe, got %d", count)
	}
}

func TestUnsubscribeFromMailboxPerUser_NonExistent(t *testing.T) {
	db := setupTestDBPerUser(t)
	defer func() { _ = db.Close() }()

	err := UnsubscribeFromMailboxPerUser(db, "INBOX")
	if err == nil {
		t.Error("Expected error when unsubscribing from non-existent subscription")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("Expected 'does not exist' error, got: %v", err)
	}
}

func TestGetUserSubscriptionsPerUser(t *testing.T) {
	db := setupTestDBPerUser(t)
	defer func() { _ = db.Close() }()

	mailboxes := []string{"Drafts", "INBOX", "Sent"}
	for _, name := range mailboxes {
		_ = SubscribeToMailboxPerUser(db, name)
	}

	subscriptions, err := GetUserSubscriptionsPerUser(db)
	if err != nil {
		t.Fatalf("GetUserSubscriptionsPerUser failed: %v", err)
	}

	if len(subscriptions) != len(mailboxes) {
		t.Errorf("Expected %d subscriptions, got %d", len(mailboxes), len(subscriptions))
	}
}

func TestIsMailboxSubscribedPerUser(t *testing.T) {
	db := setupTestDBPerUser(t)
	defer func() { _ = db.Close() }()

	subscribed, err := IsMailboxSubscribedPerUser(db, "INBOX")
	if err != nil {
		t.Fatalf("IsMailboxSubscribedPerUser failed: %v", err)
	}
	if subscribed {
		t.Error("Mailbox should not be subscribed yet")
	}

	_ = SubscribeToMailboxPerUser(db, "INBOX")

	subscribed, err = IsMailboxSubscribedPerUser(db, "INBOX")
	if err != nil {
		t.Fatalf("IsMailboxSubscribedPerUser failed after subscribing: %v", err)
	}
	if !subscribed {
		t.Error("Mailbox should be subscribed")
	}
}

func TestRecordDeliveryPerUser(t *testing.T) {
	db := setupTestDBPerUser(t)
	defer func() { _ = db.Close() }()

	messageID, _ := CreateMessage(db, "Test", "", "", time.Now(), 100)

	err := RecordDeliveryPerUser(db, messageID, "recipient@example.com", "sender@example.com", "delivered", "250 OK")
	if err != nil {
		t.Fatalf("RecordDeliveryPerUser failed: %v", err)
	}

	var recipient, sender, status string
	err = db.QueryRow("SELECT recipient, sender, status FROM deliveries WHERE message_id = ?", messageID).
		Scan(&recipient, &sender, &status)
	if err != nil {
		t.Fatalf("Failed to retrieve delivery: %v", err)
	}

	if recipient != "recipient@example.com" {
		t.Errorf("Expected recipient 'recipient@example.com', got %s", recipient)
	}
	if sender != "sender@example.com" {
		t.Errorf("Expected sender 'sender@example.com', got %s", sender)
	}
	if status != "delivered" {
		t.Errorf("Expected status 'delivered', got %s", status)
	}
}

func TestQueueOutboundMessagePerUser(t *testing.T) {
	db := setupTestDBPerUser(t)
	defer func() { _ = db.Close() }()

	messageID, _ := CreateMessage(db, "Test", "", "", time.Now(), 100)

	err := QueueOutboundMessagePerUser(db, messageID, "sender@example.com", "recipient@example.com", 5)
	if err != nil {
		t.Fatalf("QueueOutboundMessagePerUser failed: %v", err)
	}

	var sender, recipient, status string
	err = db.QueryRow("SELECT sender, recipient, status FROM outbound_queue WHERE message_id = ?", messageID).
		Scan(&sender, &recipient, &status)
	if err != nil {
		t.Fatalf("Failed to retrieve outbound message: %v", err)
	}

	if sender != "sender@example.com" {
		t.Errorf("Expected sender 'sender@example.com', got %s", sender)
	}
	if recipient != "recipient@example.com" {
		t.Errorf("Expected recipient 'recipient@example.com', got %s", recipient)
	}
	if status != "pending" {
		t.Errorf("Expected status 'pending', got %s", status)
	}
}

func TestGetPendingOutboundMessagesPerUser(t *testing.T) {
	db := setupTestDBPerUser(t)
	defer func() { _ = db.Close() }()

	messageIDs := make([]int64, 3)
	for i := range 3 {
		msgID, _ := CreateMessage(db, "Test", "", "", time.Now(), 100)
		messageIDs[i] = msgID
		_ = QueueOutboundMessagePerUser(db, msgID, "sender@example.com", "recipient@example.com", 5)
	}

	messages, err := GetPendingOutboundMessagesPerUser(db, 10)
	if err != nil {
		t.Fatalf("GetPendingOutboundMessagesPerUser failed: %v", err)
	}

	if len(messages) != len(messageIDs) {
		t.Errorf("Expected %d pending messages, got %d", len(messageIDs), len(messages))
	}
}

func TestUpdateOutboundStatusPerUser(t *testing.T) {
	db := setupTestDBPerUser(t)
	defer func() { _ = db.Close() }()

	messageID, _ := CreateMessage(db, "Test", "", "", time.Now(), 100)
	_ = QueueOutboundMessagePerUser(db, messageID, "sender@example.com", "recipient@example.com", 5)

	var queueID int64
	err := db.QueryRow("SELECT id FROM outbound_queue WHERE message_id = ?", messageID).Scan(&queueID)
	if err != nil {
		t.Fatalf("Failed to get queue ID: %v", err)
	}

	err = UpdateOutboundStatusPerUser(db, queueID, "sent", "")
	if err != nil {
		t.Fatalf("UpdateOutboundStatusPerUser failed: %v", err)
	}

	var status string
	err = db.QueryRow("SELECT status FROM outbound_queue WHERE id = ?", queueID).Scan(&status)
	if err != nil {
		t.Fatalf("Failed to retrieve status: %v", err)
	}

	if status != "sent" {
		t.Errorf("Expected status 'sent', got %s", status)
	}
}

func TestRetryOutboundMessagePerUser(t *testing.T) {
	db := setupTestDBPerUser(t)
	defer func() { _ = db.Close() }()

	messageID, _ := CreateMessage(db, "Test", "", "", time.Now(), 100)
	_ = QueueOutboundMessagePerUser(db, messageID, "sender@example.com", "recipient@example.com", 5)

	var queueID int64
	err := db.QueryRow("SELECT id FROM outbound_queue WHERE message_id = ?", messageID).Scan(&queueID)
	if err != nil {
		t.Fatalf("Failed to get queue ID: %v", err)
	}

	delay := 5 * time.Minute
	err = RetryOutboundMessagePerUser(db, queueID, delay)
	if err != nil {
		t.Fatalf("RetryOutboundMessagePerUser failed: %v", err)
	}

	var retryCount int
	err = db.QueryRow("SELECT retry_count FROM outbound_queue WHERE id = ?", queueID).Scan(&retryCount)
	if err != nil {
		t.Fatalf("Failed to retrieve retry count: %v", err)
	}

	if retryCount != 1 {
		t.Errorf("Expected retry count 1, got %d", retryCount)
	}
}
