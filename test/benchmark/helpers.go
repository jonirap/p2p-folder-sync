//go:build benchmark
// +build benchmark

package benchmark

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// copyDockerFiles copies necessary Docker configuration files to the test directory
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

// createTestConfig creates a test configuration file
func createTestConfig(t *testing.T, configDir string) {
	config := `
sync:
  folder_path: "/app/sync"
  chunk_size_min: 65536
  chunk_size_max: 2097152
  chunk_size_default: 524288
  max_concurrent_transfers: 5
  operation_log_size: 10000
  state_sync_interval: 5

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

// createSyncDirectories creates sync data directories for each peer
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

// startDockerCompose starts the Docker Compose environment
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

// stopDockerCompose stops the Docker Compose environment
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

// waitForServices waits for all Docker services to be healthy
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

// checkServicesHealthy checks if all services are healthy
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

// createFileInContainer creates a file in a Docker container
func createFileInContainer(t *testing.T, projectName, container, filePath, content string) {
	cmd := exec.Command("docker", "exec", "-i", fmt.Sprintf("%s-%s-1", projectName, container),
		"sh", "-c", fmt.Sprintf("cat > %s", filePath))

	cmd.Stdin = strings.NewReader(content)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to create file in container %s: %v\nOutput: %s", container, err, output)
	}
}

// tryReadFileFromContainer attempts to read a file from a Docker container
func tryReadFileFromContainer(t *testing.T, projectName, container, filePath string) (string, error) {
	cmd := exec.Command("docker", "exec", fmt.Sprintf("%s-%s-1", projectName, container),
		"cat", filePath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w (output: %s)", err, string(output))
	}

	return strings.TrimSpace(string(output)), nil
}

// readFileFromContainer reads a file from a Docker container
func readFileFromContainer(t *testing.T, projectName, container, filePath string) string {
	content, err := tryReadFileFromContainer(t, projectName, container, filePath)
	if err != nil {
		t.Fatalf("Failed to read file from container %s: %v", container, err)
	}
	return content
}

// getFileSizeInContainer gets the size of a file in a Docker container
func getFileSizeInContainer(t *testing.T, projectName, container, filePath string) int {
	cmd := exec.Command("docker", "exec", fmt.Sprintf("%s-%s-1", projectName, container),
		"stat", "-c", "%s", filePath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// File doesn't exist, return 0
		return 0
	}

	var size int
	fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &size)
	return size
}

// deleteFileInContainer deletes a file in a Docker container
func deleteFileInContainer(t *testing.T, projectName, container, filePath string) {
	cmd := exec.Command("docker", "exec", fmt.Sprintf("%s_%s_1", projectName, container),
		"rm", "-f", filePath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Failed to delete file in container %s: %v\nOutput: %s", container, err, output)
	}
}

// countSyncedFiles counts how many files with a given prefix have been synced
func countSyncedFiles(t *testing.T, projectName, prefix string, expectedCount int) int {
	syncedCount := 0
	for i := 0; i < expectedCount; i++ {
		filename := fmt.Sprintf("/app/sync/%s%d.txt", prefix, i)
		size := getFileSizeInContainer(t, projectName, "peer-beta", filename)
		if size > 0 {
			syncedCount++
		}
	}
	return syncedCount
}

// getMemoryUsage gets memory usage statistics for a container
func getMemoryUsage(t *testing.T, containerName string) string {
	cmd := exec.Command("docker", "stats", "--no-stream", "--format", "{{.Container}}\t{{.CPUPerc}}\t{{.MemUsage}}", containerName)

	output, err := cmd.Output()
	if err != nil {
		t.Logf("Failed to get memory usage: %v", err)
		return "unknown"
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) > 0 {
		return lines[0]
	}

	return "unknown"
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

	cmd = exec.Command("sh", "-c", "docker ps -a -q --filter name=p2p-ack | xargs -r docker rm -f")
	output, _ = cmd.CombinedOutput()
	if len(output) > 0 {
		t.Logf("Cleaned up stale ack containers: %s", output)
	}

	cmd = exec.Command("sh", "-c", "docker ps -a -q --filter name=p2p-perf | xargs -r docker rm -f")
	output, _ = cmd.CombinedOutput()
	if len(output) > 0 {
		t.Logf("Cleaned up stale perf containers: %s", output)
	}

	cmd = exec.Command("sh", "-c", "docker ps -a -q --filter name=p2p-scale | xargs -r docker rm -f")
	output, _ = cmd.CombinedOutput()
	if len(output) > 0 {
		t.Logf("Cleaned up stale scale containers: %s", output)
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

	cmd = exec.Command("sh", "-c", "docker network ls -q --filter name=p2p-ack | xargs -r docker network rm")
	output, _ = cmd.CombinedOutput()
	if len(output) > 0 {
		t.Logf("Cleaned up stale ack networks: %s", output)
	}

	cmd = exec.Command("sh", "-c", "docker network ls -q --filter name=p2p-perf | xargs -r docker network rm")
	output, _ = cmd.CombinedOutput()
	if len(output) > 0 {
		t.Logf("Cleaned up stale perf networks: %s", output)
	}

	cmd = exec.Command("sh", "-c", "docker network ls -q --filter name=p2p-scale | xargs -r docker network rm")
	output, _ = cmd.CombinedOutput()
	if len(output) > 0 {
		t.Logf("Cleaned up stale scale networks: %s", output)
	}

	// Prune unused networks
	cmd = exec.Command("docker", "network", "prune", "-f")
	cmd.Run()

	t.Logf("Docker cleanup completed")
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
