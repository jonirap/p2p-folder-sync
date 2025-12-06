package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/p2p-folder-sync/p2p-sync/internal/config"
	"github.com/p2p-folder-sync/p2p-sync/internal/database"
)

type mockMessenger struct {
	broadcastOpCalled bool
	lastOperation     *SyncOperation
	mu                sync.Mutex
}

func (m *mockMessenger) SendFile(peerID string, fileData []byte, metadata *SyncOperation) error {
	return nil
}

func (m *mockMessenger) BroadcastOperation(op *SyncOperation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.broadcastOpCalled = true
	m.lastOperation = op
	return nil
}

func (m *mockMessenger) RequestStateSync(peerID string) error {
	return nil
}

func (m *mockMessenger) ConnectToPeer(peerID string, address string, port int) error {
	return nil
}

func setupTestEngine(t *testing.T) (*Engine, func()) {
	t.Helper()

	// Create a temporary directory for the database
	dbDir, err := os.MkdirTemp("", "testdb")
	if err != nil {
		t.Fatalf("Failed to create temp dir for db: %v", err)
	}

	dbPath := filepath.Join(dbDir, "test.db")
	db, err := database.NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Create a temporary directory for sync
	syncDir, err := os.MkdirTemp("", "testsync")
	if err != nil {
		t.Fatalf("Failed to create temp dir for sync: %v", err)
	}

	cfg := &config.Config{
		Sync: config.SyncConfig{
			FolderPath: syncDir,
		},
		Conflict: config.ConflictConfig{
			ResolutionStrategy: "local",
		},
	}

	engine, err := NewEngine(cfg, db, "test-peer")
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	cleanup := func() {
		engine.Stop()
		db.Close()
		os.RemoveAll(dbDir)
		os.RemoveAll(syncDir)
	}

	return engine, cleanup
}

// TestQueueOperationConcurrent validates that queueing operations from multiple
// goroutines does not cause a race condition or a panic from the database.
// This specifically tests the mutex lock added to `queueOperation`.
func TestQueueOperationConcurrent(t *testing.T) {
	engine, cleanup := setupTestEngine(t)
	defer cleanup()

	var wg sync.WaitGroup
	numGoroutines := 50

	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(i int) {
			defer wg.Done()
			op := NewSyncOperation(OpCreate, fmt.Sprintf("file-%d.txt", i), fmt.Sprintf("file-id-%d", i), "test-peer")
			op.Checksum = "checksum"
			op.Size = 123
			op.Mtime = time.Now().UnixMilli()
			engine.queueOperation(op)
		}(i)
	}

	wg.Wait()
}

// TestRecentWritesCleanup validates that the cleanup function for the recent
// writes cache correctly removes expired entries.
func TestRecentWritesCleanup(t *testing.T) {
	engine, cleanup := setupTestEngine(t)
	defer cleanup()

	// Set a short TTL for testing
	engine.recentWritesTTL = 50 * time.Millisecond

	// Add an entry to the cache
	checksum := "expired-checksum"
	engine.addRecentWrite(checksum)

	// Verify it exists
	if !engine.isRecentWrite(checksum) {
		t.Fatal("Checksum should be considered recent immediately after adding")
	}

	// Wait for the entry to expire
	time.Sleep(engine.recentWritesTTL + 10*time.Millisecond)

	// Run cleanup and verify the entry is gone
	engine.cleanupRecentWrites()
	if engine.isRecentWrite(checksum) {
		t.Fatal("Checksum should be expired and removed after cleanup")
	}
}

func TestHandleDelete_UntrackedFile(t *testing.T) {
	engine, cleanup := setupTestEngine(t)
	defer cleanup()

	// Create a mock messenger and attach it to the engine
	messenger := &mockMessenger{}
	engine.messenger = messenger

	// Create a file in the sync directory that is NOT tracked by the database
	untrackedFilePath := filepath.Join(engine.config.Sync.FolderPath, "untracked-file.txt")
	err := os.WriteFile(untrackedFilePath, []byte("i am not tracked"), 0644)
	if err != nil {
		t.Fatalf("Failed to create untracked file: %v", err)
	}

	// Manually call handleDelete for this untracked file
	engine.handleDelete(untrackedFilePath)

	// Wait for the goroutine to complete broadcasting
	time.Sleep(100 * time.Millisecond)

	// Check if BroadcastOperation was called on the messenger
	messenger.mu.Lock()
	defer messenger.mu.Unlock()

	if !messenger.broadcastOpCalled {
		t.Fatal("BroadcastOperation was not called for a deleted untracked file")
	}

	if messenger.lastOperation.Type != OpDelete {
		t.Errorf("Expected operation type to be OpDelete, but got %s", messenger.lastOperation.Type)
	}

	relPath, _ := filepath.Rel(engine.config.Sync.FolderPath, untrackedFilePath)
	if messenger.lastOperation.Path != relPath {
		t.Errorf("Expected operation path to be %s, but got %s", relPath, messenger.lastOperation.Path)
	}

	if messenger.lastOperation.FileID == "" {
		t.Error("Expected a FileID to be generated, but it was empty")
	}
}

