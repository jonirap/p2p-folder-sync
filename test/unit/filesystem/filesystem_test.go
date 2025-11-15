package filesystem_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/p2p-folder-sync/p2p-sync/internal/filesystem"
)

func TestAtomicWriteFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")
	data := []byte("Hello, World! This is test data.")
	mode := os.FileMode(0644)

	// Write file atomically
	err := filesystem.AtomicWriteFile(filePath, data, mode)
	if err != nil {
		t.Fatalf("Failed to write file atomically: %v", err)
	}

	// Verify file was written
	readData, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read written file: %v", err)
	}

	if string(readData) != string(data) {
		t.Errorf("File content mismatch: got %q, want %q", readData, data)
	}
}

func TestFileExists(t *testing.T) {
	tmpDir := t.TempDir()
	existingFile := filepath.Join(tmpDir, "exists.txt")
	nonExistingFile := filepath.Join(tmpDir, "not-exists.txt")

	// Create a file
	err := os.WriteFile(existingFile, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	if !filesystem.FileExists(existingFile) {
		t.Error("Expected FileExists to return true for existing file")
	}

	if filesystem.FileExists(nonExistingFile) {
		t.Error("Expected FileExists to return false for non-existing file")
	}
}
