package transport

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
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
}

// NewQUICTransport creates a new QUIC transport
func NewQUICTransport(port int) *QUICTransport {
	// Create a minimal TLS config for QUIC
	// In production, this should use proper certificates
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{generateSelfSignedCert()},
		NextProtos:   []string{"p2p-sync"},
	}

	return &QUICTransport{
		port:        port,
		tlsConfig:   tlsConfig,
		stopCh:      make(chan struct{}),
		connections: make(map[string]*quic.Conn),
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

			go q.handleStream(stream)
		}
	}
}

// handleStream handles a QUIC stream
func (q *QUICTransport) handleStream(stream *quic.Stream) {
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

			// Handle message via message handler
			if q.handler != nil {
				if err := q.handler.HandleMessage(&msg); err != nil {
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

			if err := encoder.Encode(ack); err != nil {
				return
			}
		}
	}
}

// SendMessage sends a message to a specific peer
func (q *QUICTransport) SendMessage(peerID string, address string, port int, msg *messages.Message) error {
	addr := fmt.Sprintf("%s:%d", address, port)

	// Get or create connection
	conn, err := q.getOrCreateConnection(peerID, addr)
	if err != nil {
		return fmt.Errorf("failed to get connection to %s: %w", addr, err)
	}

	// Open a stream for sending the message
	stream, err := conn.OpenStreamSync(context.Background())
	if err != nil {
		// Remove failed connection
		q.connectionsMu.Lock()
		delete(q.connections, peerID)
		q.connectionsMu.Unlock()
		return fmt.Errorf("failed to open stream: %w", err)
	}
	defer stream.Close()

	// Encode and send the message
	encoder := json.NewEncoder(stream)
	if err := encoder.Encode(msg); err != nil {
		return fmt.Errorf("failed to encode message: %w", err)
	}

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
	return conn, nil
}

// SetMessageHandler sets the message handler for incoming messages
func (q *QUICTransport) SetMessageHandler(handler MessageHandler) error {
	q.handler = handler
	return nil
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
