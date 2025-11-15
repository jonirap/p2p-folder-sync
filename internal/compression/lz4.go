package compression

import (
	"fmt"

	"github.com/pierrec/lz4/v4"
)

// LZ4Compressor implements LZ4 compression
type LZ4Compressor struct {
	level int
}

// NewLZ4Compressor creates a new LZ4 compressor
func NewLZ4Compressor(level int) (*LZ4Compressor, error) {
	if level < 1 || level > 16 {
		return nil, fmt.Errorf("lz4 level must be between 1 and 16, got %d", level)
	}

	return &LZ4Compressor{
		level: level,
	}, nil
}

// Compress compresses data using LZ4
func (l *LZ4Compressor) Compress(data []byte) ([]byte, error) {
	// Estimate compressed size (LZ4 can expand data slightly)
	compressed := make([]byte, len(data)+len(data)/255+16)
	
	compressor := lz4.CompressorHC{Level: lz4.CompressionLevel(l.level)}
	n, err := compressor.CompressBlock(data, compressed)
	if err != nil {
		return nil, fmt.Errorf("failed to compress: %w", err)
	}

	return compressed[:n], nil
}

// Decompress decompresses data using LZ4
func (l *LZ4Compressor) Decompress(data []byte) ([]byte, error) {
	// We need to know the original size, but LZ4 doesn't store it
	// For now, we'll use a heuristic: try to decompress with increasing buffer sizes
	// In practice, the original size should be passed separately
	decompressed := make([]byte, len(data)*4) // Start with 4x size
	
	n, err := lz4.UncompressBlock(data, decompressed)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress: %w", err)
	}

	return decompressed[:n], nil
}

// DecompressWithSize decompresses data when the original size is known
func (l *LZ4Compressor) DecompressWithSize(data []byte, originalSize int) ([]byte, error) {
	decompressed := make([]byte, originalSize)
	n, err := lz4.UncompressBlock(data, decompressed)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress: %w", err)
	}
	if n != originalSize {
		return nil, fmt.Errorf("decompressed size %d does not match expected %d", n, originalSize)
	}
	return decompressed, nil
}

// Algorithm returns the algorithm name
func (l *LZ4Compressor) Algorithm() string {
	return "lz4"
}

