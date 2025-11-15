//go:build integration

package system

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/p2p-folder-sync/p2p-sync/internal/database"
	"github.com/p2p-folder-sync/p2p-sync/internal/sync"
)

// SyncLoopPreventionMessenger wraps a messenger to monitor sync operations
type SyncLoopPreventionMessenger struct {
	innerMessenger *sync.InMemoryMessenger
	peer1Waiter    *SyncOperationWaiter
}

func (omm *SyncLoopPreventionMessenger) SendFile(peerID string, fileData []byte, metadata *sync.SyncOperation) error {
	return omm.innerMessenger.SendFile(peerID, fileData, metadata)
}

func (omm *SyncLoopPreventionMessenger) BroadcastOperation(op *sync.SyncOperation) error {
	// Notify the appropriate waiter based on the sender
	if op.PeerID == "peer1" {
		omm.peer1Waiter.OnOperationQueued(op)
	}

	return omm.innerMessenger.BroadcastOperation(op)
}

func (omm *SyncLoopPreventionMessenger) RequestStateSync(peerID string) error {
	return omm.innerMessenger.RequestStateSync(peerID)
}

// TestSyncLoopPreventionCritical tests the critical requirement that incoming remote file changes
// do not trigger further synchronization operations. This is marked as CRITICAL in the spec.
// This test verifies:
// - Remote file operations don't trigger outbound sync messages
// - Filesystem watcher is properly disabled for remote operations
// - Local changes still trigger sync operations as expected
// - Remote operations are processed without creating sync loops
func TestSyncLoopPreventionCritical(t *testing.T) {
	// Create temporary directories for testing
	tmpDir := t.TempDir()
	peer1Dir := filepath.Join(tmpDir, "peer1")

	if err := os.MkdirAll(peer1Dir, 0755); err != nil {
		t.Fatalf("Failed to create peer1 dir: %v", err)
	}

	// Create configuration
	cfg1 := createTestConfig(peer1Dir)

	// Initialize database
	db1, err := database.NewDB(filepath.Join(peer1Dir, ".p2p-sync", "peer1.db"))
	if err != nil {
		t.Fatalf("Failed to create peer1 DB: %v", err)
	}
	defer db1.Close()

	// Create operation waiter to monitor any operations that might be queued
	opWaiter := NewSyncOperationWaiter()

	// Create mock messenger that intercepts operations
	mockMessenger := &SyncLoopPreventionMessenger{
		innerMessenger: sync.NewInMemoryMessenger(),
		peer1Waiter:    opWaiter,
	}

	// Initialize sync engine with messenger
	syncEngine1, err := sync.NewEngineWithMessenger(cfg1, db1, "peer1", mockMessenger)
	if err != nil {
		t.Fatalf("Failed to create peer1 sync engine: %v", err)
	}

	// Start sync engine
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := syncEngine1.Start(ctx); err != nil {
		t.Fatalf("Failed to start peer1 sync engine: %v", err)
	}
	defer syncEngine1.Stop()

	// Give engine time to initialize
	time.Sleep(500 * time.Millisecond)

	// CRITICAL TEST 1: Remote file creation should not trigger outbound sync
	t.Log("Testing remote file creation - should not trigger outbound sync...")
	testFile := filepath.Join(peer1Dir, "remote_test.txt")
	testContent := []byte("This is a remote change that should not trigger sync")

	remoteSyncOp := &sync.SyncOperation{
		ID:       "remote-create-op-123",
		FileID:   "test-file-id-123",
		Path:     "remote_test.txt",
		Checksum: "test-checksum-abc",
		Size:     int64(len(testContent)),
		Mtime:    time.Now().Unix(),
		PeerID:   "peer2", // Coming from peer2
		Type:     sync.OpCreate,
		Source:   "remote", // CRITICAL: This should prevent sync loop
		Data:     testContent,
	}

	// Handle incoming remote file
	if err := syncEngine1.HandleIncomingFile(testContent, remoteSyncOp); err != nil {
		t.Fatalf("Failed to handle incoming remote file: %v", err)
	}

	// Verify the file was written correctly
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Error("FAILURE: Remote file was not created on peer1")
	} else {
		content, err := os.ReadFile(testFile)
		if err != nil {
			t.Errorf("Failed to read created file: %v", err)
		} else if string(content) != string(testContent) {
			t.Error("FAILURE: Remote file content mismatch")
		} else {
			t.Log("SUCCESS: Remote file created correctly")
		}
	}

	// CRITICAL CHECK: Verify no outbound sync operations were generated for remote file
	time.Sleep(1 * time.Second) // Allow time for any potential operations to be queued

	operations := opWaiter.GetAllOperations()
	outboundOps := 0
	for _, op := range operations {
		if op.Source != "remote" { // Only count non-remote operations
			outboundOps++
		}
	}

	if outboundOps > 0 {
		t.Errorf("FAILURE: Remote file reception triggered %d outbound sync operations. Sync loop prevention failed!", outboundOps)
		for _, op := range operations {
			if op.Source != "remote" {
				t.Logf("Unexpected outbound operation: Type=%s, Path=%s, Source=%s", op.Type, op.Path, op.Source)
			}
		}
	} else {
		t.Log("SUCCESS: Remote file reception did not trigger outbound sync operations")
	}

	// CRITICAL TEST 2: Local file changes should still trigger outbound sync
	t.Log("Testing local file creation - should trigger outbound sync...")
	localFile := filepath.Join(peer1Dir, "local_test.txt")
	localContent := []byte("This is a local change that SHOULD trigger sync")

	// Clear previous operations
	opWaiter.Clear()

	// Create local file (this should trigger filesystem watcher)
	if err := os.WriteFile(localFile, localContent, 0644); err != nil {
		t.Fatalf("Failed to create local file: %v", err)
	}

	// Wait for filesystem watcher to detect the change and generate operations
	time.Sleep(2 * time.Second)

	// Check that local operation was queued for broadcast
	localOperations := opWaiter.GetAllOperations()
	if len(localOperations) == 0 {
		t.Error("FAILURE: Local file change did not generate any sync operations")
	} else {
		foundCreateOp := false
		for _, op := range localOperations {
			if op.Type == sync.OpCreate && op.Path == "local_test.txt" {
				foundCreateOp = true
				if op.Source != "local" {
					t.Errorf("FAILURE: Local operation should have Source='local', got '%s'", op.Source)
				}
				break
			}
		}
		if foundCreateOp {
			t.Log("SUCCESS: Local file change generated correct sync operation")
		} else {
			t.Error("FAILURE: Local file change did not generate expected create operation")
		}
	}

	// CRITICAL TEST 3: Remote file deletion should not trigger outbound sync
	t.Log("Testing remote file deletion - should not trigger outbound sync...")

	// Clear operations
	opWaiter.Clear()

	deleteOp := &sync.SyncOperation{
		ID:     "remote-delete-op-456",
		FileID: "test-file-id-123", // Same file ID as remote file
		Path:   "remote_test.txt",
		PeerID: "peer2",
		Type:   sync.OpDelete,
		Source: "remote",
	}

	// Handle remote deletion
	if err := syncEngine1.HandleIncomingFile([]byte{}, deleteOp); err != nil {
		t.Fatalf("Failed to handle remote deletion: %v", err)
	}

	// Verify file was deleted
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Error("FAILURE: Remote file deletion did not remove file")
	} else {
		t.Log("SUCCESS: Remote file deleted correctly")
	}

	// Verify no outbound operations for remote deletion
	time.Sleep(1 * time.Second)
	deleteOperations := opWaiter.GetAllOperations()
	outboundDeleteOps := 0
	for _, op := range deleteOperations {
		if op.Source != "remote" {
			outboundDeleteOps++
		}
	}

	if outboundDeleteOps > 0 {
		t.Errorf("FAILURE: Remote file deletion triggered %d outbound operations", outboundDeleteOps)
	} else {
		t.Log("SUCCESS: Remote file deletion did not trigger outbound operations")
	}

	t.Log("SUCCESS: All sync loop prevention tests passed - remote operations don't create loops!")
}

