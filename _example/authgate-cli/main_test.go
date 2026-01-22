package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func init() {
	// Set default values for tests (don't call initConfig to avoid flag parsing)
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}
	if clientID == "" {
		clientID = "test-client"
	}
	if tokenFile == "" {
		tokenFile = ".authgate-tokens.json"
	}
}

func TestSaveTokens_ConcurrentWrites(t *testing.T) {
	tempDir := t.TempDir()
	tokenFile = filepath.Join(tempDir, "tokens.json")

	const goroutines = 10
	var wg sync.WaitGroup

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()

			storage := &TokenStorage{
				AccessToken:  fmt.Sprintf("access-token-%d", id),
				RefreshToken: fmt.Sprintf("refresh-token-%d", id),
				TokenType:    "Bearer",
				ExpiresAt:    time.Now().Add(1 * time.Hour),
				ClientID:     fmt.Sprintf("client-%d", id),
			}

			if err := saveTokens(storage); err != nil {
				t.Errorf("Goroutine %d: Failed to save tokens: %v", id, err)
			}
		}(i)
	}

	wg.Wait()

	// Verify all tokens were saved
	data, err := os.ReadFile(tokenFile)
	if err != nil {
		t.Fatalf("Failed to read token file: %v", err)
	}

	var storageMap TokenStorageMap
	if err := json.Unmarshal(data, &storageMap); err != nil {
		t.Fatalf("Failed to parse token file: %v", err)
	}

	// Should have all client tokens
	if len(storageMap.Tokens) != goroutines {
		t.Errorf("Expected %d client tokens, got %d", goroutines, len(storageMap.Tokens))
	}

	// Verify each token
	for i := 0; i < goroutines; i++ {
		clientID := fmt.Sprintf("client-%d", i)
		token, ok := storageMap.Tokens[clientID]
		if !ok {
			t.Errorf("Missing token for client %s", clientID)
			continue
		}

		expectedAccessToken := fmt.Sprintf("access-token-%d", i)
		if token.AccessToken != expectedAccessToken {
			t.Errorf("Client %s: Expected access token %s, got %s", clientID, expectedAccessToken, token.AccessToken)
		}
	}

	// Verify no lock files remain
	lockPath := tokenFile + ".lock"
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Errorf("Lock file still exists after all saves completed")
	}
}

func TestSaveTokens_PreservesOtherClients(t *testing.T) {
	tempDir := t.TempDir()
	tokenFile = filepath.Join(tempDir, "tokens.json")

	// Save first client
	clientID = "client-1"
	storage1 := &TokenStorage{
		AccessToken:  "token-1",
		RefreshToken: "refresh-1",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		ClientID:     "client-1",
	}
	if err := saveTokens(storage1); err != nil {
		t.Fatalf("Failed to save first client: %v", err)
	}

	// Save second client (should preserve first)
	clientID = "client-2"
	storage2 := &TokenStorage{
		AccessToken:  "token-2",
		RefreshToken: "refresh-2",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		ClientID:     "client-2",
	}
	if err := saveTokens(storage2); err != nil {
		t.Fatalf("Failed to save second client: %v", err)
	}

	// Load and verify both exist
	data, err := os.ReadFile(tokenFile)
	if err != nil {
		t.Fatalf("Failed to read token file: %v", err)
	}

	var storageMap TokenStorageMap
	if err := json.Unmarshal(data, &storageMap); err != nil {
		t.Fatalf("Failed to parse token file: %v", err)
	}

	if len(storageMap.Tokens) != 2 {
		t.Errorf("Expected 2 clients, got %d", len(storageMap.Tokens))
	}

	if token, ok := storageMap.Tokens["client-1"]; !ok || token.AccessToken != "token-1" {
		t.Errorf("Client 1 token was not preserved")
	}

	if token, ok := storageMap.Tokens["client-2"]; !ok || token.AccessToken != "token-2" {
		t.Errorf("Client 2 token was not saved correctly")
	}
}

func BenchmarkSaveTokens_SingleClient(b *testing.B) {
	tempDir := b.TempDir()
	tokenFile = filepath.Join(tempDir, "tokens.json")
	clientID = "bench-client"

	storage := &TokenStorage{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		ClientID:     clientID,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := saveTokens(storage); err != nil {
			b.Fatalf("Failed to save tokens: %v", err)
		}
	}
}

func BenchmarkSaveTokens_ParallelWrites(b *testing.B) {
	tempDir := b.TempDir()
	tokenFile = filepath.Join(tempDir, "tokens.json")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		id := 0
		for pb.Next() {
			storage := &TokenStorage{
				AccessToken:  fmt.Sprintf("access-token-%d", id),
				RefreshToken: fmt.Sprintf("refresh-token-%d", id),
				TokenType:    "Bearer",
				ExpiresAt:    time.Now().Add(1 * time.Hour),
				ClientID:     fmt.Sprintf("client-%d", id),
			}

			if err := saveTokens(storage); err != nil {
				b.Fatalf("Failed to save tokens: %v", err)
			}
			id++
		}
	})
}
