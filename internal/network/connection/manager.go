package connection

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/p2p-folder-sync/p2p-sync/internal/config"
)

// ConnectionState represents the state of a connection
type ConnectionState string

const (
	StateDisconnected ConnectionState = "disconnected"
	StateConnecting   ConnectionState = "connecting"
	StateConnected    ConnectionState = "connected"
)

// ConnectionEvent represents a connection event
type ConnectionEvent int

const (
	EventConnected ConnectionEvent = iota
	EventDisconnected
	EventHeartbeatTimeout
)

// ConnectionCallback is a callback function for connection events
type ConnectionCallback func(peerID string, event ConnectionEvent, conn *Connection)

// Connection represents a peer connection
type Connection struct {
	PeerID          string
	Address         string
	Port            int
	State           ConnectionState
	ConnectedAt     time.Time
	LastSeen        time.Time
	LastHeartbeat   time.Time
	SessionKey      []byte
	RetryCount      int
	LastRetryAt     time.Time
	mu              sync.RWMutex
}

// ConnectionManager manages peer connections
type ConnectionManager struct {
	connections map[string]*Connection
	mu          sync.RWMutex
	callbacks   []ConnectionCallback
	config      *config.Config
	stopCh      chan struct{}
}

// NewConnectionManager creates a new connection manager
func NewConnectionManager() *ConnectionManager {
	return &ConnectionManager{
		connections: make(map[string]*Connection),
		callbacks:   make([]ConnectionCallback, 0),
		stopCh:      make(chan struct{}),
	}
}

// NewConnectionManagerWithConfig creates a new connection manager with config
func NewConnectionManagerWithConfig(cfg *config.Config) *ConnectionManager {
	cm := NewConnectionManager()
	cm.config = cfg
	go cm.monitorConnections()
	return cm
}

// AddConnection adds a new connection
func (cm *ConnectionManager) AddConnection(peerID, address string, port int) *Connection {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	conn := &Connection{
		PeerID:      peerID,
		Address:     address,
		Port:        port,
		State:       StateConnected,
		ConnectedAt: time.Now(),
		LastSeen:    time.Now(),
	}

	cm.connections[peerID] = conn

	// Notify callbacks
	go cm.notifyCallbacks(peerID, EventConnected, conn)

	return conn
}

// GetConnection retrieves a connection by peer ID
func (cm *ConnectionManager) GetConnection(peerID string) (*Connection, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	conn, exists := cm.connections[peerID]
	if !exists {
		return nil, fmt.Errorf("connection not found: %s", peerID)
	}
	return conn, nil
}

// UpdateConnectionState updates the state of a connection
func (cm *ConnectionManager) UpdateConnectionState(peerID string, state ConnectionState) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	conn, exists := cm.connections[peerID]
	if !exists {
		return fmt.Errorf("connection not found: %s", peerID)
	}

	oldState := conn.State

	conn.mu.Lock()
	conn.State = state
	if state == StateConnected {
		conn.LastSeen = time.Now()
		conn.LastHeartbeat = time.Now()
	}
	conn.mu.Unlock()

	// Notify callbacks for state changes
	if oldState != state {
		event := EventDisconnected
		if state == StateConnected {
			event = EventConnected
		}
		go cm.notifyCallbacks(peerID, event, conn)
	}

	return nil
}

// RemoveConnection removes a connection
func (cm *ConnectionManager) RemoveConnection(peerID string) {
	cm.mu.Lock()
	conn, exists := cm.connections[peerID]
	cm.mu.Unlock()

	if exists {
		go cm.notifyCallbacks(peerID, EventDisconnected, conn)
	}

	cm.mu.Lock()
	delete(cm.connections, peerID)
	cm.mu.Unlock()
}

// GetAllConnections returns all connections
func (cm *ConnectionManager) GetAllConnections() []*Connection {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	connections := make([]*Connection, 0, len(cm.connections))
	for _, conn := range cm.connections {
		connections = append(connections, conn)
	}
	return connections
}

// GetConnectedPeers returns all connected peer IDs
func (cm *ConnectionManager) GetConnectedPeers() []string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var peers []string
	for peerID, conn := range cm.connections {
		conn.mu.RLock()
		if conn.State == StateConnected {
			peers = append(peers, peerID)
		}
		conn.mu.RUnlock()
	}
	return peers
}

// AddConnectionCallback adds a callback for connection events
func (cm *ConnectionManager) AddConnectionCallback(callback ConnectionCallback) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.callbacks = append(cm.callbacks, callback)
}

// UpdateHeartbeat updates the heartbeat timestamp for a peer
func (cm *ConnectionManager) UpdateHeartbeat(peerID string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	conn, exists := cm.connections[peerID]
	if !exists {
		return fmt.Errorf("connection not found: %s", peerID)
	}

	conn.mu.Lock()
	conn.LastHeartbeat = time.Now()
	conn.LastSeen = time.Now()
	conn.mu.Unlock()

	return nil
}

