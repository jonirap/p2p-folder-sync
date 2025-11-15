package network

import (
	"fmt"
	"sync"

	"github.com/p2p-folder-sync/p2p-sync/internal/chunking"
	"github.com/p2p-folder-sync/p2p-sync/internal/compression"
	"github.com/p2p-folder-sync/p2p-sync/internal/config"
	"github.com/p2p-folder-sync/p2p-sync/internal/database"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/connection"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/messages"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/transport"
	syncpkg "github.com/p2p-folder-sync/p2p-sync/internal/sync"
)

// MessageHandler handles incoming network messages and routes them appropriately
type MessageHandler interface {
	HandleMessage(msg *messages.Message) error
}

// NetworkMessageHandler implements MessageHandler for the P2P sync system
type NetworkMessageHandler struct {
	config           *config.Config
	connManager      *connection.ConnectionManager
	transport        transport.Transport
	syncEngine       *syncpkg.Engine
	heartbeatManager HeartbeatManager
	chunkAssembler   *chunking.Assembler
	assembledChunks  map[string][]*chunking.Chunk // fileID -> chunks
	assemblerMu      sync.RWMutex
	peerID           string
}

// HeartbeatManager interface for handling heartbeats
type HeartbeatManager interface {
	HandleHeartbeatResponse(peerID string)
}

// NewNetworkMessageHandler creates a new network message handler
func NewNetworkMessageHandler(cfg *config.Config, syncEngine *syncpkg.Engine, heartbeatManager HeartbeatManager, peerID string) *NetworkMessageHandler {
	return &NetworkMessageHandler{
		config:           cfg,
		syncEngine:       syncEngine,
		heartbeatManager: heartbeatManager,
		chunkAssembler:   chunking.NewAssembler(),
		assembledChunks:  make(map[string][]*chunking.Chunk),
		peerID:           peerID,
	}
}

// SetSyncEngine sets the sync engine for the message handler
func (h *NetworkMessageHandler) SetSyncEngine(syncEngine *syncpkg.Engine) {
	h.syncEngine = syncEngine
}

// SetHeartbeatManager sets the heartbeat manager for the message handler
func (h *NetworkMessageHandler) SetHeartbeatManager(heartbeatManager HeartbeatManager) {
	h.heartbeatManager = heartbeatManager
}

// HandleMessage routes incoming messages to appropriate handlers
func (h *NetworkMessageHandler) HandleMessage(msg *messages.Message) error {
	switch msg.Type {
	case messages.TypeDiscovery:
		return h.handleDiscovery(msg)
	case messages.TypeDiscoveryResponse:
		return h.handleDiscoveryResponse(msg)
	case messages.TypeHandshake:
		return h.handleHandshake(msg)
	case messages.TypeHandshakeAck:
		return h.handleHandshakeAck(msg)
	case messages.TypeHandshakeComplete:
		return h.handleHandshakeComplete(msg)
	case messages.TypeStateDeclaration:
		return h.handleStateDeclaration(msg)
	case messages.TypeFileRequest:
		return h.handleFileRequest(msg)
	case messages.TypeSyncOperation:
		return h.handleSyncOperation(msg)
	case messages.TypeChunk:
		return h.handleChunk(msg)
	case messages.TypeChunkRequest:
		return h.handleChunkRequest(msg)
	case messages.TypeOperationAck:
		return h.handleOperationAck(msg)
	case messages.TypeChunkAck:
		return h.handleChunkAck(msg)
	case messages.TypeHeartbeat:
		return h.handleHeartbeat(msg)
	default:
		return fmt.Errorf("unknown message type: %s", msg.Type)
	}
}

// handleDiscovery handles peer discovery messages
func (h *NetworkMessageHandler) handleDiscovery(msg *messages.Message) error {
	// Discovery messages are handled at the transport level
	// This handler is for application-level messages
	return nil
}

// handleDiscoveryResponse handles discovery response messages
func (h *NetworkMessageHandler) handleDiscoveryResponse(msg *messages.Message) error {
	// Discovery responses are handled at the transport level
	return nil
}

// handleHandshake handles handshake initiation messages
func (h *NetworkMessageHandler) handleHandshake(msg *messages.Message) error {
	// Handshake is handled by the handshake package
	// This would be routed to the handshake handler
	return fmt.Errorf("handshake handling not implemented yet")
}

