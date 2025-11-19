package system

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/p2p-folder-sync/p2p-sync/internal/config"
	"github.com/p2p-folder-sync/p2p-sync/internal/database"
	"github.com/p2p-folder-sync/p2p-sync/internal/network"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/connection"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/discovery"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/messages"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/transport"
	syncpkg "github.com/p2p-folder-sync/p2p-sync/internal/sync"
)

// NetworkPeerSetup represents a complete peer setup with real network components
type NetworkPeerSetup struct {
	ID          string
	Dir         string
	Config      *config.Config
	Database    *database.DB
	Transport   transport.Transport
	Messenger   syncpkg.Messenger
	SyncEngine  *syncpkg.Engine
	ConnManager *connection.ConnectionManager
	Registry    *discovery.Registry
	Discovery   *discovery.UDPDiscovery
	Cleanup     func()
}

// NetworkTestHelper provides utilities for setting up real network integration tests
type NetworkTestHelper struct {
	baseDir     string
	peerCounter int
	peers       []*NetworkPeerSetup
}

// NewNetworkTestHelper creates a new network test helper
func NewNetworkTestHelper(t *testing.T) *NetworkTestHelper {
	baseDir := t.TempDir()
	return &NetworkTestHelper{
		baseDir:     baseDir,
		peerCounter: 0,
		peers:       make([]*NetworkPeerSetup, 0),
	}
}

// SetupPeer creates and configures a complete peer with real network components
func (nth *NetworkTestHelper) SetupPeer(t *testing.T, peerName string, enableEncryption bool) (*NetworkPeerSetup, error) {
	nth.peerCounter++
	peerID := fmt.Sprintf("%s-%d", peerName, nth.peerCounter)

	// Create peer directory
	peerDir := filepath.Join(nth.baseDir, peerID)
	if err := os.MkdirAll(peerDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create peer dir: %w", err)
	}

	// Get available port
	port := getAvailablePort(t)

	// Create configuration
	cfg := createTestConfig(peerDir)
	cfg.Network.Port = port

	if enableEncryption {
		cfg.Security.EncryptionAlgorithm = "aes-256-gcm"
		cfg.Security.KeyRotationInterval = 3600 // 1 hour for testing
	}

	// Initialize database
	db, err := database.NewDB(filepath.Join(peerDir, ".p2p-sync", fmt.Sprintf("%s.db", peerID)))
	if err != nil {
		return nil, fmt.Errorf("failed to create database: %w", err)
	}

	// Initialize connection manager
	connManager := connection.NewConnectionManagerWithConfig(cfg)

	// Initialize transport
	transport := transport.NewTCPTransport(cfg.Network.Port)

	// Initialize network messenger
	networkMessenger, err := network.NewNetworkMessenger(cfg, connManager, transport, peerID)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create messenger: %w", err)
	}

	// Initialize discovery registry
	peerRegistry := discovery.NewRegistry()

	// Register callback to automatically connect to discovered peers
	peerRegistry.AddDiscoveryCallback(func(discoveredPeerID string, address string, port int, capabilities map[string]interface{}, version string) {
		// Only connect if we don't already have a connection
		if _, err := connManager.GetConnection(discoveredPeerID); err != nil {
			// Attempt to connect to the discovered peer
			if err := networkMessenger.ConnectToPeer(discoveredPeerID, address, port); err != nil {
				// Log error but don't fail - discovery is best effort
				t.Logf("Failed to connect to discovered peer %s: %v", discoveredPeerID, err)
			}
		}
	})

	// Initialize UDP discovery service
	capabilities := map[string]interface{}{
		"encryption":  cfg.Security.EncryptionAlgorithm != "",
		"compression": cfg.Compression.Enabled,
		"chunking":    true,
	}
	udpDiscovery := discovery.NewUDPDiscoveryWithRegistry(cfg.Network.DiscoveryPort, peerID, capabilities, "1.0", peerRegistry)
	if err := udpDiscovery.Start(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to start UDP discovery: %w", err)
	}

	// Initialize sync engine
	syncEngine, err := syncpkg.NewEngineWithMessenger(cfg, db, peerID, networkMessenger)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create sync engine: %w", err)
	}

	// Set message handler on the network messenger
	networkMessenger.SetMessageHandler(syncEngine)

	// Create cleanup function
	cleanup := func() {
		syncEngine.Stop()
		transport.Stop()
		if udpDiscovery != nil {
			udpDiscovery.Stop()
		}
		connManager.Stop()
		db.Close()
	}

	peerSetup := &NetworkPeerSetup{
		ID:          peerID,
		Dir:         peerDir,
		Config:      cfg,
		Database:    db,
		Transport:   transport,
		Messenger:   networkMessenger,
		SyncEngine:  syncEngine,
		ConnManager: connManager,
		Registry:    peerRegistry,
		Discovery:   udpDiscovery,
		Cleanup:     cleanup,
	}

	// Track the peer for cleanup
	nth.peers = append(nth.peers, peerSetup)

	return peerSetup, nil
}

