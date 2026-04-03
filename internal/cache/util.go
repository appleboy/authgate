package cache

import (
	"context"
	"encoding/json"
	"fmt"

	"golang.org/x/sync/singleflight"
)

// doWithSingleflight runs fn under singleflight deduplication.
// The shared fetch runs in a context detached from all callers so neither
// cancellation nor any individual caller's deadline can abort the shared work.
func doWithSingleflight[T any](
	ctx context.Context,
	key string,
	sf *singleflight.Group,
	fn func(sharedCtx context.Context) (T, error),
) (T, error) {
	resultCh := sf.DoChan(key, func() (any, error) {
		result, err := fn(context.WithoutCancel(ctx))
		return result, err
	})
	select {
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	case res := <-resultCh:
		if res.Err != nil {
			var zero T
			return zero, res.Err
		}
		return res.Val.(T), nil
	}
}

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
