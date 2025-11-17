package system

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/p2p-folder-sync/p2p-sync/internal/network/messages"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/transport"
)

// FailingTransport wraps a real transport and can simulate network failures
type FailingTransport struct {
	innerTransport transport.Transport
	mu             sync.RWMutex

	// Failure simulation settings
	shouldFail     bool
	failRate       float64 // 0.0 to 1.0, probability of failure
	delayDuration  time.Duration
	dropMessages   bool
	corruptData    bool

	// Statistics
	messagesSent   int
	messagesFailed int
	messagesDropped int
	messagesCorrupted int
}

// NewFailingTransport creates a new failing transport wrapper
func NewFailingTransport(inner transport.Transport) *FailingTransport {
	return &FailingTransport{
		innerTransport: inner,
		failRate:       0.0, // No failures by default
	}
}

// SendMessage sends a message, potentially simulating failures
func (ft *FailingTransport) SendMessage(peerID string, address string, port int, msg *messages.Message) error {
	ft.mu.Lock()
	ft.messagesSent++
	ft.mu.Unlock()

	ft.mu.RLock()
	shouldFail := ft.shouldFail && rand.Float64() < ft.failRate
	ft.mu.RUnlock()

	if shouldFail {
		ft.mu.Lock()
		ft.messagesFailed++
		ft.mu.Unlock()

		// Simulate different types of failures
		if ft.dropMessages {
			ft.mu.Lock()
			ft.messagesDropped++
			ft.mu.Unlock()
			return fmt.Errorf("simulated message drop for peer %s", peerID)
		}

		if ft.corruptData {
			ft.mu.Lock()
			ft.messagesCorrupted++
			ft.mu.Unlock()
			return fmt.Errorf("simulated data corruption for peer %s", peerID)
		}

		if ft.delayDuration > 0 {
			time.Sleep(ft.delayDuration)
		}

		return fmt.Errorf("simulated network failure for peer %s", peerID)
	}

	// Add artificial delay if configured
	ft.mu.RLock()
	delay := ft.delayDuration
	ft.mu.RUnlock()

	if delay > 0 {
		time.Sleep(delay)
	}

	// Send through the inner transport
	return ft.innerTransport.SendMessage(peerID, address, port, msg)
}

// Start starts the transport
func (ft *FailingTransport) Start() error {
	return ft.innerTransport.Start()
}

// Stop stops the transport
func (ft *FailingTransport) Stop() error {
	return ft.innerTransport.Stop()
}

// SetMessageHandler sets the message handler
func (ft *FailingTransport) SetMessageHandler(handler transport.MessageHandler) error {
	return ft.innerTransport.SetMessageHandler(handler)
}

// ConnectToPeer connects to a peer
func (ft *FailingTransport) ConnectToPeer(peerID string, address string, port int) error {
	return ft.innerTransport.ConnectToPeer(peerID, address, port)
}

// EnableFailures enables failure simulation
func (ft *FailingTransport) EnableFailures() {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	ft.shouldFail = true
}

// DisableFailures disables failure simulation
func (ft *FailingTransport) DisableFailures() {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	ft.shouldFail = false
}

// SetFailureRate sets the probability of message failure (0.0 to 1.0)
func (ft *FailingTransport) SetFailureRate(rate float64) {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	if rate < 0 {
		ft.failRate = 0
	} else if rate > 1 {
		ft.failRate = 1
	} else {
		ft.failRate = rate
	}
}

// SetDelay sets artificial delay for all messages
func (ft *FailingTransport) SetDelay(delay time.Duration) {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	ft.delayDuration = delay
}

// EnableMessageDropping enables message dropping failures
func (ft *FailingTransport) EnableMessageDropping() {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	ft.dropMessages = true
	ft.corruptData = false
}

// EnableDataCorruption enables data corruption failures
func (ft *FailingTransport) EnableDataCorruption() {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	ft.corruptData = true
	ft.dropMessages = false
}

// ResetStatistics resets failure statistics
func (ft *FailingTransport) ResetStatistics() {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	ft.messagesSent = 0
	ft.messagesFailed = 0
	ft.messagesDropped = 0
	ft.messagesCorrupted = 0
}

// GetStatistics returns current failure statistics
func (ft *FailingTransport) GetStatistics() (sent, failed, dropped, corrupted int) {
	ft.mu.RLock()
	defer ft.mu.RUnlock()
	return ft.messagesSent, ft.messagesFailed, ft.messagesDropped, ft.messagesCorrupted
}

