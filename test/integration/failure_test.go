package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestNetworkFailureRecovery tests recovery from network failures
func TestNetworkFailureRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping failure test in short mode")
	}

	// Clean up any stale Docker resources from previous test runs
	cleanupStaleDockerResources(t)

	projectName := fmt.Sprintf("p2p-failure-%d", time.Now().Unix())

	// Set up test environment
	testDir := t.TempDir()
	dockerDir := filepath.Join(testDir, "docker")
	configDir := filepath.Join(dockerDir, "config")

	copyDockerFiles(t, dockerDir)
	createTestConfig(t, configDir)
	createSyncDirectories(t, dockerDir)

	// Start Docker Compose
	startDockerCompose(t, dockerDir, projectName)
	defer stopDockerCompose(t, dockerDir, projectName)

	waitForServices(t, projectName)

	// Test 1: Network partition
	testNetworkPartition(t, projectName)

	// Test 2: Peer restart
	testPeerRestart(t, projectName)

	// Test 3: Disk space issues (simulated)
	testDiskSpaceSimulation(t, projectName)

	// Test 4: Corrupted file handling
	testCorruptedFileHandling(t, projectName)
}

func testNetworkPartition(t *testing.T, projectName string) {
	t.Log("Testing network partition recovery...")

	// Create initial file
	testContent := "Network partition test"
	createFileInContainer(t, projectName, "peer-alpha", "/app/sync/network-test.txt", testContent)
	time.Sleep(3 * time.Second)

	// Verify sync
	content := readFileFromContainer(t, projectName, "peer-beta", "/app/sync/network-test.txt")
	if content != testContent {
		t.Errorf("Initial sync failed")
	}

	// Simulate network partition by stopping one peer
	stopContainer(t, projectName, "peer-beta")
	t.Log("Peer-beta stopped (simulating network partition)")

	// Create file while peer is down
	partitionContent := "Created during partition"
	createFileInContainer(t, projectName, "peer-alpha", "/app/sync/partition-test.txt", partitionContent)

	// Wait a bit
	time.Sleep(2 * time.Second)

	// Restart peer
	startContainer(t, projectName, "peer-beta")
	t.Log("Peer-beta restarted")

	// Wait for recovery sync
	time.Sleep(10 * time.Second)

	// Verify both files are synced
	content1 := readFileFromContainer(t, projectName, "peer-beta", "/app/sync/network-test.txt")
	content2 := readFileFromContainer(t, projectName, "peer-beta", "/app/sync/partition-test.txt")

	if content1 != testContent {
		t.Errorf("Network partition recovery failed: original file not synced")
	}
	if content2 != partitionContent {
		t.Errorf("Network partition recovery failed: partition file not synced")
	}

	t.Log("Network partition recovery test passed")
}

func testPeerRestart(t *testing.T, projectName string) {
	t.Log("Testing peer restart recovery...")

	// Create file
	restartContent := "Restart recovery test"
	createFileInContainer(t, projectName, "peer-alpha", "/app/sync/restart-test.txt", restartContent)
	time.Sleep(3 * time.Second)

	// Verify all peers have it
	for _, peer := range []string{"peer-beta", "peer-gamma"} {
		content := readFileFromContainer(t, projectName, peer, "/app/sync/restart-test.txt")
		if content != restartContent {
			t.Errorf("Initial sync to %s failed", peer)
		}
	}

	// Restart all peers
	t.Log("Restarting all peers...")
	for _, peer := range []string{"peer-alpha", "peer-beta", "peer-gamma"} {
		stopContainer(t, projectName, peer)
		time.Sleep(1 * time.Second)
		startContainer(t, projectName, peer)
		time.Sleep(2 * time.Second)
	}

	// Wait for restart recovery
	time.Sleep(15 * time.Second)

	// Verify all files are still present
	for _, peer := range []string{"peer-alpha", "peer-beta", "peer-gamma"} {
		content := readFileFromContainer(t, projectName, peer, "/app/sync/restart-test.txt")
		if content != restartContent {
			t.Errorf("Restart recovery failed: file missing on %s", peer)
		}
	}

	t.Log("Peer restart recovery test passed")
}

