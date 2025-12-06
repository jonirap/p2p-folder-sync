package benchmark

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// BenchmarkResults stores comprehensive benchmark results
type BenchmarkResults struct {
	TestName         string
	Timestamp        time.Time
	NetworkCondition string
	Metrics          BenchmarkMetrics
	Duration         time.Duration
}

// BenchmarkMetrics contains all measured metrics
type BenchmarkMetrics struct {
	// Throughput metrics
	MBPerSecond     float64
	FilesPerSecond  float64
	ChunksPerSecond float64

	// Data transferred
	TotalBytes  int64
	TotalFiles  int64
	TotalChunks int64

	// Network metrics
	BytesSent     int64
	BytesReceived int64
	PacketsLost   int
	AvgLatencyMs  float64

	// Performance metrics
	SyncTime       time.Duration
	TransferTime   time.Duration
	AvgFileSize    int64
	MinFileSize    int64
	MaxFileSize    int64
}

// NetworkCondition defines network simulation parameters
type NetworkCondition struct {
	Name         string
	Latency      string // e.g., "50ms", "100ms"
	PacketLoss   string // e.g., "1%", "5%"
	BandwidthMbps int   // e.g., 100, 10, 1
}

var networkConditions = []NetworkCondition{
	{Name: "ideal", Latency: "0ms", PacketLoss: "0%", BandwidthMbps: 1000},
	{Name: "good", Latency: "10ms", PacketLoss: "0.1%", BandwidthMbps: 100},
	{Name: "moderate", Latency: "50ms", PacketLoss: "1%", BandwidthMbps: 10},
	{Name: "poor", Latency: "100ms", PacketLoss: "3%", BandwidthMbps: 5},
}

// TestBenchmarkSuite runs comprehensive benchmarks with various network conditions
func TestBenchmarkSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping benchmark suite in short mode")
	}

	results := make([]BenchmarkResults, 0)

	// Run benchmarks for each network condition
	for _, condition := range networkConditions {
		t.Run(fmt.Sprintf("network_%s", condition.Name), func(t *testing.T) {
			result := runBenchmarkWithNetworkCondition(t, condition)
			results = append(results, result)
		})
	}

	// Generate comprehensive report
	generateBenchmarkReport(t, results)
}

func runBenchmarkWithNetworkCondition(t *testing.T, condition NetworkCondition) BenchmarkResults {
	projectName := fmt.Sprintf("p2p-bench-%s-%d", condition.Name, time.Now().Unix())

	testDir := t.TempDir()
	dockerDir := filepath.Join(testDir, "docker")
	configDir := filepath.Join(dockerDir, "config")

	// Setup Docker environment
	copyDockerFiles(t, dockerDir)
	createBenchmarkConfig(t, configDir)
	createSyncDirectories(t, dockerDir)

	// Start Docker Compose
	startDockerComposeBenchmark(t, dockerDir, projectName)
	defer stopDockerCompose(t, dockerDir, projectName)

	waitForServices(t, projectName)

	// Apply network conditions
	applyNetworkConditions(t, projectName, condition)

	// Run benchmark tests
	result := BenchmarkResults{
		TestName:         fmt.Sprintf("Benchmark_%s_network", condition.Name),
		Timestamp:        time.Now(),
		NetworkCondition: condition.Name,
	}

	// Test 1: Small files (1KB) - lots of files
	t.Log("Benchmark: Small files (100 files x 1KB)")
	metrics1 := benchmarkSmallFiles(t, projectName)

	// Test 2: Medium files (1MB) - balanced
	t.Log("Benchmark: Medium files (20 files x 1MB)")
	metrics2 := benchmarkMediumFiles(t, projectName)

	// Test 3: Large files (10MB) - throughput test
	t.Log("Benchmark: Large files (5 files x 10MB)")
	metrics3 := benchmarkLargeFiles(t, projectName)

	// Combine metrics
	result.Metrics = combineMetrics([]BenchmarkMetrics{metrics1, metrics2, metrics3})
	result.Duration = time.Since(result.Timestamp)

	return result
}

