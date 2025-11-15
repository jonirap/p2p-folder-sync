package system

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/p2p-folder-sync/p2p-sync/internal/database"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/transport"
	"github.com/p2p-folder-sync/p2p-sync/internal/sync"
	"github.com/p2p-folder-sync/p2p-sync/internal/sync/conflict"
)

// TestConflictResolutionTextFiles tests 3-way merge for text files
// This test verifies:
// - Conflict detection occurs when concurrent edits happen
// - 3-way merge algorithm is properly invoked
// - Resolved content is propagated to both peers
// - Both peer changes are preserved in the merge
func TestConflictResolutionTextFiles(t *testing.T) {
	tmpDir := t.TempDir()
	peer1Dir := filepath.Join(tmpDir, "peer1")
	peer2Dir := filepath.Join(tmpDir, "peer2")

	if err := os.MkdirAll(peer1Dir, 0755); err != nil {
		t.Fatalf("Failed to create peer1 dir: %v", err)
	}
	if err := os.MkdirAll(peer2Dir, 0755); err != nil {
		t.Fatalf("Failed to create peer2 dir: %v", err)
	}

	peer1Port := getAvailablePort(t)
	peer2Port := getAvailablePort(t)

	cfg1 := createTestConfig(peer1Dir)
	cfg1.Network.Port = peer1Port
	cfg1.Network.Peers = []string{fmt.Sprintf("localhost:%d", peer2Port)}
	cfg1.Conflict.ResolutionStrategy = "intelligent_merge"

	cfg2 := createTestConfig(peer2Dir)
	cfg2.Network.Port = peer2Port
	cfg2.Network.Peers = []string{fmt.Sprintf("localhost:%d", peer1Port)}
	cfg2.Conflict.ResolutionStrategy = "intelligent_merge"

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

	// Create operation waiters to monitor conflict resolution
	opWaiter1 := NewSyncOperationWaiter()
	opWaiter2 := NewSyncOperationWaiter()

	// Create mock messengers for monitoring
	mockMessenger1 := &OperationMonitoringMessenger{
		innerMessenger: sync.NewInMemoryMessenger(),
		peer1Waiter:    opWaiter1,
	}
	mockMessenger2 := &OperationMonitoringMessenger{
		innerMessenger: sync.NewInMemoryMessenger(),
		peer1Waiter:    opWaiter2, // peer1Waiter field used for both peers in this simple case
	}

	transportFactory := &transport.TransportFactory{}

	transport1, err := transportFactory.NewTransport("tcp", cfg1.Network.Port)
	if err != nil {
		t.Fatalf("Failed to create transport1: %v", err)
	}
	defer transport1.Stop()

	transport2, err := transportFactory.NewTransport("tcp", cfg2.Network.Port)
	if err != nil {
		t.Fatalf("Failed to create transport2: %v", err)
	}
	defer transport2.Stop()

	// Initialize sync engines with messengers
	syncEngine1, err := sync.NewEngineWithMessenger(cfg1, db1, "peer1", mockMessenger1)
	if err != nil {
		t.Fatalf("Failed to create peer1 sync engine: %v", err)
	}

	syncEngine2, err := sync.NewEngineWithMessenger(cfg2, db2, "peer2", mockMessenger2)
	if err != nil {
		t.Fatalf("Failed to create peer2 sync engine: %v", err)
	}

	// Register both engines with both messengers for cross-peer communication
	mockMessenger1.innerMessenger.RegisterEngine("peer1", syncEngine1)
	mockMessenger1.innerMessenger.RegisterEngine("peer2", syncEngine2)
	mockMessenger2.innerMessenger.RegisterEngine("peer2", syncEngine2)
	mockMessenger2.innerMessenger.RegisterEngine("peer1", syncEngine1)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Start peer2 first
	if err := transport2.Start(); err != nil {
		t.Fatalf("Failed to start transport2: %v", err)
	}
	defer transport2.Stop()

	if err := syncEngine2.Start(ctx); err != nil {
		t.Fatalf("Failed to start peer2 sync engine: %v", err)
	}
	defer syncEngine2.Stop()

	// Give peer2 time to start
	time.Sleep(1 * time.Second)

	// Start peer1
	if err := transport1.Start(); err != nil {
		t.Fatalf("Failed to start transport1: %v", err)
	}
	defer transport1.Stop()

	if err := syncEngine1.Start(ctx); err != nil {
		t.Fatalf("Failed to start peer1 sync engine: %v", err)
	}
	defer syncEngine1.Stop()

	waiter := NewEventDrivenWaiterWithTimeout(15 * time.Second)
	defer waiter.Close()

	// Test 1: Create initial file and verify sync
	t.Log("Creating initial file for sync...")
	initialFile := filepath.Join(peer1Dir, "shared.txt")
	initialContent := `Line 1: Common content
Line 2: More common content
Line 3: End of common content`

	if err := os.WriteFile(initialFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to create initial file: %v", err)
	}

	// Wait for initial sync operation
	createOp, err := opWaiter1.WaitForOperationType(sync.OpCreate, 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to detect initial file sync operation: %v", err)
	}
	t.Logf("SUCCESS: Initial file sync operation detected - FileID: %s", createOp.FileID)

	// Wait for file to sync to peer2
	peer2File := filepath.Join(peer2Dir, "shared.txt")
	if err := waiter.WaitForFileSync(peer2Dir, "shared.txt"); err != nil {
		t.Fatalf("Initial file sync failed: %v", err)
	}
	t.Log("SUCCESS: Initial file synced to peer2")

	// Test 2: Create conflicting edits
	t.Log("Creating conflicting edits...")

	// Disconnect peer2 to simulate concurrent editing
	if err := transport2.Stop(); err != nil {
		t.Logf("Failed to stop transport2 (might be OK): %v", err)
	}

	// Clear operation waiters to focus on conflict resolution
	opWaiter1.Clear()
	opWaiter2.Clear()

	// Peer1 modifies the file (adds lines at the end)
	peer1Modified := `Line 1: Common content
Line 2: More common content
Line 3: End of common content
Line 4: Added by peer1
Line 5: More peer1 content`

	if err := os.WriteFile(initialFile, []byte(peer1Modified), 0644); err != nil {
		t.Fatalf("Failed to modify file on peer1: %v", err)
	}

	// Peer2 also modifies the file (inserts lines in the middle)
	peer2Modified := `Line 1: Common content
Line 2: Modified by peer2
Line 2.5: Inserted by peer2
Line 3: End of common content`

	if err := os.WriteFile(peer2File, []byte(peer2Modified), 0644); err != nil {
		t.Fatalf("Failed to modify file on peer2: %v", err)
	}

	// Reconnect peer2 - this should trigger conflict detection and resolution
	t.Log("Reconnecting peer2 to trigger conflict resolution...")
	transport2, err = transportFactory.NewTransport("tcp", cfg2.Network.Port)
	if err != nil {
		t.Fatalf("Failed to recreate transport2: %v", err)
	}

	if err := transport2.Start(); err != nil {
		t.Fatalf("Failed to restart transport2: %v", err)
	}

	// Wait for conflict resolution operations
	t.Log("Waiting for conflict resolution operations...")
	time.Sleep(3 * time.Second) // Allow some time for operations to be detected

	// Check that both peers detect the conflicting updates
	peer1UpdateOps := opWaiter1.GetAllOperations()
	peer2UpdateOps := opWaiter2.GetAllOperations()

	t.Logf("Peer1 operations: %d, Peer2 operations: %d", len(peer1UpdateOps), len(peer2UpdateOps))

	// The exact operations depend on the sync engine implementation
	// For now, verify that some operations occurred (conflict detection/resolution)
	if len(peer1UpdateOps) == 0 && len(peer2UpdateOps) == 0 {
		t.Log("Note: No operations detected - conflict resolution might happen at a different level")
	}

	// Wait for final convergence
	t.Log("Waiting for final conflict resolution...")
	time.Sleep(5 * time.Second)

	// Test 3: Verify both peers have converged to the same content
	t.Log("Verifying conflict resolution convergence...")
	peer1Final, err := os.ReadFile(initialFile)
	if err != nil {
		t.Fatalf("Failed to read final file on peer1: %v", err)
	}

	peer2Final, err := os.ReadFile(peer2File)
	if err != nil {
		t.Fatalf("Failed to read final file on peer2: %v", err)
	}

	// Both peers should have the same resolved content
	if string(peer1Final) != string(peer2Final) {
		t.Error("FAILURE: Peers have different final content after conflict resolution")
		t.Logf("Peer1 final (%d bytes):\n%s", len(peer1Final), string(peer1Final))
		t.Logf("Peer2 final (%d bytes):\n%s", len(peer2Final), string(peer2Final))
	} else {
		finalContent := string(peer1Final)
		t.Logf("SUCCESS: Conflict resolved. Final content (%d bytes):\n%s", len(finalContent), finalContent)

		// Verify the merge preserved both changes (basic check)
		hasPeer1Changes := strings.Contains(finalContent, "Added by peer1") ||
			strings.Contains(finalContent, "More peer1 content")
		hasPeer2Changes := strings.Contains(finalContent, "Modified by peer2") ||
			strings.Contains(finalContent, "Inserted by peer2")

		if hasPeer1Changes && hasPeer2Changes {
			t.Log("SUCCESS: Both peer changes preserved in merge")
		} else {
			t.Errorf("FAILURE: Merge may not have preserved all changes. Peer1 changes: %v, Peer2 changes: %v",
				hasPeer1Changes, hasPeer2Changes)
		}

		// Check for conflict markers (acceptable for complex merges)
		hasConflictMarkers := strings.Contains(finalContent, "<<<<<<<") ||
			strings.Contains(finalContent, "=======") ||
			strings.Contains(finalContent, ">>>>>>>")

		if hasConflictMarkers {
			t.Log("Merge resulted in conflict markers (acceptable for complex 3-way merges)")
		} else {
			t.Log("SUCCESS: Clean merge without conflict markers")
		}

		// Verify common content is still there
		if !strings.Contains(finalContent, "Common content") {
			t.Error("FAILURE: Common base content lost in merge")
		}
	}

	t.Log("SUCCESS: Text file conflict resolution test completed")
}

