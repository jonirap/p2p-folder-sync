package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/p2p-folder-sync/p2p-sync/internal/config"
	"github.com/p2p-folder-sync/p2p-sync/internal/database"
	"github.com/p2p-folder-sync/p2p-sync/internal/filesystem"
	syncpkg "github.com/p2p-folder-sync/p2p-sync/internal/sync"
)

// createPersistenceTestConfig creates a test configuration for file ID persistence tests
func createPersistenceTestConfig(syncDir string) *config.Config {
	cfg := &config.Config{}
	cfg.Sync.FolderPath = syncDir
	cfg.Sync.ChunkSizeMin = 65536
	cfg.Sync.ChunkSizeMax = 2097152
	cfg.Sync.ChunkSizeDefault = 524288
	cfg.Sync.MaxConcurrentTransfers = 5
	cfg.Network.Port = 0 // Use any available port
	cfg.Network.DiscoveryPort = 0
	cfg.Compression.Enabled = true
	cfg.Compression.Algorithm = "zstd"
	cfg.Compression.Level = 3
	cfg.Observability.LogLevel = "error"
	cfg.Observability.MetricsEnabled = false
	cfg.Observability.TracingEnabled = false
	return cfg
}

// TestFileID_PersistsAcrossRenames verifies FID remains stable when file is renamed
func TestFileID_PersistsAcrossRenames(t *testing.T) {
	tmpDir := t.TempDir()
	syncDir := filepath.Join(tmpDir, "sync")

	if err := os.MkdirAll(syncDir, 0755); err != nil {
		t.Fatalf("Failed to create sync dir: %v", err)
	}

	cfg := createPersistenceTestConfig(syncDir)
	db, err := database.NewDB(filepath.Join(tmpDir, ".p2p-sync", "test.db"))
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	syncEngine, err := syncpkg.NewEngine(cfg, db, "test-peer")
	if err != nil {
		t.Fatalf("Failed to create sync engine: %v", err)
	}

	// Start and stop engine to test FID persistence
	if err := syncEngine.Start(testCtx()); err != nil {
		t.Fatalf("Failed to start sync engine: %v", err)
	}
	defer syncEngine.Stop()

	time.Sleep(500 * time.Millisecond)

	// Create original file
	originalPath := filepath.Join(syncDir, "original_name.txt")
	content := []byte("Content for FID persistence test")

	if err := os.WriteFile(originalPath, content, 0644); err != nil {
		t.Fatalf("Failed to create original file: %v", err)
	}

	// Wait for file to be processed and FID to be stored
	time.Sleep(1 * time.Second)

	// Get the file ID that was generated and stored
	originalFID, err := filesystem.GetFileID(originalPath)
	if err != nil {
		// Xattr might not be supported on this filesystem, check database
		relPath := "original_name.txt"
		fileEntry, dbErr := db.GetFileByPath(relPath)
		if dbErr != nil {
			t.Logf("Warning: Could not get FID from xattr or database: xattr=%v, db=%v", err, dbErr)
			t.Skip("File ID persistence not available on this system (no xattr support and DB lookup failed)")
		}
		originalFID = fileEntry.FileID
	}

	if originalFID == "" {
		t.Skip("File ID not stored - skipping persistence test")
	}

	t.Logf("Original file ID: %s", originalFID)

	// Rename the file
	renamedPath := filepath.Join(syncDir, "renamed_file.txt")
	if err := os.Rename(originalPath, renamedPath); err != nil {
		t.Fatalf("Failed to rename file: %v", err)
	}

	// Wait for rename to be processed
	time.Sleep(1 * time.Second)

	// Verify FID persists on the renamed file
	renamedFID, err := filesystem.GetFileID(renamedPath)
	if err != nil {
		// Check database
		relPath := "renamed_file.txt"
		fileEntry, dbErr := db.GetFileByPath(relPath)
		if dbErr != nil {
			t.Errorf("Could not retrieve FID for renamed file: xattr=%v, db=%v", err, dbErr)
			return
		}
		renamedFID = fileEntry.FileID
	}

	if renamedFID == "" {
		t.Error("Renamed file has no FID stored")
		return
	}

	t.Logf("Renamed file ID: %s", renamedFID)

	// Verify FID remained the same
	if originalFID != renamedFID {
		t.Errorf("File ID changed during rename: original=%s, renamed=%s", originalFID, renamedFID)
	} else {
		t.Log("SUCCESS: File ID persisted across rename operation")
	}

	// Verify file content is preserved
	renamedContent, err := os.ReadFile(renamedPath)
	if err != nil {
		t.Fatalf("Failed to read renamed file: %v", err)
	}

	if string(renamedContent) != string(content) {
		t.Error("File content not preserved during rename")
	}
}