// ConnectPeers establishes real network connections between peers
func (nth *NetworkTestHelper) ConnectPeers(peers []*NetworkPeerSetup) error {
	// Connect each peer to all other peers
	for i, peer1 := range peers {
		for j, peer2 := range peers {
			if i == j {
				continue // Don't connect peer to itself
			}

			// Add peer2 to peer1's configuration
			peer1.Config.Network.Peers = append(peer1.Config.Network.Peers,
				fmt.Sprintf("localhost:%d", peer2.Config.Network.Port))

			// Generate a shared session key for both peers
			// In a real implementation, this would be done through key exchange
			sharedSessionKey := make([]byte, 32)
			if _, err := rand.Read(sharedSessionKey); err != nil {
				return fmt.Errorf("failed to generate shared session key: %w", err)
			}

			// Initiate connection from peer1 to peer2
			if err := peer1.Messenger.ConnectToPeer(peer2.ID, "localhost", peer2.Config.Network.Port); err != nil {
				return fmt.Errorf("failed to connect peer %s to %s: %w", peer1.ID, peer2.ID, err)
			}

			// Set the session key for outgoing communication
			if err := peer1.ConnManager.SetSessionKey(peer2.ID, sharedSessionKey); err != nil {
				return fmt.Errorf("failed to set shared session key for peer %s: %w", peer1.ID, err)
			}

			// For test purposes, simulate the bidirectional connection
			// The incoming connection will be registered by TCPTransport.handleConnection()
			// when peer2 receives the connection, but we need to set up peer2's side too
			peer2.ConnManager.AddConnection(peer1.ID, "localhost", peer1.Config.Network.Port)
			peer2.ConnManager.UpdateConnectionState(peer1.ID, connection.StateConnected)

			// Set the same shared session key for peer2 to communicate back to peer1
			if err := peer2.ConnManager.SetSessionKey(peer1.ID, sharedSessionKey); err != nil {
				return fmt.Errorf("failed to set shared session key for peer %s: %w", peer2.ID, err)
			}

			// Wait for connection to be established (both directions)
			if err := nth.waitForConnection(peer1, peer2, 10*time.Second); err != nil {
				return fmt.Errorf("failed to establish connection between peer %s and %s: %w", peer1.ID, peer2.ID, err)
			}
		}
	}
	return nil
}

// cleanupAll cleans up all tracked peers
func (nth *NetworkTestHelper) cleanupAll() {
	for _, peer := range nth.peers {
		if peer.Cleanup != nil {
			peer.Cleanup()
		}
	}
	// Clear the peers list
	nth.peers = nth.peers[:0]
}

// waitForConnection waits for a connection to be established between two peers
func (nth *NetworkTestHelper) waitForConnection(peer1, peer2 *NetworkPeerSetup, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for connection between %s and %s", peer1.ID, peer2.ID)
		case <-ticker.C:
			// Check if peer2 is connected to peer1
			conn, err := peer1.ConnManager.GetConnection(peer2.ID)
			if err == nil && conn != nil {
				// Connection established
				return nil
			}
		}
	}
}

// NetworkOperationMonitoringMessenger wraps Messenger to intercept operations
type NetworkOperationMonitoringMessenger struct {
	innerMessenger syncpkg.Messenger
	monitor        *NetworkOperationMonitor
	peer1Waiter    *SyncOperationWaiter
	peer2Waiter    *SyncOperationWaiter
}

func NewNetworkOperationMonitoringMessenger(inner syncpkg.Messenger, monitor *NetworkOperationMonitor) *NetworkOperationMonitoringMessenger {
	return &NetworkOperationMonitoringMessenger{
		innerMessenger: inner,
		monitor:        monitor,
		peer1Waiter:    nil,
		peer2Waiter:    nil,
	}
}

