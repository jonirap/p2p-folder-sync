package system

import (
	"sync"
	"time"

	"github.com/p2p-folder-sync/p2p-sync/internal/chunking"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/messages"
	syncpkg "github.com/p2p-folder-sync/p2p-sync/internal/sync"
)

// MessageInterceptor intercepts and records network messages for verification
type MessageInterceptor struct {
	mu           sync.RWMutex
	sentMessages []*messages.Message
	receivedMessages []*messages.Message
}

// NewMessageInterceptor creates a new message interceptor
func NewMessageInterceptor() *MessageInterceptor {
	return &MessageInterceptor{
		sentMessages:     make([]*messages.Message, 0),
		receivedMessages: make([]*messages.Message, 0),
	}
}

// InterceptSend intercepts a message being sent
func (mi *MessageInterceptor) InterceptSend(peerID string, address string, port int, msg *messages.Message) error {
	mi.mu.Lock()
	defer mi.mu.Unlock()
	mi.sentMessages = append(mi.sentMessages, msg)
	return nil // Allow the message to continue
}

// InterceptReceive intercepts a message being received
func (mi *MessageInterceptor) InterceptReceive(msg *messages.Message) {
	mi.mu.Lock()
	defer mi.mu.Unlock()
	mi.receivedMessages = append(mi.receivedMessages, msg)
}

// GetSentMessages returns all intercepted sent messages
func (mi *MessageInterceptor) GetSentMessages() []*messages.Message {
	mi.mu.RLock()
	defer mi.mu.RUnlock()
	result := make([]*messages.Message, len(mi.sentMessages))
	copy(result, mi.sentMessages)
	return result
}

// GetReceivedMessages returns all intercepted received messages
func (mi *MessageInterceptor) GetReceivedMessages() []*messages.Message {
	mi.mu.RLock()
	defer mi.mu.RUnlock()
	result := make([]*messages.Message, len(mi.receivedMessages))
	copy(result, mi.receivedMessages)
	return result
}

// GetSentMessagesByType returns sent messages of a specific type
func (mi *MessageInterceptor) GetSentMessagesByType(msgType string) []*messages.Message {
	mi.mu.RLock()
	defer mi.mu.RUnlock()

	result := make([]*messages.Message, 0)
	for _, msg := range mi.sentMessages {
		if msg.Type == msgType {
			result = append(result, msg)
		}
	}
	return result
}

// GetReceivedMessagesByType returns received messages of a specific type
func (mi *MessageInterceptor) GetReceivedMessagesByType(msgType string) []*messages.Message {
	mi.mu.RLock()
	defer mi.mu.RUnlock()

	result := make([]*messages.Message, 0)
	for _, msg := range mi.receivedMessages {
		if msg.Type == msgType {
			result = append(result, msg)
		}
	}
	return result
}

// Clear clears all intercepted messages
func (mi *MessageInterceptor) Clear() {
	mi.mu.Lock()
	defer mi.mu.Unlock()
	mi.sentMessages = mi.sentMessages[:0]
	mi.receivedMessages = mi.receivedMessages[:0]
}

// OperationQueueMonitor monitors sync engine operation queues
type OperationQueueMonitor struct {
	mu        sync.RWMutex
	queuedOps []*syncpkg.SyncOperation
	sentOps   []*syncpkg.SyncOperation
}

// NewOperationQueueMonitor creates a new operation queue monitor
func NewOperationQueueMonitor() *OperationQueueMonitor {
	return &OperationQueueMonitor{
		queuedOps: make([]*syncpkg.SyncOperation, 0),
		sentOps:   make([]*syncpkg.SyncOperation, 0),
	}
}

// OnOperationQueued is called when an operation is queued
func (oqm *OperationQueueMonitor) OnOperationQueued(op *syncpkg.SyncOperation) {
	oqm.mu.Lock()
	defer oqm.mu.Unlock()
	oqm.queuedOps = append(oqm.queuedOps, op)
}

