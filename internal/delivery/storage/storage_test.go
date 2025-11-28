package storage

import (
	"os"
	"strings"
	"testing"
	"time"

	"raven/internal/db"
	"raven/internal/delivery/parser"
)

// helper to create a temp DBManager
func setupTestDBManager(t *testing.T) *db.DBManager {
    t.Helper()
    dir, err := os.MkdirTemp("", "storage_test_*")
    if err != nil {
        t.Fatalf("failed to create temp dir: %v", err)
    }
    t.Cleanup(func() { _ = os.RemoveAll(dir) })
    manager, err := db.NewDBManager(dir)
    if err != nil {
        t.Fatalf("failed to create db manager: %v", err)
    }
    t.Cleanup(func() { _ = manager.Close() })
    return manager
}

func sampleRawMessage(from string, to []string, subject string, body string) string {
    return "From: " + from + "\r\n" +
        "To: " + to[0] + "\r\n" +
        "Subject: " + subject + "\r\n" +
        "Date: " + time.Now().Format(time.RFC1123Z) + "\r\n" +
        "Message-ID: <testmsg@raven>\r\n" +
        "Content-Type: text/plain; charset=us-ascii\r\n" +
        "\r\n" + body + "\r\n"
}

func buildParserMessage(from string, to []string, subject string, body string) *parser.Message {
    raw := sampleRawMessage(from, to, subject, body)
    return &parser.Message{
        From:       from,
        To:         to,
        Subject:    subject,
        Date:       time.Now(),
        MessageID:  "<testmsg@raven>",
        Headers:    map[string]string{"From": from, "To": to[0], "Subject": subject},
        Body:       body,
        RawMessage: raw,
        Size:       int64(len(raw)),
    }
}

func TestDeliverMessage_NewUserAndMailbox(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)

    msg := buildParserMessage("sender@example.com", []string{"user1@example.com"}, "Test", "Hello")
    if err := stor.DeliverMessage("user1@example.com", msg, "INBOX"); err != nil {
        t.Fatalf("DeliverMessage failed: %v", err)
    }

    // Message count for user should be 1
    count, err := stor.GetMessageCount("user1")
    if err != nil {
        t.Fatalf("GetMessageCount failed: %v", err)
    }
    if count != 1 {
        t.Errorf("expected 1 message, got %d", count)
    }

    folderCount, err := stor.GetMessageCountInFolder("user1", "INBOX")
    if err != nil {
        t.Fatalf("GetMessageCountInFolder failed: %v", err)
    }
    if folderCount != 1 {
        t.Errorf("expected 1 message in INBOX, got %d", folderCount)
    }
}

func TestDeliverToMultipleRecipients(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)
    msg := buildParserMessage("sender@example.com", []string{"usera@example.com"}, "Multi", "Hi")

    recipients := []string{"usera@example.com", "invalid-no-at", "userb@example.com"}
    results := stor.DeliverToMultipleRecipients(recipients, msg, "INBOX")

    if results["usera@example.com"] != nil {
        t.Errorf("expected usera delivery ok: %v", results["usera@example.com"])
    }
    if results["userb@example.com"] != nil {
        t.Errorf("expected userb delivery ok: %v", results["userb@example.com"])
    }
    if results["invalid-no-at"] == nil {
        t.Errorf("expected invalid recipient to have error")
    }

    // verify counts for users
    countA, _ := stor.GetMessageCount("usera")
    countB, _ := stor.GetMessageCount("userb")
    if countA != 1 || countB != 1 {
        t.Errorf("expected one message for each valid user, got %d and %d", countA, countB)
    }
}

func TestCheckUserAndRecipientExists(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)

    exists, err := stor.CheckUserExists("nouser")
    if err != nil || exists {
        t.Errorf("unexpected user existence: %v %v", exists, err)
    }

    // Deliver to create user
    msg := buildParserMessage("sender@example.com", []string{"nouser@example.com"}, "X", "Body")
    if err := stor.DeliverMessage("nouser@example.com", msg, "INBOX"); err != nil {
        t.Fatalf("deliver failed: %v", err)
    }

    exists, err = stor.CheckUserExists("nouser")
    if err != nil || !exists {
        t.Errorf("expected user to exist after delivery")
    }

    recExists, err := stor.CheckRecipientExists("nouser@example.com")
    if err != nil || !recExists {
        t.Errorf("expected recipient to exist: %v %v", recExists, err)
    }
}