// handleHandshakeAck handles handshake acknowledgment messages
func (h *NetworkMessageHandler) handleHandshakeAck(msg *messages.Message) error {
	// Handshake acknowledgment is handled by the handshake package
	return fmt.Errorf("handshake ack handling not implemented yet")
}

// handleHandshakeComplete handles handshake completion messages
func (h *NetworkMessageHandler) handleHandshakeComplete(msg *messages.Message) error {
	// Handshake completion is handled by the handshake package
	return fmt.Errorf("handshake complete handling not implemented yet")
}

// handleStateDeclaration handles state declaration messages
func (h *NetworkMessageHandler) handleStateDeclaration(msg *messages.Message) error {
	if h.syncEngine == nil {
		return h.sendAcknowledgment(msg, false, "sync engine not available")
	}

	var stateMsg messages.StateDeclarationMessage
	payload, err := messages.DecodePayload(msg.Payload.([]byte), msg.Type)
	if err != nil {
		return h.sendAcknowledgment(msg, false, fmt.Sprintf("failed to decode state message: %v", err))
	}
	stateMsg = payload.(messages.StateDeclarationMessage)

	// If this is a state request (empty file manifest), respond with our state
	if len(stateMsg.FileManifest) == 0 {
		// This is a request for our state - send our file manifest
		files, err := h.getLocalFileManifest()
		if err != nil {
			return h.sendAcknowledgment(msg, false, fmt.Sprintf("failed to get file manifest: %v", err))
		}

		responseMsg := messages.NewMessage(messages.TypeStateDeclaration, stateMsg.PeerID, messages.StateDeclarationMessage{
			PeerID:       stateMsg.PeerID, // Respond to the requester
			FileManifest: files,
		})

		// Send response back to requester
		conn, err := h.connManager.GetConnection(stateMsg.PeerID)
		if err != nil {
			return h.sendAcknowledgment(msg, false, fmt.Sprintf("no connection to peer: %v", err))
		}

		if err := h.transport.SendMessage(stateMsg.PeerID, conn.Address, conn.Port, responseMsg); err != nil {
			return h.sendAcknowledgment(msg, false, fmt.Sprintf("failed to send state response: %v", err))
		}

		return h.sendAcknowledgment(msg, true, "")
	} else {
		// This is a state declaration from a peer - check what files we need
		missingFiles, err := h.compareFileManifests(stateMsg.FileManifest)
		if err != nil {
			return h.sendAcknowledgment(msg, false, fmt.Sprintf("failed to compare manifests: %v", err))
		}

		// Request missing files
		if len(missingFiles) > 0 {
			if err := h.requestMissingFiles(stateMsg.PeerID, missingFiles); err != nil {
				return h.sendAcknowledgment(msg, false, fmt.Sprintf("failed to request files: %v", err))
			}
		}

		return h.sendAcknowledgment(msg, true, "")
	}
}

// handleFileRequest handles file request messages for new peer sync
func (h *NetworkMessageHandler) handleFileRequest(msg *messages.Message) error {
	// File request handling would be implemented for load balancing
	// For now, just acknowledge receipt
	return h.sendAcknowledgment(msg, true, "")
}

