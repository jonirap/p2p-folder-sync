package transport_test

import (
	"testing"

	"github.com/p2p-folder-sync/p2p-sync/internal/network/transport"
)

func TestNewQUICTransport(t *testing.T) {
	transport := transport.NewQUICTransport(8080)

	if transport == nil {
		t.Error("Expected QUIC transport to be created")
	}
}

func TestNewTCPTransport(t *testing.T) {
	transport := transport.NewTCPTransport(8081)

	if transport == nil {
		t.Error("Expected TCP transport to be created")
	}
}

func TestTransportFactory(t *testing.T) {
	factory := &transport.TransportFactory{}

	// Test QUIC transport creation
	quicTransport, err := factory.NewTransport("quic", 8080)
	if err != nil {
		t.Fatalf("Failed to create QUIC transport: %v", err)
	}

	if quicTransport == nil {
		t.Error("Expected QUIC transport to be created")
	}

	// Test TCP transport creation
	tcpTransport, err := factory.NewTransport("tcp", 8081)
	if err != nil {
		t.Fatalf("Failed to create TCP transport: %v", err)
	}

	if tcpTransport == nil {
		t.Error("Expected TCP transport to be created")
	}
}

func TestTransportInterface(t *testing.T) {
	factory := &transport.TransportFactory{}

	// Create transports and verify they implement the Transport interface
	quicTransport, err := factory.NewTransport("quic", 8080)
	if err != nil {
		t.Fatalf("Failed to create QUIC transport: %v", err)
	}

	tcpTransport, err := factory.NewTransport("tcp", 8081)
	if err != nil {
		t.Fatalf("Failed to create TCP transport: %v", err)
	}

	// Verify they implement the Transport interface
	var _ transport.Transport = quicTransport
	var _ transport.Transport = tcpTransport
}
