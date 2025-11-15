package compression

// Compressor defines the interface for compression algorithms
type Compressor interface {
	// Compress compresses data and returns compressed data
	Compress(data []byte) ([]byte, error)

	// Decompress decompresses data and returns original data
	Decompress(data []byte) ([]byte, error)

	// Algorithm returns the algorithm name
	Algorithm() string
}

// CompressionResult represents the result of compression
type CompressionResult struct {
	Data              []byte
	OriginalSize      int64
	CompressedSize    int64
	CompressionRatio  float64
	Algorithm         string
}

// CalculateCompressionRatio calculates the compression ratio
func CalculateCompressionRatio(originalSize, compressedSize int64) float64 {
	if originalSize == 0 {
		return 0.0
	}
	return float64(compressedSize) / float64(originalSize)
}

