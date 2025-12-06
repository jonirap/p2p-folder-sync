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