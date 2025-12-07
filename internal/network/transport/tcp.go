package transport

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/p2p-folder-sync/p2p-sync/internal/crypto"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/connection"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/messages"
)

// TCPTransport handles TCP connections
type TCPTransport struct {
	port           int
	listener       *net.TCPListener
	handler        MessageHandler
	connManager    *connection.ConnectionManager
	stopCh         chan struct{}
	stopOnce       sync.Once
	connections    map[string]net.Conn // peerID -> connection
	connectionsMu  sync.RWMutex
	localPeerID    string // This transport's peer ID
	handshakesSent map[string]bool // Track which peers we've sent handshakes to
	handshakesMu   sync.RWMutex
}

// NewTCPTransport creates a new TCP transport
func NewTCPTransport(port int) *TCPTransport {
	return &TCPTransport{
		port:           port,
		stopCh:         make(chan struct{}),
		connections:    make(map[string]net.Conn),
		handshakesSent: make(map[string]bool),
	}
}

// SetConnectionManager sets the connection manager for this transport
func (t *TCPTransport) SetConnectionManager(cm *connection.ConnectionManager) {
	t.connManager = cm
}

// SetPeerID sets the local peer ID for this transport
func (t *TCPTransport) SetPeerID(peerID string) {
	t.localPeerID = peerID
}