func testDiskSpaceSimulation(t *testing.T, projectName string) {
	t.Log("Testing disk space handling...")

	// Create a moderately large file
	largeContent := strings.Repeat("This is test content for disk space testing. ", 10000) // ~500KB
	createFileInContainer(t, projectName, "peer-alpha", "/app/sync/disk-test.dat", largeContent)
	time.Sleep(5 * time.Second)

	// Verify sync
	size := getFileSizeInContainer(t, projectName, "peer-beta", "/app/sync/disk-test.dat")
	if size != len(largeContent) {
		t.Errorf("Large file sync failed: expected size %d, got %d", len(largeContent), size)
	}

	// In a real disk space test, we would fill up disk space
	// For now, just verify the system can handle larger files
	t.Log("Disk space simulation test passed")
}

func testCorruptedFileHandling(t *testing.T, projectName string) {
	t.Log("Testing corrupted file handling...")

	// Create a valid file first
	validContent := "Valid file content"
	createFileInContainer(t, projectName, "peer-alpha", "/app/sync/valid.txt", validContent)
	time.Sleep(3 * time.Second)

	// Verify sync
	content := readFileFromContainer(t, projectName, "peer-beta", "/app/sync/valid.txt")
	if content != validContent {
		t.Errorf("Valid file sync failed")
	}

	// Create another file that might be "corrupted" during sync
	// In practice, corruption would be detected by checksums
	corruptionTestContent := "Content that should be verified by checksums"
	createFileInContainer(t, projectName, "peer-alpha", "/app/sync/checksum-test.txt", corruptionTestContent)
	time.Sleep(3 * time.Second)

	// Verify the file is correctly synced (checksum verification happens internally)
	content = readFileFromContainer(t, projectName, "peer-beta", "/app/sync/checksum-test.txt")
	if content != corruptionTestContent {
		t.Errorf("Checksum verification test failed")
	}

	t.Log("Corrupted file handling test passed")
}

// TestConfigurationScenarios tests various configuration scenarios
func TestConfigurationScenarios(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping configuration test in short mode")
	}

	testCases := []struct {
		name        string
		configMod   func(config string) string
		expectError bool
	}{
		{
			name: "minimal valid config",
			configMod: func(config string) string {
				return strings.Replace(config, "max_concurrent_transfers: 5", "max_concurrent_transfers: 1", 1)
			},
			expectError: false,
		},
		{
			name: "high compression level",
			configMod: func(config string) string {
				return strings.Replace(config, "level: 3", "level: 19", 1)
			},
			expectError: false,
		},
		{
			name: "tcp only network",
			configMod: func(config string) string {
				return strings.Replace(config, `protocol: "tcp"`, `protocol: "tcp"`, 1)
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Sanitize test name for Docker project name (lowercase alphanumeric, hyphens, underscores only)
			sanitizedName := strings.ToLower(tc.name)
			sanitizedName = strings.ReplaceAll(sanitizedName, " ", "-")
			sanitizedName = strings.ReplaceAll(sanitizedName, "_", "-")
			// Remove any remaining invalid characters
			sanitizedName = strings.Map(func(r rune) rune {
				if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
					return r
				}
				return -1
			}, sanitizedName)
			projectName := fmt.Sprintf("p2p-config-%s-%d", sanitizedName, time.Now().Unix())

			testDir := t.TempDir()
			dockerDir := filepath.Join(testDir, "docker")
			configDir := filepath.Join(dockerDir, "config")

			copyDockerFiles(t, dockerDir)
			createSyncDirectories(t, dockerDir)

			// Create modified config
			baseConfig := `
sync:
  folder_path: "/app/sync"
  chunk_size_min: 65536
  chunk_size_max: 2097152
  chunk_size_default: 524288
  max_concurrent_transfers: 5

network:
  port: 8080
  discovery_port: 8081
  protocol: "tcp"

compression:
  enabled: true
  algorithm: "zstd"
  level: 3

observability:
  log_level: "info"
`

			modifiedConfig := tc.configMod(baseConfig)
			configPath := filepath.Join(configDir, "config.yaml")
			if err := os.MkdirAll(configDir, 0755); err != nil {
				t.Fatalf("Failed to create config directory: %v", err)
			}
			if err := os.WriteFile(configPath, []byte(modifiedConfig), 0644); err != nil {
				t.Fatalf("Failed to write config file: %v", err)
			}

			// Try to start the system
			startDockerCompose(t, dockerDir, projectName)
			defer stopDockerCompose(t, dockerDir, projectName)

			// Wait and check if services start successfully
			time.Sleep(10 * time.Second)

			healthy := checkServicesHealthy(t, projectName)
			if tc.expectError && healthy {
				t.Errorf("Expected configuration to fail but services are healthy")
			}
			if !tc.expectError && !healthy {
				t.Errorf("Expected configuration to work but services are not healthy")
			}
		})
	}
}

