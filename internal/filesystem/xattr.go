package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

const (
	// XattrKey is the extended attribute key for storing file IDs
	XattrKey = "user.p2p_sync.file_id"
)

// SetFileID stores a file ID in extended attributes
func SetFileID(filePath string, fileID string) error {
	// Try to set xattr, but don't fail if not supported
	// This is a best-effort operation
	if err := setXattr(filePath, XattrKey, fileID); err != nil {
		// Log but don't fail - xattrs are optional
		return nil // Return nil to indicate we tried
	}
	return nil
}

// GetFileID retrieves a file ID from extended attributes
func GetFileID(filePath string) (string, error) {
	return getXattr(filePath, XattrKey)
}

// setXattr sets an extended attribute (platform-specific)
func setXattr(filePath string, key string, value string) error {
	// Use syscall.Setxattr for Linux and macOS
	data := []byte(value)
	err := syscall.Setxattr(filePath, key, data, 0)
	if err != nil {
		return fmt.Errorf("failed to set xattr: %w", err)
	}
	return nil
}

// getXattr gets an extended attribute (platform-specific)
func getXattr(filePath string, key string) (string, error) {
	// Allocate a reasonably sized buffer
	data := make([]byte, 256)

	n, err := syscall.Getxattr(filePath, key, data)
	if err != nil {
		return "", fmt.Errorf("failed to get xattr: %w", err)
	}

	// If buffer was too small, the data might be truncated
	// For our use case (file IDs are fixed size), this should be fine
	return string(data[:n]), nil
}

// HasXattrSupport checks if extended attributes are supported on this platform
func HasXattrSupport() bool {
	// Check by trying to set a test xattr
	testFile := filepath.Join(os.TempDir(), ".p2p-sync-xattr-test")
	defer os.Remove(testFile)

	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		return false
	}

	err := setXattr(testFile, "user.p2p_sync.test", "test")
	return err == nil
}

