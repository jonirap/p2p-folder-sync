package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"syscall"
	"time"

	"github.com/p2p-folder-sync/p2p-sync/internal/network/messages"
)

// UDPDiscovery handles UDP broadcast discovery
type UDPDiscovery struct {
	port        int
	syncPort    int
	peerID      string
	capabilities map[string]interface{}
	version     string
	conn        *net.UDPConn
	registry    *Registry
	stopCh      chan struct{}
}

// NewUDPDiscovery creates a new UDP discovery service
func NewUDPDiscovery(port int, syncPort int, peerID string, capabilities map[string]interface{}, version string) *UDPDiscovery {
	return &UDPDiscovery{
		port:        port,
		syncPort:    syncPort,
		peerID:      peerID,
		capabilities: capabilities,
		version:     version,
		stopCh:      make(chan struct{}),
	}
}

// NewUDPDiscoveryWithRegistry creates a new UDP discovery service with a registry
func NewUDPDiscoveryWithRegistry(port int, syncPort int, peerID string, capabilities map[string]interface{}, version string, registry *Registry) *UDPDiscovery {
	return &UDPDiscovery{
		port:        port,
		syncPort:    syncPort,
		peerID:      peerID,
		capabilities: capabilities,
		version:     version,
		registry:    registry,
		stopCh:      make(chan struct{}),
	}
}

// Start starts the UDP discovery service
func (u *UDPDiscovery) Start() error {
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf(":%d", u.port))
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	// Use ListenConfig to set socket options for address reuse and broadcast
	// This allows multiple peers on the same machine to listen on the same port
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var opErr error
			err := c.Control(func(fd uintptr) {
				// Set SO_REUSEADDR to allow multiple bindings to the same port
				opErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
				if opErr != nil {
					return
				}
				// Set SO_REUSEPORT to allow multiple processes to bind to the same port
				opErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, 0xf /* SO_REUSEPORT */, 1)
				if opErr != nil {
					return
				}
				// Set SO_BROADCAST to allow receiving broadcast packets
				opErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
			})
			if err != nil {
				return err
			}
			return opErr
		},
	}

	// Listen on the UDP address with the custom config
	packetConn, err := lc.ListenPacket(context.Background(), "udp4", addr.String())
	if err != nil {
		return fmt.Errorf("failed to listen on UDP: %w", err)
	}

	var ok bool
	u.conn, ok = packetConn.(*net.UDPConn)
	if !ok {
		packetConn.Close()
		return fmt.Errorf("failed to convert PacketConn to UDPConn")
	}

	// Start listening for discovery messages
	go u.listen()

	// Start periodic broadcasts
	go u.broadcast()

	log.Printf("[UDP Discovery] Started for peer %s on port %d", u.peerID, u.port)
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

			// First unmarshal to a generic structure to get the message type
			var rawMsg struct {
				Type    string          `json:"type"`
				Payload json.RawMessage `json:"payload"`
			}
			if err := json.Unmarshal(buffer[:n], &rawMsg); err != nil {
				continue
			}

			switch rawMsg.Type {
			case messages.TypeDiscovery:
				// Unmarshal the discovery message payload
				var payload messages.DiscoveryMessage
				if err := json.Unmarshal(rawMsg.Payload, &payload); err != nil {
					log.Printf("[UDP Discovery] Failed to unmarshal discovery payload: %v", err)
					continue
				}
				log.Printf("[UDP Discovery] Received discovery message from %s (peer: %s)", addr, payload.PeerID)
				u.respondToDiscovery(&payload, addr)

			case messages.TypeDiscoveryResponse:
				// Unmarshal the discovery response payload
				var payload messages.DiscoveryResponseMessage
				if err := json.Unmarshal(rawMsg.Payload, &payload); err != nil {
					log.Printf("[UDP Discovery] Failed to unmarshal discovery response payload: %v", err)
					continue
				}
				log.Printf("[UDP Discovery] Received discovery response from %s (peer: %s)", addr, payload.PeerID)
				u.handleDiscoveryResponse(&payload, addr)
			}
		}
	}
}

