package handlers

import (
	"context"
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/go-authgate/authgate/internal/auth"
	"github.com/go-authgate/authgate/internal/config"
	"github.com/go-authgate/authgate/internal/core"
	"github.com/go-authgate/authgate/internal/middleware"
	"github.com/go-authgate/authgate/internal/services"
	"github.com/go-authgate/authgate/internal/util"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
)

// OAuth-specific session keys.
const (
	sessionOAuthState      = "oauth_state"
	sessionOAuthProvider   = "oauth_provider"
	sessionOAuthRedirect   = "oauth_redirect"
	sessionOAuthRememberMe = "oauth_remember_me"
)

// OAuthHandler handles OAuth authentication
type OAuthHandler struct {
	providers   map[string]*auth.OAuthProvider
	userService *services.UserService
	httpClient  *http.Client // Custom HTTP client for OAuth requests
	cfg         *config.Config
	metrics     core.Recorder
}

// NewOAuthHandler creates a new OAuth handler
func NewOAuthHandler(
	providers map[string]*auth.OAuthProvider,
	userService *services.UserService,
	httpClient *http.Client,
	cfg *config.Config,
	m core.Recorder,
) *OAuthHandler {
	return &OAuthHandler{
		providers:   providers,
		userService: userService,
		httpClient:  httpClient,
		cfg:         cfg,
		metrics:     m,
	}
}

// LoginWithProvider redirects user to OAuth provider
func (h *OAuthHandler) LoginWithProvider(c *gin.Context) {
	provider := c.Param("provider")

	// Check if provider exists
	oauthProvider, exists := h.providers[provider]
	if !exists {
		renderErrorPage(
			c,
			http.StatusBadRequest,
			"Unsupported OAuth provider. The requested OAuth provider is not configured.",
		)
		return
	}

	// Generate state for CSRF protection
	state, err := generateRandomState(32)
	if err != nil {
		log.Printf("[OAuth] Failed to generate state: %v", err)
		renderErrorPage(
			c,
			http.StatusInternalServerError,
			"Internal server error. Failed to initiate OAuth login.",
		)
		return
	}

	// Save state and redirect URL in session
	session := sessions.Default(c)
	session.Set(sessionOAuthState, state)
	session.Set(sessionOAuthProvider, provider)

	redirect := c.Query("redirect")
	if redirect != "" && util.IsRedirectSafe(redirect, h.cfg.BaseURL) {
		session.Set(sessionOAuthRedirect, redirect)
	}

	// Clear first so a previous abandoned attempt cannot silently opt this
	// login into a 30-day session.
	session.Delete(sessionOAuthRememberMe)
	if h.cfg.SessionRememberMeEnabled && c.Query(formFieldRememberMe) == "1" {
		session.Set(sessionOAuthRememberMe, true)
	}

	if err := session.Save(); err != nil {
		log.Printf("[OAuth] Failed to save session: %v", err)
		renderErrorPage(
			c,
			http.StatusInternalServerError,
			"Internal server error. Failed to save session.",
		)
		return
	}

	// Redirect to OAuth provider
	authURL := oauthProvider.GetAuthURL(state)
	c.Redirect(http.StatusTemporaryRedirect, authURL)
}

