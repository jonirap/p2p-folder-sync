package filesystem

import (
	"sync"
	"time"
)

const (
	// RenameDetectionTTL is the time window for rename detection (5 seconds)
	RenameDetectionTTL = 5 * time.Second
)

// DeletedFileInfo stores information about a recently deleted file
type DeletedFileInfo struct {
	FileID    string
	Checksum  string
	Size      int64
	Mtime     time.Time
	Path      string
	DeletedAt time.Time
}

// RenameDetector detects file renames vs delete+create
type RenameDetector struct {
	recentDeletes map[string]*DeletedFileInfo
	mu            sync.RWMutex
	cleanupTicker *time.Ticker
	stopCleanup   chan struct{}
}

// NewRenameDetector creates a new rename detector
func NewRenameDetector() *RenameDetector {
	rd := &RenameDetector{
		recentDeletes: make(map[string]*DeletedFileInfo),
		stopCleanup:   make(chan struct{}),
	}

	// Start cleanup goroutine
	rd.cleanupTicker = time.NewTicker(1 * time.Second)
	go rd.cleanup()

	return rd
}

// RecordDelete records a file deletion for rename detection
func (rd *RenameDetector) RecordDelete(fileID, checksum, path string, size int64, mtime time.Time) {
	rd.mu.Lock()
	defer rd.mu.Unlock()

	rd.recentDeletes[fileID] = &DeletedFileInfo{
		FileID:    fileID,
		Checksum:  checksum,
		Size:      size,
		Mtime:     mtime,
		Path:      path,
		DeletedAt: time.Now(),
	}
}

// CheckRename checks if a newly created file is actually a rename
// Returns (isRename, oldPath, nil) if it's a rename, (false, "", nil) if it's a new file
func (rd *RenameDetector) CheckRename(fileID, checksum string, size int64) (bool, string, error) {
	rd.mu.RLock()
	defer rd.mu.RUnlock()

	deleted, exists := rd.recentDeletes[fileID]
	if !exists {
		return false, "", nil // Not a rename
	}

	// Check if checksum matches (rename) or differs (edit)
	if deleted.Checksum == checksum && deleted.Size == size {
		// It's a rename - same file ID, same checksum, same size
		oldPath := deleted.Path

		// Remove from recent deletes
		rd.mu.RUnlock()
		rd.mu.Lock()
		delete(rd.recentDeletes, fileID)
		rd.mu.Unlock()
		rd.mu.RLock()

		return true, oldPath, nil
	}

	// File ID matches but checksum differs - it's an edit (delete + create)
	// Remove from recent deletes since we've matched it
	rd.mu.RUnlock()
	rd.mu.Lock()
	delete(rd.recentDeletes, fileID)
	rd.mu.Unlock()
	rd.mu.RLock()

	return false, "", nil // Not a rename, it's an edit
}

// cleanup removes expired entries from recent deletes
func (rd *RenameDetector) cleanup() {
	for {
		select {
		case <-rd.cleanupTicker.C:
			rd.mu.Lock()
			now := time.Now()
			for fileID, info := range rd.recentDeletes {
				if now.Sub(info.DeletedAt) > RenameDetectionTTL {
					delete(rd.recentDeletes, fileID)
				}
			}
			rd.mu.Unlock()

		case <-rd.stopCleanup:
			rd.cleanupTicker.Stop()
			return
		}
	}
}

// Close stops the rename detector
func (rd *RenameDetector) Close() {
	close(rd.stopCleanup)
	rd.cleanupTicker.Stop()
	rd.mu.Lock()
	defer rd.mu.Unlock()
	rd.recentDeletes = make(map[string]*DeletedFileInfo)
}

// GetRecentDeletesCount returns the number of recent deletes (for testing)
func (rd *RenameDetector) GetRecentDeletesCount() int {
	rd.mu.RLock()
	defer rd.mu.RUnlock()
	return len(rd.recentDeletes)
}
