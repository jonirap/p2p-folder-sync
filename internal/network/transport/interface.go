package transport

import (
	"github.com/p2p-folder-sync/p2p-sync/internal/network/messages"
)

// MessageHandler defines the interface for handling incoming messages
type MessageHandler interface {
	HandleMessage(msg *messages.Message) error
}

// Transport defines the interface for network transports
type Transport interface {
	// Start starts the transport
	Start() error
	// Stop stops the transport
	Stop() error
	// SetMessageHandler sets the message handler for incoming messages
	SetMessageHandler(handler MessageHandler) error
	// SendMessage sends a message to a specific peer at the given address
	SendMessage(peerID string, address string, port int, msg *messages.Message) error
	// ConnectToPeer establishes an outbound connection to a peer
	ConnectToPeer(peerID string, address string, port int) error
}

// TransportFactory creates transports based on configuration
type TransportFactory struct{}

// NewTransport creates a transport with fallback logic
func (f *TransportFactory) NewTransport(protocol string, port int) (Transport, error) {
	switch protocol {
	case "quic":
		// Explicit QUIC only (no fallback)
		return NewQUICTransport(port), nil
	case "tcp":
		// Explicit TCP only (no fallback)
		return NewTCPTransport(port), nil
	default:
		// Default to automatic QUIC-to-TCP fallback (spec lines 612-614)
		return NewFallbackTransport(port), nil
	}
}
