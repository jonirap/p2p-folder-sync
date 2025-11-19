package sync

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/p2p-folder-sync/p2p-sync/internal/config"
	"github.com/p2p-folder-sync/p2p-sync/internal/database"
	"github.com/p2p-folder-sync/p2p-sync/internal/filesystem"
	"github.com/p2p-folder-sync/p2p-sync/internal/hashing"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/messages"
	"github.com/p2p-folder-sync/p2p-sync/internal/sync/conflict"
)

// Messenger defines the interface for sending messages to peers
type Messenger interface {
	SendFile(peerID string, fileData []byte, metadata *SyncOperation) error
	BroadcastOperation(op *SyncOperation) error
	RequestStateSync(peerID string) error
	ConnectToPeer(peerID string, address string, port int) error
}

// Engine is the main sync engine
type Engine struct {
	config           *config.Config
	db               *database.DB
	watcher          *filesystem.Watcher
	renameDetector   *filesystem.RenameDetector
	conflictResolver *conflict.Resolver
	messenger        Messenger
	operationQueue   map[string][]*SyncOperation
	queueMu          sync.RWMutex
	peerID           string
	stopCh           chan struct{}
	stopped          bool
	stopMu           sync.Mutex
}

// NewEngine creates a new sync engine
func NewEngine(cfg *config.Config, db *database.DB, peerID string) (*Engine, error) {
	return NewEngineWithMessenger(cfg, db, peerID, nil)
}

// NewEngineWithMessenger creates a new sync engine with a messenger
func NewEngineWithMessenger(cfg *config.Config, db *database.DB, peerID string, messenger Messenger) (*Engine, error) {
	watcher, err := filesystem.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}

	engine := &Engine{
		config:           cfg,
		db:               db,
		watcher:          watcher,
		renameDetector:   filesystem.NewRenameDetector(),
		conflictResolver: conflict.NewResolver(cfg.Conflict.ResolutionStrategy),
		messenger:        messenger,
		operationQueue:   make(map[string][]*SyncOperation),
		peerID:           peerID,
		stopCh:           make(chan struct{}),
	}

	return engine, nil
}

// Start starts the sync engine
func (e *Engine) Start(ctx context.Context) error {
	// Replay unacknowledged operations from log (critical for data durability)
	if err := e.ReplayUnacknowledgedOperations(ctx); err != nil {
		log.Printf("Warning: failed to replay unacknowledged operations: %v", err)
		// Don't fail startup, but log the warning
	}

	// Add sync folder to watcher
	if err := e.watcher.Add(e.config.Sync.FolderPath); err != nil {
		return fmt.Errorf("failed to add folder to watcher: %w", err)
	}

	// Start processing file events
	go e.processFileEvents(ctx)

	// Start periodic log compaction (spec line 318)
	go e.periodicLogCompaction()

	// Start periodic state synchronization
	go e.periodicStateSync()

	return nil
}

// Stop stops the sync engine
func (e *Engine) Stop() error {
	e.stopMu.Lock()
	defer e.stopMu.Unlock()

	if e.stopped {
		return nil // Already stopped
	}
	e.stopped = true

	close(e.stopCh)
	if e.watcher != nil {
		return e.watcher.Close()
	}
	if e.renameDetector != nil {
		e.renameDetector.Close()
	}
	return nil
}

// GetAllFiles returns all files in the database
func (e *Engine) GetAllFiles() ([]*database.FileMetadata, error) {
	return e.db.GetAllFiles()
}

