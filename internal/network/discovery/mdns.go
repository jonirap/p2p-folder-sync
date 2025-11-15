package discovery

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/grandcat/zeroconf"
)

// MDNSDiscovery handles mDNS/DNS-SD peer discovery
type MDNSDiscovery struct {
	serviceName string
	serviceType string
	domain      string
	port        int
	peerID      string
	capabilities map[string]interface{}
	version     string
	server      *zeroconf.Server
	resolver    *zeroconf.Resolver
	stopCh      chan struct{}
	foundPeers  chan *PeerInfo
}

// NewMDNSDiscovery creates a new mDNS discovery service
func NewMDNSDiscovery(port int, peerID string, capabilities map[string]interface{}, version string) *MDNSDiscovery {
	return &MDNSDiscovery{
		serviceName: "p2p-sync",
		serviceType: "_p2p-sync._tcp",
		domain:      "local.",
		port:        port,
		peerID:      peerID,
		capabilities: capabilities,
		version:     version,
		stopCh:      make(chan struct{}),
		foundPeers:  make(chan *PeerInfo, 10),
	}
}

// Start starts the mDNS discovery service
func (m *MDNSDiscovery) Start() error {
	// Prepare TXT records
	txtRecords := []string{
		fmt.Sprintf("peer_id=%s", m.peerID),
		fmt.Sprintf("port=%d", m.port),
		fmt.Sprintf("encryption=%t", m.capabilities["encryption"].(bool)),
		fmt.Sprintf("compression=%t", m.capabilities["compression"].(bool)),
		fmt.Sprintf("chunking=%t", m.capabilities["chunking"].(bool)),
		fmt.Sprintf("version=%s", m.version),
	}

	// Create and start mDNS server
	server, err := zeroconf.RegisterProxy(
		m.serviceName,
		m.serviceType,
		m.domain,
		m.port,
		m.peerID, // instance name
		nil, // no specific IPs
		txtRecords,
		nil, // no specific interfaces
	)
	if err != nil {
		return fmt.Errorf("failed to register mDNS service: %w", err)
	}
	m.server = server

	// Create resolver for discovering other peers
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		server.Shutdown()
		return fmt.Errorf("failed to create mDNS resolver: %w", err)
	}
	m.resolver = resolver

	// Start browsing for services
	go m.browseServices()

	return nil
}

// Stop stops the mDNS discovery service
func (m *MDNSDiscovery) Stop() error {
	close(m.stopCh)

	if m.resolver != nil {
		// Note: zeroconf.Resolver doesn't have a Shutdown method
	}

	if m.server != nil {
		m.server.Shutdown()
	}

	return nil
}

// browseServices browses for mDNS services
func (m *MDNSDiscovery) browseServices() {
	entries := make(chan *zeroconf.ServiceEntry, 10)

	// Start browsing
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-m.stopCh
		cancel()
	}()

	err := m.resolver.Browse(ctx, m.serviceType, m.domain, entries)
	if err != nil {
		// Log error but don't panic
		return
	}

	for {
		select {
		case <-m.stopCh:
			return
		case entry := <-entries:
			m.handleServiceEntry(entry)
		}
	}
}

// handleServiceEntry processes a discovered service entry
func (m *MDNSDiscovery) handleServiceEntry(entry *zeroconf.ServiceEntry) {
	// Skip our own service
	if entry.Instance == m.peerID {
		return
	}

	// Parse TXT records
	capabilities := make(map[string]interface{})
	var version string
	var port int

	for _, txt := range entry.Text {
		switch {
		case len(txt) > 8 && txt[:8] == "peer_id=":
			// peer_id is in Instance field
		case len(txt) > 5 && txt[:5] == "port=":
			if p, err := strconv.Atoi(txt[5:]); err == nil {
				port = p
			}
		case len(txt) > 11 && txt[:11] == "encryption=":
			capabilities["encryption"] = txt[11:] == "true"
		case len(txt) > 12 && txt[:12] == "compression=":
			capabilities["compression"] = txt[12:] == "true"
		case len(txt) > 9 && txt[:9] == "chunking=":
			capabilities["chunking"] = txt[9:] == "true"
		case len(txt) > 8 && txt[:8] == "version=":
			version = txt[8:]
		}
	}

	// Use service port if TXT port not specified
	if port == 0 {
		port = entry.Port
	}

	peerInfo := &PeerInfo{
		PeerID:      entry.Instance,
		Address:     entry.AddrIPv4[0].String(), // Use first IPv4 address
		Port:        port,
		Capabilities: capabilities,
		Version:     version,
		LastSeen:    time.Now(),
	}

	// Send peer info to channel (non-blocking)
	select {
	case m.foundPeers <- peerInfo:
	default:
		// Channel full, skip this peer
	}
}

// FoundPeers returns a channel of discovered peers
func (m *MDNSDiscovery) FoundPeers() <-chan *PeerInfo {
	return m.foundPeers
}