func TestQuotaFunctions(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)

    msg := buildParserMessage("sender@example.com", []string{"quotauser@example.com"}, "Quota", strings.Repeat("A", 50))
    if err := stor.DeliverMessage("quotauser@example.com", msg, "INBOX"); err != nil {
        t.Fatalf("deliver failed: %v", err)
    }

    usage, err := stor.GetUserQuota("quotauser")
    if err != nil {
        t.Fatalf("GetUserQuota failed: %v", err)
    }
    if usage <= 0 {
        t.Errorf("expected usage > 0, got %d", usage)
    }

    // CheckQuota should succeed for large limit
    if err := stor.CheckQuota("quotauser", 10, usage+1000); err != nil {
        t.Errorf("unexpected quota failure: %v", err)
    }

    // Exceed quota
    if err := stor.CheckQuota("quotauser", 10, usage-1); err == nil {
        t.Errorf("expected quota exceeded error")
    }
}

func TestGetMessageCountInFolder_MissingFolder(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)

    // No messages delivered yet
    count, err := stor.GetMessageCountInFolder("nobody", "INBOX")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if count != 0 {
        t.Errorf("expected 0 count for missing user/folder, got %d", count)
    }
}

func TestCreateUserIfNotExists_WithDomainInName(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)

    if err := stor.CreateUserIfNotExists("domainuser@example.net"); err != nil {
        t.Fatalf("CreateUserIfNotExists failed: %v", err)
    }

    exists, err := stor.CheckRecipientExists("domainuser@example.net")
    if err != nil || !exists {
        t.Errorf("expected created user to exist: %v %v", exists, err)
    }
}

func TestDeliverMessage_InvalidRecipient(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)
    msg := buildParserMessage("sender@example.com", []string{"bad"}, "Bad", "Body")
    // Should fail because recipient lacks domain
    if err := stor.DeliverMessage("bad", msg, "INBOX"); err == nil {
        t.Errorf("expected error for invalid recipient")
    }
}

func TestDeliverMessage_CustomFolder(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)

    msg := buildParserMessage("sender@example.com", []string{"custom@example.com"}, "Custom Folder Test", "Body")
    if err := stor.DeliverMessage("custom@example.com", msg, "CustomFolder"); err != nil {
        t.Fatalf("DeliverMessage to custom folder failed: %v", err)
    }

    count, err := stor.GetMessageCountInFolder("custom", "CustomFolder")
    if err != nil {
        t.Fatalf("GetMessageCountInFolder failed: %v", err)
    }
    if count != 1 {
        t.Errorf("expected 1 message in CustomFolder, got %d", count)
    }
}

func TestDeliverMessage_MultipleDeliveries(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)

    // Deliver multiple messages to same user
    for i := 0; i < 5; i++ {
        msg := buildParserMessage("sender@example.com", []string{"multi@example.com"}, 
            "Message "+string(rune('0'+i)), "Body "+string(rune('0'+i)))
        if err := stor.DeliverMessage("multi@example.com", msg, "INBOX"); err != nil {
            t.Fatalf("DeliverMessage %d failed: %v", i, err)
        }
    }

    count, err := stor.GetMessageCount("multi")
    if err != nil {
        t.Fatalf("GetMessageCount failed: %v", err)
    }
    if count != 5 {
        t.Errorf("expected 5 messages, got %d", count)
    }
}

func TestCheckRecipientExists_InvalidEmail(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)

    exists, err := stor.CheckRecipientExists("invalid-email")
    if err == nil {
        t.Error("expected error for invalid email format")
    }
    if exists {
        t.Error("expected false for invalid email")
    }
}

func TestGetUserQuota_NonExistentUser(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)

    usage, err := stor.GetUserQuota("nonexistent")
    if err != nil {
        t.Fatalf("GetUserQuota should not error for non-existent user: %v", err)
    }
    if usage != 0 {
        t.Errorf("expected 0 usage for non-existent user, got %d", usage)
    }
}

