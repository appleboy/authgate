package httpclient

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"testing"
	"time"
)

const (
	testAPISecret      = "test-secret"
	testXSignature     = "X-Signature"
	testXTimestamp     = "X-Timestamp"
	testXNonce         = "X-Nonce"
	testXAPISecret     = "X-API-Secret"
	testExampleURL     = "http://example.com/api"
	testExampleAuthURL = "http://example.com/api/auth"
)

func TestAuthConfig_AddAuthHeaders_None(t *testing.T) {
	config := &AuthConfig{
		Mode:   AuthModeNone,
		Secret: testAPISecret,
	}

	req, err := http.NewRequestWithContext(
		context.Background(),
		"POST",
		testExampleURL,
		bytes.NewBufferString("test body"),
	)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	err = config.AddAuthHeaders(req, []byte("test body"))
	if err != nil {
		t.Errorf("AddAuthHeaders() error = %v, want nil", err)
	}

	// Should not add any auth headers in none mode
	if req.Header.Get(testXAPISecret) != "" {
		t.Errorf("Expected no X-API-Secret header in none mode")
	}
	if req.Header.Get(testXSignature) != "" {
		t.Errorf("Expected no X-Signature header in none mode")
	}
}

func TestAuthConfig_AddAuthHeaders_Simple(t *testing.T) {
	tests := []struct {
		name       string
		config     *AuthConfig
		wantHeader string
		wantValue  string
		wantErr    bool
	}{
		{
			name: "Simple mode with default header",
			config: &AuthConfig{
				Mode:   AuthModeSimple,
				Secret: "test-secret-123",
			},
			wantHeader: "X-API-Secret",
			wantValue:  "test-secret-123",
			wantErr:    false,
		},
		{
			name: "Simple mode with custom header",
			config: &AuthConfig{
				Mode:       AuthModeSimple,
				Secret:     "my-custom-secret",
				HeaderName: "X-Custom-Auth",
			},
			wantHeader: "X-Custom-Auth",
			wantValue:  "my-custom-secret",
			wantErr:    false,
		},
		{
			name: "Simple mode without secret",
			config: &AuthConfig{
				Mode:   AuthModeSimple,
				Secret: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(
				context.Background(),
				"POST",
				testExampleURL,
				bytes.NewBufferString("test body"),
			)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			err = tt.config.AddAuthHeaders(req, []byte("test body"))
			if (err != nil) != tt.wantErr {
				t.Errorf("AddAuthHeaders() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				got := req.Header.Get(tt.wantHeader)
				if got != tt.wantValue {
					t.Errorf("Header %s = %v, want %v", tt.wantHeader, got, tt.wantValue)
				}
			}
		})
	}
}

func TestAuthConfig_AddAuthHeaders_HMAC(t *testing.T) {
	config := &AuthConfig{
		Mode:   AuthModeHMAC,
		Secret: "test-secret-hmac",
	}

	body := []byte(`{"username":"test","password":"pass123"}`)
	req, err := http.NewRequestWithContext(
		context.Background(),
		"POST",
		testExampleAuthURL,
		bytes.NewBuffer(body),
	)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	err = config.AddAuthHeaders(req, body)
	if err != nil {
		t.Fatalf("AddAuthHeaders() error = %v", err)
	}

	// Check that all required headers are present
	signature := req.Header.Get(testXSignature)
	if signature == "" {
		t.Errorf("Expected X-Signature header to be set")
	}

	timestamp := req.Header.Get(testXTimestamp)
	if timestamp == "" {
		t.Errorf("Expected X-Timestamp header to be set")
	}

	nonce := req.Header.Get(testXNonce)
	if nonce == "" {
		t.Errorf("Expected X-Nonce header to be set")
	}

	// Verify signature format (should be hex string)
	if len(signature) != 64 { // SHA256 hex string is 64 characters
		t.Errorf("Signature length = %d, want 64", len(signature))
	}

	// Verify timestamp is recent (within 1 second)
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		t.Errorf("Failed to parse timestamp: %v", err)
	}

	timeDiff := time.Now().Unix() - ts
	if timeDiff > 1 {
		t.Errorf("Timestamp is too old: %d seconds", timeDiff)
	}
}

