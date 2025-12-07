package transport

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
	"golang.org/x/crypto/hkdf"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/connection"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/messages"
)

// QUICTransport handles QUIC connections
type QUICTransport struct {
	port         int
	listener     *quic.Listener
	tlsConfig    *tls.Config
	handler      MessageHandler
	stopCh       chan struct{}
	connections  map[string]*quic.Conn // peerID -> connection
	connectionsMu sync.RWMutex
	connManager  *connection.ConnectionManager
	localPeerID  string
	handshakesMu   sync.Mutex
	handshakesSent map[string]bool // peerID -> whether handshake was sent
}

// NewQUICTransport creates a new QUIC transport
func NewQUICTransport(port int) *QUICTransport {
	// Create a minimal TLS config for QUIC
	// In production, this should use proper certificates
	tlsConfig := &tls.Config{
		Certificates:       []tls.Certificate{generateSelfSignedCert()},
		NextProtos:         []string{"p2p-sync"},
		InsecureSkipVerify: true, // Skip certificate verification for self-signed certs in local/Docker deployments
	}

	return &QUICTransport{
		port:           port,
		tlsConfig:      tlsConfig,
		stopCh:         make(chan struct{}),
		connections:    make(map[string]*quic.Conn),
		handshakesSent: make(map[string]bool),
	}
}

