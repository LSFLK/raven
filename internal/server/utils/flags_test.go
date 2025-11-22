package utils

import (
	"strings"
	"testing"
)

func TestCalculateNewFlags_ReplaceFlags(t *testing.T) {
	currentFlags := "\\Seen \\Flagged"
	newFlags := []string{"\\Answered", "\\Draft"}
	operation := "FLAGS"

	result := CalculateNewFlags(currentFlags, newFlags, operation)

	if strings.Contains(result, "\\Seen") {
		t.Error("Old flags should be removed with FLAGS operation")
	}
	if strings.Contains(result, "\\Flagged") {
		t.Error("Old flags should be removed with FLAGS operation")
	}
	if !strings.Contains(result, "\\Answered") {
		t.Error("Expected \\Answered flag")
	}
	if !strings.Contains(result, "\\Draft") {
		t.Error("Expected \\Draft flag")
	}
}

func TestCalculateNewFlags_ReplaceWithEmpty(t *testing.T) {
	currentFlags := "\\Seen \\Flagged"
	newFlags := []string{}
	operation := "FLAGS"

	result := CalculateNewFlags(currentFlags, newFlags, operation)

	if result != "" {
		t.Errorf("Expected empty flags, got '%s'", result)
	}
}

func TestCalculateNewFlags_RecentPreservedOnReplace(t *testing.T) {
	currentFlags := "\\Seen \\Recent"
	newFlags := []string{"\\Answered", "\\Recent"}
	operation := "FLAGS"

	result := CalculateNewFlags(currentFlags, newFlags, operation)

	// \\Recent in newFlags should be ignored (server-managed)
	if strings.Contains(result, "\\Recent") {
		t.Error("\\Recent should not be added via FLAGS operation")
	}
	if !strings.Contains(result, "\\Answered") {
		t.Error("Expected \\Answered flag")
	}
}

func TestCalculateNewFlags_AddFlags(t *testing.T) {
	currentFlags := "\\Seen"
	newFlags := []string{"\\Flagged", "\\Answered"}
	operation := "+FLAGS"

	result := CalculateNewFlags(currentFlags, newFlags, operation)

	if !strings.Contains(result, "\\Seen") {
		t.Error("Expected existing \\Seen flag to remain")
	}
	if !strings.Contains(result, "\\Flagged") {
		t.Error("Expected \\Flagged flag to be added")
	}
	if !strings.Contains(result, "\\Answered") {
		t.Error("Expected \\Answered flag to be added")
	}
}

func TestCalculateNewFlags_AddFlagsToEmpty(t *testing.T) {
	currentFlags := ""
	newFlags := []string{"\\Seen", "\\Flagged"}
	operation := "+FLAGS"

	result := CalculateNewFlags(currentFlags, newFlags, operation)

	if !strings.Contains(result, "\\Seen") {
		t.Error("Expected \\Seen flag")
	}
	if !strings.Contains(result, "\\Flagged") {
		t.Error("Expected \\Flagged flag")
	}
}

func TestCalculateNewFlags_AddDuplicateFlags(t *testing.T) {
	currentFlags := "\\Seen"
	newFlags := []string{"\\Seen", "\\Flagged"}
	operation := "+FLAGS"

	result := CalculateNewFlags(currentFlags, newFlags, operation)

	// Should not duplicate \\Seen
	seenCount := strings.Count(result, "\\Seen")
	if seenCount > 1 {
		t.Errorf("Expected \\Seen to appear once, appeared %d times", seenCount)
	}
	if !strings.Contains(result, "\\Flagged") {
		t.Error("Expected \\Flagged flag")
	}
}

func TestCalculateNewFlags_AddRecentIgnored(t *testing.T) {
	currentFlags := "\\Seen"
	newFlags := []string{"\\Recent", "\\Flagged"}
	operation := "+FLAGS"

	result := CalculateNewFlags(currentFlags, newFlags, operation)

	if strings.Contains(result, "\\Recent") {
		t.Error("\\Recent should not be added (server-managed)")
	}
	if !strings.Contains(result, "\\Seen") {
		t.Error("Expected \\Seen flag to remain")
	}
	if !strings.Contains(result, "\\Flagged") {
		t.Error("Expected \\Flagged flag")
	}
}