func TestAuthConfig_AddAuthHeaders_HMAC_CustomHeaders(t *testing.T) {
	config := &AuthConfig{
		Mode:            AuthModeHMAC,
		Secret:          "test-secret",
		SignatureHeader: "X-Custom-Sig",
		TimestampHeader: "X-Custom-Time",
		NonceHeader:     "X-Custom-Nonce",
	}

	body := []byte("test body")
	req, err := http.NewRequestWithContext(
		context.Background(),
		"POST",
		testExampleURL,
		bytes.NewBuffer(body),
	)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	err = config.AddAuthHeaders(req, body)
	if err != nil {
		t.Fatalf("AddAuthHeaders() error = %v", err)
	}

	// Check custom headers
	if req.Header.Get("X-Custom-Sig") == "" {
		t.Errorf("Expected X-Custom-Sig header to be set")
	}
	if req.Header.Get("X-Custom-Time") == "" {
		t.Errorf("Expected X-Custom-Time header to be set")
	}
	if req.Header.Get("X-Custom-Nonce") == "" {
		t.Errorf("Expected X-Custom-Nonce header to be set")
	}
}

func TestAuthConfig_calculateHMACSignature(t *testing.T) {
	config := &AuthConfig{
		Secret: "test-secret",
	}

	timestamp := int64(1704067200) // Fixed timestamp for testing
	method := "POST"
	path := "/api/auth"
	body := []byte(`{"test":"data"}`)

	signature := config.calculateHMACSignature(timestamp, method, path, body)

	// Calculate expected signature manually
	message := "1704067200POST/api/auth{\"test\":\"data\"}"
	h := hmac.New(sha256.New, []byte("test-secret"))
	h.Write([]byte(message))
	expected := hex.EncodeToString(h.Sum(nil))

	if signature != expected {
		t.Errorf("calculateHMACSignature() = %v, want %v", signature, expected)
	}

	// Verify signature is consistent
	signature2 := config.calculateHMACSignature(timestamp, method, path, body)
	if signature != signature2 {
		t.Errorf("Signature is not consistent: %v != %v", signature, signature2)
	}
}

func TestAuthConfig_VerifyHMACSignature(t *testing.T) {
	config := &AuthConfig{
		Secret: "test-secret",
	}

	body := []byte(`{"username":"test"}`)
	timestamp := time.Now().Unix()
	signature := config.calculateHMACSignature(timestamp, "POST", "/api/auth", body)

	// Create a request with valid signature
	req, err := http.NewRequestWithContext(
		context.Background(),
		"POST",
		testExampleAuthURL,
		bytes.NewBuffer(body),
	)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	req.Header.Set(testXSignature, signature)
	req.Header.Set(testXTimestamp, strconv.FormatInt(timestamp, 10))

	err = config.VerifyHMACSignature(req, 5*time.Minute)
	if err != nil {
		t.Errorf("VerifyHMACSignature() error = %v, want nil", err)
	}
}

func TestAuthConfig_VerifyHMACSignature_InvalidSignature(t *testing.T) {
	config := &AuthConfig{
		Secret: "test-secret",
	}

	body := []byte(`{"username":"test"}`)
	timestamp := time.Now().Unix()

	// Create a request with invalid signature
	req, err := http.NewRequestWithContext(
		context.Background(),
		"POST",
		testExampleAuthURL,
		bytes.NewBuffer(body),
	)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	req.Header.Set(testXSignature, "invalid-signature-12345")
	req.Header.Set(testXTimestamp, strconv.FormatInt(timestamp, 10))

	err = config.VerifyHMACSignature(req, 5*time.Minute)
	if err == nil {
		t.Errorf("VerifyHMACSignature() error = nil, want error")
	}
}