// IntermittentTransport simulates intermittent connectivity
type IntermittentTransport struct {
	innerTransport transport.Transport
	mu             sync.RWMutex

	// Intermittent behavior
	connected     bool
	connectPeriod time.Duration
	failPeriod    time.Duration
	lastChange    time.Time
}

// NewIntermittentTransport creates a transport that alternates between connected and disconnected states
func NewIntermittentTransport(inner transport.Transport, connectPeriod, failPeriod time.Duration) *IntermittentTransport {
	return &IntermittentTransport{
		innerTransport: inner,
		connected:      true,
		connectPeriod:  connectPeriod,
		failPeriod:     failPeriod,
		lastChange:     time.Now(),
	}
}

// SendMessage sends a message only when "connected"
func (it *IntermittentTransport) SendMessage(peerID string, address string, port int, msg *messages.Message) error {
	it.mu.Lock()
	now := time.Now()
	elapsed := now.Sub(it.lastChange)

	// Toggle connection state based on timing
	if it.connected && elapsed >= it.connectPeriod {
		it.connected = false
		it.lastChange = now
	} else if !it.connected && elapsed >= it.failPeriod {
		it.connected = true
		it.lastChange = now
	}

	isConnected := it.connected
	it.mu.Unlock()

	if !isConnected {
		return fmt.Errorf("simulated intermittent connection failure for peer %s", peerID)
	}

	return it.innerTransport.SendMessage(peerID, address, port, msg)
}

// Start starts the transport
func (it *IntermittentTransport) Start() error {
	return it.innerTransport.Start()
}

// Stop stops the transport
func (it *IntermittentTransport) Stop() error {
	return it.innerTransport.Stop()
}

// SetMessageHandler sets the message handler
func (it *IntermittentTransport) SetMessageHandler(handler transport.MessageHandler) error {
	return it.innerTransport.SetMessageHandler(handler)
}

// ConnectToPeer connects to a peer
func (it *IntermittentTransport) ConnectToPeer(peerID string, address string, port int) error {
	return it.innerTransport.ConnectToPeer(peerID, address, port)
}

// IsCurrentlyConnected returns whether the transport is currently in a "connected" state
func (it *IntermittentTransport) IsCurrentlyConnected() bool {
	it.mu.RLock()
	defer it.mu.RUnlock()
	return it.connected
}

// PartitioningTransport simulates network partitions between specific peers
type PartitioningTransport struct {
	innerTransport transport.Transport
	mu             sync.RWMutex

	// Partition configuration
	partitionedPeers map[string]bool // peerID -> is partitioned
}

// NewPartitioningTransport creates a transport that can partition specific peers
func NewPartitioningTransport(inner transport.Transport) *PartitioningTransport {
	return &PartitioningTransport{
		innerTransport:    inner,
		partitionedPeers: make(map[string]bool),
	}
}

// SendMessage sends a message, failing for partitioned peers
func (pt *PartitioningTransport) SendMessage(peerID string, address string, port int, msg *messages.Message) error {
	pt.mu.RLock()
	isPartitioned := pt.partitionedPeers[peerID]
	pt.mu.RUnlock()

	if isPartitioned {
		return fmt.Errorf("simulated network partition for peer %s", peerID)
	}

	return pt.innerTransport.SendMessage(peerID, address, port, msg)
}

// Start starts the transport
func (pt *PartitioningTransport) Start() error {
	return pt.innerTransport.Start()
}

// Stop stops the transport
func (pt *PartitioningTransport) Stop() error {
	return pt.innerTransport.Stop()
}

// SetMessageHandler sets the message handler
func (pt *PartitioningTransport) SetMessageHandler(handler transport.MessageHandler) error {
	return pt.innerTransport.SetMessageHandler(handler)
}

// ConnectToPeer connects to a peer
func (pt *PartitioningTransport) ConnectToPeer(peerID string, address string, port int) error {
	return pt.innerTransport.ConnectToPeer(peerID, address, port)
}

// PartitionPeer marks a peer as partitioned (unreachable)
func (pt *PartitioningTransport) PartitionPeer(peerID string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.partitionedPeers[peerID] = true
}

// ReconnectPeer removes partition for a peer
func (pt *PartitioningTransport) ReconnectPeer(peerID string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	delete(pt.partitionedPeers, peerID)
}

// IsPeerPartitioned checks if a peer is currently partitioned
func (pt *PartitioningTransport) IsPeerPartitioned(peerID string) bool {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	return pt.partitionedPeers[peerID]
}

