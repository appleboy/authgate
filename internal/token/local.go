package token

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-authgate/authgate/internal/config"
	"github.com/go-authgate/authgate/internal/core"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var _ core.TokenProvider = (*LocalTokenProvider)(nil)

// LocalTokenProvider generates and validates JWT tokens locally
type LocalTokenProvider struct {
	config *config.Config
}

// NewLocalTokenProvider creates a new local token provider
func NewLocalTokenProvider(cfg *config.Config) *LocalTokenProvider {
	return &LocalTokenProvider{config: cfg}
}

// generateJWT creates a signed JWT token with the given claims and expiration
func (p *LocalTokenProvider) generateJWT(
	userID, clientID, scopes, tokenType string,
	expiresAt time.Time,
) (*Result, error) {
	claims := jwt.MapClaims{
		"user_id":   userID,
		"client_id": clientID,
		"scope":     scopes,
		"type":      tokenType,
		"exp":       expiresAt.Unix(),
		"iat":       time.Now().Unix(),
		"iss":       p.config.BaseURL,
		"sub":       userID,
		"jti":       uuid.New().String(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(p.config.JWTSecret))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTokenGeneration, err)
	}

	return &Result{
		TokenString: tokenString,
		TokenType:   TokenTypeBearer,
		ExpiresAt:   expiresAt,
		Claims:      claims,
	}, nil
}

// ParseJWT parses a JWT token, verifies its signature, and extracts standard claims.
// It does not check the "type" claim — callers (ValidateToken, ValidateRefreshToken)
// add their own type-specific checks on top.
func (p *LocalTokenProvider) ParseJWT(tokenString string) (*ValidationResult, error) {
	tok, err := jwt.Parse(tokenString, p.keyFunc)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	if !tok.Valid {
		return nil, ErrInvalidToken
	}

	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidToken
	}

	userID, _ := claims["user_id"].(string)
	clientID, _ := claims["client_id"].(string)
	scopes, _ := claims["scope"].(string)

	exp, ok := claims["exp"].(float64)
	if !ok {
		return nil, fmt.Errorf("%w: missing exp claim", ErrInvalidToken)
	}

	return &ValidationResult{
		Valid:     true,
		UserID:    userID,
		ClientID:  clientID,
		Scopes:    scopes,
		ExpiresAt: time.Unix(int64(exp), 0),
		Claims:    claims,
	}, nil
}

// mapRefreshError translates base token errors to refresh-specific sentinel errors.
func mapRefreshError(err error) error {
	switch {
	case errors.Is(err, ErrExpiredToken):
		return ErrExpiredRefreshToken
	case errors.Is(err, ErrInvalidToken):
		return ErrInvalidRefreshToken
	default:
		return err
	}
}

// keyFunc validates the signing method and returns the HMAC secret.
func (p *LocalTokenProvider) keyFunc(token *jwt.Token) (any, error) {
	if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
		return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
	}
	return []byte(p.config.JWTSecret), nil
}

// GenerateToken creates a JWT token using local signing
func (p *LocalTokenProvider) GenerateToken(
	ctx context.Context,
	userID, clientID, scopes string,
) (*Result, error) {
	expiresAt := time.Now().Add(p.config.JWTExpiration)
	return p.generateJWT(userID, clientID, scopes, TokenCategoryAccess, expiresAt)
}

// ValidateToken verifies a JWT access token using local verification.
// It rejects refresh tokens (type=="refresh") at the JWT level.
func (p *LocalTokenProvider) ValidateToken(
	ctx context.Context,
	tokenString string,
) (*ValidationResult, error) {
	result, err := p.ParseJWT(tokenString)
	if err != nil {
		return nil, err
	}

	tokenType, _ := result.Claims["type"].(string)
	if tokenType != TokenCategoryAccess {
		return nil, fmt.Errorf("%w: expected access token, got %q", ErrInvalidToken, tokenType)
	}

	return result, nil
}

// Name returns provider name for logging
func (p *LocalTokenProvider) Name() string {
	return "local"
}

// GenerateClientCredentialsToken creates an access token for the client_credentials grant
// using its own configurable expiry (CLIENT_CREDENTIALS_TOKEN_EXPIRATION).
// The userID field carries the synthetic machine identity "client:<clientID>".
func (p *LocalTokenProvider) GenerateClientCredentialsToken(
	ctx context.Context,
	userID, clientID, scopes string,
) (*Result, error) {
	expiresAt := time.Now().Add(p.config.ClientCredentialsTokenExpiration)
	return p.generateJWT(userID, clientID, scopes, TokenCategoryAccess, expiresAt)
}

// GenerateRefreshToken creates a refresh token JWT with longer expiration
func (p *LocalTokenProvider) GenerateRefreshToken(
	ctx context.Context,
	userID, clientID, scopes string,
) (*Result, error) {
	expiresAt := time.Now().Add(p.config.RefreshTokenExpiration)
	return p.generateJWT(userID, clientID, scopes, TokenCategoryRefresh, expiresAt)
}

// ValidateRefreshToken verifies a refresh token JWT
func (p *LocalTokenProvider) ValidateRefreshToken(
	ctx context.Context,
	tokenString string,
) (*ValidationResult, error) {
	result, err := p.ParseJWT(tokenString)
	if err != nil {
		return nil, mapRefreshError(err)
	}

	tokenType, _ := result.Claims["type"].(string)
	if tokenType != TokenCategoryRefresh {
		return nil, fmt.Errorf(
			"%w: expected refresh token, got %q",
			ErrInvalidRefreshToken,
			tokenType,
		)
	}

	return result, nil
}

// RefreshAccessToken generates new access token (and optionally new refresh token in rotation mode)
func (p *LocalTokenProvider) RefreshAccessToken(
	ctx context.Context,
	refreshToken string,
) (*RefreshResult, error) {
	enableRotation := p.config.EnableTokenRotation
	// Validate the refresh token
	validationResult, err := p.ValidateRefreshToken(ctx, refreshToken)
	if err != nil {
		return nil, err
	}

	// Generate new access token
	accessResult, err := p.GenerateToken(
		ctx,
		validationResult.UserID,
		validationResult.ClientID,
		validationResult.Scopes,
	)
	if err != nil {
		return nil, err
	}

	// Note: "type" claim already added in GenerateToken method

	result := &RefreshResult{
		AccessToken: accessResult,
	}

	// Generate new refresh token only in rotation mode
	if enableRotation {
		newRefreshToken, err := p.GenerateRefreshToken(
			ctx,
			validationResult.UserID,
			validationResult.ClientID,
			validationResult.Scopes,
		)
		if err != nil {
			return nil, err
		}
		result.RefreshToken = newRefreshToken
	}

	return result, nil
}
