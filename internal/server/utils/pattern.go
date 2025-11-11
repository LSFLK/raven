package utils

import (
	"strings"
)

// FilterMailboxes applies reference and pattern matching according to RFC 3501
func FilterMailboxes(mailboxes []string, reference, pattern string) []string {
	var matches []string
	hierarchyDelimiter := "/"

	// Construct the canonical form by combining reference and pattern
	canonicalPattern := BuildCanonicalPattern(reference, pattern, hierarchyDelimiter)

	for _, mailbox := range mailboxes {
		if MatchesPattern(mailbox, canonicalPattern, hierarchyDelimiter) {
			matches = append(matches, mailbox)
		}
	}

	// Always include INBOX if it matches the pattern (case-insensitive)
	inboxPattern := strings.ToUpper(canonicalPattern)
	if MatchesPattern("INBOX", inboxPattern, hierarchyDelimiter) {
		// Check if INBOX is already in the list
		found := false
		for _, match := range matches {
			if strings.ToUpper(match) == "INBOX" {
				found = true
				break
			}
		}
		if !found {
			matches = append(matches, "INBOX")
		}
	}

	return matches
}

// BuildCanonicalPattern builds the canonical pattern from reference and mailbox pattern
func BuildCanonicalPattern(reference, pattern, delimiter string) string {
	// If pattern starts with delimiter, it's absolute - ignore reference
	if strings.HasPrefix(pattern, delimiter) {
		return pattern
	}

	// If reference is empty, use pattern as-is
	if reference == "" {
		return pattern
	}

	// If reference doesn't end with delimiter and pattern doesn't start with delimiter,
	// and reference is not empty, append delimiter
	if !strings.HasSuffix(reference, delimiter) && !strings.HasPrefix(pattern, delimiter) {
		return reference + delimiter + pattern
	}

	// Otherwise, concatenate reference and pattern
	return reference + pattern
}

// MatchesPattern checks if a mailbox name matches a pattern with wildcards
func MatchesPattern(mailbox, pattern, delimiter string) bool {
	return MatchWildcard(mailbox, pattern, delimiter)
}

// MatchWildcard implements wildcard matching for IMAP LIST patterns
func MatchWildcard(text, pattern, delimiter string) bool {
	// Convert to case-insensitive for INBOX matching
	if strings.ToUpper(text) == "INBOX" {
		text = "INBOX"
	}
	if strings.ToUpper(pattern) == "INBOX" {
		pattern = "INBOX"
	}

	return doWildcardMatch(text, pattern, delimiter, 0, 0)
}

// doWildcardMatch performs recursive wildcard matching
func doWildcardMatch(text, pattern, delimiter string, textPos, patternPos int) bool {
	for patternPos < len(pattern) {
		switch pattern[patternPos] {
		case '*':
			// * matches zero or more characters
			patternPos++
			if patternPos >= len(pattern) {
				return true // * at end matches everything
			}

			// Try matching * with zero characters first
			if doWildcardMatch(text, pattern, delimiter, textPos, patternPos) {
				return true
			}

			// Try matching * with one or more characters
			for textPos < len(text) {
				textPos++
				if doWildcardMatch(text, pattern, delimiter, textPos, patternPos) {
					return true
				}
			}
			return false

		case '%':
			// % matches zero or more characters but not hierarchy delimiter
			patternPos++
			if patternPos >= len(pattern) {
				// % at end - check if remaining text contains delimiter
				return !strings.Contains(text[textPos:], delimiter)
			}

			// Try matching % with zero characters first
			if doWildcardMatch(text, pattern, delimiter, textPos, patternPos) {
				return true
			}

			// Try matching % with one or more characters (but not delimiter)
			for textPos < len(text) && !strings.HasPrefix(text[textPos:], delimiter) {
				textPos++
				if doWildcardMatch(text, pattern, delimiter, textPos, patternPos) {
					return true
				}
			}
			return false

		default:
			// Regular character - must match exactly
			if textPos >= len(text) || text[textPos] != pattern[patternPos] {
				return false
			}
			textPos++
			patternPos++
		}
	}

	// Pattern consumed - text should also be consumed
	return textPos >= len(text)
}
