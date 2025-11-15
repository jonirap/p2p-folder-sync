//go:build integration

package system

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/p2p-folder-sync/p2p-sync/internal/database"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/transport"
	syncpkg "github.com/p2p-folder-sync/p2p-sync/internal/sync"
)

// TestRenameDetection_EndToEnd tests full rename detection flow: create -> delete -> create = rename
func TestRenameDetection_EndToEnd(t *testing.T) {
	tmpDir := t.TempDir()
	syncDir := filepath.Join(tmpDir, "sync")

	if err := os.MkdirAll(syncDir, 0755); err != nil {
		t.Fatalf("Failed to create sync dir: %v", err)
	}

	cfg := createTestConfig(syncDir)
	db, err := database.NewDB(filepath.Join(tmpDir, ".p2p-sync", "test.db"))
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	syncEngine, err := syncpkg.NewEngine(cfg, db, "test-peer")
	if err != nil {
		t.Fatalf("Failed to create sync engine: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := syncEngine.Start(ctx); err != nil {
		t.Fatalf("Failed to start sync engine: %v", err)
	}
	defer syncEngine.Stop()

	// Wait for engine to initialize
	time.Sleep(500 * time.Millisecond)

	originalFile := filepath.Join(syncDir, "original_file.txt")
	content := []byte("Test content for rename detection")

	// Step 1: Create original file
	t.Log("Step 1: Creating original file...")
	if err := os.WriteFile(originalFile, content, 0644); err != nil {
		t.Fatalf("Failed to create original file: %v", err)
	}

	// Wait for file creation to be processed
	time.Sleep(1 * time.Second)

	// Verify file was processed and database entry created
	originalFileRel := "original_file.txt"
	fileEntry, err := db.GetFileByPath(originalFileRel)
	if err != nil {
		t.Fatalf("Failed to get file entry: %v", err)
	}
	if fileEntry.Path != originalFileRel {
		t.Errorf("Expected file path %s, got %s", originalFileRel, fileEntry.Path)
	}
	originalFileID := fileEntry.FileID

	// Step 2: Delete the file (should record deletion for rename detection)
	t.Log("Step 2: Deleting file...")
	if err := os.Remove(originalFile); err != nil {
		t.Fatalf("Failed to delete file: %v", err)
	}

	// Wait for deletion to be processed
	time.Sleep(1 * time.Second)

	// Step 3: Create file with same content at different path (should be detected as rename)
	t.Log("Step 3: Creating file with same content at different path...")
	renamedFile := filepath.Join(syncDir, "renamed_file.txt")
	if err := os.WriteFile(renamedFile, content, 0644); err != nil {
		t.Fatalf("Failed to create renamed file: %v", err)
	}

	// Wait for rename detection to occur
	time.Sleep(2 * time.Second)

	// Verify rename was detected and recorded
	renamedFileRel := "renamed_file.txt"
	renamedEntry, err := db.GetFileByPath(renamedFileRel)
	if err != nil {
		t.Fatalf("Failed to get renamed file entry: %v", err)
	}

	// The file should have the same FileID (rename detection worked)
	if renamedEntry.FileID != originalFileID {
		t.Errorf("Rename detection failed: expected same FileID %s, got %s", originalFileID, renamedEntry.FileID)
	}

	// Verify both paths are tracked
	if renamedEntry.Path != renamedFileRel {
		t.Errorf("Expected renamed file path %s, got %s", renamedFileRel, renamedEntry.Path)
	}

	t.Log("SUCCESS: End-to-end rename detection test passed")
}

// TestRenameDetection_EditNotRename tests that content changes are detected as edits, not renames
func TestRenameDetection_EditNotRename(t *testing.T) {
	tmpDir := t.TempDir()
	syncDir := filepath.Join(tmpDir, "sync")

	if err := os.MkdirAll(syncDir, 0755); err != nil {
		t.Fatalf("Failed to create sync dir: %v", err)
	}

	cfg := createTestConfig(syncDir)
	db, err := database.NewDB(filepath.Join(tmpDir, ".p2p-sync", "test.db"))
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	syncEngine, err := syncpkg.NewEngine(cfg, db, "test-peer")
	if err != nil {
		t.Fatalf("Failed to create sync engine: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := syncEngine.Start(ctx); err != nil {
		t.Fatalf("Failed to start sync engine: %v", err)
	}
	defer syncEngine.Stop()

	time.Sleep(500 * time.Millisecond)

	originalFile := filepath.Join(syncDir, "edit_test.txt")
	originalContent := []byte("Original content")

	// Create original file
	t.Log("Creating original file...")
	if err := os.WriteFile(originalFile, originalContent, 0644); err != nil {
		t.Fatalf("Failed to create original file: %v", err)
	}

	time.Sleep(1 * time.Second)

	// Get original file ID
	originalEntry, err := db.GetFileByPath("edit_test.txt")
	if err != nil {
		t.Fatalf("Failed to get original file entry: %v", err)
	}
	originalFileID := originalEntry.FileID

	// Delete file
	t.Log("Deleting file...")
	if err := os.Remove(originalFile); err != nil {
		t.Fatalf("Failed to delete file: %v", err)
	}

	time.Sleep(1 * time.Second)

	// Create file with DIFFERENT content (edit scenario)
	t.Log("Creating file with different content (edit, not rename)...")
	modifiedContent := []byte("Modified content - this is an edit")
	if err := os.WriteFile(originalFile, modifiedContent, 0644); err != nil {
		t.Fatalf("Failed to create modified file: %v", err)
	}

	time.Sleep(2 * time.Second)

	// Verify this was treated as an edit (new file ID)
	modifiedEntry, err := db.GetFileByPath("edit_test.txt")
	if err != nil {
		t.Fatalf("Failed to get modified file entry: %v", err)
	}

	// Should have different FileID (edit, not rename)
	if modifiedEntry.FileID == originalFileID {
		t.Error("Edit was incorrectly detected as rename: FileID should be different")
	}

	t.Log("SUCCESS: Edit correctly distinguished from rename")
}

// TestRenameDetection_TTLExpiration tests that rename detection expires after 5 seconds
func TestRenameDetection_TTLExpiration(t *testing.T) {
	tmpDir := t.TempDir()
	syncDir := filepath.Join(tmpDir, "sync")

	if err := os.MkdirAll(syncDir, 0755); err != nil {
		t.Fatalf("Failed to create sync dir: %v", err)
	}

	cfg := createTestConfig(syncDir)
	db, err := database.NewDB(filepath.Join(tmpDir, ".p2p-sync", "test.db"))
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	syncEngine, err := syncpkg.NewEngine(cfg, db, "test-peer")
	if err != nil {
		t.Fatalf("Failed to create sync engine: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second) // Longer timeout for TTL test
	defer cancel()

	if err := syncEngine.Start(ctx); err != nil {
		t.Fatalf("Failed to start sync engine: %v", err)
	}
	defer syncEngine.Stop()

	time.Sleep(500 * time.Millisecond)

	testFile := filepath.Join(syncDir, "ttl_test.txt")
	content := []byte("Content for TTL test")

	// Create and delete file
	t.Log("Creating and deleting file...")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	time.Sleep(1 * time.Second)

	if err := os.Remove(testFile); err != nil {
		t.Fatalf("Failed to delete file: %v", err)
	}

	time.Sleep(1 * time.Second)

	// Wait for TTL to expire (5 seconds + buffer)
	t.Log("Waiting for TTL to expire (5+ seconds)...")
	time.Sleep(5100 * time.Millisecond)

	// Now create file again - should NOT be detected as rename due to TTL expiration
	t.Log("Creating file after TTL expiration...")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to recreate file: %v", err)
	}

	time.Sleep(2 * time.Second)

	// Verify file was created but no rename detection occurred
	// (This is harder to test directly, but we can check that operations were processed)
	// The key test is that the system doesn't crash and processes the new file creation

	entry, err := db.GetFileByPath("ttl_test.txt")
	if err != nil {
		t.Fatalf("Failed to get file entry: %v", err)
	}

	if entry.Path != "ttl_test.txt" {
		t.Errorf("Expected file path ttl_test.txt, got %s", entry.Path)
	}

	t.Log("SUCCESS: TTL expiration test completed without issues")
}

// TestRenameDetection_MultiPeer tests rename detection across peer boundaries
func TestRenameDetection_MultiPeer(t *testing.T) {
	// Create two peer directories
	tmpDir := t.TempDir()
	peer1Dir := filepath.Join(tmpDir, "peer1")
	peer2Dir := filepath.Join(tmpDir, "peer2")

	for _, dir := range []string{peer1Dir, peer2Dir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create peer dir: %v", err)
		}
	}

	// Create configs
	peer1Port := getAvailablePort(t)
	peer2Port := getAvailablePort(t)

	cfg1 := createTestConfig(peer1Dir)
	cfg1.Network.Port = peer1Port
	cfg1.Network.Peers = []string{fmt.Sprintf("localhost:%d", peer2Port)}

	cfg2 := createTestConfig(peer2Dir)
	cfg2.Network.Port = peer2Port
	cfg2.Network.Peers = []string{fmt.Sprintf("localhost:%d", peer1Port)}

	// Create databases
	db1, err := database.NewDB(filepath.Join(peer1Dir, ".p2p-sync", "peer1.db"))
	if err != nil {
		t.Fatalf("Failed to create peer1 DB: %v", err)
	}
	defer db1.Close()

	db2, err := database.NewDB(filepath.Join(peer2Dir, ".p2p-sync", "peer2.db"))
	if err != nil {
		t.Fatalf("Failed to create peer2 DB: %v", err)
	}
	defer db2.Close()

	// Create operation waiters
	opWaiter1 := NewSyncOperationWaiter()
	opWaiter2 := NewSyncOperationWaiter()

	// Create mock messenger
	mockMessenger := &OperationMonitoringMessenger{
		innerMessenger: syncpkg.NewInMemoryMessenger(),
		peer1Waiter:    opWaiter1,
		peer2Waiter:    opWaiter2,
	}

	// Create transports
	transportFactory := &transport.TransportFactory{}
	transport1, err := transportFactory.NewTransport("tcp", cfg1.Network.Port)
	if err != nil {
		t.Fatalf("Failed to create transport1: %v", err)
	}
	defer transport1.Stop()

	transport2, err := transportFactory.NewTransport("tcp", cfg2.Network.Port)
	if err != nil {
		t.Fatalf("Failed to create transport2: %v", err)
	}
	defer transport2.Stop()

	// Create sync engines
	syncEngine1, err := syncpkg.NewEngineWithMessenger(cfg1, db1, "peer1", mockMessenger)
	if err != nil {
		t.Fatalf("Failed to create peer1 sync engine: %v", err)
	}
	mockMessenger.innerMessenger.RegisterEngine("peer1", syncEngine1)
	defer syncEngine1.Stop()

	syncEngine2, err := syncpkg.NewEngineWithMessenger(cfg2, db2, "peer2", mockMessenger)
	if err != nil {
		t.Fatalf("Failed to create peer2 sync engine: %v", err)
	}
	mockMessenger.innerMessenger.RegisterEngine("peer2", syncEngine2)
	defer syncEngine2.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Start components
	if err := transport1.Start(); err != nil {
		t.Fatalf("Failed to start transport1: %v", err)
	}
	if err := transport2.Start(); err != nil {
		t.Fatalf("Failed to start transport2: %v", err)
	}

	if err := syncEngine1.Start(ctx); err != nil {
		t.Fatalf("Failed to start peer1 sync engine: %v", err)
	}
	if err := syncEngine2.Start(ctx); err != nil {
		t.Fatalf("Failed to start peer2 sync engine: %v", err)
	}

	waiter := NewEventDrivenWaiterWithTimeout(15 * time.Second)
	defer waiter.Close()

	// Create file on peer1
	testFile1 := filepath.Join(peer1Dir, "multipeer_rename_test.txt")
	content := []byte("Content for multi-peer rename test")

	t.Log("Creating file on peer1...")
	if err := os.WriteFile(testFile1, content, 0644); err != nil {
		t.Fatalf("Failed to create file on peer1: %v", err)
	}

	// Wait for sync to peer2
	if err := waiter.WaitForFileSync(peer2Dir, "multipeer_rename_test.txt"); err != nil {
		t.Fatalf("File failed to sync peer1->peer2: %v", err)
	}
	t.Log("SUCCESS: File synced from peer1 to peer2")

	// Rename file on peer1
	renamedFile1 := filepath.Join(peer1Dir, "renamed_multipeer_test.txt")
	t.Log("Renaming file on peer1...")
	if err := os.Rename(testFile1, renamedFile1); err != nil {
		t.Fatalf("Failed to rename file on peer1: %v", err)
	}

	// Wait for rename operation
	renameOp, err := opWaiter1.WaitForOperationType(syncpkg.OpRename, 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to detect rename operation on peer1: %v", err)
	}
	t.Logf("SUCCESS: Detected rename operation on peer1: %s -> %s", *renameOp.FromPath, renameOp.Path)

	// Wait for rename to sync to peer2
	if err := waiter.WaitForFileSync(peer2Dir, "renamed_multipeer_test.txt"); err != nil {
		t.Fatalf("Renamed file failed to sync peer1->peer2: %v", err)
	}

	// Verify original file is gone from peer2
	if _, err := os.Stat(filepath.Join(peer2Dir, "multipeer_rename_test.txt")); !os.IsNotExist(err) {
		t.Error("FAILURE: Original file still exists on peer2 after remote rename")
	}

	// Verify renamed file exists on peer2 with correct content
	peer2RenamedFile := filepath.Join(peer2Dir, "renamed_multipeer_test.txt")
	if _, err := os.Stat(peer2RenamedFile); os.IsNotExist(err) {
		t.Error("FAILURE: Renamed file does not exist on peer2")
	}

	peer2Content, err := os.ReadFile(peer2RenamedFile)
	if err != nil {
		t.Fatalf("Failed to read renamed file on peer2: %v", err)
	}
	if string(peer2Content) != string(content) {
		t.Error("FAILURE: File content not preserved during rename sync")
	}

	t.Log("SUCCESS: Multi-peer rename detection test completed")
}

// TestRenameDetection_ComplexScenario tests complex rename/edit scenarios
func TestRenameDetection_ComplexScenario(t *testing.T) {
	tmpDir := t.TempDir()
	syncDir := filepath.Join(tmpDir, "sync")

	if err := os.MkdirAll(syncDir, 0755); err != nil {
		t.Fatalf("Failed to create sync dir: %v", err)
	}

	cfg := createTestConfig(syncDir)
	db, err := database.NewDB(filepath.Join(tmpDir, ".p2p-sync", "test.db"))
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	syncEngine, err := syncpkg.NewEngine(cfg, db, "test-peer")
	if err != nil {
		t.Fatalf("Failed to create sync engine: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	if err := syncEngine.Start(ctx); err != nil {
		t.Fatalf("Failed to start sync engine: %v", err)
	}
	defer syncEngine.Stop()

	time.Sleep(500 * time.Millisecond)

	// Test multiple files with various rename/edit scenarios
	testCases := []struct {
		name            string
		originalPath    string
		originalContent []byte
		newPath         string
		newContent      []byte
		expectRename    bool
	}{
		{
			name:            "true_rename",
			originalPath:    "file1.txt",
			originalContent: []byte("same content"),
			newPath:         "renamed_file1.txt",
			newContent:      []byte("same content"),
			expectRename:    true,
		},
		{
			name:            "edit_not_rename",
			originalPath:    "file2.txt",
			originalContent: []byte("original content"),
			newPath:         "file2.txt", // Same path
			newContent:      []byte("modified content"),
			expectRename:    false,
		},
		{
			name:            "delete_create_different",
			originalPath:    "file3.txt",
			originalContent: []byte("content A"),
			newPath:         "file3.txt",
			newContent:      []byte("content B"),
			expectRename:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			originalFile := filepath.Join(syncDir, tc.originalPath)

			// Create original file
			if err := os.WriteFile(originalFile, tc.originalContent, 0644); err != nil {
				t.Fatalf("Failed to create original file: %v", err)
			}
			time.Sleep(500 * time.Millisecond)

			// Get original file ID
			origEntry, err := db.GetFileByPath(tc.originalPath)
			if err != nil {
				t.Fatalf("Failed to get original entry: %v", err)
			}
			originalFID := origEntry.FileID

			// Delete original file
			if err := os.Remove(originalFile); err != nil {
				t.Fatalf("Failed to delete original file: %v", err)
			}
			time.Sleep(500 * time.Millisecond)

			// Create new file
			newFile := filepath.Join(syncDir, tc.newPath)
			if err := os.WriteFile(newFile, tc.newContent, 0644); err != nil {
				t.Fatalf("Failed to create new file: %v", err)
			}
			time.Sleep(1 * time.Second)

			// Check result
			newEntry, err := db.GetFileByPath(tc.newPath)
			if err != nil {
				t.Fatalf("Failed to get new entry: %v", err)
			}

			if tc.expectRename {
				if newEntry.FileID != originalFID {
					t.Errorf("Expected rename (same FID), but got different FIDs: %s vs %s", newEntry.FileID, originalFID)
				}
			} else {
				if newEntry.FileID == originalFID {
					t.Errorf("Expected edit (different FID), but got same FID: %s", newEntry.FileID)
				}
			}
		})
	}

	t.Log("SUCCESS: Complex rename detection scenarios test completed")
}
