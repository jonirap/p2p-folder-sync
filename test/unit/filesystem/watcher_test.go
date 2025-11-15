package filesystem_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/p2p-folder-sync/p2p-sync/internal/filesystem"
)

// TestWatcher_IgnorePath verifies that ignored paths don't generate events
func TestWatcher_IgnorePath(t *testing.T) {
	watcher, err := filesystem.NewWatcher()
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Close()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "ignored_file.txt")

	// Start watching the directory
	err = watcher.Add(tmpDir)
	if err != nil {
		t.Fatalf("Failed to add watch: %v", err)
	}

	// Ignore the specific file path
	watcher.IgnorePath(testFile)

	// Create the file
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Wait a bit for potential events
	time.Sleep(100 * time.Millisecond)

	// Check that no events were generated for the ignored path
	select {
	case event := <-watcher.Events():
		t.Errorf("Received unexpected event for ignored path: %+v", event)
	default:
		// No event received - this is expected
	}

	// Verify the file was actually created
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Error("Test file was not created")
	}
}

// TestWatcher_WatchPath_Reenable verifies re-enabled paths generate events again
func TestWatcher_WatchPath_Reenable(t *testing.T) {
	watcher, err := filesystem.NewWatcher()
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Close()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "reenabled_file.txt")

	// Start watching the directory
	err = watcher.Add(tmpDir)
	if err != nil {
		t.Fatalf("Failed to add watch: %v", err)
	}

	// Ignore the file initially
	watcher.IgnorePath(testFile)

	// Create file while ignored - should not generate event
	err = os.WriteFile(testFile, []byte("initial content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Check no event was generated
	select {
	case event := <-watcher.Events():
		t.Errorf("Received unexpected event for initially ignored path: %+v", event)
	default:
		// Expected - no event
	}

	// Re-enable watching for the path
	watcher.WatchPath(testFile)

	// Now modify the file - should generate an event
	err = os.WriteFile(testFile, []byte("modified content"), 0644)
	if err != nil {
		t.Fatalf("Failed to modify test file: %v", err)
	}

	// Wait for the event
	select {
	case event := <-watcher.Events():
		if event.Path != testFile {
			t.Errorf("Expected event for %s, got event for %s", testFile, event.Path)
		}
		if event.Operation != "update" {
			t.Errorf("Expected update operation, got %s", event.Operation)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("Expected event after re-enabling path, but none received")
	}
}

// TestWatcher_RemoteWriteIgnored verifies file write to ignored path doesn't trigger event
func TestWatcher_RemoteWriteIgnored(t *testing.T) {
	watcher, err := filesystem.NewWatcher()
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Close()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "remote_write_test.txt")

	// Start watching the directory
	err = watcher.Add(tmpDir)
	if err != nil {
		t.Fatalf("Failed to add watch: %v", err)
	}

	// Simulate remote operation: ignore path before writing
	watcher.IgnorePath(testFile)

	// Write file (simulating remote operation)
	err = os.WriteFile(testFile, []byte("remote operation content"), 0644)
	if err != nil {
		t.Fatalf("Failed to write remote file: %v", err)
	}

	// Wait for potential events
	time.Sleep(100 * time.Millisecond)

	// Verify no events were generated
	select {
	case event := <-watcher.Events():
		t.Errorf("Remote write generated unexpected event: %+v", event)
	default:
		// Expected - no event for remote write
	}
}

// TestWatcher_IgnoreThenWatch verifies ignore -> write -> watch -> modify sequence
func TestWatcher_IgnoreThenWatch(t *testing.T) {
	watcher, err := filesystem.NewWatcher()
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Close()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "ignore_watch_test.txt")

	// Start watching the directory
	err = watcher.Add(tmpDir)
	if err != nil {
		t.Fatalf("Failed to add watch: %v", err)
	}

	// Step 1: Ignore path and write file - should not generate event
	watcher.IgnorePath(testFile)
	err = os.WriteFile(testFile, []byte("ignored write"), 0644)
	if err != nil {
		t.Fatalf("Failed to write file while ignored: %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	select {
	case event := <-watcher.Events():
		t.Errorf("Unexpected event during ignore phase: %+v", event)
	default:
		// Expected
	}

	// Step 2: Re-enable watching
	watcher.WatchPath(testFile)

	// Step 3: Modify file - should now generate event
	err = os.WriteFile(testFile, []byte("watched modification"), 0644)
	if err != nil {
		t.Fatalf("Failed to modify file after watch re-enabled: %v", err)
	}

	// Should receive event for the modification
	select {
	case event := <-watcher.Events():
		if event.Path != testFile {
			t.Errorf("Expected event for %s, got %s", testFile, event.Path)
		}
		if event.Operation != "update" {
			t.Errorf("Expected update operation, got %s", event.Operation)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("Expected event after re-enabling watch, but none received")
	}
}

// TestWatcher_ConcurrentIgnoreWatch verifies thread-safety of ignoreMap operations
func TestWatcher_ConcurrentIgnoreWatch(t *testing.T) {
	watcher, err := filesystem.NewWatcher()
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Close()

	const numGoroutines = 10
	const numOperations = 100

	tmpDir := t.TempDir()

	// Start watching the directory
	err = watcher.Add(tmpDir)
	if err != nil {
		t.Fatalf("Failed to add watch: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Launch concurrent goroutines performing ignore/watch operations
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < numOperations; j++ {
				testPath := filepath.Join(tmpDir, "concurrent_test_"+string(rune('0'+goroutineID))+"_"+string(rune('0'+j))+".txt")

				// Alternate between ignore and watch operations
				if j%2 == 0 {
					watcher.IgnorePath(testPath)
				} else {
					watcher.WatchPath(testPath)
				}
			}
		}(i)
	}

	wg.Wait()

	// Test should complete without panics or deadlocks
	// Basic functionality check
	testFile := filepath.Join(tmpDir, "final_check.txt")
	watcher.IgnorePath(testFile)

	err = os.WriteFile(testFile, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("Failed to create final test file: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	select {
	case event := <-watcher.Events():
		t.Errorf("Ignored file generated event after concurrent operations: %+v", event)
	default:
		// Expected - no event for ignored file
	}
}

// TestWatcher_IgnorePathDuringRemoteOperation simulates HandleIncomingFile flow
func TestWatcher_IgnorePathDuringRemoteOperation(t *testing.T) {
	watcher, err := filesystem.NewWatcher()
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Close()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "remote_operation.txt")

	// Start watching the directory
	err = watcher.Add(tmpDir)
	if err != nil {
		t.Fatalf("Failed to add watch: %v", err)
	}

	// Simulate HandleIncomingFile sequence:
	// 1. Ignore path
	watcher.IgnorePath(testFile)

	// 2. Write file (remote operation)
	err = os.WriteFile(testFile, []byte("remote file content"), 0644)
	if err != nil {
		t.Fatalf("Failed to write remote file: %v", err)
	}

	// 3. Wait a bit (simulating processing time)
	time.Sleep(50 * time.Millisecond)

	// 4. Re-enable watching with delay (as in HandleIncomingFile)
	go func() {
		time.Sleep(50 * time.Millisecond)
		watcher.WatchPath(testFile)
	}()

	// 5. Verify no events were generated during the remote operation
	select {
	case event := <-watcher.Events():
		t.Errorf("Remote operation generated unexpected event: %+v", event)
	case <-time.After(100 * time.Millisecond):
		// Expected - no event during remote operation
	}

	// Now that watching is re-enabled, modifications should generate events
	time.Sleep(100 * time.Millisecond) // Ensure watch is re-enabled

	err = os.WriteFile(testFile, []byte("local modification"), 0644)
	if err != nil {
		t.Fatalf("Failed to modify file after watch re-enabled: %v", err)
	}

	// Should receive event for local modification
	select {
	case event := <-watcher.Events():
		if event.Path != testFile {
			t.Errorf("Expected event for %s, got %s", testFile, event.Path)
		}
		if event.Operation != "update" {
			t.Errorf("Expected update operation, got %s", event.Operation)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("Expected event for local modification after remote operation")
	}
}

// TestWatcher_MultipleIgnorePatterns verifies multiple paths can be ignored independently
func TestWatcher_MultipleIgnorePatterns(t *testing.T) {
	watcher, err := filesystem.NewWatcher()
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Close()

	tmpDir := t.TempDir()

	// Start watching the directory
	err = watcher.Add(tmpDir)
	if err != nil {
		t.Fatalf("Failed to add watch: %v", err)
	}

	// Create multiple test files
	files := []string{
		filepath.Join(tmpDir, "ignored1.txt"),
		filepath.Join(tmpDir, "ignored2.txt"),
		filepath.Join(tmpDir, "watched.txt"),
	}

	// Ignore first two files
	for _, file := range files[:2] {
		watcher.IgnorePath(file)
	}

	// Create all files
	for i, file := range files {
		content := []byte("content " + string(rune('0'+i)))
		err = os.WriteFile(file, content, 0644)
		if err != nil {
			t.Fatalf("Failed to create file %s: %v", file, err)
		}
	}

	time.Sleep(100 * time.Millisecond)

	// Check events - only the non-ignored file should generate an event
	eventCount := 0
	timeout := time.After(200 * time.Millisecond)

events:
	for {
		select {
		case event := <-watcher.Events():
			eventCount++
			// Should only be for the watched file
			if event.Path != files[2] {
				t.Errorf("Received event for ignored file: %s", event.Path)
			}
		case <-timeout:
			break events
		}
	}

	if eventCount != 2 {
		t.Errorf("Expected exactly 2 events (create and update for watched file), got %d", eventCount)
	}
}

// TestWatcher_IgnoreNonExistentPath verifies ignoring non-existent paths works
func TestWatcher_IgnoreNonExistentPath(t *testing.T) {
	watcher, err := filesystem.NewWatcher()
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Close()

	tmpDir := t.TempDir()

	// Start watching the directory
	err = watcher.Add(tmpDir)
	if err != nil {
		t.Fatalf("Failed to add watch: %v", err)
	}

	// Ignore a path that doesn't exist yet
	nonExistentPath := filepath.Join(tmpDir, "future_file.txt")
	watcher.IgnorePath(nonExistentPath)

	// Now create the file
	err = os.WriteFile(nonExistentPath, []byte("created after ignore"), 0644)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Should not receive any events
	select {
	case event := <-watcher.Events():
		t.Errorf("Received unexpected event for pre-ignored path: %+v", event)
	default:
		// Expected - no event
	}
}