// OnOperationSent is called when an operation is sent
func (oqm *OperationQueueMonitor) OnOperationSent(op *syncpkg.SyncOperation) {
	oqm.mu.Lock()
	defer oqm.mu.Unlock()
	oqm.sentOps = append(oqm.sentOps, op)
}

// GetQueuedOperations returns all queued operations
func (oqm *OperationQueueMonitor) GetQueuedOperations() []*syncpkg.SyncOperation {
	oqm.mu.RLock()
	defer oqm.mu.RUnlock()
	result := make([]*syncpkg.SyncOperation, len(oqm.queuedOps))
	copy(result, oqm.queuedOps)
	return result
}

// GetSentOperations returns all sent operations
func (oqm *OperationQueueMonitor) GetSentOperations() []*syncpkg.SyncOperation {
	oqm.mu.RLock()
	defer oqm.mu.RUnlock()
	result := make([]*syncpkg.SyncOperation, len(oqm.sentOps))
	copy(result, oqm.sentOps)
	return result
}

// GetQueuedOperationsByType returns queued operations of a specific type
func (oqm *OperationQueueMonitor) GetQueuedOperationsByType(opType syncpkg.OperationType) []*syncpkg.SyncOperation {
	oqm.mu.RLock()
	defer oqm.mu.RUnlock()

	result := make([]*syncpkg.SyncOperation, 0)
	for _, op := range oqm.queuedOps {
		if op.Type == opType {
			result = append(result, op)
		}
	}
	return result
}

// GetSentOperationsByType returns sent operations of a specific type
func (oqm *OperationQueueMonitor) GetSentOperationsByType(opType syncpkg.OperationType) []*syncpkg.SyncOperation {
	oqm.mu.RLock()
	defer oqm.mu.RUnlock()

	result := make([]*syncpkg.SyncOperation, 0)
	for _, op := range oqm.sentOps {
		if op.Type == opType {
			result = append(result, op)
		}
	}
	return result
}

// Clear clears all monitored operations
func (oqm *OperationQueueMonitor) Clear() {
	oqm.mu.Lock()
	defer oqm.mu.Unlock()
	oqm.queuedOps = oqm.queuedOps[:0]
	oqm.sentOps = oqm.sentOps[:0]
}

// ChunkMonitor monitors chunking operations
type ChunkMonitor struct {
	mu            sync.RWMutex
	chunksCreated []*chunking.Chunk
	chunksSent    []*chunking.Chunk
	chunksReceived []*chunking.Chunk
}

// NewChunkMonitor creates a new chunk monitor
func NewChunkMonitor() *ChunkMonitor {
	return &ChunkMonitor{
		chunksCreated:  make([]*chunking.Chunk, 0),
		chunksSent:     make([]*chunking.Chunk, 0),
		chunksReceived: make([]*chunking.Chunk, 0),
	}
}

// OnChunkCreated is called when a chunk is created
func (cm *ChunkMonitor) OnChunkCreated(chunk *chunking.Chunk) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.chunksCreated = append(cm.chunksCreated, chunk)
}

// OnChunkSent is called when a chunk is sent
func (cm *ChunkMonitor) OnChunkSent(chunk *chunking.Chunk) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.chunksSent = append(cm.chunksSent, chunk)
}

// OnChunkReceived is called when a chunk is received
func (cm *ChunkMonitor) OnChunkReceived(chunk *chunking.Chunk) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.chunksReceived = append(cm.chunksReceived, chunk)
}

// GetCreatedChunks returns all created chunks
func (cm *ChunkMonitor) GetCreatedChunks() []*chunking.Chunk {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	result := make([]*chunking.Chunk, len(cm.chunksCreated))
	copy(result, cm.chunksCreated)
	return result
}

// GetSentChunks returns all sent chunks
func (cm *ChunkMonitor) GetSentChunks() []*chunking.Chunk {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	result := make([]*chunking.Chunk, len(cm.chunksSent))
	copy(result, cm.chunksSent)
	return result
}

// GetReceivedChunks returns all received chunks
func (cm *ChunkMonitor) GetReceivedChunks() []*chunking.Chunk {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	result := make([]*chunking.Chunk, len(cm.chunksReceived))
	copy(result, cm.chunksReceived)
	return result
}

