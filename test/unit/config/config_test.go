package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/p2p-folder-sync/p2p-sync/internal/config"
)

func TestLoadConfig(t *testing.T) {
	// Test loading default config
	cfg, err := config.LoadConfig("")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg == nil {
		t.Error("Expected config to be loaded")
	}

	// Test loading config from YAML file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	// Use a path within the temp directory that can be created
	customPath := filepath.Join(tmpDir, "custom", "test", "path")

	yamlContent := `
sync:
  folder_path: "` + customPath + `"
  chunk_size_default: 1048576
  max_concurrent_transfers: 10

network:
  port: 9000
  protocol: "tcp"

compression:
  enabled: false
  algorithm: "gzip"
  level: 5
`

	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	loadedCfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config from file: %v", err)
	}

	// Verify loaded values
	if loadedCfg.Sync.FolderPath != customPath {
		t.Errorf("Expected folder path '%s', got %s", customPath, loadedCfg.Sync.FolderPath)
	}
	if loadedCfg.Sync.ChunkSizeDefault != 1048576 {
		t.Errorf("Expected chunk size 1048576, got %d", loadedCfg.Sync.ChunkSizeDefault)
	}
	if loadedCfg.Sync.MaxConcurrentTransfers != 10 {
		t.Errorf("Expected max concurrent transfers 10, got %d", loadedCfg.Sync.MaxConcurrentTransfers)
	}
	if loadedCfg.Network.Port != 9000 {
		t.Errorf("Expected port 9000, got %d", loadedCfg.Network.Port)
	}
	if loadedCfg.Network.Protocol != "tcp" {
		t.Errorf("Expected protocol 'tcp', got %s", loadedCfg.Network.Protocol)
	}
	if loadedCfg.Compression.Enabled != false {
		t.Error("Expected compression disabled")
	}
	if loadedCfg.Compression.Algorithm != "gzip" {
		t.Errorf("Expected algorithm 'gzip', got %s", loadedCfg.Compression.Algorithm)
	}
	if loadedCfg.Compression.Level != 5 {
		t.Errorf("Expected compression level 5, got %d", loadedCfg.Compression.Level)
	}
}

func TestConfigValidate(t *testing.T) {
	// Test valid config passes validation
	validCfg := config.DefaultConfig()
	validCfg.Sync.FolderPath = t.TempDir() // Use temp dir that exists

	if err := validCfg.Validate(); err != nil {
		t.Errorf("Valid config should pass validation, got error: %v", err)
	}

	// Test invalid folder path (empty)
	invalidCfg1 := config.DefaultConfig()
	invalidCfg1.Sync.FolderPath = ""
	if err := invalidCfg1.Validate(); err == nil {
		t.Error("Expected error for empty folder path")
	}

	// Test invalid folder path (non-existent and can't create)
	invalidCfg2 := config.DefaultConfig()
	invalidCfg2.Sync.FolderPath = "/nonexistent/deep/path/that/cannot/be/created"
	if err := invalidCfg2.Validate(); err == nil {
		t.Error("Expected error for invalid folder path")
	}

	// Test invalid chunk size ranges
	invalidCfg3 := config.DefaultConfig()
	invalidCfg3.Sync.FolderPath = t.TempDir()
	invalidCfg3.Sync.ChunkSizeMin = 100 // Too small
	if err := invalidCfg3.Validate(); err == nil {
		t.Error("Expected error for chunk_size_min too small")
	}

	invalidCfg4 := config.DefaultConfig()
	invalidCfg4.Sync.FolderPath = t.TempDir()
	invalidCfg4.Sync.ChunkSizeMax = 20000000 // Too large
	if err := invalidCfg4.Validate(); err == nil {
		t.Error("Expected error for chunk_size_max too large")
	}

	invalidCfg5 := config.DefaultConfig()
	invalidCfg5.Sync.FolderPath = t.TempDir()
	invalidCfg5.Sync.ChunkSizeDefault = 100 // Smaller than min
	if err := invalidCfg5.Validate(); err == nil {
		t.Error("Expected error for chunk_size_default smaller than min")
	}

	// Test invalid network port ranges
	invalidCfg6 := config.DefaultConfig()
	invalidCfg6.Sync.FolderPath = t.TempDir()
	invalidCfg6.Network.Port = 80 // Too low
	if err := invalidCfg6.Validate(); err == nil {
		t.Error("Expected error for port too low")
	}

	invalidCfg7 := config.DefaultConfig()
	invalidCfg7.Sync.FolderPath = t.TempDir()
	invalidCfg7.Network.Port = 70000 // Too high
	if err := invalidCfg7.Validate(); err == nil {
		t.Error("Expected error for port too high")
	}

	// Test invalid protocol
	invalidCfg8 := config.DefaultConfig()
	invalidCfg8.Sync.FolderPath = t.TempDir()
	invalidCfg8.Network.Protocol = "invalid"
	if err := invalidCfg8.Validate(); err == nil {
		t.Error("Expected error for invalid protocol")
	}

	// Test invalid compression levels
	invalidCfg9 := config.DefaultConfig()
	invalidCfg9.Sync.FolderPath = t.TempDir()
	invalidCfg9.Compression.Enabled = true
	invalidCfg9.Compression.Algorithm = "zstd"
	invalidCfg9.Compression.Level = 50 // Too high for zstd
	if err := invalidCfg9.Validate(); err == nil {
		t.Error("Expected error for invalid zstd compression level")
	}

	invalidCfg10 := config.DefaultConfig()
	invalidCfg10.Sync.FolderPath = t.TempDir()
	invalidCfg10.Compression.Enabled = true
	invalidCfg10.Compression.Algorithm = "gzip"
	invalidCfg10.Compression.Level = 15 // Too high for gzip
	if err := invalidCfg10.Validate(); err == nil {
		t.Error("Expected error for invalid gzip compression level")
	}

	// Test invalid log level
	invalidCfg11 := config.DefaultConfig()
	invalidCfg11.Sync.FolderPath = t.TempDir()
	invalidCfg11.Observability.LogLevel = "invalid"
	if err := invalidCfg11.Validate(); err == nil {
		t.Error("Expected error for invalid log level")
	}

	// Test invalid concurrent transfers
	invalidCfg12 := config.DefaultConfig()
	invalidCfg12.Sync.FolderPath = t.TempDir()
	invalidCfg12.Sync.MaxConcurrentTransfers = 0 // Too low
	if err := invalidCfg12.Validate(); err == nil {
		t.Error("Expected error for max_concurrent_transfers too low")
	}

	invalidCfg13 := config.DefaultConfig()
	invalidCfg13.Sync.FolderPath = t.TempDir()
	invalidCfg13.Sync.MaxConcurrentTransfers = 50 // Too high
	if err := invalidCfg13.Validate(); err == nil {
		t.Error("Expected error for max_concurrent_transfers too high")
	}
}
