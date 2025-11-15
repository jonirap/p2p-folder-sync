package crypto_test

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/p2p-folder-sync/p2p-sync/internal/crypto"
)

func TestEncryptDecrypt(t *testing.T) {
	// Generate a random key
	key := make([]byte, crypto.KeySizeAES)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	plaintext := []byte("Hello, World! This is a test message for encryption.")

	// Encrypt
	encrypted, err := crypto.Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Failed to encrypt: %v", err)
	}

	// Verify encrypted message structure
	if len(encrypted.IV) != crypto.IVSize {
		t.Errorf("Expected IV size %d, got %d", crypto.IVSize, len(encrypted.IV))
	}
	if len(encrypted.Tag) != crypto.TagSize {
		t.Errorf("Expected tag size %d, got %d", crypto.TagSize, len(encrypted.Tag))
	}
	if len(encrypted.Ciphertext) == 0 {
		t.Error("Expected non-empty ciphertext")
	}

	// Decrypt
	decrypted, err := crypto.Decrypt(encrypted, key)
	if err != nil {
		t.Fatalf("Failed to decrypt: %v", err)
	}

	// Verify plaintext matches
	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("Decrypted text doesn't match original: got %q, want %q", decrypted, plaintext)
	}
}

func TestDeriveSessionKey(t *testing.T) {
	sharedSecret := []byte("test-shared-secret")
	salt := []byte("test-salt")
	info := []byte("test-info")

	key, err := crypto.DeriveSessionKey(sharedSecret, salt, info)
	if err != nil {
		t.Fatalf("Failed to derive session key: %v", err)
	}

	if len(key) != crypto.KeySizeAES {
		t.Errorf("Expected key size %d, got %d", crypto.KeySizeAES, len(key))
	}

	// Verify deterministic output
	key2, err := crypto.DeriveSessionKey(sharedSecret, salt, info)
	if err != nil {
		t.Fatalf("Failed to derive session key second time: %v", err)
	}

	if !bytes.Equal(key, key2) {
		t.Error("Session key derivation should be deterministic")
	}
}

func TestGenerateKeyPair(t *testing.T) {
	keyPair, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	if len(keyPair.PrivateKey) != crypto.KeySize {
		t.Errorf("Expected private key size %d, got %d", crypto.KeySize, len(keyPair.PrivateKey))
	}
	if len(keyPair.PublicKey) != crypto.KeySize {
		t.Errorf("Expected public key size %d, got %d", crypto.KeySize, len(keyPair.PublicKey))
	}

	// Verify keys are different
	if bytes.Equal(keyPair.PrivateKey, keyPair.PublicKey) {
		t.Error("Private and public keys should be different")
	}
}
