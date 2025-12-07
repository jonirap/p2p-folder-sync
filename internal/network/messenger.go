package network

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/p2p-folder-sync/p2p-sync/internal/chunking"
	"github.com/p2p-folder-sync/p2p-sync/internal/compression"
	"github.com/p2p-folder-sync/p2p-sync/internal/config"
	"github.com/p2p-folder-sync/p2p-sync/internal/crypto"
	"github.com/p2p-folder-sync/p2p-sync/internal/hashing"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/connection"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/flowcontrol"
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
	flowController *flowcontrol.FlowController // Bandwidth and concurrency control
	messageHandler MessageHandler              // Application-level message handler
	pendingAcks    map[string]chan error       // messageID -> ack channel
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

	// Create flow controller with 10MB/s global bandwidth and 5 concurrent transfers
	flowController := flowcontrol.NewFlowController(10*1024*1024, 5)

	messenger := &NetworkMessenger{
		config:         cfg,
		connManager:    connManager,
		transport:      transport,
		compressor:     compressor,
		chunker:        chunker,
		flowController: flowController,
		pendingAcks:    make(map[string]chan error),
		ackTimeout:     30 * time.Second, // 30s timeout for acknowledgments
		retryCount:     3,                // Retry failed sends up to 3 times
		retryDelay:     1 * time.Second,  // 1s delay between retries
		peerID:         peerID,
	}

	// Set up message handler for receiving acknowledgments
	transport.SetMessageHandler(messenger)

	// Try to inject connection manager if transport supports it
	// This is a bit of a hack, but necessary for TCP transport to register incoming connections
	if tcpTransport, ok := transport.(interface {
		SetConnectionManager(*connection.ConnectionManager)
	}); ok {
		tcpTransport.SetConnectionManager(connManager)
	}

	transport.SetPeerID(peerID)

	return messenger, nil
}

// SendFile sends a file to a specific peer with compression, chunking, and encryption
func (nm *NetworkMessenger) SendFile(peerID string, fileData []byte, metadata *syncpkg.SyncOperation) error {
	log.Printf("DEBUG [SendFile]: Called for peer %s, file %s, size %d", peerID, metadata.Path, len(fileData))
	// Acquire transfer slot for concurrency control
	ctx := context.Background()
	log.Printf("DEBUG [SendFile]: Acquiring transfer slot for %s", metadata.FileID)
	if err := nm.flowController.AcquireTransferSlot(ctx, metadata.FileID); err != nil {
		log.Printf("DEBUG [SendFile]: Failed to acquire transfer slot: %v", err)
		return fmt.Errorf("failed to acquire transfer slot: %w", err)
	}
	defer nm.flowController.ReleaseTransferSlot(metadata.FileID)
	log.Printf("DEBUG [SendFile]: Acquired transfer slot for %s", metadata.FileID)

	// Check if peer is connected
	conn, err := nm.connManager.GetConnection(peerID)
	if err != nil {
		return fmt.Errorf("peer not connected: %s", peerID)
	}

	// Check if we have a session key
	if len(conn.SessionKey) == 0 {
		return fmt.Errorf("no session key for peer: %s", peerID)
	}

	// Check if we need to chunk the file BEFORE compression
	// This ensures that large files are chunked even if they compress well
	needsChunking := int64(len(fileData)) > nm.config.Sync.ChunkSizeDefault

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
		metadata.CompressionLevel = &[]int{nm.config.Compression.Level}[0]
	} else {
		compressedData = fileData
		compressed = false
		metadata.Compressed = &compressed
	}

	// Send as chunks if needed (based on original file size)
	if needsChunking {
		log.Printf("DEBUG [SendFile]: Sending chunked file to %s", peerID)
		return nm.sendChunkedFile(peerID, compressedData, metadata)
	}

	// Send as single sync operation
	log.Printf("DEBUG [SendFile]: Sending single sync operation to %s", peerID)
	err = nm.sendSyncOperation(peerID, compressedData, metadata)
	log.Printf("DEBUG [SendFile]: sendSyncOperation result for %s: %v", peerID, err)
	return err
}

