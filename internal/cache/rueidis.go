package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/go-authgate/authgate/internal/core"

	"github.com/redis/rueidis"
)

// Compile-time interface check.
var _ core.Cache[struct{}] = (*RueidisCache[struct{}])(nil)

// RueidisCache implements Cache interface using Redis via rueidis client.
// Suitable for multi-instance deployments where cache needs to be shared.
type RueidisCache[T any] struct {
	client    rueidis.Client
	keyPrefix string
}

// NewRueidisCache creates a new Redis cache instance using rueidis.
func NewRueidisCache[T any](
	ctx context.Context,
	addr, password string,
	db int,
	keyPrefix string,
) (*RueidisCache[T], error) {
	client, err := rueidis.NewClient(rueidis.ClientOption{
		InitAddress:  []string{addr},
		Password:     password,
		SelectDB:     db,
		DisableCache: true, // Basic mode without client-side caching
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create redis client: %w", err)
	}

	// Test connection with provided context
	if err := client.Do(ctx, client.B().Ping().Build()).Error(); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to ping Redis: %w", err)
	}

	return &RueidisCache[T]{
		client:    client,
		keyPrefix: keyPrefix,
	}, nil
}

// Get retrieves a value from Redis.
func (r *RueidisCache[T]) Get(ctx context.Context, key string) (T, error) {
	cmd := r.client.B().Get().Key(prefixedKey(r.keyPrefix, key)).Build()
	resp := r.client.Do(ctx, cmd)

	if err := resp.Error(); err != nil {
		var zero T
		if rueidis.IsRedisNil(err) {
			return zero, ErrCacheMiss
		}
		return zero, fmt.Errorf("%w: %v", ErrCacheUnavailable, err)
	}

	str, err := resp.ToString()
	if err != nil {
		var zero T
		return zero, fmt.Errorf("%w: %v", ErrInvalidValue, err)
	}

	return unmarshalValue[T](str)
}

// Set stores a value in Redis with TTL.
func (r *RueidisCache[T]) Set(ctx context.Context, key string, value T, ttl time.Duration) error {
	return redisSet(ctx, r.client, r.keyPrefix, key, value, ttl)
}

// MGet retrieves multiple values from Redis.
func (r *RueidisCache[T]) MGet(ctx context.Context, keys []string) (map[string]T, error) {
	if len(keys) == 0 {
		return make(map[string]T), nil
	}

	cmd := r.client.B().Mget().Key(prefixedKeys(r.keyPrefix, keys)...).Build()
	resp := r.client.Do(ctx, cmd)

	if err := resp.Error(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCacheUnavailable, err)
	}

	values, err := resp.ToArray()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidValue, err)
	}

	return parseMultiGetResponse[T](keys, values), nil
}

// MSet stores multiple values in Redis with TTL.
func (r *RueidisCache[T]) MSet(ctx context.Context, values map[string]T, ttl time.Duration) error {
	return redisMSet(ctx, r.client, r.keyPrefix, values, ttl)
}

// Delete removes a key from Redis.
func (r *RueidisCache[T]) Delete(ctx context.Context, key string) error {
	return redisDelete(ctx, r.client, r.keyPrefix, key)
}

// Close closes the Redis connection.
func (r *RueidisCache[T]) Close() error {
	r.client.Close()
	return nil
}

// Health checks if Redis is reachable.
func (r *RueidisCache[T]) Health(ctx context.Context) error {
	return redisHealth(ctx, r.client)
}

// GetWithFetch retrieves a value using the cache-aside pattern.
// On cache miss, fetchFunc is called and the result is stored in cache.
// No stampede protection is provided.
func (r *RueidisCache[T]) GetWithFetch(
	ctx context.Context,
	key string,
	ttl time.Duration,
	fetchFunc func(ctx context.Context, key string) (T, error),
) (T, error) {
	return defaultGetWithFetch(ctx, r, key, ttl, fetchFunc)
}
