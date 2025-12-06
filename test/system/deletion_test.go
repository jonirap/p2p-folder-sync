//go:build integration

package system

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/p2p-folder-sync/p2p-sync/internal/database"
	"github.com/p2p-folder-sync/p2p-sync/internal/sync"
)

// TestFileDeletion tests that file deletion is propagated between peers
func TestFileDeletion(t *testing.T) {
	// Create temporary directories for two peers
	tmpDir := t.TempDir()
	peer1Dir := filepath.Join(tmpDir, "peer1")
	peer2Dir := filepath.Join(tmpDir, "peer2")

	if err := os.MkdirAll(peer1Dir, 0755); err != nil {
		t.Fatalf("Failed to create peer1 dir: %v", err)
	}
	if err := os.MkdirAll(peer2Dir, 0755); err != nil {
		t.Fatalf("Failed to create peer2 dir: %v", err)
	}

	// Create configurations
	cfg1 := createTestConfig(peer1Dir)
	cfg2 := createTestConfig(peer2Dir)

	// Initialize databases
	db1, err := database.NewDB(filepath.Join(peer1Dir, ".p2p-sync", "peer1.db"))
	if err != nil {
		t.Fatalf("Failed to create peer1 DB: %v", err)
	}
	defer db1.Close()

	db2, err := database.NewDB(filepath.Join(peer2Dir, ".p2p-sync", "peer2.db"))
	if err != nil {
		t.Fatalf("Failed to create peer2 DB: %v", err)
	}
	defer db2.Close()

	// Create in-memory messenger for testing with operation monitoring
	messenger := sync.NewInMemoryMessenger()
	opWaiter1 := NewSyncOperationWaiter()
	opWaiter2 := NewSyncOperationWaiter()

	// Wrap the messenger to intercept operations
	wrappedMessenger := &OperationMonitoringMessenger{
		innerMessenger: messenger,
		peer1Waiter:    opWaiter1,
		peer2Waiter:    opWaiter2,
	}

	// Initialize sync engines
	syncEngine1, err := sync.NewEngineWithMessenger(cfg1, db1, "peer1", wrappedMessenger)
	if err != nil {
		t.Fatalf("Failed to create peer1 sync engine: %v", err)
	}
	messenger.RegisterEngine("peer1", syncEngine1)

	syncEngine2, err := sync.NewEngineWithMessenger(cfg2, db2, "peer2", wrappedMessenger)
	if err != nil {
		t.Fatalf("Failed to create peer2 sync engine: %v", err)
	}
	messenger.RegisterEngine("peer2", syncEngine2)

	// Start all components
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start sync engines
	if err := syncEngine1.Start(ctx); err != nil {
		t.Fatalf("Failed to start peer1 sync engine: %v", err)
	}
	defer syncEngine1.Stop()

	if err := syncEngine2.Start(ctx); err != nil {
		t.Fatalf("Failed to start peer2 sync engine: %v", err)
	}
	defer syncEngine2.Stop()

	// Wait for engines to initialize
	time.Sleep(500 * time.Millisecond)

	// Step 1: Create a file on peer1
	testFile := filepath.Join(peer1Dir, "test_file.txt")
	testContent := []byte("This file will be deleted")

	t.Log("Step 1: Creating file on peer1...")
	if err := ioutil.WriteFile(testFile, testContent, 0644); err != nil {
		t.Fatalf("Failed to create test file on peer1: %v", err)
	}

	// Wait for creation to be broadcast
	t.Log("Waiting for create operation to be broadcast from peer1...")
	createOp, err := opWaiter1.WaitForOperationType(sync.OpCreate, 3*time.Second)
	if err != nil {
		t.Fatalf("Failed to detect create operation broadcast: %v", err)
	}
	t.Logf("SUCCESS: Create operation broadcast - FileID: %s, Path: %s", createOp.FileID, createOp.Path)

	// Wait for file to sync to peer2 (give it some time)
	time.Sleep(1 * time.Second)

	// Verify file exists on peer2
	peer2File := filepath.Join(peer2Dir, "test_file.txt")
	if _, err := os.Stat(peer2File); os.IsNotExist(err) {
		t.Fatalf("File did not sync to peer2")
	}
	t.Log("SUCCESS: File synced to peer2")

	// Step 2: Delete the file on peer1
	t.Log("\nStep 2: Deleting file on peer1...")
	if err := os.Remove(testFile); err != nil {
		t.Fatalf("Failed to delete file on peer1: %v", err)
	}

	// Wait for delete operation to be broadcast
	t.Log("Waiting for delete operation to be broadcast from peer1...")
	deleteOp, err := opWaiter1.WaitForOperationType(sync.OpDelete, 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to detect delete operation broadcast: %v", err)
	}
	t.Logf("SUCCESS: Delete operation broadcast - FileID: %s, Path: %s", deleteOp.FileID, deleteOp.Path)

	// Wait for deletion to propagate
	time.Sleep(2 * time.Second)

	// Step 3: Verify file is deleted on peer2
	t.Log("Verifying file deletion on peer2...")
	if _, err := os.Stat(peer2File); !os.IsNotExist(err) {
		t.Errorf("FAILURE: File still exists on peer2 after deletion")
		// List files on peer2 for debugging
		files, _ := ioutil.ReadDir(peer2Dir)
		t.Log("Files on peer2:")
		for _, f := range files {
			t.Logf("  - %s", f.Name())
		}
	} else {
		t.Log("SUCCESS: File deleted on peer2")
	}

	// Verify file is deleted from database on peer2
	_, err = db2.GetFileByPath("test_file.txt")
	if err == nil {
		t.Error("FAILURE: File still in peer2 database after deletion")
	} else {
		t.Log("SUCCESS: File removed from peer2 database")
	}
}