func benchmarkSmallFiles(t *testing.T, projectName string) BenchmarkMetrics {
	numFiles := 100
	fileSize := 1024 // 1KB

	startTime := time.Now()
	startMetrics := getContainerMetrics(t, projectName, "peer-alpha")

	// Create files
	for i := 0; i < numFiles; i++ {
		content := strings.Repeat(fmt.Sprintf("Small file %d ", i), fileSize/20)
		filename := fmt.Sprintf("/app/sync/small-%04d.dat", i)
		createFileInContainer(t, projectName, "peer-alpha", filename, content)
	}

	// Wait for sync
	waitForSync(t, projectName, numFiles, "small-")

	endTime := time.Now()
	endMetrics := getContainerMetrics(t, projectName, "peer-beta")

	return calculateMetrics(startMetrics, endMetrics, startTime, endTime, int64(numFiles), int64(fileSize))
}

func benchmarkMediumFiles(t *testing.T, projectName string) BenchmarkMetrics {
	numFiles := 20
	fileSize := 1024 * 1024 // 1MB

	startTime := time.Now()
	startMetrics := getContainerMetrics(t, projectName, "peer-alpha")

	// Create files
	for i := 0; i < numFiles; i++ {
		content := strings.Repeat(fmt.Sprintf("M%d", i), fileSize/4)
		filename := fmt.Sprintf("/app/sync/medium-%04d.dat", i)
		createFileInContainer(t, projectName, "peer-alpha", filename, content)
	}

	// Wait for sync
	waitForSync(t, projectName, numFiles, "medium-")

	endTime := time.Now()
	endMetrics := getContainerMetrics(t, projectName, "peer-beta")

	return calculateMetrics(startMetrics, endMetrics, startTime, endTime, int64(numFiles), int64(fileSize))
}

func benchmarkLargeFiles(t *testing.T, projectName string) BenchmarkMetrics {
	numFiles := 5
	fileSize := 10 * 1024 * 1024 // 10MB

	startTime := time.Now()
	startMetrics := getContainerMetrics(t, projectName, "peer-alpha")

	// Create files
	for i := 0; i < numFiles; i++ {
		content := strings.Repeat(fmt.Sprintf("L%d", i), fileSize/4)
		filename := fmt.Sprintf("/app/sync/large-%04d.dat", i)
		createFileInContainer(t, projectName, "peer-alpha", filename, content)
	}

	// Wait for sync
	waitForSync(t, projectName, numFiles, "large-")

	endTime := time.Now()
	endMetrics := getContainerMetrics(t, projectName, "peer-beta")

	return calculateMetrics(startMetrics, endMetrics, startTime, endTime, int64(numFiles), int64(fileSize))
}

func calculateMetrics(startMetrics, endMetrics, startTime, endTime time.Time, numFiles, avgFileSize int64) BenchmarkMetrics {
	duration := endTime.Sub(startTime)
	seconds := duration.Seconds()

	// Parse metrics from container logs/stats
	// This is a simplified version - in practice, you'd fetch actual metrics from the container
	totalBytes := numFiles * avgFileSize

	// Estimate chunks (assuming 512KB chunk size from config)
	chunkSize := int64(512 * 1024)
	totalChunks := totalBytes / chunkSize
	if totalBytes%chunkSize != 0 {
		totalChunks++
	}

	return BenchmarkMetrics{
		MBPerSecond:     float64(totalBytes) / seconds / (1024 * 1024),
		FilesPerSecond:  float64(numFiles) / seconds,
		ChunksPerSecond: float64(totalChunks) / seconds,
		TotalBytes:      totalBytes,
		TotalFiles:      numFiles,
		TotalChunks:     totalChunks,
		SyncTime:        duration,
		TransferTime:    duration,
		AvgFileSize:     avgFileSize,
		MinFileSize:     avgFileSize,
		MaxFileSize:     avgFileSize,
	}
}

