package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/sha3"
)

const (
	// IVSize is the size of the initialization vector for AES-GCM (96 bits = 12 bytes)
	IVSize = 12
	// TagSize is the size of the GCM authentication tag (128 bits = 16 bytes)
	TagSize = 16
	// KeySizeAES is the size of AES-256 keys (256 bits = 32 bytes)
	KeySizeAES = 32
)

// EncryptedMessage represents an encrypted message
type EncryptedMessage struct {
	IV         []byte `json:"iv"`
	Ciphertext []byte `json:"ciphertext"`
	Tag        []byte `json:"tag"`
}

// Encrypt encrypts data using AES-256-GCM
func Encrypt(plaintext []byte, key []byte) (*EncryptedMessage, error) {
	if len(key) != KeySizeAES {
		return nil, fmt.Errorf("invalid key size: %d (expected %d)", len(key), KeySizeAES)
	}

	// Create cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate random IV
	iv := make([]byte, IVSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, fmt.Errorf("failed to generate IV: %w", err)
	}

	// Encrypt and authenticate
	ciphertext := aesGCM.Seal(nil, iv, plaintext, nil)

	// Split ciphertext and tag
	tag := ciphertext[len(ciphertext)-TagSize:]
	ciphertextOnly := ciphertext[:len(ciphertext)-TagSize]

	return &EncryptedMessage{
		IV:         iv,
		Ciphertext: ciphertextOnly,
		Tag:        tag,
	}, nil
}

// Decrypt decrypts data using AES-256-GCM
func Decrypt(msg *EncryptedMessage, key []byte) ([]byte, error) {
	if len(key) != KeySizeAES {
		return nil, fmt.Errorf("invalid key size: %d (expected %d)", len(key), KeySizeAES)
	}
	if len(msg.IV) != IVSize {
		return nil, fmt.Errorf("invalid IV size: %d (expected %d)", len(msg.IV), IVSize)
	}
	if len(msg.Tag) != TagSize {
		return nil, fmt.Errorf("invalid tag size: %d (expected %d)", len(msg.Tag), TagSize)
	}

	// Create cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Combine ciphertext and tag
	ciphertext := append(msg.Ciphertext, msg.Tag...)

	// Decrypt and verify
	plaintext, err := aesGCM.Open(nil, msg.IV, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}

// DeriveSessionKey derives a session key from a shared secret using HKDF
func DeriveSessionKey(sharedSecret []byte, salt []byte, info []byte) ([]byte, error) {
	hkdf := hkdf.New(sha3.New256, sharedSecret, salt, info)
	sessionKey := make([]byte, KeySizeAES)
	if _, err := io.ReadFull(hkdf, sessionKey); err != nil {
		return nil, fmt.Errorf("failed to derive session key: %w", err)
	}
	return sessionKey, nil
}

// DeriveSessionKeyFromNonces derives a session key from shared secret and nonces
func DeriveSessionKeyFromNonces(sharedSecret []byte, nonceA, nonceB []byte) ([]byte, error) {
	// Combine nonces for info
	info := append([]byte("p2p-sync-session"), nonceA...)
	info = append(info, nonceB...)

	// Use combined nonces as salt
	salt := append(nonceA, nonceB...)

	return DeriveSessionKey(sharedSecret, salt, info)
}

// EncryptAESGCM encrypts data using AES-256-GCM
// Returns: ciphertext (with tag appended), nonce, error
func EncryptAESGCM(key, plaintext, associatedData []byte) ([]byte, []byte, error) {
	if len(key) != KeySizeAES {
		return nil, nil, fmt.Errorf("invalid key size: %d (expected %d)", len(key), KeySizeAES)
	}

	// Create cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate random nonce
	nonce := make([]byte, IVSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt and authenticate (ciphertext includes the tag)
	ciphertext := aesGCM.Seal(nil, nonce, plaintext, associatedData)

	return ciphertext, nonce, nil
}

// DecryptAESGCM decrypts data using AES-256-GCM
// Expects ciphertext with tag appended
func DecryptAESGCM(key, ciphertext, nonce []byte, associatedData []byte) ([]byte, error) {
	if len(key) != KeySizeAES {
		return nil, fmt.Errorf("invalid key size: %d (expected %d)", len(key), KeySizeAES)
	}

	if len(nonce) != IVSize {
		return nil, fmt.Errorf("invalid nonce size: %d (expected %d)", len(nonce), IVSize)
	}

	// Create cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Decrypt and verify (ciphertext includes the tag)
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, associatedData)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}