func TestGetMessageCount_NonExistentUser(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)

    count, err := stor.GetMessageCount("nonexistent")
    if err != nil {
        t.Fatalf("GetMessageCount should not error for non-existent user: %v", err)
    }
    if count != 0 {
        t.Errorf("expected 0 count for non-existent user, got %d", count)
    }
}

func TestCreateUserIfNotExists_WithoutDomain(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)

    // User without @ defaults to localhost domain
    if err := stor.CreateUserIfNotExists("localuser"); err != nil {
        t.Fatalf("CreateUserIfNotExists failed: %v", err)
    }

    exists, err := stor.CheckUserExists("localuser")
    if err != nil || !exists {
        t.Errorf("expected created user to exist: %v %v", exists, err)
    }
}

func TestDeliverMessage_ToExistingFolder(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)

    // First delivery creates the folder
    msg1 := buildParserMessage("sender@example.com", []string{"foldertest@example.com"}, "First", "Body1")
    if err := stor.DeliverMessage("foldertest@example.com", msg1, "TestFolder"); err != nil {
        t.Fatalf("First delivery failed: %v", err)
    }

    // Second delivery to same folder
    msg2 := buildParserMessage("sender@example.com", []string{"foldertest@example.com"}, "Second", "Body2")
    if err := stor.DeliverMessage("foldertest@example.com", msg2, "TestFolder"); err != nil {
        t.Fatalf("Second delivery failed: %v", err)
    }

    count, err := stor.GetMessageCountInFolder("foldertest", "TestFolder")
    if err != nil {
        t.Fatalf("GetMessageCountInFolder failed: %v", err)
    }
    if count != 2 {
        t.Errorf("expected 2 messages in TestFolder, got %d", count)
    }
}

func TestDeliverToMultipleRecipients_AllValid(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)
    msg := buildParserMessage("sender@example.com", []string{"user1@example.com"}, "All Valid", "Body")

    recipients := []string{"user1@example.com", "user2@example.com", "user3@example.com"}
    results := stor.DeliverToMultipleRecipients(recipients, msg, "INBOX")

    for _, recipient := range recipients {
        if results[recipient] != nil {
            t.Errorf("expected delivery to %s to succeed: %v", recipient, results[recipient])
        }
    }

    // Verify all users got the message
    for i, recipient := range recipients {
        username := strings.Split(recipient, "@")[0]
        count, _ := stor.GetMessageCount(username)
        if count != 1 {
            t.Errorf("user%d should have 1 message, got %d", i+1, count)
        }
    }
}

func TestCheckQuota_ExactLimit(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)

    msg := buildParserMessage("sender@example.com", []string{"quotatest@example.com"}, "Quota", "Body")
    if err := stor.DeliverMessage("quotatest@example.com", msg, "INBOX"); err != nil {
        t.Fatalf("deliver failed: %v", err)
    }

    usage, _ := stor.GetUserQuota("quotatest")

    // Exactly at limit should succeed
    if err := stor.CheckQuota("quotatest", 0, usage); err != nil {
        t.Errorf("quota check at exact limit should succeed: %v", err)
    }
}

func TestDeliverMessage_LargeBody(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)

    // Create a large message body (>1024 bytes to test blob storage)
    largeBody := strings.Repeat("This is a large message body. ", 50)
    msg := buildParserMessage("sender@example.com", []string{"largeuser@example.com"}, "Large", largeBody)
    
    if err := stor.DeliverMessage("largeuser@example.com", msg, "INBOX"); err != nil {
        t.Fatalf("deliver large message failed: %v", err)
    }

    count, _ := stor.GetMessageCount("largeuser")
    if count != 1 {
        t.Errorf("expected 1 message, got %d", count)
    }

    usage, _ := stor.GetUserQuota("largeuser")
    if usage <= 0 {
        t.Errorf("expected usage > 0 for large message")
    }
}