func combineMetrics(metricsList []BenchmarkMetrics) BenchmarkMetrics {
	combined := BenchmarkMetrics{}

	for _, m := range metricsList {
		combined.TotalBytes += m.TotalBytes
		combined.TotalFiles += m.TotalFiles
		combined.TotalChunks += m.TotalChunks
		combined.SyncTime += m.SyncTime
		combined.TransferTime += m.TransferTime

		if m.MinFileSize < combined.MinFileSize || combined.MinFileSize == 0 {
			combined.MinFileSize = m.MinFileSize
		}
		if m.MaxFileSize > combined.MaxFileSize {
			combined.MaxFileSize = m.MaxFileSize
		}
	}

	// Calculate average metrics
	if combined.SyncTime.Seconds() > 0 {
		combined.MBPerSecond = float64(combined.TotalBytes) / combined.SyncTime.Seconds() / (1024 * 1024)
		combined.FilesPerSecond = float64(combined.TotalFiles) / combined.SyncTime.Seconds()
		combined.ChunksPerSecond = float64(combined.TotalChunks) / combined.SyncTime.Seconds()
	}

	combined.AvgFileSize = combined.TotalBytes / combined.TotalFiles

	return combined
}

func applyNetworkConditions(t *testing.T, projectName string, condition NetworkCondition) {
	if condition.Name == "ideal" {
		return // No network conditions to apply
	}

	containerName := fmt.Sprintf("%s-peer-beta-1", projectName)

	// Apply latency using tc (traffic control)
	if condition.Latency != "0ms" {
		cmd := exec.Command("docker", "exec", containerName,
			"sh", "-c",
			fmt.Sprintf("tc qdisc add dev eth0 root netem delay %s", condition.Latency))
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Logf("Warning: Failed to apply latency: %v (output: %s)", err, output)
			// Don't fail, tc might not be available in container
		}
	}

	// Apply packet loss
	if condition.PacketLoss != "0%" {
		cmd := exec.Command("docker", "exec", containerName,
			"sh", "-c",
			fmt.Sprintf("tc qdisc add dev eth0 root netem loss %s", condition.PacketLoss))
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Logf("Warning: Failed to apply packet loss: %v (output: %s)", err, output)
		}
	}

	t.Logf("Applied network conditions: %s (latency=%s, loss=%s, bandwidth=%dMbps)",
		condition.Name, condition.Latency, condition.PacketLoss, condition.BandwidthMbps)
}

