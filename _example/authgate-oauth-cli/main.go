package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	retry "github.com/appleboy/go-httpretry"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

var (
	serverURL         string
	clientID          string
	clientSecret      string
	redirectURI       string
	callbackPort      int
	scope             string
	tokenFile         string
	configInitialized bool
	retryClient       *retry.Client

	flagServerURL    *string
	flagClientID     *string
	flagClientSecret *string
	flagRedirectURI  *string
	flagCallbackPort *int
	flagScope        *string
	flagTokenFile    *string
)

const (
	tokenExchangeTimeout     = 10 * time.Second
	tokenVerificationTimeout = 10 * time.Second
	refreshTokenTimeout      = 10 * time.Second
)

func init() {
	_ = godotenv.Load()

	flagServerURL = flag.String(
		"server-url",
		"",
		"OAuth server URL (default: http://localhost:8080 or SERVER_URL env)",
	)
	flagClientID = flag.String("client-id", "", "OAuth client ID (required, or set CLIENT_ID env)")
	flagClientSecret = flag.String(
		"client-secret",
		"",
		"OAuth client secret (confidential clients only; omit for public/PKCE clients)",
	)
	flagRedirectURI = flag.String(
		"redirect-uri",
		"",
		"Redirect URI registered with the OAuth server (default: http://localhost:CALLBACK_PORT/callback)",
	)
	flagCallbackPort = flag.Int(
		"port",
		0,
		"Local port for the callback server (default: 8888 or CALLBACK_PORT env)",
	)
	flagScope = flag.String("scope", "", "Space-separated OAuth scopes (default: \"read write\")")
	flagTokenFile = flag.String(
		"token-file",
		"",
		"Token storage file (default: .authgate-tokens.json or TOKEN_FILE env)",
	)
}

// initConfig parses flags and initializes all configuration.
func initConfig() {
	if configInitialized {
		return
	}
	configInitialized = true

	flag.Parse()

	serverURL = getConfig(*flagServerURL, "SERVER_URL", "http://localhost:8080")
	clientID = getConfig(*flagClientID, "CLIENT_ID", "")
	clientSecret = getConfig(*flagClientSecret, "CLIENT_SECRET", "")
	scope = getConfig(*flagScope, "SCOPE", "read write")
	tokenFile = getConfig(*flagTokenFile, "TOKEN_FILE", ".authgate-tokens.json")

	// Resolve callback port (int flag needs special handling).
	portStr := ""
	if *flagCallbackPort != 0 {
		portStr = fmt.Sprintf("%d", *flagCallbackPort)
	}
	portStr = getConfig(portStr, "CALLBACK_PORT", "8888")
	if _, err := fmt.Sscanf(portStr, "%d", &callbackPort); err != nil || callbackPort <= 0 {
		callbackPort = 8888
	}

	// Resolve redirect URI (default depends on port, so compute after port is known).
	defaultRedirectURI := fmt.Sprintf("http://localhost:%d/callback", callbackPort)
	redirectURI = getConfig(*flagRedirectURI, "REDIRECT_URI", defaultRedirectURI)

	// Validate SERVER_URL.
	if err := validateServerURL(serverURL); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Invalid SERVER_URL: %v\n", err)
		os.Exit(1)
	}

	if strings.HasPrefix(strings.ToLower(serverURL), "http://") {
		fmt.Fprintln(
			os.Stderr,
			"WARNING: Using HTTP instead of HTTPS. Tokens will be transmitted in plaintext!",
		)
		fmt.Fprintln(
			os.Stderr,
			"WARNING: This is only safe for local development. Use HTTPS in production.",
		)
		fmt.Fprintln(os.Stderr)
	}

	if clientID == "" {
		fmt.Println("Error: CLIENT_ID not set. Please provide it via:")
		fmt.Println("  1. Command-line flag: -client-id=<your-client-id>")
		fmt.Println("  2. Environment variable: CLIENT_ID=<your-client-id>")
		fmt.Println("  3. .env file: CLIENT_ID=<your-client-id>")
		fmt.Println("\nYou can find the client_id in the server startup logs.")
		os.Exit(1)
	}

	if _, err := uuid.Parse(clientID); err != nil {
		fmt.Fprintf(
			os.Stderr,
			"WARNING: CLIENT_ID doesn't appear to be a valid UUID: %s\n",
			clientID,
		)
		fmt.Fprintln(os.Stderr)
	}

	// Build HTTP client with TLS and retry support.
	baseHTTPClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
			MaxIdleConns:        10,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
		},
	}

	var err error
	retryClient, err = retry.NewBackgroundClient(retry.WithHTTPClient(baseHTTPClient))
	if err != nil {
		panic(fmt.Sprintf("failed to create retry client: %v", err))
	}
}

