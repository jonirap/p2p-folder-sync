package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Config represents the complete application configuration
type Config struct {
	Sync         SyncConfig         `yaml:"sync"`
	Network      NetworkConfig      `yaml:"network"`
	Security     SecurityConfig     `yaml:"security"`
	Compression  CompressionConfig  `yaml:"compression"`
	Conflict     ConflictConfig     `yaml:"conflict"`
	Observability ObservabilityConfig `yaml:"observability"`
}

// SyncConfig contains synchronization settings
type SyncConfig struct {
	FolderPath            string `yaml:"folder_path"`
	ChunkSizeMin          int64  `yaml:"chunk_size_min"`
	ChunkSizeMax          int64  `yaml:"chunk_size_max"`
	ChunkSizeDefault      int64  `yaml:"chunk_size_default"`
	MaxConcurrentTransfers int    `yaml:"max_concurrent_transfers"`
	OperationLogSize      int    `yaml:"operation_log_size"`
}

// NetworkConfig contains network settings
type NetworkConfig struct {
	Port            int      `yaml:"port"`
	DiscoveryPort   int      `yaml:"discovery_port"`
	Protocol        string   `yaml:"protocol"`           // "quic" or "tcp"
	HeartbeatInterval int    `yaml:"heartbeat_interval"` // seconds
	ConnectionTimeout int    `yaml:"connection_timeout"`  // seconds
	Peers           []string `yaml:"peers"`              // Manual peer list
}

// SecurityConfig contains security settings
type SecurityConfig struct {
	KeyRotationInterval int64  `yaml:"key_rotation_interval"` // seconds
	EncryptionAlgorithm string `yaml:"encryption_algorithm"`
}

// CompressionConfig contains compression settings
type CompressionConfig struct {
	Enabled           bool   `yaml:"enabled"`
	FileSizeThreshold int64  `yaml:"file_size_threshold"`
	Algorithm         string `yaml:"algorithm"`
	Level             int    `yaml:"level"`
	ChunkCompression  bool   `yaml:"chunk_compression"`
}

// ConflictConfig contains conflict resolution settings
type ConflictConfig struct {
	ResolutionStrategy string `yaml:"resolution_strategy"`
}

// ObservabilityConfig contains observability settings
type ObservabilityConfig struct {
	OTELendpoint   string `yaml:"otel_endpoint"`
	LogLevel       string `yaml:"log_level"`
	MetricsEnabled bool   `yaml:"metrics_enabled"`
	TracingEnabled bool   `yaml:"tracing_enabled"`
}

// DefaultConfig returns a configuration with default values
func DefaultConfig() *Config {
	return &Config{
		Sync: SyncConfig{
			FolderPath:            "/tmp/p2p-sync",
			ChunkSizeMin:          65536,      // 64 KB
			ChunkSizeMax:          2097152,   // 2 MB
			ChunkSizeDefault:      524288,    // 512 KB
			MaxConcurrentTransfers: 5,
			OperationLogSize:       10000,
		},
		Network: NetworkConfig{
			Port:              8080,
			DiscoveryPort:     8081,
			Protocol:          "quic", // QUIC primary, TCP fallback
			HeartbeatInterval: 30,
			ConnectionTimeout: 60,
			Peers:            []string{},
		},
		Security: SecurityConfig{
			KeyRotationInterval: 86400, // 24 hours
			EncryptionAlgorithm: "aes-256-gcm",
		},
		Compression: CompressionConfig{
			Enabled:           true,
			FileSizeThreshold: 1048576, // 1 MB
			Algorithm:         "zstd",
			Level:             3,
			ChunkCompression:  true,
		},
		Conflict: ConflictConfig{
			ResolutionStrategy: "intelligent_merge",
		},
		Observability: ObservabilityConfig{
			OTELendpoint:   "",
			LogLevel:       "info",
			MetricsEnabled: true,
			TracingEnabled: true,
		},
	}
}

