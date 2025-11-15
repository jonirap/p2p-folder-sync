package system

import (
	"fmt"
	"sync"
	"time"

	"github.com/p2p-folder-sync/p2p-sync/internal/network/messages"
	syncpkg "github.com/p2p-folder-sync/p2p-sync/internal/sync"
)

// EnhancedOperationMonitor provides comprehensive monitoring for both in-memory and network operations
type EnhancedOperationMonitor struct {
	mu sync.RWMutex

	// Operation tracking
	operations   []*syncpkg.SyncOperation
	operationMap map[string]*syncpkg.SyncOperation // ID -> operation

	// Network message tracking (for real network tests)
	sentMessages     []*messages.Message
	receivedMessages []*messages.Message

	// Timing and performance tracking
	operationTimestamps map[string]time.Time // operation ID -> timestamp
	messageTimestamps   map[string]time.Time // message ID -> timestamp

	// Peer-specific tracking
	peerOperations map[string][]*syncpkg.SyncOperation // peerID -> operations
	peerMessages   map[string][]*messages.Message      // peerID -> messages
}

// NewEnhancedOperationMonitor creates a new enhanced operation monitor
func NewEnhancedOperationMonitor() *EnhancedOperationMonitor {
	return &EnhancedOperationMonitor{
		operations:          make([]*syncpkg.SyncOperation, 0),
		operationMap:        make(map[string]*syncpkg.SyncOperation),
		sentMessages:        make([]*messages.Message, 0),
		receivedMessages:    make([]*messages.Message, 0),
		operationTimestamps: make(map[string]time.Time),
		messageTimestamps:   make(map[string]time.Time),
		peerOperations:      make(map[string][]*syncpkg.SyncOperation),
		peerMessages:        make(map[string][]*messages.Message),
	}
}

// OnOperationQueued records a sync operation
func (eom *EnhancedOperationMonitor) OnOperationQueued(op *syncpkg.SyncOperation) {
	eom.mu.Lock()
	defer eom.mu.Unlock()

	// Deep copy the operation to avoid mutations
	opCopy := *op
	eom.operations = append(eom.operations, &opCopy)
	eom.operationMap[op.ID] = &opCopy
	eom.operationTimestamps[op.ID] = time.Now()

	// Track by peer
	if eom.peerOperations[op.PeerID] == nil {
		eom.peerOperations[op.PeerID] = make([]*syncpkg.SyncOperation, 0)
	}
	eom.peerOperations[op.PeerID] = append(eom.peerOperations[op.PeerID], &opCopy)
}

// OnMessageSent records a sent network message
func (eom *EnhancedOperationMonitor) OnMessageSent(msg *messages.Message) {
	eom.mu.Lock()
	defer eom.mu.Unlock()

	// Deep copy the message
	msgCopy := *msg
	eom.sentMessages = append(eom.sentMessages, &msgCopy)
	eom.messageTimestamps[msg.ID] = time.Now()

	// Extract peer ID from message (might be in payload or correlation ID)
	peerID := extractPeerIDFromMessage(msg)
	if peerID != "" {
		if eom.peerMessages[peerID] == nil {
			eom.peerMessages[peerID] = make([]*messages.Message, 0)
		}
		eom.peerMessages[peerID] = append(eom.peerMessages[peerID], &msgCopy)
	}
}

// OnMessageReceived records a received network message
func (eom *EnhancedOperationMonitor) OnMessageReceived(msg *messages.Message) {
	eom.mu.Lock()
	defer eom.mu.Unlock()

	// Deep copy the message
	msgCopy := *msg
	eom.receivedMessages = append(eom.receivedMessages, &msgCopy)

	// Extract peer ID from message
	peerID := extractPeerIDFromMessage(msg)
	if peerID != "" {
		if eom.peerMessages[peerID] == nil {
			eom.peerMessages[peerID] = make([]*messages.Message, 0)
		}
		eom.peerMessages[peerID] = append(eom.peerMessages[peerID], &msgCopy)
	}
}

