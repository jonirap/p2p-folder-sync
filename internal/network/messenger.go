package network

import (
	"crypto/rand"
	"fmt"
	"sync"
	"time"

	"github.com/p2p-folder-sync/p2p-sync/internal/chunking"
	"github.com/p2p-folder-sync/p2p-sync/internal/compression"
	"github.com/p2p-folder-sync/p2p-sync/internal/config"
	"github.com/p2p-folder-sync/p2p-sync/internal/crypto"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/connection"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/messages"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/transport"
	syncpkg "github.com/p2p-folder-sync/p2p-sync/internal/sync"
)

// NetworkMessenger implements the sync.Messenger interface for network communication
type NetworkMessenger struct {
	config         *config.Config
	connManager    *connection.ConnectionManager
	transport      transport.Transport
	compressor     compression.Compressor
	chunker        *chunking.Chunker
	messageHandler MessageHandler        // Application-level message handler
	pendingAcks    map[string]chan error // messageID -> ack channel
	pendingAcksMu  sync.RWMutex
	ackTimeout     time.Duration
	retryCount     int
	retryDelay     time.Duration
	peerID         string
}

// NewNetworkMessenger creates a new network messenger
func NewNetworkMessenger(cfg *config.Config, connManager *connection.ConnectionManager, transport transport.Transport, peerID string) (*NetworkMessenger, error) {
	// Create compressor
	compressor, err := compression.NewCompressor(cfg.Compression.Algorithm, cfg.Compression.Level)
	if err != nil {
		return nil, fmt.Errorf("failed to create compressor: %w", err)
	}

	// Create chunker
	chunker := chunking.NewChunker(cfg.Sync.ChunkSizeDefault)

	messenger := &NetworkMessenger{
		config:      cfg,
		connManager: connManager,
		transport:   transport,
		compressor:  compressor,
		chunker:     chunker,
		pendingAcks: make(map[string]chan error),
		ackTimeout:  30 * time.Second, // 30s timeout for acknowledgments
		retryCount:  3,                // Retry failed sends up to 3 times
		retryDelay:  1 * time.Second,  // 1s delay between retries
		peerID:      peerID,
	}

	// Set up message handler for receiving acknowledgments
	transport.SetMessageHandler(messenger)

	return messenger, nil
}

// SendFile sends a file to a specific peer with compression, chunking, and encryption
func (nm *NetworkMessenger) SendFile(peerID string, fileData []byte, metadata *syncpkg.SyncOperation) error {
	// Check if peer is connected
	conn, err := nm.connManager.GetConnection(peerID)
	if err != nil {
		return fmt.Errorf("peer not connected: %s", peerID)
	}

	// Check if we have a session key
	if len(conn.SessionKey) == 0 {
		return fmt.Errorf("no session key for peer: %s", peerID)
	}

	// Apply compression if enabled and file is large enough
	var compressedData []byte
	var compressed bool
	if nm.config.Compression.Enabled && int64(len(fileData)) > nm.config.Compression.FileSizeThreshold {
		compressedData, err = nm.compressor.Compress(fileData)
		if err != nil {
			return fmt.Errorf("compression failed: %w", err)
		}
		compressed = true

		// Update metadata
		metadata.Compressed = &compressed
		metadata.OriginalSize = &[]int64{int64(len(fileData))}[0]
		metadata.CompressionAlgorithm = &nm.config.Compression.Algorithm
	} else {
		compressedData = fileData
		compressed = false
		metadata.Compressed = &compressed
	}

	// Check if we need to chunk the file
	if int64(len(compressedData)) > nm.config.Sync.ChunkSizeDefault {
		return nm.sendChunkedFile(peerID, compressedData, metadata)
	}

	// Send as single sync operation
	return nm.sendSyncOperation(peerID, compressedData, metadata)
}

// sendChunkedFile sends a large file as multiple chunks
func (nm *NetworkMessenger) sendChunkedFile(peerID string, fileData []byte, metadata *syncpkg.SyncOperation) error {
	// Create chunks
	chunks, err := nm.chunker.ChunkFile(metadata.FileID, fileData)
	if err != nil {
		return fmt.Errorf("failed to chunk file: %w", err)
	}

	// Send initial sync operation (metadata only)
	if err := nm.sendSyncOperation(peerID, []byte{}, metadata); err != nil {
		return fmt.Errorf("failed to send sync operation: %w", err)
	}

	// Send each chunk
	for _, chunk := range chunks {
		chunkMsg := messages.NewMessage(
			messages.TypeChunk,
			metadata.PeerID,
			messages.ChunkMessage{
				FileID:      chunk.FileID,
				FileHash:    metadata.Checksum,
				ChunkID:     chunk.ChunkID,
				TotalChunks: chunk.TotalChunks,
				Offset:      chunk.Offset,
				Length:      chunk.Length,
				ChunkHash:   chunk.Hash,
				Data:        chunk.Data,
				IsLast:      chunk.IsLast,
			},
		)

		if err := nm.sendMessage(peerID, chunkMsg); err != nil {
			return fmt.Errorf("failed to send chunk %d: %w", chunk.ChunkID, err)
		}
	}

	return nil
}

