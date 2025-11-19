package transport

import (
	"fmt"
	"log"
	"sync"

	"github.com/p2p-folder-sync/p2p-sync/internal/network/messages"
)

// FallbackTransport implements automatic QUIC-to-TCP fallback
type FallbackTransport struct {
	primaryTransport   Transport // QUIC
	fallbackTransport  Transport // TCP
	currentTransport   Transport
	handler            MessageHandler
	port               int
	mu                 sync.RWMutex
	usedFallback       bool
	peerProtocols      map[string]string // peerID -> protocol (quic/tcp)
	peerProtocolsMu    sync.RWMutex
}

// NewFallbackTransport creates a new fallback transport that tries QUIC first, then TCP
func NewFallbackTransport(port int) *FallbackTransport {
	return &FallbackTransport{
		primaryTransport:  NewQUICTransport(port),
		fallbackTransport: NewTCPTransport(port),
		currentTransport:  nil, // Will be determined at Start()
		port:              port,
		peerProtocols:     make(map[string]string),
	}
}

// Start starts the transport with automatic fallback
func (ft *FallbackTransport) Start() error {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	// Try QUIC first
	log.Printf("Attempting to start QUIC transport on port %d...", ft.port)
	err := ft.primaryTransport.Start()
	if err == nil {
		ft.currentTransport = ft.primaryTransport
		ft.usedFallback = false
		log.Printf("SUCCESS: QUIC transport started successfully on port %d", ft.port)

		// Set message handler if already registered
		if ft.handler != nil {
			ft.primaryTransport.SetMessageHandler(ft.handler)
		}
		return nil
	}

	// QUIC failed, fall back to TCP
	log.Printf("QUIC transport failed (%v), falling back to TCP...", err)

	err = ft.fallbackTransport.Start()
	if err != nil {
		return fmt.Errorf("both QUIC and TCP transports failed to start: QUIC error: %v, TCP error: %v", err, err)
	}

	ft.currentTransport = ft.fallbackTransport
	ft.usedFallback = true
	log.Printf("SUCCESS: Fell back to TCP transport on port %d", ft.port)

	// Set message handler if already registered
	if ft.handler != nil {
		ft.fallbackTransport.SetMessageHandler(ft.handler)
	}

	return nil
}

// Stop stops the active transport
func (ft *FallbackTransport) Stop() error {
	ft.mu.RLock()
	current := ft.currentTransport
	ft.mu.RUnlock()

	if current != nil {
		return current.Stop()
	}
	return nil
}

// SetMessageHandler sets the message handler for incoming messages
func (ft *FallbackTransport) SetMessageHandler(handler MessageHandler) error {
	ft.mu.Lock()
	ft.handler = handler
	current := ft.currentTransport
	ft.mu.Unlock()

	if current != nil {
		return current.SetMessageHandler(handler)
	}
	return nil
}

// SendMessage sends a message with per-peer protocol fallback
func (ft *FallbackTransport) SendMessage(peerID string, address string, port int, msg *messages.Message) error {
	ft.mu.RLock()
	current := ft.currentTransport
	ft.mu.RUnlock()

	if current == nil {
		return fmt.Errorf("transport not started")
	}

	// Check if we know which protocol works for this peer
	ft.peerProtocolsMu.RLock()
	knownProtocol, hasKnownProtocol := ft.peerProtocols[peerID]
	ft.peerProtocolsMu.RUnlock()

	// If we're using QUIC globally and haven't tried this peer yet
	if !ft.usedFallback && !hasKnownProtocol {
		// Try QUIC first
		err := ft.primaryTransport.SendMessage(peerID, address, port, msg)
		if err == nil {
			// QUIC worked, remember this
			ft.peerProtocolsMu.Lock()
			ft.peerProtocols[peerID] = "quic"
			ft.peerProtocolsMu.Unlock()
			return nil
		}

		// QUIC failed for this peer, try TCP fallback
		log.Printf("QUIC send failed for peer %s (%v), trying TCP fallback...", peerID, err)
		err = ft.fallbackTransport.SendMessage(peerID, address, port, msg)
		if err == nil {
			// TCP worked, remember this
			ft.peerProtocolsMu.Lock()
			ft.peerProtocols[peerID] = "tcp"
			ft.peerProtocolsMu.Unlock()
			log.Printf("TCP fallback succeeded for peer %s", peerID)
			return nil
		}

		return fmt.Errorf("both QUIC and TCP failed for peer %s: %w", peerID, err)
	}

	// If we know which protocol works for this peer, use it directly
	if hasKnownProtocol {
		if knownProtocol == "quic" {
			return ft.primaryTransport.SendMessage(peerID, address, port, msg)
		}
		return ft.fallbackTransport.SendMessage(peerID, address, port, msg)
	}

	// Otherwise use the current transport
	return current.SendMessage(peerID, address, port, msg)
}

// ConnectToPeer establishes a connection with automatic fallback
func (ft *FallbackTransport) ConnectToPeer(peerID string, address string, port int) error {
	ft.mu.RLock()
	current := ft.currentTransport
	ft.mu.RUnlock()

	if current == nil {
		return fmt.Errorf("transport not started")
	}

	// If we're using QUIC, try it first
	if !ft.usedFallback {
		err := ft.primaryTransport.ConnectToPeer(peerID, address, port)
		if err == nil {
			// QUIC worked
			ft.peerProtocolsMu.Lock()
			ft.peerProtocols[peerID] = "quic"
			ft.peerProtocolsMu.Unlock()
			return nil
		}

		// QUIC failed, try TCP
		log.Printf("QUIC connection failed for peer %s (%v), trying TCP fallback...", peerID, err)
		err = ft.fallbackTransport.ConnectToPeer(peerID, address, port)
		if err == nil {
			ft.peerProtocolsMu.Lock()
			ft.peerProtocols[peerID] = "tcp"
			ft.peerProtocolsMu.Unlock()
			log.Printf("TCP fallback connection succeeded for peer %s", peerID)
			return nil
		}

		return fmt.Errorf("both QUIC and TCP connection failed for peer %s: %w", peerID, err)
	}

	// Using TCP globally
	return current.ConnectToPeer(peerID, address, port)
}

// GetActiveProtocol returns the protocol currently in use globally
func (ft *FallbackTransport) GetActiveProtocol() string {
	ft.mu.RLock()
	defer ft.mu.RUnlock()

	if ft.usedFallback {
		return "tcp"
	}
	return "quic"
}

// GetPeerProtocol returns the protocol being used for a specific peer
func (ft *FallbackTransport) GetPeerProtocol(peerID string) string {
	ft.peerProtocolsMu.RLock()
	defer ft.peerProtocolsMu.RUnlock()

	protocol, exists := ft.peerProtocols[peerID]
	if !exists {
		return ft.GetActiveProtocol()
	}
	return protocol
}
