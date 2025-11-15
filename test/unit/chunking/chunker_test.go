package chunking_test

import (
	"bytes"
	"testing"

	"github.com/p2p-folder-sync/p2p-sync/internal/chunking"
)

func TestChunkFile(t *testing.T) {
	chunker := chunking.NewChunker(1024) // 1KB chunks
	fileID := "test-file-id"
	data := make([]byte, 2500) // 2.5KB of data

	chunks, err := chunker.ChunkFile(fileID, data)
	if err != nil {
		t.Fatalf("Failed to chunk file: %v", err)
	}
	expectedChunks := 3 // 2500 bytes / 1024 = 3 chunks
	if len(chunks) != expectedChunks {
		t.Errorf("Expected %d chunks, got %d", expectedChunks, len(chunks))
	}
	// Verify first chunk
	if chunks[0].ChunkID != 0 {
		t.Errorf("Expected chunk ID 0, got %d", chunks[0].ChunkID)
	}
	if chunks[0].Offset != 0 {
		t.Errorf("Expected offset 0, got %d", chunks[0].Offset)
	}
	if chunks[0].Length != 1024 {
		t.Errorf("Expected length 1024, got %d", chunks[0].Length)
	}
	// Verify last chunk
	lastChunk := chunks[len(chunks)-1]
	if !lastChunk.IsLast {
		t.Error("Last chunk should be marked as last")
	}
	if lastChunk.TotalChunks != expectedChunks {
		t.Errorf("Expected total chunks %d, got %d", expectedChunks, lastChunk.TotalChunks)
	}

	// Verify chunk data integrity
	for i, chunk := range chunks {
		expectedOffset := int64(i) * 1024
		expectedEnd := expectedOffset + 1024
		if expectedEnd > int64(len(data)) {
			expectedEnd = int64(len(data))
		}
		expectedLength := expectedEnd - expectedOffset

		// Check chunk data matches original data range
		expectedData := data[expectedOffset:expectedEnd]
		if !bytes.Equal(chunk.Data, expectedData) {
			t.Errorf("Chunk %d data doesn't match expected range [%d:%d]", i, expectedOffset, expectedEnd)
		}

		// Verify length matches data length
		if chunk.Length != expectedLength {
			t.Errorf("Chunk %d length mismatch: expected %d, got %d", i, expectedLength, chunk.Length)
		}

		// Verify offset
		if chunk.Offset != expectedOffset {
			t.Errorf("Chunk %d offset mismatch: expected %d, got %d", i, expectedOffset, chunk.Offset)
		}
	}
}

func TestChunkEmptyFile(t *testing.T) {
	chunker := chunking.NewChunker(1024)
	fileID := "empty-file-id"
	data := []byte{}

	chunks, err := chunker.ChunkFile(fileID, data)
	if err != nil {
		t.Fatalf("Failed to chunk empty file: %v", err)
	}
	if len(chunks) != 1 {
		t.Errorf("Expected 1 chunk for empty file, got %d", len(chunks))
	}
	if chunks[0].Length != 0 {
		t.Errorf("Expected chunk length 0, got %d", chunks[0].Length)
	}
	if !chunks[0].IsLast {
		t.Error("Empty file chunk should be marked as last")
	}

	// Verify chunk data is actually empty
	if len(chunks[0].Data) != 0 {
		t.Errorf("Empty file chunk data should be empty, got length %d", len(chunks[0].Data))
	}
	if chunks[0].Length != 0 {
		t.Errorf("Empty file chunk length should be 0, got %d", chunks[0].Length)
	}
}

func TestChunkFileReconstruction(t *testing.T) {
	chunker := chunking.NewChunker(1024) // 1KB chunks
	fileID := "reconstruction-test"
	data := make([]byte, 3500) // 3.5KB of data

	// Fill data with a pattern for verification
	for i := range data {
		data[i] = byte(i % 256)
	}

	chunks, err := chunker.ChunkFile(fileID, data)
	if err != nil {
		t.Fatalf("Failed to chunk file: %v", err)
	}

	// Reconstruct the original file from chunks
	reconstructed := make([]byte, 0, len(data))
	for _, chunk := range chunks {
		reconstructed = append(reconstructed, chunk.Data...)
	}

	// Verify reconstruction matches original data
	if !bytes.Equal(data, reconstructed) {
		t.Error("Reconstructed data doesn't match original file")
	}
}
