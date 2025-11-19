package monitoring_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/p2p-folder-sync/p2p-sync/internal/monitoring"
)

func TestNewMetrics(t *testing.T) {
	m := monitoring.NewMetrics()
	if m == nil {
		t.Fatal("NewMetrics returned nil")
	}

	snapshot := m.GetSnapshot()
	if snapshot.SyncOperations.TotalOperations != 0 {
		t.Errorf("Expected 0 total operations, got %d", snapshot.SyncOperations.TotalOperations)
	}

	t.Log("SUCCESS: Metrics initialized correctly")
}

func TestRecordSyncOperation(t *testing.T) {
	m := monitoring.NewMetrics()

	// Record successful create operation
	m.RecordSyncOperation("create", 1024, true)

	snapshot := m.GetSnapshot()
	if snapshot.SyncOperations.TotalOperations != 1 {
		t.Errorf("Expected 1 total operation, got %d", snapshot.SyncOperations.TotalOperations)
	}
	if snapshot.SyncOperations.CreateOperations != 1 {
		t.Errorf("Expected 1 create operation, got %d", snapshot.SyncOperations.CreateOperations)
	}
	if snapshot.SyncOperations.BytesSynced != 1024 {
		t.Errorf("Expected 1024 bytes synced, got %d", snapshot.SyncOperations.BytesSynced)
	}

	t.Log("SUCCESS: Sync operation recorded correctly")
}

func TestRecordMultipleOperations(t *testing.T) {
	m := monitoring.NewMetrics()

	m.RecordSyncOperation("create", 1000, true)
	m.RecordSyncOperation("update", 2000, true)
	m.RecordSyncOperation("delete", 0, true)
	m.RecordSyncOperation("rename", 0, true)
	m.RecordSyncOperation("create", 500, false) // Failed

	snapshot := m.GetSnapshot()
	if snapshot.SyncOperations.TotalOperations != 5 {
		t.Errorf("Expected 5 total operations, got %d", snapshot.SyncOperations.TotalOperations)
	}
	if snapshot.SyncOperations.CreateOperations != 1 {
		t.Errorf("Expected 1 create operation, got %d", snapshot.SyncOperations.CreateOperations)
	}
	if snapshot.SyncOperations.UpdateOperations != 1 {
		t.Errorf("Expected 1 update operation, got %d", snapshot.SyncOperations.UpdateOperations)
	}
	if snapshot.SyncOperations.DeleteOperations != 1 {
		t.Errorf("Expected 1 delete operation, got %d", snapshot.SyncOperations.DeleteOperations)
	}
	if snapshot.SyncOperations.RenameOperations != 1 {
		t.Errorf("Expected 1 rename operation, got %d", snapshot.SyncOperations.RenameOperations)
	}
	if snapshot.SyncOperations.FailedOperations != 1 {
		t.Errorf("Expected 1 failed operation, got %d", snapshot.SyncOperations.FailedOperations)
	}
	if snapshot.SyncOperations.BytesSynced != 3000 {
		t.Errorf("Expected 3000 bytes synced, got %d", snapshot.SyncOperations.BytesSynced)
	}

	t.Log("SUCCESS: Multiple operations recorded correctly")
}

func TestNetworkMetrics(t *testing.T) {
	m := monitoring.NewMetrics()

	m.RecordNetworkTraffic(1000, 500)
	m.RecordMessage(true)  // Sent
	m.RecordMessage(false) // Received
	m.RecordChunk(true)
	m.RecordConnection(true)
	m.RecordConnection(false) // Failed
	m.SetActiveConnections(3)

	snapshot := m.GetSnapshot()
	if snapshot.Network.BytesSent != 1000 {
		t.Errorf("Expected 1000 bytes sent, got %d", snapshot.Network.BytesSent)
	}
	if snapshot.Network.BytesReceived != 500 {
		t.Errorf("Expected 500 bytes received, got %d", snapshot.Network.BytesReceived)
	}
	if snapshot.Network.MessagesSent != 1 {
		t.Errorf("Expected 1 message sent, got %d", snapshot.Network.MessagesSent)
	}
	if snapshot.Network.MessagesReceived != 1 {
		t.Errorf("Expected 1 message received, got %d", snapshot.Network.MessagesReceived)
	}
	if snapshot.Network.ChunksSent != 1 {
		t.Errorf("Expected 1 chunk sent, got %d", snapshot.Network.ChunksSent)
	}
	if snapshot.Network.ConnectionAttempts != 2 {
		t.Errorf("Expected 2 connection attempts, got %d", snapshot.Network.ConnectionAttempts)
	}
	if snapshot.Network.FailedConnections != 1 {
		t.Errorf("Expected 1 failed connection, got %d", snapshot.Network.FailedConnections)
	}
	if snapshot.Network.ActiveConnections != 3 {
		t.Errorf("Expected 3 active connections, got %d", snapshot.Network.ActiveConnections)
	}

	t.Log("SUCCESS: Network metrics recorded correctly")
}