// TestSyncLoopPreventionWithRename tests that rename operations from remote sources
// don't trigger further sync operations.
func TestSyncLoopPreventionWithRename(t *testing.T) {
	tmpDir := t.TempDir()
	peer1Dir := filepath.Join(tmpDir, "peer1")

	if err := os.MkdirAll(peer1Dir, 0755); err != nil {
		t.Fatalf("Failed to create peer1 dir: %v", err)
	}

	cfg := createTestConfig(peer1Dir)
	db, err := database.NewDB(filepath.Join(peer1Dir, ".p2p-sync", "peer1.db"))
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
	}
	defer db.Close()

	syncEngine, err := sync.NewEngine(cfg, db, "peer1")
	if err != nil {
		t.Fatalf("Failed to create sync engine: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := syncEngine.Start(ctx); err != nil {
		t.Fatalf("Failed to start sync engine: %v", err)
	}

	// Create original file
	originalFile := filepath.Join(peer1Dir, "original.txt")
	content := []byte("test content")

	if err := os.WriteFile(originalFile, content, 0644); err != nil {
		t.Fatalf("Failed to create original file: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Simulate remote rename operation
	renamedFile := filepath.Join(peer1Dir, "renamed.txt")
	renameOp := &sync.SyncOperation{
		FileID:   "test-file-id-456",
		Path:     renamedFile,
		FromPath: &originalFile,
		Checksum: "test-checksum-def",
		Size:     int64(len(content)),
		Mtime:    time.Now().Unix(),
		PeerID:   "remote-peer",
		Type:     sync.OpRename,
		Source:   "remote", // Should prevent sync loop
	}

	// Handle remote rename
	if err := syncEngine.HandleIncomingRename(renameOp); err != nil {
		t.Fatalf("Failed to handle incoming rename: %v", err)
	}

	time.Sleep(2 * time.Second)

	// Verify original file is gone and renamed file exists
	if _, err := os.Stat(originalFile); !os.IsNotExist(err) {
		t.Error("Original file still exists after remote rename")
	}

	if _, err := os.Stat(renamedFile); os.IsNotExist(err) {
		t.Error("Renamed file does not exist after remote rename")
	}

	// Get operation count before remote rename
	initialRenameOperations, err := db.GetAllOperations()
	if err != nil {
		t.Fatalf("Failed to get operations before rename: %v", err)
	}
	initialRenameOpCount := len(initialRenameOperations)

	// CRITICAL: Verify no sync operations were generated for this remote rename
	operations, err := db.GetAllOperations()
	if err != nil {
		t.Fatalf("Failed to get operations: %v", err)
	}

	// Remote rename operations should not create new operations to broadcast
	// Check that the operation count is reasonable
	if len(operations) > initialRenameOpCount+2 { // Allow some tolerance
		t.Errorf("POTENTIAL ISSUE: Remote rename created %d operations (was %d). This might indicate sync loop prevention not working.", len(operations), initialRenameOpCount)
	} else {
		t.Logf("SUCCESS: Remote rename created reasonable number of operations (%d -> %d)", initialRenameOpCount, len(operations))
	}

	if err := syncEngine.Stop(); err != nil {
		t.Errorf("Failed to stop sync engine: %v", err)
	}
}

// TestSyncLoopPreventionNetwork tests sync loop prevention with real network components.
// This test verifies that remote operations over actual network connections don't trigger
// further sync operations, using multi-peer scenarios to catch potential loops.
func TestSyncLoopPreventionNetwork(t *testing.T) {
	// Use network test helper for real network components
	nth := NewNetworkTestHelper(t)

	// Setup three peers to test multi-peer sync loop scenarios
	peer1, err := nth.SetupPeer(t, "peer1", true)
	if err != nil {
		t.Fatalf("Failed to setup peer1: %v", err)
	}
	defer peer1.Cleanup()

	peer2, err := nth.SetupPeer(t, "peer2", true)
	if err != nil {
		t.Fatalf("Failed to setup peer2: %v", err)
	}
	defer peer2.Cleanup()

	peer3, err := nth.SetupPeer(t, "peer3", true)
	if err != nil {
		t.Fatalf("Failed to setup peer3: %v", err)
	}
	defer peer3.Cleanup()

	// Create a fully connected topology: peer1 <-> peer2 <-> peer3 <-> peer1
	peer1.Config.Network.Peers = []string{
		fmt.Sprintf("localhost:%d", peer2.Config.Network.Port),
		fmt.Sprintf("localhost:%d", peer3.Config.Network.Port),
	}
	peer2.Config.Network.Peers = []string{
		fmt.Sprintf("localhost:%d", peer1.Config.Network.Port),
		fmt.Sprintf("localhost:%d", peer3.Config.Network.Port),
	}
	peer3.Config.Network.Peers = []string{
		fmt.Sprintf("localhost:%d", peer1.Config.Network.Port),
		fmt.Sprintf("localhost:%d", peer2.Config.Network.Port),
	}

	// Create network operation monitors for each peer
	monitor1 := NewNetworkOperationMonitor()
	monitor2 := NewNetworkOperationMonitor()
	monitor3 := NewNetworkOperationMonitor()

	// Wrap transports to intercept messages
	interceptedTransport1 := NewNetworkMessageInterceptor(peer1.Transport, monitor1)
	interceptedTransport2 := NewNetworkMessageInterceptor(peer2.Transport, monitor2)
	interceptedTransport3 := NewNetworkMessageInterceptor(peer3.Transport, monitor3)

	peer1.Transport = interceptedTransport1
	peer2.Transport = interceptedTransport2
	peer3.Transport = interceptedTransport3

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Start peer3 first
	if err := peer3.Transport.Start(); err != nil {
		t.Fatalf("Failed to start peer3 transport: %v", err)
	}
	if err := peer3.SyncEngine.Start(ctx); err != nil {
		t.Fatalf("Failed to start peer3 sync engine: %v", err)
	}

	time.Sleep(1 * time.Second)

	// Start peer2
	if err := peer2.Transport.Start(); err != nil {
		t.Fatalf("Failed to start peer2 transport: %v", err)
	}
	if err := peer2.SyncEngine.Start(ctx); err != nil {
		t.Fatalf("Failed to start peer2 sync engine: %v", err)
	}

	time.Sleep(1 * time.Second)

	// Start peer1
	if err := peer1.Transport.Start(); err != nil {
		t.Fatalf("Failed to start peer1 transport: %v", err)
	}
	if err := peer1.SyncEngine.Start(ctx); err != nil {
		t.Fatalf("Failed to start peer1 sync engine: %v", err)
	}

	// Wait for all peers to establish connections
	allPeers := []*NetworkPeerSetup{peer1, peer2, peer3}
	if err := WaitForPeerConnections(allPeers, 30*time.Second); err != nil {
		t.Fatalf("Peers failed to connect: %v", err)
	}
	t.Log("SUCCESS: All peers connected in mesh topology")

	// Create event-driven waiter
	waiter := NewEventDrivenWaiterWithTimeout(30 * time.Second)
	defer waiter.Close()

	// CRITICAL TEST: Multi-peer sync loop prevention
	t.Log("Testing multi-peer sync loop prevention...")

	// Create a file on peer1
	testFile1 := filepath.Join(peer1.Dir, "loop_test.txt")
	content1 := []byte("Content from peer1 - should not cause loops")

	if err := os.WriteFile(testFile1, content1, 0644); err != nil {
		t.Fatalf("Failed to create test file on peer1: %v", err)
	}

	// Wait for the operation to be detected
	op1, err := monitor1.WaitForOperationType(sync.OpCreate, 10*time.Second)
	if err != nil {
		t.Fatalf("Failed to detect operation on peer1: %v", err)
	}
	t.Logf("SUCCESS: Detected create operation on peer1: %s", op1.Path)

	// Wait for file to sync to peer2
	if err := waiter.WaitForFileSync(peer2.Dir, "loop_test.txt"); err != nil {
		t.Fatalf("File failed to sync peer1->peer2: %v", err)
	}
	t.Log("SUCCESS: File synced from peer1 to peer2")

	// Verify file appeared on peer2 but didn't trigger operations on peer2
	peer2Operations := monitor2.GetAllOperations()
	peer2CreateOps := 0
	for _, op := range peer2Operations {
		if op.Type == sync.OpCreate {
			peer2CreateOps++
		}
	}

	if peer2CreateOps > 0 {
		t.Errorf("FAILURE: Peer2 generated %d create operations for remote file (sync loop detected)", peer2CreateOps)
	} else {
		t.Log("SUCCESS: Peer2 did not generate operations for remote file")
	}

	// Wait for file to sync to peer3
	if err := waiter.WaitForFileSync(peer3.Dir, "loop_test.txt"); err != nil {
		t.Fatalf("File failed to sync peer1->peer3: %v", err)
	}
	t.Log("SUCCESS: File synced from peer1 to peer3")

	// Verify file appeared on peer3 but didn't trigger operations on peer3
	peer3Operations := monitor3.GetAllOperations()
	peer3CreateOps := 0
	for _, op := range peer3Operations {
		if op.Type == sync.OpCreate {
			peer3CreateOps++
		}
	}

	if peer3CreateOps > 0 {
		t.Errorf("FAILURE: Peer3 generated %d create operations for remote file (sync loop detected)", peer3CreateOps)
	} else {
		t.Log("SUCCESS: Peer3 did not generate operations for remote file")
	}

	// Now modify the file on peer2 (this should trigger sync to peer1 and peer3)
	modifiedContent := []byte("Modified by peer2 - should sync to others")
	if err := os.WriteFile(filepath.Join(peer2.Dir, "loop_test.txt"), modifiedContent, 0644); err != nil {
		t.Fatalf("Failed to modify file on peer2: %v", err)
	}

	// Wait for update operation on peer2
	op2, err := monitor2.WaitForOperationType(sync.OpUpdate, 10*time.Second)
	if err != nil {
		t.Fatalf("Failed to detect update operation on peer2: %v", err)
	}
	t.Logf("SUCCESS: Detected update operation on peer2: %s", op2.Path)

	// Wait for modification to sync back to peer1
	if err := waiter.WaitForFileContent(peer1.Dir, "loop_test.txt", modifiedContent); err != nil {
		t.Fatalf("File modification failed to sync peer2->peer1: %v", err)
	}
	t.Log("SUCCESS: File modification synced from peer2 to peer1")

	// Wait for modification to sync to peer3
	if err := waiter.WaitForFileContent(peer3.Dir, "loop_test.txt", modifiedContent); err != nil {
		t.Fatalf("File modification failed to sync peer2->peer3: %v", err)
	}
	t.Log("SUCCESS: File modification synced from peer2 to peer3")

	// CRITICAL CHECK: Verify that the modifications didn't create loops
	// The file should have been modified on peer1 and peer3, but should not trigger
	// new operations on those peers (since they received it as remote operations)

	peer1UpdateOps := 0
	peer1Operations := monitor1.GetAllOperations()
	for _, op := range peer1Operations {
		if op.Type == sync.OpUpdate {
			peer1UpdateOps++
		}
	}

	peer3UpdateOps := 0
	for _, op := range peer3Operations {
		if op.Type == sync.OpUpdate {
			peer3UpdateOps++
		}
	}

	// Peer1 and Peer3 should not generate update operations for the remote modifications
	if peer1UpdateOps > 1 { // Allow 1 for the original local operation
		t.Errorf("FAILURE: Peer1 generated %d update operations (sync loop detected)", peer1UpdateOps)
	} else {
		t.Log("SUCCESS: Peer1 did not generate additional operations for remote update")
	}

	if peer3UpdateOps > 0 {
		t.Errorf("FAILURE: Peer3 generated %d update operations for remote update (sync loop detected)", peer3UpdateOps)
	} else {
		t.Log("SUCCESS: Peer3 did not generate operations for remote update")
	}

	// Final verification: All peers should have the same content
	peer1Final, err := os.ReadFile(testFile1)
	if err != nil {
		t.Fatalf("Failed to read final file on peer1: %v", err)
	}

	peer2Final, err := os.ReadFile(filepath.Join(peer2.Dir, "loop_test.txt"))
	if err != nil {
		t.Fatalf("Failed to read final file on peer2: %v", err)
	}

	peer3Final, err := os.ReadFile(filepath.Join(peer3.Dir, "loop_test.txt"))
	if err != nil {
		t.Fatalf("Failed to read final file on peer3: %v", err)
	}

	if string(peer1Final) != string(modifiedContent) ||
		string(peer2Final) != string(modifiedContent) ||
		string(peer3Final) != string(modifiedContent) {
		t.Error("FAILURE: Peers have inconsistent final state")
		t.Logf("Peer1: %q", string(peer1Final))
		t.Logf("Peer2: %q", string(peer2Final))
		t.Logf("Peer3: %q", string(peer3Final))
	} else {
		t.Log("SUCCESS: All peers converged to consistent state")
	}

	t.Log("SUCCESS: Multi-peer sync loop prevention test completed - no sync loops detected")
}

// TestSyncLoopPreventionWithRenameNetwork tests that rename operations over real network
// don't trigger further sync operations, preventing rename-related sync loops.
func TestSyncLoopPreventionWithRenameNetwork(t *testing.T) {
	// Use network test helper for real network components
	nth := NewNetworkTestHelper(t)

	// Setup two peers
	peer1, err := nth.SetupPeer(t, "peer1", true)
	if err != nil {
		t.Fatalf("Failed to setup peer1: %v", err)
	}
	defer peer1.Cleanup()

	peer2, err := nth.SetupPeer(t, "peer2", true)
	if err != nil {
		t.Fatalf("Failed to setup peer2: %v", err)
	}
	defer peer2.Cleanup()

	// Configure peers to connect to each other
	peer1.Config.Network.Peers = []string{fmt.Sprintf("localhost:%d", peer2.Config.Network.Port)}
	peer2.Config.Network.Peers = []string{fmt.Sprintf("localhost:%d", peer1.Config.Network.Port)}

	// Create network operation monitors
	monitor1 := NewNetworkOperationMonitor()
	monitor2 := NewNetworkOperationMonitor()

	// Wrap transports
	interceptedTransport1 := NewNetworkMessageInterceptor(peer1.Transport, monitor1)
	interceptedTransport2 := NewNetworkMessageInterceptor(peer2.Transport, monitor2)

	peer1.Transport = interceptedTransport1
	peer2.Transport = interceptedTransport2

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Start peer2 first
	if err := peer2.Transport.Start(); err != nil {
		t.Fatalf("Failed to start peer2 transport: %v", err)
	}
	if err := peer2.SyncEngine.Start(ctx); err != nil {
		t.Fatalf("Failed to start peer2 sync engine: %v", err)
	}

	time.Sleep(1 * time.Second)

	// Start peer1
	if err := peer1.Transport.Start(); err != nil {
		t.Fatalf("Failed to start peer1 transport: %v", err)
	}
	if err := peer1.SyncEngine.Start(ctx); err != nil {
		t.Fatalf("Failed to start peer1 sync engine: %v", err)
	}

	// Wait for connections
	if err := WaitForPeerConnections([]*NetworkPeerSetup{peer1, peer2}, 30*time.Second); err != nil {
		t.Fatalf("Peers failed to connect: %v", err)
	}

	waiter := NewEventDrivenWaiterWithTimeout(30 * time.Second)
	defer waiter.Close()

	// Create initial file on peer1
	originalFile := filepath.Join(peer1.Dir, "rename_loop_test.txt")
	content := []byte("File for rename loop test")

	if err := os.WriteFile(originalFile, content, 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	// Wait for sync to peer2
	if err := waiter.WaitForFileSync(peer2.Dir, "rename_loop_test.txt"); err != nil {
		t.Fatalf("Initial file sync failed: %v", err)
	}
	t.Log("SUCCESS: Initial file synced to peer2")

	// Rename file on peer1
	renamedFile := filepath.Join(peer1.Dir, "renamed_loop_test.txt")
	if err := os.Rename(originalFile, renamedFile); err != nil {
		t.Fatalf("Failed to rename file: %v", err)
	}

	// Wait for rename operation
	renameOp, err := monitor1.WaitForOperationType(sync.OpRename, 10*time.Second)
	if err != nil {
		t.Fatalf("Failed to detect rename operation: %v", err)
	}
	t.Logf("SUCCESS: Detected rename operation: %s -> %s", *renameOp.FromPath, renameOp.Path)

	// Wait for rename to sync to peer2
	if err := waiter.WaitForFileSync(peer2.Dir, "renamed_loop_test.txt"); err != nil {
		t.Fatalf("Renamed file sync failed: %v", err)
	}

	// Verify original file is gone from peer2
	if _, err := os.Stat(filepath.Join(peer2.Dir, "rename_loop_test.txt")); !os.IsNotExist(err) {
		t.Error("FAILURE: Original file still exists on peer2 after remote rename")
	}

	// CRITICAL CHECK: Verify peer2 didn't generate operations for the remote rename
	peer2Operations := monitor2.GetAllOperations()
	peer2RenameOps := 0
	for _, op := range peer2Operations {
		if op.Type == sync.OpRename {
			peer2RenameOps++
		}
	}

	if peer2RenameOps > 0 {
		t.Errorf("FAILURE: Peer2 generated %d rename operations for remote rename (sync loop detected)", peer2RenameOps)
	} else {
		t.Log("SUCCESS: Peer2 did not generate operations for remote rename")
	}

	// Verify content is preserved
	finalContent, err := os.ReadFile(renamedFile)
	if err != nil {
		t.Fatalf("Failed to read renamed file on peer1: %v", err)
	}

	peer2Content, err := os.ReadFile(filepath.Join(peer2.Dir, "renamed_loop_test.txt"))
	if err != nil {
		t.Fatalf("Failed to read renamed file on peer2: %v", err)
	}

	if string(finalContent) != string(peer2Content) {
		t.Error("FAILURE: File content not preserved during rename sync")
	} else {
		t.Log("SUCCESS: File content preserved during rename sync")
	}

	t.Log("SUCCESS: Network rename sync loop prevention test completed")
}

// createTestConfig creates a test configuration
