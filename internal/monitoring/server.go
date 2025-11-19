package monitoring

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
)

// Server provides HTTP endpoints for monitoring metrics
type Server struct {
	metrics *Metrics
	port    int
	server  *http.Server
	mu      sync.Mutex
}

// NewServer creates a new monitoring server
func NewServer(metrics *Metrics, port int) *Server {
	return &Server{
		metrics: metrics,
		port:    port,
	}
}

// Start starts the monitoring HTTP server
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", s.handleMetrics)
	mux.HandleFunc("/metrics/summary", s.handleSummary)
	mux.HandleFunc("/metrics/sync", s.handleSyncMetrics)
	mux.HandleFunc("/metrics/network", s.handleNetworkMetrics)
	mux.HandleFunc("/metrics/peers", s.handlePeerMetrics)
	mux.HandleFunc("/metrics/conflicts", s.handleConflictMetrics)
	mux.HandleFunc("/metrics/errors", s.handleErrorMetrics)
	mux.HandleFunc("/health", s.handleHealth)

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: mux,
	}

	go func() {
		log.Printf("Starting monitoring server on port %d", s.port)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Monitoring server error: %v", err)
		}
	}()

	return nil
}

// Stop stops the monitoring server
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server != nil {
		return s.server.Close()
	}
	return nil
}

// handleMetrics returns all metrics as JSON
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	snapshot := s.metrics.GetSnapshot()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if err := json.NewEncoder(w).Encode(snapshot); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode metrics: %v", err), http.StatusInternalServerError)
		return
	}
}

// handleSummary returns a summary of key metrics
func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	snapshot := s.metrics.GetSnapshot()
	summary := snapshot.GetSummary()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if err := json.NewEncoder(w).Encode(summary); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode summary: %v", err), http.StatusInternalServerError)
		return
	}
}

// handleSyncMetrics returns sync operation metrics
func (s *Server) handleSyncMetrics(w http.ResponseWriter, r *http.Request) {
	s.metrics.syncOpsMu.RLock()
	syncOps := s.metrics.syncOps
	s.metrics.syncOpsMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if err := json.NewEncoder(w).Encode(syncOps); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode sync metrics: %v", err), http.StatusInternalServerError)
		return
	}
}

// handleNetworkMetrics returns network metrics
func (s *Server) handleNetworkMetrics(w http.ResponseWriter, r *http.Request) {
	s.metrics.networkMu.RLock()
	network := s.metrics.network
	s.metrics.networkMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if err := json.NewEncoder(w).Encode(network); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode network metrics: %v", err), http.StatusInternalServerError)
		return
	}
}

// handlePeerMetrics returns peer metrics
func (s *Server) handlePeerMetrics(w http.ResponseWriter, r *http.Request) {
	s.metrics.peersMu.RLock()
	peers := s.metrics.peers
	s.metrics.peersMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if err := json.NewEncoder(w).Encode(peers); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode peer metrics: %v", err), http.StatusInternalServerError)
		return
	}
}

// handleConflictMetrics returns conflict metrics
func (s *Server) handleConflictMetrics(w http.ResponseWriter, r *http.Request) {
	s.metrics.conflictsMu.RLock()
	conflicts := s.metrics.conflicts
	s.metrics.conflictsMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if err := json.NewEncoder(w).Encode(conflicts); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode conflict metrics: %v", err), http.StatusInternalServerError)
		return
	}
}

// handleErrorMetrics returns error metrics
func (s *Server) handleErrorMetrics(w http.ResponseWriter, r *http.Request) {
	s.metrics.errorsMu.RLock()
	errors := s.metrics.errors
	s.metrics.errorsMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if err := json.NewEncoder(w).Encode(errors); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode error metrics: %v", err), http.StatusInternalServerError)
		return
	}
}

// handleHealth returns health status
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	snapshot := s.metrics.GetSnapshot()

	health := map[string]interface{}{
		"status":            "healthy",
		"uptime_seconds":    snapshot.Uptime.Seconds(),
		"connected_peers":   snapshot.Peers.ConnectedPeers,
		"active_transfers":  snapshot.FlowControl.ActiveTransfers,
		"pending_conflicts": snapshot.Conflicts.PendingConflicts,
		"error_rate":        float64(snapshot.Errors.TotalErrors) / snapshot.Uptime.Seconds(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if err := json.NewEncoder(w).Encode(health); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode health: %v", err), http.StatusInternalServerError)
		return
	}
}
