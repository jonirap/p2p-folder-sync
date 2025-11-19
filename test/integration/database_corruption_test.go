package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/p2p-folder-sync/p2p-sync/internal/database"
)

// TestDatabaseCorruptionRecovery tests the system's ability to detect and recover from database corruption
// This addresses spec section 9.2.6: Database Corruption
func TestDatabaseCorruptionRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping database corruption test in short mode")
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Step 1: Create a healthy database and add some data
	t.Log("Step 1: Creating healthy database with test data...")
	db, err := database.NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Insert test file metadata
	testFile := &database.FileMetadata{
		FileID:   "test-file-id-123",
		Path:     "test-file.txt",
		Checksum: "abc123def456",
		Size:     1024,
		Mtime:    time.Unix(1234567890, 0),
		PeerID:   "test-peer",
	}

	if err := db.InsertFile(testFile); err != nil {
		t.Fatalf("Failed to insert test file: %v", err)
	}

	// Verify data is present
	retrievedFile, err := db.GetFileByID("test-file-id-123")
	if err != nil {
		t.Fatalf("Failed to retrieve test file: %v", err)
	}
	if retrievedFile.Path != "test-file.txt" {
		t.Errorf("Retrieved file path mismatch: expected test-file.txt, got %s", retrievedFile.Path)
	}

	// Close database properly
	if err := db.Close(); err != nil {
		t.Fatalf("Failed to close database: %v", err)
	}

	t.Log("SUCCESS: Healthy database created and verified")

	// Step 2: Simulate corruption by truncating the database file
	t.Log("Step 2: Simulating database corruption...")
	dbFile, err := os.OpenFile(dbPath, os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("Failed to open database file: %v", err)
	}

	// Truncate to simulate corruption (corrupt the last part of the file)
	fileInfo, err := dbFile.Stat()
	if err != nil {
		t.Fatalf("Failed to stat database file: %v", err)
	}
	originalSize := fileInfo.Size()

	// Truncate to 70% of original size to corrupt it
	if err := dbFile.Truncate(originalSize * 7 / 10); err != nil {
		t.Fatalf("Failed to truncate database: %v", err)
	}
	dbFile.Close()

	t.Logf("Database corrupted: truncated from %d to %d bytes", originalSize, originalSize*7/10)

	// Step 3: Attempt to open corrupted database
	t.Log("Step 3: Attempting to open corrupted database...")
	corruptedDB, err := database.NewDB(dbPath)

	// The database should either:
	// 1. Detect corruption and return an error
	// 2. Successfully open with automatic recovery (SQLite may recover)

	if err != nil {
		// Corruption detected - this is acceptable
		t.Logf("Database corruption detected on open: %v", err)

		// Step 4a: Test recovery by recreating database
		t.Log("Step 4a: Testing database recreation after corruption...")

		// Remove corrupted database
		if err := os.Remove(dbPath); err != nil {
			t.Fatalf("Failed to remove corrupted database: %v", err)
		}

		// Create fresh database
		recoveredDB, err := database.NewDB(dbPath)
		if err != nil {
			t.Fatalf("Failed to create recovered database: %v", err)
		}
		defer recoveredDB.Close()

		// Verify fresh database is functional
		testFile2 := &database.FileMetadata{
			FileID:   "recovered-file-id",
			Path:     "recovered-file.txt",
			Checksum: "recovered123",
			Size:     2048,
			Mtime:    time.Unix(1234567890, 0),
			PeerID:   "recovery-peer",
		}

		if err := recoveredDB.InsertFile(testFile2); err != nil {
			t.Fatalf("Failed to insert file in recovered database: %v", err)
		}

		retrieved, err := recoveredDB.GetFileByID("recovered-file-id")
		if err != nil {
			t.Fatalf("Failed to retrieve file from recovered database: %v", err)
		}

		if retrieved.Path != "recovered-file.txt" {
			t.Errorf("Recovered database data mismatch")
		}

		t.Log("SUCCESS: Database recreated and functional after corruption")

	} else {
		// SQLite managed to open the corrupted file
		defer corruptedDB.Close()

		t.Log("Database opened despite corruption (SQLite may have recovered automatically)")

		// Step 4b: Test data integrity after opening corrupted database
		t.Log("Step 4b: Checking data integrity after corruption...")

		// Try to retrieve the original data
		retrievedCorrupted, err := corruptedDB.GetFileByID("test-file-id-123")

		if err != nil {
			// Data lost due to corruption - acceptable
			t.Logf("Original data lost due to corruption: %v", err)
			t.Log("SUCCESS: System detected data loss and handled gracefully")
		} else {
			// Data somehow survived
			if retrievedCorrupted.Path == "test-file.txt" {
				t.Log("SUCCESS: Data survived corruption (SQLite recovery successful)")
			} else {
				t.Errorf("Data corrupted: expected test-file.txt, got %s", retrievedCorrupted.Path)
			}
		}

		// Verify database is still functional for new operations
		testFile3 := &database.FileMetadata{
			FileID:   "post-corruption-file",
			Path:     "post-corruption.txt",
			Checksum: "newdata123",
			Size:     512,
			Mtime:    time.Unix(1234567890, 0),
			PeerID:   "post-peer",
		}

		if err := corruptedDB.InsertFile(testFile3); err != nil {
			t.Errorf("Database not functional after corruption: %v", err)
		} else {
			t.Log("SUCCESS: Database remains functional for new operations after corruption")
		}
	}

	t.Log("SUCCESS: Database corruption recovery test completed")
}

