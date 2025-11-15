package connection

import (
	"time"

	"github.com/p2p-folder-sync/p2p-sync/internal/config"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/messages"
)

// HeartbeatManager manages heartbeat messages for connection health monitoring
type HeartbeatManager struct {
	connManager *ConnectionManager
	transport   Transport
	config      *config.Config
	stopCh      chan struct{}
}

// Transport interface for heartbeat manager
type Transport interface {
	SendMessage(peerID string, address string, port int, msg *messages.Message) error
}

// NewHeartbeatManager creates a new heartbeat manager
func NewHeartbeatManager(connManager *ConnectionManager, transport Transport, cfg *config.Config) *HeartbeatManager {
	return &HeartbeatManager{
		connManager: connManager,
		transport:   transport,
		config:      cfg,
		stopCh:      make(chan struct{}),
	}
}

// Start starts the heartbeat manager
func (hm *HeartbeatManager) Start() {
	go hm.sendHeartbeats()
}

// Stop stops the heartbeat manager
func (hm *HeartbeatManager) Stop() {
	close(hm.stopCh)
}

// sendHeartbeats sends periodic heartbeat messages to all connected peers
func (hm *HeartbeatManager) sendHeartbeats() {
	interval := time.Duration(hm.config.Network.HeartbeatInterval) * time.Second
	if interval <= 0 {
		interval = 30 * time.Second // Default to 30 seconds
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-hm.stopCh:
			return
		case <-ticker.C:
			hm.sendHeartbeatToAllPeers()
		}
	}
}

// sendHeartbeatToAllPeers sends heartbeat messages to all connected peers
func (hm *HeartbeatManager) sendHeartbeatToAllPeers() {
	connectedPeers := hm.connManager.GetConnectedPeers()

	for _, peerID := range connectedPeers {
		go hm.sendHeartbeatToPeer(peerID)
	}
}

// sendHeartbeatToPeer sends a heartbeat message to a specific peer
func (hm *HeartbeatManager) sendHeartbeatToPeer(peerID string) {
	// Get connection info
	conn, err := hm.connManager.GetConnection(peerID)
	if err != nil {
		// Peer not found, skip
		return
	}

	// Create heartbeat message
	heartbeatMsg := messages.NewMessage(
		messages.TypeHeartbeat,
		"self", // Sender ID will be set by transport
		messages.HeartbeatMessage{
			Timestamp: time.Now().UnixMilli(),
		},
	)

	// Send heartbeat message
	err = hm.transport.SendMessage(peerID, conn.Address, conn.Port, heartbeatMsg)
	if err != nil {
		// Log error but don't fail - heartbeat failures will be detected by timeout
		return
	}

	// Update our own heartbeat timestamp (for this peer)
	hm.connManager.UpdateHeartbeat(peerID)
}

// HandleHeartbeatResponse handles incoming heartbeat responses
// This would be called by the message handler when a heartbeat is received
func (hm *HeartbeatManager) HandleHeartbeatResponse(peerID string) {
	// Update the peer's heartbeat timestamp
	hm.connManager.UpdateHeartbeat(peerID)
}

// GetHeartbeatInterval returns the current heartbeat interval
func (hm *HeartbeatManager) GetHeartbeatInterval() time.Duration {
	interval := time.Duration(hm.config.Network.HeartbeatInterval) * time.Second
	if interval <= 0 {
		return 30 * time.Second
	}
	return interval
}

// GetHeartbeatTimeout returns the heartbeat timeout (2x interval)
func (hm *HeartbeatManager) GetHeartbeatTimeout() time.Duration {
	return hm.GetHeartbeatInterval() * 2
}