// Validate validates the configuration and returns an error if invalid
func (c *Config) Validate() error {
	// Validate folder path
	if c.Sync.FolderPath == "" {
		return fmt.Errorf("sync.folder_path is required")
	}
	
	// Check if folder exists and is writable
	info, err := os.Stat(c.Sync.FolderPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Try to create it
			if err := os.MkdirAll(c.Sync.FolderPath, 0755); err != nil {
				return fmt.Errorf("cannot create sync folder: %w", err)
			}
		} else {
			return fmt.Errorf("cannot access sync folder: %w", err)
		}
	} else if !info.IsDir() {
		return fmt.Errorf("sync folder path is not a directory")
	}
	
	// Check if writable
	testFile := filepath.Join(c.Sync.FolderPath, ".p2p-sync-test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		return fmt.Errorf("sync folder is not writable: %w", err)
	}
	os.Remove(testFile)

	// Validate chunk sizes
	if c.Sync.ChunkSizeMin < 4096 || c.Sync.ChunkSizeMin > 1048576 {
		return fmt.Errorf("chunk_size_min must be between 4096 and 1048576 bytes")
	}
	if c.Sync.ChunkSizeMax < 1048576 || c.Sync.ChunkSizeMax > 10485760 {
		return fmt.Errorf("chunk_size_max must be between 1048576 and 10485760 bytes")
	}
	if c.Sync.ChunkSizeDefault < c.Sync.ChunkSizeMin ||
		c.Sync.ChunkSizeDefault > c.Sync.ChunkSizeMax {
		return fmt.Errorf("chunk_size_default must be between chunk_size_min and chunk_size_max")
	}

	// Validate concurrent transfers
	if c.Sync.MaxConcurrentTransfers < 1 || c.Sync.MaxConcurrentTransfers > 20 {
		return fmt.Errorf("max_concurrent_transfers must be between 1 and 20")
	}

	// Validate network settings
	// Port 0 means "use any available port"
	if c.Network.Port != 0 && (c.Network.Port < 1024 || c.Network.Port > 65535) {
		return fmt.Errorf("network.port must be 0 or between 1024 and 65535")
	}
	if c.Network.DiscoveryPort != 0 && (c.Network.DiscoveryPort < 1024 || c.Network.DiscoveryPort > 65535) {
		return fmt.Errorf("network.discovery_port must be 0 or between 1024 and 65535")
	}
	if c.Network.Protocol != "quic" && c.Network.Protocol != "tcp" && c.Network.Protocol != "" {
		return fmt.Errorf("network.protocol must be 'quic', 'tcp', or empty (defaults to quic)")
	}

	// Validate security settings
	// Allow bypass for testing via environment variable
	if os.Getenv("P2P_TESTING_MODE") != "true" {
		if c.Security.KeyRotationInterval < 3600 || c.Security.KeyRotationInterval > 604800 {
			return fmt.Errorf("key_rotation_interval must be between 3600 and 604800 seconds")
		}
	} else {
		// In test mode, allow shorter intervals but still validate minimum
		if c.Security.KeyRotationInterval < 1 || c.Security.KeyRotationInterval > 604800 {
			return fmt.Errorf("key_rotation_interval must be between 1 and 604800 seconds (test mode)")
		}
	}

	// Validate compression settings
	if err := c.Compression.Validate(); err != nil {
		return fmt.Errorf("compression config: %w", err)
	}

	// Validate observability settings
	if c.Observability.LogLevel != "debug" &&
		c.Observability.LogLevel != "info" &&
		c.Observability.LogLevel != "warn" &&
		c.Observability.LogLevel != "error" {
		return fmt.Errorf("log_level must be one of: debug, info, warn, error")
	}

	return nil
}

// Validate validates compression configuration
func (cc *CompressionConfig) Validate() error {
	if !cc.Enabled {
		return nil
	}

	if cc.FileSizeThreshold < 1024 || cc.FileSizeThreshold > 1073741824 {
		return fmt.Errorf("file_size_threshold must be between 1024 and 1073741824 bytes")
	}

	switch cc.Algorithm {
	case "zstd":
		if cc.Level < 1 || cc.Level > 22 {
			return fmt.Errorf("zstd level must be between 1 and 22")
		}
	case "lz4":
		if cc.Level < 1 || cc.Level > 16 {
			return fmt.Errorf("lz4 level must be between 1 and 16")
		}
	case "gzip":
		if cc.Level < 1 || cc.Level > 9 {
			return fmt.Errorf("gzip level must be between 1 and 9")
		}
	case "none":
		// No level validation needed
	default:
		return fmt.Errorf("compression algorithm must be one of: zstd, lz4, gzip, none")
	}

	return nil
}

// GetDataDir returns the data directory path for storing database and other persistent data
func (c *Config) GetDataDir() string {
	return filepath.Join(c.Sync.FolderPath, ".p2p-sync")
}

// GetDBPath returns the database file path
func (c *Config) GetDBPath() string {
	return filepath.Join(c.GetDataDir(), "p2p_sync.db")
}

// GetKeychainPath returns the keychain file path
func (c *Config) GetKeychainPath() string {
	return filepath.Join(c.GetDataDir(), "keychain.db")
}

