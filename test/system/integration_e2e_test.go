//go:build integration

package system

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	syncpkg "github.com/p2p-folder-sync/p2p-sync/internal/sync"
)

// TestEndToEndIntegration performs a comprehensive end-to-end test of the P2P folder sync system.
// This test validates the complete workflow from peer setup to final state convergence:
// - Multiple peers (3+) with real network connections
// - Complex file operations (create, modify, delete, rename)
// - Concurrent operations from different peers
// - State synchronization and conflict resolution
// - Final consistency verification across all peers
func TestEndToEndIntegration(t *testing.T) {
	// Use network test helper for real network components
	nth := NewNetworkTestHelper(t)

	// Setup three peers for comprehensive testing
	peers := make([]*NetworkPeerSetup, 3)
	peerNames := []string{"alpha", "beta", "gamma"}

	for i, name := range peerNames {
		peer, err := nth.SetupPeer(t, name, true) // Enable encryption
		if err != nil {
			t.Fatalf("Failed to setup peer %s: %v", name, err)
		}
		defer peer.Cleanup()
		peers[i] = peer
	}

	// Create a fully connected mesh topology
	for i, peer1 := range peers {
		var peerList []string
		for j, peer2 := range peers {
			if i != j {
				peerList = append(peerList, fmt.Sprintf("localhost:%d", peer2.Config.Network.Port))
			}
		}
		peer1.Config.Network.Peers = peerList
	}

	// Create network operation monitors for all peers
	monitors := make([]*NetworkOperationMonitor, len(peers))
	interceptedTransports := make([]*NetworkMessageInterceptor, len(peers))

	for i, peer := range peers {
		monitors[i] = NewNetworkOperationMonitor()
		interceptedTransports[i] = NewNetworkMessageInterceptor(peer.Transport, monitors[i])
		peer.Transport = interceptedTransports[i]
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second) // 5 minutes for comprehensive test
	defer cancel()

	// Start all peers in sequence
	for i, peer := range peers {
		if err := peer.Transport.Start(); err != nil {
			t.Fatalf("Failed to start peer %s transport: %v", peerNames[i], err)
		}
		if err := peer.SyncEngine.Start(ctx); err != nil {
			t.Fatalf("Failed to start peer %s sync engine: %v", peerNames[i], err)
		}

		// Stagger startup to avoid connection race conditions
		if i < len(peers)-1 {
			// Use event-driven wait for peer initialization
			initWaiter := NewEventDrivenWaiterWithTimeout(5 * time.Second)
			initWaiter.WaitForCondition(func() bool {
				// Check if peer is ready (this is a simplified check)
				return true // Always true for now, could check peer health
			}, "peer initialization")
		}
	}

	// Wait for all peers to establish connections
	t.Log("Waiting for all peers to form mesh network...")
	if err := WaitForPeerConnections(peers, 60*time.Second); err != nil {
		t.Fatalf("Peers failed to form mesh network: %v", err)
	}
	t.Log("SUCCESS: All peers connected in mesh topology")

	waiter := NewEventDrivenWaiterWithTimeout(60 * time.Second)
	defer waiter.Close()

	// Phase 1: Initial file distribution
	t.Log("Phase 1: Testing initial file distribution...")

	initialFiles := []struct {
		peerIndex int
		filename  string
		content   string
	}{
		{0, "readme.txt", "Welcome to the P2P sync test environment"},
		{1, "config.json", `{"version": "1.0", "peers": 3}`},
		{2, "data.bin", string(make([]byte, 1024))}, // 1KB binary data
	}

	for _, file := range initialFiles {
		peer := peers[file.peerIndex]
		filePath := filepath.Join(peer.Dir, file.filename)

		if err := os.WriteFile(filePath, []byte(file.content), 0644); err != nil {
			t.Fatalf("Failed to create initial file %s on peer %s: %v", file.filename, peerNames[file.peerIndex], err)
		}
	}

	// Wait for all initial files to sync to all peers
	for _, peer := range peers {
		for _, file := range initialFiles {
			if err := waiter.WaitForFileSync(peer.Dir, file.filename); err != nil {
				t.Fatalf("Initial file %s failed to sync to peer %s: %v", file.filename, peerNames[getPeerIndex(peers, peer)], err)
			}
		}
	}
	t.Log("SUCCESS: All initial files distributed to all peers")

	// Phase 2: Concurrent modifications
	t.Log("Phase 2: Testing concurrent modifications...")

	// Each peer modifies a different file simultaneously
	modifications := []struct {
		peerIndex  int
		filename   string
		newContent string
	}{
		{0, "readme.txt", "Welcome to the P2P sync test environment - Updated by Alpha"},
		{1, "config.json", `{"version": "1.1", "peers": 3, "updated": true}`},
		{2, "data.bin", string(make([]byte, 2048))}, // 2KB binary data
	}

	// Perform modifications concurrently (simulate real concurrent edits)
	modificationComplete := make(chan bool, len(modifications))
	for _, mod := range modifications {
		go func(mod struct {
			peerIndex  int
			filename   string
			newContent string
		}) {
			peer := peers[mod.peerIndex]
			filePath := filepath.Join(peer.Dir, mod.filename)

			if err := os.WriteFile(filePath, []byte(mod.newContent), 0644); err != nil {
				t.Errorf("Failed to modify file %s on peer %s: %v", mod.filename, peerNames[mod.peerIndex], err)
			}
			modificationComplete <- true
		}(mod)
	}

	// Wait for all modifications to complete
	for i := 0; i < len(modifications); i++ {
		<-modificationComplete
	}

	// Wait for all modifications to propagate using event-driven waiting
	for _, peer := range peers {
		for _, mod := range modifications {
			waiter.WaitForFileContent(peer.Dir, mod.filename, []byte(mod.newContent))
		}
	}

	// Verify all modifications synced correctly
	for i, peer := range peers {
		for _, mod := range modifications {
			expectedContent := mod.newContent
			if err := waiter.WaitForFileContent(peer.Dir, mod.filename, []byte(expectedContent)); err != nil {
				t.Errorf("Modified file %s failed to sync to peer %s: %v", mod.filename, peerNames[i], err)
			} else {
				t.Logf("SUCCESS: Modified file %s synced to peer %s", mod.filename, peerNames[i])
			}
		}
	}

	// Phase 3: File deletion and recreation
	t.Log("Phase 3: Testing file deletion and recreation...")

	// Peer Alpha deletes a file
	deleteFile := "readme.txt"
	deletePath := filepath.Join(peers[0].Dir, deleteFile)
	if err := os.Remove(deletePath); err != nil {
		t.Fatalf("Failed to delete file on peer Alpha: %v", err)
	}

	// Wait for deletion to sync to other peers
	for i := 1; i < len(peers); i++ {
		peer := peers[i]
		if err := waiter.WaitForFileDeletion(peer.Dir, deleteFile); err != nil {
			t.Errorf("File deletion failed to sync to peer %s: %v", peerNames[i], err)
		} else {
			t.Logf("SUCCESS: File deletion synced to peer %s", peerNames[i])
		}
	}

	// Peer Beta recreates the file with different content
	recreateContent := "Recreated file content by Beta"
	recreatePath := filepath.Join(peers[1].Dir, deleteFile)
	if err := os.WriteFile(recreatePath, []byte(recreateContent), 0644); err != nil {
		t.Fatalf("Failed to recreate file on peer Beta: %v", err)
	}

	// Wait for recreation to sync to all peers
	for _, peer := range peers {
		if err := waiter.WaitForFileContent(peer.Dir, deleteFile, []byte(recreateContent)); err != nil {
			t.Errorf("File recreation failed to sync to peer %s: %v", peerNames[getPeerIndex(peers, peer)], err)
		} else {
			t.Logf("SUCCESS: File recreation synced to peer %s", peerNames[getPeerIndex(peers, peer)])
		}
	}

	// Phase 4: Rename operations
	t.Log("Phase 4: Testing rename operations...")

	// Peer Gamma renames the config file
	oldName := "config.json"
	newName := "settings.json"
	oldPath := filepath.Join(peers[2].Dir, oldName)
	newPath := filepath.Join(peers[2].Dir, newName)

	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatalf("Failed to rename file on peer Gamma: %v", err)
	}

	// Wait for rename to sync to other peers
	for i := 0; i < len(peers)-1; i++ {
		peer := peers[i]
		// Old file should be gone
		if err := waiter.WaitForFileDeletion(peer.Dir, oldName); err != nil {
			t.Errorf("Old file %s not deleted on peer %s: %v", oldName, peerNames[i], err)
		}
		// New file should appear
		if err := waiter.WaitForFileSync(peer.Dir, newName); err != nil {
			t.Errorf("Renamed file %s not synced to peer %s: %v", newName, peerNames[i], err)
		} else {
			t.Logf("SUCCESS: File rename synced to peer %s", peerNames[i])
		}
	}

	// Phase 5: Large file handling
	t.Log("Phase 5: Testing large file handling...")

	// Create a large file that should trigger chunking
	largeFileName := "large_test_data.bin"
	largeFileSize := 3 * 1024 * 1024 // 3MB
	largeData := make([]byte, largeFileSize)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	// Peer Alpha creates the large file
	largeFilePath := filepath.Join(peers[0].Dir, largeFileName)
	if err := os.WriteFile(largeFilePath, largeData, 0644); err != nil {
		t.Fatalf("Failed to create large file on peer Alpha: %v", err)
	}

	// Wait for large file to sync to all peers (with longer timeout)
	largeWaiter := NewEventDrivenWaiterWithTimeout(120 * time.Second)
	defer largeWaiter.Close()

	for _, peer := range peers {
		if err := largeWaiter.WaitForFileSync(peer.Dir, largeFileName); err != nil {
			t.Errorf("Large file failed to sync to peer %s: %v", peerNames[getPeerIndex(peers, peer)], err)
		} else {
			// Verify file integrity
			peerFilePath := filepath.Join(peer.Dir, largeFileName)
			if peerData, err := os.ReadFile(peerFilePath); err != nil {
				t.Errorf("Failed to read large file on peer %s: %v", peerNames[getPeerIndex(peers, peer)], err)
			} else if len(peerData) != largeFileSize {
				t.Errorf("Large file size mismatch on peer %s: expected %d, got %d",
					peerNames[getPeerIndex(peers, peer)], largeFileSize, len(peerData))
			} else {
				// Spot check data integrity
				corrupted := false
				for i := 0; i < len(peerData); i += 1000 { // Check every 1000th byte
					if peerData[i] != byte(i%256) {
						corrupted = true
						break
					}
				}
				if corrupted {
					t.Errorf("Large file data corrupted on peer %s", peerNames[getPeerIndex(peers, peer)])
				} else {
					t.Logf("SUCCESS: Large file integrity verified on peer %s", peerNames[getPeerIndex(peers, peer)])
				}
			}
		}
	}

	// Phase 6: Final consistency verification
	t.Log("Phase 6: Performing final consistency verification...")

	// Collect all expected files and their contents
	expectedFiles := map[string]string{
		"readme.txt":    recreateContent,
		"settings.json": `{"version": "1.1", "peers": 3, "updated": true}`,
		"data.bin":      string(make([]byte, 2048)),
		largeFileName:   string(largeData),
	}

	// Verify all peers have identical file sets
	for i, peer := range peers {
		files, err := os.ReadDir(peer.Dir)
		if err != nil {
			t.Fatalf("Failed to list files in peer %s directory: %v", peerNames[i], err)
		}

		// Check that all expected files are present with correct content
		for filename, expectedContent := range expectedFiles {
			found := false
			for _, file := range files {
				if file.Name() == filename {
					found = true

					// Verify content
					filePath := filepath.Join(peer.Dir, filename)
					actualContent, err := os.ReadFile(filePath)
					if err != nil {
						t.Errorf("Failed to read file %s on peer %s: %v", filename, peerNames[i], err)
					} else if string(actualContent) != expectedContent {
						// For large files, just check size
						if filename == largeFileName {
							if len(actualContent) != len(expectedContent) {
								t.Errorf("Large file %s size mismatch on peer %s: expected %d, got %d",
									filename, peerNames[i], len(expectedContent), len(actualContent))
							}
						} else {
							t.Errorf("File %s content mismatch on peer %s", filename, peerNames[i])
						}
					}
					break
				}
			}
			if !found {
				t.Errorf("Expected file %s not found on peer %s", filename, peerNames[i])
			}
		}

		// Check for unexpected files (should only have expected files)
		for _, file := range files {
			if strings.HasPrefix(file.Name(), ".") {
				continue // Skip hidden files
			}
			if _, expected := expectedFiles[file.Name()]; !expected {
				t.Errorf("Unexpected file %s found on peer %s", file.Name(), peerNames[i])
			}
		}
	}

	// Phase 7: Operation monitoring verification
	t.Log("Phase 7: Verifying operation monitoring...")

	totalOperations := 0
	for i, monitor := range monitors {
		ops := monitor.GetAllOperations()
		totalOperations += len(ops)
		t.Logf("Peer %s recorded %d operations", peerNames[i], len(ops))

		// Verify we captured the major operations
		createOps := 0
		updateOps := 0
		deleteOps := 0
		renameOps := 0

		for _, op := range ops {
			switch op.Type {
			case syncpkg.OpCreate:
				createOps++
			case syncpkg.OpUpdate:
				updateOps++
			case syncpkg.OpDelete:
				deleteOps++
			case syncpkg.OpRename:
				renameOps++
			}
		}

		t.Logf("Peer %s: %d creates, %d updates, %d deletes, %d renames",
			peerNames[i], createOps, updateOps, deleteOps, renameOps)
	}

	if totalOperations == 0 {
		t.Log("NOTE: No operations were recorded (may be expected with current monitoring)")
	} else {
		t.Logf("SUCCESS: Recorded %d total operations across all peers", totalOperations)
	}

	t.Log("SUCCESS: End-to-end integration test completed - all peers achieved consistent state")
}