func TestCalculateNewFlags_RemoveFlags(t *testing.T) {
	currentFlags := "\\Seen \\Flagged \\Answered"
	newFlags := []string{"\\Flagged"}
	operation := "-FLAGS"

	result := CalculateNewFlags(currentFlags, newFlags, operation)

	if strings.Contains(result, "\\Flagged") {
		t.Error("\\Flagged should be removed")
	}
	if !strings.Contains(result, "\\Seen") {
		t.Error("Expected \\Seen flag to remain")
	}
	if !strings.Contains(result, "\\Answered") {
		t.Error("Expected \\Answered flag to remain")
	}
}

func TestCalculateNewFlags_RemoveNonExistentFlag(t *testing.T) {
	currentFlags := "\\Seen"
	newFlags := []string{"\\Flagged"}
	operation := "-FLAGS"

	result := CalculateNewFlags(currentFlags, newFlags, operation)

	if !strings.Contains(result, "\\Seen") {
		t.Error("Expected \\Seen flag to remain")
	}
	if strings.Contains(result, "\\Flagged") {
		t.Error("\\Flagged should not appear (wasn't there to begin with)")
	}
}

func TestCalculateNewFlags_RemoveAllFlags(t *testing.T) {
	currentFlags := "\\Seen \\Flagged"
	newFlags := []string{"\\Seen", "\\Flagged"}
	operation := "-FLAGS"

	result := CalculateNewFlags(currentFlags, newFlags, operation)

	if result != "" {
		t.Errorf("Expected empty result, got '%s'", result)
	}
}

func TestCalculateNewFlags_RemoveRecentIgnored(t *testing.T) {
	currentFlags := "\\Seen \\Recent"
	newFlags := []string{"\\Recent"}
	operation := "-FLAGS"

	result := CalculateNewFlags(currentFlags, newFlags, operation)

	// Should not remove \\Recent (server-managed)
	if !strings.Contains(result, "\\Seen") {
		t.Error("Expected \\Seen to remain")
	}
}

func TestCalculateNewFlags_UnknownOperation(t *testing.T) {
	currentFlags := "\\Seen"
	newFlags := []string{"\\Flagged"}
	operation := "UNKNOWN"

	result := CalculateNewFlags(currentFlags, newFlags, operation)

	// Should return current flags unchanged for unknown operation
	if !strings.Contains(result, "\\Seen") {
		t.Error("Expected original flags to remain for unknown operation")
	}
	if strings.Contains(result, "\\Flagged") {
		t.Error("New flags should not be added for unknown operation")
	}
}

func TestCalculateNewFlags_EmptyCurrentFlags(t *testing.T) {
	currentFlags := ""
	newFlags := []string{"\\Seen"}
	operation := "FLAGS"

	result := CalculateNewFlags(currentFlags, newFlags, operation)

	if !strings.Contains(result, "\\Seen") {
		t.Error("Expected \\Seen flag")
	}
}

func TestGetMailboxAttributes_Drafts(t *testing.T) {
	result := GetMailboxAttributes("Drafts")
	expected := "\\Drafts"

	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestGetMailboxAttributes_Trash(t *testing.T) {
	result := GetMailboxAttributes("Trash")
	expected := "\\Trash"

	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestGetMailboxAttributes_Sent(t *testing.T) {
	result := GetMailboxAttributes("Sent")
	expected := "\\Sent"

	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestGetMailboxAttributes_INBOX(t *testing.T) {
	result := GetMailboxAttributes("INBOX")
	expected := "\\Unmarked"

	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestGetMailboxAttributes_Default(t *testing.T) {
	result := GetMailboxAttributes("CustomFolder")
	expected := "\\Unmarked"

	if result != expected {
		t.Errorf("Expected %s for custom folder, got %s", expected, result)
	}
}

func TestGetMailboxAttributes_EmptyName(t *testing.T) {
	result := GetMailboxAttributes("")
	expected := "\\Unmarked"

	if result != expected {
		t.Errorf("Expected %s for empty name, got %s", expected, result)
	}
}
