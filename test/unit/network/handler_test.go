package network_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/p2p-folder-sync/p2p-sync/internal/chunking"
	"github.com/p2p-folder-sync/p2p-sync/internal/compression"
	"github.com/p2p-folder-sync/p2p-sync/internal/config"
	"github.com/p2p-folder-sync/p2p-sync/internal/database"
	"github.com/p2p-folder-sync/p2p-sync/internal/network"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/messages"
	syncpkg "github.com/p2p-folder-sync/p2p-sync/internal/sync"
)

// Mock SyncEngine for testing
type MockSyncEngine struct {
	getAllFilesFunc       func() ([]*database.FileMetadata, error)
	handleIncomingFileFunc func([]byte, *syncpkg.SyncOperation) error
	handleIncomingRenameFunc func(*syncpkg.SyncOperation) error
}

func (m *MockSyncEngine) GetAllFiles() ([]*database.FileMetadata, error) {
	if m.getAllFilesFunc != nil {
		return m.getAllFilesFunc()
	}
	return []*database.FileMetadata{}, nil
}

func (m *MockSyncEngine) HandleIncomingFile(data []byte, op *syncpkg.SyncOperation) error {
	if m.handleIncomingFileFunc != nil {
		return m.handleIncomingFileFunc(data, op)
	}
	return nil
}

func (m *MockSyncEngine) HandleIncomingRename(op *syncpkg.SyncOperation) error {
	if m.handleIncomingRenameFunc != nil {
		return m.handleIncomingRenameFunc(op)
	}
	return nil
}

// Mock HeartbeatManager for testing
type MockHeartbeatManager struct {
	lastPeerID string
	callCount  int
}

func (m *MockHeartbeatManager) HandleHeartbeatResponse(peerID string) {
	m.lastPeerID = peerID
	m.callCount++
}

func TestNewNetworkMessageHandler(t *testing.T) {
	cfg := createTestConfig()
	mockEngine := &MockSyncEngine{}
	mockHeartbeat := &MockHeartbeatManager{}

	handler := network.NewNetworkMessageHandler(cfg, mockEngine, mockHeartbeat, "test-peer")

	if handler == nil {
		t.Fatal("Expected handler to be created")
	}
}

func TestSetSyncEngine(t *testing.T) {
	cfg := createTestConfig()
	handler := network.NewNetworkMessageHandler(cfg, nil, nil, "test-peer")

	mockEngine := &MockSyncEngine{}
	handler.SetSyncEngine(mockEngine)

	// Verify by trying to handle a message that requires sync engine
	// We can't directly test the field, but we can test behavior
}

func TestSetHeartbeatManager(t *testing.T) {
	cfg := createTestConfig()
	handler := network.NewNetworkMessageHandler(cfg, nil, nil, "test-peer")

	mockHeartbeat := &MockHeartbeatManager{}
	handler.SetHeartbeatManager(mockHeartbeat)

	// Verify by handling a heartbeat message
	msg := &messages.Message{
		ID:       "msg-1",
		Type:     messages.TypeHeartbeat,
		SenderID: "peer-1",
	}

	err := handler.HandleMessage(msg)
	if err != nil {
		t.Errorf("HandleMessage failed: %v", err)
	}

	if mockHeartbeat.callCount != 1 {
		t.Errorf("Expected heartbeat handler to be called once, got %d", mockHeartbeat.callCount)
	}
	if mockHeartbeat.lastPeerID != "peer-1" {
		t.Errorf("Expected peer ID 'peer-1', got '%s'", mockHeartbeat.lastPeerID)
	}
}

func TestHandleMessage_UnknownType(t *testing.T) {
	cfg := createTestConfig()
	handler := network.NewNetworkMessageHandler(cfg, nil, nil, "test-peer")

	msg := &messages.Message{
		ID:       "msg-1",
		Type:     "unknown-type",
		SenderID: "peer-1",
	}

	err := handler.HandleMessage(msg)
	if err == nil {
		t.Error("Expected error for unknown message type")
	}
	if !contains(err.Error(), "unknown message type") {
		t.Errorf("Expected 'unknown message type' error, got: %v", err)
	}
}