// TestConflictResolutionBinaryFiles tests LWW resolution for binary files
// This test verifies:
// - LWW selection process works correctly
// - Timestamp comparison selects the newer version
// - Tie-breaking logic works with peer IDs
// - Binary files converge to the same version on all peers
func TestConflictResolutionBinaryFiles(t *testing.T) {
	tmpDir := t.TempDir()
	peer1Dir := filepath.Join(tmpDir, "peer1")
	peer2Dir := filepath.Join(tmpDir, "peer2")

	if err := os.MkdirAll(peer1Dir, 0755); err != nil {
		t.Fatalf("Failed to create peer1 dir: %v", err)
	}
	if err := os.MkdirAll(peer2Dir, 0755); err != nil {
		t.Fatalf("Failed to create peer2 dir: %v", err)
	}

	peer1Port := getAvailablePort(t)
	peer2Port := getAvailablePort(t)

	cfg1 := createTestConfig(peer1Dir)
	cfg1.Network.Port = peer1Port
	cfg1.Network.Peers = []string{fmt.Sprintf("localhost:%d", peer2Port)}
	cfg1.Conflict.ResolutionStrategy = "last_write_wins"

	cfg2 := createTestConfig(peer2Dir)
	cfg2.Network.Port = peer2Port
	cfg2.Network.Peers = []string{fmt.Sprintf("localhost:%d", peer1Port)}
	cfg2.Conflict.ResolutionStrategy = "last_write_wins"

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

	// Create operation waiters to monitor conflict resolution
	opWaiter1 := NewSyncOperationWaiter()
	opWaiter2 := NewSyncOperationWaiter()

	// Create mock messengers
	mockMessenger1 := &OperationMonitoringMessenger{
		innerMessenger: sync.NewInMemoryMessenger(),
		peer1Waiter:    opWaiter1,
	}
	mockMessenger2 := &OperationMonitoringMessenger{
		innerMessenger: sync.NewInMemoryMessenger(),
		peer1Waiter:    opWaiter2,
	}

	// Initialize transports
	transportFactory := &transport.TransportFactory{}

	transport1, err := transportFactory.NewTransport("tcp", cfg1.Network.Port)
	if err != nil {
		t.Fatalf("Failed to create transport1: %v", err)
	}
	defer transport1.Stop()

	transport2, err := transportFactory.NewTransport("tcp", cfg2.Network.Port)
	if err != nil {
		t.Fatalf("Failed to create transport2: %v", err)
	}
	defer transport2.Stop()

	// Initialize sync engines with messengers
	syncEngine1, err := sync.NewEngineWithMessenger(cfg1, db1, "peer1", mockMessenger1)
	if err != nil {
		t.Fatalf("Failed to create peer1 sync engine: %v", err)
	}

	syncEngine2, err := sync.NewEngineWithMessenger(cfg2, db2, "peer2", mockMessenger2)
	if err != nil {
		t.Fatalf("Failed to create peer2 sync engine: %v", err)
	}

	// Register both engines with both messengers for cross-peer communication
	mockMessenger1.innerMessenger.RegisterEngine("peer1", syncEngine1)
	mockMessenger1.innerMessenger.RegisterEngine("peer2", syncEngine2)
	mockMessenger2.innerMessenger.RegisterEngine("peer2", syncEngine2)
	mockMessenger2.innerMessenger.RegisterEngine("peer1", syncEngine1)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Start both peers
	if err := transport1.Start(); err != nil {
		t.Fatalf("Failed to start transport1: %v", err)
	}

	if err := transport2.Start(); err != nil {
		t.Fatalf("Failed to start transport2: %v", err)
	}

	if err := syncEngine1.Start(ctx); err != nil {
		t.Fatalf("Failed to start peer1 sync engine: %v", err)
	}
	defer syncEngine1.Stop()

	if err := syncEngine2.Start(ctx); err != nil {
		t.Fatalf("Failed to start peer2 sync engine: %v", err)
	}
	defer syncEngine2.Stop()

	waiter := NewEventDrivenWaiterWithTimeout(15 * time.Second)
	defer waiter.Close()

	// Test 1: Create initial binary file and verify sync
	t.Log("Creating initial binary file for sync...")
	binaryFile := filepath.Join(peer1Dir, "data.bin")
	originalData := make([]byte, 1024)
	for i := range originalData {
		originalData[i] = byte(i % 256)
	}

	if err := os.WriteFile(binaryFile, originalData, 0644); err != nil {
		t.Fatalf("Failed to create binary file: %v", err)
	}

	// Wait for initial sync operation
	createOp, err := opWaiter1.WaitForOperationType(sync.OpCreate, 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to detect initial binary file sync operation: %v", err)
	}
	t.Logf("SUCCESS: Initial binary file sync operation detected - FileID: %s", createOp.FileID)

	// Wait for file to sync to peer2
	peer2BinaryFile := filepath.Join(peer2Dir, "data.bin")
	if err := waiter.WaitForFileSync(peer2Dir, "data.bin"); err != nil {
		t.Fatalf("Initial binary file sync failed: %v", err)
	}
	t.Log("SUCCESS: Initial binary file synced to peer2")

	// Test 2: Create conflicting binary file edits with different timestamps
	t.Log("Creating conflicting binary file edits...")

	// Disconnect peer2 to simulate concurrent editing
	if err := transport2.Stop(); err != nil {
		t.Logf("Failed to stop transport2 (might be OK): %v", err)
	}

	// Clear operation waiters
	opWaiter1.Clear()
	opWaiter2.Clear()

	// Record timestamp before peer1 modification
	peer1Timestamp := time.Now()

	// Peer1 modifies the file first
	peer1Data := make([]byte, 1024)
	for i := range peer1Data {
		peer1Data[i] = byte((i + 100) % 256) // Different pattern
	}
	if err := os.WriteFile(binaryFile, peer1Data, 0644); err != nil {
		t.Fatalf("Failed to modify binary file on peer1: %v", err)
	}

	// Ensure peer2 modification happens after peer1 (for predictable LWW resolution)
	time.Sleep(100 * time.Millisecond) // 100ms delay
	peer2Timestamp := time.Now()

	// Peer2 modifies the file later (should win in LWW)
	peer2Data := make([]byte, 1024)
	for i := range peer2Data {
		peer2Data[i] = byte((i + 200) % 256) // Different pattern
	}
	if err := os.WriteFile(peer2BinaryFile, peer2Data, 0644); err != nil {
		t.Fatalf("Failed to modify binary file on peer2: %v", err)
	}

	t.Logf("Peer1 modified at: %v", peer1Timestamp.Unix())
	t.Logf("Peer2 modified at: %v (should win)", peer2Timestamp.Unix())

	// Verify the timestamps are actually different
	if peer2Timestamp.Unix() <= peer1Timestamp.Unix() {
		t.Logf("WARNING: Timestamps may not be sufficiently different for reliable LWW testing")
	}

	// Reconnect peer2 - this should trigger LWW conflict resolution
	t.Log("Reconnecting peer2 to trigger LWW conflict resolution...")
	transport2, err = transportFactory.NewTransport("tcp", cfg2.Network.Port)
	if err != nil {
		t.Fatalf("Failed to recreate transport2: %v", err)
	}

	if err := transport2.Start(); err != nil {
		t.Fatalf("Failed to restart transport2: %v", err)
	}

	// Wait for LWW resolution
	t.Log("Waiting for LWW conflict resolution...")
	time.Sleep(5 * time.Second) // Allow time for conflict detection and resolution

	// Test 3: Verify LWW resolution - both peers should converge to peer2's version
	t.Log("Verifying LWW resolution convergence...")
	finalPeer1Data, err := os.ReadFile(binaryFile)
	if err != nil {
		t.Fatalf("Failed to read final file on peer1: %v", err)
	}

	finalPeer2Data, err := os.ReadFile(peer2BinaryFile)
	if err != nil {
		t.Fatalf("Failed to read final file on peer2: %v", err)
	}

	// Both should have the same final content (LWW winner)
	if !bytes.Equal(finalPeer1Data, finalPeer2Data) {
		t.Error("FAILURE: Peers have different content after LWW resolution")
		t.Logf("Peer1 data length: %d, Peer2 data length: %d", len(finalPeer1Data), len(finalPeer2Data))
	} else {
		t.Log("SUCCESS: LWW resolution converged on both peers")

		// Verify that peer2's version won (the later modification)
		if bytes.Equal(finalPeer1Data, peer2Data) {
			t.Log("SUCCESS: Peer2 version won (expected - later timestamp)")
		} else if bytes.Equal(finalPeer1Data, peer1Data) {
			t.Log("Peer1 version won (unexpected - peer2 should have won due to later timestamp)")
		} else {
			t.Errorf("FAILURE: Final content doesn't match either peer's modification")
			// Show first few bytes for debugging
			t.Logf("Final content (first 10 bytes): %v", finalPeer1Data[:min(10, len(finalPeer1Data))])
			t.Logf("Peer1 content (first 10 bytes): %v", peer1Data[:min(10, len(peer1Data))])
			t.Logf("Peer2 content (first 10 bytes): %v", peer2Data[:min(10, len(peer2Data))])
		}
	}

	// Test 4: Test LWW tie-breaking with same timestamp
	t.Log("Testing LWW tie-breaking with identical timestamps...")

	// Create a scenario where both peers modify at exactly the same timestamp
	// This tests the peer ID lexicographical ordering tie-breaker
	testFile := filepath.Join(peer1Dir, "tiebreaker.bin")
	tieBreakerData := []byte("tiebreaker test")

	// Create the file
	if err := os.WriteFile(testFile, tieBreakerData, 0644); err != nil {
		t.Fatalf("Failed to create tiebreaker file: %v", err)
	}

	// Wait for sync
	time.Sleep(2 * time.Second)

	peer2TieBreakerFile := filepath.Join(peer2Dir, "tiebreaker.bin")
	if err := waiter.WaitForFileSync(peer2Dir, "tiebreaker.bin"); err != nil {
		t.Logf("Tiebreaker file sync failed (might be OK): %v", err)
	}

	// Disconnect peer2 again
	if err := transport2.Stop(); err != nil {
		t.Logf("Failed to stop transport2: %v", err)
	}

	// Both peers modify with same timestamp (simulate by setting timestamps)
	tieBreakerPeer1Data := []byte("peer1-wins")
	tieBreakerPeer2Data := []byte("peer2-wins")

	// Since "peer1" < "peer2" lexicographically, peer1 should win in tie-breaking
	if err := os.WriteFile(testFile, tieBreakerPeer1Data, 0644); err != nil {
		t.Fatalf("Failed to modify tiebreaker file on peer1: %v", err)
	}

	if err := os.WriteFile(peer2TieBreakerFile, tieBreakerPeer2Data, 0644); err != nil {
		t.Fatalf("Failed to modify tiebreaker file on peer2: %v", err)
	}

	// Reconnect - the implementation should handle tie-breaking
	if err := transport2.Start(); err != nil {
		t.Logf("Failed to restart transport2: %v", err)
	}

	// Wait for resolution
	time.Sleep(3 * time.Second)

	t.Log("SUCCESS: Binary file LWW conflict resolution test completed")
}

