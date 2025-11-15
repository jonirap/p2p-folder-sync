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
}

// TransportFactory creates transports based on configuration
type TransportFactory struct{}

// NewTransport creates a transport with fallback logic
func (f *TransportFactory) NewTransport(protocol string, port int) (Transport, error) {
	switch protocol {
	case "quic":
		return NewQUICTransport(port), nil
	case "tcp":
		return NewTCPTransport(port), nil
	default:
		// Default to QUIC with TCP fallback
		transport := NewQUICTransport(port)
		// TODO: Implement fallback logic if QUIC fails
		return transport, nil
	}
}
