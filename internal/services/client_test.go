package services

import (
	"testing"

	"github.com/appleboy/authgate/internal/models"
	"github.com/appleboy/authgate/internal/store"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListClientsPaginatedWithCreator(t *testing.T) {
	s := setupTestStore(t)
	clientService := NewClientService(s, nil)

	// Create test users
	user1 := &models.User{
		ID:           uuid.New().String(),
		Username:     "alice",
		Email:        "alice@example.com",
		PasswordHash: "hashed_password",
		Role:         "user",
	}
	user2 := &models.User{
		ID:           uuid.New().String(),
		Username:     "bob",
		Email:        "bob@example.com",
		PasswordHash: "hashed_password",
		Role:         "user",
	}

	require.NoError(t, s.CreateUser(user1))
	require.NoError(t, s.CreateUser(user2))

	// Create test clients
	client1 := &models.OAuthApplication{
		ClientID:     uuid.New().String(),
		ClientSecret: "secret1",
		ClientName:   "Client 1",
		UserID:       user1.ID, // Created by alice
		GrantTypes:   "device_code",
		Scopes:       "read write",
		IsActive:     true,
	}
	client2 := &models.OAuthApplication{
		ClientID:     uuid.New().String(),
		ClientSecret: "secret2",
		ClientName:   "Client 2",
		UserID:       user2.ID, // Created by bob
		GrantTypes:   "device_code",
		Scopes:       "read",
		IsActive:     true,
	}
	client3 := &models.OAuthApplication{
		ClientID:     uuid.New().String(),
		ClientSecret: "secret3",
		ClientName:   "Client 3",
		UserID:       user1.ID, // Also created by alice
		GrantTypes:   "device_code",
		Scopes:       "write",
		IsActive:     false,
	}
	client4 := &models.OAuthApplication{
		ClientID:     uuid.New().String(),
		ClientSecret: "secret4",
		ClientName:   "Client 4",
		UserID:       "", // No creator (edge case)
		GrantTypes:   "device_code",
		Scopes:       "read",
		IsActive:     true,
	}

	require.NoError(t, s.CreateClient(client1))
	require.NoError(t, s.CreateClient(client2))
	require.NoError(t, s.CreateClient(client3))
	require.NoError(t, s.CreateClient(client4))

	t.Run("returns clients with creator usernames", func(t *testing.T) {
		params := store.NewPaginationParams(1, 10, "")
		clients, pagination, err := clientService.ListClientsPaginatedWithCreator(params)

		require.NoError(t, err)
		// Note: Store creates a default "AuthGate CLI" client, so we have 5 total
		assert.GreaterOrEqual(t, len(clients), 4)
		assert.GreaterOrEqual(t, int(pagination.Total), 4)

		// Find clients by name and verify creator
		clientMap := make(map[string]ClientWithCreator)
		for _, c := range clients {
			clientMap[c.ClientName] = c
		}

		assert.Equal(t, "alice", clientMap["Client 1"].CreatorUsername)
		assert.Equal(t, "bob", clientMap["Client 2"].CreatorUsername)
		assert.Equal(t, "alice", clientMap["Client 3"].CreatorUsername)
		assert.Equal(t, "", clientMap["Client 4"].CreatorUsername) // No creator
	})

	t.Run("handles pagination correctly", func(t *testing.T) {
		params := store.NewPaginationParams(1, 2, "")
		clients, pagination, err := clientService.ListClientsPaginatedWithCreator(params)

		require.NoError(t, err)
		assert.Equal(t, 2, len(clients))
		// Note: Store creates a default "AuthGate CLI" client, so we have 5 total
		assert.GreaterOrEqual(t, int(pagination.Total), 4)
		assert.Equal(t, 1, pagination.CurrentPage)
		assert.GreaterOrEqual(t, pagination.TotalPages, 2)
	})

	t.Run("handles search with creator", func(t *testing.T) {
		params := store.NewPaginationParams(1, 10, "Client 1")
		clients, pagination, err := clientService.ListClientsPaginatedWithCreator(params)

		require.NoError(t, err)
		assert.Equal(t, 1, len(clients))
		assert.Equal(t, "Client 1", clients[0].ClientName)
		assert.Equal(t, "alice", clients[0].CreatorUsername)
		assert.Equal(t, int64(1), pagination.Total)
	})

	t.Run("handles empty results", func(t *testing.T) {
		params := store.NewPaginationParams(1, 10, "NonExistent")
		clients, pagination, err := clientService.ListClientsPaginatedWithCreator(params)

		require.NoError(t, err)
		assert.Equal(t, 0, len(clients))
		assert.Equal(t, int64(0), pagination.Total)
	})

	t.Run("handles deleted user gracefully", func(t *testing.T) {
		// Create a client with a user, then delete the user
		deletedUser := &models.User{
			ID:           uuid.New().String(),
			Username:     "to-be-deleted",
			Email:        "deleted@example.com",
			PasswordHash: "hashed_password",
			Role:         "user",
		}
		require.NoError(t, s.CreateUser(deletedUser))

		clientWithDeletedUser := &models.OAuthApplication{
			ClientID:     uuid.New().String(),
			ClientSecret: "secret5",
			ClientName:   "Client With Deleted User",
			UserID:       deletedUser.ID,
			GrantTypes:   "device_code",
			Scopes:       "read",
			IsActive:     true,
		}
		require.NoError(t, s.CreateClient(clientWithDeletedUser))

		// Delete the user
		require.NoError(t, s.DeleteUser(deletedUser.ID))

		// Fetch clients with creator
		params := store.NewPaginationParams(1, 10, "Client With Deleted User")
		clients, _, err := clientService.ListClientsPaginatedWithCreator(params)

		require.NoError(t, err)
		assert.Equal(t, 1, len(clients))
		assert.Equal(t, "Client With Deleted User", clients[0].ClientName)
		assert.Equal(t, "", clients[0].CreatorUsername) // User deleted, so empty
	})
}

func TestGetUsersByIDs(t *testing.T) {
	s := setupTestStore(t)

	// Create test users
	user1 := &models.User{
		ID:           uuid.New().String(),
		Username:     "user1",
		Email:        "user1@example.com",
		PasswordHash: "hashed_password",
		Role:         "user",
	}
	user2 := &models.User{
		ID:           uuid.New().String(),
		Username:     "user2",
		Email:        "user2@example.com",
		PasswordHash: "hashed_password",
		Role:         "admin",
	}
	user3 := &models.User{
		ID:           uuid.New().String(),
		Username:     "user3",
		Email:        "user3@example.com",
		PasswordHash: "hashed_password",
		Role:         "user",
	}

	require.NoError(t, s.CreateUser(user1))
	require.NoError(t, s.CreateUser(user2))
	require.NoError(t, s.CreateUser(user3))

	t.Run("batch loads multiple users", func(t *testing.T) {
		userIDs := []string{user1.ID, user2.ID, user3.ID}
		userMap, err := s.GetUsersByIDs(userIDs)

		require.NoError(t, err)
		assert.Equal(t, 3, len(userMap))
		assert.Equal(t, "user1", userMap[user1.ID].Username)
		assert.Equal(t, "user2", userMap[user2.ID].Username)
		assert.Equal(t, "user3", userMap[user3.ID].Username)
	})

	t.Run("handles partial matches", func(t *testing.T) {
		nonExistentID := uuid.New().String()
		userIDs := []string{user1.ID, nonExistentID}
		userMap, err := s.GetUsersByIDs(userIDs)

		require.NoError(t, err)
		assert.Equal(t, 1, len(userMap))
		assert.Equal(t, "user1", userMap[user1.ID].Username)
		assert.Nil(t, userMap[nonExistentID])
	})

	t.Run("handles empty input", func(t *testing.T) {
		userMap, err := s.GetUsersByIDs([]string{})

		require.NoError(t, err)
		assert.Equal(t, 0, len(userMap))
	})

	t.Run("handles duplicate IDs efficiently", func(t *testing.T) {
		// Duplicate IDs should still result in single map entry
		userIDs := []string{user1.ID, user1.ID, user2.ID}
		userMap, err := s.GetUsersByIDs(userIDs)

		require.NoError(t, err)
		assert.Equal(t, 2, len(userMap))
		assert.Equal(t, "user1", userMap[user1.ID].Username)
		assert.Equal(t, "user2", userMap[user2.ID].Username)
	})
}