func TestHandleMessage_Discovery(t *testing.T) {
	cfg := createTestConfig()
	handler := network.NewNetworkMessageHandler(cfg, nil, nil, "test-peer")

	msg := &messages.Message{
		ID:       "msg-1",
		Type:     messages.TypeDiscovery,
		SenderID: "peer-1",
		Payload:  messages.DiscoveryMessage{},
	}

	err := handler.HandleMessage(msg)
	if err != nil {
		t.Errorf("Discovery handler failed: %v", err)
	}
}

func TestHandleMessage_DiscoveryResponse(t *testing.T) {
	cfg := createTestConfig()
	handler := network.NewNetworkMessageHandler(cfg, nil, nil, "test-peer")

	msg := &messages.Message{
		ID:       "msg-1",
		Type:     messages.TypeDiscoveryResponse,
		SenderID: "peer-1",
		Payload:  messages.DiscoveryResponseMessage{},
	}

	err := handler.HandleMessage(msg)
	if err != nil {
		t.Errorf("Discovery response handler failed: %v", err)
	}
}

func TestHandleMessage_Handshake(t *testing.T) {
	cfg := createTestConfig()
	handler := network.NewNetworkMessageHandler(cfg, nil, nil, "test-peer")

	msg := &messages.Message{
		ID:       "msg-1",
		Type:     messages.TypeHandshake,
		SenderID: "peer-1",
	}

	err := handler.HandleMessage(msg)
	if err == nil {
		t.Error("Expected error for unimplemented handshake")
	}
	if !contains(err.Error(), "not implemented") {
		t.Errorf("Expected 'not implemented' error, got: %v", err)
	}
}

func TestHandleMessage_HandshakeAck(t *testing.T) {
	cfg := createTestConfig()
	handler := network.NewNetworkMessageHandler(cfg, nil, nil, "test-peer")

	msg := &messages.Message{
		ID:       "msg-1",
		Type:     messages.TypeHandshakeAck,
		SenderID: "peer-1",
	}

	err := handler.HandleMessage(msg)
	if err == nil {
		t.Error("Expected error for unimplemented handshake ack")
	}
}

func TestHandleMessage_HandshakeComplete(t *testing.T) {
	cfg := createTestConfig()
	handler := network.NewNetworkMessageHandler(cfg, nil, nil, "test-peer")

	msg := &messages.Message{
		ID:       "msg-1",
		Type:     messages.TypeHandshakeComplete,
		SenderID: "peer-1",
	}

	err := handler.HandleMessage(msg)
	if err == nil {
		t.Error("Expected error for unimplemented handshake complete")
	}
}

func TestHandleMessage_FileRequest(t *testing.T) {
	cfg := createTestConfig()
	handler := network.NewNetworkMessageHandler(cfg, nil, nil, "test-peer")

	msg := &messages.Message{
		ID:       "msg-1",
		Type:     messages.TypeFileRequest,
		SenderID: "peer-1",
		Payload:  messages.FileRequestMessage{},
	}

	err := handler.HandleMessage(msg)
	if err != nil {
		t.Errorf("File request handler failed: %v", err)
	}
}

func TestHandleMessage_SyncOperation_Create(t *testing.T) {
	cfg := createTestConfig()
	mockEngine := &MockSyncEngine{
		handleIncomingFileFunc: func(data []byte, op *syncpkg.SyncOperation) error {
			if op.Type != syncpkg.OpCreate {
				t.Errorf("Expected OpCreate, got %v", op.Type)
			}
			if op.Source != "remote" {
				t.Errorf("Expected source 'remote', got '%s'", op.Source)
			}
			if string(data) != "test file content" {
				t.Errorf("Expected 'test file content', got '%s'", string(data))
			}
			return nil
		},
	}
	handler := network.NewNetworkMessageHandler(cfg, mockEngine, nil, "test-peer")

	fileID := "file-1"
	checksum := "abc123"
	size := int64(17)
	mtime := time.Now().Unix()

	payload := &messages.LogEntryPayload{
		OperationID: "op-1",
		Type:        string(syncpkg.OpCreate),
		Path:        "test.txt",
		FileID:      &fileID,
		Checksum:    &checksum,
		Size:        &size,
		Mtime:       &mtime,
		PeerID:      "peer-1",
		Data:        []byte("test file content"),
	}

	msg := &messages.Message{
		ID:       "msg-1",
		Type:     messages.TypeSyncOperation,
		SenderID: "peer-1",
		Payload:  payload,
	}

	err := handler.HandleMessage(msg)
	if err != nil {
		t.Errorf("Sync operation handler failed: %v", err)
	}
}

