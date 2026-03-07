package cache

import (
	"errors"
	"testing"
)

func TestMarshalValue_Roundtrip(t *testing.T) {
	type payload struct {
		Count int64  `json:"count"`
		Label string `json:"label"`
	}

	orig := payload{Count: 42, Label: "hello"}
	encoded, err := marshalValue(orig)
	if err != nil {
		t.Fatalf("marshalValue failed: %v", err)
	}
	if encoded == "" {
		t.Fatal("marshalValue returned empty string")
	}

	got, err := unmarshalValue[payload](encoded)
	if err != nil {
		t.Fatalf("unmarshalValue failed: %v", err)
	}
	if got != orig {
		t.Errorf("roundtrip mismatch: got %+v, want %+v", got, orig)
	}
}

func TestMarshalValue_Primitives(t *testing.T) {
	encoded, err := marshalValue(int64(99))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if encoded != "99" {
		t.Errorf("expected \"99\", got %q", encoded)
	}
}

func TestUnmarshalValue_InvalidJSON(t *testing.T) {
	_, err := unmarshalValue[int64]("not-json")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if !errors.Is(err, ErrInvalidValue) {
		t.Errorf("expected ErrInvalidValue, got %v", err)
	}
}

func TestUnmarshalValue_EmptyString(t *testing.T) {
	_, err := unmarshalValue[int64]("")
	if err == nil {
		t.Fatal("expected error for empty string, got nil")
	}
	if !errors.Is(err, ErrInvalidValue) {
		t.Errorf("expected ErrInvalidValue, got %v", err)
	}
}

func TestPrefixedKey(t *testing.T) {
	cases := []struct{ prefix, key, want string }{
		{"metrics:", "active_tokens", "metrics:active_tokens"},
		{"m:", "", "m:"},
		{"", "key", "key"},
	}
	for _, tc := range cases {
		if got := prefixedKey(tc.prefix, tc.key); got != tc.want {
			t.Errorf("prefixedKey(%q, %q) = %q, want %q", tc.prefix, tc.key, got, tc.want)
		}
	}
}

func TestPrefixedKeys(t *testing.T) {
	got := prefixedKeys("m:", []string{"a", "b", "c"})
	want := []string{"m:a", "m:b", "m:c"}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestPrefixedKeys_Empty(t *testing.T) {
	if got := prefixedKeys("m:", nil); len(got) != 0 {
		t.Errorf("expected empty slice for nil input, got %v", got)
	}
}
