package compression_test

import (
	"bytes"
	"testing"

	"github.com/p2p-folder-sync/p2p-sync/internal/compression"
)

func TestZstdCompression(t *testing.T) {
	compressor, err := compression.NewZstdCompressor(3)
	if err != nil {
		t.Fatalf("Failed to create zstd compressor: %v", err)
	}

	data := []byte("This is test data for compression. " + string(make([]byte, 1000)))

	compressed, err := compressor.Compress(data)
	if err != nil {
		t.Fatalf("Failed to compress data: %v", err)
	}

	if len(compressed) >= len(data) {
		t.Error("Compressed data should be smaller than original")
	}

	decompressed, err := compressor.Decompress(compressed)
	if err != nil {
		t.Fatalf("Failed to decompress data: %v", err)
	}

	if !bytes.Equal(data, decompressed) {
		t.Error("Decompressed data doesn't match original")
	}
}

func TestGzipCompression(t *testing.T) {
	compressor, err := compression.NewGzipCompressor(6)
	if err != nil {
		t.Fatalf("Failed to create gzip compressor: %v", err)
	}

	data := []byte("This is test data for gzip compression. " + string(make([]byte, 1000)))

	compressed, err := compressor.Compress(data)
	if err != nil {
		t.Fatalf("Failed to compress data: %v", err)
	}

	if len(compressed) >= len(data) {
		t.Error("Compressed data should be smaller than original")
	}

	decompressed, err := compressor.Decompress(compressed)
	if err != nil {
		t.Fatalf("Failed to decompress data: %v", err)
	}

	if !bytes.Equal(data, decompressed) {
		t.Error("Decompressed data doesn't match original")
	}
}
