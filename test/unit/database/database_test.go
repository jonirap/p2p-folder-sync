package database_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/p2p-folder-sync/p2p-sync/internal/database"
)

func TestNewDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := database.NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Test that we can query the database
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table'").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query database: %v", err)
	}

	// Should have created several tables
	if count < 5 {
		t.Errorf("Expected at least 5 tables, got %d", count)
	}
}

func TestNewDB_InvalidPath(t *testing.T) {
	// Try to create database in a directory that doesn't exist and can't be created
	invalidPath := "/nonexistent/deep/path/database.db"

	_, err := database.NewDB(invalidPath)
	if err == nil {
		t.Error("Expected error for invalid database path")
	}
}

func TestFileOperations(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := database.NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Test inserting a file
	fileMetadata := &database.FileMetadata{
		FileID:      "test-file-123",
		Path:        "/tmp/test.txt",
		Checksum:    "abc123",
		Size:        1024,
		Mtime:       time.Now(),
		PeerID:      "peer-1",
		VectorClock: map[string]int64{"peer-1": 1},
		Compressed:  false,
	}

	err = db.InsertFile(fileMetadata)
	if err != nil {
		t.Fatalf("Failed to insert file: %v", err)
	}

	// Test getting the file by ID
	retrieved, err := db.GetFile("test-file-123")
	if err != nil {
		t.Fatalf("Failed to get file: %v", err)
	}

	if retrieved.FileID != fileMetadata.FileID {
		t.Errorf("Expected file ID %s, got %s", fileMetadata.FileID, retrieved.FileID)
	}
	if retrieved.Path != fileMetadata.Path {
		t.Errorf("Expected path %s, got %s", fileMetadata.Path, retrieved.Path)
	}
	if retrieved.Checksum != fileMetadata.Checksum {
		t.Errorf("Expected checksum %s, got %s", fileMetadata.Checksum, retrieved.Checksum)
	}
	if retrieved.Size != fileMetadata.Size {
		t.Errorf("Expected size %d, got %d", fileMetadata.Size, retrieved.Size)
	}
	if retrieved.PeerID != fileMetadata.PeerID {
		t.Errorf("Expected peer ID %s, got %s", fileMetadata.PeerID, retrieved.PeerID)
	}
	if retrieved.VectorClock["peer-1"] != 1 {
		t.Errorf("Expected vector clock value 1, got %d", retrieved.VectorClock["peer-1"])
	}

	// Test getting file by path
	retrievedByPath, err := db.GetFileByPath("/tmp/test.txt")
	if err != nil {
		t.Fatalf("Failed to get file by path: %v", err)
	}

	if retrievedByPath.FileID != fileMetadata.FileID {
		t.Errorf("File by path has wrong ID: expected %s, got %s", fileMetadata.FileID, retrievedByPath.FileID)
	}

	// Test GetAllFiles
	files, err := db.GetAllFiles()
	if err != nil {
		t.Fatalf("Failed to get all files: %v", err)
	}

	if len(files) != 1 {
		t.Errorf("Expected 1 file, got %d", len(files))
	}

	// Test nonexistent file
	_, err = db.GetFile("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}

	// Test nonexistent path
	_, err = db.GetFileByPath("/nonexistent/path")
	if err == nil {
		t.Error("Expected error for nonexistent path")
	}
}
