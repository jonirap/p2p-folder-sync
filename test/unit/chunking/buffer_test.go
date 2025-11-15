package chunking_test

import (
	"reflect"
	"testing"

	"github.com/p2p-folder-sync/p2p-sync/internal/chunking"
)

func TestChunkBuffer(t *testing.T) {
	fileID := "test-file"
	totalChunks := 3
	buffer := chunking.NewChunkBuffer(fileID, totalChunks)

	// Add chunks out of order
	chunk2 := &chunking.Chunk{
		FileID:      fileID,
		ChunkID:     2,
		Offset:      2048,
		Length:      500,
		Data:        make([]byte, 500),
		Hash:        "hash2",
		IsLast:      true,
		TotalChunks: totalChunks,
	}
	chunk0 := &chunking.Chunk{
		FileID:      fileID,
		ChunkID:     0,
		Offset:      0,
		Length:      1024,
		Data:        make([]byte, 1024),
		Hash:        "hash0",
		IsLast:      false,
		TotalChunks: totalChunks,
	}
	chunk1 := &chunking.Chunk{
		FileID:      fileID,
		ChunkID:     1,
		Offset:      1024,
		Length:      1024,
		Data:        make([]byte, 1024),
		Hash:        "hash1",
		IsLast:      false,
		TotalChunks: totalChunks,
	}
	// Add out of order
	if err := buffer.AddChunk(chunk2); err != nil {
		t.Fatalf("Failed to add chunk 2: %v", err)
	}
	if buffer.IsComplete() {
		t.Error("Buffer should not be complete yet")
	}
	if err := buffer.AddChunk(chunk0); err != nil {
		t.Fatalf("Failed to add chunk 0: %v", err)
	}
	if err := buffer.AddChunk(chunk1); err != nil {
		t.Fatalf("Failed to add chunk 1: %v", err)
	}
	if !buffer.IsComplete() {
		t.Error("Buffer should be complete after adding all chunks")
	}
	chunks, err := buffer.GetChunks()
	if err != nil {
		t.Fatalf("Failed to get chunks: %v", err)
	}
	if len(chunks) != totalChunks {
		t.Errorf("Expected %d chunks, got %d", totalChunks, len(chunks))
	}
	// Verify order
	for i, chunk := range chunks {
		if chunk.ChunkID != i {
			t.Errorf("Expected chunk ID %d, got %d", i, chunk.ChunkID)
		}
	}

	// Verify chunk data integrity after reordering
	expectedChunks := []*chunking.Chunk{chunk0, chunk1, chunk2}
	for i, chunk := range chunks {
		expected := expectedChunks[i]
		if chunk.FileID != expected.FileID {
			t.Errorf("Chunk %d FileID mismatch: expected %s, got %s", i, expected.FileID, chunk.FileID)
		}
		if chunk.ChunkID != expected.ChunkID {
			t.Errorf("Chunk %d ChunkID mismatch: expected %d, got %d", i, expected.ChunkID, chunk.ChunkID)
		}
		if chunk.Offset != expected.Offset {
			t.Errorf("Chunk %d Offset mismatch: expected %d, got %d", i, expected.Offset, chunk.Offset)
		}
		if chunk.Length != expected.Length {
			t.Errorf("Chunk %d Length mismatch: expected %d, got %d", i, expected.Length, chunk.Length)
		}
		if len(chunk.Data) != len(expected.Data) {
			t.Errorf("Chunk %d Data length mismatch: expected %d, got %d", i, len(expected.Data), len(chunk.Data))
		}
		if chunk.Hash != expected.Hash {
			t.Errorf("Chunk %d Hash mismatch: expected %s, got %s", i, expected.Hash, chunk.Hash)
		}
		if chunk.IsLast != expected.IsLast {
			t.Errorf("Chunk %d IsLast mismatch: expected %t, got %t", i, expected.IsLast, chunk.IsLast)
		}
		if chunk.TotalChunks != expected.TotalChunks {
			t.Errorf("Chunk %d TotalChunks mismatch: expected %d, got %d", i, expected.TotalChunks, chunk.TotalChunks)
		}
	}
}

func TestChunkBufferMissingChunks(t *testing.T) {
	fileID := "test-file"
	totalChunks := 5
	buffer := chunking.NewChunkBuffer(fileID, totalChunks)

	// Create some chunks
	chunk0 := &chunking.Chunk{
		FileID:      fileID,
		ChunkID:     0,
		Offset:      0,
		Length:      1024,
		Data:        make([]byte, 1024),
		Hash:        "hash0",
		IsLast:      false,
		TotalChunks: totalChunks,
	}
	chunk2 := &chunking.Chunk{
		FileID:      fileID,
		ChunkID:     2,
		Offset:      2048,
		Length:      1024,
		Data:        make([]byte, 1024),
		Hash:        "hash2",
		IsLast:      false,
		TotalChunks: totalChunks,
	}

	// Add only some chunks
	buffer.AddChunk(chunk0)
	buffer.AddChunk(chunk2)
	missing := buffer.GetMissingChunks()
	expectedMissing := []int{1, 3, 4}
	if len(missing) != len(expectedMissing) {
		t.Errorf("Expected %d missing chunks, got %d", len(expectedMissing), len(missing))
	}
	if !reflect.DeepEqual(missing, expectedMissing) {
		t.Errorf("Expected missing chunks %v, got %v", expectedMissing, missing)
	}
}
