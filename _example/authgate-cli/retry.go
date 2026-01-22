package main

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// Retry configuration
const (
	maxRetries         = 3
	initialRetryDelay  = 1 * time.Second
	maxRetryDelay      = 10 * time.Second
	retryDelayMultiple = 2.0
)

// isRetryableError checks if an error is retryable
func isRetryableError(err error, resp *http.Response) bool {
	if err != nil {
		// Network errors, timeouts, connection errors are retryable
		return true
	}

	if resp == nil {
		return false
	}

	// Retry on 5xx server errors and 429 Too Many Requests
	statusCode := resp.StatusCode
	return statusCode >= 500 || statusCode == http.StatusTooManyRequests
}

// retryableHTTPRequest executes an HTTP request with retry logic using exponential backoff
func retryableHTTPRequest(
	ctx context.Context,
	client *http.Client,
	req *http.Request,
) (*http.Response, error) {
	var lastErr error
	var resp *http.Response
	delay := initialRetryDelay

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Wait before retry (exponential backoff)
			select {
			case <-ctx.Done():
				if lastErr != nil {
					return nil, fmt.Errorf("context cancelled after %d attempts: %w", attempt, lastErr)
				}
				return nil, ctx.Err()
			case <-time.After(delay):
				// Calculate next delay with exponential backoff
				delay = time.Duration(float64(delay) * retryDelayMultiple)
				if delay > maxRetryDelay {
					delay = maxRetryDelay
				}
			}
		}

		// Clone the request for retry (important: body might be consumed)
		reqClone := req.Clone(ctx)

		resp, lastErr = client.Do(reqClone)

		// Check if we should retry
		if !isRetryableError(lastErr, resp) {
			// Success or non-retryable error
			return resp, lastErr
		}

		// Close response body before retry to prevent resource leak
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
	}

	// All retries exhausted
	if lastErr != nil {
		return nil, fmt.Errorf("request failed after %d retries: %w", maxRetries, lastErr)
	}

	return resp, lastErr
}
