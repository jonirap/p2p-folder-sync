package system

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/p2p-folder-sync/p2p-sync/internal/config"
	"github.com/p2p-folder-sync/p2p-sync/internal/database"
	"github.com/p2p-folder-sync/p2p-sync/internal/sync"
)

// MockMessenger for testing operation replay
type MockMessenger struct {
	broadcastedOps []*sync.SyncOperation
	broadcastCount int
}

func (m *MockMessenger) SendFile(peerID string, fileData []byte, metadata *sync.SyncOperation) error {
	return nil
}

func (m *MockMessenger) BroadcastOperation(op *sync.SyncOperation) error {
	m.broadcastedOps = append(m.broadcastedOps, op)
	m.broadcastCount++
	return nil
}

func (m *MockMessenger) RequestStateSync(peerID string) error {
	return nil
}

func (m *MockMessenger) ConnectToPeer(peerID string, address string, port int) error {
	return nil
}

func TestOperationReplay_NoUnacknowledgedOperations(t *testing.T) {
	// Setup test environment
	tmpDir := t.TempDir()
	syncDir := filepath.Join(tmpDir, "sync")
	if err := os.MkdirAll(syncDir, 0755); err != nil {
		t.Fatalf("Failed to create sync directory: %v", err)
	}

	cfg := &config.Config{
		Sync: config.SyncConfig{
			FolderPath: syncDir,
		},
		Conflict: config.ConflictConfig{
			ResolutionStrategy: "last_write_wins",
		},
	}

	// Create database
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := database.NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create messenger mock
	messenger := &MockMessenger{}

	// Create engine
	engine, err := sync.NewEngineWithMessenger(cfg, db, "test-peer-1", messenger)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}
	defer engine.Stop()

	// Replay operations (should find none)
	ctx := context.Background()
	err = engine.ReplayUnacknowledgedOperations(ctx)
	if err != nil {
		t.Fatalf("Failed to replay operations: %v", err)
	}

	// Verify no operations were broadcasted
	if messenger.broadcastCount != 0 {
		t.Errorf("Expected 0 broadcasted operations, got %d", messenger.broadcastCount)
	}

	t.Logf("SUCCESS: No operations replayed when database is empty")
}

func TestOperationReplay_SingleUnacknowledgedOperation(t *testing.T) {
	// Setup test environment
	tmpDir := t.TempDir()
	syncDir := filepath.Join(tmpDir, "sync")
	if err := os.MkdirAll(syncDir, 0755); err != nil {
		t.Fatalf("Failed to create sync directory: %v", err)
	}

	cfg := &config.Config{
		Sync: config.SyncConfig{
			FolderPath: syncDir,
		},
		Conflict: config.ConflictConfig{
			ResolutionStrategy: "last_write_wins",
		},
	}

	// Create database
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := database.NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Insert an unacknowledged operation
	testPath := "test-file.txt"
	testFileID := "test-file-id-123"
	testChecksum := "abc123"
	testSize := int64(100)
	testPeerID := "test-peer-1"

	now := time.Now()
	entry := &database.LogEntry{
		OperationID:   "op-001",
		Timestamp:     now,
		OperationType: string(sync.OpCreate),
		PeerID:        testPeerID,
		VectorClock:   map[string]int64{testPeerID: 1},
		Acknowledged:  false, // Not acknowledged
		Persisted:     true,
		FileID:        &testFileID,
		Path:          testPath,
		Checksum:      &testChecksum,
		Size:          &testSize,
		Data:          []byte("test data"),
	}

	if err := db.AppendOperation(entry); err != nil {
		t.Fatalf("Failed to append operation: %v", err)
	}

	// Create messenger mock
	messenger := &MockMessenger{}

	// Create engine
	engine, err := sync.NewEngineWithMessenger(cfg, db, testPeerID, messenger)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}
	defer engine.Stop()

	// Replay operations
	ctx := context.Background()
	err = engine.ReplayUnacknowledgedOperations(ctx)
	if err != nil {
		t.Fatalf("Failed to replay operations: %v", err)
	}

	// Verify operation was broadcasted
	if messenger.broadcastCount != 1 {
		t.Errorf("Expected 1 broadcasted operation, got %d", messenger.broadcastCount)
	}

	if len(messenger.broadcastedOps) != 1 {
		t.Fatalf("Expected 1 operation in broadcastedOps, got %d", len(messenger.broadcastedOps))
	}

	// Verify operation details
	op := messenger.broadcastedOps[0]
	if op.ID != "op-001" {
		t.Errorf("Expected operation ID 'op-001', got '%s'", op.ID)
	}
	if op.Type != sync.OpCreate {
		t.Errorf("Expected operation type 'create', got '%s'", op.Type)
	}
	if op.Path != testPath {
		t.Errorf("Expected path '%s', got '%s'", testPath, op.Path)
	}

	t.Logf("SUCCESS: Single unacknowledged operation replayed correctly")
	t.Logf("Operation: ID=%s, Type=%s, Path=%s", op.ID, op.Type, op.Path)
}

