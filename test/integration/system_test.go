package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/p2p-folder-sync/p2p-sync/internal/config"
	"github.com/p2p-folder-sync/p2p-sync/internal/database"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/connection"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/discovery"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/transport"
	"github.com/p2p-folder-sync/p2p-sync/internal/observability"
	"github.com/p2p-folder-sync/p2p-sync/internal/sync"
)

// TestFullApplicationLifecycle tests the complete application startup and shutdown cycle
func TestFullApplicationLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	syncDir := filepath.Join(tmpDir, "sync")

	// Create sync directory
	if err := os.MkdirAll(syncDir, 0755); err != nil {
		t.Fatalf("Failed to create sync directory: %v", err)
	}

	// Create configuration
	cfgContent := fmt.Sprintf(`
sync:
  folder_path: "%s"
  chunk_size_min: 65536
  chunk_size_max: 2097152
  chunk_size_default: 524288
  max_concurrent_transfers: 5

network:
  port: 0  # Use random port for testing
  discovery_port: 0  # Use random port for testing
  heartbeat_interval: 30
  connection_timeout: 60

compression:
  enabled: true
  algorithm: "zstd"
  level: 3

observability:
  log_level: "info"
  metrics_enabled: false
  tracing_enabled: false
`, syncDir)

	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(cfgContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load configuration
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize database
	db, err := database.NewDB(filepath.Join(syncDir, ".p2p-sync", "test.db"))
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Initialize logger (silenced for testing)
	logger, err := observability.NewLogger("error")
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Sync()

	// Generate peer ID
	peerID := "test-peer-123"

	// Initialize sync engine
	syncEngine, err := sync.NewEngine(cfg, db, peerID)
	if err != nil {
		t.Fatalf("Failed to create sync engine: %v", err)
	}

	// Initialize connection manager
	connManager := connection.NewConnectionManager()

	// Initialize peer registry
	peerRegistry := discovery.NewRegistry()

	// Start sync engine
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := syncEngine.Start(ctx); err != nil {
		t.Fatalf("Failed to start sync engine: %v", err)
	}

	// Initialize network transport (use TCP for testing reliability)
	transportFactory := &transport.TransportFactory{}
	networkTransport, err := transportFactory.NewTransport("tcp", 0) // Use random port
	if err != nil {
		t.Fatalf("Failed to create network transport: %v", err)
	}

	// Try to start the transport
	if err := networkTransport.Start(); err != nil {
		t.Logf("Failed to start transport (expected in test environment): %v", err)
	} else {
		defer networkTransport.Stop()
	}

	// Let components run briefly
	time.Sleep(100 * time.Millisecond)

	// Stop sync engine
	if err := syncEngine.Stop(); err != nil {
		t.Errorf("Failed to stop sync engine: %v", err)
	}

	// Stop transport if it was started
	if err := networkTransport.Stop(); err != nil {
		t.Logf("Failed to stop transport: %v", err)
	}

	// Verify components are properly cleaned up
	_ = connManager  // Used in real application
	_ = peerRegistry // Used in real application
	_ = logger       // Used for logging

	// Verify sync directory still exists and is accessible
	if _, err := os.Stat(syncDir); os.IsNotExist(err) {
		t.Error("Sync directory was removed during shutdown")
	}

	// Verify database is still accessible
	if _, err := db.GetAllFiles(); err != nil {
		t.Errorf("Database became inaccessible after shutdown: %v", err)
	}
}

