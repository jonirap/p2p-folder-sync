package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDockerComposeSystem tests the complete P2P sync system using Docker Compose
func TestDockerComposeSystem(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Docker system test in short mode")
	}

	// Check if Docker is available
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Docker not available, skipping system test")
	}

	// Check if Docker Compose is available
	if _, err := exec.LookPath("docker-compose"); err != nil {
		t.Skip("Docker Compose not available, skipping system test")
	}

	// Clean up any stale Docker resources from previous test runs
	cleanupStaleDockerResources(t)

	projectName := fmt.Sprintf("p2p-test-%d", time.Now().Unix())

	// Set up test environment
	testDir := t.TempDir()
	dockerDir := filepath.Join(testDir, "docker")
	configDir := filepath.Join(dockerDir, "config")

	// Copy docker files
	copyDockerFiles(t, dockerDir)

	// Create test configuration
	createTestConfig(t, configDir)

	// Create sync directories
	createSyncDirectories(t, dockerDir)

	// Start Docker Compose environment
	startDockerCompose(t, dockerDir, projectName)
	defer stopDockerCompose(t, dockerDir, projectName)

	// Wait for services to be ready
	waitForServices(t, projectName)

	// Test 1: Basic file synchronization
	testBasicFileSync(t, projectName)

	// Test 2: Peer discovery
	testPeerDiscovery(t, projectName)

	// Test 3: Large file handling
	testLargeFileSync(t, projectName)

	// Test 4: Concurrent operations
	testConcurrentOperations(t, projectName)

	// Test 5: Failure recovery
	testFailureRecovery(t, projectName)

	// Test 6: Conflict resolution
	testConflictResolution(t, projectName)
}

func copyDockerFiles(t *testing.T, destDir string) {
	srcDir := "/home/jonirap/p2p-folder-sync/p2p-folder-sync"
	projectDir := "/home/jonirap/p2p-folder-sync/p2p-folder-sync"

	// Copy docker files
	dockerFiles := []string{"docker-compose.yml", "Dockerfile"}
	for _, file := range dockerFiles {
		srcPath := filepath.Join(srcDir, file)
		destPath := filepath.Join(destDir, file)

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}

		content, err := os.ReadFile(srcPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", srcPath, err)
		}

		if err := os.WriteFile(destPath, content, 0644); err != nil {
			t.Fatalf("Failed to write %s: %v", destPath, err)
		}
	}

	// Copy Go module files needed for Docker build
	goFiles := []string{"go.mod", "go.sum"}
	for _, file := range goFiles {
		srcPath := filepath.Join(projectDir, file)
		destPath := filepath.Join(destDir, file)

		content, err := os.ReadFile(srcPath)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", srcPath, err)
		}

		if err := os.WriteFile(destPath, content, 0644); err != nil {
			t.Fatalf("Failed to write %s: %v", destPath, err)
		}
	}

	// Copy source code directory
	if err := copyDir(filepath.Join(projectDir, "cmd"), filepath.Join(destDir, "cmd")); err != nil {
		t.Fatalf("Failed to copy cmd directory: %v", err)
	}
	if err := copyDir(filepath.Join(projectDir, "internal"), filepath.Join(destDir, "internal")); err != nil {
		t.Fatalf("Failed to copy internal directory: %v", err)
	}
	if err := copyDir(filepath.Join(projectDir, "pkg"), filepath.Join(destDir, "pkg")); err != nil {
		t.Fatalf("Failed to copy pkg directory: %v", err)
	}
}

func createTestConfig(t *testing.T, configDir string) {
	config := `
sync:
  folder_path: "/app/sync"
  chunk_size_min: 65536
  chunk_size_max: 2097152
  chunk_size_default: 524288
  max_concurrent_transfers: 5
  operation_log_size: 10000

network:
  port: 8080
  discovery_port: 8081
  heartbeat_interval: 30
  connection_timeout: 60
  protocol: "tcp"

security:
  key_rotation_interval: 86400
  encryption_algorithm: "aes-256-gcm"

compression:
  enabled: true
  file_size_threshold: 1048576
  algorithm: "zstd"
  level: 3
  chunk_compression: true

conflict:
  resolution_strategy: "intelligent_merge"

observability:
  log_level: "info"
  metrics_enabled: true
  tracing_enabled: false
`

	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}

	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}
}

func createSyncDirectories(t *testing.T, dockerDir string) {
	dirs := []string{
		"sync-data-alpha",
		"sync-data-beta",
		"sync-data-gamma",
	}

	for _, dir := range dirs {
		dirPath := filepath.Join(dockerDir, dir)
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			t.Fatalf("Failed to create sync directory %s: %v", dir, err)
		}
	}
}

