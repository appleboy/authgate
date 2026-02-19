package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// startCallbackServerAsync starts the callback server in a goroutine and
// returns a channel that will receive the authorization code (or error string).
func startCallbackServerAsync(t *testing.T, port int, state string) chan string {
	t.Helper()
	ch := make(chan string, 1)
	go func() {
		code, err := startCallbackServer(port, state)
		if err != nil {
			ch <- "ERROR:" + err.Error()
		} else {
			ch <- code
		}
	}()
	// Give the server a moment to bind.
	time.Sleep(50 * time.Millisecond)
	return ch
}

func TestCallbackServer_Success(t *testing.T) {
	const port = 19001
	state := "test-state-success"

	ch := startCallbackServerAsync(t, port, state)

	// Simulate the browser redirect.
	callbackURL := fmt.Sprintf(
		"http://127.0.0.1:%d/callback?code=mycode123&state=%s",
		port, state,
	)
	resp, err := http.Get(callbackURL) //nolint:noctx
	if err != nil {
		t.Fatalf("GET callback failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("unexpected status %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "Authorization Successful") {
		t.Errorf("expected success page, got: %s", string(body))
	}

	// Check code returned to CLI.
	select {
	case result := <-ch:
		if result != "mycode123" {
			t.Errorf("expected code mycode123, got: %s", result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for callback result")
	}
}

func TestCallbackServer_StateMismatch(t *testing.T) {
	const port = 19002
	state := "expected-state"

	ch := startCallbackServerAsync(t, port, state)

	callbackURL := fmt.Sprintf(
		"http://127.0.0.1:%d/callback?code=mycode&state=wrong-state",
		port,
	)
	resp, err := http.Get(callbackURL) //nolint:noctx
	if err != nil {
		t.Fatalf("GET callback failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Authorization Failed") {
		t.Errorf("expected failure page for state mismatch, got: %s", string(body))
	}

	select {
	case result := <-ch:
		if !strings.HasPrefix(result, "ERROR:") {
			t.Errorf("expected error for state mismatch, got: %s", result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for callback result")
	}
}

func TestCallbackServer_OAuthError(t *testing.T) {
	const port = 19003
	state := "state-for-error"

	ch := startCallbackServerAsync(t, port, state)

	callbackURL := fmt.Sprintf(
		"http://127.0.0.1:%d/callback?error=access_denied&error_description=User+denied&state=%s",
		port, state,
	)
	resp, err := http.Get(callbackURL) //nolint:noctx
	if err != nil {
		t.Fatalf("GET callback failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Authorization Failed") {
		t.Errorf("expected failure page for access_denied, got: %s", string(body))
	}

	select {
	case result := <-ch:
		if !strings.HasPrefix(result, "ERROR:") {
			t.Errorf("expected error for access_denied, got: %s", result)
		}
		if !strings.Contains(result, "access_denied") {
			t.Errorf("expected error to mention access_denied, got: %s", result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for callback result")
	}
}

// TestCallbackServer_DoubleCallback verifies that a second /callback hit while
// the first result is still in the channel does not block the handler goroutine
// forever. Before the sync.Once fix, the second goroutine could block on the
// buffered-channel send until the shutdown context timed out.
func TestCallbackServer_DoubleCallback(t *testing.T) {
	const port = 19005
	state := "test-state-double"

	ch := startCallbackServerAsync(t, port, state)

	url := fmt.Sprintf("http://127.0.0.1:%d/callback?code=mycode&state=%s", port, state)

	// Fire two requests nearly simultaneously so both are in-flight before the
	// server has a chance to process the first result.
	done := make(chan error, 2)
	for range 2 {
		go func() {
			resp, err := http.Get(url) //nolint:noctx
			if err == nil {
				resp.Body.Close()
			}
			done <- err
		}()
	}

	// Both handler goroutines must finish — neither should block on the send.
	for range 2 {
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("a callback handler goroutine hung on channel send")
		}
	}

	// startCallbackServer must also return promptly.
	select {
	case result := <-ch:
		if result != "mycode" {
			t.Errorf("expected mycode, got: %s", result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for callback result")
	}
}

func TestCallbackServer_MissingCode(t *testing.T) {
	const port = 19004
	state := "state-for-missing-code"

	ch := startCallbackServerAsync(t, port, state)

	// Correct state but no code parameter.
	callbackURL := fmt.Sprintf(
		"http://127.0.0.1:%d/callback?state=%s",
		port, state,
	)
	resp, err := http.Get(callbackURL) //nolint:noctx
	if err != nil {
		t.Fatalf("GET callback failed: %v", err)
	}
	defer resp.Body.Close()

	select {
	case result := <-ch:
		if !strings.HasPrefix(result, "ERROR:") {
			t.Errorf("expected error for missing code, got: %s", result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for callback result")
	}
}
