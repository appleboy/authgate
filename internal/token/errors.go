package token

import "errors"

var (
	// ErrTokenGeneration indicates token generation failed
	ErrTokenGeneration = errors.New("failed to generate token")

	// ErrTokenValidation indicates token validation failed
	ErrTokenValidation = errors.New("failed to validate token")

	// ErrInvalidToken indicates the token is invalid
	ErrInvalidToken = errors.New("invalid token")

	// ErrExpiredToken indicates the token has expired
	ErrExpiredToken = errors.New("token expired")

	// HTTP API specific errors

	// ErrHTTPTokenConnection indicates failed connection to token API
	ErrHTTPTokenConnection = errors.New("failed to connect to token API")

	// ErrHTTPTokenAuthFailed indicates token API rejected request
	ErrHTTPTokenAuthFailed = errors.New("token API rejected request")

	// ErrHTTPTokenInvalidResp indicates invalid response from token API
	ErrHTTPTokenInvalidResp = errors.New("invalid response from token API")
)
