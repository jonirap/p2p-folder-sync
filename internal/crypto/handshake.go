package crypto

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"golang.org/x/crypto/hkdf"
	"io"
)

const (
	// NonceSize is the size of nonces in bytes
	NonceSize = 32
	// ChallengeSize is the size of authentication challenges in bytes
	ChallengeSize = 32
	// SessionKeySize is the size of session keys in bytes
	SessionKeySize = 32
	// SessionKeyRotationInterval is how often to rotate session keys (24 hours)
	SessionKeyRotationInterval = 24 * time.Hour
)

// SessionKey represents an encrypted session key with metadata
type SessionKey struct {
	Key       []byte
	CreatedAt time.Time
	ExpiresAt time.Time
	PeerID    string
}

// HandshakeManager manages key exchange and authentication
type HandshakeManager struct {
	keyPair      *KeyPair
	preSharedKey []byte // Optional PSK for authentication
	sessions     map[string]*SessionKey
	mu           sync.RWMutex
	rotationDone chan struct{}
}

// HandshakeState tracks the state of an ongoing handshake
type HandshakeState struct {
	OurNonce      []byte
	OurChallenge  []byte
	PeerNonce     []byte
	PeerPublicKey []byte
	SharedSecret  []byte
	PeerID        string
}

// NewHandshakeManager creates a new handshake manager
func NewHandshakeManager(preSharedKey []byte) (*HandshakeManager, error) {
	keyPair, err := GenerateKeyPair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate key pair: %w", err)
	}

	hm := &HandshakeManager{
		keyPair:      keyPair,
		preSharedKey: preSharedKey,
		sessions:     make(map[string]*SessionKey),
		rotationDone: make(chan struct{}),
	}

	// Start key rotation goroutine
	go hm.rotateKeys()

	return hm, nil
}

// Stop stops the handshake manager
func (hm *HandshakeManager) Stop() {
	close(hm.rotationDone)
}

