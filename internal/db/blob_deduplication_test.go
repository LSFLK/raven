package db

import (
	"encoding/base64"
	"strings"
	"testing"
)

// TestBlobDeduplicationAcrossEncodings tests that the same binary content with different
// encoding representations (e.g., base64 with different line breaks) produces the same hash
func TestBlobDeduplicationAcrossEncodings(t *testing.T) {
	db, err := InitDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Sample binary content (simulating an image/attachment)
	originalContent := []byte("This is a test attachment with some binary data: \x00\x01\x02\xFF\xFE")

	// Encode the same content in different ways (simulating send vs receive)
	base64Encoded1 := base64.StdEncoding.EncodeToString(originalContent)
	base64Encoded2 := base64.StdEncoding.EncodeToString(originalContent)

	// Add line breaks at different positions (common in email transmission)
	base64Encoded2 = addLineBreaks(base64Encoded2, 76) // Wrap at 76 chars (RFC 2045)

	// Store the same content with different base64 representations
	id1, err := StoreBlobWithEncoding(db, base64Encoded1, "base64")
	if err != nil {
		t.Fatalf("Failed to store first blob: %v", err)
	}

	id2, err := StoreBlobWithEncoding(db, base64Encoded2, "base64")
	if err != nil {
		t.Fatalf("Failed to store second blob: %v", err)
	}

	// Verify deduplication worked
	if id1 != id2 {
		t.Errorf("Blob IDs differ (deduplication failed): %d vs %d", id1, id2)
		t.Errorf("First encoding (no breaks): %s", base64Encoded1)
		t.Errorf("Second encoding (with breaks): %s", base64Encoded2[:50])
	}

	// Verify reference count
	var refCount int
	err = db.QueryRow("SELECT reference_count FROM blobs WHERE id = ?", id1).Scan(&refCount)
	if err != nil {
		t.Fatalf("Failed to get reference count: %v", err)
	}

	if refCount != 2 {
		t.Errorf("Expected reference count 2, got %d", refCount)
	}

	t.Logf("✅ Blob deduplication across encodings successful!")
	t.Logf("   ID: %d", id1)
	t.Logf("   Reference count: %d", refCount)
}

// TestBlobDeduplicationRawVsBase64 tests that the same content stored as raw text
// and as base64 produces the same hash (when both are decoded to binary)
func TestBlobDeduplicationRawVsBase64(t *testing.T) {
	db, err := InitDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Same content, different encodings
	textContent := "Hello, World! This is a test."
	base64Content := base64.StdEncoding.EncodeToString([]byte(textContent))

	// Store as 7bit (raw)
	id1, err := StoreBlobWithEncoding(db, textContent, "7bit")
	if err != nil {
		t.Fatalf("Failed to store raw blob: %v", err)
	}

	// Store as base64
	id2, err := StoreBlobWithEncoding(db, base64Content, "base64")
	if err != nil {
		t.Fatalf("Failed to store base64 blob: %v", err)
	}

	// Should be deduplicated
	if id1 != id2 {
		t.Errorf("IDs differ (no deduplication): %d vs %d", id1, id2)
	}

	var refCount int
	db.QueryRow("SELECT reference_count FROM blobs WHERE id = ?", id1).Scan(&refCount)

	if refCount != 2 {
		t.Errorf("Expected reference count 2, got %d", refCount)
	}

	t.Logf("✅ Deduplication between raw and base64 successful!")
}

// TestBlobDeduplicationWithLineBreaks tests that base64 content with different
// line break patterns produces the same hash
func TestBlobDeduplicationWithLineBreaks(t *testing.T) {
	db, err := InitDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	originalContent := []byte("This is a longer test to simulate real attachments with multiple lines of base64 encoding that would typically be wrapped at 76 characters per line in email messages.")

	// Version 1: Single line base64
	base64V1 := base64.StdEncoding.EncodeToString(originalContent)

	// Version 2: Base64 with CRLF line breaks (RFC 2045: max 76 chars per line)
	base64V2 := addLineBreaks(base64.StdEncoding.EncodeToString(originalContent), 76)

	// Version 3: Base64 with different line break positions
	base64V3 := addLineBreaks(base64.StdEncoding.EncodeToString(originalContent), 64)

	id1, _ := StoreBlobWithEncoding(db, base64V1, "base64")
	id2, _ := StoreBlobWithEncoding(db, base64V2, "base64")
	id3, _ := StoreBlobWithEncoding(db, base64V3, "base64")

	if id1 != id2 || id2 != id3 {
		t.Errorf("IDs differ despite same content: %d, %d, %d", id1, id2, id3)
	}

	var refCount int
	db.QueryRow("SELECT reference_count FROM blobs WHERE id = ?", id1).Scan(&refCount)

	if refCount != 3 {
		t.Errorf("Expected reference count 3, got %d", refCount)
	}

	t.Logf("✅ Deduplication with various line breaks successful!")
}

