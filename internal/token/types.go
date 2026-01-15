package token

import "time"

// TokenResult represents the result of token generation
type TokenResult struct {
	TokenString string         // The JWT string
	TokenType   string         // "Bearer"
	ExpiresAt   time.Time      // Token expiration time
	Claims      map[string]any // Additional claims from provider
	Success     bool           // Generation success status
}

// TokenValidationResult represents the result of token verification
type TokenValidationResult struct {
	Valid     bool
	UserID    string
	ClientID  string
	Scopes    string
	ExpiresAt time.Time
	Claims    map[string]any
}
