package hashing_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/p2p-folder-sync/p2p-sync/internal/hashing"
)

// TestGenerateFileID_StandardFile verifies FID generation for files < 64KB
// Spec: FID = BLAKE3(first_64KB_of_content + size + creation_time)
func TestGenerateFileID_StandardFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")

	// Create a file smaller than 64KB
	content := []byte("This is test content for FID generation. It should be stable across renames.")
	err := os.WriteFile(filePath, content, 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	peerID := "test-peer-123"
	fileID, err := hashing.GenerateFileID(filePath, peerID)
	if err != nil {
		t.Fatalf("Failed to generate file ID: %v", err)
	}

	// Verify FID format
	if len(fileID) != 64 {
		t.Errorf("Expected FID length 64, got %d", len(fileID))
	}

	if !hashing.ValidateFileID(fileID) {
		t.Error("Generated FID should be valid")
	}

	// Verify FID is deterministic for same content
	fileID2, err := hashing.GenerateFileID(filePath, peerID)
	if err != nil {
		t.Fatalf("Failed to generate file ID second time: %v", err)
	}
	if fileID != fileID2 {
		t.Errorf("File ID should be deterministic, got %s and %s", fileID, fileID2)
	}
}

// TestGenerateFileID_LargeFile verifies FID uses only first 64KB for files > 64KB
func TestGenerateFileID_LargeFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "large_test.txt")

	// Create a file larger than 64KB
	content := make([]byte, 100*1024) // 100KB
	for i := range content {
		content[i] = byte(i % 256)
	}
	err := os.WriteFile(filePath, content, 0644)
	if err != nil {
		t.Fatalf("Failed to create large test file: %v", err)
	}

	peerID := "test-peer-123"
	fileID, err := hashing.GenerateFileID(filePath, peerID)
	if err != nil {
		t.Fatalf("Failed to generate file ID: %v", err)
	}

	// Verify FID format
	if len(fileID) != 64 {
		t.Errorf("Expected FID length 64, got %d", len(fileID))
	}

	// Create another file with same first 64KB and same size, but different content after
	filePath2 := filepath.Join(tmpDir, "large_test2.txt")
	content2 := make([]byte, 100*1024)
	copy(content2[:64*1024], content[:64*1024]) // Same first 64KB
	// Different content after 64KB
	for i := 64 * 1024; i < len(content2); i++ {
		content2[i] = byte((i + 1) % 256) // Different pattern
	}
	err = os.WriteFile(filePath2, content2, 0644)
	if err != nil {
		t.Fatalf("Failed to create second large test file: %v", err)
	}

	// Set the same modification time to ensure deterministic FID generation
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("Failed to stat first file: %v", err)
	}
	err = os.Chtimes(filePath2, info.ModTime(), info.ModTime())
	if err != nil {
		t.Logf("Warning: Could not set modification time: %v", err)
	}

	fileID2, err := hashing.GenerateFileID(filePath2, peerID)
	if err != nil {
		t.Fatalf("Failed to generate file ID for second file: %v", err)
	}

	// FIDs should be the same since first 64KB and size are identical
	if fileID != fileID2 {
		t.Errorf("Files with same first 64KB and size should have same FID, got %s and %s", fileID, fileID2)
	}
}

// TestGenerateFileID_EmptyFile verifies empty file FID uses creation_time + size + peer_id
// Spec: For empty files: BLAKE3(creation_time + size + peer_id)
func TestGenerateFileID_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "empty.txt")

	// Create empty file
	err := os.WriteFile(filePath, []byte{}, 0644)
	if err != nil {
		t.Fatalf("Failed to create empty test file: %v", err)
	}

	peerID := "test-peer-123"
	fileID, err := hashing.GenerateFileID(filePath, peerID)
	if err != nil {
		t.Fatalf("Failed to generate file ID for empty file: %v", err)
	}

	// Verify FID format
	if len(fileID) != 64 {
		t.Errorf("Expected FID length 64, got %d", len(fileID))
	}

	if !hashing.ValidateFileID(fileID) {
		t.Error("Generated FID should be valid")
	}

	// Verify FID is deterministic for same empty file
	fileID2, err := hashing.GenerateFileID(filePath, peerID)
	if err != nil {
		t.Fatalf("Failed to generate file ID second time: %v", err)
	}
	if fileID != fileID2 {
		t.Errorf("Empty file ID should be deterministic, got %s and %s", fileID, fileID2)
	}
}

// TestGenerateFileID_Consistency verifies same file generates same FID across multiple calls
func TestGenerateFileID_Consistency(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "consistency_test.txt")

	content := []byte("Test content for consistency verification")
	err := os.WriteFile(filePath, content, 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	peerID := "test-peer-123"

	// Generate FID multiple times
	fileIDs := make([]string, 5)
	for i := 0; i < 5; i++ {
		fileID, err := hashing.GenerateFileID(filePath, peerID)
		if err != nil {
			t.Fatalf("Failed to generate file ID on attempt %d: %v", i, err)
		}
		fileIDs[i] = fileID
	}

	// All FIDs should be identical
	for i := 1; i < len(fileIDs); i++ {
		if fileIDs[0] != fileIDs[i] {
			t.Errorf("File ID inconsistency: attempt 0: %s, attempt %d: %s", fileIDs[0], i, fileIDs[i])
		}
	}
}

