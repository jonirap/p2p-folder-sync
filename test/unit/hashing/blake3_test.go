package hashing_test

import (
	"bytes"
	"encoding/hex"
	"io"
	"strings"
	"testing"

	"github.com/p2p-folder-sync/p2p-sync/internal/hashing"
)

func TestHash(t *testing.T) {
	data := []byte("test data")
	hash := hashing.Hash(data)

	if len(hash) != 32 {
		t.Errorf("Expected hash length 32, got %d", len(hash))
	}
}

func TestHashString(t *testing.T) {
	data := []byte("test data")
	hashStr := hashing.HashString(data)
	if len(hashStr) != 64 { // 32 bytes = 64 hex characters
		t.Errorf("Expected hash string length 64, got %d", len(hashStr))
	}
}

func TestHashConsistency(t *testing.T) {
	data := []byte("test data")
	hash1 := hashing.HashString(data)
	hash2 := hashing.HashString(data)
	if hash1 != hash2 {
		t.Errorf("Hash should be consistent, got %s and %s", hash1, hash2)
	}
}

// TestHash_KnownTestVectors tests against official BLAKE3 test vectors
func TestHash_KnownTestVectors(t *testing.T) {
	testCases := []struct {
		name     string
		input    []byte
		expected string // BLAKE3 hash in hex (verified against implementation)
	}{
		{
			name:     "empty_string",
			input:    []byte(""),
			expected: "af1349b9f5f9a1a6a0404dea36dcc9499bcb25c9adc112b7cc9a93cae41f3262",
		},
		{
			name:     "hello_world",
			input:    []byte("hello world"),
			expected: "d74981efa70a0c880b8d8c1985d075dbcbf679b99a5f9914e5aaf96b831a9e24",
		},
		{
			name:     "single_a",
			input:    []byte("a"),
			expected: "17762fddd969a453925d65717ac3eea21320b66b54342fde15128d6caf21215f",
		},
		{
			name:     "abc",
			input:    []byte("abc"),
			expected: "6437b3ac38465133ffb63b75273a8db548c558465d79db03fd359c6cd5bd9d85",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := hashing.HashString(tc.input)
			if result != tc.expected {
				t.Errorf("Hash mismatch for %s: expected %s, got %s", tc.name, tc.expected, result)
			}
		})
	}
}

// TestHash_IncrementalHashing tests HashReader for streaming/large files
func TestHash_IncrementalHashing(t *testing.T) {
	// Test with large data (>1MB) to verify streaming works
	dataSize := 2 * 1024 * 1024 // 2MB
	largeData := make([]byte, dataSize)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	// Test HashReader vs direct Hash
	reader := bytes.NewReader(largeData)
	readerHash, err := hashing.HashReaderString(reader)
	if err != nil {
		t.Fatalf("HashReader failed: %v", err)
	}

	directHash := hashing.HashString(largeData)

	if readerHash != directHash {
		t.Errorf("HashReader and direct hash should match: reader=%s, direct=%s", readerHash, directHash)
	}

	// Test with smaller chunks to verify incremental hashing
	reader2 := bytes.NewReader(largeData)
	chunkSize := 64 * 1024 // 64KB chunks
	hasher := hashing.NewHasher()

	buffer := make([]byte, chunkSize)
	for {
		n, err := reader2.Read(buffer)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}

		hasher.Write(buffer[:n])
	}

	incrementalResult := hasher.Sum(nil)
	incrementalHex := hex.EncodeToString(incrementalResult)

	if incrementalHex != directHash {
		t.Errorf("Incremental hash should match direct hash: incremental=%s, direct=%s", incrementalHex, directHash)
	}
}

// TestHash_ErrorHandling tests error cases for hash functions
func TestHash_ErrorHandling(t *testing.T) {
	// Test HashReader with nil reader
	_, err := hashing.HashReaderString(nil)
	if err == nil {
		t.Error("Expected error for nil reader")
	}

	// Test HashReader with reader that returns error
	errorReader := &errorReader{}
	_, err = hashing.HashReaderString(errorReader)
	if err == nil {
		t.Error("Expected error for reader that returns error")
	}
}

// TestHash_ConcurrentHashing verifies multiple hashers can operate concurrently
func TestHash_ConcurrentHashing(t *testing.T) {
	const numGoroutines = 10
	const numHashesPerGoroutine = 100

	results := make(chan []string, numGoroutines)

	// Launch multiple goroutines computing hashes concurrently
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			localResults := make([]string, numHashesPerGoroutine)

			for j := 0; j < numHashesPerGoroutine; j++ {
				// Use the same data pattern for all goroutines to test concurrent access
				data := []byte("concurrent test data " + string(rune('0'+j)) + " iteration " + string(rune('0'+(j%10))))
				localResults[j] = hashing.HashString(data)
			}

			results <- localResults
		}(i)
	}

	// Collect results
	allResults := make([][]string, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		allResults[i] = <-results
	}

	// Verify all results are consistent (same input should produce same hash)
	for i := 0; i < numGoroutines; i++ {
		for j := 0; j < numHashesPerGoroutine; j++ {
			expected := allResults[0][j] // Use first goroutine's results as reference

			if allResults[i][j] != expected {
				data := []byte("concurrent test data " + string(rune('0'+j)) + " iteration " + string(rune('0'+(j%10))))
				t.Errorf("Hash inconsistency for data %q: expected %s, got %s", data, expected, allResults[i][j])
			}
		}
	}
}

// TestHashString_Format verifies hex encoding format (64 chars, lowercase)
func TestHashString_Format(t *testing.T) {
	testCases := []struct {
		name string
		data []byte
	}{
		{"empty", []byte("")},
		{"single_byte", []byte("x")},
		{"short_string", []byte("hello")},
		{"longer_string", []byte("This is a longer string for testing hex encoding format")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := hashing.HashString(tc.data)

			// Verify length (32 bytes = 64 hex chars)
			if len(result) != 64 {
				t.Errorf("Expected hash string length 64, got %d", len(result))
			}

			// Verify all characters are valid hex (0-9, a-f)
			for i, char := range result {
				if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
					t.Errorf("Invalid hex character at position %d: %c", i, char)
				}
			}

			// Verify it's lowercase (not uppercase)
			if strings.ToLower(result) != result {
				t.Error("Hash string should be lowercase")
			}
		})
	}
}

// Helper functions

// errorReader is a reader that always returns an error
type errorReader struct{}

func (er *errorReader) Read(p []byte) (n int, err error) {
	return 0, io.ErrUnexpectedEOF
}
