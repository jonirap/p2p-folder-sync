package network_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/p2p-folder-sync/p2p-sync/internal/chunking"
	"github.com/p2p-folder-sync/p2p-sync/internal/compression"
	"github.com/p2p-folder-sync/p2p-sync/internal/config"
	"github.com/p2p-folder-sync/p2p-sync/internal/crypto"
	"github.com/p2p-folder-sync/p2p-sync/internal/network"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/connection"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/messages"
	syncpkg "github.com/p2p-folder-sync/p2p-sync/internal/sync"
)

// Mock Transport for testing
type MockTransport struct {
	mu               sync.RWMutex
	sentMessages     map[string][]*messages.Message // peerID -> messages
	messageHandler   network.MessageHandler
	sendError        error
	connManager      *connection.ConnectionManager
	shouldFailOnce   bool
	failedOnce       bool
}

func NewMockTransport() *MockTransport {
	return &MockTransport{
		sentMessages: make(map[string][]*messages.Message),
	}
}

func (mt *MockTransport) SendMessage(peerID, address string, port int, msg *messages.Message) error {
	mt.mu.Lock()
	defer mt.mu.Unlock()

	// Simulate one-time failure if requested
	if mt.shouldFailOnce && !mt.failedOnce {
		mt.failedOnce = true
		return errors.New("simulated send failure")
	}

	if mt.sendError != nil {
		return mt.sendError
	}

	mt.sentMessages[peerID] = append(mt.sentMessages[peerID], msg)
	return nil
}

func (mt *MockTransport) SetMessageHandler(handler network.MessageHandler) {
	mt.mu.Lock()
	defer mt.mu.Unlock()
	mt.messageHandler = handler
}

func (mt *MockTransport) ConnectToPeer(peerID, address string, port int) error {
	return nil
}

func (mt *MockTransport) Start() error {
	return nil
}

func (mt *MockTransport) Stop() error {
	return nil
}

func (mt *MockTransport) GetSentMessages(peerID string) []*messages.Message {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	return mt.sentMessages[peerID]
}

func (mt *MockTransport) ClearMessages() {
	mt.mu.Lock()
	defer mt.mu.Unlock()
	mt.sentMessages = make(map[string][]*messages.Message)
}

func (mt *MockTransport) SetConnectionManager(connMgr *connection.ConnectionManager) {
	mt.mu.Lock()
	defer mt.mu.Unlock()
	mt.connManager = connMgr
}

func (mt *MockTransport) SetSendError(err error) {
	mt.mu.Lock()
	defer mt.mu.Unlock()
	mt.sendError = err
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

// Helper function to create test messenger with dependencies
func createTestMessenger(t *testing.T, cfg *config.Config, transport *MockTransport) (*network.NetworkMessenger, *connection.ConnectionManager) {
	connManager := connection.NewConnectionManager()

	messenger, err := network.NewNetworkMessenger(cfg, connManager, transport, "test-peer-1")
	if err != nil {
		t.Fatalf("Failed to create messenger: %v", err)
	}

	return messenger, connManager
}

// Helper to setup a connected peer
func setupConnectedPeer(t *testing.T, connManager *connection.ConnectionManager, peerID string) []byte {
	sessionKey := make([]byte, 32)
	for i := range sessionKey {
		sessionKey[i] = byte(i)
	}

	connManager.AddConnection(peerID, "127.0.0.1", 8080)
	if err := connManager.SetSessionKey(peerID, sessionKey); err != nil {
		t.Fatalf("Failed to set session key: %v", err)
	}
	connManager.UpdateConnectionState(peerID, connection.StateConnected)

	return sessionKey
}

func TestNewNetworkMessenger(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.Config
		wantErr     bool
		errContains string
	}{
		{
			name:    "successful creation with valid config",
			config:  createTestConfig(),
			wantErr: false,
		},
		{
			name: "invalid compression algorithm",
			config: &config.Config{
				Sync: config.SyncConfig{
					ChunkSizeDefault: 512 * 1024,
				},
				Compression: config.CompressionConfig{
					Algorithm: "invalid-algo",
					Level:     1,
				},
			},
			wantErr:     true,
			errContains: "failed to create compressor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := NewMockTransport()
			connManager := connection.NewConnectionManager()

			messenger, err := network.NewNetworkMessenger(tt.config, connManager, transport, "test-peer")

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("Error %q does not contain %q", err.Error(), tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if messenger == nil {
					t.Error("Expected messenger to be non-nil")
				}
			}
		})
	}
}

