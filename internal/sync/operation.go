package sync

import (
	"fmt"
	"time"
)

// OperationType represents the type of sync operation
type OperationType string

const (
	OpCreate OperationType = "create"
	OpUpdate OperationType = "update"
	OpDelete OperationType = "delete"
	OpRename OperationType = "rename"
	OpMkdir  OperationType = "mkdir"
	OpRmdir  OperationType = "rmdir"
)

// SyncOperation represents a file synchronization operation
type SyncOperation struct {
	ID                   string      `json:"id"`
	Type                 OperationType `json:"type"`
	Path                 string      `json:"path"`
	FromPath             *string     `json:"from_path,omitempty"`
	FileID               string      `json:"file_id"`
	Checksum             string      `json:"checksum"`
	Size                 int64       `json:"size"`
	Timestamp            int64       `json:"timestamp"`
	VectorClock          VectorClock `json:"vector_clock"`
	PeerID               string      `json:"peer_id"`
	Source               string      `json:"source"` // "local" or "remote"
	Mtime                int64       `json:"mtime"`
	Mode                 *uint32     `json:"mode,omitempty"`
	Data                 []byte      `json:"data,omitempty"`
	ChunkID              *int        `json:"chunk_id,omitempty"`
	IsLast               *bool       `json:"is_last,omitempty"`
	Compressed           *bool       `json:"compressed,omitempty"`
	OriginalSize         *int64       `json:"original_size,omitempty"`
	CompressionAlgorithm *string      `json:"compression_algorithm,omitempty"`
}

// NewSyncOperation creates a new sync operation
func NewSyncOperation(opType OperationType, path string, fileID string, peerID string) *SyncOperation {
	return &SyncOperation{
		ID:          generateOperationID(),
		Type:        opType,
		Path:        path,
		FileID:      fileID,
		PeerID:      peerID,
		Source:      "local",
		Timestamp:   time.Now().UnixMilli(),
		VectorClock: NewVectorClock(),
	}
}

// generateOperationID generates a unique operation ID
func generateOperationID() string {
	return fmt.Sprintf("op-%d", time.Now().UnixNano())
}