// handleSyncOperation handles sync operation messages (file operations)
func (h *NetworkMessageHandler) handleSyncOperation(msg *messages.Message) error {
	payload, ok := msg.Payload.(*messages.LogEntryPayload)
	if !ok {
		return fmt.Errorf("invalid sync operation payload")
	}

	// Convert LogEntryPayload to SyncOperation
	syncOp := &syncpkg.SyncOperation{
		ID:        payload.OperationID,
		Type:      syncpkg.OperationType(payload.Type),
		Path:      payload.Path,
		FileID:    *payload.FileID,
		Checksum:  *payload.Checksum,
		Size:      *payload.Size,
		Timestamp: payload.Timestamp,
		PeerID:    payload.PeerID,
		Source:    "remote", // All incoming operations are remote
		Mtime:     *payload.Mtime,
	}

	// Handle optional fields
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

	// Convert VectorClock map back to VectorClock
	if payload.VectorClock != nil {
		syncOp.VectorClock = syncpkg.VectorClock(payload.VectorClock)
	}

	// Handle the operation based on type
	switch syncOp.Type {
	case syncpkg.OpCreate, syncpkg.OpUpdate:
		// File data is included in the payload for small files
		fileData := payload.Data
		if len(fileData) == 0 {
			// Large file, expect chunks to follow
			// For now, just acknowledge and wait for chunks
			return h.sendAcknowledgment(msg, true, "")
		}

		// Decompress if needed
		if syncOp.Compressed != nil && *syncOp.Compressed {
			decompressed, err := h.decompressFileData(fileData, syncOp.CompressionAlgorithm)
			if err != nil {
				return h.sendAcknowledgment(msg, false, fmt.Sprintf("decompression failed: %v", err))
			}
			fileData = decompressed
		}

		// Handle the incoming file
		if err := h.syncEngine.HandleIncomingFile(fileData, syncOp); err != nil {
			return h.sendAcknowledgment(msg, false, fmt.Sprintf("file handling failed: %v", err))
		}

	case syncpkg.OpDelete:
		// Handle delete operation
		if err := h.syncEngine.HandleIncomingFile([]byte{}, syncOp); err != nil {
			return h.sendAcknowledgment(msg, false, fmt.Sprintf("delete handling failed: %v", err))
		}

	case syncpkg.OpRename:
		// Handle rename operation
		if err := h.syncEngine.HandleIncomingRename(syncOp); err != nil {
			return h.sendAcknowledgment(msg, false, fmt.Sprintf("rename handling failed: %v", err))
		}
	}

	return h.sendAcknowledgment(msg, true, "")
}

// handleChunk handles file chunk messages
func (h *NetworkMessageHandler) handleChunk(msg *messages.Message) error {
	payload, ok := msg.Payload.(*messages.ChunkMessage)
	if !ok {
		return fmt.Errorf("invalid chunk payload")
	}

	// Create chunk from payload
	chunk := &chunking.Chunk{
		FileID:      payload.FileID,
		ChunkID:     payload.ChunkID,
		Offset:      payload.Offset,
		Length:      payload.Length,
		Data:        payload.Data,
		Hash:        payload.ChunkHash,
		IsLast:      payload.IsLast,
		TotalChunks: payload.TotalChunks,
	}

	// Verify chunk hash
	if err := h.chunkAssembler.VerifyChunkHash(chunk); err != nil {
		return h.sendAcknowledgment(msg, false, fmt.Sprintf("chunk hash verification failed: %v", err))
	}

	// Store chunk for assembly
	h.assemblerMu.Lock()
	if h.assembledChunks[payload.FileID] == nil {
		h.assembledChunks[payload.FileID] = make([]*chunking.Chunk, 0)
	}
	h.assembledChunks[payload.FileID] = append(h.assembledChunks[payload.FileID], chunk)
	chunks := h.assembledChunks[payload.FileID]
	h.assemblerMu.Unlock()

	// Check if we have all chunks
	if len(chunks) == payload.TotalChunks {
		// Assemble the file
		assembledData, err := h.chunkAssembler.AssembleChunks(chunks, payload.FileHash)
		if err != nil {
			return h.sendAcknowledgment(msg, false, fmt.Sprintf("file assembly failed: %v", err))
		}

		// Clean up assembled chunks
		h.assemblerMu.Lock()
		delete(h.assembledChunks, payload.FileID)
		h.assemblerMu.Unlock()

		// Create sync operation for the assembled file
		syncOp := &syncpkg.SyncOperation{
			Type:     syncpkg.OpCreate, // Assume create for now
			Path:     "",               // Path would need to be determined
			FileID:   payload.FileID,
			Checksum: payload.FileHash,
			Size:     int64(len(assembledData)),
			PeerID:   msg.SenderID,
			Source:   "remote",
		}

		// Handle the assembled file
		if err := h.syncEngine.HandleIncomingFile(assembledData, syncOp); err != nil {
			return h.sendAcknowledgment(msg, false, fmt.Sprintf("assembled file handling failed: %v", err))
		}
	}

	return h.sendAcknowledgment(msg, true, "")
}

// handleChunkRequest handles chunk request messages
func (h *NetworkMessageHandler) handleChunkRequest(msg *messages.Message) error {
	// Chunk request handling would read and send requested chunks
	// For now, just acknowledge
	return h.sendAcknowledgment(msg, true, "")
}

// handleOperationAck handles operation acknowledgment messages
func (h *NetworkMessageHandler) handleOperationAck(msg *messages.Message) error {
	// Acknowledgments are handled by the messenger
	return nil
}

