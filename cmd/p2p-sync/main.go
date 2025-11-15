package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/p2p-folder-sync/p2p-sync/internal/config"
	"github.com/p2p-folder-sync/p2p-sync/internal/database"
	"github.com/p2p-folder-sync/p2p-sync/internal/network"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/connection"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/discovery"
	"github.com/p2p-folder-sync/p2p-sync/internal/network/transport"
	"github.com/p2p-folder-sync/p2p-sync/internal/observability"
	"github.com/p2p-folder-sync/p2p-sync/internal/sync"
)

const (
	AppName    = "p2p-sync"
	AppVersion = "0.1.0"
)

func main() {
	var configPath = flag.String("config", "", "Path to configuration file")
	flag.Parse()

	// Load configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	logger, err := observability.NewLogger(cfg.Observability.LogLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("Starting P2P Folder Sync", zap.String("version", AppVersion))

	// Initialize database
	db, err := database.NewDB(cfg.GetDBPath())
	if err != nil {
		logger.Fatal("Failed to initialize database",
			zap.Error(err),
			zap.String("path", cfg.GetDBPath()))
	}
	defer db.Close()

	logger.Info("Database initialized", zap.String("path", cfg.GetDBPath()))

	// Initialize metrics
	if cfg.Observability.MetricsEnabled {
		ctx := context.Background()
		meterProvider, metricsShutdown, err := observability.InitMetricsProvider(ctx, cfg.Observability.OTELendpoint, AppName)
		if err != nil {
			logger.Fatal("Failed to initialize metrics provider", zap.Error(err))
		}
		defer func() {
			if err := metricsShutdown(); err != nil {
				logger.Error("Failed to shutdown metrics provider", zap.Error(err))
			}
		}()

		metrics, err := observability.NewMetrics(meterProvider, AppName)
		if err != nil {
			logger.Fatal("Failed to initialize metrics", zap.Error(err))
		}
		logger.Info("Metrics initialized")
		_ = metrics // TODO: Use metrics throughout the application
	}

	// Initialize tracing
	if cfg.Observability.TracingEnabled {
		ctx := context.Background()
		tp, shutdown, err := observability.InitTracing(ctx, cfg.Observability.OTELendpoint, AppName)
		if err != nil {
			logger.Fatal("Failed to initialize tracing", zap.Error(err))
		}
		defer func() {
			if err := shutdown(); err != nil {
				logger.Error("Failed to shutdown tracing", zap.Error(err))
			}
		}()
		logger.Info("Tracing initialized")
		_ = tp // TODO: Use tracer throughout the application
	}

	// Generate or load peer ID
	peerID := os.Getenv("PEER_ID")
	if peerID == "" {
		peerID = generatePeerID()
		logger.Info("Generated peer ID", zap.String("peer_id", peerID))
	} else {
		logger.Info("Using configured peer ID", zap.String("peer_id", peerID))
	}

	// Initialize connection manager
	connManager := connection.NewConnectionManager()

	// Initialize peer registry
	peerRegistry := discovery.NewRegistry()
	_ = peerRegistry // TODO: Use in discovery integration

	// Start UDP discovery service
	capabilities := map[string]interface{}{
		"encryption":  true,
		"compression": cfg.Compression.Enabled,
		"chunking":    true,
	}
	udpDiscovery := discovery.NewUDPDiscovery(cfg.Network.DiscoveryPort, peerID, capabilities, AppVersion)
	if err := udpDiscovery.Start(); err != nil {
		logger.Fatal("Failed to start UDP discovery", zap.Error(err))
	}
	defer udpDiscovery.Stop()
	logger.Info("UDP discovery started", zap.Int("port", cfg.Network.DiscoveryPort))

	// Initialize network transport
	protocol := cfg.Network.Protocol
	if protocol == "" {
		protocol = "quic" // Default to QUIC
	}

	transportFactory := &transport.TransportFactory{}
	networkTransport, err := transportFactory.NewTransport(protocol, cfg.Network.Port)
	if err != nil {
		logger.Fatal("Failed to create network transport", zap.Error(err))
	}

	// Try to start the transport
	if err := networkTransport.Start(); err != nil {
		logger.Warn("Failed to start primary transport, attempting fallback",
			zap.String("protocol", protocol), zap.Error(err))

		// Fallback logic: if QUIC fails, try TCP
		if protocol == "quic" {
			logger.Info("Attempting TCP fallback")
			networkTransport, err = transportFactory.NewTransport("tcp", cfg.Network.Port)
			if err != nil {
				logger.Fatal("Failed to create TCP fallback transport", zap.Error(err))
			}
			if err := networkTransport.Start(); err != nil {
				logger.Fatal("Failed to start TCP fallback transport", zap.Error(err))
			}
			logger.Info("TCP fallback transport started", zap.Int("port", cfg.Network.Port))
		} else {
			logger.Fatal("Failed to start network transport", zap.Error(err))
		}
	} else {
		logger.Info("Network transport started",
			zap.String("protocol", protocol), zap.Int("port", cfg.Network.Port))
	}
	defer networkTransport.Stop()

	// Initialize network message handler
	messageHandler := network.NewNetworkMessageHandler(cfg, nil, nil, peerID) // syncEngine and heartbeatManager will be set later

	// Initialize heartbeat manager
	heartbeatManager := connection.NewHeartbeatManager(connManager, networkTransport, cfg)
	heartbeatManager.Start()

	// Set heartbeat manager in message handler
	messageHandler.SetHeartbeatManager(heartbeatManager)

	// Initialize network messenger
	networkMessenger, err := network.NewNetworkMessenger(cfg, connManager, networkTransport, peerID)
	if err != nil {
		logger.Fatal("Failed to create network messenger", zap.Error(err))
	}

	// Set message handler for network messenger (for decrypted messages)
	networkMessenger.SetMessageHandler(messageHandler)

	// Set network messenger as transport's message handler (for encrypted messages)
	if err := networkTransport.SetMessageHandler(networkMessenger); err != nil {
		logger.Fatal("Failed to set message handler", zap.Error(err))
	}

	// Initialize sync engine with messenger
	syncEngine, err := sync.NewEngineWithMessenger(cfg, db, peerID, networkMessenger)
	if err != nil {
		logger.Fatal("Failed to create sync engine", zap.Error(err))
	}

	// Set sync engine in message handler
	messageHandler.SetSyncEngine(syncEngine)

	// Start sync engine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := syncEngine.Start(ctx); err != nil {
		logger.Fatal("Failed to start sync engine", zap.Error(err))
	}
	logger.Info("Sync engine started", zap.String("folder", cfg.Sync.FolderPath))

	logger.Info("P2P Folder Sync initialized successfully")

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		logger.Info("Received shutdown signal", zap.String("signal", sig.String()))
	case <-ctx.Done():
		logger.Info("Context cancelled")
	}

	// Graceful shutdown
	logger.Info("Shutting down...")

	// Stop heartbeat manager
	heartbeatManager.Stop()

	// Stop network transport
	if err := networkTransport.Stop(); err != nil {
		logger.Error("Failed to stop network transport", zap.Error(err))
	}

	// Stop sync engine
	if err := syncEngine.Stop(); err != nil {
		logger.Error("Failed to stop sync engine", zap.Error(err))
	}

	logger.Info("Shutdown complete")
}

// generatePeerID generates a unique peer ID
func generatePeerID() string {
	// Use hostname + timestamp for uniqueness
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	timestamp := time.Now().UnixNano()
	return fmt.Sprintf("%s-%d", hostname, timestamp)
}