func TestOperationReplay_MultipleUnacknowledgedOperations(t *testing.T) {
	// Setup test environment
	tmpDir := t.TempDir()
	syncDir := filepath.Join(tmpDir, "sync")
	if err := os.MkdirAll(syncDir, 0755); err != nil {
		t.Fatalf("Failed to create sync directory: %v", err)
	}

	cfg := &config.Config{
		Sync: config.SyncConfig{
			FolderPath: syncDir,
		},
		Conflict: config.ConflictConfig{
			ResolutionStrategy: "last_write_wins",
		},
	}

	// Create database
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := database.NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	testPeerID := "test-peer-1"

	// Insert multiple unacknowledged operations
	operations := []struct {
		id       string
		opType   sync.OperationType
		path     string
		fileID   string
		checksum string
	}{
		{"op-001", sync.OpCreate, "file1.txt", "fid-001", "hash-001"},
		{"op-002", sync.OpUpdate, "file2.txt", "fid-002", "hash-002"},
		{"op-003", sync.OpCreate, "file3.txt", "fid-003", "hash-003"},
	}

	for i, opData := range operations {
		testSize := int64(100 + i)
		entry := &database.LogEntry{
			OperationID:   opData.id,
			Timestamp:     time.Now().Add(time.Duration(i) * time.Second),
			OperationType: string(opData.opType),
			PeerID:        testPeerID,
			VectorClock:   map[string]int64{testPeerID: int64(i + 1)},
			Acknowledged:  false,
			Persisted:     true,
			FileID:        &opData.fileID,
			Path:          opData.path,
			Checksum:      &opData.checksum,
			Size:          &testSize,
			Data:          []byte("test data"),
		}

		if err := db.AppendOperation(entry); err != nil {
			t.Fatalf("Failed to append operation %s: %v", opData.id, err)
		}
	}

	// Create messenger mock
	messenger := &MockMessenger{}

	// Create engine
	engine, err := sync.NewEngineWithMessenger(cfg, db, testPeerID, messenger)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}
	defer engine.Stop()

	// Replay operations
	ctx := context.Background()
	err = engine.ReplayUnacknowledgedOperations(ctx)
	if err != nil {
		t.Fatalf("Failed to replay operations: %v", err)
	}

	// Verify all operations were broadcasted
	if messenger.broadcastCount != 3 {
		t.Errorf("Expected 3 broadcasted operations, got %d", messenger.broadcastCount)
	}

	if len(messenger.broadcastedOps) != 3 {
		t.Fatalf("Expected 3 operations in broadcastedOps, got %d", len(messenger.broadcastedOps))
	}

	// Verify operation order and details
	for i, op := range messenger.broadcastedOps {
		expectedOp := operations[i]
		if op.ID != expectedOp.id {
			t.Errorf("Operation %d: expected ID '%s', got '%s'", i, expectedOp.id, op.ID)
		}
		if op.Type != expectedOp.opType {
			t.Errorf("Operation %d: expected type '%s', got '%s'", i, expectedOp.opType, op.Type)
		}
		if op.Path != expectedOp.path {
			t.Errorf("Operation %d: expected path '%s', got '%s'", i, expectedOp.path, op.Path)
		}
		t.Logf("Operation %d replayed: ID=%s, Type=%s, Path=%s", i, op.ID, op.Type, op.Path)
	}

	t.Logf("SUCCESS: All %d unacknowledged operations replayed correctly", len(operations))
}

