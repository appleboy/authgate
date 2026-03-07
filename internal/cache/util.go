package cache

import (
	"encoding/json"
	"fmt"
)

// prefixedKey prepends prefix to key.
func prefixedKey(prefix, key string) string {
	return prefix + key
}

// prefixedKeys returns a new slice with prefix prepended to each key.
func prefixedKeys(prefix string, keys []string) []string {
	full := make([]string, len(keys))
	for i, k := range keys {
		full[i] = prefix + k
	}
	return full
}

// marshalValue encodes a value to its JSON string representation.
func marshalValue[T any](value T) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidValue, err)
	}
	return string(encoded), nil
}

// unmarshalValue decodes a JSON string into a value.
func unmarshalValue[T any](str string) (T, error) {
	var value T
	if err := json.Unmarshal([]byte(str), &value); err != nil {
		var zero T
		return zero, fmt.Errorf("%w: %v", ErrInvalidValue, err)
	}
	return value, nil
}
