package filesystem_test

import (
	"sync"
	"testing"
	"time"

	"github.com/p2p-folder-sync/p2p-sync/internal/filesystem"
)

// TestRenameDetector_RecordDelete verifies deletion recording stores FID, checksum, size, mtime
func TestRenameDetector_RecordDelete(t *testing.T) {
	detector := filesystem.NewRenameDetector()
	defer detector.Close()

	fileID := "test-file-id-123"
	checksum := "abc123checksum"
	size := int64(1024)
	mtime := time.Now()
	path := "/test/path/file.txt"

	// Record deletion
	detector.RecordDelete(fileID, checksum, path, size, mtime)

	// Verify deletion was recorded
	count := detector.GetRecentDeletesCount()
	if count != 1 {
		t.Errorf("Expected 1 recent delete, got %d", count)
	}
}

// TestRenameDetector_CheckRename_MatchingFIDAndChecksum verifies FID match + checksum match = rename
// Spec: If match found AND checksum matches: RENAME operation
func TestRenameDetector_CheckRename_MatchingFIDAndChecksum(t *testing.T) {
	detector := filesystem.NewRenameDetector()
	defer detector.Close()

	fileID := "test-file-id-456"
	checksum := "matching-checksum-789"
	size := int64(2048)
	mtime := time.Now()
	path := "/original/path/file.txt"

	// First record a deletion
	detector.RecordDelete(fileID, checksum, path, size, mtime)

	// Then check for rename with same FID and checksum
	isRename, oldPath, err := detector.CheckRename(fileID, checksum, size)
	if err != nil {
		t.Fatalf("CheckRename failed: %v", err)
	}

	if !isRename {
		t.Error("Expected rename detection when FID and checksum match")
	}

	if oldPath != path {
		t.Errorf("Expected old path %s, got %s", path, oldPath)
	}

	// Entry should be removed after successful rename detection
	count := detector.GetRecentDeletesCount()
	if count != 0 {
		t.Errorf("Expected entry to be removed after rename detection, got %d entries", count)
	}
}

// TestRenameDetector_CheckRename_MatchingFIDDifferentChecksum verifies FID match + checksum mismatch = edit (not rename)
// Spec: If match found BUT checksum differs: DELETE + CREATE (edit)
func TestRenameDetector_CheckRename_MatchingFIDDifferentChecksum(t *testing.T) {
	detector := filesystem.NewRenameDetector()
	defer detector.Close()

	fileID := "test-file-id-789"
	originalChecksum := "original-checksum-abc"
	newChecksum := "modified-checksum-def"
	size := int64(3072)
	mtime := time.Now()
	path := "/original/path/file.txt"

	// First record a deletion
	detector.RecordDelete(fileID, originalChecksum, path, size, mtime)

	// Then check for rename with same FID but different checksum (edit scenario)
	isRename, oldPath, err := detector.CheckRename(fileID, newChecksum, size)
	if err != nil {
		t.Fatalf("CheckRename failed: %v", err)
	}

	if isRename {
		t.Error("Expected no rename detection when checksum differs (edit scenario)")
	}

	if oldPath != "" {
		t.Errorf("Expected empty old path for edit scenario, got %s", oldPath)
	}

	// Entry should be removed after checking (even for edit)
	count := detector.GetRecentDeletesCount()
	if count != 0 {
		t.Errorf("Expected entry to be removed after edit detection, got %d entries", count)
	}
}

// TestRenameDetector_CheckRename_NoMatch verifies no FID match = create (not rename)
// Spec: If no match: CREATE operation
func TestRenameDetector_CheckRename_NoMatch(t *testing.T) {
	detector := filesystem.NewRenameDetector()
	defer detector.Close()

	fileID := "nonexistent-file-id"
	checksum := "some-checksum"
	size := int64(1024)

	// Check for rename without any prior deletions
	isRename, oldPath, err := detector.CheckRename(fileID, checksum, size)
	if err != nil {
		t.Fatalf("CheckRename failed: %v", err)
	}

	if isRename {
		t.Error("Expected no rename detection when no matching deletion exists")
	}

	if oldPath != "" {
		t.Errorf("Expected empty old path when no match, got %s", oldPath)
	}

	// No entries should exist
	count := detector.GetRecentDeletesCount()
	if count != 0 {
		t.Errorf("Expected no entries, got %d", count)
	}
}

// TestRenameDetector_TTLExpiration verifies entries expire after 5 seconds (RenameDetectionTTL)
func TestRenameDetector_TTLExpiration(t *testing.T) {
	detector := filesystem.NewRenameDetector()
	defer detector.Close()

	fileID := "expiring-file-id"
	checksum := "expiring-checksum"
	size := int64(512)
	mtime := time.Now()
	path := "/expiring/path/file.txt"

	// Record deletion
	detector.RecordDelete(fileID, checksum, path, size, mtime)

	// Verify it's recorded
	count := detector.GetRecentDeletesCount()
	if count != 1 {
		t.Errorf("Expected 1 recent delete before expiration, got %d", count)
	}

	// Wait for TTL to expire (5 seconds + small buffer)
	time.Sleep(5100 * time.Millisecond)

	// Check that entry has expired
	isRename, oldPath, err := detector.CheckRename(fileID, checksum, size)
	if err != nil {
		t.Fatalf("CheckRename failed: %v", err)
	}

	if isRename {
		t.Error("Expected no rename detection after TTL expiration")
	}

	if oldPath != "" {
		t.Errorf("Expected empty old path after expiration, got %s", oldPath)
	}
}

