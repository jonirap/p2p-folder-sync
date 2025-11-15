package system

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/p2p-folder-sync/p2p-sync/internal/config"
	"github.com/p2p-folder-sync/p2p-sync/internal/database"
	"github.com/p2p-folder-sync/p2p-sync/internal/network"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/connection"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/transport"
	syncpkg "github.com/p2p-folder-sync/p2p-sync/internal/sync"
)

// PeerSetup represents a complete peer setup for integration testing
type PeerSetup struct {
	ID          string
	Dir         string
	Config      *config.Config
	Database    *database.DB
	Transport   transport.Transport
	Messenger   syncpkg.Messenger
	SyncEngine  *syncpkg.Engine
	ConnManager *connection.ConnectionManager
	Cleanup     func()
}

// IntegrationTestHelper provides utilities for setting up integration tests
type IntegrationTestHelper struct {
	baseDir     string
	peerCounter int
}

// NewIntegrationTestHelper creates a new integration test helper
func NewIntegrationTestHelper(t *testing.T) *IntegrationTestHelper {
	baseDir := t.TempDir()
	return &IntegrationTestHelper{
		baseDir:     baseDir,
		peerCounter: 0,
	}
}

// SetupPeer creates and configures a complete peer for testing
func (ith *IntegrationTestHelper) SetupPeer(t *testing.T, peerName string, enableEncryption bool) (*PeerSetup, error) {
	ith.peerCounter++
	peerID := fmt.Sprintf("%s-%d", peerName, ith.peerCounter)

	// Create peer directory
	peerDir := filepath.Join(ith.baseDir, peerID)
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
		transport.Stop()
		return nil, fmt.Errorf("failed to create messenger: %w", err)
	}

	// Initialize sync engine
	syncEngine, err := syncpkg.NewEngineWithMessenger(cfg, db, peerID, networkMessenger)
	if err != nil {
		db.Close()
		transport.Stop()
		return nil, fmt.Errorf("failed to create sync engine: %w", err)
	}

	// Create cleanup function
	cleanup := func() {
		syncEngine.Stop()
		transport.Stop()
		connManager.Stop()
		db.Close()
	}

	return &PeerSetup{
		ID:          peerID,
		Dir:         peerDir,
		Config:      cfg,
		Database:    db,
		Transport:   transport,
		Messenger:   networkMessenger,
		SyncEngine:  syncEngine,
		ConnManager: connManager,
		Cleanup:     cleanup,
	}, nil
}

// ConnectPeers establishes connections between peers
func (ith *IntegrationTestHelper) ConnectPeers(peers []*PeerSetup) error {
	// Connect each peer to all other peers
	for i, peer1 := range peers {
		for j, peer2 := range peers {
			if i == j {
				continue // Don't connect peer to itself
			}

			// Add peer2 to peer1's configuration
			peer1.Config.Network.Peers = append(peer1.Config.Network.Peers,
				fmt.Sprintf("localhost:%d", peer2.Config.Network.Port))
		}
	}

	// Start all peers
	for _, peer := range peers {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

		if err := peer.Transport.Start(); err != nil {
			cancel()
			return fmt.Errorf("failed to start transport for peer %s: %w", peer.ID, err)
		}

		if err := peer.SyncEngine.Start(ctx); err != nil {
			cancel()
			return fmt.Errorf("failed to start sync engine for peer %s: %w", peer.ID, err)
		}

		cancel()
	}

	// Wait for connections to establish
	time.Sleep(2 * time.Second)

	return nil
}

// WaitForSyncCompletion waits for sync to complete between peers
func (ith *IntegrationTestHelper) WaitForSyncCompletion(t *testing.T, peers []*PeerSetup, expectedFiles map[string][]string, timeout time.Duration) error {
	waiter := NewEventDrivenWaiterWithTimeout(timeout)
	defer waiter.Close()

	// For each peer, check that all expected files are present
	for _, peer := range peers {
		expectedPeerFiles, exists := expectedFiles[peer.ID]
		if !exists {
			continue
		}

		for _, filename := range expectedPeerFiles {
			if err := waiter.WaitForFileSync(peer.Dir, filename); err != nil {
				return fmt.Errorf("peer %s missing expected file %s: %w", peer.ID, filename, err)
			}
		}
	}

	return nil
}

