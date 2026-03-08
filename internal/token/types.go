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

// Result is an alias for core.TokenResult.
// All existing callers using *token.Result continue to compile unchanged.
type Result = core.TokenResult

// ValidationResult is an alias for core.TokenValidationResult.
type ValidationResult = core.TokenValidationResult

// RefreshResult is an alias for core.TokenRefreshResult.
type RefreshResult = core.TokenRefreshResult
