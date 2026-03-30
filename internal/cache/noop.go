package cache

import (
	"context"
	"time"

	"github.com/go-authgate/authgate/internal/core"
)

// Compile-time interface check.
var _ core.Cache[struct{}] = (*NoopCache[struct{}])(nil)

// NoopCache implements Cache interface with no-op behavior.
// Used when caching is disabled.
type NoopCache[T any] struct{}

// NewNoopCache creates a new no-op cache instance.
func NewNoopCache[T any]() *NoopCache[T] {
	return &NoopCache[T]{}
}

func (n *NoopCache[T]) Get(_ context.Context, _ string) (T, error) {
	var zero T
	return zero, ErrCacheMiss
}

func (n *NoopCache[T]) Set(_ context.Context, _ string, _ T, _ time.Duration) error {
	return nil
}

func (n *NoopCache[T]) Delete(_ context.Context, _ string) error {
	return nil
}

func (n *NoopCache[T]) Close() error {
	return nil
}

func (n *NoopCache[T]) Health(_ context.Context) error {
	return nil
}

func (n *NoopCache[T]) GetWithFetch(
	ctx context.Context,
	key string,
	_ time.Duration,
	fetchFunc func(ctx context.Context, key string) (T, error),
) (T, error) {
	return fetchFunc(ctx, key)
}
