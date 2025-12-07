package system

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestSessionKeyEstablishment verifies that session keys are properly established
// between peers on both sides of the connection
func TestSessionKeyEstablishment(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping system test in short mode")
	}

	// Create three peers (alpha, beta, gamma) to test the exact scenario from the bug
	nth := NewNetworkTestHelper(t)

	alpha, err := nth.SetupPeer(t, "alpha", true)
	if err != nil {
		t.Fatalf("Failed to setup alpha: %v", err)
	}
	defer alpha.Cleanup()

	beta, err := nth.SetupPeer(t, "beta", true)
	if err != nil {
		t.Fatalf("Failed to setup beta: %v", err)
	}
	defer beta.Cleanup()

	gamma, err := nth.SetupPeer(t, "gamma", true)
	if err != nil {
		t.Fatalf("Failed to setup gamma: %v", err)
	}
	defer gamma.Cleanup()

	// Start all transports and sync engines
	if err := alpha.Transport.Start(); err != nil {
		t.Fatalf("Failed to start alpha transport: %v", err)
	}
	if err := beta.Transport.Start(); err != nil {
		t.Fatalf("Failed to start beta transport: %v", err)
	}
	if err := gamma.Transport.Start(); err != nil {
		t.Fatalf("Failed to start gamma transport: %v", err)
	}

	ctx := context.Background()
	if err := alpha.SyncEngine.Start(ctx); err != nil {
		t.Fatalf("Failed to start alpha sync engine: %v", err)
	}
	if err := beta.SyncEngine.Start(ctx); err != nil {
		t.Fatalf("Failed to start beta sync engine: %v", err)
	}
	if err := gamma.SyncEngine.Start(ctx); err != nil {
		t.Fatalf("Failed to start gamma sync engine: %v", err)
	}

	// Connect beta to alpha and gamma (beta is in the middle)
	t.Log("Connecting beta to alpha...")
	if err := beta.Messenger.ConnectToPeer(alpha.ID, "localhost", alpha.Config.Network.Port); err != nil {
		t.Fatalf("Failed to connect beta to alpha: %v", err)
	}

	t.Log("Connecting beta to gamma...")
	if err := beta.Messenger.ConnectToPeer(gamma.ID, "localhost", gamma.Config.Network.Port); err != nil {
		t.Fatalf("Failed to connect beta to gamma: %v", err)
	}

	// Wait for connections to stabilize
	time.Sleep(2 * time.Second)

	// Verify session keys are established on beta's side
	t.Log("Verifying session keys on beta's side...")
	alphaKeyFromBeta, err := beta.ConnManager.GetSessionKey(alpha.ID)
	if err != nil {
		t.Fatalf("Beta doesn't have session key for alpha: %v", err)
	}
	if len(alphaKeyFromBeta) == 0 {
		t.Fatal("Beta's session key for alpha is empty")
	}

	gammaKeyFromBeta, err := beta.ConnManager.GetSessionKey(gamma.ID)
	if err != nil {
		t.Fatalf("Beta doesn't have session key for gamma: %v", err)
	}
	if len(gammaKeyFromBeta) == 0 {
		t.Fatal("Beta's session key for gamma is empty")
	}

	// Verify session keys are established on the other peers' sides
	t.Log("Verifying session keys on alpha's side...")
	betaKeyFromAlpha, err := alpha.ConnManager.GetSessionKey(beta.ID)
	if err != nil {
		t.Fatalf("Alpha doesn't have session key for beta: %v", err)
	}
	if len(betaKeyFromAlpha) == 0 {
		t.Fatal("Alpha's session key for beta is empty")
	}

	t.Log("Verifying session keys on gamma's side...")
	betaKeyFromGamma, err := gamma.ConnManager.GetSessionKey(beta.ID)
	if err != nil {
		t.Fatalf("Gamma doesn't have session key for beta: %v", err)
	}
	if len(betaKeyFromGamma) == 0 {
		t.Fatal("Gamma's session key for beta is empty")
	}

	// Verify that the session keys are symmetric (both sides have the same key)
	t.Log("Verifying session keys are symmetric...")
	if !bytesEqual(alphaKeyFromBeta, betaKeyFromAlpha) {
		t.Fatal("Session keys between alpha and beta don't match (not symmetric)")
	}

	if !bytesEqual(gammaKeyFromBeta, betaKeyFromGamma) {
		t.Fatal("Session keys between gamma and beta don't match (not symmetric)")
	}

	t.Log("Session key establishment test passed!")
}