func TestDeliverMessage_MultipleFolders(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)

    // Deliver to different folders for same user
    folders := []string{"INBOX", "Sent", "Drafts", "Archive"}
    for _, folder := range folders {
        msg := buildParserMessage("sender@example.com", []string{"multifolderuser@example.com"}, 
            "Folder: "+folder, "Body")
        if err := stor.DeliverMessage("multifolderuser@example.com", msg, folder); err != nil {
            t.Fatalf("deliver to %s failed: %v", folder, err)
        }
    }

    totalCount, _ := stor.GetMessageCount("multifolderuser")
    if totalCount != 4 {
        t.Errorf("expected 4 total messages, got %d", totalCount)
    }

    for _, folder := range folders {
        count, _ := stor.GetMessageCountInFolder("multifolderuser", folder)
        if count != 1 {
            t.Errorf("expected 1 message in %s, got %d", folder, count)
        }
    }
}

func TestCheckRecipientExists_AfterCreation(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)

    // Create user first
    if err := stor.CreateUserIfNotExists("checkuser@example.com"); err != nil {
        t.Fatalf("CreateUserIfNotExists failed: %v", err)
    }

    // Check recipient should now find the user
    exists, err := stor.CheckRecipientExists("checkuser@example.com")
    if err != nil {
        t.Errorf("CheckRecipientExists failed: %v", err)
    }
    if !exists {
        t.Error("expected recipient to exist after creation")
    }
}

func TestGetMessageCountInFolder_AfterDelivery(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)

    // Initial count should be 0
    count1, _ := stor.GetMessageCountInFolder("countuser", "INBOX")
    if count1 != 0 {
        t.Errorf("expected 0 initial messages, got %d", count1)
    }

    // Deliver message
    msg := buildParserMessage("sender@example.com", []string{"countuser@example.com"}, "Count Test", "Body")
    if err := stor.DeliverMessage("countuser@example.com", msg, "INBOX"); err != nil {
        t.Fatalf("deliver failed: %v", err)
    }

    // Count should now be 1
    count2, _ := stor.GetMessageCountInFolder("countuser", "INBOX")
    if count2 != 1 {
        t.Errorf("expected 1 message after delivery, got %d", count2)
    }
}

func TestDeliverMessage_MultipartMessage(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)

    // Create a multipart message
    boundary := "boundary123"
    rawMsg := "From: sender@example.com\r\n" +
        "To: multipart@example.com\r\n" +
        "Subject: Multipart Test\r\n" +
        "Date: " + time.Now().Format(time.RFC1123Z) + "\r\n" +
        "Content-Type: multipart/mixed; boundary=\"" + boundary + "\"\r\n" +
        "\r\n" +
        "--" + boundary + "\r\n" +
        "Content-Type: text/plain; charset=utf-8\r\n" +
        "\r\n" +
        "Plain text part\r\n" +
        "--" + boundary + "\r\n" +
        "Content-Type: text/html; charset=utf-8\r\n" +
        "\r\n" +
        "<html><body>HTML part</body></html>\r\n" +
        "--" + boundary + "--\r\n"

    msg := &parser.Message{
        From:       "sender@example.com",
        To:         []string{"multipart@example.com"},
        Subject:    "Multipart Test",
        Date:       time.Now(),
        MessageID:  "<multipart@raven>",
        Headers:    map[string]string{"From": "sender@example.com", "To": "multipart@example.com", "Subject": "Multipart Test"},
        Body:       "Plain text part",
        RawMessage: rawMsg,
        Size:       int64(len(rawMsg)),
    }

    if err := stor.DeliverMessage("multipart@example.com", msg, "INBOX"); err != nil {
        t.Fatalf("deliver multipart message failed: %v", err)
    }

    count, _ := stor.GetMessageCount("multipart")
    if count != 1 {
        t.Errorf("expected 1 message, got %d", count)
    }
}

func TestDeliverToMultipleRecipients_PartialFailure(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)
    msg := buildParserMessage("sender@example.com", []string{"partial@example.com"}, "Partial", "Body")

    recipients := []string{"good1@example.com", "good2@example.com", "also-bad"}
    results := stor.DeliverToMultipleRecipients(recipients, msg, "INBOX")

    // Check successful deliveries
    if results["good1@example.com"] != nil {
        t.Errorf("expected good1 delivery to succeed: %v", results["good1@example.com"])
    }
    if results["good2@example.com"] != nil {
        t.Errorf("expected good2 delivery to succeed: %v", results["good2@example.com"])
    }

    // Check failed delivery (no @ sign)
    if results["also-bad"] == nil {
        t.Error("expected also-bad delivery to fail")
    }
}