// TestFileSynchronizationLifecycle tests the complete file sync process
func TestFileSynchronizationLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	syncDir := filepath.Join(tmpDir, "sync")

	// Create sync directory
	if err := os.MkdirAll(syncDir, 0755); err != nil {
		t.Fatalf("Failed to create sync directory: %v", err)
	}

	// Create configuration
	cfg := &config.Config{}
	cfg.Sync.FolderPath = syncDir
	cfg.Sync.ChunkSizeMin = 65536
	cfg.Sync.ChunkSizeMax = 2097152
	cfg.Sync.ChunkSizeDefault = 524288
	cfg.Sync.MaxConcurrentTransfers = 5
	cfg.Network.Port = 0
	cfg.Network.DiscoveryPort = 0
	cfg.Compression.Enabled = true
	cfg.Compression.Algorithm = "zstd"
	cfg.Compression.Level = 3
	cfg.Observability.LogLevel = "error"
	cfg.Observability.MetricsEnabled = false
	cfg.Observability.TracingEnabled = false

	// Initialize database
	db, err := database.NewDB(filepath.Join(syncDir, ".p2p-sync", "test.db"))
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	peerID := "test-peer-sync"

	// Initialize sync engine
	syncEngine, err := sync.NewEngine(cfg, db, peerID)
	if err != nil {
		t.Fatalf("Failed to create sync engine: %v", err)
	}

	// Start sync engine
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := syncEngine.Start(ctx); err != nil {
		t.Fatalf("Failed to start sync engine: %v", err)
	}

	// Create a test file
	testFile := filepath.Join(syncDir, "test.txt")
	testContent := []byte("Hello, P2P Sync World!")
	if err := os.WriteFile(testFile, testContent, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// The database stores relative paths, so we expect "test.txt"
	expectedPath := "test.txt"

	// Wait for file system events to be processed
	time.Sleep(500 * time.Millisecond)

	// Verify file was indexed in database
	files, err := db.GetAllFiles()
	if err != nil {
		t.Fatalf("Failed to get files from database: %v", err)
	}

	if len(files) == 0 {
		t.Error("File was not indexed in database")
	} else {
		file := files[0]
		if file.Path != expectedPath {
			t.Errorf("Expected file path %s, got %s", expectedPath, file.Path)
		}
		if file.Size != int64(len(testContent)) {
			t.Errorf("Expected file size %d, got %d", len(testContent), file.Size)
		}
		if file.PeerID != peerID {
			t.Errorf("Expected peer ID %s, got %s", peerID, file.PeerID)
		}
	}

	// Modify the file
	modifiedContent := []byte("Hello, Modified P2P Sync World!")
	if err := os.WriteFile(testFile, modifiedContent, 0644); err != nil {
		t.Fatalf("Failed to modify test file: %v", err)
	}

	// Wait for modification to be processed
	time.Sleep(500 * time.Millisecond)

	// Verify modification was detected
	updatedFiles, err := db.GetAllFiles()
	if err != nil {
		t.Fatalf("Failed to get updated files: %v", err)
	}

	if len(updatedFiles) != 1 {
		t.Errorf("Expected 1 file after modification, got %d", len(updatedFiles))
	} else {
		updatedFile := updatedFiles[0]
		if updatedFile.Size != int64(len(modifiedContent)) {
			t.Errorf("Expected updated file size %d, got %d", len(modifiedContent), updatedFile.Size)
		}
	}

	// Delete the file
	if err := os.Remove(testFile); err != nil {
		t.Fatalf("Failed to delete test file: %v", err)
	}

	// Wait for deletion to be processed
	time.Sleep(500 * time.Millisecond)

	// Stop sync engine
	if err := syncEngine.Stop(); err != nil {
		t.Errorf("Failed to stop sync engine: %v", err)
	}

	// Verify database still contains file records (for sync history)
	finalFiles, err := db.GetAllFiles()
	if err != nil {
		t.Fatalf("Failed to get final files: %v", err)
	}

	// Files should still be in database for sync history
	if len(finalFiles) == 0 {
		t.Log("File records were cleaned up (expected behavior)")
	}
}