// deriveSessionKey derives a deterministic session key from two peer IDs
// Both peers will derive the same key by sorting the IDs
func (t *TCPTransport) deriveSessionKey(peerID1, peerID2 string) []byte {
	// Sort peer IDs to ensure both sides derive the same key
	peerIDs := []string{peerID1, peerID2}
	sort.Strings(peerIDs)

	// Create a deterministic shared secret from the sorted peer IDs
	combined := peerIDs[0] + ":" + peerIDs[1]
	sharedSecret := sha256.Sum256([]byte(combined))

	// Derive session key using HKDF
	salt := []byte("p2p-sync-session-salt")
	info := []byte("p2p-sync-aes-gcm-key")
	sessionKey, err := crypto.DeriveSessionKey(sharedSecret[:], salt, info)
	if err != nil {
		// Fallback to using the hash directly (should not happen in practice)
		return sharedSecret[:]
	}

	return sessionKey
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

	fmt.Printf("DEBUG [TCP handleConnection]: Started for connection from %v\n", conn.RemoteAddr())

	// Set keep-alive
	conn.SetKeepAlive(true)
	conn.SetKeepAlivePeriod(60 * time.Second)

	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	var peerID string
	connectionRegistered := false

	for {
		select {
		case <-t.stopCh:
			fmt.Printf("DEBUG [TCP handleConnection]: Stopped via stopCh for peer %s\n", peerID)
			return
		default:
			var msg messages.Message
			if err := decoder.Decode(&msg); err != nil {
				fmt.Printf("DEBUG [TCP handleConnection]: Decode error from %s (connection %v): %v\n", peerID, conn.RemoteAddr(), err)
				return
			}
			fmt.Printf("DEBUG [TCP handleConnection]: Decoded message type=%s from %s\n", msg.Type, msg.SenderID)

			// Register incoming connection on first message if we have a connection manager
			if !connectionRegistered && t.connManager != nil && msg.SenderID != "" {
				peerID = msg.SenderID

				// Check if this is a handshake message
				if msg.Type == "handshake" {
					fmt.Printf("DEBUG [TCP]: Received handshake from %s\n", peerID)

					// Get the remote address for the connection
					remoteAddr := conn.RemoteAddr().String()
					host, portStr, err := net.SplitHostPort(remoteAddr)
					if err != nil {
						host = "unknown"
						portStr = "0"
					}
					port := 0
					fmt.Sscanf(portStr, "%d", &port)

					// Add the connection to the connection manager
					t.connManager.AddConnection(peerID, host, port)
					t.connManager.UpdateConnectionState(peerID, connection.StateConnected)

					// Derive and set session key now that we know the real peer ID
					if t.localPeerID != "" {
						sessionKey := t.deriveSessionKey(t.localPeerID, peerID)
						if err := t.connManager.SetSessionKey(peerID, sessionKey); err != nil {
							fmt.Fprintf(os.Stderr, "Failed to set session key for peer %s: %v\n", peerID, err)
						} else {
							fmt.Printf("DEBUG [TCP]: Set session key for peer %s\n", peerID)
						}
					}

					// Store the connection for our internal tracking ONLY if we don't already have one
					// (to avoid overwriting outbound connections with incoming ones)
					t.connectionsMu.Lock()
					if _, exists := t.connections[peerID]; !exists {
						t.connections[peerID] = conn
						fmt.Printf("DEBUG [TCP]: Stored incoming connection for peer %s\n", peerID)
					} else {
						fmt.Printf("DEBUG [TCP]: Skipping storage of incoming connection for peer %s (already have outbound)\n", peerID)
					}
					t.connectionsMu.Unlock()

					// Send handshake acknowledgment
					t.sendHandshake(conn, t.localPeerID)

					connectionRegistered = true
					continue // Don't pass handshake to handler
				}

				// For non-handshake first messages, register normally
				remoteAddr := conn.RemoteAddr().String()
				host, portStr, err := net.SplitHostPort(remoteAddr)
				if err != nil {
					host = "unknown"
					portStr = "0"
				}
				port := 0
				fmt.Sscanf(portStr, "%d", &port)

				t.connManager.AddConnection(peerID, host, port)
				t.connManager.UpdateConnectionState(peerID, connection.StateConnected)

				t.connectionsMu.Lock()
				if _, exists := t.connections[peerID]; !exists {
					t.connections[peerID] = conn
					fmt.Printf("DEBUG [TCP]: Stored incoming connection for peer %s (non-handshake first message)\n", peerID)
				} else {
					fmt.Printf("DEBUG [TCP]: Skipping storage of incoming connection for peer %s (already have outbound, non-handshake first)\n", peerID)
				}
				t.connectionsMu.Unlock()

				connectionRegistered = true
			}

			// Handle handshake responses (when we get a handshake back after initiating)
			if msg.Type == "handshake" && connectionRegistered {
				// Check if this is a handshake response (peerID is a placeholder like "peer-beta")
				// vs a handshake from an incoming connection (peerID is already the real ID)
				if msg.SenderID != peerID && t.localPeerID != "" {
					// This is a response to our handshake - update the connection mapping
					fmt.Printf("DEBUG [TCP]: Received handshake response from %s (was %s), updating\n", msg.SenderID, peerID)
					t.updatePeerID(peerID, msg.SenderID)
					peerID = msg.SenderID

					// Set session key with the real peer ID
					sessionKey := t.deriveSessionKey(t.localPeerID, peerID)
					if err := t.connManager.SetSessionKey(peerID, sessionKey); err != nil {
						fmt.Fprintf(os.Stderr, "Failed to set session key for peer %s: %v\n", peerID, err)
					} else {
						fmt.Printf("DEBUG [TCP]: Updated session key for peer %s\n", peerID)
					}
				} else {
					fmt.Printf("DEBUG [TCP]: Received handshake from %s (already registered)\n", msg.SenderID)
				}
				continue // Don't pass handshake to handler
			}

			// Handle message via message handler
			fmt.Printf("DEBUG [TCP handleConnection]: Received message type=%s from sender=%s, handler=%v\n", msg.Type, msg.SenderID, t.handler != nil)
			if t.handler != nil {
				if err := t.handler.HandleMessage(&msg); err != nil {
					fmt.Printf("DEBUG [TCP handleConnection]: Handler error for type=%s: %v\n", msg.Type, err)
					// Send error acknowledgment
					ack := messages.NewMessage(messages.TypeOperationAck, "", messages.OperationAckMessage{
						OperationID: msg.ID,
						Success:     false,
						Error:       err.Error(),
					})
					encoder.Encode(ack)
					continue
				}
				fmt.Printf("DEBUG [TCP handleConnection]: Handler success for type=%s\n", msg.Type)
			} else {
				fmt.Printf("DEBUG [TCP handleConnection]: No handler set! Message type=%s dropped\n", msg.Type)
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

	fmt.Printf("DEBUG [TCP SendMessage]: Sending type=%s to peer=%s on connection %v\n", msg.Type, peerID, conn.RemoteAddr())

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

	// Register connection with connection manager (temporarily with placeholder ID)
	if t.connManager != nil {
		t.connManager.AddConnection(peerID, address, port)
		t.connManager.UpdateConnectionState(peerID, connection.StateConnected)
	}

	// Send handshake to exchange peer IDs
	if t.localPeerID != "" {
		t.handshakesMu.Lock()
		alreadySent := t.handshakesSent[peerID]
		t.handshakesSent[peerID] = true
		t.handshakesMu.Unlock()

		if !alreadySent {
			fmt.Printf("DEBUG [TCP]: Sending handshake to %s with local peer ID %s\n", peerID, t.localPeerID)
			if err := t.sendHandshake(conn, t.localPeerID); err != nil {
				return nil, fmt.Errorf("failed to send handshake: %w", err)
			}

			// Wait for handshake response synchronously to ensure session key is established
			if err := t.readHandshakeResponse(conn, peerID); err != nil {
				return nil, fmt.Errorf("failed to complete handshake: %w", err)
			}
		}
	}

	return conn, nil
}

// readHandshakeResponse reads a single handshake response from an outbound connection
func (t *TCPTransport) readHandshakeResponse(conn net.Conn, placeholderPeerID string) error {
	decoder := json.NewDecoder(conn)
	var msg messages.Message

	// Set a timeout for receiving the handshake
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	defer conn.SetReadDeadline(time.Time{}) // Clear deadline after reading

	if err := decoder.Decode(&msg); err != nil {
		fmt.Printf("DEBUG [TCP]: Failed to read handshake response from %s: %v\n", placeholderPeerID, err)
		return fmt.Errorf("failed to decode handshake response: %w", err)
	}

	if msg.Type == "handshake" && msg.SenderID != "" {
		fmt.Printf("DEBUG [TCP]: Received handshake response from %s (was %s), updating\n", msg.SenderID, placeholderPeerID)

		// Update the connection mapping with the real peer ID
		t.updatePeerID(placeholderPeerID, msg.SenderID)

		// Set session key with the real peer ID
		if t.localPeerID != "" {
			sessionKey := t.deriveSessionKey(t.localPeerID, msg.SenderID)
			if err := t.connManager.SetSessionKey(msg.SenderID, sessionKey); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to set session key for peer %s: %v\n", msg.SenderID, err)
				return fmt.Errorf("failed to set session key: %w", err)
			} else {
				fmt.Printf("DEBUG [TCP]: Set session key for peer %s on outbound connection\n", msg.SenderID)
			}
		}
		return nil
	}

	return fmt.Errorf("invalid handshake response from %s", placeholderPeerID)
}

// sendHandshake sends a handshake message with the local peer ID
func (t *TCPTransport) sendHandshake(conn net.Conn, localPeerID string) error {
	handshakeMsg := &messages.Message{
		ID:        messages.GenerateMessageID(),
		Type:      "handshake",
		SenderID:  localPeerID,
		Timestamp: time.Now().Unix(),
		Payload:   map[string]interface{}{"peer_id": localPeerID},
	}

	encoder := json.NewEncoder(conn)
	return encoder.Encode(handshakeMsg)
}

// updatePeerID updates a peer ID in the connection map (when we learn the real peer ID)
func (t *TCPTransport) updatePeerID(oldID, newID string) {
	t.connectionsMu.Lock()
	defer t.connectionsMu.Unlock()

	if conn, exists := t.connections[oldID]; exists {
		delete(t.connections, oldID)
		t.connections[newID] = conn
		fmt.Printf("DEBUG [TCP]: Updated connection mapping from %s to %s\n", oldID, newID)
	}

	// Also update connection manager
	if t.connManager != nil {
		if oldConn, err := t.connManager.GetConnection(oldID); err == nil {
			t.connManager.RemoveConnection(oldID)
			t.connManager.AddConnection(newID, oldConn.Address, oldConn.Port)
			t.connManager.UpdateConnectionState(newID, connection.StateConnected)
		}
	}
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

// ConnectToPeer establishes an outbound connection to a peer and registers it
func (t *TCPTransport) ConnectToPeer(peerID, address string, port int) error {
	_, err := t.getOrCreateConnection(peerID, address, port)
	return err
}