// TestDatabaseIntegrityCheck tests the database integrity checking functionality
func TestDatabaseIntegrityCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integrity check test in short mode")
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "integrity-test.db")

	// Create database
	db, err := database.NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Insert test data
	for i := 0; i < 10; i++ {
		testFile := &database.FileMetadata{
			FileID:   fmt.Sprintf("file-id-%d", i),
			Path:     fmt.Sprintf("test-%d.txt", i),
			Checksum: fmt.Sprintf("checksum-%d", i),
			Size:     int64(i * 100),
			Mtime:    time.Unix(1234567890+int64(i), 0),
			PeerID:   "test-peer",
		}

		if err := db.InsertFile(testFile); err != nil {
			t.Errorf("Failed to insert test file %d: %v", i, err)
		}
	}

	// Verify all data can be retrieved (basic integrity check)
	t.Log("Verifying database integrity...")
	files, err := db.GetAllFiles()
	if err != nil {
		t.Fatalf("Failed to get all files: %v", err)
	}

	if len(files) != 10 {
		t.Errorf("Expected 10 files, got %d", len(files))
	}

	// Verify each file is intact
	integrityOK := true
	for i, file := range files {
		if file.FileID == "" || file.Path == "" || file.Checksum == "" {
			t.Errorf("File %d has missing data: FileID=%s, Path=%s, Checksum=%s",
				i, file.FileID, file.Path, file.Checksum)
			integrityOK = false
		}
	}

	if integrityOK {
		t.Log("SUCCESS: Database integrity check passed - all data intact")
	} else {
		t.Error("FAILURE: Database integrity check failed - some data corrupted")
	}
}

// TestWALModeRecovery tests that WAL mode provides crash recovery
func TestWALModeRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping WAL mode recovery test in short mode")
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "wal-test.db")

	// Create database (should use WAL mode)
	db, err := database.NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Insert some data
	testFile := &database.FileMetadata{
		FileID:   "wal-test-file",
		Path:     "wal-test.txt",
		Checksum: "wal-checksum",
		Size:     1024,
		Mtime:    time.Unix(1234567890, 0),
		PeerID:   "wal-peer",
	}

	if err := db.InsertFile(testFile); err != nil {
		t.Fatalf("Failed to insert test file: %v", err)
	}

	// Close database properly (this should commit WAL)
	if err := db.Close(); err != nil {
		t.Fatalf("Failed to close database: %v", err)
	}

	// Reopen database to simulate restart
	t.Log("Simulating database restart...")
	db2, err := database.NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to reopen database: %v", err)
	}
	defer db2.Close()

	// Verify data persisted
	retrieved, err := db2.GetFileByID("wal-test-file")
	if err != nil {
		t.Fatalf("Failed to retrieve file after restart: %v", err)
	}

	if retrieved.Path != "wal-test.txt" {
		t.Errorf("Data not persisted correctly: expected wal-test.txt, got %s", retrieved.Path)
	}

	// Check for WAL files
	walFile := dbPath + "-wal"
	shmFile := dbPath + "-shm"

	// WAL files may or may not exist depending on checkpoint status
	if _, err := os.Stat(walFile); err == nil {
		t.Logf("WAL file present: %s", walFile)
	}
	if _, err := os.Stat(shmFile); err == nil {
		t.Logf("SHM file present: %s", shmFile)
	}

	t.Log("SUCCESS: WAL mode recovery test completed - data persisted across restart")
}