// respondToDiscovery responds to a discovery message
func (u *UDPDiscovery) respondToDiscovery(payload *messages.DiscoveryMessage, sourceAddr *net.UDPAddr) {
	// Skip our own discovery messages
	if payload.PeerID == u.peerID {
		log.Printf("[UDP Discovery] Skipping own discovery message")
		return
	}

	// Send response to the peer's UDP discovery port (not the ephemeral source port)
	responseAddr := &net.UDPAddr{
		IP:   sourceAddr.IP,
		Port: payload.Port, // Use the port from the discovery message
	}

	response := messages.NewMessage(
		messages.TypeDiscoveryResponse,
		u.peerID,
		messages.DiscoveryResponseMessage{
			PeerID:   u.peerID,
			Port:     u.port,
			SyncPort: u.syncPort,
			Version:  u.version,
		},
	)

	data, err := response.Encode()
	if err != nil {
		log.Printf("[UDP Discovery] Failed to encode response: %v", err)
		return
	}

	if _, err := u.conn.WriteToUDP(data, responseAddr); err != nil {
		log.Printf("[UDP Discovery] Failed to send response to %s: %v", responseAddr, err)
	} else {
		log.Printf("[UDP Discovery] Sent discovery response to %s (peer %s)", responseAddr, payload.PeerID)
	}
}

// handleDiscoveryResponse handles a discovery response by registering the peer
func (u *UDPDiscovery) handleDiscoveryResponse(payload *messages.DiscoveryResponseMessage, addr *net.UDPAddr) {
	// Skip our own responses
	if payload.PeerID == u.peerID {
		log.Printf("[UDP Discovery] Skipping own discovery response")
		return
	}

	log.Printf("[UDP Discovery] Discovered peer: %s at %s:%d (sync port: %d)", payload.PeerID, addr.IP, payload.Port, payload.SyncPort)

	// Register the discovered peer using the TCP sync port
	if u.registry != nil {
		u.registry.AddOrUpdatePeer(
			payload.PeerID,
			addr.IP.String(),
			payload.SyncPort, // Use the TCP sync port for connections
			map[string]interface{}{}, // No capabilities in response
			payload.Version,
		)
		log.Printf("[UDP Discovery] Registered peer %s in registry with sync port %d", payload.PeerID, payload.SyncPort)
	} else {
		log.Printf("[UDP Discovery] WARNING: No registry available to register peer %s", payload.PeerID)
	}
}

// broadcast periodically broadcasts discovery messages
func (u *UDPDiscovery) broadcast() {
	// Send initial discovery message immediately
	log.Printf("[UDP Discovery] Sending initial discovery broadcast for peer %s", u.peerID)
	u.sendDiscovery()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-u.stopCh:
			return
		case <-ticker.C:
			log.Printf("[UDP Discovery] Sending periodic discovery broadcast for peer %s", u.peerID)
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
			SyncPort:    u.syncPort,
			Capabilities: u.capabilities,
			Version:     u.version,
		},
	)

	data, err := discoveryMsg.Encode()
	if err != nil {
		log.Printf("[UDP Discovery] Failed to encode discovery message: %v", err)
		return
	}

	// Broadcast to subnet using the configured port
	// Support both general broadcast and Docker network broadcast
	broadcastAddrs := []string{
		fmt.Sprintf("255.255.255.255:%d", u.port), // General broadcast
		fmt.Sprintf("172.20.255.255:%d", u.port),  // Docker bridge network broadcast
	}

	log.Printf("[UDP Discovery] Broadcasting discovery message for peer %s to addresses: %v", u.peerID, broadcastAddrs)

	for _, broadcastAddr := range broadcastAddrs {
		addr, err := net.ResolveUDPAddr("udp", broadcastAddr)
		if err != nil {
			continue
		}

		// Use ListenConfig to set SO_BROADCAST
		lc := net.ListenConfig{
			Control: func(network, address string, c syscall.RawConn) error {
				var opErr error
				err := c.Control(func(fd uintptr) {
					// Set SO_BROADCAST to allow broadcast packets
					opErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
				})
				if err != nil {
					return err
				}
				return opErr
			},
		}

		// Create a connection with broadcast enabled
		packetConn, err := lc.ListenPacket(context.Background(), "udp4", "0.0.0.0:0")
		if err != nil {
			continue
		}

		conn, ok := packetConn.(*net.UDPConn)
		if !ok {
			packetConn.Close()
			continue
		}

		n, err := conn.WriteToUDP(data, addr)
		if err != nil {
			log.Printf("[UDP Discovery] Failed to broadcast to %s: %v", broadcastAddr, err)
		} else {
			log.Printf("[UDP Discovery] Sent %d bytes to %s", n, broadcastAddr)
		}
		conn.Close()
	}
}

