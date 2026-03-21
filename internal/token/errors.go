package token

import "errors"

var (
	// ErrTokenGeneration indicates token generation failed
	ErrTokenGeneration = errors.New("failed to generate token")

	// ErrInvalidToken indicates the token is invalid
	ErrInvalidToken = errors.New("invalid token")

	// ErrExpiredToken indicates the token has expired
	ErrExpiredToken = errors.New("token expired")

	// Refresh token specific errors

	// ErrInvalidRefreshToken indicates the refresh token is invalid
	ErrInvalidRefreshToken = errors.New("invalid refresh token")

	// ErrExpiredRefreshToken indicates the refresh token has expired
	ErrExpiredRefreshToken = errors.New("refresh token expired")

	// ErrInvalidScope indicates scope validation failed
	ErrInvalidScope = errors.New("invalid scope")
)