func startDockerCompose(t *testing.T, dockerDir, projectName string) {
	cmd := exec.Command("docker-compose",
		"-f", filepath.Join(dockerDir, "docker-compose.yml"),
		"-p", projectName,
		"up", "-d")

	cmd.Dir = dockerDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to start Docker Compose: %v\nOutput: %s", err, output)
	}

	t.Logf("Docker Compose started successfully")
}

func stopDockerCompose(t *testing.T, dockerDir, projectName string) {
	cmd := exec.Command("docker-compose",
		"-f", filepath.Join(dockerDir, "docker-compose.yml"),
		"-p", projectName,
		"down", "-v")

	cmd.Dir = dockerDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Failed to stop Docker Compose: %v\nOutput: %s", err, output)
	} else {
		t.Logf("Docker Compose stopped successfully")
	}
}

func waitForServices(t *testing.T, projectName string) {
	// Wait for containers to be healthy
	maxRetries := 30
	retryDelay := 2 * time.Second

	for i := 0; i < maxRetries; i++ {
		if checkServicesHealthy(t, projectName) {
			t.Logf("All services are healthy after %d attempts", i+1)
			return
		}
		time.Sleep(retryDelay)
	}

	t.Fatal("Services did not become healthy within timeout")
}

func checkServicesHealthy(t *testing.T, projectName string) bool {
	// First check container names
	cmd := exec.Command("docker", "ps",
		"--filter", fmt.Sprintf("label=com.docker.compose.project=%s", projectName),
		"--format", "{{.Names}}")

	namesOutput, err := cmd.Output()
	if err != nil {
		t.Logf("Failed to check container names: %v", err)
		return false
	}

	containerNames := strings.Split(strings.TrimSpace(string(namesOutput)), "\n")
	t.Logf("Running containers for project %s: %s", projectName, string(namesOutput))

	cmd = exec.Command("docker", "ps",
		"--filter", fmt.Sprintf("label=com.docker.compose.project=%s", projectName),
		"--format", "{{.Status}}")

	output, err := cmd.Output()
	if err != nil {
		t.Logf("Failed to check container status: %v", err)
		return false
	}

	status := string(output)
	lines := strings.Split(strings.TrimSpace(status), "\n")

	if len(lines) < 3 {
		t.Logf("Not all containers are running. Expected 3, got %d. Status: %s", len(lines), status)
		return false
	}

	// Check if all containers are healthy/running
	for i, line := range lines {
		if !strings.Contains(line, "Up") {
			t.Logf("Container not healthy: %s", line)
			// Log container logs to debug why it's not running
			if i < len(containerNames) {
				logCmd := exec.Command("docker", "logs", containerNames[i])
				if logs, err := logCmd.Output(); err == nil {
					t.Logf("Container %s logs: %s", containerNames[i], string(logs))
				}
			}
			return false
		}
	}

	return true
}

func testBasicFileSync(t *testing.T, projectName string) {
	t.Log("Testing basic file synchronization...")

	// Wait for peers to establish connections
	t.Log("Waiting for peers to connect...")
	time.Sleep(3 * time.Second)

	// Create a test file in peer-alpha
	testContent := "Hello from peer-alpha!"
	createFileInContainer(t, projectName, "peer-alpha", "/app/sync/test.txt", testContent)

	// Wait for sync
	time.Sleep(5 * time.Second)

	// Check if file exists in peer-beta
	defer func() {
		if t.Failed() {
			t.Log("=== peer-alpha logs ===")
			t.Log(getContainerLogs(t, projectName, "peer-alpha", 50))
			t.Log("=== peer-beta logs ===")
			t.Log(getContainerLogs(t, projectName, "peer-beta", 50))
			t.Log("=== peer-gamma logs ===")
			t.Log(getContainerLogs(t, projectName, "peer-gamma", 50))
		}
	}()

	content := readFileFromContainer(t, projectName, "peer-beta", "/app/sync/test.txt")
	if content != testContent {
		t.Errorf("File sync failed: expected %q, got %q", testContent, content)
	}

	// Check if file exists in peer-gamma
	content = readFileFromContainer(t, projectName, "peer-gamma", "/app/sync/test.txt")
	if content != testContent {
		t.Errorf("File sync failed: expected %q, got %q", testContent, content)
	}

	t.Log("Basic file synchronization test passed")
}

