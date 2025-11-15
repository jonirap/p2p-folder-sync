package sync

import (
	"os"
	"path/filepath"
	"sync"
)

// InMemoryMessenger provides in-memory messaging between sync engines for testing
type InMemoryMessenger struct {
	mu          sync.RWMutex
	engines     map[string]*Engine
	peerMapping map[string]string // peerID -> engine peerID
}

// NewInMemoryMessenger creates a new in-memory messenger
func NewInMemoryMessenger() *InMemoryMessenger {
	return &InMemoryMessenger{
		engines:     make(map[string]*Engine),
		peerMapping: make(map[string]string),
	}
}

// RegisterEngine registers a sync engine with the messenger
func (m *InMemoryMessenger) RegisterEngine(peerID string, engine *Engine) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.engines[peerID] = engine
	m.peerMapping[peerID] = peerID
}

// RequestStateSync is a stub implementation for testing (state sync not used in in-memory tests)
func (m *InMemoryMessenger) RequestStateSync(peerID string) error {
	// State sync is not implemented for in-memory testing
	return nil
}

// SendFile sends a file to a specific peer
func (m *InMemoryMessenger) SendFile(peerID string, fileData []byte, metadata *SyncOperation) error {
	m.mu.RLock()
	engine, exists := m.engines[peerID]
	m.mu.RUnlock()

	if !exists {
		return nil // Peer not found, silently ignore for testing
	}

	// Deliver the file to the peer's sync engine
	go func() {
		if err := engine.HandleIncomingFile(fileData, metadata); err != nil {
			// Log error but don't fail
			_ = err
		}
	}()

	return nil
}

// BroadcastOperation broadcasts an operation to all other peers
func (m *InMemoryMessenger) BroadcastOperation(op *SyncOperation) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Handle different operation types
	switch op.Type {
	case OpCreate, OpUpdate:
		// Find the source engine to get the correct absolute path
		sourceEngine, exists := m.engines[op.PeerID]
		if !exists {
			return nil // Source peer not found
		}

		// Convert relative path back to absolute path for source
		sourcePath := filepath.Join(sourceEngine.config.Sync.FolderPath, op.Path)
		fileData, err := os.ReadFile(sourcePath)
		if err != nil {
			return err // Can't send file if we can't read it
		}

		// Send to all other peers
		for peerID, engine := range m.engines {
			if peerID != op.PeerID {
				go func(targetPeerID string, targetEngine *Engine) {
					// Send file data (keep relative path)
					fileOp := &SyncOperation{
						ID:        op.ID,
						Type:      op.Type,
						Path:      op.Path, // Keep relative path
						FileID:    op.FileID,
						Checksum:  op.Checksum,
						Size:      op.Size,
						Timestamp: op.Timestamp,
						PeerID:    op.PeerID,
						Source:    "remote",
						Mtime:     op.Mtime,
					}
					if err := targetEngine.HandleIncomingFile(fileData, fileOp); err != nil {
						_ = err
					}
				}(peerID, engine)
			}
		}
	case OpDelete:
		// For delete operations, just broadcast the operation to all peers
		for peerID, engine := range m.engines {
			if peerID != op.PeerID {
				go func(targetPeerID string, targetEngine *Engine) {
					// Send delete operation (keep relative path)
					deleteOp := &SyncOperation{
						ID:        op.ID,
						Type:      op.Type,
						Path:      op.Path, // Keep relative path
						FileID:    op.FileID,
						Timestamp: op.Timestamp,
						PeerID:    op.PeerID,
						Source:    "remote",
					}
					// For delete operations, we call HandleIncomingFile with empty data
					// The sync engine should handle this appropriately
					if err := targetEngine.HandleIncomingFile([]byte{}, deleteOp); err != nil {
						_ = err
					}
				}(peerID, engine)
			}
		}
	case OpRename:
		// For rename operations, broadcast the operation to all peers
		for peerID, engine := range m.engines {
			if peerID != op.PeerID {
				go func(targetPeerID string, targetEngine *Engine) {
					// Send rename operation (keep paths relative, HandleIncomingRename will convert)
					renameOp := &SyncOperation{
						ID:        op.ID,
						Type:      op.Type,
						Path:      op.Path,     // Keep relative path
						FromPath:  op.FromPath, // Keep relative path
						FileID:    op.FileID,
						Checksum:  op.Checksum,
						Size:      op.Size,
						Timestamp: op.Timestamp,
						PeerID:    op.PeerID,
						Source:    "remote",
						Mtime:     op.Mtime,
					}
					// For rename operations, call HandleIncomingRename
					if err := targetEngine.HandleIncomingRename(renameOp); err != nil {
						_ = err
					}
				}(peerID, engine)
			}
		}
	}

	return nil
}