// sendChunkedFile sends a large file as multiple chunks
func (nm *NetworkMessenger) sendChunkedFile(peerID string, fileData []byte, metadata *syncpkg.SyncOperation) error {
	// Calculate hash of the data being chunked (could be compressed)
	// This is used for chunk assembly verification
	chunkDataHash := hashing.HashString(fileData)

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
	ctx := context.Background()
	for _, chunk := range chunks {
		// Apply bandwidth limiting before sending chunk
		if err := nm.flowController.Wait(ctx, metadata.FileID, chunk.Length); err != nil {
			return fmt.Errorf("flow control failed for chunk %d: %w", chunk.ChunkID, err)
		}

		chunkMsg := messages.NewMessage(
			messages.TypeChunk,
			metadata.PeerID,
			messages.ChunkMessage{
				FileID:      chunk.FileID,
				FileHash:    chunkDataHash, // Use hash of chunked data, not original file
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

	// Apply bandwidth limiting for file data
	if len(fileData) > 0 {
		ctx := context.Background()
		if err := nm.flowController.Wait(ctx, metadata.FileID, int64(len(fileData))); err != nil {
			return fmt.Errorf("flow control failed: %w", err)
		}
	}

	return nm.sendMessage(peerID, msg)
}

// BroadcastOperation broadcasts an operation to all connected peers
func (nm *NetworkMessenger) BroadcastOperation(op *syncpkg.SyncOperation) error {
	connectedPeers := nm.connManager.GetConnectedPeers()

	log.Printf("DEBUG [NetworkMessenger]: BroadcastOperation called for %s %s, connected peers: %d, list: %v", op.Type, op.Path, len(connectedPeers), connectedPeers)
	log.Printf("DEBUG [NetworkMessenger]: Our peer ID: %s, operation peer ID: %s", nm.peerID, op.PeerID)

	if len(connectedPeers) == 0 {
		log.Printf("DEBUG [NetworkMessenger]: No peers connected, cannot broadcast %s %s", op.Type, op.Path)
		// No peers connected, operation will be queued for later
		return nil
	}

	// Send to each peer concurrently to avoid blocking
	var wg sync.WaitGroup
	var lastErr error
	var errMu sync.Mutex

	for _, peerID := range connectedPeers {
		log.Printf("DEBUG [NetworkMessenger]: Checking peer %s (op.PeerID=%s)", peerID, op.PeerID)
		if peerID == op.PeerID {
			// Don't send to ourselves
			log.Printf("DEBUG [NetworkMessenger]: Skipping peer %s (same as operation peer ID)", peerID)
			continue
		}

		wg.Add(1)
		go func(pid string) {
			defer wg.Done()

			// For create/update operations, we need to read the file
			if op.Type == syncpkg.OpCreate || op.Type == syncpkg.OpUpdate {
				// Read file data from disk
				filePath := filepath.Join(nm.config.Sync.FolderPath, op.Path)
				log.Printf("DEBUG [NetworkMessenger]: Reading file %s for peer %s", filePath, pid)
				fileData, err := os.ReadFile(filePath)
				if err != nil {
					log.Printf("DEBUG [NetworkMessenger]: Failed to read file %s: %v", filePath, err)
					errMu.Lock()
					lastErr = fmt.Errorf("failed to read file %s: %w", filePath, err)
					errMu.Unlock()
					return
				}

				// Send file data to peer
				log.Printf("DEBUG [NetworkMessenger]: Sending file %s (%d bytes) to peer %s", op.Path, len(fileData), pid)
				if err := nm.SendFile(pid, fileData, op); err != nil {
					log.Printf("DEBUG [NetworkMessenger]: Failed to send file to peer %s: %v", pid, err)
					errMu.Lock()
					lastErr = fmt.Errorf("failed to send to peer %s: %w", pid, err)
					errMu.Unlock()
				} else {
					log.Printf("DEBUG [NetworkMessenger]: Successfully sent file %s to peer %s", op.Path, pid)
				}
			} else {
				// For other operations (delete, rename), send metadata only
				if err := nm.sendSyncOperation(pid, []byte{}, op); err != nil {
					errMu.Lock()
					lastErr = fmt.Errorf("failed to send to peer %s: %w", pid, err)
					errMu.Unlock()
				}
			}
		}(peerID)
	}

	// Wait for all sends to complete
	wg.Wait()
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
		log.Printf("DEBUG [sendMessage]: Sending message %s to peer %s", msg.ID, peerID)
		if err := nm.transport.SendMessage(peerID, conn.Address, conn.Port, encryptedMsg); err != nil {
			log.Printf("DEBUG [sendMessage]: Send failed for peer %s: %v", peerID, err)
			lastErr = fmt.Errorf("send attempt %d failed: %w", attempt+1, err)
			continue
		}
		log.Printf("DEBUG [sendMessage]: Message sent, waiting for ack from %s", peerID)

		// Wait for acknowledgment
		select {
		case ackErr := <-ackCh:
			if ackErr != nil {
				log.Printf("DEBUG [sendMessage]: Ack error from %s: %v", peerID, ackErr)
				lastErr = fmt.Errorf("acknowledgment error: %w", ackErr)
				continue
			}
			// Success!
			log.Printf("DEBUG [sendMessage]: Got ack from %s, success!", peerID)
			nm.untrackAcknowledgment(msg.ID)
			return nil
		case <-time.After(nm.ackTimeout):
			log.Printf("DEBUG [sendMessage]: Ack timeout from %s after %v", peerID, nm.ackTimeout)
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
	log.Printf("DEBUG [NetworkMessenger]: HandleMessage called, type=%s, sender=%s", msg.Type, msg.SenderID)

	// Decrypt the message if it's encrypted
	conn, err := nm.connManager.GetConnection(msg.SenderID)
	if err != nil {
		log.Printf("DEBUG [NetworkMessenger]: Unknown sender: %s", msg.SenderID)
		return fmt.Errorf("unknown sender: %s", msg.SenderID)
	}

	log.Printf("DEBUG [NetworkMessenger]: Got connection for sender %s", msg.SenderID)

	// Check if payload is encrypted
	var encryptedMsg *crypto.EncryptedMessage
	var ok bool

	// Try direct type assertion first
	if em, typeOk := msg.Payload.(*crypto.EncryptedMessage); typeOk {
		encryptedMsg = em
		ok = true
	} else if payloadMap, mapOk := msg.Payload.(map[string]interface{}); mapOk {
		// Check if it looks like an EncryptedMessage
		if iv, hasIV := payloadMap["iv"]; hasIV {
			if ct, hasCT := payloadMap["ciphertext"]; hasCT {
				if tag, hasTag := payloadMap["tag"]; hasTag {
					// Try to convert to byte slices
					if ivBytes, ok1 := toByteSlice(iv); ok1 {
						if ctBytes, ok2 := toByteSlice(ct); ok2 {
							if tagBytes, ok3 := toByteSlice(tag); ok3 {
								encryptedMsg = &crypto.EncryptedMessage{
									IV:         ivBytes,
									Ciphertext: ctBytes,
									Tag:        tagBytes,
								}
								ok = true
							}
						}
					}
				}
			}
		}
	}

	if ok {
		log.Printf("DEBUG [NetworkMessenger]: Found encrypted payload, decrypting")
		// Decrypt the payload
		decryptedData, err := crypto.Decrypt(encryptedMsg, conn.SessionKey)
		if err != nil {
			log.Printf("DEBUG [NetworkMessenger]: Decryption failed: %v", err)
			return fmt.Errorf("failed to decrypt message: %w", err)
		}
		log.Printf("DEBUG [NetworkMessenger]: Decryption successful, %d bytes", len(decryptedData))

		// Parse the decrypted payload
		payload, err := messages.DecodePayload(decryptedData, msg.Type)
		if err != nil {
			log.Printf("DEBUG [NetworkMessenger]: Decode failed: %v", err)
			return fmt.Errorf("failed to decode decrypted payload: %w", err)
		}
		log.Printf("DEBUG [NetworkMessenger]: Payload decoded successfully, type=%T", payload)
		msg.Payload = payload
	} else {
		log.Printf("DEBUG [NetworkMessenger]: No encrypted payload detected")
	}

	log.Printf("DEBUG [NetworkMessenger]: Checking if acknowledgment message, type=%s", msg.Type)
	// Handle acknowledgments
	if msg.Type == messages.TypeOperationAck || msg.Type == messages.TypeChunkAck {
		log.Printf("DEBUG [NetworkMessenger]: This is an acknowledgment, handling it")
		return nm.handleAcknowledgment(msg)
	}

	// Forward decrypted message to application handler
	if nm.messageHandler != nil {
		log.Printf("DEBUG [NetworkMessenger]: Forwarding to application handler, type=%s", msg.Type)
		return nm.messageHandler.HandleMessage(msg)
	}

	log.Printf("DEBUG [NetworkMessenger]: No message handler set!")
	return nil
}

// handleAcknowledgment processes acknowledgment messages
func (nm *NetworkMessenger) handleAcknowledgment(msg *messages.Message) error {
	// CRITICAL FIX: Use CorrelationID to match the original message, not the ACK's own ID
	if msg.CorrelationID == nil || *msg.CorrelationID == "" {
		log.Printf("DEBUG [handleAcknowledgment]: ACK message has no CorrelationID, cannot match: %s", msg.ID)
		return fmt.Errorf("acknowledgment message missing correlation ID")
	}

	originalMessageID := *msg.CorrelationID
	log.Printf("DEBUG [handleAcknowledgment]: Received ACK for original message %s from %s", originalMessageID, msg.SenderID)

	nm.pendingAcksMu.RLock()
	ackCh, exists := nm.pendingAcks[originalMessageID]
	nm.pendingAcksMu.RUnlock()

	if !exists {
		log.Printf("DEBUG [handleAcknowledgment]: No pending ACK found for message %s, might have timed out", originalMessageID)
		// Acknowledgment for unknown message, might have already timed out
		return nil
	}

	log.Printf("DEBUG [handleAcknowledgment]: Found pending ACK for message %s", originalMessageID)

	// Check if acknowledgment indicates success or failure
	var ackErr error
	if ackPayload, ok := msg.Payload.(*messages.OperationAckMessage); ok {
		if !ackPayload.Success {
			ackErr = fmt.Errorf("operation failed: %s", ackPayload.Error)
			log.Printf("DEBUG [handleAcknowledgment]: ACK indicates failure: %s", ackPayload.Error)
		} else {
			log.Printf("DEBUG [handleAcknowledgment]: ACK indicates success")
		}
	} else if chunkAckPayload, ok := msg.Payload.(*messages.ChunkAckMessage); ok {
		if !chunkAckPayload.Success {
			ackErr = fmt.Errorf("chunk operation failed: %s", chunkAckPayload.Error)
			log.Printf("DEBUG [handleAcknowledgment]: Chunk ACK indicates failure: %s", chunkAckPayload.Error)
		} else {
			log.Printf("DEBUG [handleAcknowledgment]: Chunk ACK indicates success")
		}
	}

	// Send acknowledgment result to waiting sender
	select {
	case ackCh <- ackErr:
		log.Printf("DEBUG [handleAcknowledgment]: Successfully sent ACK result to waiting channel")
	default:
		// Channel already closed or full, ignore
		log.Printf("DEBUG [handleAcknowledgment]: Could not send to ACK channel (closed or full)")
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
	// The transport layer (TCP/QUIC) handles the session key exchange during handshake
	if err := nm.transport.ConnectToPeer(peerID, address, port); err != nil {
		return fmt.Errorf("failed to connect transport to peer %s: %w", peerID, err)
	}

	// Connection and session key are already set up by the transport layer
	// Verify that the session key was established
	if _, err := nm.connManager.GetSessionKey(peerID); err != nil {
		return fmt.Errorf("session key not established for peer %s: %w", peerID, err)
	}

	return nil
}

// toByteSlice converts an interface{} to []byte if possible
func toByteSlice(v interface{}) ([]byte, bool) {
	switch b := v.(type) {
	case []byte:
		return b, true
	case []interface{}:
		// JSON unmarshals byte slices as []interface{} with numbers
		bytes := make([]byte, len(b))
		for i, val := range b {
			if num, ok := val.(float64); ok {
				bytes[i] = byte(num)
			} else {
				return nil, false
			}
		}
		return bytes, true
	case string:
		// Base64 encoded byte slices
		if data, err := base64.StdEncoding.DecodeString(b); err == nil {
			fmt.Printf("DEBUG: Successfully decoded base64 string to %d bytes\n", len(data))
			return data, true
		} else {
			fmt.Printf("DEBUG: Failed to decode base64 string: %v\n", err)
			return nil, false
		}
	default:
		return nil, false
	}
}