func NewNetworkOperationMonitoringMessengerWithWaiters(inner syncpkg.Messenger, monitor *NetworkOperationMonitor, peer1Waiter, peer2Waiter *SyncOperationWaiter) *NetworkOperationMonitoringMessenger {
	return &NetworkOperationMonitoringMessenger{
		innerMessenger: inner,
		monitor:        monitor,
		peer1Waiter:    peer1Waiter,
		peer2Waiter:    peer2Waiter,
	}
}

func (nomm *NetworkOperationMonitoringMessenger) SendFile(peerID string, fileData []byte, metadata *syncpkg.SyncOperation) error {
	return nomm.innerMessenger.SendFile(peerID, fileData, metadata)
}

func (nomm *NetworkOperationMonitoringMessenger) BroadcastOperation(op *syncpkg.SyncOperation) error {
	// Notify the monitor
	nomm.monitor.OnOperationQueued(op)

	// Notify the appropriate waiter based on the sender
	if op.PeerID == "peer1" && nomm.peer1Waiter != nil {
		nomm.peer1Waiter.OnOperationQueued(op)
	} else if op.PeerID == "peer2" && nomm.peer2Waiter != nil {
		nomm.peer2Waiter.OnOperationQueued(op)
	}

	return nomm.innerMessenger.BroadcastOperation(op)
}

func (nomm *NetworkOperationMonitoringMessenger) RequestStateSync(peerID string) error {
	return nomm.innerMessenger.RequestStateSync(peerID)
}

func (nomm *NetworkOperationMonitoringMessenger) ConnectToPeer(peerID, address string, port int) error {
	if networkMessenger, ok := nomm.innerMessenger.(*network.NetworkMessenger); ok {
		return networkMessenger.ConnectToPeer(peerID, address, port)
	}
	return fmt.Errorf("inner messenger does not support ConnectToPeer")
}

// NetworkOperationMonitor monitors network operations and messages
type NetworkOperationMonitor struct {
	mu           sync.RWMutex
	sentMessages []*messages.Message
	recvMessages []*messages.Message
	operations   []*syncpkg.SyncOperation
}

// NewNetworkOperationMonitor creates a new network operation monitor
func NewNetworkOperationMonitor() *NetworkOperationMonitor {
	return &NetworkOperationMonitor{
		sentMessages: make([]*messages.Message, 0),
		recvMessages: make([]*messages.Message, 0),
		operations:   make([]*syncpkg.SyncOperation, 0),
	}
}

// OnMessageSent records a sent message
func (nom *NetworkOperationMonitor) OnMessageSent(msg *messages.Message) {
	nom.mu.Lock()
	defer nom.mu.Unlock()
	nom.sentMessages = append(nom.sentMessages, msg)
}

// OnMessageReceived records a received message
func (nom *NetworkOperationMonitor) OnMessageReceived(msg *messages.Message) {
	nom.mu.Lock()
	defer nom.mu.Unlock()
	nom.recvMessages = append(nom.recvMessages, msg)
}

// OnOperationQueued records a sync operation
func (nom *NetworkOperationMonitor) OnOperationQueued(op *syncpkg.SyncOperation) {
	nom.mu.Lock()
	defer nom.mu.Unlock()
	nom.operations = append(nom.operations, op)
}

// GetSentMessages returns all sent messages
func (nom *NetworkOperationMonitor) GetSentMessages() []*messages.Message {
	nom.mu.RLock()
	defer nom.mu.RUnlock()
	result := make([]*messages.Message, len(nom.sentMessages))
	copy(result, nom.sentMessages)
	return result
}

// GetReceivedMessages returns all received messages
func (nom *NetworkOperationMonitor) GetReceivedMessages() []*messages.Message {
	nom.mu.RLock()
	defer nom.mu.RUnlock()
	result := make([]*messages.Message, len(nom.recvMessages))
	copy(result, nom.recvMessages)
	return result
}

// GetAllOperations returns all queued operations
func (nom *NetworkOperationMonitor) GetAllOperations() []*syncpkg.SyncOperation {
	nom.mu.RLock()
	defer nom.mu.RUnlock()
	result := make([]*syncpkg.SyncOperation, len(nom.operations))
	copy(result, nom.operations)
	return result
}

// GetOperationsByType returns operations of a specific type
func (nom *NetworkOperationMonitor) GetOperationsByType(opType syncpkg.OperationType) []*syncpkg.SyncOperation {
	nom.mu.RLock()
	defer nom.mu.RUnlock()

	var result []*syncpkg.SyncOperation
	for _, op := range nom.operations {
		if op.Type == opType {
			result = append(result, op)
		}
	}
	return result
}

