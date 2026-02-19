package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestPollForToken_AuthorizationPending(t *testing.T) {
	attempts := atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)

		// Return authorization_pending for first 2 attempts, then success
		if attempts.Load() < 3 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error":             "authorization_pending",
				"error_description": "User has not yet authorized",
			})
			return
		}

		// Success on 3rd attempt
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "test-access-token",
			"refresh_token": "test-refresh-token",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
	defer server.Close()

	config := &oauth2.Config{
		ClientID: "test-client",
		Endpoint: oauth2.Endpoint{
			TokenURL: server.URL,
		},
	}

	deviceAuth := &oauth2.DeviceAuthResponse{
		DeviceCode: "test-device-code",
		Interval:   1, // 1 second for testing
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	token, err := pollForTokenWithProgress(ctx, config, deviceAuth)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	if token.AccessToken != "test-access-token" {
		t.Errorf("Expected access token 'test-access-token', got '%s'", token.AccessToken)
	}

	if attempts.Load() < 3 {
		t.Errorf("Expected at least 3 attempts, got %d", attempts.Load())
	}
}

func TestPollForToken_SlowDown(t *testing.T) {
	attempts := atomic.Int32{}
	slowDownCount := atomic.Int32{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)

		// Return slow_down for first 2 attempts
		if attempts.Load() <= 2 {
			slowDownCount.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error":             "slow_down",
				"error_description": "Polling too frequently",
			})
			return
		}

		// Return authorization_pending after slow_down
		if attempts.Load() < 5 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error":             "authorization_pending",
				"error_description": "User has not yet authorized",
			})
			return
		}

		// Success
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "test-access-token",
			"refresh_token": "test-refresh-token",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
	defer server.Close()

	config := &oauth2.Config{
		ClientID: "test-client",
		Endpoint: oauth2.Endpoint{
			TokenURL: server.URL,
		},
	}

	deviceAuth := &oauth2.DeviceAuthResponse{
		DeviceCode: "test-device-code",
		Interval:   1, // 1 second for testing
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	token, err := pollForTokenWithProgress(ctx, config, deviceAuth)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	if token.AccessToken != "test-access-token" {
		t.Errorf("Expected access token 'test-access-token', got '%s'", token.AccessToken)
	}

	if slowDownCount.Load() < 2 {
		t.Errorf("Expected at least 2 slow_down responses, got %d", slowDownCount.Load())
	}

	// Verify that polling continued after slow_down
	if attempts.Load() < 5 {
		t.Errorf(
			"Expected at least 5 attempts (2 slow_down + 2 pending + 1 success), got %d",
			attempts.Load(),
		)
	}
}

func TestPollForToken_ExpiredToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error":             "expired_token",
			"error_description": "Device code has expired",
		})
	}))
	defer server.Close()

	config := &oauth2.Config{
		ClientID: "test-client",
		Endpoint: oauth2.Endpoint{
			TokenURL: server.URL,
		},
	}

	deviceAuth := &oauth2.DeviceAuthResponse{
		DeviceCode: "test-device-code",
		Interval:   1,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := pollForTokenWithProgress(ctx, config, deviceAuth)
	if err == nil {
		t.Fatal("Expected error for expired token, got nil")
	}

	if err.Error() != "device code expired, please restart the flow" {
		t.Errorf("Expected 'device code expired' error, got: %v", err)
	}
}

func TestPollForToken_AccessDenied(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error":             "access_denied",
			"error_description": "User denied the authorization request",
		})
	}))
	defer server.Close()

	config := &oauth2.Config{
		ClientID: "test-client",
		Endpoint: oauth2.Endpoint{
			TokenURL: server.URL,
		},
	}

	deviceAuth := &oauth2.DeviceAuthResponse{
		DeviceCode: "test-device-code",
		Interval:   1,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := pollForTokenWithProgress(ctx, config, deviceAuth)
	if err == nil {
		t.Fatal("Expected error for access denied, got nil")
	}

	if err.Error() != "user denied authorization" {
		t.Errorf("Expected 'user denied authorization' error, got: %v", err)
	}
}

func TestPollForToken_ContextTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always return pending
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error":             "authorization_pending",
			"error_description": "User has not yet authorized",
		})
	}))
	defer server.Close()

	config := &oauth2.Config{
		ClientID: "test-client",
		Endpoint: oauth2.Endpoint{
			TokenURL: server.URL,
		},
	}

	deviceAuth := &oauth2.DeviceAuthResponse{
		DeviceCode: "test-device-code",
		Interval:   1,
	}

	// Very short timeout to trigger context cancellation
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := pollForTokenWithProgress(ctx, config, deviceAuth)
	if err == nil {
		t.Fatal("Expected context timeout error, got nil")
	}

	// Context error should be wrapped in the error chain
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Expected context.DeadlineExceeded in error chain, got: %v", err)
	}
}

func TestExchangeDeviceCode_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}

		if err := r.ParseForm(); err != nil {
			t.Fatalf("Failed to parse form: %v", err)
		}

		if r.FormValue("grant_type") != "urn:ietf:params:oauth:grant-type:device_code" {
			t.Errorf("Expected device_code grant type, got %s", r.FormValue("grant_type"))
		}

		if r.FormValue("device_code") != "test-device-code" {
			t.Errorf(
				"Expected device_code 'test-device-code', got '%s'",
				r.FormValue("device_code"),
			)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "test-access-token",
			"refresh_token": "test-refresh-token",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
	defer server.Close()

	ctx := context.Background()
	token, err := exchangeDeviceCode(ctx, server.URL, "test-client", "test-device-code")
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	if token.AccessToken != "test-access-token" {
		t.Errorf("Expected access token 'test-access-token', got '%s'", token.AccessToken)
	}

	if token.RefreshToken != "test-refresh-token" {
		t.Errorf("Expected refresh token 'test-refresh-token', got '%s'", token.RefreshToken)
	}

	if token.TokenType != "Bearer" {
		t.Errorf("Expected token type 'Bearer', got '%s'", token.TokenType)
	}
}
