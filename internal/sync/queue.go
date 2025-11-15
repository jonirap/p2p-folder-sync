package sync

import (
	"sync"
)

// OperationQueue manages operation queues per file
type OperationQueue struct {
	queues map[string][]*SyncOperation
	mu     sync.RWMutex
}

// NewOperationQueue creates a new operation queue
func NewOperationQueue() *OperationQueue {
	return &OperationQueue{
		queues: make(map[string][]*SyncOperation),
	}
}

// Enqueue adds an operation to the queue for a file
func (q *OperationQueue) Enqueue(fileID string, op *SyncOperation) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.queues[fileID] = append(q.queues[fileID], op)
}

// Dequeue removes and returns the next operation for a file
func (q *OperationQueue) Dequeue(fileID string) *SyncOperation {
	q.mu.Lock()
	defer q.mu.Unlock()

	queue, exists := q.queues[fileID]
	if !exists || len(queue) == 0 {
		return nil
	}

	op := queue[0]
	q.queues[fileID] = queue[1:]

	if len(q.queues[fileID]) == 0 {
		delete(q.queues, fileID)
	}

	return op
}

// Peek returns the next operation without removing it
func (q *OperationQueue) Peek(fileID string) *SyncOperation {
	q.mu.RLock()
	defer q.mu.RUnlock()

	queue, exists := q.queues[fileID]
	if !exists || len(queue) == 0 {
		return nil
	}

	return queue[0]
}

// IsEmpty checks if the queue for a file is empty
func (q *OperationQueue) IsEmpty(fileID string) bool {
	q.mu.RLock()
	defer q.mu.RUnlock()

	queue, exists := q.queues[fileID]
	return !exists || len(queue) == 0
}

// Size returns the number of operations in the queue for a file
func (q *OperationQueue) Size(fileID string) int {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return len(q.queues[fileID])
}