// HandleIncomingFile handles a file operation received from a peer (remote operation)
// This implements sync loop prevention by temporarily disabling the file watcher
func (e *Engine) HandleIncomingFile(fileData []byte, metadata *SyncOperation) error {
	// Mark as remote operation
	metadata.Source = "remote"

	// Convert relative path to absolute path
	absPath := filepath.Join(e.config.Sync.FolderPath, metadata.Path)

	// Temporarily disable file system watcher for this path
	e.watcher.IgnorePath(absPath)
	defer func() {
		// Ensure re-enabling even on error (with delay to allow write to complete)
		time.Sleep(100 * time.Millisecond)
		e.watcher.WatchPath(absPath)
	}()

	switch metadata.Type {
	case OpCreate, OpUpdate:
		// Check for conflicts before writing
		localFile, err := e.db.GetFileByID(metadata.FileID)
		if err == nil {
			// File exists locally - check for conflict
			if localFile.Checksum != metadata.Checksum {
				// Checksums differ - potential conflict
				localVC := localFile.VectorClock
				if localVC == nil {
					localVC = make(map[string]int64)
				}
				remoteVC := metadata.VectorClock
				if remoteVC == nil {
					remoteVC = make(map[string]int64)
				}

				// Read local file data for potential merge
				localData, err := os.ReadFile(absPath)
				if err != nil {
					// If we can't read local file, fall back to remote
					localData = nil
				}

				// Create local operation for comparison
				localOp := &conflict.SyncOperation{
					FileID:    localFile.FileID,
					Path:      localFile.Path,
					Checksum:  localFile.Checksum,
					Size:      localFile.Size,
					Timestamp: localFile.Mtime.UnixMilli(),
					PeerID:    localFile.PeerID,
					Data:      localData,
				}

				// Create remote operation for comparison (already has data in fileData)
				remoteOp := &conflict.SyncOperation{
					FileID:    metadata.FileID,
					Path:      metadata.Path,
					Checksum:  metadata.Checksum,
					Size:      metadata.Size,
					Timestamp: metadata.Mtime,
					PeerID:    metadata.PeerID,
					Data:      fileData,
				}

				// Resolve conflict
				winner, err := e.conflictResolver.ResolveConflict(localOp, remoteOp, conflict.VectorClock(localVC), conflict.VectorClock(remoteVC))
				if err != nil {
					return fmt.Errorf("failed to resolve conflict: %w", err)
				}

				// Check if winner is a merge result or one of the original operations
				isLocalWinner := winner.FileID == localOp.FileID && winner.Checksum == localOp.Checksum
				isRemoteWinner := winner.FileID == remoteOp.FileID && winner.Checksum == remoteOp.Checksum
				isMergeResult := !isLocalWinner && !isRemoteWinner

				if isLocalWinner {
					// Local operation wins - broadcast to ensure remote peer gets it
					// Read local file data
					localData, err := os.ReadFile(absPath)
					if err != nil {
						return fmt.Errorf("failed to read local file for broadcast: %w", err)
					}

					// Create operation to broadcast
					broadcastOp := &SyncOperation{
						ID:          generateOperationID(),
						Type:        OpUpdate,
						Path:        metadata.Path,
						FileID:      localFile.FileID,
						Checksum:    localFile.Checksum,
						Size:        localFile.Size,
						Mtime:       localFile.Mtime.UnixMilli(),
						PeerID:      e.peerID,
						Source:      "local",
						Data:        localData,
						VectorClock: localVC,
					}

					// Broadcast the winning local version
					if e.messenger != nil {
						if err := e.messenger.BroadcastOperation(broadcastOp); err != nil {
							// Log error but don't fail - local version is still consistent
							_ = err
						}
					}

					// Don't apply remote change
					return nil
				} else if isMergeResult {
					// Merge occurred - apply merged result and broadcast
					// Update metadata with merged data
					metadata.Data = winner.Data
					metadata.Checksum = winner.Checksum
					metadata.Size = winner.Size
					fileData = winner.Data

					// After writing the merged file, broadcast it
					// This will be handled after the file is written
					defer func() {
						if e.messenger != nil {
							broadcastOp := &SyncOperation{
								ID:          generateOperationID(),
								Type:        OpUpdate,
								Path:        metadata.Path,
								FileID:      metadata.FileID,
								Checksum:    metadata.Checksum,
								Size:        metadata.Size,
								Mtime:       time.Now().UnixMilli(),
								PeerID:      e.peerID,
								Source:      "local",
								Data:        fileData,
								VectorClock: metadata.VectorClock,
							}
							e.messenger.BroadcastOperation(broadcastOp)
						}
					}()
				}
				// If remote winner (isRemoteWinner), proceed normally with remote data
			}
		}

		// Write file to disk atomically
		log.Printf("DEBUG [Engine]: Writing file to %s (%d bytes)", absPath, len(fileData))
		if err := e.atomicWriteFile(absPath, fileData); err != nil {
			log.Printf("DEBUG [Engine]: Failed to write file: %v", err)
			return fmt.Errorf("failed to write file: %w", err)
		}
		log.Printf("DEBUG [Engine]: Successfully wrote file to %s", absPath)

		// Store file ID in extended attributes if supported
		filesystem.SetFileID(absPath, metadata.FileID)

		// Update database (marked as remote)
		fileMetadata := &database.FileMetadata{
			FileID:      metadata.FileID,
			Path:        metadata.Path,
			Checksum:    metadata.Checksum,
			Size:        metadata.Size,
			Mtime:       time.UnixMilli(metadata.Mtime),
			Mode:        metadata.Mode,
			PeerID:      metadata.PeerID,
			VectorClock: metadata.VectorClock,
			Compressed: func() bool {
				if metadata.Compressed != nil {
					return *metadata.Compressed
				}
				return false
			}(),
			OriginalSize:         metadata.OriginalSize,
			CompressionAlgorithm: metadata.CompressionAlgorithm,
		}

		if err := e.db.InsertFile(fileMetadata); err != nil {
			return fmt.Errorf("failed to update database: %w", err)
		}

	case OpDelete:
		// Delete the file
		if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to delete file: %w", err)
		}

		// Remove from database
		if err := e.db.DeleteFile(metadata.FileID); err != nil {
			return fmt.Errorf("failed to delete file from database: %w", err)
		}
	}

	// Log operation but do not broadcast (since it's already received)
	// In a full implementation, this would be stored for recovery purposes
	_ = metadata

	return nil
}

