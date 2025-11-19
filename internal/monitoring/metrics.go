package monitoring

import (
	"sync"
	"time"
)

// Metrics collects and tracks system-wide metrics for monitoring
type Metrics struct {
	// Sync operation metrics
	syncOps      SyncOperationMetrics
	syncOpsMu    sync.RWMutex

	// Network metrics
	network      NetworkMetrics
	networkMu    sync.RWMutex

	// Flow control metrics
	flowControl  FlowControlMetrics
	flowControlMu sync.RWMutex

	// Peer metrics
	peers        PeerMetrics
	peersMu      sync.RWMutex

	// Conflict metrics
	conflicts    ConflictMetrics
	conflictsMu  sync.RWMutex

	// Error metrics
	errors       ErrorMetrics
	errorsMu     sync.RWMutex

	// Start time for uptime calculation
	startTime    time.Time
}

// SyncOperationMetrics tracks file synchronization operations
type SyncOperationMetrics struct {
	TotalOperations    int64
	CreateOperations   int64
	UpdateOperations   int64
	DeleteOperations   int64
	RenameOperations   int64
	FailedOperations   int64
	LastOperationTime  time.Time
	BytesSynced        int64
	FilesSynced        int64
}

// NetworkMetrics tracks network statistics
type NetworkMetrics struct {
	BytesSent          int64
	BytesReceived      int64
	MessagesSent       int64
	MessagesReceived   int64
	ChunksSent         int64
	ChunksReceived     int64
	ConnectionAttempts int64
	FailedConnections  int64
	ActiveConnections  int64
}

// FlowControlMetrics tracks bandwidth and concurrency control
type FlowControlMetrics struct {
	ActiveTransfers    int64
	TotalTransfers     int64
	QueuedTransfers    int64
	BandwidthUsage     int64  // Current bytes/sec
	PeakBandwidth      int64  // Peak bytes/sec observed
	ThrottledOperations int64 // Number of times bandwidth limiting kicked in
}

// PeerMetrics tracks peer connection statistics
type PeerMetrics struct {
	ConnectedPeers     int64
	TotalPeersDiscovered int64
	PeerConnectTime    map[string]time.Time
	PeerDisconnects    int64
	PeerReconnects     int64
}

// ConflictMetrics tracks conflict resolution statistics
type ConflictMetrics struct {
	TotalConflicts     int64
	ResolvedConflicts  int64
	PendingConflicts   int64
	LWWResolutions     int64  // Last-Write-Wins resolutions
	ThreeWayMerges     int64  // 3-way merge resolutions
	ManualResolutions  int64  // Requiring manual intervention
}

// ErrorMetrics tracks errors and failures
type ErrorMetrics struct {
	TotalErrors        int64
	NetworkErrors      int64
	FileSystemErrors   int64
	SyncErrors         int64
	EncryptionErrors   int64
	LastError          string
	LastErrorTime      time.Time
}

// NewMetrics creates a new metrics collector
func NewMetrics() *Metrics {
	return &Metrics{
		startTime: time.Now(),
		peers: PeerMetrics{
			PeerConnectTime: make(map[string]time.Time),
		},
	}
}

// RecordSyncOperation records a sync operation
func (m *Metrics) RecordSyncOperation(opType string, bytes int64, success bool) {
	m.syncOpsMu.Lock()
	defer m.syncOpsMu.Unlock()

	m.syncOps.TotalOperations++
	m.syncOps.LastOperationTime = time.Now()

	if success {
		m.syncOps.BytesSynced += bytes
		m.syncOps.FilesSynced++

		switch opType {
		case "create":
			m.syncOps.CreateOperations++
		case "update":
			m.syncOps.UpdateOperations++
		case "delete":
			m.syncOps.DeleteOperations++
		case "rename":
			m.syncOps.RenameOperations++
		}
	} else {
		m.syncOps.FailedOperations++
	}
}

// RecordNetworkTraffic records network bytes sent/received
func (m *Metrics) RecordNetworkTraffic(sent, received int64) {
	m.networkMu.Lock()
	defer m.networkMu.Unlock()

	m.network.BytesSent += sent
	m.network.BytesReceived += received
}

// RecordMessage records a message sent or received
func (m *Metrics) RecordMessage(sent bool) {
	m.networkMu.Lock()
	defer m.networkMu.Unlock()

	if sent {
		m.network.MessagesSent++
	} else {
		m.network.MessagesReceived++
	}
}

// RecordChunk records a chunk sent or received
func (m *Metrics) RecordChunk(sent bool) {
	m.networkMu.Lock()
	defer m.networkMu.Unlock()

	if sent {
		m.network.ChunksSent++
	} else {
		m.network.ChunksReceived++
	}
}

// RecordConnection records a connection attempt
func (m *Metrics) RecordConnection(success bool) {
	m.networkMu.Lock()
	defer m.networkMu.Unlock()

	m.network.ConnectionAttempts++
	if !success {
		m.network.FailedConnections++
	}
}

// SetActiveConnections sets the current number of active connections
func (m *Metrics) SetActiveConnections(count int64) {
	m.networkMu.Lock()
	defer m.networkMu.Unlock()

	m.network.ActiveConnections = count
}

