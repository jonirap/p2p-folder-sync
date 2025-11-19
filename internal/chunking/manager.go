package chunking

import (
	"fmt"
	"sync"
	"time"
)

const (
	// MaxRetransmissionAttempts is the maximum number of times to request retransmission
	MaxRetransmissionAttempts = 3
)

// ChunkReceipt tracks when a chunk was received
type ChunkReceipt struct {
	Chunk       *Chunk
	ReceivedAt  time.Time
	Verified    bool
}

// FileTransfer tracks an ongoing file transfer with chunking
type FileTransfer struct {
	FileID              string
	FileHash            string
	TotalChunks         int
	ReceivedChunks      map[int]*ChunkReceipt // ChunkID -> Receipt
	LastReceived        time.Time
	RetransmissionCount int
	Complete            bool
	ExpectedSize        int64
	mu                  sync.RWMutex
}

// ChunkManager manages chunk reception, timeout, and retransmission
type ChunkManager struct {
	transfers map[string]*FileTransfer // FileID -> Transfer
	mu        sync.RWMutex
	stopCh    chan struct{}
	assembler *Assembler
}

// NewChunkManager creates a new chunk manager
func NewChunkManager() *ChunkManager {
	cm := &ChunkManager{
		transfers: make(map[string]*FileTransfer),
		stopCh:    make(chan struct{}),
		assembler: NewAssembler(),
	}

	// Start timeout monitor
	go cm.monitorTimeouts()

	return cm
}

// Stop stops the chunk manager
func (cm *ChunkManager) Stop() {
	close(cm.stopCh)
}

// StartTransfer initializes tracking for a new file transfer
func (cm *ChunkManager) StartTransfer(fileID string, fileHash string, totalChunks int, expectedSize int64) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if _, exists := cm.transfers[fileID]; exists {
		return fmt.Errorf("transfer already exists for file %s", fileID)
	}

	cm.transfers[fileID] = &FileTransfer{
		FileID:         fileID,
		FileHash:       fileHash,
		TotalChunks:    totalChunks,
		ReceivedChunks: make(map[int]*ChunkReceipt),
		LastReceived:   time.Now(),
		Complete:       false,
		ExpectedSize:   expectedSize,
	}

	return nil
}

// ReceiveChunk processes an incoming chunk
func (cm *ChunkManager) ReceiveChunk(chunk *Chunk) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	transfer, exists := cm.transfers[chunk.FileID]
	if !exists {
		// Auto-create transfer if first chunk received
		transfer = &FileTransfer{
			FileID:         chunk.FileID,
			FileHash:       "", // Will be set when all chunks received
			TotalChunks:    0,  // Unknown until last chunk
			ReceivedChunks: make(map[int]*ChunkReceipt),
			LastReceived:   time.Now(),
			Complete:       false,
		}
		cm.transfers[chunk.FileID] = transfer
	}

	transfer.mu.Lock()
	defer transfer.mu.Unlock()

	// Check if chunk already received
	if _, exists := transfer.ReceivedChunks[chunk.ChunkID]; exists {
		// Duplicate chunk, ignore
		return nil
	}

	// Verify chunk hash
	if err := cm.assembler.VerifyChunkHash(chunk); err != nil {
		return fmt.Errorf("chunk verification failed: %w", err)
	}

	// Store chunk receipt
	transfer.ReceivedChunks[chunk.ChunkID] = &ChunkReceipt{
		Chunk:      chunk,
		ReceivedAt: time.Now(),
		Verified:   true,
	}
	transfer.LastReceived = time.Now()

	// If this is the last chunk, update total chunks
	if chunk.IsLast {
		transfer.TotalChunks = chunk.ChunkID + 1
		transfer.FileHash = chunk.FileHash
	}

	// Check if transfer is complete
	if transfer.TotalChunks > 0 && len(transfer.ReceivedChunks) == transfer.TotalChunks {
		transfer.Complete = true
	}

	return nil
}

// IsTransferComplete checks if a file transfer is complete
func (cm *ChunkManager) IsTransferComplete(fileID string) bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	transfer, exists := cm.transfers[fileID]
	if !exists {
		return false
	}

	transfer.mu.RLock()
	defer transfer.mu.RUnlock()

	return transfer.Complete
}