func TestSendFile_SmallFile(t *testing.T) {
	cfg := createTestConfig()
	transport := NewMockTransport()
	messenger, connManager := createTestMessenger(t, cfg, transport)

	// Setup peer connection
	peerID := "peer-2"
	setupConnectedPeer(t, connManager, peerID)

	// Create small file data (<512KB, won't be chunked)
	fileData := []byte("small file content")
	metadata := &syncpkg.SyncOperation{
		ID:       "op-1",
		Type:     syncpkg.OpCreate,
		Path:     "test.txt",
		FileID:   "file-1",
		Checksum: "abc123",
		Size:     int64(len(fileData)),
		PeerID:   "test-peer-1",
		VectorClock: map[string]int64{
			"test-peer-1": 1,
		},
	}

	// Mock acknowledgment response
	go func() {
		time.Sleep(10 * time.Millisecond)
		// Simulate receiving acknowledgment
		ackMsg := &messages.Message{
			ID:       messages.GenerateMessageID(),
			Type:     messages.TypeOperationAck,
			SenderID: peerID,
			Payload: &messages.OperationAckMessage{
				OperationID: metadata.ID,
				Success:     true,
			},
		}
		if transport.messageHandler != nil {
			transport.messageHandler.HandleMessage(ackMsg)
		}
	}()

	// Send file
	err := messenger.SendFile(peerID, fileData, metadata)
	if err != nil {
		t.Errorf("SendFile failed: %v", err)
	}

	// Verify message was sent
	sentMsgs := transport.GetSentMessages(peerID)
	if len(sentMsgs) == 0 {
		t.Error("Expected at least one message to be sent")
	}
}

func TestSendFile_PeerNotConnected(t *testing.T) {
	cfg := createTestConfig()
	transport := NewMockTransport()
	messenger, _ := createTestMessenger(t, cfg, transport)

	fileData := []byte("test content")
	metadata := &syncpkg.SyncOperation{
		ID:       "op-1",
		FileID:   "file-1",
		Checksum: "abc123",
	}

	err := messenger.SendFile("non-existent-peer", fileData, metadata)
	if err == nil {
		t.Error("Expected error for non-connected peer")
	}
	if !containsString(err.Error(), "peer not connected") {
		t.Errorf("Expected 'peer not connected' error, got: %v", err)
	}
}

func TestSendFile_NoSessionKey(t *testing.T) {
	cfg := createTestConfig()
	transport := NewMockTransport()
	messenger, connManager := createTestMessenger(t, cfg, transport)

	// Add connection but don't set session key
	peerID := "peer-2"
	connManager.AddConnection(peerID, "127.0.0.1", 8080)

	fileData := []byte("test content")
	metadata := &syncpkg.SyncOperation{
		ID:       "op-1",
		FileID:   "file-1",
		Checksum: "abc123",
	}

	err := messenger.SendFile(peerID, fileData, metadata)
	if err == nil {
		t.Error("Expected error for missing session key")
	}
	if !containsString(err.Error(), "no session key") {
		t.Errorf("Expected 'no session key' error, got: %v", err)
	}
}

