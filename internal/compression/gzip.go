package compression

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
)

// GzipCompressor implements gzip compression
type GzipCompressor struct {
	level int
}

// NewGzipCompressor creates a new gzip compressor
func NewGzipCompressor(level int) (*GzipCompressor, error) {
	if level < 1 || level > 9 {
		return nil, fmt.Errorf("gzip level must be between 1 and 9, got %d", level)
	}

	return &GzipCompressor{
		level: level,
	}, nil
}

// Compress compresses data using gzip
func (g *GzipCompressor) Compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer, err := gzip.NewWriterLevel(&buf, g.level)
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip writer: %w", err)
	}

	if _, err := writer.Write(data); err != nil {
		writer.Close()
		return nil, fmt.Errorf("failed to write data: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close gzip writer: %w", err)
	}

	return buf.Bytes(), nil
}

// Decompress decompresses data using gzip
func (g *GzipCompressor) Decompress(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress: %w", err)
	}

	return decompressed, nil
}

// Algorithm returns the algorithm name
func (g *GzipCompressor) Algorithm() string {
	return "gzip"
}