func TestAuthConfig_VerifyHMACSignature_ExpiredTimestamp(t *testing.T) {
	config := &AuthConfig{
		Secret: "test-secret",
	}

	body := []byte(`{"username":"test"}`)
	// Timestamp from 10 minutes ago
	timestamp := time.Now().Add(-10 * time.Minute).Unix()
	signature := config.calculateHMACSignature(timestamp, "POST", "/api/auth", body)

	req, err := http.NewRequestWithContext(
		context.Background(),
		"POST",
		testExampleAuthURL,
		bytes.NewBuffer(body),
	)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	req.Header.Set(testXSignature, signature)
	req.Header.Set(testXTimestamp, strconv.FormatInt(timestamp, 10))

	// Verify with 5 minute max age - should fail
	err = config.VerifyHMACSignature(req, 5*time.Minute)
	if err == nil {
		t.Errorf("VerifyHMACSignature() error = nil, want expired error")
	}
}

func TestAuthConfig_VerifyHMACSignature_MissingHeaders(t *testing.T) {
	config := &AuthConfig{
		Secret: "test-secret",
	}

	tests := []struct {
		name      string
		setupReq  func() *http.Request
		wantError bool
	}{
		{
			name: "Missing signature header",
			setupReq: func() *http.Request {
				req, _ := http.NewRequestWithContext(
					context.Background(),
					"POST",
					testExampleURL,
					bytes.NewBufferString("test"),
				)
				req.Header.Set(testXTimestamp, strconv.FormatInt(time.Now().Unix(), 10))
				return req
			},
			wantError: true,
		},
		{
			name: "Missing timestamp header",
			setupReq: func() *http.Request {
				req, _ := http.NewRequestWithContext(
					context.Background(),
					"POST",
					testExampleURL,
					bytes.NewBufferString("test"),
				)
				req.Header.Set(testXSignature, "some-signature")
				return req
			},
			wantError: true,
		},
		{
			name: "Both headers present",
			setupReq: func() *http.Request {
				body := []byte("test")
				req, _ := http.NewRequestWithContext(
					context.Background(),
					"POST",
					testExampleURL,
					bytes.NewBuffer(body),
				)
				timestamp := time.Now().Unix()
				signature := config.calculateHMACSignature(timestamp, "POST", "/api", body)
				req.Header.Set(testXSignature, signature)
				req.Header.Set(testXTimestamp, strconv.FormatInt(timestamp, 10))
				return req
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.setupReq()
			// Need to read body first to recreate it for verification
			bodyBytes, _ := io.ReadAll(req.Body)
			req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

			err := config.VerifyHMACSignature(req, 5*time.Minute)
			if (err != nil) != tt.wantError {
				t.Errorf("VerifyHMACSignature() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestNewAuthConfig(t *testing.T) {
	config := NewAuthConfig("hmac", "my-secret")

	if config.Mode != "hmac" {
		t.Errorf("Mode = %v, want hmac", config.Mode)
	}

	if config.Secret != "my-secret" {
		t.Errorf("Secret = %v, want my-secret", config.Secret)
	}

	// Check defaults
	if config.HeaderName != testXAPISecret {
		t.Errorf("HeaderName = %v, want %s", config.HeaderName, testXAPISecret)
	}

	if config.SignatureHeader != testXSignature {
		t.Errorf("SignatureHeader = %v, want %s", config.SignatureHeader, testXSignature)
	}

	if config.TimestampHeader != testXTimestamp {
		t.Errorf("TimestampHeader = %v, want %s", config.TimestampHeader, testXTimestamp)
	}

	if config.NonceHeader != testXNonce {
		t.Errorf("NonceHeader = %v, want %s", config.NonceHeader, testXNonce)
	}
}