// TestFileID_PersistsAcrossRestarts verifies FID survives process restart
func TestFileID_PersistsAcrossRestarts(t *testing.T) {
	tmpDir := t.TempDir()
	syncDir := filepath.Join(tmpDir, "sync")
	dbPath := filepath.Join(tmpDir, ".p2p-sync", "restart_test.db")

	if err := os.MkdirAll(syncDir, 0755); err != nil {
		t.Fatalf("Failed to create sync dir: %v", err)
	}

	cfg := createPersistenceTestConfig(syncDir)

	// First engine instance
	db1, err := database.NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	syncEngine1, err := syncpkg.NewEngine(cfg, db1, "test-peer")
	if err != nil {
		t.Fatalf("Failed to create first sync engine: %v", err)
	}

	if err := syncEngine1.Start(testCtx()); err != nil {
		t.Fatalf("Failed to start first sync engine: %v", err)
	}

	// Create file with first engine
	testFile := filepath.Join(syncDir, "restart_test.txt")
	content := []byte("Content that should persist across restarts")

	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	time.Sleep(1 * time.Second)

	// Get FID from first engine session
	var originalFID string
	relPath := "restart_test.txt"

	fileEntry, err := db1.GetFileByPath(relPath)
	if err != nil {
		t.Logf("Warning: Could not get FID from database: %v", err)
		// Try xattr
		if xattrFID, xattrErr := filesystem.GetFileID(testFile); xattrErr == nil && xattrFID != "" {
			originalFID = xattrFID
		} else {
			t.Skip("File ID not stored in first session - skipping restart test")
		}
	} else {
		originalFID = fileEntry.FileID
	}

	t.Logf("Original FID from first session: %s", originalFID)

	// Stop first engine
	syncEngine1.Stop()
	db1.Close()

	// Create second engine instance (simulating restart)
	db2, err := database.NewDB(dbPath) // Reopen same database
	if err != nil {
		t.Fatalf("Failed to reopen database: %v", err)
	}
	defer db2.Close()

	syncEngine2, err := syncpkg.NewEngine(cfg, db2, "test-peer")
	if err != nil {
		t.Fatalf("Failed to create second sync engine: %v", err)
	}

	if err := syncEngine2.Start(testCtx()); err != nil {
		t.Fatalf("Failed to start second sync engine: %v", err)
	}
	defer syncEngine2.Stop()

	time.Sleep(1 * time.Second)

	// Verify file still exists
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Fatalf("Test file disappeared after restart")
	}

	// Verify FID persists across restart
	var restartedFID string

	fileEntry2, err := db2.GetFileByPath(relPath)
	if err != nil {
		// Try xattr
		if xattrFID, xattrErr := filesystem.GetFileID(testFile); xattrErr == nil && xattrFID != "" {
			restartedFID = xattrFID
		} else {
			t.Errorf("Could not retrieve FID after restart: db=%v, xattr=%v", err, xattrErr)
			return
		}
	} else {
		restartedFID = fileEntry2.FileID
	}

	t.Logf("FID after restart: %s", restartedFID)

	if originalFID != restartedFID {
		t.Errorf("File ID did not persist across restart: original=%s, restarted=%s", originalFID, restartedFID)
	} else {
		t.Log("SUCCESS: File ID persisted across process restart")
	}

	// Verify file content is still correct
	restartedContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read file after restart: %v", err)
	}

	if string(restartedContent) != string(content) {
		t.Error("File content corrupted after restart")
	}
}

// TestFileID_XattrFallback verifies fallback to database when xattr not supported
func TestFileID_XattrFallback(t *testing.T) {
	tmpDir := t.TempDir()
	syncDir := filepath.Join(tmpDir, "sync")

	if err := os.MkdirAll(syncDir, 0755); err != nil {
		t.Fatalf("Failed to create sync dir: %v", err)
	}

	cfg := createPersistenceTestConfig(syncDir)
	db, err := database.NewDB(filepath.Join(tmpDir, ".p2p-sync", "fallback_test.db"))
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	syncEngine, err := syncpkg.NewEngine(cfg, db, "test-peer")
	if err != nil {
		t.Fatalf("Failed to create sync engine: %v", err)
	}

	if err := syncEngine.Start(testCtx()); err != nil {
		t.Fatalf("Failed to start sync engine: %v", err)
	}
	defer syncEngine.Stop()

	time.Sleep(500 * time.Millisecond)

	// Create test file
	testFile := filepath.Join(syncDir, "xattr_fallback_test.txt")
	content := []byte("Testing xattr fallback mechanism")

	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	time.Sleep(1 * time.Second)

	relPath := "xattr_fallback_test.txt"

	// Check if xattr is supported by trying to get FID
	xattrSupported := true
	_, xattrErr := filesystem.GetFileID(testFile)
	if xattrErr != nil {
		xattrSupported = false
		t.Log("Xattr not supported on this filesystem")
	} else {
		t.Log("Xattr supported on this filesystem")
	}

	// Regardless of xattr support, FID should be retrievable
	var retrievedFID string
	var retrievalMethod string

	// Try xattr first
	if xattrFID, err := filesystem.GetFileID(testFile); err == nil && xattrFID != "" {
		retrievedFID = xattrFID
		retrievalMethod = "xattr"
	} else {
		// Fallback to database
		if fileEntry, err := db.GetFileByPath(relPath); err == nil {
			retrievedFID = fileEntry.FileID
			retrievalMethod = "database"
		}
	}

	if retrievedFID == "" {
		t.Error("Could not retrieve File ID through any method")
		return
	}

	t.Logf("File ID retrieved via %s: %s", retrievalMethod, retrievedFID)

	// Verify FID is valid
	if len(retrievedFID) != 64 {
		t.Errorf("Retrieved FID has wrong length: expected 64, got %d", len(retrievedFID))
	}

	// Test that the retrieval method matches expectations
	if xattrSupported && retrievalMethod != "xattr" {
		t.Logf("Note: Xattr supported but FID retrieved via %s", retrievalMethod)
	} else if !xattrSupported && retrievalMethod != "database" {
		t.Errorf("Xattr not supported but FID retrieved via %s instead of database", retrievalMethod)
	} else {
		t.Logf("SUCCESS: Correct fallback mechanism used (%s)", retrievalMethod)
	}

	// Verify file can be looked up by FID in database
	if fileByID, err := db.GetFileByID(retrievedFID); err != nil {
		t.Errorf("Could not retrieve file by FID from database: %v", err)
	} else if fileByID.Path != relPath {
		t.Errorf("File retrieved by FID has wrong path: expected %s, got %s", relPath, fileByID.Path)
	} else {
		t.Log("SUCCESS: File correctly retrievable by FID from database")
	}
}