func TestHandleMessage_SyncOperation_Delete(t *testing.T) {
	cfg := createTestConfig()
	mockEngine := &MockSyncEngine{
		handleIncomingFileFunc: func(data []byte, op *syncpkg.SyncOperation) error {
			if op.Type != syncpkg.OpDelete {
				t.Errorf("Expected OpDelete, got %v", op.Type)
			}
			if len(data) != 0 {
				t.Error("Expected empty data for delete operation")
			}
			return nil
		},
	}
	handler := network.NewNetworkMessageHandler(cfg, mockEngine, nil, "test-peer")

	fileID := "file-1"
	checksum := "abc123"
	size := int64(0)
	mtime := time.Now().Unix()

	payload := &messages.LogEntryPayload{
		OperationID: "op-1",
		Type:        string(syncpkg.OpDelete),
		Path:        "test.txt",
		FileID:      &fileID,
		Checksum:    &checksum,
		Size:        &size,
		Mtime:       &mtime,
		PeerID:      "peer-1",
		Data:        []byte{},
	}

	msg := &messages.Message{
		ID:       "msg-1",
		Type:     messages.TypeSyncOperation,
		SenderID: "peer-1",
		Payload:  payload,
	}

	err := handler.HandleMessage(msg)
	if err != nil {
		t.Errorf("Delete operation handler failed: %v", err)
	}
}

func TestHandleMessage_SyncOperation_Rename(t *testing.T) {
	cfg := createTestConfig()
	mockEngine := &MockSyncEngine{
		handleIncomingRenameFunc: func(op *syncpkg.SyncOperation) error {
			if op.Type != syncpkg.OpRename {
				t.Errorf("Expected OpRename, got %v", op.Type)
			}
			return nil
		},
	}
	handler := network.NewNetworkMessageHandler(cfg, mockEngine, nil, "test-peer")

	fileID := "file-1"
	checksum := "abc123"
	size := int64(100)
	mtime := time.Now().Unix()
	fromPath := "old.txt"

	payload := &messages.LogEntryPayload{
		OperationID: "op-1",
		Type:        string(syncpkg.OpRename),
		Path:        "new.txt",
		FromPath:    &fromPath,
		FileID:      &fileID,
		Checksum:    &checksum,
		Size:        &size,
		Mtime:       &mtime,
		PeerID:      "peer-1",
		Data:        []byte{},
	}

	msg := &messages.Message{
		ID:       "msg-1",
		Type:     messages.TypeSyncOperation,
		SenderID: "peer-1",
		Payload:  payload,
	}

	err := handler.HandleMessage(msg)
	if err != nil {
		t.Errorf("Rename operation handler failed: %v", err)
	}
}

func TestHandleMessage_SyncOperation_WithCompression(t *testing.T) {
	cfg := createTestConfig()

	// Create compressed data
	compressor, err := compression.NewCompressor("zstd", 3)
	if err != nil {
		t.Fatalf("Failed to create compressor: %v", err)
	}

	originalData := []byte("test file content that will be compressed")
	compressedData, err := compressor.Compress(originalData)
	if err != nil {
		t.Fatalf("Failed to compress data: %v", err)
	}

	mockEngine := &MockSyncEngine{
		handleIncomingFileFunc: func(data []byte, op *syncpkg.SyncOperation) error {
			// Verify decompressed data matches original
			if string(data) != string(originalData) {
				t.Errorf("Expected decompressed data '%s', got '%s'", originalData, data)
			}
			return nil
		},
	}
	handler := network.NewNetworkMessageHandler(cfg, mockEngine, nil, "test-peer")

	fileID := "file-1"
	checksum := "abc123"
	size := int64(len(compressedData))
	mtime := time.Now().Unix()
	compressed := true
	originalSize := int64(len(originalData))
	algorithm := "zstd"

	payload := &messages.LogEntryPayload{
		OperationID:          "op-1",
		Type:                 string(syncpkg.OpCreate),
		Path:                 "test.txt",
		FileID:               &fileID,
		Checksum:             &checksum,
		Size:                 &size,
		Mtime:                &mtime,
		PeerID:               "peer-1",
		Data:                 compressedData,
		Compressed:           &compressed,
		OriginalSize:         &originalSize,
		CompressionAlgorithm: &algorithm,
	}

	msg := &messages.Message{
		ID:       "msg-1",
		Type:     messages.TypeSyncOperation,
		SenderID: "peer-1",
		Payload:  payload,
	}

	err = handler.HandleMessage(msg)
	if err != nil {
		t.Errorf("Compressed sync operation handler failed: %v", err)
	}
}