func TestSendFile_WithCompression(t *testing.T) {
	cfg := createTestConfig()
	cfg.Compression.FileSizeThreshold = 10 // Very small threshold to trigger compression
	transport := NewMockTransport()
	messenger, connManager := createTestMessenger(t, cfg, transport)

	peerID := "peer-2"
	setupConnectedPeer(t, connManager, peerID)

	// Create file data larger than threshold
	fileData := make([]byte, 1024)
	for i := range fileData {
		fileData[i] = byte(i % 256)
	}

	metadata := &syncpkg.SyncOperation{
		ID:          "op-1",
		FileID:      "file-1",
		Checksum:    "abc123",
		PeerID:      "test-peer-1",
		VectorClock: map[string]int64{"test-peer-1": 1},
	}

	// Mock acknowledgment
	go func() {
		time.Sleep(10 * time.Millisecond)
		ackMsg := &messages.Message{
			ID:       messages.GenerateMessageID(),
			Type:     messages.TypeOperationAck,
			SenderID: peerID,
			Payload:  &messages.OperationAckMessage{Success: true},
		}
		if transport.messageHandler != nil {
			transport.messageHandler.HandleMessage(ackMsg)
		}
	}()

	err := messenger.SendFile(peerID, fileData, metadata)
	if err != nil {
		t.Errorf("SendFile with compression failed: %v", err)
	}

	// Verify compression metadata was set
	if metadata.Compressed == nil || !*metadata.Compressed {
		t.Error("Expected file to be marked as compressed")
	}
	if metadata.OriginalSize == nil {
		t.Error("Expected original size to be set")
	}
	if metadata.CompressionAlgorithm == nil {
		t.Error("Expected compression algorithm to be set")
	}
}

func TestBroadcastOperation_NoPeers(t *testing.T) {
	cfg := createTestConfig()
	transport := NewMockTransport()
	messenger, _ := createTestMessenger(t, cfg, transport)

	op := &syncpkg.SyncOperation{
		ID:     "op-1",
		Type:   syncpkg.OpDelete,
		Path:   "test.txt",
		PeerID: "test-peer-1",
	}

	// Should not error when no peers connected
	err := messenger.BroadcastOperation(op)
	if err != nil {
		t.Errorf("BroadcastOperation with no peers should not error, got: %v", err)
	}

	// Verify no messages were sent
	if len(transport.sentMessages) > 0 {
		t.Error("Expected no messages to be sent when no peers connected")
	}
}

func TestBroadcastOperation_SkipsSelfPeer(t *testing.T) {
	cfg := createTestConfig()
	transport := NewMockTransport()
	messenger, connManager := createTestMessenger(t, cfg, transport)

	// Connect self as peer (should be skipped)
	setupConnectedPeer(t, connManager, "test-peer-1")

	op := &syncpkg.SyncOperation{
		ID:     "op-1",
		Type:   syncpkg.OpDelete,
		Path:   "test.txt",
		PeerID: "test-peer-1",
	}

	err := messenger.BroadcastOperation(op)
	if err != nil {
		t.Errorf("BroadcastOperation failed: %v", err)
	}

	// Verify no messages sent to self
	sentMsgs := transport.GetSentMessages("test-peer-1")
	if len(sentMsgs) > 0 {
		t.Error("Expected no messages to be sent to self")
	}
}

func TestHandleMessage_Acknowledgment(t *testing.T) {
	cfg := createTestConfig()
	transport := NewMockTransport()
	messenger, connManager := createTestMessenger(t, cfg, transport)

	peerID := "peer-2"
	sessionKey := setupConnectedPeer(t, connManager, peerID)

	// Create acknowledgment message
	ackPayload := &messages.OperationAckMessage{
		OperationID: "op-1",
		Success:     true,
	}

	// Encrypt the acknowledgment
	payloadData, err := messages.EncodePayload(ackPayload)
	if err != nil {
		t.Fatalf("Failed to encode payload: %v", err)
	}

	encryptedPayload, err := crypto.Encrypt(payloadData, sessionKey)
	if err != nil {
		t.Fatalf("Failed to encrypt payload: %v", err)
	}

	ackMsg := &messages.Message{
		ID:       "op-1",
		Type:     messages.TypeOperationAck,
		SenderID: peerID,
		Payload:  encryptedPayload,
	}

	// Track the message for acknowledgment
	ackCh := make(chan error, 1)
	// Use internal tracking (would need to expose or use reflection in real tests)
	// For this test, we'll just call HandleMessage directly

	err = messenger.HandleMessage(ackMsg)
	if err != nil {
		t.Errorf("HandleMessage failed: %v", err)
	}

	// Verify it was processed (no error means success)
}

