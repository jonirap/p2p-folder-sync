package chunking

import (
	"fmt"
	"io"

	"github.com/p2p-folder-sync/p2p-sync/internal/hashing"
)

// Chunk represents a file chunk
type Chunk struct {
	FileID      string
	ChunkID     int
	Offset      int64
	Length      int64
	Data        []byte
	Hash        string
	FileHash    string // Full file hash (for verification after assembly)
	IsLast      bool
	TotalChunks int
}

// Chunker splits files into chunks
type Chunker struct {
	chunkSize int64
}

// NewChunker creates a new chunker with the specified chunk size
func NewChunker(chunkSize int64) *Chunker {
	return &Chunker{
		chunkSize: chunkSize,
	}
}

// ChunkFile splits a file into chunks
func (c *Chunker) ChunkFile(fileID string, data []byte) ([]*Chunk, error) {
	if len(data) == 0 {
		// Empty file - return single empty chunk
		hash := hashing.HashString(data)
		return []*Chunk{
			{
				FileID:      fileID,
				ChunkID:     0,
				Offset:      0,
				Length:      0,
				Data:        []byte{},
				Hash:        hash,
				IsLast:      true,
				TotalChunks: 1,
			},
		}, nil
	}

	var chunks []*Chunk
	totalSize := int64(len(data))
	totalChunks := int((totalSize + c.chunkSize - 1) / c.chunkSize) // Ceiling division

	for i := 0; i < totalChunks; i++ {
		offset := int64(i) * c.chunkSize
		end := offset + c.chunkSize
		if end > totalSize {
			end = totalSize
		}

		chunkData := data[offset:end]
		hash := hashing.HashString(chunkData)

		chunk := &Chunk{
			FileID:      fileID,
			ChunkID:     i,
			Offset:      offset,
			Length:      int64(len(chunkData)),
			Data:        chunkData,
			Hash:        hash,
			IsLast:      i == totalChunks-1,
			TotalChunks: totalChunks,
		}

		chunks = append(chunks, chunk)
	}

	return chunks, nil
}

// ChunkReader splits data from a reader into chunks
func (c *Chunker) ChunkReader(fileID string, reader io.Reader) ([]*Chunk, error) {
	var chunks []*Chunk
	var offset int64
	chunkID := 0

	for {
		chunkData := make([]byte, c.chunkSize)
		n, err := reader.Read(chunkData)
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("failed to read chunk: %w", err)
		}

		if n == 0 {
			break
		}

		chunkData = chunkData[:n]
		hash := hashing.HashString(chunkData)

		isLast := err == io.EOF

		chunk := &Chunk{
			FileID:      fileID,
			ChunkID:     chunkID,
			Offset:      offset,
			Length:      int64(n),
			Data:        chunkData,
			Hash:        hash,
			IsLast:      isLast,
			TotalChunks: -1, // Unknown until we finish
		}

		chunks = append(chunks, chunk)
		offset += int64(n)
		chunkID++

		if isLast {
			break
		}
	}

	// Update TotalChunks for all chunks
	for _, chunk := range chunks {
		chunk.TotalChunks = len(chunks)
	}

	return chunks, nil
}

// CalculateChunkCount calculates the number of chunks for a given file size
func (c *Chunker) CalculateChunkCount(fileSize int64) int {
	if fileSize == 0 {
		return 1
	}
	return int((fileSize + c.chunkSize - 1) / c.chunkSize)
}