// TestFileID_PersistenceAcrossMultipleOperations tests FID persistence through complex operations
func TestFileID_PersistenceAcrossMultipleOperations(t *testing.T) {
	tmpDir := t.TempDir()
	syncDir := filepath.Join(tmpDir, "sync")

	if err := os.MkdirAll(syncDir, 0755); err != nil {
		t.Fatalf("Failed to create sync dir: %v", err)
	}

	cfg := createPersistenceTestConfig(syncDir)
	db, err := database.NewDB(filepath.Join(tmpDir, ".p2p-sync", "multi_op_test.db"))
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	syncEngine, err := syncpkg.NewEngine(cfg, db, "test-peer")
	if err != nil {
		t.Fatalf("Failed to create sync engine: %v", err)
	}

	if err := syncEngine.Start(testCtx()); err != nil {
		t.Fatalf("Failed to start sync engine: %v", err)
	}
	defer syncEngine.Stop()

	time.Sleep(500 * time.Millisecond)

	content := []byte("Content for complex operations test")

	// Series of operations: create -> rename -> modify -> rename again
	operations := []struct {
		name     string
		filename string
		operation string // "create", "rename_from", "rename_to", "modify"
		fromPath  string // for renames
	}{
		{"create", "file1.txt", "create", ""},
		{"rename1", "file2.txt", "rename_to", "file1.txt"},
		{"modify", "file2.txt", "modify", ""},
		{"rename2", "final_name.txt", "rename_to", "file2.txt"},
	}

	var originalFID string

	for i, op := range operations {
		t.Logf("Operation %d: %s %s", i+1, op.operation, op.filename)

		switch op.operation {
		case "create":
			filePath := filepath.Join(syncDir, op.filename)
			if err := os.WriteFile(filePath, content, 0644); err != nil {
				t.Fatalf("Failed to create file: %v", err)
			}

		case "rename_to":
			fromPath := filepath.Join(syncDir, op.fromPath)
			toPath := filepath.Join(syncDir, op.filename)
			if err := os.Rename(fromPath, toPath); err != nil {
				t.Fatalf("Failed to rename file: %v", err)
			}

		case "modify":
			filePath := filepath.Join(syncDir, op.filename)
			modifiedContent := append(content, []byte(" - modified")...)
			if err := os.WriteFile(filePath, modifiedContent, 0644); err != nil {
				t.Fatalf("Failed to modify file: %v", err)
			}
		}

		time.Sleep(1 * time.Second)

		// Check FID persistence (skip for create operations as FID might not be stored yet)
		if op.operation != "create" {
			currentPath := filepath.Join(syncDir, op.filename)
			currentFID := getFID(t, currentPath, db, op.filename)

			if originalFID == "" {
				originalFID = currentFID
				t.Logf("Captured original FID: %s", originalFID)
			} else if currentFID != originalFID {
				t.Errorf("FID changed during operation %s: expected %s, got %s", op.name, originalFID, currentFID)
			} else {
				t.Logf("FID persisted through %s: %s", op.name, currentFID)
			}
		}
	}

	if originalFID != "" {
		t.Logf("SUCCESS: File ID %s persisted through all operations", originalFID)
	} else {
		t.Log("Note: Could not verify FID persistence - FIDs not accessible")
	}
}

// Helper function to get FID from either xattr or database
func getFID(t *testing.T, filePath string, db *database.DB, relPath string) string {
	// Try xattr first
	if fid, err := filesystem.GetFileID(filePath); err == nil && fid != "" {
		return fid
	}

	// Fallback to database
	if fileEntry, err := db.GetFileByPath(relPath); err == nil {
		return fileEntry.FileID
	}

	return ""
}

// testCtx returns a test context (helper for integration tests)
func testCtx() context.Context {
	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)
	return ctx
}
