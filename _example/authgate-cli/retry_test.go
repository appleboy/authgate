package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		statusCode int
		want       bool
	}{
		{
			name:       "network error - retryable",
			err:        fmt.Errorf("connection refused"),
			statusCode: 0,
			want:       true,
		},
		{
			name:       "500 internal server error - retryable",
			err:        nil,
			statusCode: http.StatusInternalServerError,
			want:       true,
		},
		{
			name:       "502 bad gateway - retryable",
			err:        nil,
			statusCode: http.StatusBadGateway,
			want:       true,
		},
		{
			name:       "503 service unavailable - retryable",
			err:        nil,
			statusCode: http.StatusServiceUnavailable,
			want:       true,
		},
		{
			name:       "429 too many requests - retryable",
			err:        nil,
			statusCode: http.StatusTooManyRequests,
			want:       true,
		},
		{
			name:       "200 OK - not retryable",
			err:        nil,
			statusCode: http.StatusOK,
			want:       false,
		},
		{
			name:       "400 bad request - not retryable",
			err:        nil,
			statusCode: http.StatusBadRequest,
			want:       false,
		},
		{
			name:       "401 unauthorized - not retryable",
			err:        nil,
			statusCode: http.StatusUnauthorized,
			want:       false,
		},
		{
			name:       "404 not found - not retryable",
			err:        nil,
			statusCode: http.StatusNotFound,
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp *http.Response
			if tt.statusCode != 0 {
				resp = &http.Response{StatusCode: tt.statusCode}
			}

			got := isRetryableError(tt.err, resp)
			if got != tt.want {
				t.Errorf("isRetryableError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRetryableHTTPRequest_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))
	defer server.Close()

	ctx := context.Background()
	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := retryableHTTPRequest(ctx, httpClient, req)
	if err != nil {
		t.Fatalf("retryableHTTPRequest() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestRetryableHTTPRequest_RetryOn500(t *testing.T) {
	var attemptCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := attemptCount.Add(1)
		if count < 3 {
			// Fail first 2 attempts
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("server error"))
			return
		}
		// Succeed on 3rd attempt
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))
	defer server.Close()

	ctx := context.Background()
	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := retryableHTTPRequest(ctx, httpClient, req)
	if err != nil {
		t.Fatalf("retryableHTTPRequest() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	finalCount := attemptCount.Load()
	if finalCount != 3 {
		t.Errorf("Expected 3 attempts, got %d", finalCount)
	}
}

func TestRetryableHTTPRequest_NoRetryOn400(t *testing.T) {
	var attemptCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	}))
	defer server.Close()

	ctx := context.Background()
	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := retryableHTTPRequest(ctx, httpClient, req)
	if err != nil {
		t.Fatalf("retryableHTTPRequest() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}

	finalCount := attemptCount.Load()
	if finalCount != 1 {
		t.Errorf("Expected only 1 attempt (no retry), got %d", finalCount)
	}
}

func TestRetryableHTTPRequest_ExponentialBackoff(t *testing.T) {
	var attemptCount atomic.Int32
	var timestamps []time.Time
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		timestamps = append(timestamps, time.Now())
		mu.Unlock()

		count := attemptCount.Add(1)
		if count <= maxRetries {
			// Fail all attempts except the last
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx := context.Background()
	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	_, err = retryableHTTPRequest(ctx, httpClient, req)
	if err != nil {
		t.Fatalf("retryableHTTPRequest() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	// Should have maxRetries + 1 attempts (initial + retries)
	expectedAttempts := maxRetries + 1
	if len(timestamps) != expectedAttempts {
		t.Errorf("Expected %d attempts, got %d", expectedAttempts, len(timestamps))
	}

	// Check that delays are increasing (exponential backoff)
	if len(timestamps) >= 3 {
		delay1 := timestamps[1].Sub(timestamps[0])
		delay2 := timestamps[2].Sub(timestamps[1])

		// Second delay should be roughly 2x the first delay (with some tolerance)
		minExpectedDelay2 := delay1 * 15 / 10 // 1.5x (allowing some variance)
		if delay2 < minExpectedDelay2 {
			t.Errorf(
				"Expected exponential backoff: delay2 (%v) should be >= 1.5x delay1 (%v)",
				delay2,
				delay1,
			)
		}
	}
}

func TestRetryableHTTPRequest_ContextTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always return 500 to trigger retries
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	_, err = retryableHTTPRequest(ctx, httpClient, req)
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}
}