// OAuthCallback handles OAuth provider callback
func (h *OAuthHandler) OAuthCallback(c *gin.Context) {
	provider := c.Param("provider")
	code := c.Query("code")
	state := c.Query("state")

	// Verify provider exists
	oauthProvider, exists := h.providers[provider]
	if !exists {
		renderErrorPage(c, http.StatusBadRequest, "Invalid provider. OAuth provider not found.")
		return
	}

	if len(state) > maxStateLength {
		renderErrorPage(
			c,
			http.StatusBadRequest,
			"Invalid state parameter. State parameter exceeds maximum length.",
		)
		return
	}

	// Verify state (CSRF protection)
	session := sessions.Default(c)
	savedState := session.Get(sessionOAuthState)
	savedProvider := session.Get(sessionOAuthProvider)

	if savedState == nil || savedProvider == nil {
		renderErrorPage(
			c,
			http.StatusBadRequest,
			"Invalid session. OAuth session expired or invalid. Please try again.",
		)
		return
	}

	savedStateStr, ok1 := savedState.(string)
	savedProviderStr, ok2 := savedProvider.(string)
	if !ok1 || !ok2 || state != savedStateStr || provider != savedProviderStr {
		renderErrorPage(
			c,
			http.StatusBadRequest,
			"Invalid state. CSRF validation failed. Please try again.",
		)
		return
	}

	// Use custom HTTP client for OAuth requests
	ctx := context.WithValue(c.Request.Context(), oauth2.HTTPClient, h.httpClient)

	// Exchange code for token
	token, err := oauthProvider.ExchangeCode(ctx, code)
	if err != nil {
		log.Printf("[OAuth] Failed to exchange code: %v", err)
		renderErrorPage(
			c,
			http.StatusInternalServerError,
			"OAuth error. Failed to exchange authorization code.",
		)
		return
	}

	// Get user info from provider
	userInfo, err := oauthProvider.GetUserInfo(ctx, token)
	if err != nil {
		log.Printf("[OAuth] Failed to get user info: %v", err)
		renderErrorPage(
			c,
			http.StatusInternalServerError,
			"OAuth error. Failed to retrieve user information from provider.",
		)
		return
	}

	// Authenticate or create user
	user, err := h.userService.AuthenticateWithOAuth(
		c.Request.Context(),
		provider,
		userInfo,
		token,
	)
	if err != nil {
		// Record failure
		h.metrics.RecordOAuthCallback(provider, false)

		log.Printf("[OAuth] Authentication failed: %v", err)

		// Handle specific errors
		switch {
		case errors.Is(err, services.ErrOAuthAutoRegisterDisabled):
			renderErrorPage(
				c,
				http.StatusForbidden,
				"Registration Disabled. New account registration via OAuth is currently disabled. Please contact your administrator.",
			)
		case errors.Is(err, services.ErrAccountDisabled):
			renderErrorPage(
				c,
				http.StatusForbidden,
				"Account Disabled. Your account has been disabled by an administrator. Please contact your administrator for assistance.",
			)
		case errors.Is(err, services.ErrAmbiguousEmail):
			renderErrorPage(
				c,
				http.StatusConflict,
				"Duplicate Account Detected. Multiple local accounts share this email address. Please contact your administrator to merge or remove the duplicates before signing in via OAuth.",
			)
		default:
			renderErrorPage(
				c,
				http.StatusInternalServerError,
				"Authentication failed. Unable to authenticate your account at this time. Please try again later.",
			)
		}
		return
	}

	// Record success
	h.metrics.RecordOAuthCallback(provider, true)

	// Clear OAuth session data
	session.Delete(sessionOAuthState)
	session.Delete(sessionOAuthProvider)

	// Save user ID and username in session
	session.Set(middleware.SessionUserID, user.ID)
	session.Set(middleware.SessionUsername, user.Username)
	session.Set(middleware.SessionLastActivity, time.Now().Unix()) // Set initial last activity time

	if remember, _ := session.Get(sessionOAuthRememberMe).(bool); remember &&
		h.cfg.SessionRememberMeEnabled {
		middleware.ApplyRememberMe(session, h.cfg.SessionRememberMeMaxAge, h.cfg.IsProduction)
	}
	session.Delete(sessionOAuthRememberMe)

	// Set session fingerprint if enabled
	if h.cfg.SessionFingerprint {
		clientIP := c.GetString(middleware.ContextKeyClientIP) // Set by RequestContextMiddleware
		userAgent := c.Request.UserAgent()
		fingerprint := middleware.GenerateFingerprint(
			clientIP,
			userAgent,
			h.cfg.SessionFingerprintIP,
		)
		session.Set(middleware.SessionFingerprint, fingerprint)
	}

	// Get redirect URL
	redirectURL := "/account/sessions"
	if savedRedirect := session.Get(sessionOAuthRedirect); savedRedirect != nil {
		redirectURL = savedRedirect.(string)
		session.Delete(sessionOAuthRedirect)
	}

	if err := session.Save(); err != nil {
		log.Printf("[OAuth] Failed to save session: %v", err)
		renderErrorPage(
			c,
			http.StatusInternalServerError,
			"Internal server error. Failed to save session.",
		)
		return
	}

	log.Printf("[OAuth] User authenticated: user=%s provider=%s", user.Username, provider)
	c.Redirect(http.StatusFound, redirectURL)
}

// generateRandomState returns a URL-safe base64-encoded string of nBytes random bytes.
func generateRandomState(nBytes int) (string, error) {
	b, err := util.CryptoRandomBytes(nBytes)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
