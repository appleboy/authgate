package util

// UniqueKeys extracts unique non-empty string keys from a slice using keyFn.
func UniqueKeys[T any](items []T, keyFn func(T) string) []string {
	seen := make(map[string]bool, len(items))
	keys := make([]string, 0, len(items))
	for _, item := range items {
		k := keyFn(item)
		if k != "" && !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	return keys
}
