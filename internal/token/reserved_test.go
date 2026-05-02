package token

import (
	"errors"
	"testing"
)

func TestBuildReservedClaimKeys_DefaultPrefix(t *testing.T) {
	got := BuildReservedClaimKeys("extra")

	// Static RFC/OIDC/AuthGate-internal keys must always be reserved.
	staticKeys := []string{
		"iss", "sub", "aud", "exp", "nbf", "iat", "jti",
		"type", "scope", "user_id", "client_id",
		"azp", "amr", "acr", "auth_time", "nonce", "at_hash",
	}
	for _, k := range staticKeys {
		if _, ok := got[k]; !ok {
			t.Errorf("expected %q to be reserved (static key)", k)
		}
	}

	// Every PrivateClaim entry contributes <prefix>_<logical>.
	for _, pc := range PrivateClaims {
		composed := EmittedName("extra", pc.LogicalName)
		if _, ok := got[composed]; !ok {
			t.Errorf("expected %q (composed) to be reserved", composed)
		}
	}

	// Bare logical names MUST also be reserved so callers cannot smuggle a
	// legacy claim name (`domain` / `project` / `service_account`) past the
	// parser. Without this, an attacker could re-introduce the pre-prefix
	// keys that an un-migrated downstream consumer might still trust.
	for _, bare := range []string{"domain", "project", "service_account"} {
		if _, ok := got[bare]; !ok {
			t.Errorf("bare %q must be reserved (legacy-name impersonation guard)", bare)
		}
	}

	// Sanity: arbitrary non-reserved keys remain free.
	for _, allowed := range []string{"tenant", "trace_id", "department", "role", "feature_flags"} {
		if _, ok := got[allowed]; ok {
			t.Errorf("expected %q to NOT be reserved", allowed)
		}
	}
}

func TestBuildReservedClaimKeys_CustomPrefix(t *testing.T) {
	got := BuildReservedClaimKeys("acme")
	for _, pc := range PrivateClaims {
		want := "acme_" + pc.LogicalName
		if _, ok := got[want]; !ok {
			t.Errorf("expected %q to be reserved under custom prefix", want)
		}
	}
	// extra_* are NOT reserved under the acme deployment.
	for _, pc := range PrivateClaims {
		stale := "extra_" + pc.LogicalName
		if _, ok := got[stale]; ok {
			t.Errorf("%q must NOT be reserved when prefix=acme", stale)
		}
	}
}

func TestEmittedName(t *testing.T) {
	cases := []struct {
		prefix, logical, want string
	}{
		{"extra", "domain", "extra_domain"},
		{"acme", "project", "acme_project"},
		{"acme", "service_account", "acme_service_account"},
		{"x", "domain", "x_domain"},
	}
	for _, c := range cases {
		if got := EmittedName(c.prefix, c.logical); got != c.want {
			t.Errorf("EmittedName(%q, %q) = %q, want %q",
				c.prefix, c.logical, got, c.want)
		}
	}
}

func TestValidateExtraClaims(t *testing.T) {
	reserved := BuildReservedClaimKeys("extra")
	tests := []struct {
		name    string
		input   map[string]any
		wantErr bool
	}{
		{name: "nil map", input: nil, wantErr: false},
		{name: "empty map", input: map[string]any{}, wantErr: false},
		{
			name:    "all custom keys",
			input:   map[string]any{"tenant": "acme", "request_id": "abc"},
			wantErr: false,
		},
		{
			name:    "rejects iss",
			input:   map[string]any{"tenant": "acme", "iss": "evil"},
			wantErr: true,
		},
		{
			name:    "rejects sub",
			input:   map[string]any{"sub": "user-2"},
			wantErr: true,
		},
		{
			name:    "rejects prefixed project",
			input:   map[string]any{EmittedName("extra", "project"): "fake"},
			wantErr: true,
		},
		{
			name:    "rejects prefixed domain",
			input:   map[string]any{EmittedName("extra", "domain"): "evil"},
			wantErr: true,
		},
		{
			name:    "rejects bare project (legacy-name impersonation guard)",
			input:   map[string]any{"project": "user-set"},
			wantErr: true,
		},
		{
			name:    "rejects bare domain (legacy-name impersonation guard)",
			input:   map[string]any{"domain": "evil"},
			wantErr: true,
		},
		{
			name:    "rejects bare service_account (legacy-name impersonation guard)",
			input:   map[string]any{"service_account": "evil"},
			wantErr: true,
		},
		{
			name:    "rejects empty key",
			input:   map[string]any{"": "v"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateExtraClaims(tt.input, reserved)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !errors.Is(err, ErrReservedClaimKey) {
					t.Fatalf("expected ErrReservedClaimKey, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
		})
	}
}