func TestFlowControlMetrics(t *testing.T) {
	m := monitoring.NewMetrics()

	m.UpdateFlowControl(2, 5, 1000000) // 1MB/s
	m.RecordTransfer()
	m.RecordThrottle()

	snapshot := m.GetSnapshot()
	if snapshot.FlowControl.ActiveTransfers != 2 {
		t.Errorf("Expected 2 active transfers, got %d", snapshot.FlowControl.ActiveTransfers)
	}
	if snapshot.FlowControl.QueuedTransfers != 5 {
		t.Errorf("Expected 5 queued transfers, got %d", snapshot.FlowControl.QueuedTransfers)
	}
	if snapshot.FlowControl.BandwidthUsage != 1000000 {
		t.Errorf("Expected 1000000 bandwidth usage, got %d", snapshot.FlowControl.BandwidthUsage)
	}
	if snapshot.FlowControl.TotalTransfers != 1 {
		t.Errorf("Expected 1 total transfer, got %d", snapshot.FlowControl.TotalTransfers)
	}
	if snapshot.FlowControl.ThrottledOperations != 1 {
		t.Errorf("Expected 1 throttled operation, got %d", snapshot.FlowControl.ThrottledOperations)
	}

	t.Log("SUCCESS: Flow control metrics recorded correctly")
}

func TestPeerMetrics(t *testing.T) {
	m := monitoring.NewMetrics()

	m.RecordPeerConnect("peer1")
	m.RecordPeerConnect("peer2")
	m.RecordPeerDisconnect("peer1")

	snapshot := m.GetSnapshot()
	if snapshot.Peers.ConnectedPeers != 1 {
		t.Errorf("Expected 1 connected peer, got %d", snapshot.Peers.ConnectedPeers)
	}
	if snapshot.Peers.TotalPeersDiscovered != 2 {
		t.Errorf("Expected 2 total peers discovered, got %d", snapshot.Peers.TotalPeersDiscovered)
	}
	if snapshot.Peers.PeerDisconnects != 1 {
		t.Errorf("Expected 1 peer disconnect, got %d", snapshot.Peers.PeerDisconnects)
	}

	t.Log("SUCCESS: Peer metrics recorded correctly")
}

func TestConflictMetrics(t *testing.T) {
	m := monitoring.NewMetrics()

	m.RecordConflict("lww")
	m.RecordConflict("3way")
	m.RecordConflictResolution()

	snapshot := m.GetSnapshot()
	if snapshot.Conflicts.TotalConflicts != 2 {
		t.Errorf("Expected 2 total conflicts, got %d", snapshot.Conflicts.TotalConflicts)
	}
	if snapshot.Conflicts.LWWResolutions != 1 {
		t.Errorf("Expected 1 LWW resolution, got %d", snapshot.Conflicts.LWWResolutions)
	}
	if snapshot.Conflicts.ThreeWayMerges != 1 {
		t.Errorf("Expected 1 3-way merge, got %d", snapshot.Conflicts.ThreeWayMerges)
	}
	if snapshot.Conflicts.ResolvedConflicts != 1 {
		t.Errorf("Expected 1 resolved conflict, got %d", snapshot.Conflicts.ResolvedConflicts)
	}
	if snapshot.Conflicts.PendingConflicts != 1 {
		t.Errorf("Expected 1 pending conflict, got %d", snapshot.Conflicts.PendingConflicts)
	}

	t.Log("SUCCESS: Conflict metrics recorded correctly")
}