func TestDeliverMessage_EmptyBody(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)

    msg := buildParserMessage("sender@example.com", []string{"empty@example.com"}, "Empty Body", "")
    if err := stor.DeliverMessage("empty@example.com", msg, "INBOX"); err != nil {
        t.Fatalf("deliver empty body message failed: %v", err)
    }

    count, _ := stor.GetMessageCount("empty")
    if count != 1 {
        t.Errorf("expected 1 message, got %d", count)
    }
}

func TestDeliverMessage_SpecialCharactersInSubject(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)

    specialSubject := "Test: Special <Chars> & \"Quotes\" [Brackets]"
    msg := buildParserMessage("sender@example.com", []string{"special@example.com"}, specialSubject, "Body")
    if err := stor.DeliverMessage("special@example.com", msg, "INBOX"); err != nil {
        t.Fatalf("deliver message with special chars failed: %v", err)
    }

    count, _ := stor.GetMessageCount("special")
    if count != 1 {
        t.Errorf("expected 1 message, got %d", count)
    }
}

func TestCreateUserIfNotExists_MultipleCalls(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)

    // First call creates user
    if err := stor.CreateUserIfNotExists("idempotent@example.com"); err != nil {
        t.Fatalf("first CreateUserIfNotExists failed: %v", err)
    }

    // Second call should succeed (idempotent)
    if err := stor.CreateUserIfNotExists("idempotent@example.com"); err != nil {
        t.Fatalf("second CreateUserIfNotExists failed: %v", err)
    }

    exists, _ := stor.CheckUserExists("idempotent")
    if !exists {
        t.Error("expected user to exist")
    }
}

func TestDeliverMessage_WithDifferentDomains(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)

    // Deliver to different domains
    domains := []string{"domain1.com", "domain2.com", "domain3.org"}
    for _, domain := range domains {
        recipient := "user@" + domain
        msg := buildParserMessage("sender@example.com", []string{recipient}, "Domain Test", "Body")
        if err := stor.DeliverMessage(recipient, msg, "INBOX"); err != nil {
            t.Fatalf("deliver to %s failed: %v", domain, err)
        }
    }

    // Verify all deliveries
    for _, domain := range domains {
        exists, _ := stor.CheckRecipientExists("user@" + domain)
        if !exists {
            t.Errorf("expected user@%s to exist", domain)
        }
    }
}

func TestGetUserQuota_AfterMultipleDeliveries(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)

    recipient := "quotamulti@example.com"
    initialUsage, _ := stor.GetUserQuota("quotamulti")

    // Deliver multiple messages
    for i := 0; i < 3; i++ {
        body := strings.Repeat("X", 100+i*10)
        msg := buildParserMessage("sender@example.com", []string{recipient}, "Quota Test", body)
        if err := stor.DeliverMessage(recipient, msg, "INBOX"); err != nil {
            t.Fatalf("delivery %d failed: %v", i, err)
        }
    }

    finalUsage, _ := stor.GetUserQuota("quotamulti")
    if finalUsage <= initialUsage {
        t.Errorf("expected quota to increase after deliveries: initial=%d, final=%d", initialUsage, finalUsage)
    }
}

func TestGetMessageCountInFolder_NonExistentFolder(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)

    // Create user but not the folder
    msg := buildParserMessage("sender@example.com", []string{"folderuser@example.com"}, "Test", "Body")
    if err := stor.DeliverMessage("folderuser@example.com", msg, "INBOX"); err != nil {
        t.Fatalf("deliver failed: %v", err)
    }

    // Check non-existent folder
    count, err := stor.GetMessageCountInFolder("folderuser", "NonExistent")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if count != 0 {
        t.Errorf("expected 0 count for non-existent folder, got %d", count)
    }
}