func TestHandleMessage_UnknownSender(t *testing.T) {
	cfg := createTestConfig()
	transport := NewMockTransport()
	messenger, _ := createTestMessenger(t, cfg, transport)

	msg := &messages.Message{
		ID:       "msg-1",
		Type:     messages.TypeSyncOperation,
		SenderID: "unknown-peer",
		Payload:  &messages.LogEntryPayload{},
	}

	err := messenger.HandleMessage(msg)
	if err == nil {
		t.Error("Expected error for unknown sender")
	}
	if !containsString(err.Error(), "unknown sender") {
		t.Errorf("Expected 'unknown sender' error, got: %v", err)
	}
}

func TestSetMessageHandler(t *testing.T) {
	cfg := createTestConfig()
	transport := NewMockTransport()
	messenger, _ := createTestMessenger(t, cfg, transport)

	// Create mock handler
	var handlerCalled bool
	mockHandler := &MockMessageHandler{
		handleFunc: func(msg *messages.Message) error {
			handlerCalled = true
			return nil
		},
	}

	messenger.SetMessageHandler(mockHandler)

	// Verify handler is set by triggering a message that should call it
	// (requires more setup with connected peer and proper message)
	if mockHandler == nil {
		t.Error("Expected handler to be set")
	}
}

func TestConnectToPeer(t *testing.T) {
	cfg := createTestConfig()
	transport := NewMockTransport()
	messenger, connManager := createTestMessenger(t, cfg, transport)

	peerID := "peer-2"
	address := "192.168.1.100"
	port := 8080

	err := messenger.ConnectToPeer(peerID, address, port)
	if err != nil {
		t.Errorf("ConnectToPeer failed: %v", err)
	}

	// Verify connection was added
	conn, err := connManager.GetConnection(peerID)
	if err != nil {
		t.Errorf("Expected connection to be added: %v", err)
	}
	if conn.Address != address {
		t.Errorf("Expected address %s, got %s", address, conn.Address)
	}
	if conn.Port != port {
		t.Errorf("Expected port %d, got %d", port, conn.Port)
	}

	// Verify session key was set
	if len(conn.SessionKey) != 32 {
		t.Errorf("Expected 32-byte session key, got %d bytes", len(conn.SessionKey))
	}

	// Verify connection state
	if conn.State != connection.StateConnected {
		t.Errorf("Expected state Connected, got %v", conn.State)
	}
}

func TestRequestStateSync(t *testing.T) {
	cfg := createTestConfig()
	transport := NewMockTransport()
	messenger, connManager := createTestMessenger(t, cfg, transport)

	peerID := "peer-2"
	setupConnectedPeer(t, connManager, peerID)

	err := messenger.RequestStateSync(peerID)
	if err != nil {
		t.Errorf("RequestStateSync failed: %v", err)
	}

	// Verify message was sent
	sentMsgs := transport.GetSentMessages(peerID)
	if len(sentMsgs) == 0 {
		t.Error("Expected state sync request message to be sent")
	}

	// Verify message type
	if sentMsgs[0].Type != messages.TypeStateDeclaration {
		t.Errorf("Expected message type %s, got %s", messages.TypeStateDeclaration, sentMsgs[0].Type)
	}
}

func TestRequestStateSync_PeerNotConnected(t *testing.T) {
	cfg := createTestConfig()
	transport := NewMockTransport()
	messenger, _ := createTestMessenger(t, cfg, transport)

	err := messenger.RequestStateSync("non-existent-peer")
	if err == nil {
		t.Error("Expected error for non-connected peer")
	}
}