func getConfig(flagValue, envKey, defaultValue string) string {
	if flagValue != "" {
		return flagValue
	}
	return getEnv(envKey, defaultValue)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func validateServerURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("server URL cannot be empty")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL scheme must be http or https, got: %s", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("URL must include a host")
	}
	return nil
}

// isPublicClient returns true when no client secret is configured —
// i.e., this is a public client that must use PKCE.
func isPublicClient() bool {
	return clientSecret == ""
}

// -----------------------------------------------------------------------
// Token storage
// -----------------------------------------------------------------------

// ErrorResponse is an OAuth error payload.
type ErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// ErrRefreshTokenExpired indicates the refresh token has expired or is invalid.
var ErrRefreshTokenExpired = fmt.Errorf("refresh token expired or invalid")

// TokenStorage holds persisted OAuth tokens for one client.
type TokenStorage struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
	ClientID     string    `json:"client_id"`
}

// TokenStorageMap manages tokens for multiple clients in one file.
type TokenStorageMap struct {
	Tokens map[string]*TokenStorage `json:"tokens"`
}

func loadTokens() (*TokenStorage, error) {
	data, err := os.ReadFile(tokenFile)
	if err != nil {
		return nil, err
	}
	var storageMap TokenStorageMap
	if err := json.Unmarshal(data, &storageMap); err != nil {
		return nil, fmt.Errorf("failed to parse token file: %w", err)
	}
	if storageMap.Tokens == nil {
		return nil, fmt.Errorf("no tokens in file")
	}
	if storage, ok := storageMap.Tokens[clientID]; ok {
		return storage, nil
	}
	return nil, fmt.Errorf("no tokens found for client_id: %s", clientID)
}

func saveTokens(storage *TokenStorage) error {
	if storage.ClientID == "" {
		storage.ClientID = clientID
	}

	lock, err := acquireFileLock(tokenFile)
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer lock.release()

	var storageMap TokenStorageMap
	if existing, err := os.ReadFile(tokenFile); err == nil {
		if unmarshalErr := json.Unmarshal(existing, &storageMap); unmarshalErr != nil {
			storageMap.Tokens = make(map[string]*TokenStorage)
		}
	}
	if storageMap.Tokens == nil {
		storageMap.Tokens = make(map[string]*TokenStorage)
	}

	storageMap.Tokens[storage.ClientID] = storage

	data, err := json.MarshalIndent(storageMap, "", "  ")
	if err != nil {
		return err
	}

	tempFile := tokenFile + ".tmp"
	if err := os.WriteFile(tempFile, data, 0o600); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := os.Rename(tempFile, tokenFile); err != nil {
		if removeErr := os.Remove(tempFile); removeErr != nil {
			return fmt.Errorf(
				"failed to rename temp file: %v; also failed to remove temp file: %w",
				err,
				removeErr,
			)
		}
		return fmt.Errorf("failed to rename temp file: %w", err)
	}
	return nil
}

// validateTokenResponse performs basic sanity checks on a token response.
func validateTokenResponse(accessToken, tokenType string, expiresIn int) error {
	if accessToken == "" {
		return fmt.Errorf("access_token is empty")
	}
	if len(accessToken) < 10 {
		return fmt.Errorf("access_token is too short (length: %d)", len(accessToken))
	}
	if expiresIn <= 0 {
		return fmt.Errorf("expires_in must be positive, got: %d", expiresIn)
	}
	if tokenType != "" && tokenType != "Bearer" {
		return fmt.Errorf("unexpected token_type: %s (expected Bearer)", tokenType)
	}
	return nil
}

// -----------------------------------------------------------------------
// Authorization Code Flow
// -----------------------------------------------------------------------