func TestCheckUserExists_WithDifferentCases(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)

    // Create user
    msg := buildParserMessage("sender@example.com", []string{"testuser@example.com"}, "Test", "Body")
    if err := stor.DeliverMessage("testuser@example.com", msg, "INBOX"); err != nil {
        t.Fatalf("deliver failed: %v", err)
    }

    // Check with exact case
    exists, _ := stor.CheckUserExists("testuser")
    if !exists {
        t.Error("expected testuser to exist")
    }
}

func TestDeliverMessage_WithAttachment(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)

    // Create message with attachment-like content
    boundary := "attach123"
    rawMsg := "From: sender@example.com\r\n" +
        "To: attach@example.com\r\n" +
        "Subject: With Attachment\r\n" +
        "Date: " + time.Now().Format(time.RFC1123Z) + "\r\n" +
        "Content-Type: multipart/mixed; boundary=\"" + boundary + "\"\r\n" +
        "\r\n" +
        "--" + boundary + "\r\n" +
        "Content-Type: text/plain\r\n" +
        "\r\n" +
        "Message body\r\n" +
        "--" + boundary + "\r\n" +
        "Content-Type: application/octet-stream\r\n" +
        "Content-Disposition: attachment; filename=\"file.bin\"\r\n" +
        "\r\n" +
        "Binary data here\r\n" +
        "--" + boundary + "--\r\n"

    msg := &parser.Message{
        From:       "sender@example.com",
        To:         []string{"attach@example.com"},
        Subject:    "With Attachment",
        Date:       time.Now(),
        MessageID:  "<attach@raven>",
        Headers:    map[string]string{},
        Body:       "Message body",
        RawMessage: rawMsg,
        Size:       int64(len(rawMsg)),
    }

    if err := stor.DeliverMessage("attach@example.com", msg, "INBOX"); err != nil {
        t.Fatalf("deliver with attachment failed: %v", err)
    }

    count, _ := stor.GetMessageCount("attach")
    if count != 1 {
        t.Errorf("expected 1 message, got %d", count)
    }
}

// Spam filtering tests

func TestSpamFiltering_RspamdActionReject(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)

    msg := buildParserMessage("sender@example.com", []string{"spamuser@example.com"}, "Spam Test", "Spam body")
    msg.Headers["X-Rspamd-Action"] = "reject"

    if err := stor.DeliverMessage("spamuser@example.com", msg, "INBOX"); err != nil {
        t.Fatalf("deliver failed: %v", err)
    }

    // Should be in Spam folder, not INBOX
    spamCount, _ := stor.GetMessageCountInFolder("spamuser", "Spam")
    if spamCount != 1 {
        t.Errorf("expected 1 message in Spam folder, got %d", spamCount)
    }

    inboxCount, _ := stor.GetMessageCountInFolder("spamuser", "INBOX")
    if inboxCount != 0 {
        t.Errorf("expected 0 messages in INBOX, got %d", inboxCount)
    }
}

func TestSpamFiltering_RspamdActionAddHeader(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)

    msg := buildParserMessage("sender@example.com", []string{"spamuser2@example.com"}, "Spam Test", "Spam body")
    msg.Headers["X-Rspamd-Action"] = "add header"

    if err := stor.DeliverMessage("spamuser2@example.com", msg, "INBOX"); err != nil {
        t.Fatalf("deliver failed: %v", err)
    }

    spamCount, _ := stor.GetMessageCountInFolder("spamuser2", "Spam")
    if spamCount != 1 {
        t.Errorf("expected 1 message in Spam folder, got %d", spamCount)
    }
}

func TestSpamFiltering_RspamdActionRewriteSubject(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)

    msg := buildParserMessage("sender@example.com", []string{"spamuser3@example.com"}, "Spam Test", "Spam body")
    msg.Headers["X-Rspamd-Action"] = "rewrite subject"

    if err := stor.DeliverMessage("spamuser3@example.com", msg, "INBOX"); err != nil {
        t.Fatalf("deliver failed: %v", err)
    }

    spamCount, _ := stor.GetMessageCountInFolder("spamuser3", "Spam")
    if spamCount != 1 {
        t.Errorf("expected 1 message in Spam folder, got %d", spamCount)
    }
}