// TestSessionKeyFileBroadcast tests the exact scenario from the bug:
// Beta creates a file and tries to broadcast it to alpha and gamma
func TestSessionKeyFileBroadcast(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping system test in short mode")
	}

	// Create three peers
	nth := NewNetworkTestHelper(t)

	alpha, err := nth.SetupPeer(t, "alpha", true)
	if err != nil {
		t.Fatalf("Failed to setup alpha: %v", err)
	}
	defer alpha.Cleanup()

	beta, err := nth.SetupPeer(t, "beta", true)
	if err != nil {
		t.Fatalf("Failed to setup beta: %v", err)
	}
	defer beta.Cleanup()

	gamma, err := nth.SetupPeer(t, "gamma", true)
	if err != nil {
		t.Fatalf("Failed to setup gamma: %v", err)
	}
	defer gamma.Cleanup()

	// Start all transports and sync engines
	if err := alpha.Transport.Start(); err != nil {
		t.Fatalf("Failed to start alpha transport: %v", err)
	}
	if err := beta.Transport.Start(); err != nil {
		t.Fatalf("Failed to start beta transport: %v", err)
	}
	if err := gamma.Transport.Start(); err != nil {
		t.Fatalf("Failed to start gamma transport: %v", err)
	}

	ctx := context.Background()
	if err := alpha.SyncEngine.Start(ctx); err != nil {
		t.Fatalf("Failed to start alpha sync engine: %v", err)
	}
	if err := beta.SyncEngine.Start(ctx); err != nil {
		t.Fatalf("Failed to start beta sync engine: %v", err)
	}
	if err := gamma.SyncEngine.Start(ctx); err != nil {
		t.Fatalf("Failed to start gamma sync engine: %v", err)
	}

	// Connect beta to alpha and gamma
	t.Log("Setting up peer connections...")
	if err := beta.Messenger.ConnectToPeer(alpha.ID, "localhost", alpha.Config.Network.Port); err != nil {
		t.Fatalf("Failed to connect beta to alpha: %v", err)
	}
	if err := beta.Messenger.ConnectToPeer(gamma.ID, "localhost", gamma.Config.Network.Port); err != nil {
		t.Fatalf("Failed to connect beta to gamma: %v", err)
	}

	// Wait for connections to stabilize
	time.Sleep(2 * time.Second)

	// Create a file on beta (this was the scenario that triggered the bug)
	testFile := "then.txt"
	testContent := "test"
	testPath := filepath.Join(beta.Config.Sync.FolderPath, testFile)

	t.Logf("Creating test file on beta: %s", testPath)
	if err := os.WriteFile(testPath, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Wait for file to be detected and synchronized
	t.Log("Waiting for file synchronization...")
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	alphaReceived := false
	gammaReceived := false

	for !alphaReceived || !gammaReceived {
		select {
		case <-timeout:
			if !alphaReceived {
				t.Error("Timeout waiting for file to sync to alpha")
			}
			if !gammaReceived {
				t.Error("Timeout waiting for file to sync to gamma")
			}
			t.Fatal("File broadcast failed - this indicates session key issue")
		case <-ticker.C:
			// Check if alpha received the file
			if !alphaReceived {
				alphaPath := filepath.Join(alpha.Config.Sync.FolderPath, testFile)
				if content, err := os.ReadFile(alphaPath); err == nil {
					if string(content) == testContent {
						alphaReceived = true
						t.Log("✓ Alpha successfully received file")
					}
				}
			}

			// Check if gamma received the file
			if !gammaReceived {
				gammaPath := filepath.Join(gamma.Config.Sync.FolderPath, testFile)
				if content, err := os.ReadFile(gammaPath); err == nil {
					if string(content) == testContent {
						gammaReceived = true
						t.Log("✓ Gamma successfully received file")
					}
				}
			}
		}
	}

	t.Log("File broadcast test passed! Beta successfully sent file to both alpha and gamma.")
}