// sendSyncOperation sends a sync operation message
func (nm *NetworkMessenger) sendSyncOperation(peerID string, fileData []byte, metadata *syncpkg.SyncOperation) error {
	// Convert sync operation to message payload
	payload := messages.LogEntryPayload{
		OperationID:          metadata.ID,
		Timestamp:            metadata.Timestamp,
		Type:                 string(metadata.Type),
		PeerID:               metadata.PeerID,
		Path:                 metadata.Path,
		FileID:               &metadata.FileID,
		Checksum:             &metadata.Checksum,
		Size:                 &metadata.Size,
		Mtime:                &metadata.Mtime,
		Data:                 fileData, // Include file data for small files
		Compressed:           metadata.Compressed,
		OriginalSize:         metadata.OriginalSize,
		CompressionAlgorithm: metadata.CompressionAlgorithm,
	}

	// Convert VectorClock to map
	if metadata.VectorClock != nil {
		vcMap := make(map[string]int64)
		// VectorClock is a map[string]int64, copy it
		for k, v := range metadata.VectorClock {
			vcMap[k] = v
		}
		payload.VectorClock = vcMap
	}

	msg := messages.NewMessage(messages.TypeSyncOperation, metadata.PeerID, payload)

	return nm.sendMessage(peerID, msg)
}

// BroadcastOperation broadcasts an operation to all connected peers
func (nm *NetworkMessenger) BroadcastOperation(op *syncpkg.SyncOperation) error {
	connectedPeers := nm.connManager.GetConnectedPeers()

	if len(connectedPeers) == 0 {
		// No peers connected, operation will be queued for later
		return nil
	}

	var lastErr error
	for _, peerID := range connectedPeers {
		if peerID == op.PeerID {
			// Don't send to ourselves
			continue
		}

		// For create/update operations, we need to read the file
		if op.Type == syncpkg.OpCreate || op.Type == syncpkg.OpUpdate {
			// Read file data and send to peer
			if err := nm.SendFile(peerID, []byte{}, op); err != nil {
				lastErr = fmt.Errorf("failed to send to peer %s: %w", peerID, err)
			}
		} else {
			// For other operations (delete, rename), send metadata only
			if err := nm.sendSyncOperation(peerID, []byte{}, op); err != nil {
				lastErr = fmt.Errorf("failed to send to peer %s: %w", peerID, err)
			}
		}
	}

	return lastErr
}

// sendMessage sends a message to a peer with encryption and retry logic
func (nm *NetworkMessenger) sendMessage(peerID string, msg *messages.Message) error {
	conn, err := nm.connManager.GetConnection(peerID)
	if err != nil {
		return fmt.Errorf("peer not connected: %s", peerID)
	}

	// Encrypt the message payload
	encryptedPayload, err := nm.encryptMessagePayload(msg, conn.SessionKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt message: %w", err)
	}

	// Create encrypted message
	encryptedMsg := &messages.Message{
		ID:            msg.ID,
		Type:          msg.Type,
		Timestamp:     msg.Timestamp,
		SenderID:      msg.SenderID,
		Payload:       encryptedPayload,
		CorrelationID: msg.CorrelationID,
	}

	// Send with retry logic
	var lastErr error
	for attempt := 0; attempt < nm.retryCount; attempt++ {
		if attempt > 0 {
			time.Sleep(nm.retryDelay)
		}

		// Set up acknowledgment tracking
		ackCh := nm.trackAcknowledgment(msg.ID)

		// Get peer connection info
		conn, err := nm.connManager.GetConnection(peerID)
		if err != nil {
			return fmt.Errorf("no connection info for peer: %s", peerID)
		}

		// Send the message via transport
		if err := nm.transport.SendMessage(peerID, conn.Address, conn.Port, encryptedMsg); err != nil {
			lastErr = fmt.Errorf("send attempt %d failed: %w", attempt+1, err)
			continue
		}

		// Wait for acknowledgment
		select {
		case ackErr := <-ackCh:
			if ackErr != nil {
				lastErr = fmt.Errorf("acknowledgment error: %w", ackErr)
				continue
			}
			// Success!
			nm.untrackAcknowledgment(msg.ID)
			return nil
		case <-time.After(nm.ackTimeout):
			lastErr = fmt.Errorf("acknowledgment timeout after %v", nm.ackTimeout)
			continue
		}
	}

	nm.untrackAcknowledgment(msg.ID)
	return fmt.Errorf("failed to send message after %d attempts: %w", nm.retryCount, lastErr)
}

// encryptMessagePayload encrypts the message payload
func (nm *NetworkMessenger) encryptMessagePayload(msg *messages.Message, sessionKey []byte) (*crypto.EncryptedMessage, error) {
	if len(sessionKey) == 0 {
		return nil, fmt.Errorf("no session key available")
	}

	// Convert payload to JSON for encryption
	payloadData, err := messages.EncodePayload(msg.Payload)
	if err != nil {
		return nil, fmt.Errorf("failed to encode payload: %w", err)
	}

	// Encrypt the payload
	return crypto.Encrypt(payloadData, sessionKey)
}

