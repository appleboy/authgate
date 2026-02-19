package main

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestValidateServerURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid http", "http://localhost:8080", false},
		{"valid https", "https://auth.example.com", false},
		{"empty", "", true},
		{"no scheme", "localhost:8080", true},
		{"bad scheme", "ftp://example.com", true},
		{"no host", "http://", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateServerURL(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateServerURL(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
			}
		})
	}
}

func TestGetConfig_Priority(t *testing.T) {
	t.Setenv("MYKEY", "from-env")

	// Flag value wins over env.
	if got := getConfig("from-flag", "MYKEY", "default"); got != "from-flag" {
		t.Errorf("expected flag value, got %q", got)
	}

	// Env wins over default when flag is empty.
	if got := getConfig("", "MYKEY", "default"); got != "from-env" {
		t.Errorf("expected env value, got %q", got)
	}

	// Default used when both flag and env are empty.
	t.Setenv("MYKEY", "")
	if got := getConfig("", "MYKEY", "default"); got != "default" {
		t.Errorf("expected default, got %q", got)
	}
}

func TestValidateTokenResponse(t *testing.T) {
	tests := []struct {
		name        string
		accessToken string
		tokenType   string
		expiresIn   int
		wantErr     bool
	}{
		{"valid bearer", "a-long-enough-token", "Bearer", 3600, false},
		{"valid empty type", "a-long-enough-token", "", 3600, false},
		{"empty access token", "", "Bearer", 3600, true},
		{"too short token", "short", "Bearer", 3600, true},
		{"zero expires_in", "a-long-enough-token", "Bearer", 0, true},
		{"negative expires_in", "a-long-enough-token", "Bearer", -1, true},
		{"wrong token type", "a-long-enough-token", "MAC", 3600, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateTokenResponse(tc.accessToken, tc.tokenType, tc.expiresIn)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateTokenResponse() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestSaveAndLoadTokens(t *testing.T) {
	// Use a temp file for token storage.
	tmpFile, err := os.CreateTemp(t.TempDir(), "tokens-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	originalTokenFile := tokenFile
	originalClientID := clientID
	t.Cleanup(func() {
		tokenFile = originalTokenFile
		clientID = originalClientID
	})

	tokenFile = tmpFile.Name()
	clientID = "test-client-id"

	storage := &TokenStorage{
		AccessToken:  "access-token-value",
		RefreshToken: "refresh-token-value",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(time.Hour).UTC().Truncate(time.Second),
		ClientID:     clientID,
	}

	if err := saveTokens(storage); err != nil {
		t.Fatalf("saveTokens() error: %v", err)
	}

	loaded, err := loadTokens()
	if err != nil {
		t.Fatalf("loadTokens() error: %v", err)
	}

	if loaded.AccessToken != storage.AccessToken {
		t.Errorf("AccessToken mismatch: got %q, want %q", loaded.AccessToken, storage.AccessToken)
	}
	if loaded.RefreshToken != storage.RefreshToken {
		t.Errorf(
			"RefreshToken mismatch: got %q, want %q",
			loaded.RefreshToken,
			storage.RefreshToken,
		)
	}
	if loaded.ClientID != storage.ClientID {
		t.Errorf("ClientID mismatch: got %q, want %q", loaded.ClientID, storage.ClientID)
	}
}

func TestSaveTokens_MultipleClients(t *testing.T) {
	tmpFile, err := os.CreateTemp(t.TempDir(), "tokens-multi-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	originalTokenFile := tokenFile
	originalClientID := clientID
	t.Cleanup(func() {
		tokenFile = originalTokenFile
		clientID = originalClientID
	})

	tokenFile = tmpFile.Name()

	// Save tokens for two different clients.
	for _, id := range []string{"client-a", "client-b"} {
		clientID = id
		if err := saveTokens(&TokenStorage{
			AccessToken:  "token-" + id,
			RefreshToken: "refresh-" + id,
			TokenType:    "Bearer",
			ExpiresAt:    time.Now().Add(time.Hour),
			ClientID:     id,
		}); err != nil {
			t.Fatalf("saveTokens(%s) error: %v", id, err)
		}
	}

	// Both clients should be present in the file.
	data, _ := os.ReadFile(tokenFile)
	var sm TokenStorageMap
	if err := json.Unmarshal(data, &sm); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(sm.Tokens) != 2 {
		t.Errorf("expected 2 tokens, got %d", len(sm.Tokens))
	}
	for _, id := range []string{"client-a", "client-b"} {
		if _, ok := sm.Tokens[id]; !ok {
			t.Errorf("token for %s not found", id)
		}
	}
}

func TestBuildAuthURL_ContainsRequiredParams(t *testing.T) {
	originalServerURL := serverURL
	originalClientID := clientID
	originalRedirectURI := redirectURI
	originalScope := scope
	t.Cleanup(func() {
		serverURL = originalServerURL
		clientID = originalClientID
		redirectURI = originalRedirectURI
		scope = originalScope
	})

	serverURL = "http://localhost:8080"
	clientID = "my-client-id"
	redirectURI = "http://localhost:8888/callback"
	scope = "read write"

	pkce := &PKCEParams{
		Verifier:  "test-verifier",
		Challenge: "test-challenge",
		Method:    "S256",
	}
	state := "random-state"

	u := buildAuthURL(state, pkce)

	for _, want := range []string{
		"client_id=my-client-id",
		"redirect_uri=",
		"response_type=code",
		"scope=",
		"state=random-state",
		"code_challenge=test-challenge",
		"code_challenge_method=S256",
	} {
		if !containsSubstring(u, want) {
			t.Errorf("auth URL missing %q\nURL: %s", want, u)
		}
	}
}

func TestIsPublicClient(t *testing.T) {
	orig := clientSecret
	t.Cleanup(func() { clientSecret = orig })

	clientSecret = ""
	if !isPublicClient() {
		t.Error("expected public client when secret is empty")
	}

	clientSecret = "secret"
	if isPublicClient() {
		t.Error("expected confidential client when secret is set")
	}
}

// containsSubstring is a helper to avoid importing strings in tests.
func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && findSubstring(s, sub)
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