// TestRenameDetector_SizeMismatch verifies same FID and checksum but different size = not rename
func TestRenameDetector_SizeMismatch(t *testing.T) {
	detector := filesystem.NewRenameDetector()
	defer detector.Close()

	fileID := "size-mismatch-file-id"
	checksum := "same-checksum"
	originalSize := int64(1024)
	newSize := int64(2048) // Different size
	mtime := time.Now()
	path := "/size/mismatch/file.txt"

	// Record deletion with original size
	detector.RecordDelete(fileID, checksum, path, originalSize, mtime)

	// Check for rename with same FID/checksum but different size
	isRename, oldPath, err := detector.CheckRename(fileID, checksum, newSize)
	if err != nil {
		t.Fatalf("CheckRename failed: %v", err)
	}

	if isRename {
		t.Error("Expected no rename detection when size differs")
	}

	if oldPath != "" {
		t.Errorf("Expected empty old path when size differs, got %s", oldPath)
	}

	// Entry should still be removed
	count := detector.GetRecentDeletesCount()
	if count != 0 {
		t.Errorf("Expected entry to be removed after size mismatch check, got %d entries", count)
	}
}

// TestRenameDetector_Cleanup verifies cleanup goroutine removes expired entries
func TestRenameDetector_Cleanup(t *testing.T) {
	detector := filesystem.NewRenameDetector()
	defer detector.Close()

	// Record multiple deletions
	for i := 0; i < 3; i++ {
		fileID := "cleanup-test-file-" + string(rune('0'+i))
		checksum := "cleanup-checksum-" + string(rune('0'+i))
		size := int64(100 * (i + 1))
		mtime := time.Now()
		path := "/cleanup/test/file" + string(rune('0'+i)) + ".txt"

		detector.RecordDelete(fileID, checksum, path, size, mtime)
	}

	// Verify all are recorded
	count := detector.GetRecentDeletesCount()
	if count != 3 {
		t.Errorf("Expected 3 recent deletes initially, got %d", count)
	}

	// Wait for cleanup cycle (1 second interval) + TTL expiration
	time.Sleep(6100 * time.Millisecond)

	// Check that all entries have been cleaned up
	finalCount := detector.GetRecentDeletesCount()
	if finalCount != 0 {
		t.Errorf("Expected all entries to be cleaned up after TTL, got %d", finalCount)
	}
}

// TestRenameDetector_ConcurrentAccess verifies thread-safety of recentDeletes map
func TestRenameDetector_ConcurrentAccess(t *testing.T) {
	detector := filesystem.NewRenameDetector()
	defer detector.Close()

	const numGoroutines = 10
	const numOperations = 50

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Launch multiple goroutines performing operations concurrently
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < numOperations; j++ {
				fileID := "concurrent-file-" + string(rune('0'+goroutineID)) + "-" + string(rune('0'+j))
				checksum := "concurrent-checksum-" + string(rune('0'+goroutineID)) + "-" + string(rune('0'+j))
				size := int64((goroutineID + 1) * 100)
				path := "/concurrent/test/file" + string(rune('0'+goroutineID)) + "-" + string(rune('0'+j)) + ".txt"

				// Mix of record and check operations
				if j%2 == 0 {
					detector.RecordDelete(fileID, checksum, path, size, time.Now())
				} else {
					detector.CheckRename(fileID, checksum, size)
				}
			}
		}(i)
	}

	wg.Wait()

	// Test should complete without panics or race conditions
	// We don't check exact counts since concurrent operations make it unpredictable,
	// but the detector should remain functional
	finalCount := detector.GetRecentDeletesCount()
	if finalCount < 0 {
		t.Error("Invalid negative count after concurrent operations")
	}

	t.Logf("Concurrent access test completed with %d final entries", finalCount)
}

// TestRenameDetector_MultipleEntries verifies handling of multiple recent deletes
func TestRenameDetector_MultipleEntries(t *testing.T) {
	detector := filesystem.NewRenameDetector()
	defer detector.Close()

	// Record multiple deletions
	entries := []struct {
		fileID   string
		checksum string
		size     int64
		path     string
	}{
		{"file1", "checksum1", 1024, "/path/file1.txt"},
		{"file2", "checksum2", 2048, "/path/file2.txt"},
		{"file3", "checksum3", 3072, "/path/file3.txt"},
	}

	for _, entry := range entries {
		detector.RecordDelete(entry.fileID, entry.checksum, entry.path, entry.size, time.Now())
	}

	// Verify all recorded
	count := detector.GetRecentDeletesCount()
	if count != len(entries) {
		t.Errorf("Expected %d entries, got %d", len(entries), count)
	}

	// Test matching one entry
	isRename, oldPath, err := detector.CheckRename(entries[1].fileID, entries[1].checksum, entries[1].size)
	if err != nil {
		t.Fatalf("CheckRename failed: %v", err)
	}

	if !isRename {
		t.Error("Expected rename detection for matching entry")
	}

	if oldPath != entries[1].path {
		t.Errorf("Expected old path %s, got %s", entries[1].path, oldPath)
	}

	// Verify only that entry was removed
	remainingCount := detector.GetRecentDeletesCount()
	if remainingCount != len(entries)-1 {
		t.Errorf("Expected %d entries remaining, got %d", len(entries)-1, remainingCount)
	}

	// Test non-matching entry
	isRename2, oldPath2, err := detector.CheckRename("nonexistent", "checksum", 999)
	if err != nil {
		t.Fatalf("CheckRename failed for nonexistent: %v", err)
	}

	if isRename2 {
		t.Error("Expected no rename detection for nonexistent entry")
	}

	if oldPath2 != "" {
		t.Errorf("Expected empty old path for nonexistent, got %s", oldPath2)
	}
}