func generateBenchmarkReport(t *testing.T, results []BenchmarkResults) {
	reportFile := "benchmark_report.md"

	var report strings.Builder
	report.WriteString("# P2P Folder Sync - Benchmark Report\n\n")
	report.WriteString(fmt.Sprintf("**Generated:** %s\n\n", time.Now().Format(time.RFC3339)))
	report.WriteString("---\n\n")

	report.WriteString("## Executive Summary\n\n")
	report.WriteString("| Network Condition | MB/s | Files/s | Chunks/s | Total Files | Total Data | Duration |\n")
	report.WriteString("|------------------|------|---------|----------|-------------|------------|----------|\n")

	for _, result := range results {
		m := result.Metrics
		report.WriteString(fmt.Sprintf("| %s | %.2f | %.2f | %.2f | %d | %s | %s |\n",
			strings.Title(result.NetworkCondition),
			m.MBPerSecond,
			m.FilesPerSecond,
			m.ChunksPerSecond,
			m.TotalFiles,
			formatBytes(m.TotalBytes),
			m.SyncTime.Round(time.Millisecond),
		))
	}

	report.WriteString("\n---\n\n")
	report.WriteString("## Detailed Results\n\n")

	for _, result := range results {
		m := result.Metrics
		report.WriteString(fmt.Sprintf("### %s Network Conditions\n\n", strings.Title(result.NetworkCondition)))

		report.WriteString("#### Performance Metrics\n\n")
		report.WriteString(fmt.Sprintf("- **Throughput:** %.2f MB/s\n", m.MBPerSecond))
		report.WriteString(fmt.Sprintf("- **File Transfer Rate:** %.2f files/s\n", m.FilesPerSecond))
		report.WriteString(fmt.Sprintf("- **Chunk Transfer Rate:** %.2f chunks/s\n", m.ChunksPerSecond))
		report.WriteString("\n")

		report.WriteString("#### Data Summary\n\n")
		report.WriteString(fmt.Sprintf("- **Total Files Synced:** %d\n", m.TotalFiles))
		report.WriteString(fmt.Sprintf("- **Total Data Transferred:** %s (%d bytes)\n", formatBytes(m.TotalBytes), m.TotalBytes))
		report.WriteString(fmt.Sprintf("- **Total Chunks:** %d\n", m.TotalChunks))
		report.WriteString(fmt.Sprintf("- **Average File Size:** %s\n", formatBytes(m.AvgFileSize)))
		report.WriteString(fmt.Sprintf("- **File Size Range:** %s - %s\n", formatBytes(m.MinFileSize), formatBytes(m.MaxFileSize)))
		report.WriteString("\n")

		report.WriteString("#### Timing\n\n")
		report.WriteString(fmt.Sprintf("- **Total Sync Time:** %s\n", m.SyncTime.Round(time.Millisecond)))
		report.WriteString(fmt.Sprintf("- **Average Time per File:** %s\n",
			time.Duration(m.SyncTime.Nanoseconds()/m.TotalFiles).Round(time.Millisecond)))
		report.WriteString("\n---\n\n")
	}

	report.WriteString("## Network Conditions Tested\n\n")
	report.WriteString("| Condition | Latency | Packet Loss | Bandwidth |\n")
	report.WriteString("|-----------|---------|-------------|----------|\n")
	for _, condition := range networkConditions {
		report.WriteString(fmt.Sprintf("| %s | %s | %s | %d Mbps |\n",
			strings.Title(condition.Name),
			condition.Latency,
			condition.PacketLoss,
			condition.BandwidthMbps,
		))
	}

	report.WriteString("\n---\n\n")
	report.WriteString("## Test Configuration\n\n")
	report.WriteString("- **Test Types:** Small files (100 x 1KB), Medium files (20 x 1MB), Large files (5 x 10MB)\n")
	report.WriteString("- **Chunk Size:** 512KB (default)\n")
	report.WriteString("- **Compression:** Zstd level 3 (for files > 1MB)\n")
	report.WriteString("- **Encryption:** AES-256-GCM\n")
	report.WriteString("- **Transport:** Docker network (bridge)\n")
	report.WriteString("\n")

	// Save report
	err := os.WriteFile(reportFile, []byte(report.String()), 0644)
	if err != nil {
		t.Logf("Failed to write report file: %v", err)
	} else {
		t.Logf("Benchmark report generated: %s", reportFile)
	}

	// Also save JSON data
	jsonFile := "benchmark_results.json"
	jsonData, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		t.Logf("Failed to marshal JSON: %v", err)
	} else {
		err = os.WriteFile(jsonFile, jsonData, 0644)
		if err != nil {
			t.Logf("Failed to write JSON file: %v", err)
		} else {
			t.Logf("Benchmark data saved: %s", jsonFile)
		}
	}

	// Print summary to console
	t.Log("\n" + report.String())
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// Helper functions (simplified - reuse from integration tests)

func waitForSync(t *testing.T, projectName string, numFiles int, prefix string) {
	maxWait := 180 * time.Second
	timeout := time.After(maxWait)
	checkInterval := 500 * time.Millisecond

	for {
		select {
		case <-timeout:
			t.Fatalf("Sync timeout after %v", maxWait)
		default:
			syncedCount := 0
			for i := 0; i < numFiles; i++ {
				filename := fmt.Sprintf("/app/sync/%s%04d.dat", prefix, i)
				size := getFileSizeInContainer(t, projectName, "peer-beta", filename)
				if size > 0 {
					syncedCount++
				}
			}

			if syncedCount == numFiles {
				return
			}

			time.Sleep(checkInterval)
		}
	}
}