// WaitForOperationType waits for an operation of the specified type
func (nom *NetworkOperationMonitor) WaitForOperationType(opType syncpkg.OperationType, timeout time.Duration) (*syncpkg.SyncOperation, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout waiting for operation type %s", opType)
		case <-ticker.C:
			if ops := nom.GetOperationsByType(opType); len(ops) > 0 {
				return ops[0], nil
			}
		}
	}
}

// Clear resets all monitoring data
func (nom *NetworkOperationMonitor) Clear() {
	nom.mu.Lock()
	defer nom.mu.Unlock()
	nom.sentMessages = nom.sentMessages[:0]
	nom.recvMessages = nom.recvMessages[:0]
	nom.operations = nom.operations[:0]
}

// NetworkMessageInterceptor wraps transport to intercept messages for monitoring
type NetworkMessageInterceptor struct {
	innerTransport transport.Transport
	monitor        *NetworkOperationMonitor
}

func NewNetworkMessageInterceptor(transport transport.Transport, monitor *NetworkOperationMonitor) *NetworkMessageInterceptor {
	return &NetworkMessageInterceptor{
		innerTransport: transport,
		monitor:        monitor,
	}
}

func (nmi *NetworkMessageInterceptor) SendMessage(peerID string, address string, port int, msg *messages.Message) error {
	// Record the sent message
	nmi.monitor.OnMessageSent(msg)

	// Send through the inner transport
	return nmi.innerTransport.SendMessage(peerID, address, port, msg)
}

func (nmi *NetworkMessageInterceptor) Start() error {
	return nmi.innerTransport.Start()
}

func (nmi *NetworkMessageInterceptor) Stop() error {
	return nmi.innerTransport.Stop()
}

func (nmi *NetworkMessageInterceptor) SetMessageHandler(handler transport.MessageHandler) error {
	return nmi.innerTransport.SetMessageHandler(handler)
}

func (nmi *NetworkMessageInterceptor) ConnectToPeer(peerID string, address string, port int) error {
	return nmi.innerTransport.ConnectToPeer(peerID, address, port)
}

// WaitForPeerConnections waits for all peers to be connected to each other
func WaitForPeerConnections(peers []*NetworkPeerSetup, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	expectedConnections := len(peers) * (len(peers) - 1) // Each peer connects to all others

	// Log initial state
	fmt.Printf("Waiting for %d total connections across %d peers (expected: %d connections per peer)\n",
		expectedConnections, len(peers), len(peers)-1)

	startTime := time.Now()
	attemptCount := 0

	for {
		select {
		case <-ctx.Done():
			// Provide detailed diagnostic information on timeout
			fmt.Printf("TIMEOUT: Failed to establish connections after %v\n", time.Since(startTime))

			// Print detailed connection status for each peer
			for _, peer := range peers {
				connections := peer.ConnManager.GetConnectedPeers()
				fmt.Printf("Peer %s: %d/%d connections (%v)\n",
					peer.ID, len(connections), len(peers)-1, connections)

				// Print all connection states for debugging
				allConns := peer.ConnManager.GetAllConnections()
				for _, conn := range allConns {
					fmt.Printf("  Connection to %s: %s\n", conn.PeerID, conn.State)
				}
			}

			return fmt.Errorf("timeout waiting for peer connections after %d attempts", attemptCount)
		case <-ticker.C:
			attemptCount++
			actualConnections := 0
			totalConnectedPeers := 0

			for _, peer := range peers {
				connections := peer.ConnManager.GetConnectedPeers()
				actualConnections += len(connections)
				if len(connections) > 0 {
					totalConnectedPeers++
				}
			}

			// Log progress every 5 attempts (1 second)
			if attemptCount%5 == 0 {
				fmt.Printf("Attempt %d: %d/%d total connections, %d/%d peers have connections\n",
					attemptCount, actualConnections, expectedConnections, totalConnectedPeers, len(peers))
			}

			if actualConnections >= expectedConnections {
				fmt.Printf("SUCCESS: All connections established after %v (%d attempts)\n",
					time.Since(startTime), attemptCount)
				return nil // All peers are connected
			}
		}
	}
}

