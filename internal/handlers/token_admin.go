package handlers

import (
	"context"
	"net/http"
	"net/url"

	"github.com/go-authgate/authgate/internal/middleware"
	"github.com/go-authgate/authgate/internal/services"
	"github.com/go-authgate/authgate/internal/templates"

	"github.com/gin-gonic/gin"
)

const adminTokensPath = "/admin/tokens"

type TokenAdminHandler struct {
	tokenService *services.TokenService
}

func NewTokenAdminHandler(ts *services.TokenService) *TokenAdminHandler {
	return &TokenAdminHandler{tokenService: ts}
}

func (h *TokenAdminHandler) ShowTokensPage(c *gin.Context) {
	user := getUserFromContext(c)
	if user == nil {
		renderErrorPage(c, http.StatusUnauthorized, "User not authenticated")
		return
	}

	params := parseTokenPaginationParams(c)

	tokens, pagination, err := h.tokenService.ListAllTokensPaginated(params)
	if err != nil {
		renderErrorPage(c, http.StatusInternalServerError, "Failed to retrieve tokens")
		return
	}

	templates.RenderTempl(c, http.StatusOK, templates.AdminTokens(templates.TokensPageProps{
		BaseProps:      templates.BaseProps{CSRFToken: middleware.GetCSRFToken(c)},
		NavbarProps:    buildNavbarProps(c, user, "tokens"),
		Tokens:         tokens,
		Pagination:     pagination,
		Search:         params.Search,
		PageSize:       params.PageSize,
		StatusFilter:   params.StatusFilter,
		CategoryFilter: params.CategoryFilter,
		Success:        c.Query("success"),
	}))
}

// tokenAction extracts tokenID + userID, calls the service method, and
// redirects back to the token list with a success or error message.
func (h *TokenAdminHandler) tokenAction(
	c *gin.Context,
	action func(ctx context.Context, tokenID, userID string) error,
	businessErr error,
	businessMsg, successMsg string,
) {
	tokenID := c.Param("id")
	if tokenID == "" {
		renderErrorPage(c, http.StatusBadRequest, "Token ID is required")
		return
	}

	userID := getUserIDFromContext(c)

	if err := action(c.Request.Context(), tokenID, userID); err != nil {
		if businessErr != nil && err == businessErr {
			c.Redirect(http.StatusFound,
				adminTokensPath+"?success="+url.QueryEscape(businessMsg))
			return
		}
		renderErrorPage(c, http.StatusInternalServerError, "Failed to update token")
		return
	}

	c.Redirect(http.StatusFound,
		adminTokensPath+"?success="+url.QueryEscape(successMsg))
}

func (h *TokenAdminHandler) RevokeToken(c *gin.Context) {
	h.tokenAction(c,
		h.tokenService.RevokeTokenByID, nil, "",
		"Token revoked successfully")
}

func (h *TokenAdminHandler) DisableToken(c *gin.Context) {
	h.tokenAction(c,
		h.tokenService.DisableToken,
		services.ErrTokenCannotDisable,
		"Token cannot be disabled (only active tokens can be disabled)",
		"Token disabled successfully")
}

func (h *TokenAdminHandler) EnableToken(c *gin.Context) {
	h.tokenAction(c,
		h.tokenService.EnableToken,
		services.ErrTokenCannotEnable,
		"Token cannot be enabled (only disabled tokens can be re-enabled)",
		"Token enabled successfully")
}
