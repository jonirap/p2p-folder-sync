package chunking_test

import (
	"testing"

	"github.com/p2p-folder-sync/p2p-sync/internal/chunking"
	"github.com/p2p-folder-sync/p2p-sync/internal/hashing"
)

func TestChunkManager_StartTransfer(t *testing.T) {
	cm := chunking.NewChunkManager()
	defer cm.Stop()

	fileID := "test-file-1"
	fileHash := "test-hash"
	totalChunks := 5
	expectedSize := int64(1000)

	err := cm.StartTransfer(fileID, fileHash, totalChunks, expectedSize)
	if err != nil {
		t.Fatalf("Failed to start transfer: %v", err)
	}

	// Try to start same transfer again - should fail
	err = cm.StartTransfer(fileID, fileHash, totalChunks, expectedSize)
	if err == nil {
		t.Fatal("Expected error when starting duplicate transfer")
	}

	t.Logf("SUCCESS: Transfer started correctly")
}

func TestChunkManager_ReceiveChunksInOrder(t *testing.T) {
	cm := chunking.NewChunkManager()
	defer cm.Stop()

	fileID := "test-file-2"
	fileData := []byte("Hello, World! This is test data.")
	fileHash := hashing.HashString(fileData)

	// Create chunks
	chunker := chunking.NewChunker(10) // 10-byte chunks
	chunks, err := chunker.ChunkFile(fileID, fileData)
	if err != nil {
		t.Fatalf("Failed to chunk file: %v", err)
	}

	// Add FileHash to chunks
	for _, chunk := range chunks {
		chunk.FileHash = fileHash
	}

	// Start transfer
	err = cm.StartTransfer(fileID, fileHash, len(chunks), int64(len(fileData)))
	if err != nil {
		t.Fatalf("Failed to start transfer: %v", err)
	}

	// Receive chunks in order
	for _, chunk := range chunks {
		err = cm.ReceiveChunk(chunk)
		if err != nil {
			t.Fatalf("Failed to receive chunk %d: %v", chunk.ChunkID, err)
		}
	}

	// Check if transfer is complete
	if !cm.IsTransferComplete(fileID) {
		t.Fatal("Transfer should be complete")
	}

	// Assemble file
	assembled, err := cm.AssembleFile(fileID)
	if err != nil {
		t.Fatalf("Failed to assemble file: %v", err)
	}

	if string(assembled) != string(fileData) {
		t.Fatalf("Assembled data doesn't match original.\nExpected: %s\nGot: %s", fileData, assembled)
	}

	t.Logf("SUCCESS: Chunks received in order and assembled correctly")
}

func TestChunkManager_ReceiveChunksOutOfOrder(t *testing.T) {
	cm := chunking.NewChunkManager()
	defer cm.Stop()

	fileID := "test-file-3"
	fileData := []byte("Out of order chunk test data here!")
	fileHash := hashing.HashString(fileData)

	// Create chunks
	chunker := chunking.NewChunker(8) // 8-byte chunks
	chunks, err := chunker.ChunkFile(fileID, fileData)
	if err != nil {
		t.Fatalf("Failed to chunk file: %v", err)
	}

	// Add FileHash to chunks
	for _, chunk := range chunks {
		chunk.FileHash = fileHash
	}

	// Start transfer
	err = cm.StartTransfer(fileID, fileHash, len(chunks), int64(len(fileData)))
	if err != nil {
		t.Fatalf("Failed to start transfer: %v", err)
	}

	// Receive chunks out of order: last, first, middle
	order := []int{len(chunks) - 1, 0}
	for i := 1; i < len(chunks)-1; i++ {
		order = append(order, i)
	}

	for _, idx := range order {
		err = cm.ReceiveChunk(chunks[idx])
		if err != nil {
			t.Fatalf("Failed to receive chunk %d: %v", idx, err)
		}
	}

	// Check if transfer is complete
	if !cm.IsTransferComplete(fileID) {
		t.Fatal("Transfer should be complete")
	}

	// Assemble file
	assembled, err := cm.AssembleFile(fileID)
	if err != nil {
		t.Fatalf("Failed to assemble file: %v", err)
	}

	if string(assembled) != string(fileData) {
		t.Fatalf("Assembled data doesn't match original")
	}

	t.Logf("SUCCESS: Out-of-order chunks assembled correctly")
}