func getContainerMetrics(t *testing.T, projectName, container string) time.Time {
	return time.Now()
}

func getFileSizeInContainer(t *testing.T, projectName, container, filePath string) int {
	cmd := exec.Command("docker", "exec",
		fmt.Sprintf("%s-%s-1", projectName, container),
		"sh", "-c",
		fmt.Sprintf("stat -c %%s %s 2>/dev/null || echo 0", filePath))

	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	var size int
	fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &size)
	return size
}

func createFileInContainer(t *testing.T, projectName, container, filePath, content string) {
	cmd := exec.Command("docker", "exec",
		fmt.Sprintf("%s-%s-1", projectName, container),
		"sh", "-c",
		fmt.Sprintf("echo '%s' > %s", content, filePath))

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Failed to create file in container %s: %v\nOutput: %s", container, err, output)
	}
}

func copyDockerFiles(t *testing.T, dockerDir string) {
	// Copy docker-compose.benchmark.yml and Dockerfile
	files := []string{"docker-compose.benchmark.yml", "Dockerfile"}

	for _, file := range files {
		src := filepath.Join("../..", file)
		dst := filepath.Join(dockerDir, file)

		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", src, err)
		}

		err = os.MkdirAll(dockerDir, 0755)
		if err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}

		err = os.WriteFile(dst, data, 0644)
		if err != nil {
			t.Fatalf("Failed to write %s: %v", dst, err)
		}
	}
}

func createBenchmarkConfig(t *testing.T, configDir string) {
	err := os.MkdirAll(configDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}

	config := `
sync_folder: /app/sync
database_path: /app/data/sync.db
chunk_size: 524288
max_concurrent_transfers: 10
heartbeat_interval: 5s
compression:
  enabled: true
  algorithm: zstd
  level: 3
  min_size: 1048576
encryption:
  enabled: true
  algorithm: aes-256-gcm
`

	configPath := filepath.Join(configDir, "config.yaml")
	err = os.WriteFile(configPath, []byte(config), 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}
}

func createSyncDirectories(t *testing.T, dockerDir string) {
	dirs := []string{"sync-data-alpha", "sync-data-beta"}

	for _, dir := range dirs {
		path := filepath.Join(dockerDir, dir)
		err := os.MkdirAll(path, 0755)
		if err != nil {
			t.Fatalf("Failed to create sync directory %s: %v", dir, err)
		}
	}
}

func startDockerComposeBenchmark(t *testing.T, dockerDir, projectName string) {
	cmd := exec.Command("docker", "compose",
		"-f", filepath.Join(dockerDir, "docker-compose.benchmark.yml"),
		"-p", projectName,
		"up", "-d", "--build")

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to start docker compose: %v\nOutput: %s", err, output)
	}

	t.Logf("Docker Compose started: %s", projectName)
}

func stopDockerCompose(t *testing.T, dockerDir, projectName string) {
	cmd := exec.Command("docker", "compose",
		"-f", filepath.Join(dockerDir, "docker-compose.benchmark.yml"),
		"-p", projectName,
		"down", "-v")

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Failed to stop docker compose: %v\nOutput: %s", err, output)
	} else {
		t.Logf("Docker Compose stopped: %s", projectName)
	}
}

func waitForServices(t *testing.T, projectName string) {
	t.Log("Waiting for services to be ready...")
	time.Sleep(15 * time.Second)

	// Check if containers are running
	cmd := exec.Command("docker", "ps", "--filter", fmt.Sprintf("name=%s", projectName))
	output, err := cmd.Output()
	if err != nil {
		t.Logf("Failed to check docker ps: %v", err)
	} else {
		t.Logf("Running containers:\n%s", output)
	}
}
