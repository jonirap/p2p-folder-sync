package state

import (
	"hash/fnv"
	"sort"

	"github.com/p2p-folder-sync/p2p-sync/internal/network/messages"
)

// LoadBalancer distributes file requests across multiple peers
type LoadBalancer struct {
	peers []string
}

// NewLoadBalancer creates a new load balancer
func NewLoadBalancer(peers []string) *LoadBalancer {
	// Sort peers for consistent hashing
	sortedPeers := make([]string, len(peers))
	copy(sortedPeers, peers)
	sort.Strings(sortedPeers)

	return &LoadBalancer{
		peers: sortedPeers,
	}
}

// DistributeFiles distributes files across peers using consistent hashing
func (lb *LoadBalancer) DistributeFiles(files []messages.FileManifestEntry) map[string][]messages.FileManifestEntry {
	if len(lb.peers) == 0 {
		return make(map[string][]messages.FileManifestEntry)
	}

	distribution := make(map[string][]messages.FileManifestEntry)
	for _, peer := range lb.peers {
		distribution[peer] = make([]messages.FileManifestEntry, 0)
	}

	// Use consistent hashing to assign files to peers
	for _, file := range files {
		peer := lb.selectPeer(file.FileID)
		distribution[peer] = append(distribution[peer], file)
	}

	return distribution
}

// selectPeer selects a peer for a file using consistent hashing
func (lb *LoadBalancer) selectPeer(fileID string) string {
	if len(lb.peers) == 0 {
		return ""
	}
	if len(lb.peers) == 1 {
		return lb.peers[0]
	}

	// Simple hash-based selection
	hash := fnv.New32a()
	hash.Write([]byte(fileID))
	hashValue := hash.Sum32()

	index := int(hashValue) % len(lb.peers)
	return lb.peers[index]
}

// GetPeerForFile returns the peer responsible for a specific file
func (lb *LoadBalancer) GetPeerForFile(fileID string) string {
	return lb.selectPeer(fileID)
}