func TestChunkManager_GetMissingChunks(t *testing.T) {
	cm := chunking.NewChunkManager()
	defer cm.Stop()

	fileID := "test-file-4"
	totalChunks := 5
	fileHash := "test-hash"

	err := cm.StartTransfer(fileID, fileHash, totalChunks, 1000)
	if err != nil {
		t.Fatalf("Failed to start transfer: %v", err)
	}

	// Receive only chunks 0, 2, 4
	receivedChunks := []int{0, 2, 4}
	for _, chunkID := range receivedChunks {
		chunk := &chunking.Chunk{
			FileID:      fileID,
			ChunkID:     chunkID,
			Offset:      int64(chunkID * 100),
			Length:      100,
			Data:        make([]byte, 100),
			Hash:        hashing.HashString(make([]byte, 100)),
			FileHash:    fileHash,
			IsLast:      chunkID == totalChunks-1,
			TotalChunks: totalChunks,
		}

		err = cm.ReceiveChunk(chunk)
		if err != nil {
			t.Fatalf("Failed to receive chunk %d: %v", chunkID, err)
		}
	}

	// Get missing chunks
	missing, err := cm.GetMissingChunks(fileID)
	if err != nil {
		t.Fatalf("Failed to get missing chunks: %v", err)
	}

	expectedMissing := []int{1, 3}
	if len(missing) != len(expectedMissing) {
		t.Fatalf("Expected %d missing chunks, got %d", len(expectedMissing), len(missing))
	}

	for i, chunkID := range missing {
		if chunkID != expectedMissing[i] {
			t.Errorf("Expected missing chunk %d, got %d", expectedMissing[i], chunkID)
		}
	}

	t.Logf("SUCCESS: Missing chunks identified correctly: %v", missing)
}

func TestChunkManager_TransferStatus(t *testing.T) {
	cm := chunking.NewChunkManager()
	defer cm.Stop()

	fileID := "test-file-5"
	totalChunks := 3
	fileHash := "test-hash"

	err := cm.StartTransfer(fileID, fileHash, totalChunks, 300)
	if err != nil {
		t.Fatalf("Failed to start transfer: %v", err)
	}

	// Initially: 0 received, not complete
	received, total, complete, err := cm.GetTransferStatus(fileID)
	if err != nil {
		t.Fatalf("Failed to get status: %v", err)
	}

	if received != 0 || total != totalChunks || complete {
		t.Fatalf("Initial status incorrect: received=%d, total=%d, complete=%v", received, total, complete)
	}

	// Receive first chunk
	chunk := &chunking.Chunk{
		FileID:      fileID,
		ChunkID:     0,
		Offset:      0,
		Length:      100,
		Data:        make([]byte, 100),
		Hash:        hashing.HashString(make([]byte, 100)),
		FileHash:    fileHash,
		IsLast:      false,
		TotalChunks: totalChunks,
	}

	err = cm.ReceiveChunk(chunk)
	if err != nil {
		t.Fatalf("Failed to receive chunk: %v", err)
	}

	// Status: 1 received, not complete
	received, total, complete, err = cm.GetTransferStatus(fileID)
	if err != nil {
		t.Fatalf("Failed to get status: %v", err)
	}

	if received != 1 || total != totalChunks || complete {
		t.Fatalf("After one chunk: received=%d, total=%d, complete=%v", received, total, complete)
	}

	t.Logf("SUCCESS: Transfer status tracking works correctly")
}

func TestChunkManager_RetransmissionCount(t *testing.T) {
	cm := chunking.NewChunkManager()
	defer cm.Stop()

	fileID := "test-file-6"
	fileHash := "test-hash"

	err := cm.StartTransfer(fileID, fileHash, 5, 500)
	if err != nil {
		t.Fatalf("Failed to start transfer: %v", err)
	}

	// Initially 0 retransmissions
	count := cm.GetRetransmissionCount(fileID)
	if count != 0 {
		t.Fatalf("Expected 0 retransmissions, got %d", count)
	}

	// Increment retransmission count
	err = cm.IncrementRetransmissionCount(fileID)
	if err != nil {
		t.Fatalf("Failed to increment retransmission count: %v", err)
	}

	count = cm.GetRetransmissionCount(fileID)
	if count != 1 {
		t.Fatalf("Expected 1 retransmission, got %d", count)
	}

	// Increment again
	err = cm.IncrementRetransmissionCount(fileID)
	if err != nil {
		t.Fatalf("Failed to increment retransmission count: %v", err)
	}

	count = cm.GetRetransmissionCount(fileID)
	if count != 2 {
		t.Fatalf("Expected 2 retransmissions, got %d", count)
	}

	t.Logf("SUCCESS: Retransmission count tracking works correctly")
}

func TestChunkManager_Cleanup(t *testing.T) {
	cm := chunking.NewChunkManager()
	defer cm.Stop()

	fileID := "test-file-7"
	fileHash := "test-hash"

	err := cm.StartTransfer(fileID, fileHash, 3, 300)
	if err != nil {
		t.Fatalf("Failed to start transfer: %v", err)
	}

	// Transfer exists
	_, _, _, err = cm.GetTransferStatus(fileID)
	if err != nil {
		t.Fatalf("Transfer should exist: %v", err)
	}

	// Cleanup transfer
	cm.CleanupTransfer(fileID)

	// Transfer should not exist anymore
	_, _, _, err = cm.GetTransferStatus(fileID)
	if err == nil {
		t.Fatal("Transfer should not exist after cleanup")
	}

	t.Logf("SUCCESS: Transfer cleanup works correctly")
}
