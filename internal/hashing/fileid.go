package hashing

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"time"
)

const (
	// FileIDPrefixSize is the number of bytes to read for file ID generation
	FileIDPrefixSize = 64 * 1024 // 64 KB
)

// GenerateFileID generates a stable file ID based on content, size, and creation time
func GenerateFileID(filePath string, peerID string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Get file info
	info, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("failed to stat file: %w", err)
	}

	size := info.Size()
	creationTime := info.ModTime() // Using ModTime as creation time approximation

	// For empty files, use creation time + size + peer_id
	if size == 0 {
		data := fmt.Sprintf("%d:%d:%s", creationTime.UnixNano(), size, peerID)
		return HashString([]byte(data)), nil
	}

	// Read first 64KB (or entire file if smaller)
	readSize := FileIDPrefixSize
	if size < int64(readSize) {
		readSize = int(size)
	}

	prefix := make([]byte, readSize)
	n, err := file.ReadAt(prefix, 0)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("failed to read file prefix: %w", err)
	}
	prefix = prefix[:n]

	// Combine: prefix + size + creation_time
	data := append(prefix, []byte(fmt.Sprintf(":%d:%d", size, creationTime.UnixNano()))...)
	return HashString(data), nil
}

// GenerateFileIDFromData generates a file ID from file data, size, and creation time
func GenerateFileIDFromData(data []byte, size int64, creationTime time.Time, peerID string) string {
	if size == 0 {
		// Empty file: use creation_time + size + peer_id
		input := fmt.Sprintf("%d:%d:%s", creationTime.UnixNano(), size, peerID)
		return HashString([]byte(input))
	}

	// Read first 64KB (or entire data if smaller)
	readSize := FileIDPrefixSize
	if len(data) < readSize {
		readSize = len(data)
	}

	prefix := data[:readSize]

	// Combine: prefix + size + creation_time
	input := append(prefix, []byte(fmt.Sprintf(":%d:%d", size, creationTime.UnixNano()))...)
	return HashString(input)
}

// ValidateFileID validates that a file ID is a valid hex string of correct length
func ValidateFileID(fileID string) bool {
	// BLAKE3 produces 32-byte (256-bit) hashes, which is 64 hex characters
	if len(fileID) != 64 {
		return false
	}
	_, err := hex.DecodeString(fileID)
	return err == nil
}


