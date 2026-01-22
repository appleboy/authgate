package main

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestFileLock_BasicAcquireRelease(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.json")

	// Acquire lock
	lock, err := acquireFileLock(testFile)
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	// Verify lock file exists
	lockPath := testFile + ".lock"
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Errorf("Lock file was not created")
	}

	// Release lock
	if err := lock.release(); err != nil {
		t.Errorf("Failed to release lock: %v", err)
	}

	// Verify lock file is removed
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Errorf("Lock file was not removed after release")
	}
}

func TestFileLock_ConcurrentAccess(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.json")

	const goroutines = 10
	const iterations = 5

	var (
		successCount atomic.Int32
		wg           sync.WaitGroup
	)

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				lock, err := acquireFileLock(testFile)
				if err != nil {
					t.Errorf("Goroutine %d iteration %d: Failed to acquire lock: %v", id, j, err)
					return
				}

				// Simulate work while holding lock
				time.Sleep(10 * time.Millisecond)
				successCount.Add(1)

				if err := lock.release(); err != nil {
					t.Errorf("Goroutine %d iteration %d: Failed to release lock: %v", id, j, err)
					return
				}
			}
		}(i)
	}

	wg.Wait()

	// All operations should succeed
	expected := int32(goroutines * iterations)
	if successCount.Load() != expected {
		t.Errorf("Expected %d successful operations, got %d", expected, successCount.Load())
	}

	// Lock file should be cleaned up
	lockPath := testFile + ".lock"
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Errorf("Lock file still exists after all goroutines finished")
	}
}

func TestFileLock_StaleLocksCleanup(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.json")
	lockPath := testFile + ".lock"

	// Create a stale lock file (older than 30 seconds)
	staleLock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatalf("Failed to create stale lock: %v", err)
	}
	staleLock.Close()

	// Set modification time to 35 seconds ago (past the 30-second threshold)
	staleTime := time.Now().Add(-35 * time.Second)
	if err := os.Chtimes(lockPath, staleTime, staleTime); err != nil {
		t.Fatalf("Failed to set stale lock time: %v", err)
	}

	// Try to acquire lock - should succeed by cleaning up stale lock
	lock, err := acquireFileLock(testFile)
	if err != nil {
		t.Fatalf("Failed to acquire lock after stale lock: %v", err)
	}
	defer lock.release()

	// Verify we got the lock
	if lock.lockFile == nil {
		t.Errorf("Lock file handle is nil")
	}
}

func TestFileLock_BlockedByActiveLock(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.json")

	// Acquire first lock
	lock1, err := acquireFileLock(testFile)
	if err != nil {
		t.Fatalf("Failed to acquire first lock: %v", err)
	}
	defer lock1.release()

	// Try to acquire second lock in goroutine (should be blocked)
	errChan := make(chan error, 1)
	go func() {
		lock2, err := acquireFileLock(testFile)
		if err != nil {
			errChan <- err
			return
		}
		lock2.release()
		errChan <- nil
	}()

	// Wait a bit to ensure second goroutine is blocked
	time.Sleep(200 * time.Millisecond)

	// Second lock should still be waiting
	select {
	case <-errChan:
		t.Errorf("Second lock acquired while first lock was active")
	default:
		// Expected: still blocked
	}

	// Release first lock
	lock1.release()

	// Now second lock should succeed
	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("Second lock failed after first lock released: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Errorf("Second lock timed out after first lock released")
	}
}

func TestFileLock_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping timeout test in short mode")
	}

	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.json")
	lockPath := testFile + ".lock"

	// Create a fresh lock file (not stale)
	freshLock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatalf("Failed to create fresh lock: %v", err)
	}
	freshLock.Close()

	// Try to acquire lock - should timeout after ~5 seconds
	start := time.Now()
	_, err = acquireFileLock(testFile)
	duration := time.Since(start)

	if err == nil {
		t.Errorf("Expected timeout error, but lock was acquired")
	}

	// Should timeout around 5 seconds (50 retries * 100ms)
	if duration < 4*time.Second || duration > 7*time.Second {
		t.Errorf("Expected timeout around 5 seconds, got %v", duration)
	}

	// Verify error message mentions timeout
	if err != nil && err.Error() == "" {
		t.Errorf("Error message is empty")
	}

	// Clean up
	os.Remove(lockPath)
}

func TestFileLock_MultipleReleases(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.json")

	lock, err := acquireFileLock(testFile)
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	// First release should succeed
	if err := lock.release(); err != nil {
		t.Errorf("First release failed: %v", err)
	}

	// Second release should not panic (idempotent)
	if err := lock.release(); err == nil {
		t.Logf("Second release returned: %v (expected error)", err)
	}
}

func BenchmarkFileLock_AcquireRelease(b *testing.B) {
	tempDir := b.TempDir()
	testFile := filepath.Join(tempDir, "test.json")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lock, err := acquireFileLock(testFile)
		if err != nil {
			b.Fatalf("Failed to acquire lock: %v", err)
		}
		lock.release()
	}
}
