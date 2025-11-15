package discovery

import (
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/p2p-folder-sync/p2p-sync/internal/network/messages"
)

// UDPDiscovery handles UDP broadcast discovery
type UDPDiscovery struct {
	port        int
	peerID      string
	capabilities map[string]interface{}
	version     string
	conn        *net.UDPConn
	registry    *Registry
	stopCh      chan struct{}
}

// NewUDPDiscovery creates a new UDP discovery service
func NewUDPDiscovery(port int, peerID string, capabilities map[string]interface{}, version string) *UDPDiscovery {
	return &UDPDiscovery{
		port:        port,
		peerID:      peerID,
		capabilities: capabilities,
		version:     version,
		stopCh:      make(chan struct{}),
	}
}

// NewUDPDiscoveryWithRegistry creates a new UDP discovery service with a registry
func NewUDPDiscoveryWithRegistry(port int, peerID string, capabilities map[string]interface{}, version string, registry *Registry) *UDPDiscovery {
	return &UDPDiscovery{
		port:        port,
		peerID:      peerID,
		capabilities: capabilities,
		version:     version,
		registry:    registry,
		stopCh:      make(chan struct{}),
	}
}

// Start starts the UDP discovery service
func (u *UDPDiscovery) Start() error {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", u.port))
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on UDP: %w", err)
	}
	u.conn = conn

	// Start listening for discovery messages
	go u.listen()

	// Start periodic broadcasts
	go u.broadcast()

	return nil
}

// Stop stops the UDP discovery service
func (u *UDPDiscovery) Stop() error {
	close(u.stopCh)
	if u.conn != nil {
		return u.conn.Close()
	}
	return nil
}

// listen listens for discovery messages
func (u *UDPDiscovery) listen() {
	buffer := make([]byte, 4096)
	for {
		select {
		case <-u.stopCh:
			return
		default:
			u.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
			n, addr, err := u.conn.ReadFromUDP(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				continue
			}

			var msg messages.Message
			if err := json.Unmarshal(buffer[:n], &msg); err != nil {
				continue
			}

			switch msg.Type {
			case messages.TypeDiscovery:
				// Respond to discovery
				u.respondToDiscovery(addr)
			case messages.TypeDiscoveryResponse:
				// Handle discovery response - register the peer
				u.handleDiscoveryResponse(&msg, addr)
			}
		}
	}
}

// respondToDiscovery responds to a discovery message
func (u *UDPDiscovery) respondToDiscovery(addr *net.UDPAddr) {
	response := messages.NewMessage(
		messages.TypeDiscoveryResponse,
		u.peerID,
		messages.DiscoveryResponseMessage{
			PeerID:  u.peerID,
			Port:    u.port,
			Version: u.version,
		},
	)

	data, err := response.Encode()
	if err != nil {
		return
	}

	u.conn.WriteToUDP(data, addr)
}

// handleDiscoveryResponse handles a discovery response by registering the peer
func (u *UDPDiscovery) handleDiscoveryResponse(msg *messages.Message, addr *net.UDPAddr) {
	payload, ok := msg.Payload.(*messages.DiscoveryResponseMessage)
	if !ok {
		return
	}

	// Skip our own responses
	if payload.PeerID == u.peerID {
		return
	}

	// Register the discovered peer
	if u.registry != nil {
		u.registry.AddOrUpdatePeer(
			payload.PeerID,
			addr.IP.String(),
			payload.Port,
			map[string]interface{}{}, // No capabilities in response
			payload.Version,
		)
	}
}

// broadcast periodically broadcasts discovery messages
func (u *UDPDiscovery) broadcast() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-u.stopCh:
			return
		case <-ticker.C:
			u.sendDiscovery()
		}
	}
}

// sendDiscovery sends a discovery broadcast
func (u *UDPDiscovery) sendDiscovery() {
	discoveryMsg := messages.NewMessage(
		messages.TypeDiscovery,
		u.peerID,
		messages.DiscoveryMessage{
			PeerID:      u.peerID,
			Port:        u.port,
			Capabilities: u.capabilities,
			Version:     u.version,
		},
	)

	data, err := discoveryMsg.Encode()
	if err != nil {
		return
	}

	// Broadcast to subnet
	addr, err := net.ResolveUDPAddr("udp", "255.255.255.255:8081")
	if err != nil {
		return
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return
	}
	defer conn.Close()

	conn.Write(data)
}

