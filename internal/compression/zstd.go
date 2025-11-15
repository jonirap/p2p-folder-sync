package compression

import (
	"fmt"

	"github.com/klauspost/compress/zstd"
)

// ZstdCompressor implements Zstandard compression
type ZstdCompressor struct {
	level int
	enc   *zstd.Encoder
	dec   *zstd.Decoder
}

// NewZstdCompressor creates a new Zstandard compressor
func NewZstdCompressor(level int) (*ZstdCompressor, error) {
	if level < 1 || level > 22 {
		return nil, fmt.Errorf("zstd level must be between 1 and 22, got %d", level)
	}

	// Create encoder with specified level
	opts := []zstd.EOption{
		zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(level)),
	}
	enc, err := zstd.NewWriter(nil, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create zstd encoder: %w", err)
	}

	// Create decoder
	dec, err := zstd.NewReader(nil)
	if err != nil {
		enc.Close()
		return nil, fmt.Errorf("failed to create zstd decoder: %w", err)
	}

	return &ZstdCompressor{
		level: level,
		enc:   enc,
		dec:   dec,
	}, nil
}

// Compress compresses data using Zstandard
func (z *ZstdCompressor) Compress(data []byte) ([]byte, error) {
	return z.enc.EncodeAll(data, nil), nil
}

// Decompress decompresses data using Zstandard
func (z *ZstdCompressor) Decompress(data []byte) ([]byte, error) {
	return z.dec.DecodeAll(data, nil)
}

// Algorithm returns the algorithm name
func (z *ZstdCompressor) Algorithm() string {
	return "zstd"
}

// Close closes the compressor and releases resources
func (z *ZstdCompressor) Close() error {
	if z.enc != nil {
		z.enc.Close()
	}
	if z.dec != nil {
		z.dec.Close()
	}
	return nil
}

