package filesystem

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// AtomicWriteFile writes a file atomically using a temporary file and rename
func AtomicWriteFile(filePath string, data []byte, mode os.FileMode) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Create temporary file in the same directory
	tmpFile, err := os.CreateTemp(dir, ".p2p-sync-tmp-")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Ensure cleanup on error
	defer func() {
		if err != nil {
			os.Remove(tmpPath)
		}
	}()

	// Write data to temp file
	if _, err = tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Set permissions
	if err = tmpFile.Chmod(mode); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	// Sync to disk
	if err = tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to sync temp file: %w", err)
	}

	// Close temp file
	if err = tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomic rename
	if err = os.Rename(tmpPath, filePath); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// AtomicWriteFileFromReader writes a file atomically from an io.Reader
func AtomicWriteFileFromReader(filePath string, reader io.Reader, mode os.FileMode) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Create temporary file in the same directory
	tmpFile, err := os.CreateTemp(dir, ".p2p-sync-tmp-")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Ensure cleanup on error
	defer func() {
		if err != nil {
			os.Remove(tmpPath)
		}
	}()

	// Copy data from reader to temp file
	if _, err = io.Copy(tmpFile, reader); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to copy data: %w", err)
	}

	// Set permissions
	if err = tmpFile.Chmod(mode); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	// Sync to disk
	if err = tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to sync temp file: %w", err)
	}

	// Close temp file
	if err = tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomic rename
	if err = os.Rename(tmpPath, filePath); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// GetFileMetadata retrieves file metadata
func GetFileMetadata(filePath string) (size int64, mtime int64, mode os.FileMode, err error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to stat file: %w", err)
	}

	return info.Size(), info.ModTime().Unix(), info.Mode(), nil
}

// EnsureDirectory ensures a directory exists
func EnsureDirectory(dirPath string) error {
	return os.MkdirAll(dirPath, 0755)
}

// RemoveFile removes a file
func RemoveFile(filePath string) error {
	return os.Remove(filePath)
}

// RemoveDirectory removes a directory (must be empty)
func RemoveDirectory(dirPath string) error {
	return os.Remove(dirPath)
}

// RemoveDirectoryRecursive removes a directory and all its contents
func RemoveDirectoryRecursive(dirPath string) error {
	return os.RemoveAll(dirPath)
}

// FileExists checks if a file exists
func FileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return err == nil
}

// IsDirectory checks if a path is a directory
func IsDirectory(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

