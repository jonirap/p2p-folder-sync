package discovery

import (
	"fmt"
	"sync"
	"time"
)

// PeerDiscoveryCallback is called when a new peer is discovered
type PeerDiscoveryCallback func(peerID string, address string, port int, capabilities map[string]interface{}, version string)

// PeerInfo represents discovered peer information
type PeerInfo struct {
	PeerID      string
	Address     string
	Port        int
	Capabilities map[string]interface{}
	Version     string
	LastSeen    time.Time
}

// Registry maintains a registry of discovered peers
type Registry struct {
	peers      map[string]*PeerInfo
	mu         sync.RWMutex
	callbacks  []PeerDiscoveryCallback
}

// NewRegistry creates a new peer registry
func NewRegistry() *Registry {
	return &Registry{
		peers:     make(map[string]*PeerInfo),
		callbacks: make([]PeerDiscoveryCallback, 0),
	}
}

// AddOrUpdatePeer adds or updates a peer in the registry
func (r *Registry) AddOrUpdatePeer(peerID string, address string, port int, capabilities map[string]interface{}, version string) {
	r.mu.Lock()
	isNew := r.peers[peerID] == nil
	r.peers[peerID] = &PeerInfo{
		PeerID:      peerID,
		Address:     address,
		Port:        port,
		Capabilities: capabilities,
		Version:     version,
		LastSeen:    time.Now(),
	}
	r.mu.Unlock()

	// Notify callbacks if this is a new peer
	if isNew {
		r.notifyCallbacks(peerID, address, port, capabilities, version)
	}
}

// GetPeer retrieves a peer by ID
func (r *Registry) GetPeer(peerID string) (*PeerInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	peer, exists := r.peers[peerID]
	if !exists {
		return nil, fmt.Errorf("peer not found: %s", peerID)
	}
	return peer, nil
}

// GetAllPeers returns all registered peers
func (r *Registry) GetAllPeers() []*PeerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	peers := make([]*PeerInfo, 0, len(r.peers))
	for _, peer := range r.peers {
		peers = append(peers, peer)
	}
	return peers
}

// RemovePeer removes a peer from the registry
func (r *Registry) RemovePeer(peerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.peers, peerID)
}

// CleanupStalePeers removes peers that haven't been seen recently
func (r *Registry) CleanupStalePeers(maxAge time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	for peerID, peer := range r.peers {
		if now.Sub(peer.LastSeen) > maxAge {
			delete(r.peers, peerID)
		}
	}
}

// AddDiscoveryCallback adds a callback to be called when new peers are discovered
func (r *Registry) AddDiscoveryCallback(callback PeerDiscoveryCallback) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.callbacks = append(r.callbacks, callback)
}

// notifyCallbacks notifies all registered callbacks about a newly discovered peer
func (r *Registry) notifyCallbacks(peerID string, address string, port int, capabilities map[string]interface{}, version string) {
	r.mu.RLock()
	callbacks := make([]PeerDiscoveryCallback, len(r.callbacks))
	copy(callbacks, r.callbacks)
	r.mu.RUnlock()

	for _, callback := range callbacks {
		go callback(peerID, address, port, capabilities, version)
	}
}

