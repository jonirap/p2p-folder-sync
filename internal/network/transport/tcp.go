package transport

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/p2p-folder-sync/p2p-sync/internal/network/messages"
)

// TCPTransport handles TCP connections
type TCPTransport struct {
	port         int
	listener     *net.TCPListener
	handler      MessageHandler
	stopCh       chan struct{}
	stopOnce     sync.Once
	connections  map[string]net.Conn // peerID -> connection
	connectionsMu sync.RWMutex
}

// NewTCPTransport creates a new TCP transport
func NewTCPTransport(port int) *TCPTransport {
	return &TCPTransport{
		port:        port,
		stopCh:      make(chan struct{}),
		connections: make(map[string]net.Conn),
	}
}

// Start starts listening on TCP
func (t *TCPTransport) Start() error {
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf(":%d", t.port))
	if err != nil {
		return fmt.Errorf("failed to resolve TCP address: %w", err)
	}

	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on TCP: %w", err)
	}
	t.listener = listener

	go t.accept()

	return nil
}

// Stop stops the TCP transport
func (t *TCPTransport) Stop() error {
	var err error
	t.stopOnce.Do(func() {
		close(t.stopCh)
		if t.listener != nil {
			err = t.listener.Close()
		}
	})
	return err
}

// accept accepts incoming connections
func (t *TCPTransport) accept() {
	for {
		select {
		case <-t.stopCh:
			return
		default:
			t.listener.SetDeadline(time.Now().Add(1 * time.Second))
			conn, err := t.listener.AcceptTCP()
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				continue
			}

			go t.handleConnection(conn)
		}
	}
}

// handleConnection handles a TCP connection
func (t *TCPTransport) handleConnection(conn *net.TCPConn) {
	defer conn.Close()

	// Set keep-alive
	conn.SetKeepAlive(true)
	conn.SetKeepAlivePeriod(60 * time.Second)

	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	for {
		select {
		case <-t.stopCh:
			return
		default:
			var msg messages.Message
			if err := decoder.Decode(&msg); err != nil {
				return
			}

			// Handle message via message handler
			if t.handler != nil {
				if err := t.handler.HandleMessage(&msg); err != nil {
					// Send error acknowledgment
					ack := messages.NewMessage(messages.TypeOperationAck, "", messages.OperationAckMessage{
						OperationID: msg.ID,
						Success:     false,
						Error:       err.Error(),
					})
					encoder.Encode(ack)
					continue
				}
			}

			// Send success acknowledgment
			ack := messages.NewMessage(messages.TypeOperationAck, "", messages.OperationAckMessage{
				OperationID: msg.ID,
				Success:     true,
			})
			encoder.Encode(ack)
		}
	}
}

// SendMessage sends a message to a specific peer
func (t *TCPTransport) SendMessage(peerID string, address string, port int, msg *messages.Message) error {
	// Get or create connection
	conn, err := t.getOrCreateConnection(peerID, address, port)
	if err != nil {
		return fmt.Errorf("failed to get connection to %s:%d: %w", address, port, err)
	}

	// Encode and send the message
	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(msg); err != nil {
		// Remove failed connection
		t.connectionsMu.Lock()
		delete(t.connections, peerID)
		conn.Close()
		t.connectionsMu.Unlock()
		return fmt.Errorf("failed to encode message: %w", err)
	}

	return nil
}

// getOrCreateConnection gets an existing connection or creates a new one
func (t *TCPTransport) getOrCreateConnection(peerID, address string, port int) (net.Conn, error) {
	t.connectionsMu.RLock()
	conn, exists := t.connections[peerID]
	t.connectionsMu.RUnlock()

	if exists {
		return conn, nil
	}

	// Create new connection
	t.connectionsMu.Lock()
	defer t.connectionsMu.Unlock()

	// Double-check after acquiring write lock
	if conn, exists := t.connections[peerID]; exists {
		return conn, nil
	}

	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", address, port))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve address: %w", err)
	}

	conn, err = net.DialTCP("tcp", nil, addr)
	if err != nil {
		return nil, err
	}

	// Set keep-alive
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(60 * time.Second)
	}

	t.connections[peerID] = conn
	return conn, nil
}

// SetMessageHandler sets the message handler for incoming messages
func (t *TCPTransport) SetMessageHandler(handler MessageHandler) error {
	t.handler = handler
	return nil
}

// Connect connects to a remote peer
func (t *TCPTransport) Connect(address string, port int) (net.Conn, error) {
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", address, port))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve address: %w", err)
	}

	conn, err := net.DialTCP("tcp", nil, addr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	conn.SetKeepAlive(true)
	conn.SetKeepAlivePeriod(60 * time.Second)

	return conn, nil
}