// performAuthCodeFlow runs the full Authorization Code Flow and returns
// tokens on success.
func performAuthCodeFlow() (*TokenStorage, error) {
	// Generate CSRF state.
	state, err := generateState()
	if err != nil {
		return nil, fmt.Errorf("failed to generate state: %w", err)
	}

	// Generate PKCE params (always — even for confidential clients it adds security).
	pkce, err := GeneratePKCE()
	if err != nil {
		return nil, fmt.Errorf("failed to generate PKCE: %w", err)
	}

	// Build authorization URL.
	authURL := buildAuthURL(state, pkce)

	fmt.Println("Step 1: Opening authorization URL in your browser...")
	fmt.Printf("\n  %s\n\n", authURL)

	if err := openBrowser(authURL); err != nil {
		fmt.Println("Could not open browser automatically. Please open the URL above manually.")
	} else {
		fmt.Println("Browser opened. Please complete authorization in your browser.")
	}

	// Start local callback server and wait for the code.
	fmt.Printf("Step 2: Waiting for callback on http://localhost:%d/callback ...\n", callbackPort)
	code, err := startCallbackServer(callbackPort, state)
	if err != nil {
		return nil, fmt.Errorf("authorization failed: %w", err)
	}
	fmt.Println("Authorization code received!")

	// Exchange code for tokens.
	fmt.Println("Step 3: Exchanging authorization code for tokens...")
	storage, err := exchangeCode(code, pkce.Verifier)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}

	if err := saveTokens(storage); err != nil {
		fmt.Printf("Warning: Failed to save tokens: %v\n", err)
	} else {
		fmt.Printf("Tokens saved to %s\n", tokenFile)
	}

	return storage, nil
}