// handleChunkAck handles chunk acknowledgment messages
func (h *NetworkMessageHandler) handleChunkAck(msg *messages.Message) error {
	// Acknowledgments are handled by the messenger
	return nil
}

// handleHeartbeat handles heartbeat messages
func (h *NetworkMessageHandler) handleHeartbeat(msg *messages.Message) error {
	// Update heartbeat timestamp for the sender
	if h.heartbeatManager != nil {
		h.heartbeatManager.HandleHeartbeatResponse(msg.SenderID)
	}
	return nil
}

// sendAcknowledgment sends an acknowledgment for a received message
func (h *NetworkMessageHandler) sendAcknowledgment(originalMsg *messages.Message, success bool, errorMsg string) error {
	// This would send an acknowledgment back to the sender
	// For now, just log the acknowledgment
	if success {
		fmt.Printf("Acknowledged message %s from %s\n", originalMsg.ID, originalMsg.SenderID)
	} else {
		fmt.Printf("Rejected message %s from %s: %s\n", originalMsg.ID, originalMsg.SenderID, errorMsg)
	}
	return nil
}

// decompressFileData decompresses file data using the specified algorithm
func (h *NetworkMessageHandler) decompressFileData(data []byte, algorithm *string) ([]byte, error) {
	if algorithm == nil {
		return data, nil
	}

	// Create decompressor based on algorithm
	compressor, err := compression.NewCompressor(*algorithm, 0) // Level doesn't matter for decompression
	if err != nil {
		return nil, fmt.Errorf("failed to create decompressor: %w", err)
	}

	return compressor.Decompress(data)
}

// getLocalFileManifest returns a manifest of all local files
func (h *NetworkMessageHandler) getLocalFileManifest() ([]messages.FileManifestEntry, error) {
	// Query all files from database
	files, err := h.syncEngine.GetAllFiles()
	if err != nil {
		return nil, fmt.Errorf("failed to get files from database: %w", err)
	}

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

	return manifest, nil
}

// compareFileManifests compares peer's manifest with local files and returns missing files
func (h *NetworkMessageHandler) compareFileManifests(peerManifest []messages.FileManifestEntry) ([]messages.FileManifestEntry, error) {
	localFiles, err := h.syncEngine.GetAllFiles()
	if err != nil {
		return nil, fmt.Errorf("failed to get local files: %w", err)
	}

	// Create map of local files by FileID
	localFileMap := make(map[string]*database.FileMetadata)
	for _, file := range localFiles {
		localFileMap[file.FileID] = file
	}

	var missingFiles []messages.FileManifestEntry

	for _, peerFile := range peerManifest {
		localFile, exists := localFileMap[peerFile.FileID]
		if !exists {
			// File doesn't exist locally
			missingFiles = append(missingFiles, peerFile)
		} else if localFile.Checksum != peerFile.Hash {
			// File exists but checksum differs (conflict)
			// For now, treat as missing (will be resolved by conflict resolver)
			missingFiles = append(missingFiles, peerFile)
		}
		// If file exists and checksum matches, we already have it
	}

	return missingFiles, nil
}

// requestMissingFiles sends file requests for missing files
func (h *NetworkMessageHandler) requestMissingFiles(peerID string, missingFiles []messages.FileManifestEntry) error {
	if len(missingFiles) == 0 {
		return nil
	}

	// Convert to RequestedFile format
	requestedFiles := make([]messages.RequestedFile, 0, len(missingFiles))
	for _, file := range missingFiles {
		requestedFiles = append(requestedFiles, messages.RequestedFile{
			FileID:   file.FileID,
			Priority: "normal", // Could be determined by file type/size
		})
	}

	// Create file request message
	msg := &messages.Message{
		ID:       messages.GenerateMessageID(),
		Type:     messages.TypeFileRequest,
		SenderID: h.peerID,
		Payload: messages.FileRequestMessage{
			RequestedFiles: requestedFiles,
			PeerCapabilities: messages.PeerCapabilities{
				SupportsCompression:    true,
				MaxConcurrentTransfers: 5,
			},
		},
	}

	// Send to peer
	conn, err := h.connManager.GetConnection(peerID)
	if err != nil {
		return fmt.Errorf("no connection to peer %s: %w", peerID, err)
	}

	return h.transport.SendMessage(peerID, conn.Address, conn.Port, msg)
}