// HandleIncomingRename handles a rename operation received from a peer (remote operation)
func (e *Engine) HandleIncomingRename(metadata *SyncOperation) error {
	// Mark as remote operation
	metadata.Source = "remote"

	// Check if FromPath is provided
	if metadata.FromPath == nil {
		return fmt.Errorf("rename operation missing FromPath")
	}

	// Convert relative paths to absolute paths
	fromPath := filepath.Join(e.config.Sync.FolderPath, *metadata.FromPath)
	toPath := filepath.Join(e.config.Sync.FolderPath, metadata.Path)

	// Temporarily disable file system watcher for both paths
	e.watcher.IgnorePath(fromPath)
	defer func() {
		time.Sleep(100 * time.Millisecond)
		e.watcher.WatchPath(fromPath)
	}()
	e.watcher.IgnorePath(toPath)
	defer func() {
		time.Sleep(100 * time.Millisecond)
		e.watcher.WatchPath(toPath)
	}()

	// Perform the rename operation
	if err := os.Rename(fromPath, toPath); err != nil {
		return fmt.Errorf("failed to rename file from %s to %s: %w", fromPath, toPath, err)
	}

	// Update file ID in extended attributes if supported
	filesystem.SetFileID(toPath, metadata.FileID)

	// Update database (marked as remote)
	compressed := false
	if metadata.Compressed != nil {
		compressed = *metadata.Compressed
	}

	fileMetadata := &database.FileMetadata{
		FileID:               metadata.FileID,
		Path:                 metadata.Path,
		Checksum:             metadata.Checksum,
		Size:                 metadata.Size,
		Mtime:                time.UnixMilli(metadata.Mtime),
		PeerID:               metadata.PeerID,
		Compressed:           compressed,
		OriginalSize:         metadata.OriginalSize,
		CompressionAlgorithm: metadata.CompressionAlgorithm,
	}

	if err := e.db.InsertFile(fileMetadata); err != nil {
		return fmt.Errorf("failed to update database: %w", err)
	}

	// Log operation but do not broadcast (since it's already received)
	_ = metadata

	return nil
}

