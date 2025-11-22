package utils

import (
	"testing"
)

func TestFilterMailboxes_ExactMatch(t *testing.T) {
	mailboxes := []string{"INBOX", "Sent", "Drafts", "Trash"}
	matches := FilterMailboxes(mailboxes, "", "Sent")

	if len(matches) != 1 {
		t.Errorf("Expected 1 match, got %d", len(matches))
	}
	if len(matches) > 0 && matches[0] != "Sent" {
		t.Errorf("Expected 'Sent', got '%s'", matches[0])
	}
}

func TestFilterMailboxes_Wildcard(t *testing.T) {
	mailboxes := []string{"INBOX", "Sent", "Drafts", "Trash"}
	matches := FilterMailboxes(mailboxes, "", "*")

	if len(matches) != 4 {
		t.Errorf("Expected 4 matches, got %d", len(matches))
	}
}

func TestFilterMailboxes_PercentWildcard(t *testing.T) {
	mailboxes := []string{"INBOX", "Archive/2023", "Archive/2024", "Sent"}
	matches := FilterMailboxes(mailboxes, "", "Archive/%")

	expectedCount := 2
	if len(matches) != expectedCount {
		t.Errorf("Expected %d matches, got %d: %v", expectedCount, len(matches), matches)
	}
}

func TestFilterMailboxes_WithReference(t *testing.T) {
	mailboxes := []string{"Work/Projects", "Work/Archive", "Personal/Family"}
	matches := FilterMailboxes(mailboxes, "Work/", "*")

	// Should match Work/Projects and Work/Archive
	if len(matches) < 2 {
		t.Errorf("Expected at least 2 matches with Work/ reference, got %d: %v", len(matches), matches)
	}
}

func TestFilterMailboxes_INBOXAlwaysIncluded(t *testing.T) {
	mailboxes := []string{"Sent", "Drafts"}
	matches := FilterMailboxes(mailboxes, "", "*")

	// INBOX should be added even if not in original list
	found := false
	for _, m := range matches {
		if m == "INBOX" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected INBOX to be included in matches")
	}
}

func TestFilterMailboxes_INBOXCaseInsensitive(t *testing.T) {
	mailboxes := []string{"inbox", "Sent"}
	matches := FilterMailboxes(mailboxes, "", "*")

	// Should match inbox case-insensitively
	foundInbox := false
	for _, m := range matches {
		if m == "inbox" || m == "INBOX" {
			foundInbox = true
			break
		}
	}
	if !foundInbox {
		t.Error("Expected INBOX to match case-insensitively")
	}
}

func TestBuildCanonicalPattern_AbsolutePattern(t *testing.T) {
	pattern := BuildCanonicalPattern("ignored/ref", "/absolute/path", "/")
	expected := "/absolute/path"

	if pattern != expected {
		t.Errorf("Expected '%s', got '%s'", expected, pattern)
	}
}

func TestBuildCanonicalPattern_EmptyReference(t *testing.T) {
	pattern := BuildCanonicalPattern("", "pattern", "/")
	expected := "pattern"

	if pattern != expected {
		t.Errorf("Expected '%s', got '%s'", expected, pattern)
	}
}

func TestBuildCanonicalPattern_ReferenceWithDelimiter(t *testing.T) {
	pattern := BuildCanonicalPattern("ref/", "pattern", "/")
	expected := "ref/pattern"

	if pattern != expected {
		t.Errorf("Expected '%s', got '%s'", expected, pattern)
	}
}

func TestBuildCanonicalPattern_ReferenceWithoutDelimiter(t *testing.T) {
	pattern := BuildCanonicalPattern("ref", "pattern", "/")
	expected := "ref/pattern"

	if pattern != expected {
		t.Errorf("Expected '%s', got '%s'", expected, pattern)
	}
}

func TestBuildCanonicalPattern_PatternStartsWithDelimiter(t *testing.T) {
	pattern := BuildCanonicalPattern("ref", "/pattern", "/")
	// When pattern starts with delimiter, it's absolute - reference is ignored
	expected := "/pattern"

	if pattern != expected {
		t.Errorf("Expected '%s', got '%s'", expected, pattern)
	}
}

func TestMatchesPattern_ExactMatch(t *testing.T) {
	if !MatchesPattern("INBOX", "INBOX", "/") {
		t.Error("Expected exact match for INBOX")
	}
}

func TestMatchesPattern_NoMatch(t *testing.T) {
	if MatchesPattern("INBOX", "Sent", "/") {
		t.Error("Expected no match between INBOX and Sent")
	}
}

func TestMatchWildcard_StarMatchesAll(t *testing.T) {
	if !MatchWildcard("anything", "*", "/") {
		t.Error("* should match any string")
	}
	if !MatchWildcard("path/to/folder", "*", "/") {
		t.Error("* should match paths with delimiters")
	}
}

func TestMatchWildcard_StarAtEnd(t *testing.T) {
	if !MatchWildcard("Archive/2023", "Archive/*", "/") {
		t.Error("Archive/* should match Archive/2023")
	}
	if !MatchWildcard("Archive/2023/Jan", "Archive/*", "/") {
		t.Error("Archive/* should match Archive/2023/Jan")
	}
}

func TestMatchWildcard_StarInMiddle(t *testing.T) {
	if !MatchWildcard("prefix_middle_suffix", "prefix*suffix", "/") {
		t.Error("prefix*suffix should match prefix_middle_suffix")
	}
}