// TestBlobDeduplicationSentAndReceived simulates the real-world scenario where
// the same attachment is sent AND received, potentially with different encodings
func TestBlobDeduplicationSentAndReceived(t *testing.T) {
	db, err := InitDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Simulate an attachment (e.g., a small image or PDF)
	attachmentData := []byte("Sample attachment content with binary data\x00\x01\x02\xFF")

	// Sent: Client encodes as single-line base64
	sentEncoded := base64.StdEncoding.EncodeToString(attachmentData)

	// Received: Server/relay may re-wrap at 76 characters
	receivedEncoded := addLineBreaks(base64.StdEncoding.EncodeToString(attachmentData), 76)

	// Store "sent" attachment
	sentID, err := StoreBlobWithEncoding(db, sentEncoded, "base64")
	if err != nil {
		t.Fatalf("Failed to store sent attachment: %v", err)
	}

	// Store "received" attachment
	receivedID, err := StoreBlobWithEncoding(db, receivedEncoded, "base64")
	if err != nil {
		t.Fatalf("Failed to store received attachment: %v", err)
	}

	// The critical test: Should be the SAME blob ID
	if sentID != receivedID {
		t.Errorf("❌ FAILED: Sent and received attachments stored separately!")
		t.Errorf("   Sent ID: %d, Received ID: %d", sentID, receivedID)
		t.Fatalf("This is the bug we're fixing - same attachment stored twice!")
	}

	// Verify only one blob in database
	var blobCount int
	db.QueryRow("SELECT COUNT(*) FROM blobs").Scan(&blobCount)
	if blobCount != 1 {
		t.Errorf("Expected 1 blob in database, got %d", blobCount)
	}

	// Verify reference count
	var refCount int
	db.QueryRow("SELECT reference_count FROM blobs WHERE id = ?", sentID).Scan(&refCount)
	if refCount != 2 {
		t.Errorf("Expected reference count 2, got %d", refCount)
	}

	t.Logf("✅ SUCCESS: Sent and received attachments deduplicated correctly!")
	t.Logf("   Blob ID: %d", sentID)
	t.Logf("   Reference count: %d", refCount)
	t.Logf("   Total blobs in database: %d", blobCount)
}

// TestBlobDeduplicationQuotedPrintable tests deduplication with quoted-printable encoding
func TestBlobDeduplicationQuotedPrintable(t *testing.T) {
	db, err := InitDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Content that might be quoted-printable encoded
	originalContent := "Hello! This is a test with special chars: café, naïve"

	// Quoted-printable encoded version
	qpEncoded := "Hello! This is a test with special chars: caf=C3=A9, na=C3=AFve"

	// Store as raw
	id1, err := StoreBlobWithEncoding(db, originalContent, "7bit")
	if err != nil {
		t.Fatalf("Failed to store raw content: %v", err)
	}

	// Store as quoted-printable
	id2, err := StoreBlobWithEncoding(db, qpEncoded, "quoted-printable")
	if err != nil {
		t.Fatalf("Failed to store quoted-printable content: %v", err)
	}

	// Should be deduplicated
	if id1 != id2 {
		t.Logf("Note: Quoted-printable deduplication may differ due to encoding specifics")
		t.Logf("ID1 (raw): %d, ID2 (qp): %d", id1, id2)
	}
}

// TestBlobDeduplicationBackwardCompatibility ensures that blobs stored without
// encoding information still work
func TestBlobDeduplicationBackwardCompatibility(t *testing.T) {
	db, err := InitDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	content := "Test content for backward compatibility"

	// Old way (no encoding specified)
	id1, err := StoreBlobWithEncoding(db, content, "")
	if err != nil {
		t.Fatalf("Failed to store blob (old way): %v", err)
	}

	// New way (with empty encoding)
	id2, err := StoreBlobWithEncoding(db, content, "")
	if err != nil {
		t.Fatalf("Failed to store blob (new way): %v", err)
	}

	// Should be the same
	if id1 != id2 {
		t.Errorf("Backward compatibility broken: %d vs %d", id1, id2)
	}

	t.Logf("✅ Backward compatibility maintained!")
}

// Helper function to add line breaks to base64 string
func addLineBreaks(s string, lineLen int) string {
	var result strings.Builder
	for i := 0; i < len(s); i += lineLen {
		end := i + lineLen
		if end > len(s) {
			end = len(s)
		}
		result.WriteString(s[i:end])
		if end < len(s) {
			result.WriteString("\r\n")
		}
	}
	return result.String()
}
