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
