package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/rueidis"
)

// redisSet stores a value in Redis with TTL using the provided client.
func redisSet[T any](ctx context.Context, client rueidis.Client, keyPrefix, key string, value T, ttl time.Duration) error {
	encoded, err := marshalValue(value)
	if err != nil {
		return err
	}

	cmd := client.B().Set().
		Key(prefixedKey(keyPrefix, key)).
		Value(encoded).
		Ex(ttl).
		Build()

	if err := client.Do(ctx, cmd).Error(); err != nil {
		return fmt.Errorf("%w: %v", ErrCacheUnavailable, err)
	}

	return nil
}

// redisMSet stores multiple values in Redis with TTL using pipelined SET commands.
func redisMSet[T any](ctx context.Context, client rueidis.Client, keyPrefix string, values map[string]T, ttl time.Duration) error {
	if len(values) == 0 {
		return nil
	}

	cmds := make(rueidis.Commands, 0, len(values))
	for key, value := range values {
		encoded, err := marshalValue(value)
		if err != nil {
			return err
		}

		cmd := client.B().Set().
			Key(prefixedKey(keyPrefix, key)).
			Value(encoded).
			Ex(ttl).
			Build()
		cmds = append(cmds, cmd)
	}

	for _, resp := range client.DoMulti(ctx, cmds...) {
		if err := resp.Error(); err != nil {
			return fmt.Errorf("%w: %v", ErrCacheUnavailable, err)
		}
	}

	return nil
}

// redisDelete removes a key from Redis.
func redisDelete(ctx context.Context, client rueidis.Client, keyPrefix, key string) error {
	cmd := client.B().Del().Key(prefixedKey(keyPrefix, key)).Build()
	if err := client.Do(ctx, cmd).Error(); err != nil {
		return fmt.Errorf("%w: %v", ErrCacheUnavailable, err)
	}

	return nil
}

// redisHealth checks if Redis is reachable.
func redisHealth(ctx context.Context, client rueidis.Client) error {
	cmd := client.B().Ping().Build()
	if err := client.Do(ctx, cmd).Error(); err != nil {
		return fmt.Errorf("%w: %v", ErrCacheUnavailable, err)
	}
	return nil
}

// parseMultiGetResponse maps Redis MGET results back to their original keys,
// skipping nil or unparseable entries.
//
// Note: rueidis.RedisMessage has unexported fields and cannot be constructed
// outside the rueidis package, so this function is not directly unit-testable.
// The decode path (unmarshalValue) is covered by TestMarshalValue/TestUnmarshalValue,
// and the full MGet behaviour is exercised by Redis integration tests.
func parseMultiGetResponse[T any](keys []string, values []rueidis.RedisMessage) map[string]T {
	result := make(map[string]T, len(keys))
	for i, val := range values {
		if val.IsNil() {
			continue
		}
		str, err := val.ToString()
		if err != nil {
			continue
		}
		item, err := unmarshalValue[T](str)
		if err != nil {
			continue
		}
		result[keys[i]] = item
	}
	return result
}

// defaultGetWithFetch implements the cache-aside pattern shared by
// MemoryCache and RueidisCache. On cache hit, returns the cached value.
// On miss, calls fetchFunc and stores the result.
// No stampede protection is provided.
func defaultGetWithFetch[T any](
	ctx context.Context,
	c cacheGetSetter[T],
	key string,
	ttl time.Duration,
	fetchFunc func(ctx context.Context, key string) (T, error),
) (T, error) {
	if value, err := c.Get(ctx, key); err == nil {
		return value, nil
	}
	value, err := fetchFunc(ctx, key)
	if err != nil {
		var zero T
		return zero, err
	}
	_ = c.Set(ctx, key, value, ttl)
	return value, nil
}

// cacheGetSetter is the minimal interface needed by defaultGetWithFetch.
type cacheGetSetter[T any] interface {
	Get(ctx context.Context, key string) (T, error)
	Set(ctx context.Context, key string, value T, ttl time.Duration) error
}