// ThrottlingTransport simulates network bandwidth limitations
type ThrottlingTransport struct {
	innerTransport transport.Transport
	mu             sync.RWMutex

	// Throttling settings
	bandwidthBytesPerSec int64 // bytes per second
	lastSendTime         time.Time
	bytesSentSinceLast   int64
}

// NewThrottlingTransport creates a transport with bandwidth throttling
func NewThrottlingTransport(inner transport.Transport, bandwidthBytesPerSec int64) *ThrottlingTransport {
	return &ThrottlingTransport{
		innerTransport:       inner,
		bandwidthBytesPerSec: bandwidthBytesPerSec,
		lastSendTime:         time.Now(),
	}
}

// SendMessage sends a message with bandwidth throttling
func (tt *ThrottlingTransport) SendMessage(peerID string, address string, port int, msg *messages.Message) error {
	tt.mu.Lock()
	now := time.Now()
	elapsed := now.Sub(tt.lastSendTime)

	// Calculate how many bytes we can send in this time period
	maxBytes := int64(float64(tt.bandwidthBytesPerSec) * elapsed.Seconds())

	// Estimate message size (rough approximation)
	messageSize := int64(len(fmt.Sprintf("%v", msg))) + 100 // rough overhead

	if tt.bytesSentSinceLast+messageSize > maxBytes {
		// Calculate how long to wait
		excessBytes := tt.bytesSentSinceLast + messageSize - maxBytes
		waitTime := time.Duration(float64(excessBytes) / float64(tt.bandwidthBytesPerSec) * float64(time.Second))
		tt.mu.Unlock()

		time.Sleep(waitTime)

		tt.mu.Lock()
		tt.bytesSentSinceLast = messageSize
		tt.lastSendTime = time.Now()
	} else {
		tt.bytesSentSinceLast += messageSize
	}
	tt.mu.Unlock()

	return tt.innerTransport.SendMessage(peerID, address, port, msg)
}

// Start starts the transport
func (tt *ThrottlingTransport) Start() error {
	return tt.innerTransport.Start()
}

// Stop stops the transport
func (tt *ThrottlingTransport) Stop() error {
	return tt.innerTransport.Stop()
}

// SetMessageHandler sets the message handler
func (tt *ThrottlingTransport) SetMessageHandler(handler transport.MessageHandler) error {
	return tt.innerTransport.SetMessageHandler(handler)
}

// SetBandwidth updates the bandwidth limit
func (tt *ThrottlingTransport) SetBandwidth(bytesPerSec int64) {
	tt.mu.Lock()
	defer tt.mu.Unlock()
	tt.bandwidthBytesPerSec = bytesPerSec
	tt.lastSendTime = time.Now()
	tt.bytesSentSinceLast = 0
}

// CompositeFailingTransport combines multiple failure modes
type CompositeFailingTransport struct {
	innerTransport transport.Transport
	transports     []transport.Transport // Chain of transport wrappers
}

// NewCompositeFailingTransport creates a transport that can apply multiple failure modes
func NewCompositeFailingTransport(inner transport.Transport) *CompositeFailingTransport {
	return &CompositeFailingTransport{
		innerTransport: inner,
		transports:     []transport.Transport{inner},
	}
}

// AddFailureMode adds a failure mode to the transport chain
func (cft *CompositeFailingTransport) AddFailureMode(wrapper transport.Transport) {
	cft.transports = append([]transport.Transport{wrapper}, cft.transports...)
}

// SendMessage sends through the transport chain
func (cft *CompositeFailingTransport) SendMessage(peerID string, address string, port int, msg *messages.Message) error {
	// Use the first transport in the chain (the outermost wrapper)
	return cft.transports[0].SendMessage(peerID, address, port, msg)
}

// Start starts all transports in the chain
func (cft *CompositeFailingTransport) Start() error {
	for _, t := range cft.transports {
		if err := t.Start(); err != nil {
			return err
		}
	}
	return nil
}

// Stop stops all transports in the chain
func (cft *CompositeFailingTransport) Stop() error {
	for _, t := range cft.transports {
		if err := t.Stop(); err != nil {
			return err
		}
	}
	return nil
}

// SetMessageHandler sets the message handler on all transports in the chain
func (cft *CompositeFailingTransport) SetMessageHandler(handler transport.MessageHandler) error {
	for _, t := range cft.transports {
		if err := t.SetMessageHandler(handler); err != nil {
			return err
		}
	}
	return nil
}
