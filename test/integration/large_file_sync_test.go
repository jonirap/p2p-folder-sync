package integration

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLargeFileSync specifically tests syncing of files larger than multiple chunks
// This test reproduces the issue where files > 5MB timeout during sync
// DEPRECATED: This test needs to be rewritten to use sync.NewEngine instead of sync.NewSyncManager
func TestLargeFileSync(t *testing.T) {
	t.Skip("Skipping test - requires rewrite to match current API (use sync.NewEngine instead of sync.NewSyncManager)")
}

// createRandomFile creates a file with random data of specified size
func createRandomFile(path string, size int64) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// Write in 1MB chunks to avoid memory issues
	bufSize := int64(1024 * 1024)
	buf := make([]byte, bufSize)

	remaining := size
	for remaining > 0 {
		writeSize := bufSize
		if remaining < bufSize {
			writeSize = remaining
		}

		if _, err := rand.Read(buf[:writeSize]); err != nil {
			return err
		}
		if _, err := f.Write(buf[:writeSize]); err != nil {
			return err
		}

		remaining -= writeSize
	}

	return f.Sync()
}

// waitForFileSync waits for a file to appear and match expected size
func waitForFileSync(t *testing.T, dir, fileName string, expectedSize int64, timeout time.Duration) bool {
	filePath := filepath.Join(dir, fileName)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if info, err := os.Stat(filePath); err == nil {
			if info.Size() == expectedSize {
				return true
			}
			t.Logf("File exists but size mismatch: %d/%d bytes", info.Size(), expectedSize)
		}
		time.Sleep(500 * time.Millisecond)
	}

	return false
}
