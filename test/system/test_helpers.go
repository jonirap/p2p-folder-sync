package system

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/p2p-folder-sync/p2p-sync/internal/filesystem"
	syncpkg "github.com/p2p-folder-sync/p2p-sync/internal/sync"
)

// EventDrivenWaiter provides event-driven waiting utilities to replace sleep-based timing
type EventDrivenWaiter struct {
	timeout time.Duration
	ticker  *time.Ticker
}

// NewEventDrivenWaiter creates a new event-driven waiter with default timeout
func NewEventDrivenWaiter() *EventDrivenWaiter {
	return &EventDrivenWaiter{
		timeout: 30 * time.Second, // Default 30 second timeout
		ticker:  time.NewTicker(100 * time.Millisecond),
	}
}

// NewEventDrivenWaiterWithTimeout creates a new event-driven waiter with custom timeout
func NewEventDrivenWaiterWithTimeout(timeout time.Duration) *EventDrivenWaiter {
	return &EventDrivenWaiter{
		timeout: timeout,
		ticker:  time.NewTicker(100 * time.Millisecond),
	}
}

// WaitForFileSync waits for a file to appear in a directory
func (w *EventDrivenWaiter) WaitForFileSync(dir, filename string) error {
	ctx, cancel := context.WithTimeout(context.Background(), w.timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for file %s to sync to %s", filename, dir)
		case <-w.ticker.C:
			filePath := filepath.Join(dir, filename)
			if _, err := os.Stat(filePath); err == nil {
				return nil // File exists
			}
		}
	}
}

// WaitForFileDeletion waits for a file to be deleted from a directory
func (w *EventDrivenWaiter) WaitForFileDeletion(dir, filename string) error {
	ctx, cancel := context.WithTimeout(context.Background(), w.timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for file %s to be deleted from %s", filename, dir)
		case <-w.ticker.C:
			filePath := filepath.Join(dir, filename)
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				return nil // File no longer exists
			}
		}
	}
}

// WaitForFileContent waits for a file to have specific content
func (w *EventDrivenWaiter) WaitForFileContent(dir, filename string, expectedContent []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), w.timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for file %s content to match", filename)
		case <-w.ticker.C:
			filePath := filepath.Join(dir, filename)
			if content, err := os.ReadFile(filePath); err == nil {
				if string(content) == string(expectedContent) {
					return nil // Content matches
				}
			}
		}
	}
}

// WaitForMultipleFiles waits for multiple files to appear
func (w *EventDrivenWaiter) WaitForMultipleFiles(dir string, filenames []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), w.timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for all files to sync to %s", dir)
		case <-w.ticker.C:
			allPresent := true
			for _, filename := range filenames {
				filePath := filepath.Join(dir, filename)
				if _, err := os.Stat(filePath); os.IsNotExist(err) {
					allPresent = false
					break
				}
			}
			if allPresent {
				return nil
			}
		}
	}
}

// WaitForOperationCount waits for the operation queue to reach a specific count
func (w *EventDrivenWaiter) WaitForOperationCount(engine *syncpkg.Engine, expectedCount int) error {
	ctx, cancel := context.WithTimeout(context.Background(), w.timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for operation count to reach %d", expectedCount)
		case <-w.ticker.C:
			// Note: This would need access to internal engine state
			// For now, we'll need to add a method to expose operation count
			_ = engine
			// if count := engine.GetOperationCount(); count >= expectedCount {
			// 	return nil
			// }
		}
	}
}

// WaitForCondition waits for a custom condition to be true
func (w *EventDrivenWaiter) WaitForCondition(condition func() bool, description string) error {
	ctx, cancel := context.WithTimeout(context.Background(), w.timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for condition: %s", description)
		case <-w.ticker.C:
			if condition() {
				return nil
			}
		}
	}
}

// Close closes the waiter and cleans up resources
func (w *EventDrivenWaiter) Close() {
	if w.ticker != nil {
		w.ticker.Stop()
	}
}

// WaitForSyncOperation waits for a specific sync operation to be queued
type SyncOperationWaiter struct {
	mu         sync.RWMutex
	operations []*syncpkg.SyncOperation
	waitCh     chan *syncpkg.SyncOperation
}

// NewSyncOperationWaiter creates a waiter for sync operations
func NewSyncOperationWaiter() *SyncOperationWaiter {
	return &SyncOperationWaiter{
		operations: make([]*syncpkg.SyncOperation, 0),
		waitCh:     make(chan *syncpkg.SyncOperation, 10),
	}
}