// WaitForMessageDelivery waits for a message to be delivered between peers
func WaitForMessageDelivery(monitor *NetworkOperationMonitor, messageType string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for message delivery of type %s", messageType)
		case <-ticker.C:
			messages := monitor.GetReceivedMessages()
			for _, msg := range messages {
				if msg.Type == messageType {
					return nil // Message delivered
				}
			}
		}
	}
}

// WaitForOperationPropagation waits for an operation to be received by all peers
func WaitForOperationPropagation(monitors []*NetworkOperationMonitor, operationType syncpkg.OperationType, expectedCount int, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for operation propagation")
		case <-ticker.C:
			totalOperations := 0
			for _, monitor := range monitors {
				operations := monitor.GetOperationsByType(operationType)
				totalOperations += len(operations)
			}

			if totalOperations >= expectedCount {
				return nil // All operations propagated
			}
		}
	}
}

// ConnectionHealthChecker provides utilities to check peer connection health
type ConnectionHealthChecker struct {
	peers []*NetworkPeerSetup
}

// NewConnectionHealthChecker creates a new connection health checker
func NewConnectionHealthChecker(peers []*NetworkPeerSetup) *ConnectionHealthChecker {
	return &ConnectionHealthChecker{peers: peers}
}

// CheckAllConnections returns the health status of all peer connections
func (chc *ConnectionHealthChecker) CheckAllConnections() ConnectionHealthStatus {
	status := ConnectionHealthStatus{
		TotalPeers:        len(chc.peers),
		ConnectedPeers:    0,
		DisconnectedPeers: 0,
		PeerStatuses:      make(map[string]PeerConnectionStatus),
	}

	for _, peer := range chc.peers {
		connections := peer.ConnManager.GetConnectedPeers()
		expectedConnections := len(chc.peers) - 1 // All other peers

		peerStatus := PeerConnectionStatus{
			PeerID:              peer.ID,
			ExpectedConnections: expectedConnections,
			ActualConnections:   len(connections),
			IsFullyConnected:    len(connections) >= expectedConnections,
			ConnectedPeers:      connections,
		}

		status.PeerStatuses[peer.ID] = peerStatus

		if peerStatus.IsFullyConnected {
			status.ConnectedPeers++
		} else {
			status.DisconnectedPeers++
		}
	}

	status.IsFullyConnected = status.DisconnectedPeers == 0
	return status
}

// WaitForHealthyConnections waits until all peers have healthy connections
func (chc *ConnectionHealthChecker) WaitForHealthyConnections(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			status := chc.CheckAllConnections()
			return fmt.Errorf("timeout waiting for healthy connections. Status: %+v", status)
		case <-ticker.C:
			status := chc.CheckAllConnections()
			if status.IsFullyConnected {
				return nil // All connections healthy
			}
		}
	}
}

// ConnectionHealthStatus represents the overall health of peer connections
type ConnectionHealthStatus struct {
	TotalPeers        int
	ConnectedPeers    int
	DisconnectedPeers int
	IsFullyConnected  bool
	PeerStatuses      map[string]PeerConnectionStatus
}

// PeerConnectionStatus represents the connection status of a single peer
type PeerConnectionStatus struct {
	PeerID              string
	ExpectedConnections int
	ActualConnections   int
	IsFullyConnected    bool
	ConnectedPeers      []string
}

// MessageDeliveryTracker tracks message delivery between peers
type MessageDeliveryTracker struct {
	sentMessages      map[string]*messages.Message
	deliveredMessages map[string]bool
	monitor           *NetworkOperationMonitor
	mu                sync.RWMutex
}

// NewMessageDeliveryTracker creates a new message delivery tracker
func NewMessageDeliveryTracker(monitor *NetworkOperationMonitor) *MessageDeliveryTracker {
	return &MessageDeliveryTracker{
		sentMessages:      make(map[string]*messages.Message),
		deliveredMessages: make(map[string]bool),
		monitor:           monitor,
	}
}

// TrackMessage records that a message was sent
func (mdt *MessageDeliveryTracker) TrackMessage(msg *messages.Message) {
	mdt.mu.Lock()
	defer mdt.mu.Unlock()
	mdt.sentMessages[msg.ID] = msg
}

// ConfirmDelivery marks a message as delivered
func (mdt *MessageDeliveryTracker) ConfirmDelivery(messageID string) {
	mdt.mu.Lock()
	defer mdt.mu.Unlock()
	mdt.deliveredMessages[messageID] = true
}

