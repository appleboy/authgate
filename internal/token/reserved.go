package token

import (
	"errors"
	"fmt"
)

// ErrReservedClaimKey is returned when caller-supplied extra claims attempt
// to set a reserved JWT/OIDC standard claim. The issuer owns these claims
// and callers must not provide them via extra_claims.
var ErrReservedClaimKey = errors.New("reserved claim key")

// staticReservedClaimKeys lists the JWT/OIDC standard claim keys plus the
// AuthGate-internal claims that are reserved on every deployment regardless of
// the configured private-claim prefix. Server-attested private claims
// (composed from PrivateClaims + the runtime prefix) are merged on top by
// BuildReservedClaimKeys.
//
// Defence layering:
//  1. Primary — ParseExtraClaims/ValidateExtraClaims reject these keys at the
//     handler edge before the request reaches the token provider.
//  2. Supplementary — generateJWT explicitly overwrites the standard claims it
//     manages (iss/sub/aud/exp/iat/jti/type/scope/user_id/client_id), and
//     drops claims that have no place in an access token: the registered JWT
//     claim nbf and the OIDC ID-token claims (azp/amr/acr/auth_time/nonce/
//     at_hash). This is not a universal override of every entry in this list
//     — server-attested private claims (e.g. extra_domain, extra_project,
//     extra_service_account under the default prefix) are intentionally left
//     alone so the service layer can set them via buildClientClaims /
//     buildServerClaims.
//
// Any change to this list MUST be mirrored in
// config.jwtPrivateClaimStaticReservedKeys so the prefix collision check
// stays accurate.
var staticReservedClaimKeys = []string{
	// RFC 7519 §4.1 registered claim names
	"iss", "sub", "aud", "exp", "nbf", "iat", "jti",

	// AuthGate-internal claims set unconditionally by generateJWT
	"type", "scope", "user_id", "client_id",

	// OIDC ID token standard claims (OIDC Core 1.0 §2)
	"azp", "amr", "acr", "auth_time", "nonce", "at_hash",
}

// BuildReservedClaimKeys returns the set of JWT claim keys that callers must
// not supply via extra_claims for a deployment configured with the given
// private-claim prefix. It includes the static RFC/OIDC/AuthGate-internal
// keys plus the composed `<prefix>_<logical>` key for every entry in
// PrivateClaims.
//
// Build once at parser construction time and reuse — the result is intended
// to be passed into ValidateExtraClaims rather than recomputed per request.
func BuildReservedClaimKeys(prefix string) map[string]struct{} {
	out := make(map[string]struct{}, len(staticReservedClaimKeys)+len(PrivateClaims))
	for _, k := range staticReservedClaimKeys {
		out[k] = struct{}{}
	}
	for _, pc := range PrivateClaims {
		out[EmittedName(prefix, pc.LogicalName)] = struct{}{}
	}
	return out
}

// ValidateExtraClaims rejects empty keys and any key in the supplied reserved
// set. Returns the first violation found; nil for an empty or nil map. The
// reserved set must be supplied by the caller (typically built once via
// BuildReservedClaimKeys at parser construction time). No additional
// key-format validation (length, character set, namespacing) is performed —
// callers that need stricter input rules must layer them on top.
func ValidateExtraClaims(m map[string]any, reserved map[string]struct{}) error {
	for k := range m {
		if k == "" {
			return fmt.Errorf("%w: empty key", ErrReservedClaimKey)
		}
		if _, ok := reserved[k]; ok {
			return fmt.Errorf("%w: %q", ErrReservedClaimKey, k)
		}
	}
	return nil
}