// VerifyFileContents verifies that files have the expected content across all peers
func (ith *IntegrationTestHelper) VerifyFileContents(t *testing.T, peers []*PeerSetup, fileChecks map[string]map[string][]byte) error {
	for peerID, fileContents := range fileChecks {
		// Find the peer
		var peer *PeerSetup
		for _, p := range peers {
			if p.ID == peerID {
				peer = p
				break
			}
		}
		if peer == nil {
			return fmt.Errorf("peer %s not found", peerID)
		}

		// Check each file
		for filename, expectedContent := range fileContents {
			filePath := filepath.Join(peer.Dir, filename)
			actualContent, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read file %s on peer %s: %w", filename, peerID, err)
			}

			if string(actualContent) != string(expectedContent) {
				return fmt.Errorf("file %s content mismatch on peer %s", filename, peerID)
			}
		}
	}

	return nil
}

// CreateTestFile creates a test file on a specific peer
func (ith *IntegrationTestHelper) CreateTestFile(peer *PeerSetup, filename, content string) error {
	filePath := filepath.Join(peer.Dir, filename)
	return os.WriteFile(filePath, []byte(content), 0644)
}

// CreateTestFileWithContent creates a test file with binary content
func (ith *IntegrationTestHelper) CreateTestFileWithContent(peer *PeerSetup, filename string, content []byte) error {
	filePath := filepath.Join(peer.Dir, filename)
	return os.WriteFile(filePath, content, 0644)
}

// DeleteTestFile deletes a test file from a specific peer
func (ith *IntegrationTestHelper) DeleteTestFile(peer *PeerSetup, filename string) error {
	filePath := filepath.Join(peer.Dir, filename)
	return os.Remove(filePath)
}

// RenameTestFile renames a test file on a specific peer
func (ith *IntegrationTestHelper) RenameTestFile(peer *PeerSetup, oldName, newName string) error {
	oldPath := filepath.Join(peer.Dir, oldName)
	newPath := filepath.Join(peer.Dir, newName)
	return os.Rename(oldPath, newPath)
}

// SimulateNetworkInterruption simulates network interruption by stopping transports
func (ith *IntegrationTestHelper) SimulateNetworkInterruption(peers []*PeerSetup, disconnectedPeers []string) error {
	for _, peerID := range disconnectedPeers {
		for _, peer := range peers {
			if peer.ID == peerID {
				if err := peer.Transport.Stop(); err != nil {
					return fmt.Errorf("failed to stop transport for peer %s: %w", peerID, err)
				}
				break
			}
		}
	}
	return nil
}

// RestoreNetworkConnection restores network connection by restarting transports
func (ith *IntegrationTestHelper) RestoreNetworkConnection(peers []*PeerSetup, reconnectedPeers []string) error {
	for _, peerID := range reconnectedPeers {
		for _, peer := range peers {
			if peer.ID == peerID {
				// Recreate transport with same port
				newTransport := transport.NewTCPTransport(peer.Config.Network.Port)

				if err := newTransport.Start(); err != nil {
					return fmt.Errorf("failed to restart transport for peer %s: %w", peerID, err)
				}

				peer.Transport = newTransport
				break
			}
		}
	}

	// Wait for reconnections
	time.Sleep(2 * time.Second)
	return nil
}

// GetPeerConnectionStates returns the connection states for all peers
func (ith *IntegrationTestHelper) GetPeerConnectionStates(peers []*PeerSetup) map[string]string {
	states := make(map[string]string)
	for _, peer := range peers {
		// This would need to be implemented based on the connection manager's state tracking
		// For now, return a placeholder
		states[peer.ID] = "connected" // Placeholder
	}
	return states
}

// CleanupAllPeers cleans up all peer resources
func (ith *IntegrationTestHelper) CleanupAllPeers(peers []*PeerSetup) {
	for _, peer := range peers {
		if peer.Cleanup != nil {
			peer.Cleanup()
		}
	}
}

