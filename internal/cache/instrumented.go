package cache

import (
	"context"
	"errors"
	"time"

	"github.com/go-authgate/authgate/internal/core"

	"github.com/prometheus/client_golang/prometheus"
)

// Compile-time interface check.
var _ core.Cache[struct{}] = (*InstrumentedCache[struct{}])(nil)

// InstrumentedCache wraps a cache implementation with Prometheus metrics instrumentation.
// Records cache hits, misses, and errors for observability.
// This wrapper is transparent and does not change cache behavior.
type InstrumentedCache[T any] struct {
	underlying core.Cache[T]

	// Pre-resolved counters (avoids WithLabelValues map lookup per call).
	hitCounter  prometheus.Counter
	missCounter prometheus.Counter
	errGet      prometheus.Counter
	errSet      prometheus.Counter
	errDelete   prometheus.Counter
	errHealth   prometheus.Counter
	errFetch    prometheus.Counter
}

// NewInstrumentedCache creates a new instrumented cache wrapper.
// cacheName is used as a Prometheus label to distinguish between different caches.
func NewInstrumentedCache[T any](underlying core.Cache[T], cacheName string) *InstrumentedCache[T] {
	m := getMetrics()
	return &InstrumentedCache[T]{
		underlying:  underlying,
		hitCounter:  m.hits.WithLabelValues(cacheName),
		missCounter: m.misses.WithLabelValues(cacheName),
		errGet:      m.errors.WithLabelValues(cacheName, opGet),
		errSet:      m.errors.WithLabelValues(cacheName, opSet),
		errDelete:   m.errors.WithLabelValues(cacheName, opDelete),
		errHealth:   m.errors.WithLabelValues(cacheName, opHealth),
		errFetch:    m.errors.WithLabelValues(cacheName, opGetWithFetch),
	}
}

// Get retrieves a value from cache and records metrics.
func (i *InstrumentedCache[T]) Get(ctx context.Context, key string) (T, error) {
	value, err := i.underlying.Get(ctx, key)
	switch {
	case err == nil:
		i.hitCounter.Inc()
	case errors.Is(err, ErrCacheMiss):
		i.missCounter.Inc()
	default:
		i.errGet.Inc()
	}
	return value, err
}

// Set stores a value in cache and records errors.
func (i *InstrumentedCache[T]) Set(
	ctx context.Context,
	key string,
	value T,
	ttl time.Duration,
) error {
	err := i.underlying.Set(ctx, key, value, ttl)
	if err != nil {
		i.errSet.Inc()
	}
	return err
}

// Delete removes a key from cache and records errors.
func (i *InstrumentedCache[T]) Delete(ctx context.Context, key string) error {
	err := i.underlying.Delete(ctx, key)
	if err != nil {
		i.errDelete.Inc()
	}
	return err
}

// Close closes the underlying cache connection.
func (i *InstrumentedCache[T]) Close() error {
	return i.underlying.Close()
}

// Health checks the underlying cache health and records errors.
func (i *InstrumentedCache[T]) Health(ctx context.Context) error {
	err := i.underlying.Health(ctx)
	if err != nil {
		i.errHealth.Inc()
	}
	return err
}

// GetWithFetch implements the cache-aside pattern with metrics instrumentation.
// Wraps fetchFunc to detect whether it was called (miss) or not (hit), without
// calling underlying.Get() a second time. This preserves optimizations in the
// underlying implementation (e.g., stampede protection in RueidisAsideCache).
func (i *InstrumentedCache[T]) GetWithFetch(
	ctx context.Context,
	key string,
	ttl time.Duration,
	fetchFunc func(ctx context.Context, key string) (T, error),
) (T, error) {
	fetchCalled := false
	wrapped := func(ctx context.Context, key string) (T, error) {
		fetchCalled = true
		return fetchFunc(ctx, key)
	}

	value, err := i.underlying.GetWithFetch(ctx, key, ttl, wrapped)
	switch {
	case err != nil && !errors.Is(err, ErrCacheMiss):
		i.errFetch.Inc()
	case fetchCalled:
		i.missCounter.Inc()
	default:
		i.hitCounter.Inc()
	}
	return value, err
}
