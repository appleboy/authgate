package token

import "github.com/go-authgate/authgate/internal/core"

// Token type constants
const (
	TokenTypeBearer = "Bearer"
)

// Token category constants used in the "type" JWT claim.
const (
	TokenCategoryAccess  = "access"
	TokenCategoryRefresh = "refresh"
)

// PrivateClaim describes a server-attested private JWT claim emitted by
// AuthGate under the configurable prefix (JWT_PRIVATE_CLAIM_PREFIX). The
// emitted key is composed at runtime via EmittedName(prefix, LogicalName).
//
// These claims are AuthGate-internal: their values come either from the
// OAuthApplication row (project, service_account) or from the AuthGate process
// configuration (domain). A signed JWT only proves AuthGate emitted these
// values, not that the named project / service account is actually owned by
// the token holder. Downstream gateways that route or authorize on these
// claims must verify the JWT signature AND apply their own access policies —
// never treat these values as authoritative proof of identity. See
// docs/JWT_VERIFICATION.md for the full trust model.
type PrivateClaim struct {
	// LogicalName is the stable identifier used internally and inside
	// EmittedName composition. It must NOT be changed without coordinating
	// with downstream consumers — the composed emitted key is part of the
	// public token contract.
	LogicalName string
}

// privateClaims is the canonical registry of server-attested private claims
// AuthGate may emit on issued JWTs. The list order is significant only for
// deterministic iteration (tests, validation messages); emission and lookup
// are key-based. Adding a new claim here requires:
//  1. Appending the entry to this slice, AND
//  2. Setting its source in services.buildClientClaims or
//     services.buildServerClaims.
//
// No other wiring sites need to change — reserved-key derivation, parser
// validation, and tests all read from this registry.
//
// Unexported so other packages cannot accidentally append/mutate it at
// runtime — that would silently change reserved-key derivation and claim
// stripping for every subsequent token, including in parallel tests.
// External callers must go through PrivateClaimRegistry which returns a
// defensive copy.
var privateClaims = []PrivateClaim{
	{LogicalName: "domain"},
	{LogicalName: "project"},
	{LogicalName: "service_account"},
}

// PrivateClaimRegistry returns a defensive copy of the server-attested
// private-claim registry. Callers that need to iterate the registry from
// outside this package (tests, downstream tooling) must use this accessor;
// the underlying slice is intentionally unexported to prevent cross-package
// mutation.
func PrivateClaimRegistry() []PrivateClaim {
	out := make([]PrivateClaim, len(privateClaims))
	copy(out, privateClaims)
	return out
}

// EmittedName composes the JWT key written for a private claim:
// "<prefix>_<logical>". AuthGate adds the separating underscore itself; the
// configured prefix must therefore not have a trailing underscore (validated
// at startup in config.validateJWTPrivateClaimPrefix).
//
// Centralizing composition here means every emission site, every reserved-key
// derivation, and every test reads the same source of truth — there is no
// place where someone could forget to add the underscore.
func EmittedName(prefix, logical string) string {
	return prefix + "_" + logical
}

// Result is an alias for core.TokenResult.
// All existing callers using *token.Result continue to compile unchanged.
type Result = core.TokenResult

// ValidationResult is an alias for core.TokenValidationResult.
type ValidationResult = core.TokenValidationResult

// RefreshResult is an alias for core.TokenRefreshResult.
type RefreshResult = core.TokenRefreshResult