func testPeerDiscovery(t *testing.T, projectName string) {
	t.Log("Testing peer discovery...")

	// Check that all peers can discover each other
	// This would typically involve checking logs or API endpoints
	// For now, we'll verify that the containers are communicating

	// Create a file in peer-beta
	testContent := "Discovery test from peer-beta"
	createFileInContainer(t, projectName, "peer-beta", "/app/sync/discovery.txt", testContent)

	time.Sleep(3 * time.Second)

	// Verify it appears in other peers
	content := readFileFromContainer(t, projectName, "peer-alpha", "/app/sync/discovery.txt")
	if content != testContent {
		t.Errorf("Peer discovery failed: file not synced to peer-alpha")
	}

	content = readFileFromContainer(t, projectName, "peer-gamma", "/app/sync/discovery.txt")
	if content != testContent {
		t.Errorf("Peer discovery failed: file not synced to peer-gamma")
	}

	t.Log("Peer discovery test passed")
}

func testLargeFileSync(t *testing.T, projectName string) {
	t.Log("Testing large file synchronization...")

	// Create a 5MB test file
	largeContent := strings.Repeat("Large file content for testing chunking and sync efficiency. ", 50000)
	expectedSize := len(largeContent)

	createFileInContainer(t, projectName, "peer-alpha", "/app/sync/large.dat", largeContent)

	// Wait longer for large file sync
	time.Sleep(15 * time.Second)

	// Verify file size and content in peer-beta
	size := getFileSizeInContainer(t, projectName, "peer-beta", "/app/sync/large.dat")
	if size != expectedSize {
		t.Errorf("Large file sync failed: expected size %d, got %d", expectedSize, size)
	}

	// Verify first and last parts of content
	content := readFileFromContainer(t, projectName, "peer-beta", "/app/sync/large.dat")
	if len(content) != expectedSize {
		t.Errorf("Large file content size mismatch: expected %d, got %d", expectedSize, len(content))
	}

	if !strings.HasPrefix(content, "Large file content") {
		t.Errorf("Large file content prefix mismatch")
	}

	if !strings.HasSuffix(strings.TrimSpace(content), "efficiency.") {
		t.Errorf("Large file content suffix mismatch")
	}

	t.Log("Large file synchronization test passed")
}

func testConcurrentOperations(t *testing.T, projectName string) {
	t.Log("Testing concurrent operations...")

	// Create multiple files simultaneously across different peers
	files := []struct {
		container string
		filename  string
		content   string
	}{
		{"peer-alpha", "/app/sync/concurrent-1.txt", "Concurrent file 1"},
		{"peer-beta", "/app/sync/concurrent-2.txt", "Concurrent file 2"},
		{"peer-gamma", "/app/sync/concurrent-3.txt", "Concurrent file 3"},
	}

	// Create files concurrently (as much as possible with sequential commands)
	for _, file := range files {
		createFileInContainer(t, projectName, file.container, file.filename, file.content)
	}

	// Wait for sync
	time.Sleep(10 * time.Second)

	// Verify all files are synced to all peers
	for _, file := range files {
		for _, peer := range []string{"peer-alpha", "peer-beta", "peer-gamma"} {
			content := readFileFromContainer(t, projectName, peer, file.filename)
			if content != file.content {
				t.Errorf("Concurrent sync failed: %s not synced to %s", file.filename, peer)
			}
		}
	}

	t.Log("Concurrent operations test passed")
}

func testFailureRecovery(t *testing.T, projectName string) {
	t.Log("Testing failure recovery...")

	// Stop one peer
	stopContainer(t, projectName, "peer-beta")

	// Create a file in peer-alpha while peer-beta is down
	testContent := "Recovery test content"
	createFileInContainer(t, projectName, "peer-alpha", "/app/sync/recovery.txt", testContent)

	// Wait a bit
	time.Sleep(3 * time.Second)

	// Restart peer-beta
	startContainer(t, projectName, "peer-beta")

	// Wait for recovery sync
	time.Sleep(10 * time.Second)

	// Verify peer-beta received the file
	content := readFileFromContainer(t, projectName, "peer-beta", "/app/sync/recovery.txt")
	if content != testContent {
		t.Errorf("Failure recovery failed: expected %q, got %q", testContent, content)
	}

	t.Log("Failure recovery test passed")
}

func testConflictResolution(t *testing.T, projectName string) {
	t.Log("Testing conflict resolution...")

	// Create the same file in two peers with different content
	content1 := "Content from peer-alpha"
	content2 := "Content from peer-beta"

	// Create files simultaneously
	go createFileInContainer(t, projectName, "peer-alpha", "/app/sync/conflict.txt", content1)
	go createFileInContainer(t, projectName, "peer-beta", "/app/sync/conflict.txt", content2)

	// Wait for both operations
	time.Sleep(2 * time.Second)

	// Wait for sync and conflict resolution
	time.Sleep(10 * time.Second)

	// Check what happened (depends on conflict resolution strategy)
	// For now, just verify the system doesn't crash
	content := readFileFromContainer(t, projectName, "peer-gamma", "/app/sync/conflict.txt")
	if content == "" {
		t.Error("Conflict resolution failed: no content in final file")
	}

	t.Logf("Conflict resolution test completed. Final content: %q", content)
}