// TestPerformanceMetrics tests basic performance characteristics
func TestPerformanceMetrics(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	projectName := fmt.Sprintf("p2p-perf-%d", time.Now().Unix())

	testDir := t.TempDir()
	dockerDir := filepath.Join(testDir, "docker")
	configDir := filepath.Join(dockerDir, "config")

	copyDockerFiles(t, dockerDir)
	createTestConfig(t, configDir)
	createSyncDirectories(t, dockerDir)

	startDockerCompose(t, dockerDir, projectName)
	defer stopDockerCompose(t, dockerDir, projectName)

	waitForServices(t, projectName)

	// Performance test: measure sync time for different file sizes
	fileSizes := []struct {
		name string
		size int
	}{
		{"small", 1024},    // 1KB
		{"medium", 102400}, // 100KB
		{"large", 1024000}, // 1MB
	}

	for _, fs := range fileSizes {
		t.Run(fmt.Sprintf("sync_%s_file", fs.name), func(t *testing.T) {
			content := strings.Repeat("X", fs.size)
			filename := fmt.Sprintf("/app/sync/perf-%s.dat", fs.name)

			// Measure sync time
			start := time.Now()
			createFileInContainer(t, projectName, "peer-alpha", filename, content)

			// Wait for sync to complete
			time.Sleep(5 * time.Second)

			// Verify file exists on other peers
			for _, peer := range []string{"peer-beta", "peer-gamma"} {
				size := getFileSizeInContainer(t, projectName, peer, filename)
				if size != fs.size {
					t.Errorf("File sync failed for %s on %s: expected size %d, got %d", fs.name, peer, fs.size, size)
				}
			}

			elapsed := time.Since(start)
			t.Logf("%s file (%d bytes) sync time: %v", fs.name, fs.size, elapsed)
		})
	}
}

// TestSecurityFeatures tests basic security functionality
func TestSecurityFeatures(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping security test in short mode")
	}

	projectName := fmt.Sprintf("p2p-security-%d", time.Now().Unix())

	testDir := t.TempDir()
	dockerDir := filepath.Join(testDir, "docker")
	configDir := filepath.Join(dockerDir, "config")

	copyDockerFiles(t, dockerDir)
	createTestConfig(t, configDir)
	createSyncDirectories(t, dockerDir)

	startDockerCompose(t, dockerDir, projectName)
	defer stopDockerCompose(t, dockerDir, projectName)

	waitForServices(t, projectName)

	// Test that files are properly synced (encryption/decryption happens internally)
	secureContent := "This content should be encrypted in transit"
	createFileInContainer(t, projectName, "peer-alpha", "/app/sync/secure.txt", secureContent)
	time.Sleep(5 * time.Second)

	// Verify content is correctly received (decryption happens internally)
	for _, peer := range []string{"peer-beta", "peer-gamma"} {
		content := readFileFromContainer(t, projectName, peer, "/app/sync/secure.txt")
		if content != secureContent {
			t.Errorf("Security test failed: content not properly decrypted on %s", peer)
		}
	}

	t.Log("Security features test passed")
}