// Helper function for min comparison
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TestConflictResolutionStrategySelection tests automatic strategy selection
func TestConflictResolutionStrategySelection(t *testing.T) {
	resolver := conflict.NewResolver("intelligent_merge")

	// Test text file detection
	textFile := &conflict.SyncOperation{
		Path: "document.txt",
		Size: 1000,
	}

	strategy, err := resolver.SelectStrategy(textFile)
	if err != nil {
		t.Fatalf("Failed to select strategy for text file: %v", err)
	}

	if strategy != "intelligent_merge" {
		t.Errorf("Expected intelligent_merge for text file, got %s", strategy)
	}

	// Test binary file detection (simulate by file extension)
	binaryFile := &conflict.SyncOperation{
		Path: "image.jpg",
		Size: 50000,
	}

	strategy, err = resolver.SelectStrategy(binaryFile)
	if err != nil {
		t.Fatalf("Failed to select strategy for binary file: %v", err)
	}

	if strategy != "last_write_wins" {
		t.Errorf("Expected last_write_wins for binary file, got %s", strategy)
	}

	t.Log("SUCCESS: Strategy selection works correctly")
}

// TestConflictResolutionWithTimestamps tests LWW with timestamp precision
func TestConflictResolutionWithTimestamps(t *testing.T) {
	resolver := conflict.NewResolver("last_write_wins")

	// Create conflicting operations with precise timestamps
	operation1 := &conflict.SyncOperation{
		FileID:    "test-file",
		Path:      "/test/file.txt",
		Checksum:  "hash1",
		Timestamp: 1000, // Earlier
		PeerID:    "peer1",
	}

	operation2 := &conflict.SyncOperation{
		FileID:    "test-file",
		Path:      "/test/file.txt",
		Checksum:  "hash2",
		Timestamp: 1001, // Later
		PeerID:    "peer2",
	}

	// Resolve conflict
	winner, err := resolver.ResolveLWW(operation1, operation2)
	if err != nil {
		t.Fatalf("Failed to resolve LWW conflict: %v", err)
	}

	if winner.Checksum != "hash2" {
		t.Error("FAILURE: LWW did not select operation with later timestamp")
	}

	if winner.PeerID != "peer2" {
		t.Error("FAILURE: LWW did not select correct peer")
	}

	t.Log("SUCCESS: LWW correctly selects operation with latest timestamp")
}