func TestSpamFiltering_XSpamStatusYes(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)

    msg := buildParserMessage("sender@example.com", []string{"spamuser4@example.com"}, "Spam Test", "Spam body")
    msg.Headers["X-Spam-Status"] = "Yes, score=10.5"

    if err := stor.DeliverMessage("spamuser4@example.com", msg, "INBOX"); err != nil {
        t.Fatalf("deliver failed: %v", err)
    }

    spamCount, _ := stor.GetMessageCountInFolder("spamuser4", "Spam")
    if spamCount != 1 {
        t.Errorf("expected 1 message in Spam folder, got %d", spamCount)
    }
}

func TestSpamFiltering_NoActionGoesToInbox(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)

    msg := buildParserMessage("sender@example.com", []string{"hamuser@example.com"}, "Ham Test", "Ham body")
    msg.Headers["X-Rspamd-Action"] = "no action"
    msg.Headers["X-Spam-Status"] = "No, score=-1.10"

    if err := stor.DeliverMessage("hamuser@example.com", msg, "INBOX"); err != nil {
        t.Fatalf("deliver failed: %v", err)
    }

    inboxCount, _ := stor.GetMessageCountInFolder("hamuser", "INBOX")
    if inboxCount != 1 {
        t.Errorf("expected 1 message in INBOX, got %d", inboxCount)
    }

    spamCount, _ := stor.GetMessageCountInFolder("hamuser", "Spam")
    if spamCount != 0 {
        t.Errorf("expected 0 messages in Spam folder, got %d", spamCount)
    }
}

func TestSpamFiltering_NoHeadersGoesToInbox(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)

    msg := buildParserMessage("sender@example.com", []string{"cleanuser@example.com"}, "Clean Test", "Clean body")
    // No Rspamd headers at all

    if err := stor.DeliverMessage("cleanuser@example.com", msg, "INBOX"); err != nil {
        t.Fatalf("deliver failed: %v", err)
    }

    inboxCount, _ := stor.GetMessageCountInFolder("cleanuser", "INBOX")
    if inboxCount != 1 {
        t.Errorf("expected 1 message in INBOX, got %d", inboxCount)
    }

    spamCount, _ := stor.GetMessageCountInFolder("cleanuser", "Spam")
    if spamCount != 0 {
        t.Errorf("expected 0 messages in Spam folder, got %d", spamCount)
    }
}

func TestSpamFiltering_CaseInsensitive(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)

    msg := buildParserMessage("sender@example.com", []string{"caseuser@example.com"}, "Case Test", "Body")
    msg.Headers["X-Rspamd-Action"] = "REJECT"  // Uppercase
    msg.Headers["X-Spam-Status"] = "YES, score=5.0"  // Uppercase

    if err := stor.DeliverMessage("caseuser@example.com", msg, "INBOX"); err != nil {
        t.Fatalf("deliver failed: %v", err)
    }

    spamCount, _ := stor.GetMessageCountInFolder("caseuser", "Spam")
    if spamCount != 1 {
        t.Errorf("expected 1 message in Spam folder (case-insensitive matching), got %d", spamCount)
    }
}

func TestSpamFiltering_GreylistGoesToInbox(t *testing.T) {
    mgr := setupTestDBManager(t)
    stor := NewStorage(mgr)

    msg := buildParserMessage("sender@example.com", []string{"greyuser@example.com"}, "Greylist Test", "Body")
    msg.Headers["X-Rspamd-Action"] = "greylist"

    if err := stor.DeliverMessage("greyuser@example.com", msg, "INBOX"); err != nil {
        t.Fatalf("deliver failed: %v", err)
    }

    // Greylist action should go to INBOX, not Spam
    inboxCount, _ := stor.GetMessageCountInFolder("greyuser", "INBOX")
    if inboxCount != 1 {
        t.Errorf("expected 1 message in INBOX for greylist, got %d", inboxCount)
    }

    spamCount, _ := stor.GetMessageCountInFolder("greyuser", "Spam")
    if spamCount != 0 {
        t.Errorf("expected 0 messages in Spam folder for greylist, got %d", spamCount)
    }
}