// SetupMultiPeerNetwork creates a complete multi-peer network for testing
func SetupMultiPeerNetwork(t *testing.T, peerCount int, enableEncryption bool) ([]*PeerSetup, func(), error) {
	helper := NewIntegrationTestHelper(t)
	peers := make([]*PeerSetup, peerCount)

	// Create all peers
	for i := 0; i < peerCount; i++ {
		peerName := fmt.Sprintf("peer%d", i+1)
		peer, err := helper.SetupPeer(t, peerName, enableEncryption)
		if err != nil {
			// Cleanup already created peers
			for j := 0; j < i; j++ {
				if peers[j] != nil && peers[j].Cleanup != nil {
					peers[j].Cleanup()
				}
			}
			return nil, nil, fmt.Errorf("failed to setup peer %d: %w", i+1, err)
		}
		peers[i] = peer
	}

	// Connect all peers
	if err := helper.ConnectPeers(peers); err != nil {
		helper.CleanupAllPeers(peers)
		return nil, nil, fmt.Errorf("failed to connect peers: %w", err)
	}

	cleanup := func() {
		helper.CleanupAllPeers(peers)
	}

	return peers, cleanup, nil
}

// VerifySyncConsistency verifies that all peers have consistent file states
func VerifySyncConsistency(t *testing.T, peers []*PeerSetup, testFiles []string) error {
	if len(peers) == 0 {
		return fmt.Errorf("no peers to verify")
	}

	// Use first peer as reference
	referencePeer := peers[0]

	for _, filename := range testFiles {
		referencePath := filepath.Join(referencePeer.Dir, filename)

		// Get reference file content
		referenceContent, err := os.ReadFile(referencePath)
		if err != nil {
			if os.IsNotExist(err) {
				// File should not exist on other peers either
				for _, peer := range peers[1:] {
					peerPath := filepath.Join(peer.Dir, filename)
					if _, err := os.Stat(peerPath); !os.IsNotExist(err) {
						return fmt.Errorf("file %s exists on peer %s but not on reference peer %s",
							filename, peer.ID, referencePeer.ID)
					}
				}
			} else {
				return fmt.Errorf("failed to read reference file %s: %w", filename, err)
			}
		} else {
			// File exists, verify content matches on all peers
			for _, peer := range peers[1:] {
				peerPath := filepath.Join(peer.Dir, filename)
				peerContent, err := os.ReadFile(peerPath)
				if err != nil {
					return fmt.Errorf("failed to read file %s on peer %s: %w", filename, peer.ID, err)
				}

				if string(peerContent) != string(referenceContent) {
					return fmt.Errorf("file %s content mismatch between peer %s and reference peer %s",
						filename, peer.ID, referencePeer.ID)
				}
			}
		}
	}

	return nil
}

// createTestConfig creates a test configuration
func createTestConfig(syncDir string) *config.Config {
	return &config.Config{
		Sync: config.SyncConfig{
			FolderPath:             syncDir,
			ChunkSizeMin:           65536,
			ChunkSizeMax:           2097152,
			ChunkSizeDefault:       524288,
			MaxConcurrentTransfers: 5,
			OperationLogSize:       10000,
		},
		Network: config.NetworkConfig{
			Port:              0, // Use random port for testing
			DiscoveryPort:     0,
			Protocol:          "tcp",
			HeartbeatInterval: 5,
			ConnectionTimeout: 30,
			Peers:             []string{},
		},
		Security: config.SecurityConfig{
			KeyRotationInterval: 86400,
			EncryptionAlgorithm: "aes-256-gcm",
		},
		Compression: config.CompressionConfig{
			Enabled:           true,
			FileSizeThreshold: 1048576,
			Algorithm:         "zstd",
			Level:             3,
			ChunkCompression:  true,
		},
		Conflict: config.ConflictConfig{
			ResolutionStrategy: "intelligent_merge",
		},
		Observability: config.ObservabilityConfig{
			LogLevel:       "debug",
			MetricsEnabled: true,
			TracingEnabled: false,
		},
	}
}

// getAvailablePort finds an available TCP port for testing
func getAvailablePort(t *testing.T) int {
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to get available port: %v", err)
	}
	defer listener.Close()

	addr := listener.Addr().(*net.TCPAddr)
	return addr.Port
}
