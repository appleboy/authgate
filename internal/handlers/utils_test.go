package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-authgate/authgate/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestParsePaginationParams(t *testing.T) {
	tests := []struct {
		name         string
		query        string
		wantPage     int
		wantPageSize int
		wantSearch   string
	}{
		{
			name:         "defaults",
			query:        "",
			wantPage:     1,
			wantPageSize: 10,
			wantSearch:   "",
		},
		{
			name:         "custom values",
			query:        "page=3&page_size=25&search=foo",
			wantPage:     3,
			wantPageSize: 25,
			wantSearch:   "foo",
		},
		{
			name:         "invalid page normalized to 1",
			query:        "page=abc",
			wantPage:     1,
			wantPageSize: 10,
			wantSearch:   "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request, _ = http.NewRequestWithContext(
				context.Background(),
				http.MethodGet,
				"/?"+tc.query,
				nil,
			)

			params := parsePaginationParams(c)
			assert.Equal(t, tc.wantPage, params.Page)
			assert.Equal(t, tc.wantPageSize, params.PageSize)
			assert.Equal(t, tc.wantSearch, params.Search)
		})
	}
}

func TestRespondOAuthError(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		errorCode   string
		description string
		wantDesc    bool
	}{
		{
			name:        "with description",
			status:      http.StatusBadRequest,
			errorCode:   "invalid_grant",
			description: "Token expired",
			wantDesc:    true,
		},
		{
			name:        "without description",
			status:      http.StatusUnauthorized,
			errorCode:   "invalid_client",
			description: "",
			wantDesc:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request, _ = http.NewRequestWithContext(
				context.Background(),
				http.MethodPost,
				"/",
				nil,
			)

			respondOAuthError(c, tc.status, tc.errorCode, tc.description)

			assert.Equal(t, tc.status, w.Code)

			var body map[string]string
			err := json.Unmarshal(w.Body.Bytes(), &body)
			require.NoError(t, err)
			assert.Equal(t, tc.errorCode, body["error"])

			if tc.wantDesc {
				assert.Equal(t, tc.description, body["error_description"])
			} else {
				_, exists := body["error_description"]
				assert.False(t, exists)
			}
		})
	}
}

func TestGetUserFromContext(t *testing.T) {
	t.Run("user present", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		user := &models.User{Username: "alice"}
		c.Set("user", user)

		got := getUserFromContext(c)
		require.NotNil(t, got)
		assert.Equal(t, "alice", got.Username)
	})

	t.Run("no user", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		got := getUserFromContext(c)
		assert.Nil(t, got)
	})

	t.Run("wrong type", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("user", "not-a-user")

		got := getUserFromContext(c)
		assert.Nil(t, got)
	})
}