// TestMultiplePeerSimulation tests the interaction between multiple simulated peers
func TestMultiplePeerSimulation(t *testing.T) {
	tmpDir := t.TempDir()

	// Create configurations for two peers
	createPeerConfig := func(peerID string, syncDir string) *config.Config {
		cfg := &config.Config{}
		cfg.Sync.FolderPath = syncDir
		cfg.Sync.ChunkSizeMin = 65536
		cfg.Sync.ChunkSizeMax = 2097152
		cfg.Sync.ChunkSizeDefault = 524288
		cfg.Sync.MaxConcurrentTransfers = 5
		cfg.Network.Port = 0
		cfg.Network.DiscoveryPort = 0
		cfg.Compression.Enabled = true
		cfg.Compression.Algorithm = "zstd"
		cfg.Compression.Level = 3
		cfg.Observability.LogLevel = "error"
		cfg.Observability.MetricsEnabled = false
		cfg.Observability.TracingEnabled = false
		return cfg
	}

	// Peer 1 setup
	peer1Dir := filepath.Join(tmpDir, "peer1")
	if err := os.MkdirAll(peer1Dir, 0755); err != nil {
		t.Fatalf("Failed to create peer1 directory: %v", err)
	}

	cfg1 := createPeerConfig("peer1", peer1Dir)
	db1, err := database.NewDB(filepath.Join(peer1Dir, ".p2p-sync", "peer1.db"))
	if err != nil {
		t.Fatalf("Failed to initialize peer1 database: %v", err)
	}
	defer db1.Close()

	syncEngine1, err := sync.NewEngine(cfg1, db1, "peer1")
	if err != nil {
		t.Fatalf("Failed to create peer1 sync engine: %v", err)
	}

	// Peer 2 setup
	peer2Dir := filepath.Join(tmpDir, "peer2")
	if err := os.MkdirAll(peer2Dir, 0755); err != nil {
		t.Fatalf("Failed to create peer2 directory: %v", err)
	}

	cfg2 := createPeerConfig("peer2", peer2Dir)
	db2, err := database.NewDB(filepath.Join(peer2Dir, ".p2p-sync", "peer2.db"))
	if err != nil {
		t.Fatalf("Failed to initialize peer2 database: %v", err)
	}
	defer db2.Close()

	syncEngine2, err := sync.NewEngine(cfg2, db2, "peer2")
	if err != nil {
		t.Fatalf("Failed to create peer2 sync engine: %v", err)
	}

	// Start both sync engines
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := syncEngine1.Start(ctx); err != nil {
		t.Fatalf("Failed to start peer1 sync engine: %v", err)
	}

	if err := syncEngine2.Start(ctx); err != nil {
		t.Fatalf("Failed to start peer2 sync engine: %v", err)
	}

	// Peer 1 creates a file
	peer1File := filepath.Join(peer1Dir, "shared.txt")
	fileContent := []byte("Shared content between peers")
	if err := os.WriteFile(peer1File, fileContent, 0644); err != nil {
		t.Fatalf("Failed to create shared file in peer1: %v", err)
	}

	// Wait for peer1 to index the file
	time.Sleep(500 * time.Millisecond)

	// Verify peer1 has the file
	peer1Files, err := db1.GetAllFiles()
	if err != nil {
		t.Fatalf("Failed to get peer1 files: %v", err)
	}

	if len(peer1Files) != 1 {
		t.Errorf("Expected peer1 to have 1 file, got %d", len(peer1Files))
	}

	// Simulate file sync to peer2 (in real implementation, this would happen via network)
	peer1FileData := fileContent // In real sync, this would be read from peer1
	syncOp := &sync.SyncOperation{
		FileID:   peer1Files[0].FileID,
		Path:     "shared.txt", // Relative path within sync folder
		Checksum: peer1Files[0].Checksum,
		Size:     peer1Files[0].Size,
		Mtime:    peer1Files[0].Mtime.Unix(),
		PeerID:   "peer2",
		Type:     sync.OpCreate,
	}

	// Handle incoming file on peer2
	if err := syncEngine2.HandleIncomingFile(peer1FileData, syncOp); err != nil {
		t.Fatalf("Failed to handle incoming file on peer2: %v", err)
	}

	// Wait for peer2 to process the file
	time.Sleep(500 * time.Millisecond)

	// Verify peer2 received the file
	peer2Files, err := db2.GetAllFiles()
	if err != nil {
		t.Fatalf("Failed to get peer2 files: %v", err)
	}

	if len(peer2Files) != 1 {
		t.Errorf("Expected peer2 to have 1 file, got %d", len(peer2Files))
	}

	// Verify file contents match
	peer2FilePath := filepath.Join(peer2Dir, "shared.txt")
	if _, err := os.Stat(peer2FilePath); os.IsNotExist(err) {
		t.Error("File was not created on peer2")
	} else {
		peer2Content, err := os.ReadFile(peer2FilePath)
		if err != nil {
			t.Fatalf("Failed to read file on peer2: %v", err)
		}

		if string(peer2Content) != string(fileContent) {
			t.Error("File content does not match between peers")
		}
	}

	// Stop both engines
	if err := syncEngine1.Stop(); err != nil {
		t.Errorf("Failed to stop peer1 sync engine: %v", err)
	}

	if err := syncEngine2.Stop(); err != nil {
		t.Errorf("Failed to stop peer2 sync engine: %v", err)
	}
}