// trackAcknowledgment starts tracking an acknowledgment for a message
func (nm *NetworkMessenger) trackAcknowledgment(messageID string) chan error {
	nm.pendingAcksMu.Lock()
	defer nm.pendingAcksMu.Unlock()

	ackCh := make(chan error, 1)
	nm.pendingAcks[messageID] = ackCh
	return ackCh
}

// untrackAcknowledgment stops tracking an acknowledgment
func (nm *NetworkMessenger) untrackAcknowledgment(messageID string) {
	nm.pendingAcksMu.Lock()
	defer nm.pendingAcksMu.Unlock()
	delete(nm.pendingAcks, messageID)
}

// HandleMessage handles incoming messages (implements MessageHandler)
func (nm *NetworkMessenger) HandleMessage(msg *messages.Message) error {
	// Decrypt the message if it's encrypted
	conn, err := nm.connManager.GetConnection(msg.SenderID)
	if err != nil {
		return fmt.Errorf("unknown sender: %s", msg.SenderID)
	}

	// Check if payload is encrypted
	if encryptedMsg, ok := msg.Payload.(*crypto.EncryptedMessage); ok {
		// Decrypt the payload
		decryptedData, err := crypto.Decrypt(encryptedMsg, conn.SessionKey)
		if err != nil {
			return fmt.Errorf("failed to decrypt message: %w", err)
		}

		// Parse the decrypted payload
		payload, err := messages.DecodePayload(decryptedData, msg.Type)
		if err != nil {
			return fmt.Errorf("failed to decode decrypted payload: %w", err)
		}
		msg.Payload = payload
	}

	// Handle acknowledgments
	if msg.Type == messages.TypeOperationAck || msg.Type == messages.TypeChunkAck {
		return nm.handleAcknowledgment(msg)
	}

	// Forward decrypted message to application handler
	if nm.messageHandler != nil {
		return nm.messageHandler.HandleMessage(msg)
	}

	return nil
}

// handleAcknowledgment processes acknowledgment messages
func (nm *NetworkMessenger) handleAcknowledgment(msg *messages.Message) error {
	nm.pendingAcksMu.RLock()
	ackCh, exists := nm.pendingAcks[msg.ID]
	nm.pendingAcksMu.RUnlock()

	if !exists {
		// Acknowledgment for unknown message, ignore
		return nil
	}

	// Check if acknowledgment indicates success or failure
	var ackErr error
	if ackPayload, ok := msg.Payload.(*messages.OperationAckMessage); ok {
		if !ackPayload.Success {
			ackErr = fmt.Errorf("operation failed: %s", ackPayload.Error)
		}
	} else if chunkAckPayload, ok := msg.Payload.(*messages.ChunkAckMessage); ok {
		if !chunkAckPayload.Success {
			ackErr = fmt.Errorf("chunk operation failed: %s", chunkAckPayload.Error)
		}
	}

	// Send acknowledgment result to waiting sender
	select {
	case ackCh <- ackErr:
	default:
		// Channel already closed or full, ignore
	}

	return nil
}

// SetMessageHandler sets the message handler for non-acknowledgment messages
func (nm *NetworkMessenger) SetMessageHandler(handler MessageHandler) {
	nm.messageHandler = handler
}

// RequestStateSync requests state synchronization from a peer
func (nm *NetworkMessenger) RequestStateSync(peerID string) error {
	// Create a state declaration request message
	msg := &messages.Message{
		ID:       messages.GenerateMessageID(),
		Type:     messages.TypeStateDeclaration,
		SenderID: nm.peerID,
		Payload: messages.StateDeclarationMessage{
			PeerID: peerID, // Request state from this peer
		},
	}

	// Send to the peer
	conn, err := nm.connManager.GetConnection(peerID)
	if err != nil {
		return fmt.Errorf("no connection to peer %s: %w", peerID, err)
	}

	return nm.transport.SendMessage(peerID, conn.Address, conn.Port, msg)
}

// ConnectToPeer establishes a connection to a peer and performs session key exchange
func (nm *NetworkMessenger) ConnectToPeer(peerID, address string, port int) error {
	// Establish transport connection
	if err := nm.transport.ConnectToPeer(peerID, address, port); err != nil {
		return fmt.Errorf("failed to connect transport to peer %s: %w", peerID, err)
	}

	// Add connection to connection manager
	nm.connManager.AddConnection(peerID, address, port)

	// TODO: Perform session key exchange
	// For now, we'll generate a simple session key
	// In production, this would involve ECDH key exchange
	sessionKey := make([]byte, 32)
	if _, err := rand.Read(sessionKey); err != nil {
		return fmt.Errorf("failed to generate session key: %w", err)
	}

	// Set the session key
	if err := nm.connManager.SetSessionKey(peerID, sessionKey); err != nil {
		return fmt.Errorf("failed to set session key for peer %s: %w", peerID, err)
	}

	nm.connManager.UpdateConnectionState(peerID, connection.StateConnected)

	return nil
}