// GetCreatedChunksForFile returns created chunks for a specific file
func (cm *ChunkMonitor) GetCreatedChunksForFile(fileID string) []*chunking.Chunk {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	result := make([]*chunking.Chunk, 0)
	for _, chunk := range cm.chunksCreated {
		if chunk.FileID == fileID {
			result = append(result, chunk)
		}
	}
	return result
}

// GetReceivedChunksForFile returns received chunks for a specific file
func (cm *ChunkMonitor) GetReceivedChunksForFile(fileID string) []*chunking.Chunk {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	result := make([]*chunking.Chunk, 0)
	for _, chunk := range cm.chunksReceived {
		if chunk.FileID == fileID {
			result = append(result, chunk)
		}
	}
	return result
}

// Clear clears all monitored chunks
func (cm *ChunkMonitor) Clear() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.chunksCreated = cm.chunksCreated[:0]
	cm.chunksSent = cm.chunksSent[:0]
	cm.chunksReceived = cm.chunksReceived[:0]
}

// HeartbeatMonitor monitors heartbeat messages
type HeartbeatMonitor struct {
	mu               sync.RWMutex
	sentHeartbeats   []time.Time
	receivedHeartbeats []time.Time
}

// NewHeartbeatMonitor creates a new heartbeat monitor
func NewHeartbeatMonitor() *HeartbeatMonitor {
	return &HeartbeatMonitor{
		sentHeartbeats:     make([]time.Time, 0),
		receivedHeartbeats: make([]time.Time, 0),
	}
}

// OnHeartbeatSent is called when a heartbeat is sent
func (hm *HeartbeatMonitor) OnHeartbeatSent(timestamp time.Time) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.sentHeartbeats = append(hm.sentHeartbeats, timestamp)
}

// OnHeartbeatReceived is called when a heartbeat is received
func (hm *HeartbeatMonitor) OnHeartbeatReceived(timestamp time.Time) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.receivedHeartbeats = append(hm.receivedHeartbeats, timestamp)
}

// GetSentHeartbeats returns all sent heartbeat timestamps
func (hm *HeartbeatMonitor) GetSentHeartbeats() []time.Time {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	result := make([]time.Time, len(hm.sentHeartbeats))
	copy(result, hm.sentHeartbeats)
	return result
}

// GetReceivedHeartbeats returns all received heartbeat timestamps
func (hm *HeartbeatMonitor) GetReceivedHeartbeats() []time.Time {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	result := make([]time.Time, len(hm.receivedHeartbeats))
	copy(result, hm.receivedHeartbeats)
	return result
}

// GetSentHeartbeatCount returns the number of sent heartbeats
func (hm *HeartbeatMonitor) GetSentHeartbeatCount() int {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	return len(hm.sentHeartbeats)
}

// GetReceivedHeartbeatCount returns the number of received heartbeats
func (hm *HeartbeatMonitor) GetReceivedHeartbeatCount() int {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	return len(hm.receivedHeartbeats)
}

// Clear clears all monitored heartbeats
func (hm *HeartbeatMonitor) Clear() {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.sentHeartbeats = hm.sentHeartbeats[:0]
	hm.receivedHeartbeats = hm.receivedHeartbeats[:0]
}

// MockTransport is a mock transport for testing
type MockTransport struct {
	interceptor *MessageInterceptor
	sentMessages []*messages.Message
}

// NewMockTransport creates a new mock transport
func NewMockTransport() *MockTransport {
	return &MockTransport{
		interceptor:   NewMessageInterceptor(),
		sentMessages:  make([]*messages.Message, 0),
	}
}

// SendMessage intercepts and records message sends
func (mt *MockTransport) SendMessage(peerID string, address string, port int, msg interface{}) error {
	if message, ok := msg.(*messages.Message); ok {
		mt.interceptor.InterceptSend(peerID, address, port, message)
	}
	return nil // Mock success
}

// GetInterceptor returns the message interceptor
func (mt *MockTransport) GetInterceptor() *MessageInterceptor {
	return mt.interceptor
}