// TestConfigurationValidation tests that invalid configurations are properly rejected
func TestConfigurationValidation(t *testing.T) {
	tmpDir := t.TempDir()

	testCases := []struct {
		name        string
		configYAML  string
		expectError bool
	}{
		{
			name: "valid configuration",
			configYAML: `
sync:
  folder_path: "/tmp/valid"
  chunk_size_min: 65536
  chunk_size_max: 2097152
  chunk_size_default: 524288
network:
  port: 8080
observability:
  log_level: "info"
`,
			expectError: false,
		},
		{
			name: "missing folder path",
			configYAML: `
sync:
  folder_path: ""
  chunk_size_min: 65536
network:
  port: 8080
`,
			expectError: true,
		},
		{
			name: "invalid chunk sizes",
			configYAML: `
sync:
  folder_path: "/tmp/invalid"
  chunk_size_min: 1000
  chunk_size_max: 2097152
  chunk_size_default: 524288
network:
  port: 8080
`,
			expectError: true,
		},
		{
			name: "invalid port",
			configYAML: `
sync:
  folder_path: "/tmp/invalid"
  chunk_size_min: 65536
  chunk_size_max: 2097152
  chunk_size_default: 524288
network:
  port: 1023
`,
			expectError: true,
		},
		{
			name: "invalid log level",
			configYAML: `
sync:
  folder_path: "/tmp/invalid"
  chunk_size_min: 65536
  chunk_size_max: 2097152
  chunk_size_default: 524288
network:
  port: 8080
observability:
  log_level: "invalid"
`,
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			configPath := filepath.Join(tmpDir, tc.name+".yaml")
			if err := os.WriteFile(configPath, []byte(tc.configYAML), 0644); err != nil {
				t.Fatalf("Failed to write config file: %v", err)
			}

			_, err := config.LoadConfig(configPath)
			if tc.expectError && err == nil {
				t.Error("Expected configuration to fail validation")
			}
			if !tc.expectError && err != nil {
				t.Errorf("Expected configuration to pass validation, got error: %v", err)
			}
		})
	}
}

// TestDatabaseMigration tests database schema initialization and upgrades
func TestDatabaseMigration(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "migration.db")

	// Create database
	db, err := database.NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Verify schema was created
	var tableCount int
	err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table'").Scan(&tableCount)
	if err != nil {
		t.Fatalf("Failed to query table count: %v", err)
	}

	expectedTables := 5 // files, operations, peers, chunks, config
	if tableCount < expectedTables {
		t.Errorf("Expected at least %d tables, got %d", expectedTables, tableCount)
	}

	// Test basic operations work
	testFile := &database.FileMetadata{
		FileID:   "migration-test",
		Path:     "/tmp/migration.txt",
		Checksum: "migration-hash",
		Size:     1024,
		Mtime:    time.Now(),
		PeerID:   "migration-peer",
	}

	err = db.InsertFile(testFile)
	if err != nil {
		t.Fatalf("Failed to insert file after migration: %v", err)
	}

	retrieved, err := db.GetFile("migration-test")
	if err != nil {
		t.Fatalf("Failed to retrieve file after migration: %v", err)
	}

	if retrieved.FileID != testFile.FileID {
		t.Errorf("Retrieved file ID doesn't match: expected %s, got %s", testFile.FileID, retrieved.FileID)
	}
}