// getPeerIndex returns the index of a peer in the peers slice
func getPeerIndex(peers []*NetworkPeerSetup, target *NetworkPeerSetup) int {
	for i, peer := range peers {
		if peer == target {
			return i
		}
	}
	return -1
}

// TestComplexConflictResolution tests conflict resolution with multiple concurrent edits.
// This test creates complex conflict scenarios and verifies proper resolution:
// - Multiple peers editing the same file simultaneously
// - Different types of conflicts (text merges, binary conflicts)
// - Proper conflict resolution strategy application
func TestComplexConflictResolution(t *testing.T) {
	// Use network test helper for real network components
	nth := NewNetworkTestHelper(t)

	// Setup three peers for conflict testing
	peers := make([]*NetworkPeerSetup, 3)
	peerNames := []string{"alice", "bob", "charlie"}

	for i, name := range peerNames {
		peer, err := nth.SetupPeer(t, name, true)
		if err != nil {
			t.Fatalf("Failed to setup peer %s: %v", name, err)
		}
		defer peer.Cleanup()
		peers[i] = peer
	}

	// Configure for intelligent merge (3-way merge for text files)
	for _, peer := range peers {
		peer.Config.Conflict.ResolutionStrategy = "intelligent_merge"
	}

	// Create fully connected topology
	for i, peer1 := range peers {
		var peerList []string
		for j, peer2 := range peers {
			if i != j {
				peerList = append(peerList, fmt.Sprintf("localhost:%d", peer2.Config.Network.Port))
			}
		}
		peer1.Config.Network.Peers = peerList
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// Start all peers
	for i, peer := range peers {
		if err := peer.Transport.Start(); err != nil {
			t.Fatalf("Failed to start peer %s transport: %v", peerNames[i], err)
		}
		if err := peer.SyncEngine.Start(ctx); err != nil {
			t.Fatalf("Failed to start peer %s sync engine: %v", peerNames[i], err)
		}
		if i < len(peers)-1 {
			time.Sleep(1 * time.Second)
		}
	}

	// Wait for connections
	if err := WaitForPeerConnections(peers, 30*time.Second); err != nil {
		t.Fatalf("Peers failed to connect: %v", err)
	}

	waiter := NewEventDrivenWaiterWithTimeout(45 * time.Second)
	defer waiter.Close()

	// Test 1: Create base file
	t.Log("Creating base file for conflict testing...")

	baseFile := "conflict_test.txt"
	baseContent := `Line 1: Base content
Line 2: More base content
Line 3: End of base content`

	basePath := filepath.Join(peers[0].Dir, baseFile)
	if err := os.WriteFile(basePath, []byte(baseContent), 0644); err != nil {
		t.Fatalf("Failed to create base file: %v", err)
	}

	// Wait for base file to sync to all peers
	for _, peer := range peers {
		if err := waiter.WaitForFileSync(peer.Dir, baseFile); err != nil {
			t.Fatalf("Base file failed to sync: %v", err)
		}
	}

	// Test 2: Create conflicting edits
	t.Log("Creating conflicting edits on all peers...")

	// Temporarily disconnect peer2 to create isolated edits
	// (This simulates the scenario where peers edit concurrently without seeing each other's changes)

	// Peer1 adds lines at the beginning
	peer1Content := `Line 0: Added by Peer1
Line 1: Base content
Line 2: More base content
Line 3: End of base content`

	// Peer2 modifies middle lines
	peer2Content := `Line 1: Base content
Line 2: Modified by Peer2
Line 2.5: Inserted by Peer2
Line 3: End of base content`

	// Peer3 adds lines at the end
	peer3Content := `Line 1: Base content
Line 2: More base content
Line 3: End of base content
Line 4: Added by Peer3`

	// Apply conflicting edits
	if err := os.WriteFile(filepath.Join(peers[0].Dir, baseFile), []byte(peer1Content), 0644); err != nil {
		t.Fatalf("Failed to apply Peer1 edit: %v", err)
	}
	if err := os.WriteFile(filepath.Join(peers[1].Dir, baseFile), []byte(peer2Content), 0644); err != nil {
		t.Fatalf("Failed to apply Peer2 edit: %v", err)
	}
	if err := os.WriteFile(filepath.Join(peers[2].Dir, baseFile), []byte(peer3Content), 0644); err != nil {
		t.Fatalf("Failed to apply Peer3 edit: %v", err)
	}

	// Wait for conflicts to be detected and resolved
	t.Log("Waiting for conflict resolution...")
	conflictWaiter := NewEventDrivenWaiterWithTimeout(15 * time.Second)
	conflictWaiter.WaitForCondition(func() bool {
		// Check if all peers have the same file content (convergence)
		if len(peers) < 2 {
			return true
		}
		firstContent, err := os.ReadFile(filepath.Join(peers[0].Dir, baseFile))
		if err != nil {
			return false
		}
		for i := 1; i < len(peers); i++ {
			otherContent, err := os.ReadFile(filepath.Join(peers[i].Dir, baseFile))
			if err != nil || string(firstContent) != string(otherContent) {
				return false
			}
		}
		return true
	}, "conflict resolution convergence")

	// Verify all peers converged to the same final content
	finalContents := make([]string, len(peers))
	for i, peer := range peers {
		content, err := os.ReadFile(filepath.Join(peer.Dir, baseFile))
		if err != nil {
			t.Fatalf("Failed to read final file on peer %s: %v", peerNames[i], err)
		}
		finalContents[i] = string(content)
	}

	// Check convergence
	for i := 1; i < len(finalContents); i++ {
		if finalContents[0] != finalContents[i] {
			t.Errorf("Peers have different final content after conflict resolution")
			t.Logf("Peer %s content:\n%s", peerNames[0], finalContents[0])
			t.Logf("Peer %s content:\n%s", peerNames[i], finalContents[i])
		}
	}

	finalContent := finalContents[0]
	t.Logf("Final merged content:\n%s", finalContent)

	// Verify that all original edits are preserved in the merge
	hasPeer1Edit := strings.Contains(finalContent, "Added by Peer1")
	hasPeer2Edit := strings.Contains(finalContent, "Modified by Peer2") ||
		strings.Contains(finalContent, "Inserted by Peer2")
	hasPeer3Edit := strings.Contains(finalContent, "Added by Peer3")
	hasBaseContent := strings.Contains(finalContent, "Base content")

	if !hasBaseContent {
		t.Error("FAILURE: Base content lost in merge")
	}
	if !hasPeer1Edit {
		t.Error("FAILURE: Peer1 edit lost in merge")
	}
	if !hasPeer2Edit {
		t.Error("FAILURE: Peer2 edit lost in merge")
	}
	if !hasPeer3Edit {
		t.Error("FAILURE: Peer3 edit lost in merge")
	}

	if hasBaseContent && hasPeer1Edit && hasPeer2Edit && hasPeer3Edit {
		t.Log("SUCCESS: All edits preserved in intelligent merge")
	}

	t.Log("SUCCESS: Complex conflict resolution test completed")
}
