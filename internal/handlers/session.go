package handlers

import (
	"net/http"

	"github.com/appleboy/authgate/internal/services"

	"github.com/gin-gonic/gin"
)

type SessionHandler struct {
	tokenService *services.TokenService
}

func NewSessionHandler(ts *services.TokenService) *SessionHandler {
	return &SessionHandler{tokenService: ts}
}

// ListSessions shows all active sessions (tokens) for the current user
func (h *SessionHandler) ListSessions(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.HTML(http.StatusUnauthorized, "error.html", gin.H{
			"Error": "User not authenticated",
		})
		return
	}

	tokens, err := h.tokenService.GetUserTokensWithClient(userID.(string))
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"Error": "Failed to retrieve sessions",
		})
		return
	}

	// Get CSRF token from context (set by middleware)
	csrfToken, _ := c.Get("csrf_token")

	c.HTML(http.StatusOK, "account/sessions.html", gin.H{
		"Sessions":   tokens,
		"csrf_token": csrfToken,
	})
}

// validateTokenOwnership checks if the current user owns the specified token
// Returns the tokenID if valid, otherwise renders error and returns empty string
func (h *SessionHandler) validateTokenOwnership(
	c *gin.Context,
	actionName string,
) (tokenID string, valid bool) {
	userIDVal, exists := c.Get("user_id")
	if !exists {
		c.HTML(http.StatusUnauthorized, "error.html", gin.H{
			"Error": "User not authenticated",
		})
		return "", false
	}

	tokenID = c.Param("id")
	if tokenID == "" {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{
			"Error": "Token ID is required",
		})
		return "", false
	}

	userID := userIDVal.(string)

	// Verify that this token belongs to the current user
	tokens, err := h.tokenService.GetUserTokens(userID)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"Error": "Failed to retrieve sessions",
		})
		return "", false
	}

	found := false
	for _, token := range tokens {
		if token.ID == tokenID {
			found = true
			break
		}
	}

	if !found {
		c.HTML(http.StatusForbidden, "error.html", gin.H{
			"Error": "You don't have permission to " + actionName + " this token",
		})
		return "", false
	}

	return tokenID, true
}

// RevokeSession revokes a specific session by token ID
func (h *SessionHandler) RevokeSession(c *gin.Context) {
	tokenID, valid := h.validateTokenOwnership(c, "revoke")
	if !valid {
		return
	}

	// Revoke the token
	if err := h.tokenService.RevokeTokenByID(tokenID); err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"Error": "Failed to revoke session",
		})
		return
	}

	c.Redirect(http.StatusFound, "/account/sessions")
}

// RevokeAllSessions revokes all sessions for the current user
func (h *SessionHandler) RevokeAllSessions(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.HTML(http.StatusUnauthorized, "error.html", gin.H{
			"Error": "User not authenticated",
		})
		return
	}

	if err := h.tokenService.RevokeAllUserTokens(userID.(string)); err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"Error": "Failed to revoke all sessions",
		})
		return
	}

	c.Redirect(http.StatusFound, "/account/sessions")
}

// DisableSession temporarily disables a specific session by token ID
func (h *SessionHandler) DisableSession(c *gin.Context) {
	tokenID, valid := h.validateTokenOwnership(c, "disable")
	if !valid {
		return
	}

	// Disable the token
	if err := h.tokenService.DisableToken(tokenID); err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"Error": "Failed to disable session",
		})
		return
	}

	c.Redirect(http.StatusFound, "/account/sessions")
}

// EnableSession re-enables a previously disabled session by token ID
func (h *SessionHandler) EnableSession(c *gin.Context) {
	tokenID, valid := h.validateTokenOwnership(c, "enable")
	if !valid {
		return
	}

	// Enable the token
	if err := h.tokenService.EnableToken(tokenID); err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"Error": "Failed to enable session",
		})
		return
	}

	c.Redirect(http.StatusFound, "/account/sessions")
}