// GetAllOperations returns all recorded operations
func (eom *EnhancedOperationMonitor) GetAllOperations() []*syncpkg.SyncOperation {
	eom.mu.RLock()
	defer eom.mu.RUnlock()

	result := make([]*syncpkg.SyncOperation, len(eom.operations))
	copy(result, eom.operations)
	return result
}

// GetOperationsByPeer returns operations for a specific peer
func (eom *EnhancedOperationMonitor) GetOperationsByPeer(peerID string) []*syncpkg.SyncOperation {
	eom.mu.RLock()
	defer eom.mu.RUnlock()

	ops := eom.peerOperations[peerID]
	if ops == nil {
		return []*syncpkg.SyncOperation{}
	}

	result := make([]*syncpkg.SyncOperation, len(ops))
	copy(result, ops)
	return result
}

// GetOperationsByType returns operations of a specific type
func (eom *EnhancedOperationMonitor) GetOperationsByType(opType syncpkg.OperationType) []*syncpkg.SyncOperation {
	eom.mu.RLock()
	defer eom.mu.RUnlock()

	var result []*syncpkg.SyncOperation
	for _, op := range eom.operations {
		if op.Type == opType {
			result = append(result, op)
		}
	}
	return result
}

// GetOperationByID returns a specific operation by ID
func (eom *EnhancedOperationMonitor) GetOperationByID(id string) (*syncpkg.SyncOperation, bool) {
	eom.mu.RLock()
	defer eom.mu.RUnlock()

	op, exists := eom.operationMap[id]
	if !exists {
		return nil, false
	}

	// Return a copy
	opCopy := *op
	return &opCopy, true
}

// GetSentMessages returns all sent messages
func (eom *EnhancedOperationMonitor) GetSentMessages() []*messages.Message {
	eom.mu.RLock()
	defer eom.mu.RUnlock()

	result := make([]*messages.Message, len(eom.sentMessages))
	copy(result, eom.sentMessages)
	return result
}

// GetReceivedMessages returns all received messages
func (eom *EnhancedOperationMonitor) GetReceivedMessages() []*messages.Message {
	eom.mu.RLock()
	defer eom.mu.RUnlock()

	result := make([]*messages.Message, len(eom.receivedMessages))
	copy(result, eom.receivedMessages)
	return result
}

// GetMessagesByType returns messages of a specific type
func (eom *EnhancedOperationMonitor) GetMessagesByType(msgType string) []*messages.Message {
	eom.mu.RLock()
	defer eom.mu.RUnlock()

	var result []*messages.Message
	for _, msg := range eom.sentMessages {
		if msg.Type == msgType {
			result = append(result, msg)
		}
	}
	for _, msg := range eom.receivedMessages {
		if msg.Type == msgType {
			result = append(result, msg)
		}
	}
	return result
}

// GetOperationTiming returns the timestamp when an operation was recorded
func (eom *EnhancedOperationMonitor) GetOperationTiming(operationID string) (time.Time, bool) {
	eom.mu.RLock()
	defer eom.mu.RUnlock()

	timestamp, exists := eom.operationTimestamps[operationID]
	return timestamp, exists
}

// GetMessageTiming returns the timestamp when a message was recorded
func (eom *EnhancedOperationMonitor) GetMessageTiming(messageID string) (time.Time, bool) {
	eom.mu.RLock()
	defer eom.mu.RUnlock()

	timestamp, exists := eom.messageTimestamps[messageID]
	return timestamp, exists
}

// GetOperationCount returns the total number of operations recorded
func (eom *EnhancedOperationMonitor) GetOperationCount() int {
	eom.mu.RLock()
	defer eom.mu.RUnlock()
	return len(eom.operations)
}

// GetMessageCount returns the total number of messages recorded
func (eom *EnhancedOperationMonitor) GetMessageCount() int {
	eom.mu.RLock()
	defer eom.mu.RUnlock()
	return len(eom.sentMessages) + len(eom.receivedMessages)
}

