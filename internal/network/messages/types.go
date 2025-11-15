package messages

// Message type constants
const (
	TypeDiscovery         = "discovery"
	TypeDiscoveryResponse = "discovery_response"
	TypeHandshake        = "handshake"
	TypeHandshakeAck     = "handshake_ack"
	TypeHandshakeComplete = "handshake_complete"
	TypeStateDeclaration  = "state_declaration"
	TypeFileRequest       = "file_request"
	TypeSyncOperation    = "sync_operation"
	TypeChunk             = "chunk"
	TypeChunkRequest      = "chunk_request"
	TypeOperationAck     = "operation_ack"
	TypeChunkAck          = "chunk_ack"
	TypeHeartbeat         = "heartbeat"
)

// DiscoveryMessage represents a peer discovery message
type DiscoveryMessage struct {
	PeerID      string                 `json:"peer_id"`
	Port        int                    `json:"port"`
	Capabilities map[string]interface{} `json:"capabilities"`
	Version     string                 `json:"version"`
}

// DiscoveryResponseMessage represents a discovery response
type DiscoveryResponseMessage struct {
	PeerID  string `json:"peer_id"`
	Port    int    `json:"port"`
	Version string `json:"version"`
}

// HandshakeMessage represents a handshake message
type HandshakeMessage struct {
	PublicKey     string `json:"public_key"`
	Nonce         []byte `json:"nonce"`
	AuthChallenge []byte `json:"auth_challenge,omitempty"`
}

// HandshakeChallengeMessage represents a handshake challenge
type HandshakeChallengeMessage struct {
	AuthResponse  []byte `json:"auth_response"`
	PublicKey     string `json:"public_key"`
	Nonce         []byte `json:"nonce"`
	AuthChallenge []byte `json:"auth_challenge"`
}

// HandshakeCompleteMessage represents handshake completion
type HandshakeCompleteMessage struct {
	AuthResponse []byte `json:"auth_response"`
}

// StateDeclarationMessage represents a state declaration
type StateDeclarationMessage struct {
	PeerID           string                 `json:"peer_id"`
	VectorClock      map[string]int64       `json:"vector_clock"`
	FileManifest     []FileManifestEntry    `json:"file_manifest"`
	PendingOperations []LogEntryPayload     `json:"pending_operations"`
}

// FileManifestEntry represents a file in the manifest
type FileManifestEntry struct {
	FileID          string `json:"file_id"`
	Path            string `json:"path"`
	Hash            string `json:"hash"`
	Size            int64  `json:"size"`
	Mtime           int64  `json:"mtime"`
	LastModifiedBy  string `json:"last_modified_by"`
}

// FileRequestMessage represents a file request
type FileRequestMessage struct {
	RequestedFiles   []RequestedFile  `json:"requested_files"`
	PeerCapabilities PeerCapabilities  `json:"peer_capabilities"`
}

// RequestedFile represents a requested file
type RequestedFile struct {
	FileID   string `json:"file_id"`
	Priority string `json:"priority"` // "high", "normal", "low"
}

// PeerCapabilities represents peer capabilities
type PeerCapabilities struct {
	SupportsCompression    bool `json:"supports_compression"`
	MaxConcurrentTransfers int  `json:"max_concurrent_transfers"`
}

// LogEntryPayload represents a log entry in messages
type LogEntryPayload struct {
	OperationID string                 `json:"operation_id"`
	Timestamp   int64                  `json:"timestamp"`
	Type        string                 `json:"type"`
	PeerID      string                 `json:"peer_id"`
	VectorClock map[string]int64        `json:"vector_clock"`
	FileID      *string                `json:"file_id,omitempty"`
	Path        string                 `json:"path"`
	FromPath    *string                `json:"from_path,omitempty"`
	Checksum    *string                `json:"checksum,omitempty"`
	Size        *int64                 `json:"size,omitempty"`
	Mtime       *int64                 `json:"mtime,omitempty"`
	Mode        *uint32                `json:"mode,omitempty"`
	Data        []byte                 `json:"data,omitempty"`
	Compressed  *bool                  `json:"compressed,omitempty"`
	OriginalSize *int64                `json:"original_size,omitempty"`
	CompressionAlgorithm *string         `json:"compression_algorithm,omitempty"`
}

// ChunkMessage represents a file chunk
type ChunkMessage struct {
	FileID      string `json:"file_id"`
	FileHash    string `json:"file_hash"`
	ChunkID     int    `json:"chunk_id"`
	TotalChunks int    `json:"total_chunks"`
	Offset      int64  `json:"offset"`
	Length      int64  `json:"length"`
	ChunkHash   string `json:"chunk_hash"`
	Data        []byte `json:"data"`
	IsLast      bool   `json:"is_last"`
}

// ChunkRequestMessage represents a request for specific chunks
type ChunkRequestMessage struct {
	FileID   string `json:"file_id"`
	ChunkIDs []int  `json:"chunk_ids"`
}

// OperationAckMessage represents an operation acknowledgment
type OperationAckMessage struct {
	OperationID string `json:"operation_id"`
	Success     bool   `json:"success"`
	Error       string `json:"error,omitempty"`
}

// ChunkAckMessage represents a chunk acknowledgment
type ChunkAckMessage struct {
	FileID  string `json:"file_id"`
	ChunkID int    `json:"chunk_id"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// SyncOperationMessage represents a sync operation
type SyncOperationMessage struct {
	OperationID  string  `json:"operation_id"`
	Type         string  `json:"type"`
	Path         string  `json:"path"`
	FromPath     *string `json:"from_path,omitempty"`
	FileID       string  `json:"file_id"`
	Checksum     *string `json:"checksum,omitempty"`
	Size         *int64  `json:"size,omitempty"`
	Mtime        *int64  `json:"mtime,omitempty"`
	Mode         *uint32 `json:"mode,omitempty"`
	Data         []byte  `json:"data,omitempty"`
	Compressed   *bool   `json:"compressed,omitempty"`
	OriginalSize *int64  `json:"original_size,omitempty"`
	CompressionAlgorithm *string `json:"compression_algorithm,omitempty"`
}

// HeartbeatMessage represents a heartbeat message
type HeartbeatMessage struct {
	Timestamp int64 `json:"timestamp"`
}

