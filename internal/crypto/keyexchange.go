package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/curve25519"
)

const (
	// KeySize is the size of Curve25519 keys in bytes
	KeySize = 32
)

// KeyPair represents a public/private key pair
type KeyPair struct {
	PrivateKey []byte
	PublicKey  []byte
}

// GenerateKeyPair generates a new Curve25519 key pair
func GenerateKeyPair() (*KeyPair, error) {
	privateKey := make([]byte, KeySize)
	if _, err := rand.Read(privateKey); err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Apply Curve25519 clamping
	privateKey[0] &= 248
	privateKey[31] &= 127
	privateKey[31] |= 64

	// Compute public key
	publicKey, err := curve25519.X25519(privateKey, curve25519.Basepoint)
	if err != nil {
		return nil, fmt.Errorf("failed to compute public key: %w", err)
	}

	return &KeyPair{
		PrivateKey: privateKey,
		PublicKey:  publicKey,
	}, nil
}

// ComputeSharedSecret computes the shared secret using ECDH
func ComputeSharedSecret(privateKey, peerPublicKey []byte) ([]byte, error) {
	if len(privateKey) != KeySize {
		return nil, fmt.Errorf("invalid private key size: %d", len(privateKey))
	}
	if len(peerPublicKey) != KeySize {
		return nil, fmt.Errorf("invalid public key size: %d", len(peerPublicKey))
	}

	sharedSecret, err := curve25519.X25519(privateKey, peerPublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to compute shared secret: %w", err)
	}

	return sharedSecret, nil
}

// EncodePublicKey encodes a public key to base64
func EncodePublicKey(publicKey []byte) string {
	return base64.StdEncoding.EncodeToString(publicKey)
}

// DecodePublicKey decodes a base64-encoded public key
func DecodePublicKey(encoded string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode public key: %w", err)
	}
	if len(decoded) != KeySize {
		return nil, fmt.Errorf("invalid public key size: %d", len(decoded))
	}
	return decoded, nil
}

// EncodePrivateKey encodes a private key to base64 (for storage)
func EncodePrivateKey(privateKey []byte) string {
	return base64.StdEncoding.EncodeToString(privateKey)
}

// DecodePrivateKey decodes a base64-encoded private key
func DecodePrivateKey(encoded string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode private key: %w", err)
	}
	if len(decoded) != KeySize {
		return nil, fmt.Errorf("invalid private key size: %d", len(decoded))
	}
	return decoded, nil
}

// GenerateECDHKeyPair is an alias for GenerateKeyPair for compatibility
func GenerateECDHKeyPair() (*KeyPair, error) {
	return GenerateKeyPair()
}
