package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/p2p-folder-sync/p2p-sync/internal/config"
	"github.com/p2p-folder-sync/p2p-sync/internal/database"
)

func TestConfigLoad(t *testing.T) {
	// Create a temporary config directory
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	
	// Create a minimal config file
	configContent := `
sync:
  folder_path: "` + tmpDir + `/sync"
  chunk_size_min: 65536
  chunk_size_max: 2097152
  chunk_size_default: 524288
  max_concurrent_transfers: 5

network:
  port: 8080
  discovery_port: 8081
  heartbeat_interval: 30
  connection_timeout: 60

compression:
  enabled: true
  file_size_threshold: 1048576
  algorithm: "zstd"
  level: 3
`
	
	if err := os.WriteFile(cfgPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	
	if cfg.Sync.FolderPath == "" {
		t.Error("Folder path should be set")
	}
	
	if cfg.Network.Port != 8080 {
		t.Errorf("Expected port 8080, got %d", cfg.Network.Port)
	}
}

func TestDatabaseInit(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	
	db, err := database.NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()
	
	// Test that we can query the database
	files, err := db.GetAllFiles()
	if err != nil {
		t.Fatalf("Failed to get files: %v", err)
	}
	
	// Should return an empty slice, not nil
	if files == nil {
		t.Error("GetAllFiles should return a slice, not nil")
	}
	
	// Should be empty for a new database
	if len(files) != 0 {
		t.Errorf("Expected 0 files in new database, got %d", len(files))
	}
}