// InitiateHandshake creates the initial handshake message (Step 1)
// Returns: publicKey, nonce, authChallenge
func (hm *HandshakeManager) InitiateHandshake() (string, []byte, []byte, error) {
	nonce := make([]byte, NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return "", nil, nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	challenge := make([]byte, ChallengeSize)
	if _, err := rand.Read(challenge); err != nil {
		return "", nil, nil, fmt.Errorf("failed to generate challenge: %w", err)
	}

	publicKey := EncodePublicKey(hm.keyPair.PublicKey)
	return publicKey, nonce, challenge, nil
}

// ProcessHandshake processes an incoming handshake and generates challenge response (Step 2)
// Returns: authResponse, publicKey, nonce, authChallenge
func (hm *HandshakeManager) ProcessHandshake(peerPublicKey string, peerNonce, peerChallenge []byte) ([]byte, string, []byte, []byte, *HandshakeState, error) {
	// Decode peer's public key
	peerPubKey, err := DecodePublicKey(peerPublicKey)
	if err != nil {
		return nil, "", nil, nil, nil, fmt.Errorf("failed to decode peer public key: %w", err)
	}

	// Compute shared secret
	sharedSecret, err := ComputeSharedSecret(hm.keyPair.PrivateKey, peerPubKey)
	if err != nil {
		return nil, "", nil, nil, nil, fmt.Errorf("failed to compute shared secret: %w", err)
	}

	// Generate authentication response to peer's challenge
	authResponse := hm.generateAuthResponse(sharedSecret, peerChallenge)

	// Generate our nonce and challenge
	ourNonce := make([]byte, NonceSize)
	if _, err := rand.Read(ourNonce); err != nil {
		return nil, "", nil, nil, nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ourChallenge := make([]byte, ChallengeSize)
	if _, err := rand.Read(ourChallenge); err != nil {
		return nil, "", nil, nil, nil, fmt.Errorf("failed to generate challenge: %w", err)
	}

	// Create handshake state for verification
	state := &HandshakeState{
		OurNonce:      ourNonce,
		OurChallenge:  ourChallenge,
		PeerNonce:     peerNonce,
		PeerPublicKey: peerPubKey,
		SharedSecret:  sharedSecret,
	}

	ourPublicKey := EncodePublicKey(hm.keyPair.PublicKey)
	return authResponse, ourPublicKey, ourNonce, ourChallenge, state, nil
}

// CompleteHandshake completes the handshake by verifying final response (Step 3)
func (hm *HandshakeManager) CompleteHandshake(state *HandshakeState, peerAuthResponse []byte, peerID string) (*SessionKey, error) {
	// Verify peer's authentication response
	expectedResponse := hm.generateAuthResponse(state.SharedSecret, state.OurChallenge)
	if !hmac.Equal(expectedResponse, peerAuthResponse) {
		return nil, fmt.Errorf("authentication failed: invalid response")
	}

	// Derive session key using HKDF
	sessionKey, err := hm.deriveSessionKey(state.SharedSecret, state.PeerNonce, state.OurNonce)
	if err != nil {
		return nil, fmt.Errorf("failed to derive session key: %w", err)
	}

	// Create and store session
	session := &SessionKey{
		Key:       sessionKey,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(SessionKeyRotationInterval),
		PeerID:    peerID,
	}

	hm.mu.Lock()
	hm.sessions[peerID] = session
	hm.mu.Unlock()

	return session, nil
}

// VerifyAndComplete verifies the handshake response and derives session key (Initiator's Step 3)
func (hm *HandshakeManager) VerifyAndComplete(ourNonce, ourChallenge []byte, peerPublicKey string, peerNonce, peerChallenge, peerAuthResponse []byte, peerID string) (*SessionKey, error) {
	// Decode peer's public key
	peerPubKey, err := DecodePublicKey(peerPublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode peer public key: %w", err)
	}

	// Compute shared secret
	sharedSecret, err := ComputeSharedSecret(hm.keyPair.PrivateKey, peerPubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to compute shared secret: %w", err)
	}

	// Verify peer's authentication response
	expectedResponse := hm.generateAuthResponse(sharedSecret, ourChallenge)
	if !hmac.Equal(expectedResponse, peerAuthResponse) {
		return nil, fmt.Errorf("authentication failed: invalid response")
	}

	// Generate our final auth response
	ourAuthResponse := hm.generateAuthResponse(sharedSecret, peerChallenge)

	// Derive session key using HKDF
	sessionKey, err := hm.deriveSessionKey(sharedSecret, ourNonce, peerNonce)
	if err != nil {
		return nil, fmt.Errorf("failed to derive session key: %w", err)
	}

	// Create and store session
	session := &SessionKey{
		Key:       sessionKey,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(SessionKeyRotationInterval),
		PeerID:    peerID,
	}

	hm.mu.Lock()
	hm.sessions[peerID] = session
	hm.mu.Unlock()

	// Return our final auth response along with session
	_ = ourAuthResponse // Will be sent in handshake_complete message

	return session, nil
}

// GetSessionKey retrieves the session key for a peer
func (hm *HandshakeManager) GetSessionKey(peerID string) (*SessionKey, error) {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	session, ok := hm.sessions[peerID]
	if !ok {
		return nil, fmt.Errorf("no session found for peer %s", peerID)
	}

	// Check if session has expired
	if time.Now().After(session.ExpiresAt) {
		return nil, fmt.Errorf("session expired for peer %s", peerID)
	}

	return session, nil
}

// RemoveSession removes a session for a peer
func (hm *HandshakeManager) RemoveSession(peerID string) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	delete(hm.sessions, peerID)
}

// generateAuthResponse generates an HMAC-based authentication response
func (hm *HandshakeManager) generateAuthResponse(sharedSecret, challenge []byte) []byte {
	// Use HMAC-SHA256 with shared secret as key
	// If PSK is available, include it in the key derivation
	key := sharedSecret
	if hm.preSharedKey != nil {
		h := sha256.New()
		h.Write(sharedSecret)
		h.Write(hm.preSharedKey)
		key = h.Sum(nil)
	}

	mac := hmac.New(sha256.New, key)
	mac.Write(challenge)
	return mac.Sum(nil)
}

// deriveSessionKey derives a session key using HKDF-SHA256
func (hm *HandshakeManager) deriveSessionKey(sharedSecret, nonceA, nonceB []byte) ([]byte, error) {
	// Create info string for HKDF
	info := []byte("p2p-sync-session")

	// Combine nonces (ensure deterministic ordering)
	salt := make([]byte, 0, len(nonceA)+len(nonceB))
	salt = append(salt, nonceA...)
	salt = append(salt, nonceB...)

	// Derive key using HKDF
	hkdfReader := hkdf.New(sha256.New, sharedSecret, salt, info)
	sessionKey := make([]byte, SessionKeySize)
	if _, err := io.ReadFull(hkdfReader, sessionKey); err != nil {
		return nil, fmt.Errorf("failed to derive session key: %w", err)
	}

	return sessionKey, nil
}

// rotateKeys periodically rotates expired session keys
func (hm *HandshakeManager) rotateKeys() {
	ticker := time.NewTicker(1 * time.Hour) // Check every hour
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			hm.cleanupExpiredSessions()
		case <-hm.rotationDone:
			return
		}
	}
}

// cleanupExpiredSessions removes expired sessions
func (hm *HandshakeManager) cleanupExpiredSessions() {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	now := time.Now()
	for peerID, session := range hm.sessions {
		if now.After(session.ExpiresAt) {
			delete(hm.sessions, peerID)
		}
	}
}

// IsSessionValid checks if a session is valid and not expired
func (hm *HandshakeManager) IsSessionValid(peerID string) bool {
	session, err := hm.GetSessionKey(peerID)
	return err == nil && session != nil
}

// GetPrivateKey returns the private key (for testing purposes)
func (hm *HandshakeManager) GetPrivateKey() []byte {
	return hm.keyPair.PrivateKey
}

// GetPublicKey returns the public key (for testing purposes)
func (hm *HandshakeManager) GetPublicKey() []byte {
	return hm.keyPair.PublicKey
}
