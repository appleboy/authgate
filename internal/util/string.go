package util

import "strings"

// ScopeSet parses a space-separated scope string into a boolean lookup map.
func ScopeSet(scopes string) map[string]bool {
	set := make(map[string]bool)
	for s := range strings.FieldsSeq(scopes) {
		set[s] = true
	}
	return set
}

// IsScopeSubset returns true if every scope in requested is present in allowed.
// Both are space-separated scope strings. An empty requested string is always valid.
func IsScopeSubset(allowed, requested string) bool {
	if requested == "" {
		return true
	}
	allowedSet := ScopeSet(allowed)
	for sc := range strings.FieldsSeq(requested) {
		if !allowedSet[sc] {
			return false
		}
	}
	return true
}

// TruncateString truncates s to maxLen runes and appends "..." if truncated.
func TruncateString(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