// TestSessionKeyMultiHop tests that session keys work in a multi-hop scenario
func TestSessionKeyMultiHop(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping system test in short mode")
	}

	// Create four peers in a chain: alpha <-> beta <-> gamma <-> delta
	nth := NewNetworkTestHelper(t)

	alpha, err := nth.SetupPeer(t, "alpha", true)
	if err != nil {
		t.Fatalf("Failed to setup alpha: %v", err)
	}
	defer alpha.Cleanup()

	beta, err := nth.SetupPeer(t, "beta", true)
	if err != nil {
		t.Fatalf("Failed to setup beta: %v", err)
	}
	defer beta.Cleanup()

	gamma, err := nth.SetupPeer(t, "gamma", true)
	if err != nil {
		t.Fatalf("Failed to setup gamma: %v", err)
	}
	defer gamma.Cleanup()

	delta, err := nth.SetupPeer(t, "delta", true)
	if err != nil {
		t.Fatalf("Failed to setup delta: %v", err)
	}
	defer delta.Cleanup()

	// Start all transports and sync engines
	if err := alpha.Transport.Start(); err != nil {
		t.Fatalf("Failed to start alpha transport: %v", err)
	}
	if err := beta.Transport.Start(); err != nil {
		t.Fatalf("Failed to start beta transport: %v", err)
	}
	if err := gamma.Transport.Start(); err != nil {
		t.Fatalf("Failed to start gamma transport: %v", err)
	}
	if err := delta.Transport.Start(); err != nil {
		t.Fatalf("Failed to start delta transport: %v", err)
	}

	ctx := context.Background()
	if err := alpha.SyncEngine.Start(ctx); err != nil {
		t.Fatalf("Failed to start alpha sync engine: %v", err)
	}
	if err := beta.SyncEngine.Start(ctx); err != nil {
		t.Fatalf("Failed to start beta sync engine: %v", err)
	}
	if err := gamma.SyncEngine.Start(ctx); err != nil {
		t.Fatalf("Failed to start gamma sync engine: %v", err)
	}
	if err := delta.SyncEngine.Start(ctx); err != nil {
		t.Fatalf("Failed to start delta sync engine: %v", err)
	}

	// Connect peers in a chain
	t.Log("Setting up chain topology...")
	if err := beta.Messenger.ConnectToPeer(alpha.ID, "localhost", alpha.Config.Network.Port); err != nil {
		t.Fatalf("Failed to connect beta to alpha: %v", err)
	}
	if err := gamma.Messenger.ConnectToPeer(beta.ID, "localhost", beta.Config.Network.Port); err != nil {
		t.Fatalf("Failed to connect gamma to beta: %v", err)
	}
	if err := delta.Messenger.ConnectToPeer(gamma.ID, "localhost", gamma.Config.Network.Port); err != nil {
		t.Fatalf("Failed to connect delta to gamma: %v", err)
	}

	time.Sleep(2 * time.Second)

	// Verify all session keys are established
	t.Log("Verifying all session keys in chain...")
	if _, err := beta.ConnManager.GetSessionKey(alpha.ID); err != nil {
		t.Fatalf("Beta missing session key for alpha: %v", err)
	}
	if _, err := gamma.ConnManager.GetSessionKey(beta.ID); err != nil {
		t.Fatalf("Gamma missing session key for beta: %v", err)
	}
	if _, err := delta.ConnManager.GetSessionKey(gamma.ID); err != nil {
		t.Fatalf("Delta missing session key for gamma: %v", err)
	}

	// Create a file on alpha and verify it propagates through the chain
	testFile := "chain-test.txt"
	testContent := "propagating through chain"
	testPath := filepath.Join(alpha.Config.Sync.FolderPath, testFile)

	t.Log("Creating test file on alpha...")
	if err := os.WriteFile(testPath, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Wait for file to propagate through the entire chain
	t.Log("Waiting for file to propagate through chain...")
	if !waitForFileSync(t, beta.Config.Sync.FolderPath, testFile, testContent, 30*time.Second) {
		t.Fatal("File didn't reach beta")
	}
	t.Log("✓ File reached beta")

	if !waitForFileSync(t, gamma.Config.Sync.FolderPath, testFile, testContent, 30*time.Second) {
		t.Fatal("File didn't reach gamma")
	}
	t.Log("✓ File reached gamma")

	if !waitForFileSync(t, delta.Config.Sync.FolderPath, testFile, testContent, 30*time.Second) {
		t.Fatal("File didn't reach delta")
	}
	t.Log("✓ File reached delta")

	t.Log("Multi-hop session key test passed!")
}

// Helper function to check if two byte slices are equal
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Helper function to wait for a file to sync
func waitForFileSync(t *testing.T, folderPath, fileName, expectedContent string, timeout time.Duration) bool {
	deadline := time.After(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			return false
		case <-ticker.C:
			filePath := filepath.Join(folderPath, fileName)
			if content, err := os.ReadFile(filePath); err == nil {
				if string(content) == expectedContent {
					return true
				}
			}
		}
	}
}