func TestErrorMetrics(t *testing.T) {
	m := monitoring.NewMetrics()

	m.RecordError("network", "connection timeout")
	m.RecordError("filesystem", "file not found")

	snapshot := m.GetSnapshot()
	if snapshot.Errors.TotalErrors != 2 {
		t.Errorf("Expected 2 total errors, got %d", snapshot.Errors.TotalErrors)
	}
	if snapshot.Errors.NetworkErrors != 1 {
		t.Errorf("Expected 1 network error, got %d", snapshot.Errors.NetworkErrors)
	}
	if snapshot.Errors.FileSystemErrors != 1 {
		t.Errorf("Expected 1 filesystem error, got %d", snapshot.Errors.FileSystemErrors)
	}
	if snapshot.Errors.LastError != "file not found" {
		t.Errorf("Expected last error 'file not found', got '%s'", snapshot.Errors.LastError)
	}

	t.Log("SUCCESS: Error metrics recorded correctly")
}

func TestMetricsSummary(t *testing.T) {
	m := monitoring.NewMetrics()

	m.RecordSyncOperation("create", 1000, true)
	m.RecordNetworkTraffic(500, 300)
	m.RecordPeerConnect("peer1")

	// Wait a moment for uptime
	time.Sleep(10 * time.Millisecond)

	snapshot := m.GetSnapshot()
	summary := snapshot.GetSummary()

	if summary["total_operations"].(int64) != 1 {
		t.Errorf("Expected 1 total operation in summary")
	}
	if summary["bytes_synced"].(int64) != 1000 {
		t.Errorf("Expected 1000 bytes synced in summary")
	}
	if summary["connected_peers"].(int64) != 1 {
		t.Errorf("Expected 1 connected peer in summary")
	}

	uptimeSecs := summary["uptime_seconds"].(float64)
	if uptimeSecs < 0.01 {
		t.Errorf("Expected uptime > 0.01 seconds, got %f", uptimeSecs)
	}

	t.Log("SUCCESS: Metrics summary generated correctly")
}

func TestMonitoringServer(t *testing.T) {
	m := monitoring.NewMetrics()
	m.RecordSyncOperation("create", 1024, true)

	server := monitoring.NewServer(m, 0) // Port 0 for test
	server.Start()
	defer server.Stop()

	// Give server time to start
	time.Sleep(10 * time.Millisecond)

	// We can't test the actual HTTP server easily without a real HTTP client
	// This test just verifies the server can be started/stopped without errors

	t.Log("SUCCESS: Monitoring server initialized")
}

func TestMetricsSnapshot(t *testing.T) {
	m := monitoring.NewMetrics()

	// Record various metrics
	m.RecordSyncOperation("create", 1000, true)
	m.RecordNetworkTraffic(500, 300)
	m.RecordPeerConnect("peer1")

	// Get snapshot
	snapshot1 := m.GetSnapshot()

	// Record more metrics
	m.RecordSyncOperation("update", 2000, true)

	// Get another snapshot
	snapshot2 := m.GetSnapshot()

	// Snapshots should be independent
	if snapshot1.SyncOperations.TotalOperations == snapshot2.SyncOperations.TotalOperations {
		t.Error("Snapshots should differ after recording more operations")
	}

	if snapshot1.SyncOperations.TotalOperations != 1 {
		t.Errorf("First snapshot should have 1 operation, got %d", snapshot1.SyncOperations.TotalOperations)
	}
	if snapshot2.SyncOperations.TotalOperations != 2 {
		t.Errorf("Second snapshot should have 2 operations, got %d", snapshot2.SyncOperations.TotalOperations)
	}

	t.Log("SUCCESS: Metrics snapshots work correctly")
}

func TestConcurrentMetrics(t *testing.T) {
	m := monitoring.NewMetrics()

	// Concurrent writes
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				m.RecordSyncOperation("create", 100, true)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	snapshot := m.GetSnapshot()
	if snapshot.SyncOperations.TotalOperations != 1000 {
		t.Errorf("Expected 1000 total operations, got %d", snapshot.SyncOperations.TotalOperations)
	}

	t.Log("SUCCESS: Concurrent metrics recording works correctly")
}

func TestMetricsJSON(t *testing.T) {
	m := monitoring.NewMetrics()
	m.RecordSyncOperation("create", 1024, true)

	snapshot := m.GetSnapshot()

	// Test JSON encoding
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("Failed to marshal snapshot: %v", err)
	}

	// Test JSON decoding
	var decoded monitoring.MetricsSnapshot
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal snapshot: %v", err)
	}

	if decoded.SyncOperations.TotalOperations != 1 {
		t.Errorf("Expected 1 operation after unmarshal, got %d", decoded.SyncOperations.TotalOperations)
	}

	t.Log("SUCCESS: Metrics JSON serialization works correctly")
}