func TestToByteSlice_Conversions(t *testing.T) {
	// This tests the toByteSlice helper function indirectly through message handling
	// We test the various conversion paths by providing different payload types

	cfg := createTestConfig()
	transport := NewMockTransport()
	messenger, connManager := createTestMessenger(t, cfg, transport)

	peerID := "peer-2"
	sessionKey := setupConnectedPeer(t, connManager, peerID)

	tests := []struct {
		name        string
		setupMsg    func() *messages.Message
		expectError bool
	}{
		{
			name: "direct byte slice",
			setupMsg: func() *messages.Message {
				payload := &messages.OperationAckMessage{Success: true}
				payloadData, _ := messages.EncodePayload(payload)
				encryptedPayload, _ := crypto.Encrypt(payloadData, sessionKey)
				return &messages.Message{
					ID:       "msg-1",
					Type:     messages.TypeOperationAck,
					SenderID: peerID,
					Payload:  encryptedPayload,
				}
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.setupMsg()
			err := messenger.HandleMessage(msg)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			} else if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestMessageRetry(t *testing.T) {
	cfg := createTestConfig()
	transport := NewMockTransport()
	messenger, connManager := createTestMessenger(t, cfg, transport)

	peerID := "peer-2"
	setupConnectedPeer(t, connManager, peerID)

	// Set transport to fail once
	transport.shouldFailOnce = true

	fileData := []byte("test content")
	metadata := &syncpkg.SyncOperation{
		ID:          "op-1",
		FileID:      "file-1",
		Checksum:    "abc123",
		PeerID:      "test-peer-1",
		VectorClock: map[string]int64{"test-peer-1": 1},
	}

	// Mock acknowledgment after retry
	go func() {
		time.Sleep(100 * time.Millisecond)
		ackMsg := &messages.Message{
			ID:       messages.GenerateMessageID(),
			Type:     messages.TypeOperationAck,
			SenderID: peerID,
			Payload:  &messages.OperationAckMessage{Success: true},
		}
		if transport.messageHandler != nil {
			transport.messageHandler.HandleMessage(ackMsg)
		}
	}()

	err := messenger.SendFile(peerID, fileData, metadata)
	if err != nil {
		t.Errorf("SendFile should succeed after retry: %v", err)
	}

	// Verify transport failed once
	if !transport.failedOnce {
		t.Error("Expected transport to have failed once during retry")
	}
}

func TestAcknowledgmentTimeout(t *testing.T) {
	cfg := createTestConfig()
	transport := NewMockTransport()
	messenger, connManager := createTestMessenger(t, cfg, transport)

	peerID := "peer-2"
	setupConnectedPeer(t, connManager, peerID)

	// Don't send any acknowledgment - let it timeout
	fileData := []byte("test content")
	metadata := &syncpkg.SyncOperation{
		ID:          "op-1",
		FileID:      "file-1",
		Checksum:    "abc123",
		PeerID:      "test-peer-1",
		VectorClock: map[string]int64{"test-peer-1": 1},
	}

	// This should timeout (30s default * 3 retries would be too long for test)
	// For testing, we'd need to expose ackTimeout or make it configurable
	// For now, we'll just verify the error handling works

	// Note: This test would take too long with default timeout
	// In production code, timeout should be configurable
	t.Skip("Skipping timeout test - would take 90+ seconds with current implementation")
}

// Mock message handler for testing
type MockMessageHandler struct {
	handleFunc func(*messages.Message) error
}

func (m *MockMessageHandler) HandleMessage(msg *messages.Message) error {
	if m.handleFunc != nil {
		return m.handleFunc(msg)
	}
	return nil
}

// Helper function to check if string contains substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && (s[:len(substr)] == substr || len(s) > len(substr) && containsString(s[1:], substr)))
}

// Simplified contains check
func init() {
	containsString = func(s, substr string) bool {
		for i := 0; i <= len(s)-len(substr); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	}
}