// WaitForDelivery waits for a specific message to be delivered
func (mdt *MessageDeliveryTracker) WaitForDelivery(messageID string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for message %s delivery", messageID)
		case <-ticker.C:
			mdt.mu.RLock()
			delivered := mdt.deliveredMessages[messageID]
			mdt.mu.RUnlock()

			if delivered {
				return nil // Message delivered
			}
		}
	}
}

// GetDeliveryStatus returns the delivery status of all tracked messages
func (mdt *MessageDeliveryTracker) GetDeliveryStatus() MessageDeliveryStatus {
	mdt.mu.RLock()
	defer mdt.mu.RUnlock()

	status := MessageDeliveryStatus{
		TotalSent:       len(mdt.sentMessages),
		TotalDelivered:  0,
		PendingDelivery: make([]string, 0),
	}

	for messageID, delivered := range mdt.deliveredMessages {
		if delivered {
			status.TotalDelivered++
		} else {
			status.PendingDelivery = append(status.PendingDelivery, messageID)
		}
	}

	// Add messages that haven't been marked as delivered yet
	for messageID := range mdt.sentMessages {
		if !mdt.deliveredMessages[messageID] {
			found := false
			for _, pending := range status.PendingDelivery {
				if pending == messageID {
					found = true
					break
				}
			}
			if !found {
				status.PendingDelivery = append(status.PendingDelivery, messageID)
			}
		}
	}

	status.DeliveryRate = float64(status.TotalDelivered) / float64(status.TotalSent) * 100.0
	return status
}

// MessageDeliveryStatus represents the delivery status of messages
type MessageDeliveryStatus struct {
	TotalSent       int
	TotalDelivered  int
	DeliveryRate    float64
	PendingDelivery []string
}

// SynchronizationWaiter provides utilities for waiting on various sync operations
type SynchronizationWaiter struct {
	timeout time.Duration
}

// NewSynchronizationWaiter creates a new synchronization waiter
func NewSynchronizationWaiter(timeout time.Duration) *SynchronizationWaiter {
	return &SynchronizationWaiter{timeout: timeout}
}

// WaitForFileConvergence waits for all peers to have the same version of a file
func (sw *SynchronizationWaiter) WaitForFileConvergence(peers []*NetworkPeerSetup, filename string) error {
	ctx, cancel := context.WithTimeout(context.Background(), sw.timeout)
	defer cancel()

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for file convergence: %s", filename)
		case <-ticker.C:
			if sw.checkFileConvergence(peers, filename) {
				return nil // All peers have converged
			}
		}
	}
}

// checkFileConvergence checks if all peers have the same version of a file
func (sw *SynchronizationWaiter) checkFileConvergence(peers []*NetworkPeerSetup, filename string) bool {
	if len(peers) < 2 {
		return true
	}

	// Get content from first peer
	firstContent, err := os.ReadFile(filepath.Join(peers[0].Dir, filename))
	if err != nil {
		return false
	}

	// Check all other peers have the same content
	for i := 1; i < len(peers); i++ {
		otherContent, err := os.ReadFile(filepath.Join(peers[i].Dir, filename))
		if err != nil || string(firstContent) != string(otherContent) {
			return false
		}
	}

	return true
}

// WaitForStateSynchronization waits for all peers to reach a synchronized state
func (sw *SynchronizationWaiter) WaitForStateSynchronization(peers []*NetworkPeerSetup, expectedFiles map[string]bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), sw.timeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for state synchronization")
		case <-ticker.C:
			if sw.checkStateSynchronization(peers, expectedFiles) {
				return nil // State synchronized
			}
		}
	}
}

// checkStateSynchronization checks if all peers have the expected files
func (sw *SynchronizationWaiter) checkStateSynchronization(peers []*NetworkPeerSetup, expectedFiles map[string]bool) bool {
	for _, peer := range peers {
		files, err := os.ReadDir(peer.Dir)
		if err != nil {
			return false
		}

		peerFiles := make(map[string]bool)
		for _, file := range files {
			if strings.HasPrefix(file.Name(), ".") {
				continue // Skip hidden files
			}
			peerFiles[file.Name()] = true
		}

		// Check that peer has all expected files
		for filename := range expectedFiles {
			if !peerFiles[filename] {
				return false
			}
		}

		// Check that peer doesn't have unexpected files
		for filename := range peerFiles {
			if !expectedFiles[filename] {
				return false
			}
		}
	}

	return true
}