// UpdateFlowControl updates flow control metrics
func (m *Metrics) UpdateFlowControl(active, queued, bandwidth int64) {
	m.flowControlMu.Lock()
	defer m.flowControlMu.Unlock()

	m.flowControl.ActiveTransfers = active
	m.flowControl.QueuedTransfers = queued
	m.flowControl.BandwidthUsage = bandwidth

	// Track peak bandwidth
	if bandwidth > m.flowControl.PeakBandwidth {
		m.flowControl.PeakBandwidth = bandwidth
	}
}

// RecordTransfer records a file transfer
func (m *Metrics) RecordTransfer() {
	m.flowControlMu.Lock()
	defer m.flowControlMu.Unlock()

	m.flowControl.TotalTransfers++
}

// RecordThrottle records a throttling event
func (m *Metrics) RecordThrottle() {
	m.flowControlMu.Lock()
	defer m.flowControlMu.Unlock()

	m.flowControl.ThrottledOperations++
}

// RecordPeerConnect records a peer connection
func (m *Metrics) RecordPeerConnect(peerID string) {
	m.peersMu.Lock()
	defer m.peersMu.Unlock()

	m.peers.ConnectedPeers++
	m.peers.TotalPeersDiscovered++
	m.peers.PeerConnectTime[peerID] = time.Now()
}

// RecordPeerDisconnect records a peer disconnection
func (m *Metrics) RecordPeerDisconnect(peerID string) {
	m.peersMu.Lock()
	defer m.peersMu.Unlock()

	m.peers.ConnectedPeers--
	m.peers.PeerDisconnects++
	delete(m.peers.PeerConnectTime, peerID)
}

// RecordConflict records a conflict detection
func (m *Metrics) RecordConflict(resolutionType string) {
	m.conflictsMu.Lock()
	defer m.conflictsMu.Unlock()

	m.conflicts.TotalConflicts++
	m.conflicts.PendingConflicts++

	switch resolutionType {
	case "lww":
		m.conflicts.LWWResolutions++
	case "3way":
		m.conflicts.ThreeWayMerges++
	case "manual":
		m.conflicts.ManualResolutions++
	}
}

// RecordConflictResolution records a conflict resolution
func (m *Metrics) RecordConflictResolution() {
	m.conflictsMu.Lock()
	defer m.conflictsMu.Unlock()

	m.conflicts.ResolvedConflicts++
	m.conflicts.PendingConflicts--
}

// RecordError records an error
func (m *Metrics) RecordError(errorType, errorMsg string) {
	m.errorsMu.Lock()
	defer m.errorsMu.Unlock()

	m.errors.TotalErrors++
	m.errors.LastError = errorMsg
	m.errors.LastErrorTime = time.Now()

	switch errorType {
	case "network":
		m.errors.NetworkErrors++
	case "filesystem":
		m.errors.FileSystemErrors++
	case "sync":
		m.errors.SyncErrors++
	case "encryption":
		m.errors.EncryptionErrors++
	}
}

// GetSnapshot returns a snapshot of all metrics
func (m *Metrics) GetSnapshot() MetricsSnapshot {
	m.syncOpsMu.RLock()
	syncOps := m.syncOps
	m.syncOpsMu.RUnlock()

	m.networkMu.RLock()
	network := m.network
	m.networkMu.RUnlock()

	m.flowControlMu.RLock()
	flowControl := m.flowControl
	m.flowControlMu.RUnlock()

	m.peersMu.RLock()
	peers := m.peers
	m.peersMu.RUnlock()

	m.conflictsMu.RLock()
	conflicts := m.conflicts
	m.conflictsMu.RUnlock()

	m.errorsMu.RLock()
	errors := m.errors
	m.errorsMu.RUnlock()

	return MetricsSnapshot{
		SyncOperations: syncOps,
		Network:        network,
		FlowControl:    flowControl,
		Peers:          peers,
		Conflicts:      conflicts,
		Errors:         errors,
		Uptime:         time.Since(m.startTime),
		Timestamp:      time.Now(),
	}
}

// MetricsSnapshot represents a point-in-time snapshot of all metrics
type MetricsSnapshot struct {
	SyncOperations SyncOperationMetrics
	Network        NetworkMetrics
	FlowControl    FlowControlMetrics
	Peers          PeerMetrics
	Conflicts      ConflictMetrics
	Errors         ErrorMetrics
	Uptime         time.Duration
	Timestamp      time.Time
}

// GetSummary returns a human-readable summary of key metrics
func (s *MetricsSnapshot) GetSummary() map[string]interface{} {
	return map[string]interface{}{
		"uptime_seconds":          s.Uptime.Seconds(),
		"total_operations":        s.SyncOperations.TotalOperations,
		"files_synced":            s.SyncOperations.FilesSynced,
		"bytes_synced":            s.SyncOperations.BytesSynced,
		"connected_peers":         s.Peers.ConnectedPeers,
		"active_transfers":        s.FlowControl.ActiveTransfers,
		"bandwidth_usage_bps":     s.FlowControl.BandwidthUsage,
		"total_conflicts":         s.Conflicts.TotalConflicts,
		"resolved_conflicts":      s.Conflicts.ResolvedConflicts,
		"total_errors":            s.Errors.TotalErrors,
		"network_bytes_sent":      s.Network.BytesSent,
		"network_bytes_received":  s.Network.BytesReceived,
	}
}
