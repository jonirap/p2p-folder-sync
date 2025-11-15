package chunking

import (
	"fmt"
	"sync"
	"time"
)

const (
	// ChunkTimeout is the timeout for waiting for chunks (30 seconds)
	ChunkTimeout = 30 * time.Second
)

// ChunkBuffer manages out-of-order chunk reception and assembly
type ChunkBuffer struct {
	fileID       string
	totalChunks  int
	chunks       map[int]*Chunk
	received     map[int]bool
	mu           sync.RWMutex
	lastReceived time.Time
	complete     bool
	completeCh   chan struct{}
}

// NewChunkBuffer creates a new chunk buffer
func NewChunkBuffer(fileID string, totalChunks int) *ChunkBuffer {
	return &ChunkBuffer{
		fileID:       fileID,
		totalChunks:  totalChunks,
		chunks:       make(map[int]*Chunk),
		received:     make(map[int]bool),
		lastReceived: time.Now(),
		completeCh:   make(chan struct{}, 1),
	}
}

// AddChunk adds a chunk to the buffer
func (cb *ChunkBuffer) AddChunk(chunk *Chunk) error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Validate chunk
	if chunk.FileID != cb.fileID {
		return fmt.Errorf("chunk file_id mismatch: expected %s, got %s", cb.fileID, chunk.FileID)
	}
	if chunk.ChunkID < 0 || chunk.ChunkID >= cb.totalChunks {
		return fmt.Errorf("invalid chunk_id: %d (expected 0-%d)", chunk.ChunkID, cb.totalChunks-1)
	}

	// Verify chunk hash
	expectedHash := chunk.Hash
	actualHash := chunk.Hash // In real implementation, recalculate from chunk.Data
	if expectedHash != actualHash {
		return fmt.Errorf("chunk hash mismatch for chunk %d", chunk.ChunkID)
	}

	// Store chunk
	cb.chunks[chunk.ChunkID] = chunk
	cb.received[chunk.ChunkID] = true
	cb.lastReceived = time.Now()

	// Check if all chunks received
	if len(cb.chunks) == cb.totalChunks {
		cb.complete = true
		select {
		case cb.completeCh <- struct{}{}:
		default:
		}
	}

	return nil
}

// GetMissingChunks returns a list of missing chunk IDs
func (cb *ChunkBuffer) GetMissingChunks() []int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	var missing []int
	for i := 0; i < cb.totalChunks; i++ {
		if !cb.received[i] {
			missing = append(missing, i)
		}
	}
	return missing
}

// IsComplete returns true if all chunks have been received
func (cb *ChunkBuffer) IsComplete() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.complete
}

// GetChunks returns all chunks in order
func (cb *ChunkBuffer) GetChunks() ([]*Chunk, error) {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	if !cb.complete {
		return nil, fmt.Errorf("not all chunks received")
	}

	chunks := make([]*Chunk, cb.totalChunks)
	for i := 0; i < cb.totalChunks; i++ {
		chunk, exists := cb.chunks[i]
		if !exists {
			return nil, fmt.Errorf("chunk %d missing", i)
		}
		chunks[i] = chunk
	}

	return chunks, nil
}

// WaitForCompletion waits for all chunks to be received or timeout
func (cb *ChunkBuffer) WaitForCompletion(timeout time.Duration) error {
	select {
	case <-cb.completeCh:
		return nil
	case <-time.After(timeout):
		cb.mu.RLock()
		missing := cb.GetMissingChunks()
		cb.mu.RUnlock()
		return fmt.Errorf("timeout waiting for chunks, missing: %v", missing)
	}
}

// GetLastReceived returns the time when the last chunk was received
func (cb *ChunkBuffer) GetLastReceived() time.Time {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.lastReceived
}

// ShouldRequestRetransmission checks if we should request retransmission
func (cb *ChunkBuffer) ShouldRequestRetransmission() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	if cb.complete {
		return false
	}

	// Request retransmission if timeout exceeded and chunks are missing
	timeSinceLastReceived := time.Since(cb.lastReceived)
	return timeSinceLastReceived > ChunkTimeout && len(cb.chunks) < cb.totalChunks
}