// GetPeerStats returns statistics for each peer
func (eom *EnhancedOperationMonitor) GetPeerStats() map[string]PeerStats {
	eom.mu.RLock()
	defer eom.mu.RUnlock()

	stats := make(map[string]PeerStats)

	// Count operations per peer
	for peerID, ops := range eom.peerOperations {
		stats[peerID] = PeerStats{
			PeerID:         peerID,
			OperationCount: len(ops),
			MessageCount:   len(eom.peerMessages[peerID]),
		}
	}

	// Count messages for peers not in operations
	for peerID, msgs := range eom.peerMessages {
		if _, exists := stats[peerID]; !exists {
			stats[peerID] = PeerStats{
				PeerID:         peerID,
				OperationCount: 0,
				MessageCount:   len(msgs),
			}
		} else {
			s := stats[peerID]
			s.MessageCount = len(msgs)
			stats[peerID] = s
		}
	}

	return stats
}

// PeerStats contains statistics for a specific peer
type PeerStats struct {
	PeerID         string
	OperationCount int
	MessageCount   int
}

// WaitForOperationType waits for an operation of the specified type
func (eom *EnhancedOperationMonitor) WaitForOperationType(opType syncpkg.OperationType, timeout time.Duration) (*syncpkg.SyncOperation, error) {
	startTime := time.Now()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-time.After(timeout):
			return nil, fmt.Errorf("timeout waiting for operation type %s after %v", opType, time.Since(startTime))
		case <-ticker.C:
			if ops := eom.GetOperationsByType(opType); len(ops) > 0 {
				return ops[0], nil
			}
		}
	}
}

// WaitForMessageType waits for a message of the specified type
func (eom *EnhancedOperationMonitor) WaitForMessageType(msgType string, timeout time.Duration) (*messages.Message, error) {
	startTime := time.Now()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-time.After(timeout):
			return nil, fmt.Errorf("timeout waiting for message type %s after %v", msgType, time.Since(startTime))
		case <-ticker.C:
			if msgs := eom.GetMessagesByType(msgType); len(msgs) > 0 {
				return msgs[0], nil
			}
		}
	}
}

// WaitForOperationCount waits until the operation count reaches the specified number
func (eom *EnhancedOperationMonitor) WaitForOperationCount(expectedCount int, timeout time.Duration) error {
	startTime := time.Now()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-time.After(timeout):
			return fmt.Errorf("timeout waiting for operation count %d (got %d) after %v",
				expectedCount, eom.GetOperationCount(), time.Since(startTime))
		case <-ticker.C:
			if eom.GetOperationCount() >= expectedCount {
				return nil
			}
		}
	}
}

// Clear resets all monitoring data
func (eom *EnhancedOperationMonitor) Clear() {
	eom.mu.Lock()
	defer eom.mu.Unlock()

	eom.operations = eom.operations[:0]
	eom.operationMap = make(map[string]*syncpkg.SyncOperation)
	eom.sentMessages = eom.sentMessages[:0]
	eom.receivedMessages = eom.receivedMessages[:0]
	eom.operationTimestamps = make(map[string]time.Time)
	eom.messageTimestamps = make(map[string]time.Time)
	eom.peerOperations = make(map[string][]*syncpkg.SyncOperation)
	eom.peerMessages = make(map[string][]*messages.Message)
}

// extractPeerIDFromMessage attempts to extract a peer ID from a message
func extractPeerIDFromMessage(msg *messages.Message) string {
	// Try correlation ID first (might contain peer ID)
	if msg.CorrelationID != nil && *msg.CorrelationID != "" {
		// Correlation ID might be in format "peerID-operationID"
		// This is a heuristic - in a real implementation, you'd want more structured correlation IDs
		return *msg.CorrelationID
	}

	// Try sender ID
	if msg.SenderID != "" {
		return msg.SenderID
	}

	// For sync operations, try to extract from payload
	if msg.Type == messages.TypeSyncOperation {
		// This would require decoding the payload, which might be encrypted
		// For now, return empty string
	}

	return ""
}

// Note: UniversalOperationMonitor removed due to cross-package dependencies.
// Use EnhancedOperationMonitor and NetworkOperationMonitor directly instead.
