package compression

import (
	"fmt"
)

// NewCompressor creates a compressor based on algorithm name
func NewCompressor(algorithm string, level int) (Compressor, error) {
	switch algorithm {
	case "zstd":
		return NewZstdCompressor(level)
	case "lz4":
		return NewLZ4Compressor(level)
	case "gzip":
		return NewGzipCompressor(level)
	case "none":
		return NewNoOpCompressor(), nil
	default:
		return nil, fmt.Errorf("unknown compression algorithm: %s", algorithm)
	}
}

// NoOpCompressor is a compressor that doesn't compress (pass-through)
type NoOpCompressor struct{}

// NewNoOpCompressor creates a no-op compressor
func NewNoOpCompressor() *NoOpCompressor {
	return &NoOpCompressor{}
}

// Compress returns data as-is
func (n *NoOpCompressor) Compress(data []byte) ([]byte, error) {
	return data, nil
}

// Decompress returns data as-is
func (n *NoOpCompressor) Decompress(data []byte) ([]byte, error) {
	return data, nil
}

// Algorithm returns "none"
func (n *NoOpCompressor) Algorithm() string {
	return "none"
}

