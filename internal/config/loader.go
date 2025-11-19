package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadConfig loads configuration from file and environment variables
func LoadConfig(configPath string) (*Config, error) {
	cfg := DefaultConfig()

	// Load from YAML file if it exists
	if configPath != "" {
		if _, err := os.Stat(configPath); err == nil {
			data, err := os.ReadFile(configPath)
			if err != nil {
				return nil, fmt.Errorf("failed to read config file: %w", err)
			}

			if err := yaml.Unmarshal(data, cfg); err != nil {
				return nil, fmt.Errorf("failed to parse config file: %w", err)
			}
		}
	}

	// Override with environment variables
	loadFromEnv(cfg)

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// loadFromEnv loads configuration from environment variables
func loadFromEnv(cfg *Config) {
	// Sync folder path
	if val := os.Getenv("P2P_SYNC_FOLDER"); val != "" {
		cfg.Sync.FolderPath = val
	}

	// Network settings
	if val := os.Getenv("P2P_PORT"); val != "" {
		if port := parseInt(val); port > 0 {
			cfg.Network.Port = port
		}
	}
	if val := os.Getenv("P2P_DISCOVERY_PORT"); val != "" {
		if port := parseInt(val); port > 0 {
			cfg.Network.DiscoveryPort = port
		}
	}
	if val := os.Getenv("P2P_PROTOCOL"); val != "" {
		cfg.Network.Protocol = val
	}

	// Manual peer list
	if val := os.Getenv("PEERS"); val != "" {
		peers := strings.Split(val, ",")
		cfg.Network.Peers = make([]string, 0, len(peers))
		for _, peer := range peers {
			peer = strings.TrimSpace(peer)
			if peer != "" {
				cfg.Network.Peers = append(cfg.Network.Peers, peer)
			}
		}
	}

	// Compression settings
	if val := os.Getenv("P2P_COMPRESSION_ENABLED"); val != "" {
		cfg.Compression.Enabled = val == "true" || val == "1"
	}
	if val := os.Getenv("P2P_COMPRESSION_ALGORITHM"); val != "" {
		cfg.Compression.Algorithm = val
	}

	// Observability
	if val := os.Getenv("OTEL_ENDPOINT"); val != "" {
		cfg.Observability.OTELendpoint = val
	}
	if val := os.Getenv("P2P_LOG_LEVEL"); val != "" {
		cfg.Observability.LogLevel = val
	}

	// Peer ID
	if val := os.Getenv("PEER_ID"); val != "" {
		// Store in a way that can be accessed later
		// This will be handled by the main application
	}
}

// parseInt parses an integer from a string, returns 0 on error
func parseInt(s string) int {
	var val int
	fmt.Sscanf(s, "%d", &val)
	return val
}


