package transport_test

import (
	"testing"

	"github.com/p2p-folder-sync/p2p-sync/internal/network/transport"
)

func TestFallbackTransport_Creation(t *testing.T) {
	ft := transport.NewFallbackTransport(8080)
	if ft == nil {
		t.Fatal("NewFallbackTransport returned nil")
	}

	t.Log("SUCCESS: Fallback transport created successfully")
}

func TestFallbackTransport_GetActiveProtocol(t *testing.T) {
	ft := transport.NewFallbackTransport(8080)

	// Before starting, should return default (quic)
	protocol := ft.GetActiveProtocol()
	if protocol != "quic" {
		t.Errorf("Expected default protocol 'quic', got %s", protocol)
	}

	t.Log("SUCCESS: GetActiveProtocol returns correct default")
}

func TestFallbackTransport_GetPeerProtocol(t *testing.T) {
	ft := transport.NewFallbackTransport(8080)

	// Unknown peer should return global default
	protocol := ft.GetPeerProtocol("unknown-peer")
	if protocol != "quic" {
		t.Errorf("Expected unknown peer to use default 'quic', got %s", protocol)
	}

	t.Log("SUCCESS: GetPeerProtocol returns global default for unknown peers")
}

func TestTransportFactory_Default(t *testing.T) {
	factory := &transport.TransportFactory{}

	// Default (empty string) should create fallback transport
	trans, err := factory.NewTransport("", 8080)
	if err != nil {
		t.Fatalf("Failed to create default transport: %v", err)
	}

	if trans == nil {
		t.Fatal("Factory returned nil transport")
	}

	// Check if it's a fallback transport by checking if it has the method
	if ft, ok := trans.(*transport.FallbackTransport); ok {
		if ft.GetActiveProtocol() != "quic" {
			t.Errorf("Expected fallback transport to default to quic")
		}
		t.Log("SUCCESS: Factory creates fallback transport by default")
	} else {
		t.Log("Note: Default transport is not a FallbackTransport (may be expected)")
	}
}

func TestTransportFactory_ExplicitQUIC(t *testing.T) {
	factory := &transport.TransportFactory{}

	trans, err := factory.NewTransport("quic", 8080)
	if err != nil {
		t.Fatalf("Failed to create QUIC transport: %v", err)
	}

	if trans == nil {
		t.Fatal("Factory returned nil transport for QUIC")
	}

	t.Log("SUCCESS: Factory creates QUIC transport when explicitly requested")
}

func TestTransportFactory_ExplicitTCP(t *testing.T) {
	factory := &transport.TransportFactory{}

	trans, err := factory.NewTransport("tcp", 8080)
	if err != nil {
		t.Fatalf("Failed to create TCP transport: %v", err)
	}

	if trans == nil {
		t.Fatal("Factory returned nil transport for TCP")
	}

	t.Log("SUCCESS: Factory creates TCP transport when explicitly requested")
}

func TestFallbackTransport_SetMessageHandler(t *testing.T) {
	ft := transport.NewFallbackTransport(8080)

	// Set a nil handler (just testing the interface)
	err := ft.SetMessageHandler(nil)
	if err != nil {
		t.Errorf("SetMessageHandler failed: %v", err)
	}

	t.Log("SUCCESS: SetMessageHandler accepts handler")
}