func TestHandleMessage_SyncOperation_NoSyncEngine(t *testing.T) {
	cfg := createTestConfig()
	handler := network.NewNetworkMessageHandler(cfg, nil, nil, "test-peer")

	fileID := "file-1"
	checksum := "abc123"
	size := int64(10)
	mtime := time.Now().Unix()

	payload := &messages.LogEntryPayload{
		OperationID: "op-1",
		Type:        string(syncpkg.OpCreate),
		Path:        "test.txt",
		FileID:      &fileID,
		Checksum:    &checksum,
		Size:        &size,
		Mtime:       &mtime,
		PeerID:      "peer-1",
		Data:        []byte("test"),
	}

	msg := &messages.Message{
		ID:       "msg-1",
		Type:     messages.TypeSyncOperation,
		SenderID: "peer-1",
		Payload:  payload,
	}

	err := handler.HandleMessage(msg)
	// Should not error but will log that sync engine is not available
	if err != nil {
		t.Errorf("Expected no error when sync engine unavailable, got: %v", err)
	}
}

func TestHandleMessage_Chunk(t *testing.T) {
	cfg := createTestConfig()
	mockEngine := &MockSyncEngine{}
	handler := network.NewNetworkMessageHandler(cfg, mockEngine, nil, "test-peer")

	// Create chunk data
	chunkData := []byte("chunk data for testing")
	chunker := chunking.NewChunker(512 * 1024)
	chunk, err := chunker.CreateChunk("file-1", 0, chunkData, 0, len(chunkData), true, 1)
	if err != nil {
		t.Fatalf("Failed to create chunk: %v", err)
	}

	payload := &messages.ChunkMessage{
		FileID:      chunk.FileID,
		FileHash:    "abc123",
		ChunkID:     chunk.ChunkID,
		TotalChunks: 1,
		Offset:      chunk.Offset,
		Length:      chunk.Length,
		ChunkHash:   chunk.Hash,
		Data:        chunk.Data,
		IsLast:      true,
	}

	msg := &messages.Message{
		ID:       "msg-1",
		Type:     messages.TypeChunk,
		SenderID: "peer-1",
		Payload:  payload,
	}

	err = handler.HandleMessage(msg)
	if err != nil {
		t.Errorf("Chunk handler failed: %v", err)
	}
}

func TestHandleMessage_Chunk_MultipleChunks(t *testing.T) {
	cfg := createTestConfig()
	mockEngine := &MockSyncEngine{
		handleIncomingFileFunc: func(data []byte, op *syncpkg.SyncOperation) error {
			// Will be called when all chunks are assembled
			return nil
		},
	}
	handler := network.NewNetworkMessageHandler(cfg, mockEngine, nil, "test-peer")

	// Create 3 chunks
	fileData := make([]byte, 1500) // 1.5KB
	for i := range fileData {
		fileData[i] = byte(i % 256)
	}

	chunker := chunking.NewChunker(512) // 512 bytes per chunk
	chunks, err := chunker.ChunkFile("file-1", fileData)
	if err != nil {
		t.Fatalf("Failed to chunk file: %v", err)
	}

	// Send all chunks
	for _, chunk := range chunks {
		payload := &messages.ChunkMessage{
			FileID:      chunk.FileID,
			FileHash:    "filehash123",
			ChunkID:     chunk.ChunkID,
			TotalChunks: chunk.TotalChunks,
			Offset:      chunk.Offset,
			Length:      chunk.Length,
			ChunkHash:   chunk.Hash,
			Data:        chunk.Data,
			IsLast:      chunk.IsLast,
		}

		msg := &messages.Message{
			ID:       fmt.Sprintf("msg-%d", chunk.ChunkID),
			Type:     messages.TypeChunk,
			SenderID: "peer-1",
			Payload:  payload,
		}

		err := handler.HandleMessage(msg)
		if err != nil {
			t.Errorf("Chunk handler failed for chunk %d: %v", chunk.ChunkID, err)
		}
	}
}