func TestOperationReplay_MixedAcknowledgedAndUnacknowledged(t *testing.T) {
	// Setup test environment
	tmpDir := t.TempDir()
	syncDir := filepath.Join(tmpDir, "sync")
	if err := os.MkdirAll(syncDir, 0755); err != nil {
		t.Fatalf("Failed to create sync directory: %v", err)
	}

	cfg := &config.Config{
		Sync: config.SyncConfig{
			FolderPath: syncDir,
		},
		Conflict: config.ConflictConfig{
			ResolutionStrategy: "last_write_wins",
		},
	}

	// Create database
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := database.NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	testPeerID := "test-peer-1"

	// Insert operations with mixed acknowledgment status
	operations := []struct {
		id           string
		path         string
		acknowledged bool
	}{
		{"op-001", "file1.txt", true},  // Acknowledged
		{"op-002", "file2.txt", false}, // Unacknowledged
		{"op-003", "file3.txt", true},  // Acknowledged
		{"op-004", "file4.txt", false}, // Unacknowledged
	}

	for i, opData := range operations {
		testFileID := "fid-" + opData.id
		testChecksum := "hash-" + opData.id
		testSize := int64(100 + i)

		entry := &database.LogEntry{
			OperationID:   opData.id,
			Timestamp:     time.Now().Add(time.Duration(i) * time.Second),
			OperationType: string(sync.OpCreate),
			PeerID:        testPeerID,
			VectorClock:   map[string]int64{testPeerID: int64(i + 1)},
			Acknowledged:  opData.acknowledged,
			Persisted:     true,
			FileID:        &testFileID,
			Path:          opData.path,
			Checksum:      &testChecksum,
			Size:          &testSize,
			Data:          []byte("test data"),
		}

		if err := db.AppendOperation(entry); err != nil {
			t.Fatalf("Failed to append operation %s: %v", opData.id, err)
		}
	}

	// Create messenger mock
	messenger := &MockMessenger{}

	// Create engine
	engine, err := sync.NewEngineWithMessenger(cfg, db, testPeerID, messenger)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}
	defer engine.Stop()

	// Replay operations
	ctx := context.Background()
	err = engine.ReplayUnacknowledgedOperations(ctx)
	if err != nil {
		t.Fatalf("Failed to replay operations: %v", err)
	}

	// Only unacknowledged operations should be replayed
	expectedReplayCount := 2
	if messenger.broadcastCount != expectedReplayCount {
		t.Errorf("Expected %d broadcasted operations, got %d", expectedReplayCount, messenger.broadcastCount)
	}

	// Verify only unacknowledged operations were replayed
	expectedIDs := []string{"op-002", "op-004"}
	for i, op := range messenger.broadcastedOps {
		if op.ID != expectedIDs[i] {
			t.Errorf("Expected operation ID '%s', got '%s'", expectedIDs[i], op.ID)
		}
	}

	t.Logf("SUCCESS: Only unacknowledged operations (%d) were replayed", expectedReplayCount)
}

func TestOperationReplay_RemoteOperations(t *testing.T) {
	// Setup test environment
	tmpDir := t.TempDir()
	syncDir := filepath.Join(tmpDir, "sync")
	if err := os.MkdirAll(syncDir, 0755); err != nil {
		t.Fatalf("Failed to create sync directory: %v", err)
	}

	cfg := &config.Config{
		Sync: config.SyncConfig{
			FolderPath: syncDir,
		},
		Conflict: config.ConflictConfig{
			ResolutionStrategy: "last_write_wins",
		},
	}

	// Create database
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := database.NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	localPeerID := "test-peer-1"
	remotePeerID := "remote-peer-2"

	// Insert unacknowledged operation from remote peer
	testFileID := "fid-remote"
	testChecksum := "hash-remote"
	testSize := int64(200)

	entry := &database.LogEntry{
		OperationID:   "op-remote-001",
		Timestamp:     time.Now(),
		OperationType: string(sync.OpCreate),
		PeerID:        remotePeerID, // From remote peer
		VectorClock:   map[string]int64{remotePeerID: 1},
		Acknowledged:  false,
		Persisted:     true,
		FileID:        &testFileID,
		Path:          "remote-file.txt",
		Checksum:      &testChecksum,
		Size:          &testSize,
		Data:          []byte("remote data"),
	}

	if err := db.AppendOperation(entry); err != nil {
		t.Fatalf("Failed to append operation: %v", err)
	}

	// Create messenger mock
	messenger := &MockMessenger{}

	// Create engine with local peer ID
	engine, err := sync.NewEngineWithMessenger(cfg, db, localPeerID, messenger)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}
	defer engine.Stop()

	// Replay operations
	ctx := context.Background()
	err = engine.ReplayUnacknowledgedOperations(ctx)
	if err != nil {
		t.Fatalf("Failed to replay operations: %v", err)
	}

	// Remote operations should not be rebroadcasted
	if messenger.broadcastCount != 0 {
		t.Errorf("Expected 0 broadcasted operations for remote peer, got %d", messenger.broadcastCount)
	}

	t.Logf("SUCCESS: Remote operations were not rebroadcasted")
}
