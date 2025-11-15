package state

import (
	"github.com/p2p-folder-sync/p2p-sync/internal/database"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/messages"
	"github.com/p2p-folder-sync/p2p-sync/internal/sync"
)

// StateDeclaration represents a peer's current state
type StateDeclaration struct {
	PeerID           string
	VectorClock      sync.VectorClock
	FileManifest     []messages.FileManifestEntry
	PendingOperations []messages.LogEntryPayload
}

// BuildStateDeclaration builds a state declaration from the database
func BuildStateDeclaration(db *database.DB, peerID string) (*StateDeclaration, error) {
	// Get all files
	files, err := db.GetAllFiles()
	if err != nil {
		return nil, err
	}

	// Build file manifest
	manifest := make([]messages.FileManifestEntry, 0, len(files))
	for _, file := range files {
		manifest = append(manifest, messages.FileManifestEntry{
			FileID:         file.FileID,
			Path:           file.Path,
			Hash:           file.Checksum,
			Size:           file.Size,
			Mtime:          file.Mtime.Unix(),
			LastModifiedBy: file.PeerID,
		})
	}

	// Get unacknowledged operations
	operations, err := db.GetUnacknowledgedOperations()
	if err != nil {
		return nil, err
	}

	// Build pending operations
	pendingOps := make([]messages.LogEntryPayload, 0, len(operations))
	for _, op := range operations {
		payload := messages.LogEntryPayload{
			OperationID: op.OperationID,
			Timestamp:   op.Timestamp.Unix(),
			Type:        op.OperationType,
			PeerID:      op.PeerID,
			VectorClock: op.VectorClock,
			Path:        op.Path,
		}

		if op.FileID != nil {
			payload.FileID = op.FileID
		}
		if op.FromPath != nil {
			payload.FromPath = op.FromPath
		}
		if op.Checksum != nil {
			payload.Checksum = op.Checksum
		}
		if op.Size != nil {
			payload.Size = op.Size
		}
		if op.Mtime != nil {
			ts := op.Mtime.Unix()
			payload.Mtime = &ts
		}
		if op.Mode != nil {
			payload.Mode = op.Mode
		}
		if len(op.Data) > 0 {
			payload.Data = op.Data
		}
		payload.Compressed = &op.Compressed
		if op.OriginalSize != nil {
			payload.OriginalSize = op.OriginalSize
		}
		if op.CompressionAlgorithm != nil {
			payload.CompressionAlgorithm = op.CompressionAlgorithm
		}

		pendingOps = append(pendingOps, payload)
	}

	// Build vector clock from operations
	vc := sync.NewVectorClock()
	for _, op := range operations {
		vc.Merge(op.VectorClock)
	}

	return &StateDeclaration{
		PeerID:           peerID,
		VectorClock:      vc,
		FileManifest:     manifest,
		PendingOperations: pendingOps,
	}, nil
}