func TestMatchWildcard_PercentNoDelimiter(t *testing.T) {
	if !MatchWildcard("Archive2023", "Archive%", "/") {
		t.Error("Archive% should match Archive2023")
	}
	if MatchWildcard("Archive/2023", "Archive%", "/") {
		t.Error("Archive% should not match Archive/2023 (contains delimiter)")
	}
}

func TestMatchWildcard_PercentAtEnd(t *testing.T) {
	if !MatchWildcard("Work", "Work%", "/") {
		t.Error("Work% should match Work (zero characters)")
	}
	if !MatchWildcard("WorkItems", "Work%", "/") {
		t.Error("Work% should match WorkItems")
	}
}

func TestMatchWildcard_INBOXCaseInsensitive(t *testing.T) {
	if !MatchWildcard("INBOX", "inbox", "/") {
		t.Error("INBOX matching should be case-insensitive")
	}
	if !MatchWildcard("inbox", "INBOX", "/") {
		t.Error("inbox should match INBOX")
	}
}

func TestMatchWildcard_CaseSensitiveNonINBOX(t *testing.T) {
	if MatchWildcard("sent", "Sent", "/") {
		t.Error("Non-INBOX matching should be case-sensitive")
	}
}

func TestDoWildcardMatch_EmptyPattern(t *testing.T) {
	if !doWildcardMatch("", "", "/", 0, 0) {
		t.Error("Empty pattern should match empty text")
	}
	if doWildcardMatch("text", "", "/", 0, 0) {
		t.Error("Empty pattern should not match non-empty text")
	}
}

func TestDoWildcardMatch_EmptyText(t *testing.T) {
	if doWildcardMatch("", "pattern", "/", 0, 0) {
		t.Error("Empty text should not match non-empty pattern")
	}
}

func TestDoWildcardMatch_StarMatchesZero(t *testing.T) {
	if !doWildcardMatch("test", "t*est", "/", 0, 0) {
		t.Error("* should match zero characters")
	}
}

func TestDoWildcardMatch_StarMatchesMultiple(t *testing.T) {
	if !doWildcardMatch("testingmore", "t*more", "/", 0, 0) {
		t.Error("* should match multiple characters")
	}
}

func TestDoWildcardMatch_PercentMatchesZero(t *testing.T) {
	if !doWildcardMatch("test", "t%est", "/", 0, 0) {
		t.Error("% should match zero characters")
	}
}

func TestDoWildcardMatch_PercentStopsAtDelimiter(t *testing.T) {
	if doWildcardMatch("path/file", "path%file", "/", 0, 0) {
		t.Error("% should not match across delimiter")
	}
}

func TestDoWildcardMatch_PercentWithoutDelimiter(t *testing.T) {
	if !doWildcardMatch("pathfile", "path%file", "/", 0, 0) {
		t.Error("% should match characters without delimiter")
	}
}

func TestDoWildcardMatch_RegularCharacterMismatch(t *testing.T) {
	if doWildcardMatch("test", "text", "/", 0, 0) {
		t.Error("Should not match when regular characters differ")
	}
}

func TestDoWildcardMatch_PatternLongerThanText(t *testing.T) {
	if doWildcardMatch("test", "testing", "/", 0, 0) {
		t.Error("Should not match when pattern is longer than text")
	}
}

func TestDoWildcardMatch_ComplexPattern(t *testing.T) {
	if !doWildcardMatch("Work/Projects/2023", "Work/*/2023", "/", 0, 0) {
		t.Error("Should match complex pattern with *")
	}
	if !doWildcardMatch("WorkProjects", "Work%Projects", "/", 0, 0) {
		t.Error("Should match complex pattern with %")
	}
}

func TestDoWildcardMatch_MultipleWildcards(t *testing.T) {
	if !doWildcardMatch("abc_def_ghi", "a*c*f*i", "/", 0, 0) {
		t.Error("Should match pattern with multiple *")
	}
	if !doWildcardMatch("abcdefghi", "a%c%f%i", "/", 0, 0) {
		t.Error("Should match pattern with multiple %")
	}
}

func TestDoWildcardMatch_StarAtStart(t *testing.T) {
	if !doWildcardMatch("anything", "*thing", "/", 0, 0) {
		t.Error("*thing should match anything")
	}
}

func TestDoWildcardMatch_PercentAtStart(t *testing.T) {
	if !doWildcardMatch("anything", "%thing", "/", 0, 0) {
		t.Error("Pattern with % at start should match anything (no delimiter)")
	}
}

func TestFilterMailboxes_NoMatches(t *testing.T) {
	mailboxes := []string{"Sent", "Drafts"}
	matches := FilterMailboxes(mailboxes, "", "NonExistent")

	// Only INBOX might be added if it matches the pattern
	for _, m := range matches {
		if m != "INBOX" {
			t.Errorf("Expected no matches except possibly INBOX, got %s", m)
		}
	}
}

func TestMatchWildcard_EdgeCaseEmptyStrings(t *testing.T) {
	if !MatchWildcard("", "", "/") {
		t.Error("Empty string should match empty pattern")
	}
	if !MatchWildcard("", "*", "/") {
		t.Error("* should match empty string")
	}
	if !MatchWildcard("", "%", "/") {
		t.Error("% should match empty string")
	}
}