// AssembleFile assembles all received chunks into the complete file
func (cm *ChunkManager) AssembleFile(fileID string) ([]byte, error) {
	cm.mu.RLock()
	transfer, exists := cm.transfers[fileID]
	cm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("transfer not found for file %s", fileID)
	}

	transfer.mu.RLock()
	defer transfer.mu.RUnlock()

	if !transfer.Complete {
		return nil, fmt.Errorf("transfer not complete: %d/%d chunks received",
			len(transfer.ReceivedChunks), transfer.TotalChunks)
	}

	// Extract chunks from receipts
	chunks := make([]*Chunk, 0, len(transfer.ReceivedChunks))
	for _, receipt := range transfer.ReceivedChunks {
		chunks = append(chunks, receipt.Chunk)
	}

	// Assemble and verify
	data, err := cm.assembler.AssembleChunks(chunks, transfer.FileHash)
	if err != nil {
		return nil, fmt.Errorf("assembly failed: %w", err)
	}

	return data, nil
}

// GetMissingChunks returns the IDs of chunks that haven't been received yet
func (cm *ChunkManager) GetMissingChunks(fileID string) ([]int, error) {
	cm.mu.RLock()
	transfer, exists := cm.transfers[fileID]
	cm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("transfer not found for file %s", fileID)
	}

	transfer.mu.RLock()
	defer transfer.mu.RUnlock()

	if transfer.TotalChunks == 0 {
		// Don't know total chunks yet, can't determine missing
		return nil, nil
	}

	missing := make([]int, 0)
	for i := 0; i < transfer.TotalChunks; i++ {
		if _, received := transfer.ReceivedChunks[i]; !received {
			missing = append(missing, i)
		}
	}

	return missing, nil
}

// HasTimedOut checks if a transfer has timed out waiting for chunks
func (cm *ChunkManager) HasTimedOut(fileID string) bool {
	cm.mu.RLock()
	transfer, exists := cm.transfers[fileID]
	cm.mu.RUnlock()

	if !exists {
		return false
	}

	transfer.mu.RLock()
	defer transfer.mu.RUnlock()

	if transfer.Complete {
		return false
	}

	// Timeout if we know we're missing chunks and haven't received anything recently
	if transfer.TotalChunks > 0 && len(transfer.ReceivedChunks) < transfer.TotalChunks {
		timeSinceLastChunk := time.Since(transfer.LastReceived)
		return timeSinceLastChunk > ChunkTimeout
	}

	return false
}

// IncrementRetransmissionCount increments the retransmission attempt counter
func (cm *ChunkManager) IncrementRetransmissionCount(fileID string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	transfer, exists := cm.transfers[fileID]
	if !exists {
		return fmt.Errorf("transfer not found for file %s", fileID)
	}

	transfer.mu.Lock()
	defer transfer.mu.Unlock()

	transfer.RetransmissionCount++
	return nil
}

// GetRetransmissionCount gets the current retransmission attempt count
func (cm *ChunkManager) GetRetransmissionCount(fileID string) int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	transfer, exists := cm.transfers[fileID]
	if !exists {
		return 0
	}

	transfer.mu.RLock()
	defer transfer.mu.RUnlock()

	return transfer.RetransmissionCount
}

// CleanupTransfer removes a completed or failed transfer
func (cm *ChunkManager) CleanupTransfer(fileID string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	delete(cm.transfers, fileID)
}

// monitorTimeouts periodically checks for timed-out transfers
func (cm *ChunkManager) monitorTimeouts() {
	ticker := time.NewTicker(5 * time.Second) // Check every 5 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cm.checkTimeouts()
		case <-cm.stopCh:
			return
		}
	}
}

// checkTimeouts checks all transfers for timeouts
func (cm *ChunkManager) checkTimeouts() {
	cm.mu.RLock()
	fileIDs := make([]string, 0, len(cm.transfers))
	for fileID := range cm.transfers {
		fileIDs = append(fileIDs, fileID)
	}
	cm.mu.RUnlock()

	for _, fileID := range fileIDs {
		if cm.HasTimedOut(fileID) {
			// Transfer has timed out - application should request retransmission
			// This is just monitoring - actual retransmission request is handled by caller
		}
	}
}

// GetTransferStatus returns the status of a file transfer
func (cm *ChunkManager) GetTransferStatus(fileID string) (received int, total int, complete bool, err error) {
	cm.mu.RLock()
	transfer, exists := cm.transfers[fileID]
	cm.mu.RUnlock()

	if !exists {
		return 0, 0, false, fmt.Errorf("transfer not found for file %s", fileID)
	}

	transfer.mu.RLock()
	defer transfer.mu.RUnlock()

	return len(transfer.ReceivedChunks), transfer.TotalChunks, transfer.Complete, nil
}