// Start starts listening on QUIC
func (q *QUICTransport) Start() error {
	addr := fmt.Sprintf(":%d", q.port)

	listener, err := quic.ListenAddr(addr, q.tlsConfig, &quic.Config{
		KeepAlivePeriod: 60 * time.Second,
		MaxIdleTimeout:  300 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("failed to listen on QUIC: %w", err)
	}
	q.listener = listener

	go q.accept()

	return nil
}

// Stop stops the QUIC transport
func (q *QUICTransport) Stop() error {
	close(q.stopCh)
	if q.listener != nil {
		return q.listener.Close()
	}
	return nil
}

// accept accepts incoming QUIC connections
func (q *QUICTransport) accept() {
	for {
		select {
		case <-q.stopCh:
			return
		default:
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			conn, err := q.listener.Accept(ctx)
			cancel()

			if err != nil {
				if netErr, ok := err.(interface{ Timeout() bool }); ok && netErr.Timeout() {
					continue
				}
				continue
			}

			go q.handleConnection(conn)
		}
	}
}

// handleConnection handles a QUIC connection
func (q *QUICTransport) handleConnection(conn *quic.Conn) {
	defer conn.CloseWithError(0, "")

	for {
		select {
		case <-q.stopCh:
			return
		default:
			stream, err := conn.AcceptStream(context.Background())
			if err != nil {
				return
			}

			go q.handleStream(conn, stream)
		}
	}
}

// handleStream handles a QUIC stream
func (q *QUICTransport) handleStream(conn *quic.Conn, stream *quic.Stream) {
	defer stream.Close()

	decoder := json.NewDecoder(stream)
	encoder := json.NewEncoder(stream)

	for {
		select {
		case <-q.stopCh:
			return
		default:
			var msg messages.Message
			if err := decoder.Decode(&msg); err != nil {
				return
			}

			// Handle handshake messages specially
			if msg.Type == "handshake" && msg.SenderID != "" {
				fmt.Printf("DEBUG [QUIC]: Received handshake from %s on inbound stream\n", msg.SenderID)

				// Store the inbound connection so it can be reused for outbound messages
				q.connectionsMu.Lock()
				if _, exists := q.connections[msg.SenderID]; !exists {
					q.connections[msg.SenderID] = conn
					fmt.Printf("DEBUG [QUIC]: Stored inbound connection for peer %s\n", msg.SenderID)
				}
				q.connectionsMu.Unlock()

				// Register the inbound connection if not already registered
				if q.connManager != nil {
					// Get the remote address from the QUIC connection
					remoteAddr := conn.RemoteAddr().String()
					// Parse the address to extract host (remove port)
					host := remoteAddr
					port := q.port
					if colonIdx := len(remoteAddr) - 1; colonIdx > 0 {
						for i := len(remoteAddr) - 1; i >= 0; i-- {
							if remoteAddr[i] == ':' {
								host = remoteAddr[:i]
								break
							}
						}
					}

					fmt.Printf("DEBUG [QUIC]: Registering inbound connection from %s at %s:%d\n", msg.SenderID, host, port)
					_ = q.connManager.AddConnection(msg.SenderID, host, port)
					_ = q.connManager.UpdateConnectionState(msg.SenderID, connection.StateConnected)
				}

				// Set session key for the peer
				if q.localPeerID != "" && q.connManager != nil {
					sessionKey := q.deriveSessionKey(q.localPeerID, msg.SenderID)
					if err := q.connManager.SetSessionKey(msg.SenderID, sessionKey); err != nil {
						fmt.Printf("DEBUG [QUIC]: Failed to set session key for peer %s: %v\n", msg.SenderID, err)
					} else {
						fmt.Printf("DEBUG [QUIC]: Set session key for peer %s on inbound stream\n", msg.SenderID)
					}
				}

				// Send handshake response
				handshakeResponse := &messages.Message{
					ID:        messages.GenerateMessageID(),
					Type:      "handshake",
					SenderID:  q.localPeerID,
					Timestamp: time.Now().Unix(),
					Payload:   map[string]interface{}{"peer_id": q.localPeerID},
				}
				if err := encoder.Encode(handshakeResponse); err != nil {
					fmt.Printf("DEBUG [QUIC]: Failed to send handshake response to %s: %v\n", msg.SenderID, err)
				}
				return // Close this stream after handshake
			}

			// Handle message via message handler
			if q.handler != nil {
				if err := q.handler.HandleMessage(&msg); err != nil {
					// Send error acknowledgment with CorrelationID
					ack := messages.NewMessage(messages.TypeOperationAck, "", messages.OperationAckMessage{
						OperationID: msg.ID,
						Success:     false,
						Error:       err.Error(),
					}).WithCorrelationID(msg.ID)
					encoder.Encode(ack)
					continue
				}
			}

			// Send success acknowledgment with CorrelationID
			ack := messages.NewMessage(messages.TypeOperationAck, "", messages.OperationAckMessage{
				OperationID: msg.ID,
				Success:     true,
			}).WithCorrelationID(msg.ID)

			if err := encoder.Encode(ack); err != nil {
				return
			}
		}
	}
}

// SendMessage sends a message to a specific peer
func (q *QUICTransport) SendMessage(peerID string, address string, port int, msg *messages.Message) error {
	addr := fmt.Sprintf("%s:%d", address, port)
	fmt.Printf("DEBUG [QUIC.SendMessage]: Sending message %s (type: %s) to peer %s at %s\n", msg.ID, msg.Type, peerID, addr)

	// Get or create connection
	conn, err := q.getOrCreateConnection(peerID, addr)
	if err != nil {
		fmt.Printf("DEBUG [QUIC.SendMessage]: Failed to get connection to %s: %v\n", peerID, err)
		return fmt.Errorf("failed to get connection to %s: %w", addr, err)
	}
	fmt.Printf("DEBUG [QUIC.SendMessage]: Got connection for peer %s\n", peerID)

	// Open a stream for sending the message
	stream, err := conn.OpenStreamSync(context.Background())
	if err != nil {
		fmt.Printf("DEBUG [QUIC.SendMessage]: Failed to open stream to %s: %v\n", peerID, err)
		// Remove failed connection
		q.connectionsMu.Lock()
		delete(q.connections, peerID)
		q.connectionsMu.Unlock()
		return fmt.Errorf("failed to open stream: %w", err)
	}
	defer stream.Close()
	fmt.Printf("DEBUG [QUIC.SendMessage]: Opened stream to peer %s\n", peerID)

	// Encode and send the message
	encoder := json.NewEncoder(stream)
	if err := encoder.Encode(msg); err != nil {
		fmt.Printf("DEBUG [QUIC.SendMessage]: Failed to encode message to %s: %v\n", peerID, err)
		return fmt.Errorf("failed to encode message: %w", err)
	}
	fmt.Printf("DEBUG [QUIC.SendMessage]: Successfully sent message %s to peer %s\n", msg.ID, peerID)

	return nil
}

// getOrCreateConnection gets an existing connection or creates a new one
func (q *QUICTransport) getOrCreateConnection(peerID, addr string) (*quic.Conn, error) {
	q.connectionsMu.RLock()
	conn, exists := q.connections[peerID]
	q.connectionsMu.RUnlock()

	if exists {
		return conn, nil
	}

	// Create new connection
	q.connectionsMu.Lock()
	defer q.connectionsMu.Unlock()

	// Double-check after acquiring write lock
	if conn, exists := q.connections[peerID]; exists {
		return conn, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := quic.DialAddr(ctx, addr, q.tlsConfig, &quic.Config{
		KeepAlivePeriod: 60 * time.Second,
		MaxIdleTimeout:  300 * time.Second,
	})
	if err != nil {
		return nil, err
	}

	q.connections[peerID] = conn

	// Register connection with connection manager
	if q.connManager != nil {
		// Parse address to get host and port
		host := addr
		port := q.port
		if colonIdx := len(addr) - 1; colonIdx > 0 {
			for i := len(addr) - 1; i >= 0; i-- {
				if addr[i] == ':' {
					host = addr[:i]
					fmt.Sscanf(addr[i+1:], "%d", &port)
					break
				}
			}
		}
		q.connManager.AddConnection(peerID, host, port)
		q.connManager.UpdateConnectionState(peerID, connection.StateConnected)
	}

	// Start handleConnection goroutine for outbound connections to accept incoming streams
	go q.handleConnection(conn)
	fmt.Printf("DEBUG [QUIC]: Started handleConnection for outbound connection to %s\n", peerID)

	// Send handshake to exchange peer IDs
	if q.localPeerID != "" {
		q.handshakesMu.Lock()
		alreadySent := q.handshakesSent[peerID]
		q.handshakesSent[peerID] = true
		q.handshakesMu.Unlock()

		if !alreadySent {
			fmt.Printf("DEBUG [QUIC]: Sending handshake to %s with local peer ID %s\n", peerID, q.localPeerID)
			if err := q.sendAndReceiveHandshake(conn, peerID, q.localPeerID); err != nil {
				return nil, fmt.Errorf("failed to complete handshake: %w", err)
			}
		}
	}

	return conn, nil
}

// SetMessageHandler sets the message handler for incoming messages
func (q *QUICTransport) SetMessageHandler(handler MessageHandler) error {
	q.handler = handler
	return nil
}

// SetConnectionManager sets the connection manager
func (q *QUICTransport) SetConnectionManager(cm *connection.ConnectionManager) {
	q.connManager = cm
}

// SetPeerID sets the local peer ID for session key derivation
func (q *QUICTransport) SetPeerID(peerID string) {
	q.localPeerID = peerID
}

// Connect establishes a QUIC connection to a peer
func (q *QUICTransport) Connect(addr string) (*quic.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return quic.DialAddr(ctx, addr, q.tlsConfig, &quic.Config{
		KeepAlivePeriod: 60 * time.Second,
		MaxIdleTimeout:  300 * time.Second,
	})
}

// ConnectToPeer establishes an outbound connection to a peer and registers it
func (q *QUICTransport) ConnectToPeer(peerID, address string, port int) error {
	addr := fmt.Sprintf("%s:%d", address, port)
	_, err := q.getOrCreateConnection(peerID, addr)
	return err
}

// sendAndReceiveHandshake sends a handshake and receives the response on the same stream
func (q *QUICTransport) sendAndReceiveHandshake(conn *quic.Conn, peerID, localPeerID string) error {
	// Open a stream for the handshake exchange
	stream, err := conn.OpenStreamSync(context.Background())
	if err != nil {
		return fmt.Errorf("failed to open stream for handshake: %w", err)
	}
	defer stream.Close()

	// Send handshake message
	handshakeMsg := &messages.Message{
		ID:        messages.GenerateMessageID(),
		Type:      "handshake",
		SenderID:  localPeerID,
		Timestamp: time.Now().Unix(),
		Payload:   map[string]interface{}{"peer_id": localPeerID},
	}

	encoder := json.NewEncoder(stream)
	if err := encoder.Encode(handshakeMsg); err != nil {
		return fmt.Errorf("failed to encode handshake: %w", err)
	}

	fmt.Printf("DEBUG [QUIC]: Sent handshake to %s, waiting for response on same stream...\n", peerID)

	// Read response on the SAME stream with timeout
	type result struct {
		msg messages.Message
		err error
	}
	resultCh := make(chan result, 1)

	go func() {
		decoder := json.NewDecoder(stream)
		var msg messages.Message
		err := decoder.Decode(&msg)
		resultCh <- result{msg: msg, err: err}
	}()

	select {
	case res := <-resultCh:
		if res.err != nil {
			fmt.Printf("DEBUG [QUIC]: Failed to read handshake response from %s: %v\n", peerID, res.err)
			return fmt.Errorf("failed to decode handshake response: %w", res.err)
		}

		if res.msg.Type == "handshake" && res.msg.SenderID != "" {
			fmt.Printf("DEBUG [QUIC]: Received handshake response from %s on same stream\n", res.msg.SenderID)

			// Set session key with the peer ID
			if q.localPeerID != "" && q.connManager != nil {
				sessionKey := q.deriveSessionKey(q.localPeerID, res.msg.SenderID)
				if err := q.connManager.SetSessionKey(res.msg.SenderID, sessionKey); err != nil {
					fmt.Printf("DEBUG [QUIC]: Failed to set session key for peer %s: %v\n", res.msg.SenderID, err)
					return fmt.Errorf("failed to set session key: %w", err)
				}
				fmt.Printf("DEBUG [QUIC]: Set session key for peer %s on outbound connection\n", res.msg.SenderID)
			}
			return nil
		}

		return fmt.Errorf("invalid handshake response from %s", peerID)

	case <-time.After(10 * time.Second):
		fmt.Printf("DEBUG [QUIC]: Timeout waiting for handshake response from %s\n", peerID)
		return fmt.Errorf("timeout waiting for handshake response")
	}
}

// sendHandshake sends a handshake message with the local peer ID
func (q *QUICTransport) sendHandshake(conn *quic.Conn, localPeerID string) error {
	handshakeMsg := &messages.Message{
		ID:        messages.GenerateMessageID(),
		Type:      "handshake",
		SenderID:  localPeerID,
		Timestamp: time.Now().Unix(),
		Payload:   map[string]interface{}{"peer_id": localPeerID},
	}

	// Open a stream for sending the handshake
	stream, err := conn.OpenStreamSync(context.Background())
	if err != nil {
		return fmt.Errorf("failed to open stream for handshake: %w", err)
	}
	defer stream.Close()

	encoder := json.NewEncoder(stream)
	return encoder.Encode(handshakeMsg)
}

// deriveSessionKey derives a session key from two peer IDs
func (q *QUICTransport) deriveSessionKey(peerID1, peerID2 string) []byte {
	// Sort peer IDs to ensure both sides derive the same key
	peerIDs := []string{peerID1, peerID2}
	sort.Strings(peerIDs)

	// Create a deterministic shared secret from the sorted peer IDs
	combined := peerIDs[0] + ":" + peerIDs[1]
	sharedSecret := sha256.Sum256([]byte(combined))

	// Derive session key using HKDF
	salt := []byte("p2p-sync-session-salt")
	info := []byte("p2p-sync-session-key")
	hkdfReader := hkdf.New(sha256.New, sharedSecret[:], salt, info)

	sessionKey := make([]byte, 32)
	if _, err := io.ReadFull(hkdfReader, sessionKey); err != nil {
		panic(fmt.Sprintf("failed to derive session key: %v", err))
	}

	return sessionKey
}

// generateSelfSignedCert generates a self-signed certificate for testing
// In production, this should use proper certificates
func generateSelfSignedCert() tls.Certificate {
	// For development/testing, we can generate a self-signed cert
	// In production, use proper certificate management (Let's Encrypt, etc.)

	// Generate a minimal self-signed certificate
	// This is not secure for production use
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(fmt.Sprintf("failed to generate private key: %v", err))
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"p2p-sync"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:  x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		panic(fmt.Sprintf("failed to create certificate: %v", err))
	}

	return tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  priv,
	}
}
