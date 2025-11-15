package state_test

import (
	"testing"

	"github.com/p2p-folder-sync/p2p-sync/internal/network/messages"
	"github.com/p2p-folder-sync/p2p-sync/internal/state"
)

func TestReconciler(t *testing.T) {
	reconciler := state.NewReconciler()

	if reconciler == nil {
		t.Error("Expected reconciler to be created")
	}

	// Test AssignFilesToPeers with actual peer states
	peers := map[string]*state.PeerState{
		"peer1": {
			PeerID: "peer1",
			FileManifest: []state.FileManifestEntry{
				{FileID: "file1", Size: 100},
				{FileID: "file2", Size: 200},
			},
		},
		"peer2": {
			PeerID: "peer2",
			FileManifest: []state.FileManifestEntry{
				{FileID: "file3", Size: 150},
			},
		},
		"peer3": {
			PeerID: "peer3",
			FileManifest: []state.FileManifestEntry{},
		},
	}

	files := []string{"fileA", "fileB", "fileC", "fileD", "fileE"}

	assignments := reconciler.AssignFilesToPeers(peers, files)

	// Verify all files are assigned
	totalAssigned := 0
	for _, assignedFiles := range assignments {
		totalAssigned += len(assignedFiles)
	}
	if totalAssigned != len(files) {
		t.Errorf("Expected %d files assigned, got %d", len(files), totalAssigned)
	}

	// Verify round-robin distribution (simple check that all peers got assignments)
	if len(assignments) != len(peers) {
		t.Errorf("Expected assignments for %d peers, got %d", len(peers), len(assignments))
	}

	// Test with empty peer map
	emptyAssignments := reconciler.AssignFilesToPeers(map[string]*state.PeerState{}, files)
	if len(emptyAssignments) != 0 {
		t.Error("Expected no assignments for empty peer map")
	}
}

func TestLoadBalancer(t *testing.T) {
	peers := []string{"peer1", "peer2", "peer3"}
	lb := state.NewLoadBalancer(peers)

	if lb == nil {
		t.Error("Expected load balancer to be created")
	}

	// Test DistributeFiles
	files := []messages.FileManifestEntry{
		{FileID: "file1", Path: "/path1", Hash: "hash1", Size: 100},
		{FileID: "file2", Path: "/path2", Hash: "hash2", Size: 200},
		{FileID: "file3", Path: "/path3", Hash: "hash3", Size: 150},
		{FileID: "file4", Path: "/path4", Hash: "hash4", Size: 300},
		{FileID: "file5", Path: "/path5", Hash: "hash5", Size: 50},
	}

	distribution := lb.DistributeFiles(files)

	// Verify all files are distributed
	totalDistributed := 0
	for _, peerFiles := range distribution {
		totalDistributed += len(peerFiles)
	}
	if totalDistributed != len(files) {
		t.Errorf("Expected %d files distributed, got %d", len(files), totalDistributed)
	}

	// Verify all peers got some files (assuming even distribution)
	if len(distribution) != len(peers) {
		t.Errorf("Expected distribution for %d peers, got %d", len(peers), len(distribution))
	}

	// Test GetPeerForFile - verify consistent hashing
	testFileID := "consistent-test-file"
	peer1 := lb.GetPeerForFile(testFileID)
	peer2 := lb.GetPeerForFile(testFileID)
	if peer1 != peer2 {
		t.Error("GetPeerForFile should return consistent results for same file ID")
	}

	// Verify returned peer is in the peer list
	found := false
	for _, peer := range peers {
		if peer == peer1 {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("GetPeerForFile returned peer %s not in peer list %v", peer1, peers)
	}

	// Test with different file IDs to verify distribution
	filePeers := make(map[string]string)
	testFileIDs := []string{"file-a", "file-b", "file-c", "file-d", "file-e"}
	for _, fileID := range testFileIDs {
		filePeers[fileID] = lb.GetPeerForFile(fileID)
	}

	// Should have some distribution (not all files going to same peer)
	uniquePeers := make(map[string]bool)
	for _, peer := range filePeers {
		uniquePeers[peer] = true
	}
	if len(uniquePeers) < 2 {
		t.Log("Warning: All files assigned to same peer, may indicate poor distribution")
	}

	// Test with empty peer list
	emptyLB := state.NewLoadBalancer([]string{})
	emptyDistribution := emptyLB.DistributeFiles(files)
	if len(emptyDistribution) != 0 {
		t.Error("Expected empty distribution for empty peer list")
	}

	emptyPeer := emptyLB.GetPeerForFile("test")
	if emptyPeer != "" {
		t.Errorf("Expected empty string for GetPeerForFile with empty peer list, got %s", emptyPeer)
	}

	// Test with single peer
	singleLB := state.NewLoadBalancer([]string{"only-peer"})
	singlePeer := singleLB.GetPeerForFile("test")
	if singlePeer != "only-peer" {
		t.Errorf("Expected 'only-peer' for single peer load balancer, got %s", singlePeer)
	}
}