// GetSessionKey returns the session key for a peer
func (cm *ConnectionManager) GetSessionKey(peerID string) ([]byte, error) {
	conn, err := cm.GetConnection(peerID)
	if err != nil {
		return nil, err
	}

	conn.mu.RLock()
	defer conn.mu.RUnlock()
	return conn.SessionKey, nil
}

// SetSessionKey sets the session key for a peer
func (cm *ConnectionManager) SetSessionKey(peerID string, sessionKey []byte) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	conn, exists := cm.connections[peerID]
	if !exists {
		return fmt.Errorf("connection not found: %s", peerID)
	}

	conn.mu.Lock()
	conn.SessionKey = make([]byte, len(sessionKey))
	copy(conn.SessionKey, sessionKey)
	conn.mu.Unlock()

	return nil
}

// AttemptReconnection attempts to reconnect to a disconnected peer with exponential backoff
func (cm *ConnectionManager) AttemptReconnection(peerID string) error {
	cm.mu.Lock()
	conn, exists := cm.connections[peerID]
	cm.mu.Unlock()

	if !exists {
		return fmt.Errorf("connection not found: %s", peerID)
	}

	conn.mu.Lock()
	if conn.State == StateConnected {
		conn.mu.Unlock()
		return nil // Already connected
	}

	// Check if we should attempt reconnection
	now := time.Now()
	backoffDuration := cm.calculateBackoffDuration(conn.RetryCount)

	if now.Sub(conn.LastRetryAt) < backoffDuration {
		conn.mu.Unlock()
		return nil // Too soon to retry
	}

	conn.RetryCount++
	conn.LastRetryAt = now
	conn.State = StateConnecting
	conn.mu.Unlock()

	// Notify callbacks about reconnection attempt
	go cm.notifyCallbacks(peerID, EventDisconnected, conn) // Using disconnected as we're attempting to reconnect

	// TODO: Actually attempt the connection through transport
	// This would be called by the transport layer when connection succeeds/fails

	return nil
}

// calculateBackoffDuration calculates exponential backoff duration
func (cm *ConnectionManager) calculateBackoffDuration(retryCount int) time.Duration {
	if retryCount == 0 {
		return 0
	}

	// Exponential backoff: base 1s, max 5min, multiplier 2
	baseDelay := time.Second
	maxDelay := 5 * time.Minute
	multiplier := 2.0

	delay := float64(baseDelay) * math.Pow(multiplier, float64(retryCount-1))
	if delay > float64(maxDelay) {
		delay = float64(maxDelay)
	}

	return time.Duration(delay)
}

// monitorConnections monitors connection health and triggers reconnections
func (cm *ConnectionManager) monitorConnections() {
	if cm.config == nil {
		return
	}

	ticker := time.NewTicker(30 * time.Second) // Check every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-cm.stopCh:
			return
		case <-ticker.C:
			cm.checkConnectionHealth()
		}
	}
}

// checkConnectionHealth checks for heartbeat timeouts and triggers reconnections
func (cm *ConnectionManager) checkConnectionHealth() {
	if cm.config == nil {
		return
	}

	cm.mu.RLock()
	connections := make(map[string]*Connection)
	for k, v := range cm.connections {
		connections[k] = v
	}
	cm.mu.RUnlock()

	now := time.Now()
	heartbeatTimeout := time.Duration(cm.config.Network.HeartbeatInterval*2) * time.Second // 2x heartbeat interval

	for peerID, conn := range connections {
		conn.mu.RLock()
		timeSinceLastHeartbeat := now.Sub(conn.LastHeartbeat)
		isConnected := conn.State == StateConnected
		conn.mu.RUnlock()

		if isConnected && timeSinceLastHeartbeat > heartbeatTimeout {
			// Heartbeat timeout - mark as disconnected
			cm.UpdateConnectionState(peerID, StateDisconnected)
			go cm.notifyCallbacks(peerID, EventHeartbeatTimeout, conn)

			// Attempt reconnection
			go cm.AttemptReconnection(peerID)
		}
	}
}

// notifyCallbacks notifies all registered callbacks about a connection event
func (cm *ConnectionManager) notifyCallbacks(peerID string, event ConnectionEvent, conn *Connection) {
	cm.mu.RLock()
	callbacks := make([]ConnectionCallback, len(cm.callbacks))
	copy(callbacks, cm.callbacks)
	cm.mu.RUnlock()

	for _, callback := range callbacks {
		go callback(peerID, event, conn)
	}
}

// Stop stops the connection manager
func (cm *ConnectionManager) Stop() {
	close(cm.stopCh)
}