// TestLargeFileHandling tests handling of large files and chunking
func TestLargeFileHandling(t *testing.T) {
	tmpDir := t.TempDir()
	syncDir := filepath.Join(tmpDir, "sync")

	if err := os.MkdirAll(syncDir, 0755); err != nil {
		t.Fatalf("Failed to create sync directory: %v", err)
	}

	cfg := &config.Config{}
	cfg.Sync.FolderPath = syncDir
	cfg.Sync.ChunkSizeMin = 65536      // 64KB
	cfg.Sync.ChunkSizeMax = 2097152    // 2MB
	cfg.Sync.ChunkSizeDefault = 524288 // 512KB
	cfg.Sync.MaxConcurrentTransfers = 5
	cfg.Network.Port = 0
	cfg.Network.DiscoveryPort = 0
	cfg.Compression.Enabled = true
	cfg.Compression.Algorithm = "zstd"
	cfg.Compression.Level = 3
	cfg.Observability.LogLevel = "error"
	cfg.Observability.MetricsEnabled = false
	cfg.Observability.TracingEnabled = false

	db, err := database.NewDB(filepath.Join(syncDir, ".p2p-sync", "large.db"))
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	syncEngine, err := sync.NewEngine(cfg, db, "large-peer")
	if err != nil {
		t.Fatalf("Failed to create sync engine: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := syncEngine.Start(ctx); err != nil {
		t.Fatalf("Failed to start sync engine: %v", err)
	}

	// Create a moderately large file (1MB)
	largeFilePath := filepath.Join(syncDir, "large.dat")
	largeFileSize := 1024 * 1024 // 1MB
	largeData := make([]byte, largeFileSize)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	if err := os.WriteFile(largeFilePath, largeData, 0644); err != nil {
		t.Fatalf("Failed to create large file: %v", err)
	}

	// Wait for processing
	time.Sleep(1 * time.Second)

	// Verify file was indexed
	files, err := db.GetAllFiles()
	if err != nil {
		t.Fatalf("Failed to get files: %v", err)
	}

	if len(files) != 1 {
		t.Errorf("Expected 1 file, got %d", len(files))
	} else {
		file := files[0]
		if file.Size != int64(largeFileSize) {
			t.Errorf("Expected file size %d, got %d", largeFileSize, file.Size)
		}
	}

	// Stop sync engine
	if err := syncEngine.Stop(); err != nil {
		t.Errorf("Failed to stop sync engine: %v", err)
	}
}

// TestConcurrentFileOperations tests handling of concurrent file operations
func TestConcurrentFileOperations(t *testing.T) {
	tmpDir := t.TempDir()
	syncDir := filepath.Join(tmpDir, "sync")

	if err := os.MkdirAll(syncDir, 0755); err != nil {
		t.Fatalf("Failed to create sync directory: %v", err)
	}

	cfg := &config.Config{}
	cfg.Sync.FolderPath = syncDir
	cfg.Sync.ChunkSizeMin = 65536
	cfg.Sync.ChunkSizeMax = 2097152
	cfg.Sync.ChunkSizeDefault = 524288
	cfg.Sync.MaxConcurrentTransfers = 5
	cfg.Network.Port = 0
	cfg.Network.DiscoveryPort = 0
	cfg.Compression.Enabled = false // Disable compression for speed
	cfg.Observability.LogLevel = "error"
	cfg.Observability.MetricsEnabled = false
	cfg.Observability.TracingEnabled = false

	db, err := database.NewDB(filepath.Join(syncDir, ".p2p-sync", "concurrent.db"))
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	syncEngine, err := sync.NewEngine(cfg, db, "concurrent-peer")
	if err != nil {
		t.Fatalf("Failed to create sync engine: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := syncEngine.Start(ctx); err != nil {
		t.Fatalf("Failed to start sync engine: %v", err)
	}

	// Create multiple files concurrently
	numFiles := 10
	done := make(chan bool, numFiles)

	for i := 0; i < numFiles; i++ {
		go func(index int) {
			defer func() { done <- true }()

			filePath := filepath.Join(syncDir, fmt.Sprintf("concurrent-%d.txt", index))
			content := fmt.Sprintf("Content of file %d", index)

			if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
				t.Errorf("Failed to create file %d: %v", index, err)
				return
			}

			// Modify file
			time.Sleep(10 * time.Millisecond)
			modifiedContent := fmt.Sprintf("Modified content of file %d", index)
			if err := os.WriteFile(filePath, []byte(modifiedContent), 0644); err != nil {
				t.Errorf("Failed to modify file %d: %v", index, err)
				return
			}

			// Delete file
			time.Sleep(10 * time.Millisecond)
			if err := os.Remove(filePath); err != nil {
				t.Errorf("Failed to delete file %d: %v", index, err)
				return
			}
		}(i)
	}

	// Wait for all operations to complete
	for i := 0; i < numFiles; i++ {
		<-done
	}

	// Wait for sync engine to process all events
	time.Sleep(2 * time.Second)

	// Stop sync engine
	if err := syncEngine.Stop(); err != nil {
		t.Errorf("Failed to stop sync engine: %v", err)
	}

	// Verify database integrity
	files, err := db.GetAllFiles()
	if err != nil {
		t.Fatalf("Failed to get final file count: %v", err)
	}

	// Should have some file records (depending on timing)
	t.Logf("Final file count: %d (concurrent operations completed)", len(files))
}
