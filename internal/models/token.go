package models

import (
	"time"
)

// TokenStatus represents the lifecycle state of an access or refresh token.
type TokenStatus = string

const (
	TokenStatusActive   TokenStatus = "active"
	TokenStatusDisabled TokenStatus = "disabled"
	TokenStatusRevoked  TokenStatus = "revoked"
)

// TokenCategory distinguishes access tokens from refresh tokens.
type TokenCategory = string

const (
	TokenCategoryAccess  TokenCategory = "access"
	TokenCategoryRefresh TokenCategory = "refresh"
)

type AccessToken struct {
	ID              string `gorm:"primaryKey"`
	TokenHash       string `gorm:"uniqueIndex;not null"`
	RawToken        string `gorm:"-"` // In-memory only; never persisted to DB
	TokenType       string `gorm:"not null;default:'Bearer'"`
	TokenCategory   string `gorm:"not null;default:'access';index"` // 'access' or 'refresh'
	Status          string `gorm:"not null;default:'active';index"` // 'active', 'disabled', 'revoked'
	UserID          string `gorm:"not null;index"`
	ClientID        string `gorm:"not null;index"`
	Scopes          string `gorm:"not null"` // space-separated scopes
	ExpiresAt       time.Time
	CreatedAt       time.Time
	LastUsedAt      *time.Time `gorm:"index"`                     // Last time token was used (for refresh tokens)
	ParentTokenID   string     `gorm:"index"`                     // Links access tokens to their refresh token
	TokenFamilyID   string     `gorm:"index;default:'';not null"` // Stable root ID for rotation replay detection
	AuthorizationID *uint      `gorm:"index"`                     // FK → UserAuthorization.ID (nil for device_code grants)
}

func (t *AccessToken) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

func (t *AccessToken) IsActive() bool {
	return t.Status == TokenStatusActive
}

func (t *AccessToken) IsRevoked() bool {
	return t.Status == TokenStatusRevoked
}

func (t *AccessToken) IsDisabled() bool {
	return t.Status == TokenStatusDisabled
}

func (t *AccessToken) IsAccessToken() bool {
	return t.TokenCategory == TokenCategoryAccess
}

func (t *AccessToken) IsRefreshToken() bool {
	return t.TokenCategory == TokenCategoryRefresh
}
