package connection_test

import (
	"testing"

	"github.com/p2p-folder-sync/p2p-sync/internal/config"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/connection"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/messages"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/transport"
)

// mockTransport implements the Transport interface for testing
type mockTransport struct{}

func (m *mockTransport) Start() error                                             { return nil }
func (m *mockTransport) Stop() error                                              { return nil }
func (m *mockTransport) SetMessageHandler(handler transport.MessageHandler) error { return nil }
func (m *mockTransport) SendMessage(peerID string, address string, port int, msg *messages.Message) error {
	return nil
}

func TestNewConnectionManager(t *testing.T) {
	cm := connection.NewConnectionManager()

	if cm == nil {
		t.Error("Expected connection manager to be created")
	}
}

func TestConnectionManager_AddGetRemove(t *testing.T) {
	cm := connection.NewConnectionManager()
	peerID := "test-peer"
	address := "192.168.1.100"
	port := 8080

	// Add connection
	conn := cm.AddConnection(peerID, address, port)
	if conn == nil {
		t.Error("Expected connection to be created")
	}

	// Get connection
	retrieved, err := cm.GetConnection(peerID)
	if err != nil {
		t.Fatalf("Failed to get connection: %v", err)
	}
	if retrieved.PeerID != peerID {
		t.Errorf("Expected peer ID %s, got %s", peerID, retrieved.PeerID)
	}

	// Remove connection
	cm.RemoveConnection(peerID)

	// Verify it's gone
	_, err = cm.GetConnection(peerID)
	if err == nil {
		t.Error("Expected error when getting removed connection")
	}
}

func TestConnectionManager_GetAllConnections(t *testing.T) {
	cm := connection.NewConnectionManager()

	// Add multiple connections
	cm.AddConnection("peer1", "192.168.1.100", 8080)
	cm.AddConnection("peer2", "192.168.1.101", 8081)

	connections := cm.GetAllConnections()
	if len(connections) != 2 {
		t.Errorf("Expected 2 connections, got %d", len(connections))
	}
}

func TestConnectionManager_UpdateConnectionState(t *testing.T) {
	cm := connection.NewConnectionManager()
	peerID := "test-peer"

	cm.AddConnection(peerID, "192.168.1.100", 8080)

	// Update state
	err := cm.UpdateConnectionState(peerID, connection.StateConnected)
	if err != nil {
		t.Fatalf("Failed to update connection state: %v", err)
	}

	// Get connection and check state
	conn, err := cm.GetConnection(peerID)
	if err != nil {
		t.Fatalf("Failed to get connection: %v", err)
	}
	if conn.State != connection.StateConnected {
		t.Errorf("Expected state %s, got %s", connection.StateConnected, conn.State)
	}
}

func TestConnectionManager_GetConnectedPeers(t *testing.T) {
	cm := connection.NewConnectionManager()

	// Add connections with different states
	cm.AddConnection("peer1", "192.168.1.100", 8080)
	cm.AddConnection("peer2", "192.168.1.101", 8081)
	cm.AddConnection("peer3", "192.168.1.102", 8082)

	cm.UpdateConnectionState("peer1", connection.StateConnected)
	cm.UpdateConnectionState("peer2", connection.StateDisconnected)
	cm.UpdateConnectionState("peer3", connection.StateConnecting)

	connectedPeers := cm.GetConnectedPeers()
	if len(connectedPeers) != 1 {
		t.Errorf("Expected 1 connected peer, got %d", len(connectedPeers))
	}
	if len(connectedPeers) > 0 && connectedPeers[0] != "peer1" {
		t.Errorf("Expected peer1 to be connected, got %s", connectedPeers[0])
	}
}

func TestNewHeartbeatManager(t *testing.T) {
	// Create mock dependencies
	connManager := connection.NewConnectionManager()
	transport := &mockTransport{}
	cfg := &config.Config{
		Network: config.NetworkConfig{
			HeartbeatInterval: 30,
		},
	}

	hm := connection.NewHeartbeatManager(connManager, transport, cfg)

	if hm == nil {
		t.Error("Expected heartbeat manager to be created")
	}
}