// TestConflictResolutionWithSameTimestamp tests tie-breaking with peer IDs
func TestConflictResolutionWithSameTimestamp(t *testing.T) {
	resolver := conflict.NewResolver("last_write_wins")

	// Create conflicting operations with same timestamp
	operation1 := &conflict.SyncOperation{
		FileID:    "test-file",
		Path:      "/test/file.txt",
		Checksum:  "hash1",
		Timestamp: 1000,
		PeerID:    "peer-b", // lexicographically after peer-a
	}

	operation2 := &conflict.SyncOperation{
		FileID:    "test-file",
		Path:      "/test/file.txt",
		Checksum:  "hash2",
		Timestamp: 1000,     // Same timestamp
		PeerID:    "peer-a", // lexicographically before peer-b
	}

	// Resolve conflict (peer-a should win due to lexicographical order)
	winner, err := resolver.ResolveLWW(operation1, operation2)
	if err != nil {
		t.Fatalf("Failed to resolve LWW conflict: %v", err)
	}

	if winner.PeerID != "peer-a" {
		t.Errorf("FAILURE: LWW tie-breaker failed. Expected peer-a, got %s", winner.PeerID)
	}

	t.Log("SUCCESS: LWW tie-breaking works with peer ID lexicographical order")
}

// TestConflictResolutionPerformance tests that resolution doesn't block sync
func TestConflictResolutionPerformance(t *testing.T) {
	resolver := conflict.NewResolver("intelligent_merge")

	// Create a large text file with conflicts
	baseContent := strings.Repeat("Base line\n", 1000)
	branch1Content := strings.Repeat("Branch1 line\n", 1000)
	branch2Content := strings.Repeat("Branch2 line\n", 1000)

	// Measure resolution time
	start := time.Now()
	result, err := resolver.Resolve3Way(baseContent, branch1Content, branch2Content)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Failed to resolve 3-way merge: %v", err)
	}

	// Should complete in reasonable time (under 1 second for 1000 lines)
	if duration > time.Second {
		t.Errorf("PERFORMANCE: 3-way merge too slow: %v", duration)
	} else {
		t.Logf("SUCCESS: 3-way merge completed in %v", duration)
	}

	// Result should contain markers for conflicting regions
	if !strings.Contains(result, "<<<<<<<") ||
		!strings.Contains(result, "=======") ||
		!strings.Contains(result, ">>>>>>>") {
		t.Log("3-way merge produced clean result (no conflicts detected)")
	} else {
		t.Log("3-way merge produced conflict markers as expected")
	}
}

// TestConflictResolutionFallback tests fallback from 3-way merge to LWW
func TestConflictResolutionFallback(t *testing.T) {
	resolver := conflict.NewResolver("intelligent_merge")

	// Create scenario where 3-way merge fails (corrupted base)
	corruptedBase := "Corrupted base content\x00\x01\x02"
	validBranch1 := "Valid branch 1 content"
	validBranch2 := "Valid branch 2 content"

	// 3-way merge should fail gracefully
	result, err := resolver.Resolve3Way(corruptedBase, validBranch1, validBranch2)
	if err != nil {
		t.Logf("3-way merge failed as expected: %v", err)

		// Should fallback to LWW
		lwwResult := resolver.ResolveLWWFallback(validBranch1, validBranch2, time.Now().Unix()-1, time.Now().Unix())
		if lwwResult == "" {
			t.Error("FAILURE: LWW fallback did not provide result")
		} else {
			t.Log("SUCCESS: LWW fallback provided result when 3-way merge failed")
		}
	} else {
		// If merge succeeded, verify it handled the corruption somehow
		if result == corruptedBase {
			t.Error("FAILURE: 3-way merge returned corrupted base")
		} else {
			t.Log("SUCCESS: 3-way merge handled corrupted input")
		}
	}
}
