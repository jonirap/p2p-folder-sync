package chunking

import (
	"fmt"
	"sort"

	"github.com/p2p-folder-sync/p2p-sync/internal/hashing"
)

// Assembler assembles chunks into a complete file
type Assembler struct{}

// NewAssembler creates a new assembler
func NewAssembler() *Assembler {
	return &Assembler{}
}

// AssembleChunks assembles chunks into a complete file and verifies the hash
// Chunks can arrive in any order and will be sorted by ChunkID for assembly
func (a *Assembler) AssembleChunks(chunks []*Chunk, expectedFileHash string) ([]byte, error) {
	if len(chunks) == 0 {
		return nil, fmt.Errorf("no chunks to assemble")
	}

	// Sort chunks by ChunkID to handle out-of-order arrival
	sortedChunks := make([]*Chunk, len(chunks))
	copy(sortedChunks, chunks)

	// Sort by ChunkID
	sort.Slice(sortedChunks, func(i, j int) bool {
		return sortedChunks[i].ChunkID < sortedChunks[j].ChunkID
	})

	// Calculate total size
	var totalSize int64
	for _, chunk := range sortedChunks {
		totalSize += chunk.Length
	}

	// Assemble data
	assembled := make([]byte, totalSize)
	var offset int64

	for _, chunk := range sortedChunks {
		if chunk.Offset != offset {
			return nil, fmt.Errorf("chunk offset mismatch: expected %d, got %d", offset, chunk.Offset)
		}

		copy(assembled[offset:], chunk.Data)
		offset += chunk.Length
	}

	// Verify file hash
	actualHash := hashing.HashString(assembled)
	if actualHash != expectedFileHash {
		return nil, fmt.Errorf("file hash mismatch: expected %s, got %s", expectedFileHash, actualHash)
	}

	return assembled, nil
}

// VerifyChunkHash verifies a chunk's hash
func (a *Assembler) VerifyChunkHash(chunk *Chunk) error {
	expectedHash := chunk.Hash
	actualHash := hashing.HashString(chunk.Data)
	if actualHash != expectedHash {
		return fmt.Errorf("chunk %d hash mismatch: expected %s, got %s", chunk.ChunkID, expectedHash, actualHash)
	}
	return nil
}

// VerifyAllChunks verifies all chunks' hashes
func (a *Assembler) VerifyAllChunks(chunks []*Chunk) error {
	for _, chunk := range chunks {
		if err := a.VerifyChunkHash(chunk); err != nil {
			return err
		}
	}
	return nil
}