// TestEngineVectorClockIncrement validates that the engine's vector clock is
// incremented when local operations are queued
func TestEngineVectorClockIncrement(t *testing.T) {
	engine, cleanup := setupTestEngine(t)
	defer cleanup()

	// Create first local operation
	op1 := NewSyncOperation(OpCreate, "file1.txt", "file-id-1", "test-peer")
	op1.Checksum = "checksum1"
	op1.Size = 100

	// Queue the operation
	engine.queueOperation(op1)

	// Verify vector clock was set and incremented
	if op1.VectorClock == nil {
		t.Fatal("Operation vector clock should not be nil after queueing")
	}
	if op1.VectorClock["test-peer"] != 1 {
		t.Errorf("Expected vector clock['test-peer'] = 1, got %d", op1.VectorClock["test-peer"])
	}

	// Create second local operation
	op2 := NewSyncOperation(OpUpdate, "file1.txt", "file-id-1", "test-peer")
	op2.Checksum = "checksum2"
	op2.Size = 150

	// Queue the second operation
	engine.queueOperation(op2)

	// Verify vector clock was incremented
	if op2.VectorClock["test-peer"] != 2 {
		t.Errorf("Expected vector clock['test-peer'] = 2, got %d", op2.VectorClock["test-peer"])
	}

	// Verify operations have causal relationship (not concurrent)
	if op1.VectorClock.IsConcurrent(op2.VectorClock) {
		t.Error("Sequential operations should NOT be concurrent")
	}

	// Verify op1 < op2
	if op1.VectorClock.Compare(op2.VectorClock) != -1 {
		t.Errorf("Expected op1 < op2, got comparison = %d", op1.VectorClock.Compare(op2.VectorClock))
	}
}

// TestEngineVectorClockMergeRemote validates that remote vector clocks are
// merged when processing incoming operations
func TestEngineVectorClockMergeRemote(t *testing.T) {
	engine, cleanup := setupTestEngine(t)
	defer cleanup()

	// Create a remote operation with its own vector clock
	remoteOp := NewSyncOperation(OpCreate, "remote-file.txt", "remote-file-id", "remote-peer")
	remoteOp.Checksum = "remote-checksum"
	remoteOp.Size = 200
	remoteOp.Source = "remote"

	// Set remote vector clock
	remoteOp.VectorClock = NewVectorClock()
	remoteOp.VectorClock.Set("remote-peer", 5)

	// Process the remote operation
	remoteData := []byte("remote file content")
	engine.HandleIncomingFile(remoteData, remoteOp)

	// Wait a bit for async processing
	time.Sleep(50 * time.Millisecond)

	// Verify engine's vector clock was merged
	engine.vectorClockMu.Lock()
	engineVC := engine.vectorClock
	engine.vectorClockMu.Unlock()

	if engineVC["remote-peer"] != 5 {
		t.Errorf("Expected engine vector clock['remote-peer'] = 5, got %d", engineVC["remote-peer"])
	}

	// Now create a local operation and verify it has both clocks
	localOp := NewSyncOperation(OpCreate, "local-file.txt", "local-file-id", "test-peer")
	localOp.Checksum = "local-checksum"
	localOp.Size = 100

	engine.queueOperation(localOp)

	// Verify local operation has both peer clocks
	if localOp.VectorClock["test-peer"] != 1 {
		t.Errorf("Expected local operation vector clock['test-peer'] = 1, got %d", localOp.VectorClock["test-peer"])
	}
	if localOp.VectorClock["remote-peer"] != 5 {
		t.Errorf("Expected local operation vector clock['remote-peer'] = 5 (from merge), got %d", localOp.VectorClock["remote-peer"])
	}
}

// TestNoConflictForSequentialEdits validates that sequential edits (non-concurrent)
// do not trigger conflict markers in files
func TestNoConflictForSequentialEdits(t *testing.T) {
	// This test validates that:
	// 1. When an operation has VC {peer-a: 1}
	// 2. And a later operation has VC {peer-a: 1, peer-b: 1}
	// 3. They are NOT concurrent (because peer-b includes peer-a's state)
	// 4. No conflict markers should be written

	// Simulate peer-a's first operation
	vc1 := NewVectorClock()
	vc1.Set("peer-a", 1)

	// Simulate peer-b's operation after receiving peer-a's
	vc2 := NewVectorClock()
	vc2.Set("peer-a", 1) // peer-b has seen peer-a's operation
	vc2.Set("peer-b", 1) // peer-b makes its own edit

	// Verify NOT concurrent
	if vc1.IsConcurrent(vc2) {
		t.Error("VC {peer-a:1} and VC {peer-a:1, peer-b:1} should NOT be concurrent")
		t.Errorf("vc1: %v", vc1)
		t.Errorf("vc2: %v", vc2)
	}

	// Verify causal relationship: vc1 < vc2
	comparison := vc1.Compare(vc2)
	if comparison != -1 {
		t.Errorf("Expected vc1 < vc2 (comparison = -1), got %d", comparison)
	}
}