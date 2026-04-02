package store

import (
	"testing"
	"time"

	"github.com/go-authgate/authgate/internal/models"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTokensPaginated(t *testing.T) {
	store := createFreshStore(t, "sqlite", nil)

	// Create a test user and client
	user := &models.User{
		ID:       uuid.New().String(),
		Username: "tokenadmin",
		Email:    "tokenadmin@test.com",
		Role:     models.UserRoleUser,
	}
	require.NoError(t, store.CreateUser(user))

	client := &models.OAuthApplication{
		ClientID:   uuid.New().String(),
		ClientName: "TestApp",
		UserID:     user.ID,
		Status:     models.ClientStatusActive,
	}
	require.NoError(t, store.CreateClient(client))

	// Create some tokens
	for range 5 {
		tok := &models.AccessToken{
			ID:            uuid.New().String(),
			TokenHash:     uuid.New().String(),
			TokenType:     "Bearer",
			TokenCategory: models.TokenCategoryAccess,
			Status:        models.TokenStatusActive,
			UserID:        user.ID,
			ClientID:      client.ClientID,
			Scopes:        "openid profile",
			ExpiresAt:     time.Now().Add(1 * time.Hour),
		}
		require.NoError(t, store.CreateAccessToken(tok))
	}

	// Create a refresh token
	refreshTok := &models.AccessToken{
		ID:            uuid.New().String(),
		TokenHash:     uuid.New().String(),
		TokenType:     "Bearer",
		TokenCategory: models.TokenCategoryRefresh,
		Status:        models.TokenStatusActive,
		UserID:        user.ID,
		ClientID:      client.ClientID,
		Scopes:        "openid profile",
		ExpiresAt:     time.Now().Add(24 * time.Hour),
	}
	require.NoError(t, store.CreateAccessToken(refreshTok))

	t.Run("basic pagination", func(t *testing.T) {
		params := NewPaginationParams(1, 10, "")
		tokens, pagination, err := store.GetTokensPaginated(params)
		require.NoError(t, err)
		assert.Equal(t, int64(6), pagination.Total)
		assert.Len(t, tokens, 6)
	})

	t.Run("with page size", func(t *testing.T) {
		params := NewPaginationParams(1, 3, "")
		tokens, pagination, err := store.GetTokensPaginated(params)
		require.NoError(t, err)
		assert.Len(t, tokens, 3)
		assert.Equal(t, int64(6), pagination.Total)
		assert.True(t, pagination.HasNext)
	})

	t.Run("status filter", func(t *testing.T) {
		params := NewPaginationParams(1, 10, "")
		params.StatusFilter = models.TokenStatusActive
		tokens, _, err := store.GetTokensPaginated(params)
		require.NoError(t, err)
		assert.Len(t, tokens, 6) // all are active
	})

	t.Run("category filter", func(t *testing.T) {
		params := NewPaginationParams(1, 10, "")
		params.CategoryFilter = models.TokenCategoryRefresh
		tokens, _, err := store.GetTokensPaginated(params)
		require.NoError(t, err)
		assert.Len(t, tokens, 1) // only the refresh token
	})

	t.Run("search by client name", func(t *testing.T) {
		params := NewPaginationParams(1, 10, "TestApp")
		tokens, _, err := store.GetTokensPaginated(params)
		require.NoError(t, err)
		assert.Len(t, tokens, 6)
	})

	t.Run("search by username", func(t *testing.T) {
		params := NewPaginationParams(1, 10, "tokenadmin")
		tokens, _, err := store.GetTokensPaginated(params)
		require.NoError(t, err)
		assert.Len(t, tokens, 6)
	})

	t.Run("search no match", func(t *testing.T) {
		params := NewPaginationParams(1, 10, "nonexistent")
		tokens, pagination, err := store.GetTokensPaginated(params)
		require.NoError(t, err)
		assert.Empty(t, tokens)
		assert.Equal(t, int64(0), pagination.Total)
	})
}

func TestGetDashboardCounts(t *testing.T) {
	store := createFreshStore(t, "sqlite", nil)

	// Seeded: 1 admin user, 1 active client
	counts, err := store.GetDashboardCounts()
	require.NoError(t, err)

	// Seeded admin
	assert.Equal(t, int64(1), counts.TotalUsers)
	assert.Equal(t, int64(1), counts.AdminUsers)

	// Seeded client (active)
	assert.GreaterOrEqual(t, counts.TotalClients, int64(1))
	assert.GreaterOrEqual(t, counts.ActiveClients, int64(1))

	// No tokens yet
	assert.Equal(t, int64(0), counts.ActiveAccessTokens)

	// Add a user, client, and token
	user := &models.User{
		ID: uuid.New().
			String(),
		Username: "dc_user",
		Email:    "dc@test.com",
		Role:     models.UserRoleUser,
	}
	require.NoError(t, store.CreateUser(user))

	client := &models.OAuthApplication{
		ClientID: uuid.New().
			String(),
		ClientName: "DCApp",
		UserID:     user.ID,
		Status:     models.ClientStatusPending,
	}
	require.NoError(t, store.CreateClient(client))

	tok := &models.AccessToken{
		ID: uuid.New().String(), TokenHash: uuid.New().String(),
		TokenCategory: models.TokenCategoryAccess, Status: models.TokenStatusActive,
		UserID: user.ID, ClientID: client.ClientID, ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	require.NoError(t, store.CreateAccessToken(tok))

	counts, err = store.GetDashboardCounts()
	require.NoError(t, err)

	assert.Equal(t, int64(2), counts.TotalUsers)
	assert.Equal(t, int64(1), counts.AdminUsers)         // still 1 admin
	assert.Equal(t, int64(1), counts.PendingClients)     // new pending client
	assert.Equal(t, int64(1), counts.ActiveAccessTokens) // new access token
}