// OnOperationQueued is called when an operation is queued (to be hooked into messenger)
func (w *SyncOperationWaiter) OnOperationQueued(op *syncpkg.SyncOperation) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.operations = append(w.operations, op)
	select {
	case w.waitCh <- op:
	default:
		// Channel full, drop operation (for testing)
	}
}

// WaitForOperationType waits for an operation of a specific type
func (w *SyncOperationWaiter) WaitForOperationType(opType syncpkg.OperationType, timeout time.Duration) (*syncpkg.SyncOperation, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout waiting for operation type %v", opType)
		case op := <-w.waitCh:
			if op.Type == opType {
				return op, nil
			}
		}
	}
}

// WaitForOperationOnFile waits for an operation on a specific file
func (w *SyncOperationWaiter) WaitForOperationOnFile(filename string, timeout time.Duration) (*syncpkg.SyncOperation, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout waiting for operation on file %s", filename)
		case op := <-w.waitCh:
			if filepath.Base(op.Path) == filename {
				return op, nil
			}
		}
	}
}

// GetAllOperations returns all captured operations
func (w *SyncOperationWaiter) GetAllOperations() []*syncpkg.SyncOperation {
	w.mu.RLock()
	defer w.mu.RUnlock()
	result := make([]*syncpkg.SyncOperation, len(w.operations))
	copy(result, w.operations)
	return result
}

// Clear clears all captured operations
func (w *SyncOperationWaiter) Clear() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.operations = w.operations[:0]
}

// FileEventWaiter waits for filesystem events
type FileEventWaiter struct {
	events chan filesystem.FileEvent
	waiter *EventDrivenWaiter
}

// NewFileEventWaiter creates a waiter for filesystem events
func NewFileEventWaiter() *FileEventWaiter {
	return &FileEventWaiter{
		events: make(chan filesystem.FileEvent, 10),
		waiter: NewEventDrivenWaiter(),
	}
}

// OnFileEvent is called when a filesystem event occurs
func (w *FileEventWaiter) OnFileEvent(event filesystem.FileEvent) {
	select {
	case w.events <- event:
	default:
		// Channel full, drop event
	}
}

// WaitForEventType waits for a filesystem event of a specific type
func (w *FileEventWaiter) WaitForEventType(eventType string) (filesystem.FileEvent, error) {
	for {
		select {
		case event := <-w.events:
			if event.Operation == eventType {
				return event, nil
			}
		case <-time.After(w.waiter.timeout):
			return filesystem.FileEvent{}, fmt.Errorf("timeout waiting for event type %s", eventType)
		}
	}
}

// WaitForEventOnFile waits for an event on a specific file
func (w *FileEventWaiter) WaitForEventOnFile(filename, eventType string) (filesystem.FileEvent, error) {
	for {
		select {
		case event := <-w.events:
			if filepath.Base(event.Path) == filename && event.Operation == eventType {
				return event, nil
			}
		case <-time.After(w.waiter.timeout):
			return filesystem.FileEvent{}, fmt.Errorf("timeout waiting for %s event on file %s", eventType, filename)
		}
	}
}

// OperationMonitoringMessenger wraps a messenger to monitor sync operations
type OperationMonitoringMessenger struct {
	innerMessenger *syncpkg.InMemoryMessenger
	peer1Waiter    *SyncOperationWaiter
	peer2Waiter    *SyncOperationWaiter
}

func (omm *OperationMonitoringMessenger) SendFile(peerID string, fileData []byte, metadata *syncpkg.SyncOperation) error {
	return omm.innerMessenger.SendFile(peerID, fileData, metadata)
}

func (omm *OperationMonitoringMessenger) BroadcastOperation(op *syncpkg.SyncOperation) error {
	// Notify the appropriate waiter based on the sender
	if op.PeerID == "peer1" && omm.peer1Waiter != nil {
		omm.peer1Waiter.OnOperationQueued(op)
	} else if op.PeerID == "peer2" && omm.peer2Waiter != nil {
		omm.peer2Waiter.OnOperationQueued(op)
	}

	return omm.innerMessenger.BroadcastOperation(op)
}

func (omm *OperationMonitoringMessenger) RequestStateSync(peerID string) error {
	return omm.innerMessenger.RequestStateSync(peerID)
}

func (omm *OperationMonitoringMessenger) ConnectToPeer(peerID string, address string, port int) error {
	return omm.innerMessenger.ConnectToPeer(peerID, address, port)
}