// Helper functions

func createFileInContainer(t *testing.T, projectName, container, filePath, content string) {
	cmd := exec.Command("docker", "exec", "-i", fmt.Sprintf("%s-%s-1", projectName, container),
		"sh", "-c", fmt.Sprintf("cat > %s", filePath))

	cmd.Stdin = strings.NewReader(content)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to create file in container %s: %v\nOutput: %s", container, err, output)
	}
}

func readFileFromContainer(t *testing.T, projectName, container, filePath string) string {
	cmd := exec.Command("docker", "exec", fmt.Sprintf("%s-%s-1", projectName, container),
		"cat", filePath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to read file from container %s: %v\nOutput: %s", container, err, string(output))
	}

	return strings.TrimSpace(string(output))
}

func getFileSizeInContainer(t *testing.T, projectName, container, filePath string) int {
	cmd := exec.Command("docker", "exec", fmt.Sprintf("%s-%s-1", projectName, container),
		"stat", "-c", "%s", filePath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to get file size from container %s: %v\nOutput: %s", container, err, string(output))
	}

	var size int
	fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &size)
	return size
}

func stopContainer(t *testing.T, projectName, container string) {
	cmd := exec.Command("docker", "stop", fmt.Sprintf("%s-%s-1", projectName, container))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to stop container %s: %v\nOutput: %s", container, err, output)
	}
}

func startContainer(t *testing.T, projectName, container string) {
	cmd := exec.Command("docker", "start", fmt.Sprintf("%s-%s-1", projectName, container))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to start container %s: %v\nOutput: %s", container, err, output)
	}
}

func getContainerLogs(t *testing.T, projectName, container string, lines int) string {
	cmd := exec.Command("docker", "logs", "--tail", fmt.Sprintf("%d", lines), fmt.Sprintf("%s-%s-1", projectName, container))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Failed to get logs from container %s: %v", container, err)
		return ""
	}
	return string(output)
}

// copyDir recursively copies a directory from src to dst
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Calculate relative path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		// Create destination path
		destPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}

		// Copy file
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(destPath, data, 0644)
	})
}

// cleanupStaleDockerResources removes any leftover Docker containers and networks from previous test runs
func cleanupStaleDockerResources(t *testing.T) {
	// Stop and remove all p2p-test containers
	cmd := exec.Command("sh", "-c", "docker ps -a -q --filter name=p2p-test | xargs -r docker rm -f")
	output, _ := cmd.CombinedOutput()
	if len(output) > 0 {
		t.Logf("Cleaned up stale containers: %s", output)
	}

	cmd = exec.Command("sh", "-c", "docker ps -a -q --filter name=p2p-partition | xargs -r docker rm -f")
	output, _ = cmd.CombinedOutput()
	if len(output) > 0 {
		t.Logf("Cleaned up stale partition containers: %s", output)
	}

	cmd = exec.Command("sh", "-c", "docker ps -a -q --filter name=p2p-reliability | xargs -r docker rm -f")
	output, _ = cmd.CombinedOutput()
	if len(output) > 0 {
		t.Logf("Cleaned up stale reliability containers: %s", output)
	}

	cmd = exec.Command("sh", "-c", "docker ps -a -q --filter name=p2p-config | xargs -r docker rm -f")
	output, _ = cmd.CombinedOutput()
	if len(output) > 0 {
		t.Logf("Cleaned up stale config containers: %s", output)
	}

	// Remove all p2p-test networks
	cmd = exec.Command("sh", "-c", "docker network ls -q --filter name=p2p-test | xargs -r docker network rm")
	output, _ = cmd.CombinedOutput()
	if len(output) > 0 {
		t.Logf("Cleaned up stale test networks: %s", output)
	}

	cmd = exec.Command("sh", "-c", "docker network ls -q --filter name=p2p-partition | xargs -r docker network rm")
	output, _ = cmd.CombinedOutput()
	if len(output) > 0 {
		t.Logf("Cleaned up stale partition networks: %s", output)
	}

	cmd = exec.Command("sh", "-c", "docker network ls -q --filter name=p2p-reliability | xargs -r docker network rm")
	output, _ = cmd.CombinedOutput()
	if len(output) > 0 {
		t.Logf("Cleaned up stale reliability networks: %s", output)
	}

	cmd = exec.Command("sh", "-c", "docker network ls -q --filter name=p2p-config | xargs -r docker network rm")
	output, _ = cmd.CombinedOutput()
	if len(output) > 0 {
		t.Logf("Cleaned up stale config networks: %s", output)
	}

	// Prune unused networks
	cmd = exec.Command("docker", "network", "prune", "-f")
	cmd.Run()

	t.Logf("Docker cleanup completed")
}
