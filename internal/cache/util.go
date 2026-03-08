package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// prefixedKey prepends prefix to key.
func prefixedKey(prefix, key string) string {
	return prefix + key
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

// fetchThrough implements the cache-aside pattern: try Get, on miss call
// fetchFunc and store the result via Set. Used by MemoryCache and RueidisCache.
func fetchThrough[T any](
	ctx context.Context,
	key string,
	ttl time.Duration,
	get func(context.Context, string) (T, error),
	set func(context.Context, string, T, time.Duration) error,
	fetchFunc func(context.Context, string) (T, error),
) (T, error) {
	if value, err := get(ctx, key); err == nil {
		return value, nil
	}
	value, err := fetchFunc(ctx, key)
	if err != nil {
		var zero T
		return zero, err
	}
	_ = set(ctx, key, value, ttl)
	return value, nil
}