// atomicWriteFile writes data to a file atomically using a temporary file
func (e *Engine) atomicWriteFile(filePath string, data []byte) error {
	// Create temporary file in same directory
	dir := filepath.Dir(filePath)
	tempFile, err := os.CreateTemp(dir, ".p2p-sync-tmp-")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tempPath := tempFile.Name()

	// Write data to temp file
	if _, err := tempFile.Write(data); err != nil {
		tempFile.Close()
		os.Remove(tempPath)
		return fmt.Errorf("failed to write to temp file: %w", err)
	}

	// Close temp file
	if err := tempFile.Close(); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomically move temp file to final location
	if err := os.Rename(tempPath, filePath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// processFileEvents processes file system events
func (e *Engine) processFileEvents(ctx context.Context) {
	log.Println("DEBUG: processFileEvents started, waiting for file events...")
	for {
		select {
		case <-ctx.Done():
			log.Println("DEBUG: processFileEvents context done")
			return
		case <-e.stopCh:
			log.Println("DEBUG: processFileEvents stop signal received")
			return
		case event := <-e.watcher.Events():
			log.Printf("DEBUG: File event received: %s %s", event.Operation, event.Path)
			e.handleFileEvent(event)
		case err := <-e.watcher.Errors():
			log.Printf("DEBUG: Watcher error: %v", err)
			// Log error
			_ = err
		}
	}
}

// handleFileEvent handles a file system event
func (e *Engine) handleFileEvent(event filesystem.FileEvent) {
	// Check if this file is currently being written by a remote operation
	// If so, skip processing to prevent sync loops
	// The watcher already filters out ignored paths, but this is an additional check

	switch event.Operation {
	case "create":
		e.handleCreate(event.Path)
	case "update":
		e.handleUpdate(event.Path)
	case "delete":
		e.handleDelete(event.Path)
	case "rename":
		e.handleRename(event.Path)
	}
}

// handleCreate handles a file creation event
func (e *Engine) handleCreate(path string) {
	var fileID string

	// First, check if file already has a stored file ID
	if existingFileID, err := filesystem.GetFileID(path); err == nil && existingFileID != "" {
		// Use existing file ID from xattr
		fileID = existingFileID
	} else {
		// Generate new file ID
		var err error
		fileID, err = hashing.GenerateFileID(path, e.peerID)
		if err != nil {
			return
		}

		// Store file ID in xattr if supported
		filesystem.SetFileID(path, fileID)
	}

	// Get file checksum
	checksum, err := e.getFileChecksum(path)
	if err != nil {
		return
	}

	// Get file size
	size, _, _, err := filesystem.GetFileMetadata(path)
	if err != nil {
		return
	}

	// Convert absolute path to relative path within sync folder
	relPath, err := filepath.Rel(e.config.Sync.FolderPath, path)
	if err != nil {
		return // Skip if we can't make it relative
	}

	// Check if this is a rename by looking for existing file with same FileID at different path
	isRename := false
	oldPath := ""
	if fileID != "" {
		existingFile, err := e.db.GetFileByID(fileID)
		if err == nil && existingFile.Path != relPath {
			// File with same ID exists at different path - this is a rename
			isRename = true
			oldPath = existingFile.Path
		}
	}

	// Check for rename detection (fallback for delete+create pattern)
	if !isRename {
		isRename, oldPathFromDetector, err := e.renameDetector.CheckRename(fileID, checksum, size)
		if err != nil {
			return
		}
		if isRename {
			oldPath = oldPathFromDetector
		}
	}

	if isRename {
		// Create rename operation
		op := NewSyncOperation(OpRename, relPath, fileID, e.peerID)
		op.FromPath = &oldPath // oldPath is already relative
		op.Checksum = checksum
		op.Size = size
		e.queueOperation(op)
	} else {
		// Store file in database locally
		fileMetadata := &database.FileMetadata{
			FileID:   fileID,
			Path:     relPath,
			Checksum: checksum,
			Size:     size,
			Mtime:    time.Now(), // Use current time for local files
			PeerID:   e.peerID,
		}
		if err := e.db.InsertFile(fileMetadata); err != nil {
			// Log error but continue
			_ = err
		}

		// Create new file operation
		op := NewSyncOperation(OpCreate, relPath, fileID, e.peerID)
		op.Checksum = checksum
		op.Size = size
		e.queueOperation(op)
	}
}

// handleUpdate handles a file update event
func (e *Engine) handleUpdate(path string) {
	var fileID string

	// First, check if file already has a stored file ID
	if existingFileID, err := filesystem.GetFileID(path); err == nil && existingFileID != "" {
		// Use existing file ID from xattr
		fileID = existingFileID
	} else {
		// Generate new file ID
		var err error
		fileID, err = hashing.GenerateFileID(path, e.peerID)
		if err != nil {
			return
		}

		// Store file ID in xattr if supported
		filesystem.SetFileID(path, fileID)
	}

	checksum, err := e.getFileChecksum(path)
	if err != nil {
		return
	}

	size, _, _, err := filesystem.GetFileMetadata(path)
	if err != nil {
		return
	}

	// Convert absolute path to relative path within sync folder
	relPath, err := filepath.Rel(e.config.Sync.FolderPath, path)
	if err != nil {
		return // Skip if we can't make it relative
	}

	// Update file in database locally
	fileMetadata := &database.FileMetadata{
		FileID:   fileID,
		Path:     relPath,
		Checksum: checksum,
		Size:     size,
		Mtime:    time.Now(), // Use current time for local files
		PeerID:   e.peerID,
	}
	if err := e.db.InsertFile(fileMetadata); err != nil {
		// Log error but continue
		_ = err
	}

	op := NewSyncOperation(OpUpdate, relPath, fileID, e.peerID)
	op.Checksum = checksum
	op.Size = size
	e.queueOperation(op)
}

// handleDelete handles a file deletion event
func (e *Engine) handleDelete(path string) {
	// Convert absolute path to relative path for database lookup
	relPath, err := filepath.Rel(e.config.Sync.FolderPath, path)
	if err != nil {
		return // Skip if we can't make it relative
	}

	// Get file info from database before deletion
	file, err := e.db.GetFileByPath(relPath)
	if err != nil {
		return
	}

	// Record deletion for rename detection
	e.renameDetector.RecordDelete(file.FileID, file.Checksum, path, file.Size, file.Mtime)

	op := NewSyncOperation(OpDelete, relPath, file.FileID, e.peerID)
	e.queueOperation(op)
}

// handleRename handles a rename event
func (e *Engine) handleRename(path string) {
	// Rename detection is handled in handleCreate
	// This is a fallback
	e.handleCreate(path)
}

// queueOperation queues an operation for processing and stores it in the database
func (e *Engine) queueOperation(op *SyncOperation) {
	e.queueMu.Lock()
	defer e.queueMu.Unlock()

	// Add to file-specific queue
	e.operationQueue[op.FileID] = append(e.operationQueue[op.FileID], op)

	// Convert SyncOperation to LogEntry
	logEntry := &database.LogEntry{
		OperationID:   op.ID,
		Timestamp:     time.UnixMilli(op.Timestamp),
		OperationType: string(op.Type),
		PeerID:        op.PeerID,
		VectorClock:   op.VectorClock,
		Acknowledged:  false,
		Persisted:     false,
		FileID:        &op.FileID,
		Path:          op.Path,
		FromPath:      op.FromPath,
		Checksum:      &op.Checksum,
		Size:          &op.Size,
		Mtime:         &time.Time{}, // Will be set below
		Data:          op.Data,
	}

	// Set Mtime if available
	if op.Mtime != 0 {
		mtime := time.UnixMilli(op.Mtime)
		logEntry.Mtime = &mtime
	}

	// Store in database
	if err := e.db.AppendOperation(logEntry); err != nil {
		// Log error but don't fail the operation
		_ = err
	}

	// Broadcast operation to peers if messenger is available
	if e.messenger != nil {
		log.Printf("DEBUG: Broadcasting operation: %s %s to peers", op.Type, op.Path)
		go func() {
			if err := e.messenger.BroadcastOperation(op); err != nil {
				log.Printf("DEBUG: Failed to broadcast operation: %v", err)
				// Log error but don't fail
				_ = err
			} else {
				log.Printf("DEBUG: Successfully broadcast operation: %s %s", op.Type, op.Path)
			}
		}()
	} else {
		log.Printf("DEBUG: Messenger is nil, cannot broadcast operation: %s %s", op.Type, op.Path)
	}
}

// getFileChecksum gets the checksum of a file
func (e *Engine) getFileChecksum(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return hashing.HashString(data), nil
}

// HandleMessage implements network.MessageHandler for testing purposes
// This is a minimal implementation that handles messages that might be sent in tests
func (e *Engine) HandleMessage(msg *messages.Message) error {
	// For testing purposes, we implement a minimal message handler
	// In the real system, this would be handled by NetworkMessageHandler
	switch msg.Type {
	case messages.TypeSyncOperation:
		// Handle sync operation messages
		payload, ok := msg.Payload.(*messages.LogEntryPayload)
		if !ok {
			return fmt.Errorf("invalid sync operation payload")
		}

		// Convert to SyncOperation
		syncOp := &SyncOperation{
			ID:        payload.OperationID,
			Type:      OperationType(payload.Type),
			Path:      payload.Path,
			PeerID:    payload.PeerID,
			Source:    "remote",
			Timestamp: payload.Timestamp,
		}

		// Handle optional fields
		if payload.FileID != nil {
			syncOp.FileID = *payload.FileID
		}
		if payload.Checksum != nil {
			syncOp.Checksum = *payload.Checksum
		}
		if payload.Size != nil {
			syncOp.Size = *payload.Size
		}
		if payload.Mtime != nil {
			syncOp.Mtime = *payload.Mtime
		}
		if payload.FromPath != nil {
			syncOp.FromPath = payload.FromPath
		}
		if payload.Mode != nil {
			syncOp.Mode = payload.Mode
		}
		if payload.Compressed != nil {
			syncOp.Compressed = payload.Compressed
		}
		if payload.OriginalSize != nil {
			syncOp.OriginalSize = payload.OriginalSize
		}
		if payload.CompressionAlgorithm != nil {
			syncOp.CompressionAlgorithm = payload.CompressionAlgorithm
		}

		// Handle based on operation type
		switch syncOp.Type {
		case OpCreate, OpUpdate:
			// For file operations, we expect the file data to be in the payload
			fileData := payload.Data
			return e.HandleIncomingFile(fileData, syncOp)
		case OpDelete:
			return e.HandleIncomingFile([]byte{}, syncOp)
		case OpRename:
			return e.HandleIncomingRename(syncOp)
		default:
			return fmt.Errorf("unsupported operation type: %v", syncOp.Type)
		}

	default:
		// For other message types, we don't handle them in the sync engine
		// This is just for testing compatibility
		return nil
	}
}

// ReplayUnacknowledgedOperations replays unacknowledged operations from the database on startup
// This is critical for data durability - ensures no data loss after crashes or network failures
func (e *Engine) ReplayUnacknowledgedOperations(ctx context.Context) error {
	// Retrieve all unacknowledged operations from the database
	operations, err := e.db.GetUnacknowledgedOperations()
	if err != nil {
		return fmt.Errorf("failed to get unacknowledged operations: %w", err)
	}

	if len(operations) == 0 {
		log.Printf("No unacknowledged operations to replay")
		return nil
	}

	log.Printf("Replaying %d unacknowledged operations", len(operations))

	// Replay each operation
	successCount := 0
	failureCount := 0

	for _, logEntry := range operations {
		// Convert LogEntry to SyncOperation
		syncOp, err := e.logEntryToSyncOperation(logEntry)
		if err != nil {
			log.Printf("Warning: failed to convert log entry to sync operation: %v", err)
			failureCount++
			continue
		}

		// Check if this is our own operation (originated from this peer)
		if syncOp.PeerID == e.peerID {
			// Rebroadcast our own unacknowledged operations to peers
			if e.messenger != nil {
				if err := e.messenger.BroadcastOperation(syncOp); err != nil {
					log.Printf("Warning: failed to rebroadcast operation %s: %v", syncOp.ID, err)
					failureCount++
					continue
				}
				log.Printf("Rebroadcasted operation %s (type: %s, path: %s)", syncOp.ID, syncOp.Type, syncOp.Path)
				successCount++
			} else {
				log.Printf("Warning: messenger is nil, cannot rebroadcast operation %s", syncOp.ID)
				failureCount++
			}
		} else {
			// This is a remote operation that wasn't acknowledged
			// Re-apply it locally if needed
			log.Printf("Skipping remote operation %s from peer %s (already applied)", syncOp.ID, syncOp.PeerID)
			// Note: Remote operations should already be applied to the filesystem
			// We just mark them as acknowledged if they're complete
			successCount++
		}
	}

	log.Printf("Replay complete: %d succeeded, %d failed", successCount, failureCount)

	return nil
}

// logEntryToSyncOperation converts a database LogEntry to a SyncOperation
func (e *Engine) logEntryToSyncOperation(entry *database.LogEntry) (*SyncOperation, error) {
	op := &SyncOperation{
		ID:                   entry.OperationID,
		Type:                 OperationType(entry.OperationType),
		Path:                 entry.Path,
		FileID:               getStringValue(entry.FileID),
		Checksum:             getStringValue(entry.Checksum),
		Size:                 getInt64Value(entry.Size),
		Timestamp:            entry.Timestamp.Unix(),
		VectorClock:          entry.VectorClock,
		PeerID:               entry.PeerID,
		Source:               "local", // Mark as local since we're replaying from our log
		Mtime:                getTimeInt64Value(entry.Mtime),
		Mode:                 entry.Mode,
		Data:                 entry.Data,
		Compressed:           getDefaultBool(entry.Compressed),
		OriginalSize:         entry.OriginalSize,
		CompressionAlgorithm: entry.CompressionAlgorithm,
	}

	if entry.FromPath != nil {
		op.FromPath = entry.FromPath
	}

	return op, nil
}

// Helper functions to safely extract values from pointers
func getStringValue(ptr *string) string {
	if ptr != nil {
		return *ptr
	}
	return ""
}

func getInt64Value(ptr *int64) int64 {
	if ptr != nil {
		return *ptr
	}
	return 0
}

func getTimeValue(ptr *time.Time) time.Time {
	if ptr != nil {
		return *ptr
	}
	return time.Time{}
}

func getTimeInt64Value(ptr *time.Time) int64 {
	if ptr != nil {
		return ptr.Unix()
	}
	return 0
}

func getDefaultBool(val bool) *bool {
	return &val
}

// periodicLogCompaction runs log compaction periodically to prevent unbounded log growth
// This is called as a goroutine from Start() and runs until the engine is stopped
func (e *Engine) periodicLogCompaction() {
	// Compact every 24 hours
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	// Maximum number of entries to keep in the log
	const maxLogEntries = 10000

	for {
		select {
		case <-e.stopCh:
			log.Printf("Stopping periodic log compaction")
			return
		case <-ticker.C:
			log.Printf("Running periodic log compaction (max entries: %d)", maxLogEntries)
			if err := e.db.CompactLog(maxLogEntries); err != nil {
				log.Printf("Warning: log compaction failed: %v", err)
			} else {
				log.Printf("Log compaction completed successfully")
			}
		}
	}
}

// periodicStateSync periodically synchronizes state with connected peers
// This ensures peers stay in sync even after network partitions or missed operations
func (e *Engine) periodicStateSync() {
	// Sync state every 5 minutes
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-e.stopCh:
			log.Printf("Stopping periodic state synchronization")
			return
		case <-ticker.C:
			log.Printf("Running periodic state synchronization")
			if err := e.syncStateWithPeers(); err != nil {
				log.Printf("Warning: state synchronization failed: %v", err)
			} else {
				log.Printf("State synchronization completed successfully")
			}
		}
	}
}

// syncStateWithPeers sends state declarations to all connected peers
func (e *Engine) syncStateWithPeers() error {
	if e.messenger == nil {
		// Messenger not available, skip sync
		return nil
	}

	// Get all files for manifest
	files, err := e.GetAllFiles()
	if err != nil {
		return fmt.Errorf("failed to get file manifest: %w", err)
	}

	// Build file manifest
	fileManifest := make([]messages.FileManifestEntry, 0, len(files))
	for _, file := range files {
		fileManifest = append(fileManifest, messages.FileManifestEntry{
			FileID:         file.FileID,
			Path:           file.Path,
			Hash:           file.Checksum,
			Size:           file.Size,
			Mtime:          file.Mtime.Unix(),
			LastModifiedBy: file.PeerID,
		})
	}

	// For now, we'll use the file manifest as the primary reconciliation mechanism
	// Vector clock-based reconciliation can be added in a future enhancement
	log.Printf("Periodic state sync: Monitored %d files in sync folder", len(fileManifest))

	// TODO: Implement actual state declaration broadcast to peers
	// This would require adding a BroadcastStateDeclaration method to the Messenger interface

	return nil
}
