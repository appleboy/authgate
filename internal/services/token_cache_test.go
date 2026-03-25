package services

import (
	"context"
	"testing"
	"time"

	"github.com/go-authgate/authgate/internal/cache"
	"github.com/go-authgate/authgate/internal/config"
	"github.com/go-authgate/authgate/internal/metrics"
	"github.com/go-authgate/authgate/internal/models"
	"github.com/go-authgate/authgate/internal/store"
	"github.com/go-authgate/authgate/internal/token"
	"github.com/go-authgate/authgate/internal/util"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCachedTokenService(
	t *testing.T,
) (*TokenService, *store.Store, *cache.MemoryCache[models.AccessToken]) {
	t.Helper()
	s := setupTestStore(t)
	memCache := cache.NewMemoryCache[models.AccessToken]()
	cfg := &config.Config{
		JWTExpiration:                    1 * time.Hour,
		ClientCredentialsTokenExpiration: 1 * time.Hour,
		JWTSecret:                        "test-secret",
		BaseURL:                          "http://localhost:8080",
		TokenCacheEnabled:                true,
		TokenCacheTTL:                    5 * time.Minute,
	}
	localProvider, err := token.NewLocalTokenProvider(cfg)
	require.NoError(t, err)
	deviceService := NewDeviceService(s, cfg, nil, metrics.NewNoopMetrics())
	svc := NewTokenService(
		s, cfg, deviceService, localProvider, nil, metrics.NewNoopMetrics(), memCache,
	)
	return svc, s, memCache
}

func TestValidateToken_CacheHit(t *testing.T) {
	svc, s, memCache := newCachedTokenService(t)
	ctx := context.Background()

	// Generate a real JWT token
	result, err := svc.tokenProvider.GenerateToken(ctx, "test-user", "test-client", "read")
	require.NoError(t, err)

	// Store token in DB
	tok := &models.AccessToken{
		ID:            uuid.New().String(),
		TokenHash:     util.SHA256Hex(result.TokenString),
		TokenType:     "Bearer",
		TokenCategory: models.TokenCategoryAccess,
		Status:        models.TokenStatusActive,
		UserID:        "test-user",
		ClientID:      "test-client",
		Scopes:        "read",
		ExpiresAt:     result.ExpiresAt,
	}
	err = s.CreateAccessToken(tok)
	require.NoError(t, err)

	// First call: cache miss, loads from DB
	_, err = svc.ValidateToken(ctx, result.TokenString)
	require.NoError(t, err)

	// Verify token is now in cache
	cached, err := memCache.Get(ctx, util.SHA256Hex(result.TokenString))
	require.NoError(t, err)
	assert.Equal(t, tok.ID, cached.ID)

	// Second call: should succeed (hits cache)
	_, err = svc.ValidateToken(ctx, result.TokenString)
	require.NoError(t, err)
}

func TestValidateToken_CacheInvalidatedOnRevoke(t *testing.T) {
	svc, s, memCache := newCachedTokenService(t)
	ctx := context.Background()

	// Generate a real JWT token
	result, err := svc.tokenProvider.GenerateToken(ctx, "test-user", "test-client", "read")
	require.NoError(t, err)

	// Store token in DB
	tok := &models.AccessToken{
		ID:            uuid.New().String(),
		TokenHash:     util.SHA256Hex(result.TokenString),
		TokenType:     "Bearer",
		TokenCategory: models.TokenCategoryAccess,
		Status:        models.TokenStatusActive,
		UserID:        "test-user",
		ClientID:      "test-client",
		Scopes:        "read",
		ExpiresAt:     result.ExpiresAt,
	}
	err = s.CreateAccessToken(tok)
	require.NoError(t, err)

	// Validate to populate cache
	_, err = svc.ValidateToken(ctx, result.TokenString)
	require.NoError(t, err)

	// Verify cache is populated
	_, err = memCache.Get(ctx, util.SHA256Hex(result.TokenString))
	require.NoError(t, err)

	// Revoke token
	err = svc.RevokeToken(result.TokenString)
	require.NoError(t, err)

	// Verify cache is invalidated
	_, err = memCache.Get(ctx, util.SHA256Hex(result.TokenString))
	assert.Error(t, err, "cache should be invalidated after revocation")
}

func TestValidateToken_CacheInvalidatedOnDisable(t *testing.T) {
	svc, s, memCache := newCachedTokenService(t)
	ctx := context.Background()

	// Generate a real JWT token
	result, err := svc.tokenProvider.GenerateToken(ctx, "test-user", "test-client", "read")
	require.NoError(t, err)

	// Store token in DB
	tok := &models.AccessToken{
		ID:            uuid.New().String(),
		TokenHash:     util.SHA256Hex(result.TokenString),
		TokenType:     "Bearer",
		TokenCategory: models.TokenCategoryAccess,
		Status:        models.TokenStatusActive,
		UserID:        "test-user",
		ClientID:      "test-client",
		Scopes:        "read",
		ExpiresAt:     result.ExpiresAt,
	}
	err = s.CreateAccessToken(tok)
	require.NoError(t, err)

	// Validate to populate cache
	_, err = svc.ValidateToken(ctx, result.TokenString)
	require.NoError(t, err)

	// Disable token
	err = svc.DisableToken(ctx, tok.ID, "admin")
	require.NoError(t, err)

	// Verify cache is invalidated
	_, err = memCache.Get(ctx, util.SHA256Hex(result.TokenString))
	assert.Error(t, err, "cache should be invalidated after disable")
}

func TestValidateToken_NilCache(t *testing.T) {
	// Test that everything works when cache is nil (disabled)
	s := setupTestStore(t)
	cfg := &config.Config{
		JWTExpiration:                    1 * time.Hour,
		ClientCredentialsTokenExpiration: 1 * time.Hour,
		JWTSecret:                        "test-secret",
		BaseURL:                          "http://localhost:8080",
	}
	localProvider, err := token.NewLocalTokenProvider(cfg)
	require.NoError(t, err)
	deviceService := NewDeviceService(s, cfg, nil, metrics.NewNoopMetrics())
	svc := NewTokenService(
		s, cfg, deviceService, localProvider, nil, metrics.NewNoopMetrics(), nil,
	)

	ctx := context.Background()
	result, err := svc.tokenProvider.GenerateToken(ctx, "test-user", "test-client", "read")
	require.NoError(t, err)

	tok := &models.AccessToken{
		ID:            uuid.New().String(),
		TokenHash:     util.SHA256Hex(result.TokenString),
		TokenType:     "Bearer",
		TokenCategory: models.TokenCategoryAccess,
		Status:        models.TokenStatusActive,
		UserID:        "test-user",
		ClientID:      "test-client",
		Scopes:        "read",
		ExpiresAt:     result.ExpiresAt,
	}
	err = s.CreateAccessToken(tok)
	require.NoError(t, err)

	// Should work without cache
	_, err = svc.ValidateToken(ctx, result.TokenString)
	require.NoError(t, err)

	// Revoke should also work without cache
	err = svc.RevokeToken(result.TokenString)
	require.NoError(t, err)
}

func TestValidateToken_CacheExpiredTokenRejected(t *testing.T) {
	svc, s, _ := newCachedTokenService(t)
	ctx := context.Background()

	// Generate a real JWT token
	result, err := svc.tokenProvider.GenerateToken(ctx, "test-user", "test-client", "read")
	require.NoError(t, err)

	// Store token with past expiration
	tok := &models.AccessToken{
		ID:            uuid.New().String(),
		TokenHash:     util.SHA256Hex(result.TokenString),
		TokenType:     "Bearer",
		TokenCategory: models.TokenCategoryAccess,
		Status:        models.TokenStatusActive,
		UserID:        "test-user",
		ClientID:      "test-client",
		Scopes:        "read",
		ExpiresAt:     time.Now().Add(-1 * time.Hour), // Already expired in DB
	}
	err = s.CreateAccessToken(tok)
	require.NoError(t, err)

	// Even if token is cached, expired tokens should be rejected
	_, err = svc.ValidateToken(ctx, result.TokenString)
	assert.Error(t, err, "expired token should be rejected even if cached")
}

func TestRevokeTokenByStatus_CacheInvalidated(t *testing.T) {
	svc, s, memCache := newCachedTokenService(t)
	ctx := context.Background()

	// Generate a real JWT token
	result, err := svc.tokenProvider.GenerateToken(ctx, "test-user", "test-client", "read")
	require.NoError(t, err)

	// Store token in DB
	tok := &models.AccessToken{
		ID:            uuid.New().String(),
		TokenHash:     util.SHA256Hex(result.TokenString),
		TokenType:     "Bearer",
		TokenCategory: models.TokenCategoryAccess,
		Status:        models.TokenStatusActive,
		UserID:        "test-user",
		ClientID:      "test-client",
		Scopes:        "read",
		ExpiresAt:     result.ExpiresAt,
	}
	err = s.CreateAccessToken(tok)
	require.NoError(t, err)

	// Populate cache
	_, err = svc.ValidateToken(ctx, result.TokenString)
	require.NoError(t, err)

	// Verify cache is populated
	_, err = memCache.Get(ctx, util.SHA256Hex(result.TokenString))
	require.NoError(t, err)

	// Revoke by status
	err = svc.RevokeTokenByStatus(tok.ID)
	require.NoError(t, err)

	// Verify cache is invalidated
	_, err = memCache.Get(ctx, util.SHA256Hex(result.TokenString))
	assert.Error(t, err, "cache should be invalidated after RevokeTokenByStatus")
}