// buildAuthURL constructs the /oauth/authorize URL with all required parameters.
func buildAuthURL(state string, pkce *PKCEParams) string {
	params := url.Values{}
	params.Set("client_id", clientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("response_type", "code")
	params.Set("scope", scope)
	params.Set("state", state)
	params.Set("code_challenge", pkce.Challenge)
	params.Set("code_challenge_method", pkce.Method)

	return serverURL + "/oauth/authorize?" + params.Encode()
}

// exchangeCode exchanges an authorization code for access + refresh tokens.
func exchangeCode(code, codeVerifier string) (*TokenStorage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), tokenExchangeTimeout)
	defer cancel()

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	data.Set("client_id", clientID)

	if isPublicClient() {
		// Public client: send code_verifier for PKCE verification.
		data.Set("code_verifier", codeVerifier)
	} else {
		// Confidential client: send client_secret (and also verifier for PKCE).
		data.Set("client_secret", clientSecret)
		data.Set("code_verifier", codeVerifier)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		serverURL+"/oauth/token",
		strings.NewReader(data.Encode()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := retryClient.Do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		if jsonErr := json.Unmarshal(body, &errResp); jsonErr == nil && errResp.Error != "" {
			return nil, fmt.Errorf("%s: %s", errResp.Error, errResp.ErrorDescription)
		}
		return nil, fmt.Errorf(
			"token exchange failed with status %d: %s",
			resp.StatusCode,
			string(body),
		)
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	if err := validateTokenResponse(
		tokenResp.AccessToken,
		tokenResp.TokenType,
		tokenResp.ExpiresIn,
	); err != nil {
		return nil, fmt.Errorf("invalid token response: %w", err)
	}

	return &TokenStorage{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		ClientID:     clientID,
	}, nil
}

// -----------------------------------------------------------------------
// Token refresh
// -----------------------------------------------------------------------

func refreshAccessToken(refreshToken string) (*TokenStorage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), refreshTokenTimeout)
	defer cancel()

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)
	data.Set("client_id", clientID)
	if !isPublicClient() {
		data.Set("client_secret", clientSecret)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		serverURL+"/oauth/token",
		strings.NewReader(data.Encode()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := retryClient.Do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		if jsonErr := json.Unmarshal(body, &errResp); jsonErr == nil {
			if errResp.Error == "invalid_grant" || errResp.Error == "invalid_token" {
				return nil, ErrRefreshTokenExpired
			}
			return nil, fmt.Errorf("%s: %s", errResp.Error, errResp.ErrorDescription)
		}
		return nil, fmt.Errorf("refresh failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	if err := validateTokenResponse(
		tokenResp.AccessToken,
		tokenResp.TokenType,
		tokenResp.ExpiresIn,
	); err != nil {
		return nil, fmt.Errorf("invalid token response: %w", err)
	}

	// Preserve the old refresh token in fixed-mode (server may not return a new one).
	newRefreshToken := tokenResp.RefreshToken
	if newRefreshToken == "" {
		newRefreshToken = refreshToken
	}

	storage := &TokenStorage{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: newRefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		ClientID:     clientID,
	}

	if err := saveTokens(storage); err != nil {
		fmt.Printf("Warning: Failed to save refreshed tokens: %v\n", err)
	}
	return storage, nil
}

// -----------------------------------------------------------------------
// Token verification / API demo
// -----------------------------------------------------------------------

func verifyToken(accessToken string) error {
	ctx, cancel := context.WithTimeout(context.Background(), tokenVerificationTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/oauth/tokeninfo", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := retryClient.Do(ctx, req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		if jsonErr := json.Unmarshal(body, &errResp); jsonErr == nil {
			return fmt.Errorf("%s: %s", errResp.Error, errResp.ErrorDescription)
		}
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	fmt.Printf("Token Info: %s\n", string(body))
	return nil
}

// makeAPICallWithAutoRefresh demonstrates the 401 → refresh → retry pattern.
func makeAPICallWithAutoRefresh(storage *TokenStorage) error {
	req, err := http.NewRequest(http.MethodGet, serverURL+"/oauth/tokeninfo", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+storage.AccessToken)

	resp, err := retryClient.Do(context.Background(), req)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		fmt.Println("Access token rejected (401), refreshing...")

		newStorage, err := refreshAccessToken(storage.RefreshToken)
		if err != nil {
			if err == ErrRefreshTokenExpired {
				return ErrRefreshTokenExpired
			}
			return fmt.Errorf("refresh failed: %w", err)
		}

		storage.AccessToken = newStorage.AccessToken
		storage.RefreshToken = newStorage.RefreshToken
		storage.ExpiresAt = newStorage.ExpiresAt

		fmt.Println("Token refreshed, retrying API call...")

		req, err = http.NewRequest(http.MethodGet, serverURL+"/oauth/tokeninfo", nil)
		if err != nil {
			return fmt.Errorf("failed to create retry request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+storage.AccessToken)

		resp, err = retryClient.Do(context.Background(), req)
		if err != nil {
			return fmt.Errorf("retry failed: %w", err)
		}
		defer resp.Body.Close()
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API call failed with status %d: %s", resp.StatusCode, string(body))
	}

	fmt.Println("API call successful!")
	return nil
}

// -----------------------------------------------------------------------
// main
// -----------------------------------------------------------------------

func main() {
	initConfig()

	clientMode := "public (PKCE)"
	if !isPublicClient() {
		clientMode = "confidential"
	}
	fmt.Printf("=== OAuth 2.0 Authorization Code Flow CLI Demo ===\n")
	fmt.Printf("Client mode : %s\n", clientMode)
	fmt.Printf("Server URL  : %s\n", serverURL)
	fmt.Printf("Client ID   : %s\n", clientID)
	fmt.Println()

	var storage *TokenStorage

	// Try to reuse or refresh existing tokens.
	existing, err := loadTokens()
	if err == nil && existing != nil {
		fmt.Println("Found existing tokens.")
		if time.Now().Before(existing.ExpiresAt) {
			fmt.Println("Access token is still valid, using it.")
			storage = existing
		} else {
			fmt.Println("Access token expired, attempting refresh...")
			newStorage, err := refreshAccessToken(existing.RefreshToken)
			if err != nil {
				fmt.Printf("Refresh failed: %v\n", err)
				fmt.Println("Starting new authorization flow...")
			} else {
				storage = newStorage
				fmt.Println("Token refreshed successfully.")
			}
		}
	} else {
		fmt.Println("No existing tokens found, starting Authorization Code Flow...")
	}

	// No valid tokens — start the full flow.
	if storage == nil {
		storage, err = performAuthCodeFlow()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Authorization failed: %v\n", err)
			os.Exit(1)
		}
	}

	// Display token info.
	fmt.Printf("\n========================================\n")
	fmt.Printf("Current Token Info:\n")
	preview := storage.AccessToken
	if len(preview) > 50 {
		preview = preview[:50]
	}
	fmt.Printf("Access Token : %s...\n", preview)
	fmt.Printf("Token Type   : %s\n", storage.TokenType)
	fmt.Printf("Expires In   : %s\n", time.Until(storage.ExpiresAt).Round(time.Second))
	fmt.Printf("========================================\n")

	// Verify token against server.
	fmt.Println("\nVerifying token with server...")
	if err := verifyToken(storage.AccessToken); err != nil {
		fmt.Printf("Token verification failed: %v\n", err)
	} else {
		fmt.Println("Token verified successfully.")
	}

	// Demonstrate auto-refresh on 401.
	fmt.Println("\nDemonstrating automatic refresh on API call...")
	if err := makeAPICallWithAutoRefresh(storage); err != nil {
		if err == ErrRefreshTokenExpired {
			fmt.Println("Refresh token expired, re-authenticating...")
			storage, err = performAuthCodeFlow()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Re-authentication failed: %v\n", err)
				os.Exit(1)
			}
			if err := makeAPICallWithAutoRefresh(storage); err != nil {
				fmt.Fprintf(os.Stderr, "API call failed after re-authentication: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("API call successful after re-authentication.")
		} else {
			fmt.Fprintf(os.Stderr, "API call failed: %v\n", err)
		}
	}
}