func TestHandleMessage_ChunkRequest(t *testing.T) {
	cfg := createTestConfig()
	handler := network.NewNetworkMessageHandler(cfg, nil, nil, "test-peer")

	msg := &messages.Message{
		ID:       "msg-1",
		Type:     messages.TypeChunkRequest,
		SenderID: "peer-1",
		Payload:  messages.ChunkRequestMessage{},
	}

	err := handler.HandleMessage(msg)
	if err != nil {
		t.Errorf("Chunk request handler failed: %v", err)
	}
}

func TestHandleMessage_OperationAck(t *testing.T) {
	cfg := createTestConfig()
	handler := network.NewNetworkMessageHandler(cfg, nil, nil, "test-peer")

	msg := &messages.Message{
		ID:       "msg-1",
		Type:     messages.TypeOperationAck,
		SenderID: "peer-1",
		Payload:  &messages.OperationAckMessage{Success: true},
	}

	err := handler.HandleMessage(msg)
	if err != nil {
		t.Errorf("Operation ack handler failed: %v", err)
	}
}

func TestHandleMessage_ChunkAck(t *testing.T) {
	cfg := createTestConfig()
	handler := network.NewNetworkMessageHandler(cfg, nil, nil, "test-peer")

	msg := &messages.Message{
		ID:       "msg-1",
		Type:     messages.TypeChunkAck,
		SenderID: "peer-1",
		Payload:  &messages.ChunkAckMessage{Success: true},
	}

	err := handler.HandleMessage(msg)
	if err != nil {
		t.Errorf("Chunk ack handler failed: %v", err)
	}
}

func TestHandleMessage_Heartbeat(t *testing.T) {
	cfg := createTestConfig()
	mockHeartbeat := &MockHeartbeatManager{}
	handler := network.NewNetworkMessageHandler(cfg, nil, mockHeartbeat, "test-peer")

	msg := &messages.Message{
		ID:       "msg-1",
		Type:     messages.TypeHeartbeat,
		SenderID: "peer-1",
	}

	err := handler.HandleMessage(msg)
	if err != nil {
		t.Errorf("Heartbeat handler failed: %v", err)
	}

	if mockHeartbeat.callCount != 1 {
		t.Errorf("Expected 1 heartbeat call, got %d", mockHeartbeat.callCount)
	}
	if mockHeartbeat.lastPeerID != "peer-1" {
		t.Errorf("Expected peer ID 'peer-1', got '%s'", mockHeartbeat.lastPeerID)
	}
}

func TestHandleMessage_Heartbeat_NoManager(t *testing.T) {
	cfg := createTestConfig()
	handler := network.NewNetworkMessageHandler(cfg, nil, nil, "test-peer")

	msg := &messages.Message{
		ID:       "msg-1",
		Type:     messages.TypeHeartbeat,
		SenderID: "peer-1",
	}

	// Should not error even if heartbeat manager is nil
	err := handler.HandleMessage(msg)
	if err != nil {
		t.Errorf("Heartbeat handler should not fail when manager is nil: %v", err)
	}
}

func TestHandleMessage_InvalidPayload(t *testing.T) {
	cfg := createTestConfig()
	mockEngine := &MockSyncEngine{}
	handler := network.NewNetworkMessageHandler(cfg, mockEngine, nil, "test-peer")

	// Send sync operation with wrong payload type
	msg := &messages.Message{
		ID:       "msg-1",
		Type:     messages.TypeSyncOperation,
		SenderID: "peer-1",
		Payload:  "invalid payload type",
	}

	err := handler.HandleMessage(msg)
	if err == nil {
		t.Error("Expected error for invalid payload type")
	}
}

func TestHandleMessage_Chunk_InvalidPayload(t *testing.T) {
	cfg := createTestConfig()
	handler := network.NewNetworkMessageHandler(cfg, nil, nil, "test-peer")

	msg := &messages.Message{
		ID:       "msg-1",
		Type:     messages.TypeChunk,
		SenderID: "peer-1",
		Payload:  "invalid chunk payload",
	}

	err := handler.HandleMessage(msg)
	if err == nil {
		t.Error("Expected error for invalid chunk payload")
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
