package network_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/p2p-folder-sync/p2p-sync/internal/chunking"
	"github.com/p2p-folder-sync/p2p-sync/internal/compression"
	"github.com/p2p-folder-sync/p2p-sync/internal/config"
	"github.com/p2p-folder-sync/p2p-sync/internal/database"
	"github.com/p2p-folder-sync/p2p-sync/internal/network"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/messages"
	syncpkg "github.com/p2p-folder-sync/p2p-sync/internal/sync"
)

// Helper to create a real engine for testing
func createTestEngine(t *testing.T, peerID string) (*syncpkg.Engine, *database.DB, string, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "handler-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	cfg := &config.Config{
		Sync: config.SyncConfig{
			FolderPath:             tmpDir,
			ChunkSizeMin:           64 * 1024,
			ChunkSizeMax:           2 * 1024 * 1024,
			ChunkSizeDefault:       512 * 1024,
			MaxConcurrentTransfers: 5,
		},
		Network: config.NetworkConfig{
			Port:          8080,
			DiscoveryPort: 8081,
		},
	}

	dbPath := filepath.Join(tmpDir, ".p2p-sync", "test.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create db dir: %v", err)
	}

	db, err := database.NewDB(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create database: %v", err)
	}

	engine, err := syncpkg.NewEngine(cfg, db, peerID)
	if err != nil {
		db.Close()
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create engine: %v", err)
	}

	cleanup := func() {
		engine.Stop()
		db.Close()
		os.RemoveAll(tmpDir)
	}

	return engine, db, tmpDir, cleanup
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
	engine, _, _, cleanup := createTestEngine(t, "test-peer")
	defer cleanup()

	mockHeartbeat := &MockHeartbeatManager{}

	handler := network.NewNetworkMessageHandler(cfg, engine, mockHeartbeat, "test-peer")

	if handler == nil {
		t.Fatal("Expected handler to be created")
	}
}

func TestSetSyncEngine(t *testing.T) {
	cfg := createTestConfig()
	handler := network.NewNetworkMessageHandler(cfg, nil, nil, "test-peer")

	engine, _, _, cleanup := createTestEngine(t, "test-peer")
	defer cleanup()

	handler.SetSyncEngine(engine)

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
}

// Helper function to create test config
func createTestConfig() *config.Config {
	return &config.Config{
		Sync: config.SyncConfig{
			FolderPath:             "/tmp/test-sync",
			ChunkSizeMin:           64 * 1024,
			ChunkSizeMax:           2 * 1024 * 1024,
			ChunkSizeDefault:       512 * 1024,
			MaxConcurrentTransfers: 5,
		},
		Network: config.NetworkConfig{
			Port:          8080,
			DiscoveryPort: 8081,
		},
		Compression: config.CompressionConfig{
			Enabled:           true,
			FileSizeThreshold: 1024 * 1024, // 1 MB
			Algorithm:         "zstd",
			Level:             3,
		},
	}
}

func TestHandleMessage_Heartbeat(t *testing.T) {
	cfg := createTestConfig()
	engine, _, _, cleanup := createTestEngine(t, "test-peer")
	defer cleanup()

	mockHeartbeat := &MockHeartbeatManager{}
	handler := network.NewNetworkMessageHandler(cfg, engine, mockHeartbeat, "test-peer")

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
		t.Errorf("Expected 1 heartbeat call, got %d", mockHeartbeat.callCount)
	}

	if mockHeartbeat.lastPeerID != "peer-1" {
		t.Errorf("Expected peer ID 'peer-1', got '%s'", mockHeartbeat.lastPeerID)
	}
}

func TestHandleMessage_UnknownType(t *testing.T) {
	cfg := createTestConfig()
	engine, _, _, cleanup := createTestEngine(t, "test-peer")
	defer cleanup()

	handler := network.NewNetworkMessageHandler(cfg, engine, nil, "test-peer")

	msg := &messages.Message{
		ID:       "msg-1",
		Type:     "unknown-type",
		SenderID: "peer-1",
	}

	err := handler.HandleMessage(msg)
	if err == nil {
		t.Error("Expected error for unknown message type")
	}
}

func TestHandleMessage_Chunk(t *testing.T) {
	cfg := createTestConfig()
	engine, _, _, cleanup := createTestEngine(t, "test-peer")
	defer cleanup()

	handler := network.NewNetworkMessageHandler(cfg, engine, nil, "test-peer")

	// Create chunk data
	chunkData := []byte("chunk data for testing")
	chunk := &chunking.Chunk{
		FileID:      "file-1",
		ChunkID:     0,
		Offset:      0,
		Length:      int64(len(chunkData)),
		Data:        chunkData,
		Hash:        "chunk-hash",
		FileHash:    "abc123",
		IsLast:      true,
		TotalChunks: 1,
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

	err := handler.HandleMessage(msg)
	if err != nil {
		t.Errorf("Chunk handler failed: %v", err)
	}
}

func TestHandleMessage_Chunk_MultipleChunks(t *testing.T) {
	cfg := createTestConfig()
	engine, _, _, cleanup := createTestEngine(t, "test-peer")
	defer cleanup()

	handler := network.NewNetworkMessageHandler(cfg, engine, nil, "test-peer")

	// Simulate multiple chunks
	totalChunks := 3
	fileData := []byte("This is test data that will be split into multiple chunks")
	chunkSize := len(fileData) / totalChunks

	chunker := chunking.NewChunker(int64(chunkSize))
	chunks, err := chunker.ChunkFile("file-2", fileData)
	if err != nil {
		t.Fatalf("Failed to chunk file: %v", err)
	}

	// Send all chunks
	for _, chunk := range chunks {
		payload := &messages.ChunkMessage{
			FileID:      chunk.FileID,
			FileHash:    chunk.FileHash,
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
			t.Errorf("Failed to handle chunk %d: %v", chunk.ChunkID, err)
		}
	}
}

func TestCompression(t *testing.T) {
	testCases := []struct {
		name      string
		algorithm string
		data      []byte
	}{
		{
			name:      "zstd",
			algorithm: "zstd",
			data:      []byte("This is test data that should be compressed using zstd"),
		},
		{
			name:      "gzip",
			algorithm: "gzip",
			data:      []byte("This is test data that should be compressed using gzip"),
		},
	}

	for _, tc := range testCases{
		t.Run(tc.name, func(t *testing.T) {
			compressor, err := compression.NewCompressor(tc.algorithm, 3)
			if err != nil {
				t.Fatalf("Failed to create compressor: %v", err)
			}

			compressed, err := compressor.Compress(tc.data)
			if err != nil {
				t.Fatalf("Compression failed: %v", err)
			}

			if len(compressed) >= len(tc.data) {
				t.Logf("Warning: Compressed data (%d bytes) is not smaller than original (%d bytes)",
					len(compressed), len(tc.data))
			}

			decompressed, err := compressor.Decompress(compressed)
			if err != nil {
				t.Fatalf("Decompression failed: %v", err)
			}

			if string(decompressed) != string(tc.data) {
				t.Errorf("Decompressed data doesn't match original.\nExpected: %s\nGot: %s",
					string(tc.data), string(decompressed))
			}
		})
	}
}
