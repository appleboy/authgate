package cache

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNoopCache_Get(t *testing.T) {
	c := NewNoopCache[string]()
	_, err := c.Get(context.Background(), "key")
	if !errors.Is(err, ErrCacheMiss) {
		t.Fatalf("Get: want ErrCacheMiss, got %v", err)
	}
}

func TestNoopCache_Set(t *testing.T) {
	c := NewNoopCache[string]()
	if err := c.Set(context.Background(), "key", "val", time.Minute); err != nil {
		t.Fatalf("Set: unexpected error: %v", err)
	}
	// Value should not be retained
	_, err := c.Get(context.Background(), "key")
	if !errors.Is(err, ErrCacheMiss) {
		t.Fatalf("Get after Set: want ErrCacheMiss, got %v", err)
	}
}

func TestNoopCache_Delete(t *testing.T) {
	c := NewNoopCache[string]()
	if err := c.Delete(context.Background(), "key"); err != nil {
		t.Fatalf("Delete: unexpected error: %v", err)
	}
}

func TestNoopCache_Close(t *testing.T) {
	c := NewNoopCache[string]()
	if err := c.Close(); err != nil {
		t.Fatalf("Close: unexpected error: %v", err)
	}
}

func TestNoopCache_Health(t *testing.T) {
	c := NewNoopCache[string]()
	if err := c.Health(context.Background()); err != nil {
		t.Fatalf("Health: unexpected error: %v", err)
	}
}

func TestNoopCache_GetWithFetch(t *testing.T) {
	c := NewNoopCache[int]()
	called := false
	val, err := c.GetWithFetch(context.Background(), "k", time.Minute,
		func(_ context.Context, key string) (int, error) {
			called = true
			if key != "k" {
				t.Fatalf("fetchFunc received key %q, want %q", key, "k")
			}
			return 42, nil
		},
	)
	if err != nil {
		t.Fatalf("GetWithFetch: unexpected error: %v", err)
	}
	if !called {
		t.Fatal("GetWithFetch: fetchFunc was not called")
	}
	if val != 42 {
		t.Fatalf("GetWithFetch: got %d, want 42", val)
	}
}

func TestNoopCache_GetWithFetch_PropagatesError(t *testing.T) {
	c := NewNoopCache[int]()
	wantErr := errors.New("db down")
	_, err := c.GetWithFetch(context.Background(), "k", time.Minute,
		func(_ context.Context, _ string) (int, error) {
			return 0, wantErr
		},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("GetWithFetch: got error %v, want %v", err, wantErr)
	}
}
