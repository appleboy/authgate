package main

import (
	"context"
	"fmt"
	"html"
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	// callbackTimeout is how long we wait for the browser to deliver the code.
	callbackTimeout = 5 * time.Minute
)

// callbackResult holds the outcome of the local callback round-trip.
type callbackResult struct {
	Code  string
	Error string
	Desc  string
}

// startCallbackServer starts a local HTTP server on the given port and waits
// for the OAuth callback. It validates the returned state against expectedState
// and returns the authorization code (or an error).
//
// The server shuts itself down after the first request.
func startCallbackServer(port int, expectedState string) (string, error) {
	resultCh := make(chan callbackResult, 1)

	// sendResult delivers the result exactly once. Any concurrent or subsequent
	// invocations (e.g. a browser retry or automated agent) are silently
	// discarded, preventing a goroutine from blocking forever on the send.
	var once sync.Once
	sendResult := func(r callbackResult) {
		once.Do(func() { resultCh <- r })
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		// Check for OAuth error response first.
		if oauthErr := q.Get("error"); oauthErr != "" {
			desc := q.Get("error_description")
			writeCallbackPage(w, false, oauthErr, desc)
			sendResult(callbackResult{Error: oauthErr, Desc: desc})
			return
		}

		// Validate state (CSRF protection).
		state := q.Get("state")
		if state != expectedState {
			writeCallbackPage(w, false, "state_mismatch",
				"State parameter does not match. Possible CSRF attack.")
			sendResult(callbackResult{
				Error: "state_mismatch",
				Desc:  "state parameter mismatch",
			})
			return
		}

		code := q.Get("code")
		if code == "" {
			writeCallbackPage(w, false, "missing_code", "No authorization code in callback.")
			sendResult(callbackResult{Error: "missing_code", Desc: "code parameter missing"})
			return
		}

		writeCallbackPage(w, true, "", "")
		sendResult(callbackResult{Code: code})
	})

	srv := &http.Server{
		Addr:         fmt.Sprintf("127.0.0.1:%d", port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Use a listener so we can report the actual bound port.
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return "", fmt.Errorf("failed to start callback server on port %d: %w", port, err)
	}

	// Serve in background; shut down after receiving the result.
	go func() {
		_ = srv.Serve(ln)
	}()

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	// Wait for callback or timeout.
	select {
	case result := <-resultCh:
		if result.Error != "" {
			if result.Desc != "" {
				return "", fmt.Errorf("%s: %s", result.Error, result.Desc)
			}
			return "", fmt.Errorf("%s", result.Error)
		}
		return result.Code, nil

	case <-time.After(callbackTimeout):
		return "", fmt.Errorf("timed out waiting for browser authorization (%s)", callbackTimeout)
	}
}

// writeCallbackPage writes a minimal HTML response to the browser tab.
func writeCallbackPage(w http.ResponseWriter, success bool, errCode, errDesc string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if success {
		fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head><title>Authorization Successful</title></head>
<body style="font-family:sans-serif;text-align:center;padding:4rem">
  <h1 style="color:#2ea44f">&#10003; Authorization Successful</h1>
  <p>You have been successfully authorized.</p>
  <p>You can close this tab and return to your terminal.</p>
</body>
</html>`)
		return
	}

	msg := errCode
	if errDesc != "" {
		msg = errDesc
	}
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><title>Authorization Failed</title></head>
<body style="font-family:sans-serif;text-align:center;padding:4rem">
  <h1 style="color:#cb2431">&#10007; Authorization Failed</h1>
  <p>%s</p>
  <p>You can close this tab and check your terminal for details.</p>
</body>
</html>`, html.EscapeString(msg))
}
