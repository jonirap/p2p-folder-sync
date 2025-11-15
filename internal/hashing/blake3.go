package hashing

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/zeebo/blake3"
)

// Hash computes a BLAKE3 hash of the input data
func Hash(data []byte) []byte {
	hasher := blake3.New()
	hasher.Write(data)
	return hasher.Sum(nil)
}

// HashString computes a BLAKE3 hash and returns it as a hex string
func HashString(data []byte) string {
	return hex.EncodeToString(Hash(data))
}

// HashReader computes a BLAKE3 hash of data read from an io.Reader
func HashReader(reader io.Reader) ([]byte, error) {
	if reader == nil {
		return nil, fmt.Errorf("reader cannot be nil")
	}
	hasher := blake3.New()
	if _, err := io.Copy(hasher, reader); err != nil {
		return nil, err
	}
	return hasher.Sum(nil), nil
}

// HashReaderString computes a BLAKE3 hash from a reader and returns it as a hex string
func HashReaderString(reader io.Reader) (string, error) {
	hash, err := HashReader(reader)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash), nil
}

// NewHasher creates a new BLAKE3 hasher for streaming
func NewHasher() *blake3.Hasher {
	return blake3.New()
}

// GenerateRandomHash generates a random hash (useful for testing)
func GenerateRandomHash() string {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		panic(err)
	}
	return hex.EncodeToString(data)
}