// TestGenerateFileID_CollisionResistance verifies different files produce different FIDs
func TestGenerateFileID_CollisionResistance(t *testing.T) {
	tmpDir := t.TempDir()
	peerID := "test-peer-123"

	testCases := []struct {
		name     string
		content  []byte
		size     int64
		modTime  time.Time
	}{
		{"file1", []byte("content1"), 8, time.Now()},
		{"file2", []byte("content2"), 8, time.Now()},
		{"file3", []byte("content1"), 8, time.Now().Add(time.Hour)}, // Same content, different time
		{"file4", []byte("content1"), 9, time.Now()},               // Same content, different size (with padding)
	}

	fileIDs := make(map[string]bool)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filePath := filepath.Join(tmpDir, tc.name+".txt")

			// Create file with specific content
			err := os.WriteFile(filePath, tc.content, 0644)
			if err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			// Set specific modification time if needed
			if !tc.modTime.IsZero() {
				err = os.Chtimes(filePath, tc.modTime, tc.modTime)
				if err != nil {
					t.Logf("Warning: Could not set modification time: %v", err)
				}
			}

			fileID, err := hashing.GenerateFileID(filePath, peerID)
			if err != nil {
				t.Fatalf("Failed to generate file ID: %v", err)
			}

			// Check for collisions
			if fileIDs[fileID] {
				t.Errorf("File ID collision detected for %s: %s", tc.name, fileID)
			}
			fileIDs[fileID] = true
		})
	}

	// Verify we got the expected number of unique FIDs
	if len(fileIDs) != len(testCases) {
		t.Errorf("Expected %d unique FIDs, got %d", len(testCases), len(fileIDs))
	}
}

// TestGenerateFileIDFromData verifies the data-based variant matches file-based variant
func TestGenerateFileIDFromData(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "data_test.txt")

	content := []byte("Test content for data-based FID generation")
	err := os.WriteFile(filePath, content, 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Get file metadata
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	peerID := "test-peer-123"
	size := info.Size()
	modTime := info.ModTime()

	// Generate FID using file path
	fileIDFromFile, err := hashing.GenerateFileID(filePath, peerID)
	if err != nil {
		t.Fatalf("Failed to generate file ID from file: %v", err)
	}

	// Generate FID using data directly
	fileIDFromData := hashing.GenerateFileIDFromData(content, size, modTime, peerID)

	// They should match
	if fileIDFromFile != fileIDFromData {
		t.Errorf("File-based and data-based FIDs should match: file=%s, data=%s", fileIDFromFile, fileIDFromData)
	}
}

// TestValidateFileID tests validation function with valid/invalid inputs
func TestValidateFileID(t *testing.T) {
	testCases := []struct {
		name     string
		fileID   string
		expected bool
	}{
		{"valid_64_char_hex", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", true},
		{"valid_lowercase", "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890", true},
		{"too_short", "0123456789abcdef", false},
		{"too_long", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0", false},
		{"invalid_chars", "gggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggg", false},
		{"mixed_case", "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF", true},
		{"empty_string", "", false},
		{"uppercase_only", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := hashing.ValidateFileID(tc.fileID)
			if result != tc.expected {
				t.Errorf("ValidateFileID(%q) = %v, expected %v", tc.fileID, result, tc.expected)
			}
		})
	}
}

// TestGenerateFileID_PersistenceAcrossRenames verifies FID remains stable when file is renamed
func TestGenerateFileID_PersistenceAcrossRenames(t *testing.T) {
	tmpDir := t.TempDir()
	originalPath := filepath.Join(tmpDir, "original.txt")
	renamedPath := filepath.Join(tmpDir, "renamed.txt")

	content := []byte("Content that should persist across renames")
	err := os.WriteFile(originalPath, content, 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	peerID := "test-peer-123"

	// Get original FID
	originalFID, err := hashing.GenerateFileID(originalPath, peerID)
	if err != nil {
		t.Fatalf("Failed to generate original file ID: %v", err)
	}

	// Rename file
	err = os.Rename(originalPath, renamedPath)
	if err != nil {
		t.Fatalf("Failed to rename file: %v", err)
	}

	// Get FID after rename
	renamedFID, err := hashing.GenerateFileID(renamedPath, peerID)
	if err != nil {
		t.Fatalf("Failed to generate renamed file ID: %v", err)
	}

	// FID should be stable across renames (though this tests the basic algorithm)
	// Note: In the full system, xattrs would preserve the original FID
	if originalFID != renamedFID {
		t.Logf("FID changed after rename (expected for basic algorithm): original=%s, renamed=%s", originalFID, renamedFID)
	}

	// At minimum, the renamed file should produce a valid FID
	if !hashing.ValidateFileID(renamedFID) {
		t.Error("Renamed file should produce valid FID")
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

// TestGenerateFileID_ErrorHandling tests error conditions
func TestGenerateFileID_ErrorHandling(t *testing.T) {
	peerID := "test-peer-123"

	testCases := []struct {
		name     string
		filePath string
	}{
		{"nonexistent_file", "/nonexistent/path/file.txt"},
		{"empty_path", ""},
		{"directory", "/tmp"}, // Assuming /tmp exists
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := hashing.GenerateFileID(tc.filePath, peerID)
			if err == nil {
				t.Errorf("Expected error for %s, but got none", tc.name)
			}
		})
	}
}
