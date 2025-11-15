package integration

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestPerformanceBenchmarks runs performance benchmarks
func TestPerformanceBenchmarks(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance benchmarks in short mode")
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

	// Run performance tests
	testFileSyncPerformance(t, projectName)
	testConcurrentSyncPerformance(t, projectName)
	testNetworkThroughput(t, projectName)
	testMemoryUsage(t, projectName)
	testDiskIOPerformance(t, projectName)
}

func testFileSyncPerformance(t *testing.T, projectName string) {
	t.Log("Testing file sync performance...")

	testCases := []struct {
		name        string
		size        int
		description string
	}{
		{"tiny", 100, "100 bytes"},
		{"small", 1024, "1KB"},
		{"medium", 1024 * 100, "100KB"},
		{"large", 1024 * 1024, "1MB"},
		{"xlarge", 5 * 1024 * 1024, "5MB"},
	}

	results := make([]string, 0, len(testCases))

	for _, tc := range testCases {
		t.Run(tc.name+"_file_sync", func(t *testing.T) {
			content := strings.Repeat("X", tc.size)
			filename := fmt.Sprintf("/app/sync/perf-%s.dat", tc.name)

			// Measure creation time
			start := time.Now()
			createFileInContainer(t, projectName, "peer-alpha", filename, content)
			creationTime := time.Since(start)

			// Measure sync time (wait for file to appear on peer-beta)
			syncStart := time.Now()
			maxWait := 60 * time.Second
			timeout := time.After(maxWait)

			for {
				select {
				case <-timeout:
					t.Fatalf("Sync timeout for %s file after %v", tc.name, maxWait)
				default:
					size := getFileSizeInContainer(t, projectName, "peer-beta", filename)
					if size == tc.size {
						syncTime := time.Since(syncStart)
						result := fmt.Sprintf("%s (%s): creation=%v, sync=%v, total=%v",
							tc.name, tc.description, creationTime, syncTime, creationTime+syncTime)
						results = append(results, result)
						t.Log(result)
						return
					}
					time.Sleep(100 * time.Millisecond)
				}
			}
		})
	}

	t.Log("File sync performance results:")
	for _, result := range results {
		t.Log("  " + result)
	}
}

func testConcurrentSyncPerformance(t *testing.T, projectName string) {
	t.Log("Testing concurrent sync performance...")

	numFiles := 20
	fileSize := 50 * 1024 // 50KB each

	start := time.Now()

	// Create files concurrently (simulated)
	for i := 0; i < numFiles; i++ {
		go func(index int) {
			content := strings.Repeat(fmt.Sprintf("File %d content ", index), fileSize/20)
			filename := fmt.Sprintf("/app/sync/concurrent-%03d.dat", index)
			createFileInContainer(t, projectName, "peer-alpha", filename, content)
		}(i)
	}

	creationTime := time.Since(start)
	t.Logf("Created %d files in %v", numFiles, creationTime)

	// Wait for all files to sync
	syncStart := time.Now()
	maxWait := 120 * time.Second
	timeout := time.After(maxWait)

	for {
		select {
		case <-timeout:
			t.Fatalf("Concurrent sync timeout after %v", maxWait)
		default:
			syncedCount := 0
			for i := 0; i < numFiles; i++ {
				filename := fmt.Sprintf("/app/sync/concurrent-%03d.dat", i)
				size := getFileSizeInContainer(t, projectName, "peer-beta", filename)
				if size > 0 {
					syncedCount++
				}
			}

			if syncedCount == numFiles {
				syncTime := time.Since(syncStart)
				totalTime := time.Since(start)

				t.Logf("Concurrent sync performance:")
				t.Logf("  Files: %d", numFiles)
				t.Logf("  Total size: %d KB", (numFiles*fileSize)/1024)
				t.Logf("  Creation time: %v", creationTime)
				t.Logf("  Sync time: %v", syncTime)
				t.Logf("  Total time: %v", totalTime)
				t.Logf("  Files/sec: %.2f", float64(numFiles)/syncTime.Seconds())
				return
			}

			time.Sleep(1 * time.Second)
		}
	}
}

func testNetworkThroughput(t *testing.T, projectName string) {
	t.Log("Testing network throughput...")

	// Test with different file sizes to measure throughput
	sizes := []int{
		100 * 1024,  // 100KB
		1000 * 1024, // 1MB
		5000 * 1024, // 5MB
	}

	results := make([]string, 0, len(sizes))

	for _, size := range sizes {
		content := strings.Repeat("T", size)
		filename := fmt.Sprintf("/app/sync/throughput-%d.dat", size)

		start := time.Now()
		createFileInContainer(t, projectName, "peer-alpha", filename, content)

		// Wait for sync
		timeout := time.After(60 * time.Second)
		for {
			select {
			case <-timeout:
				t.Fatalf("Throughput test timeout for %d bytes", size)
			default:
				actualSize := getFileSizeInContainer(t, projectName, "peer-beta", filename)
				if actualSize == size {
					elapsed := time.Since(start)
					throughput := float64(size) / elapsed.Seconds() / (1024 * 1024) // MB/s
					result := fmt.Sprintf("%d bytes: %v (%f MB/s)", size, elapsed, throughput)
					results = append(results, result)
					goto nextSize
				}
				time.Sleep(200 * time.Millisecond)
			}
		}
	nextSize:
	}

	t.Log("Network throughput results:")
	for _, result := range results {
		t.Log("  " + result)
	}
}

func testMemoryUsage(t *testing.T, projectName string) {
	t.Log("Testing memory usage patterns...")

	// Monitor memory usage during large file sync
	containerName := fmt.Sprintf("%s_peer-alpha_1", projectName)

	// Get baseline memory
	baseline := getMemoryUsage(t, containerName)
	t.Logf("Baseline memory usage: %s", baseline)

	// Create large file
	largeContent := strings.Repeat("Memory test content ", 50000) // ~1MB
	createFileInContainer(t, projectName, "peer-alpha", "/app/sync/memory-large.dat", largeContent)

	time.Sleep(2 * time.Second)

	// Check memory during sync
	duringSync := getMemoryUsage(t, containerName)
	t.Logf("Memory during sync: %s", duringSync)

	// Wait for sync completion
	time.Sleep(10 * time.Second)

	// Check memory after sync
	afterSync := getMemoryUsage(t, containerName)
	t.Logf("Memory after sync: %s", afterSync)

	t.Log("Memory usage test completed")
}

func testDiskIOPerformance(t *testing.T, projectName string) {
	t.Log("Testing disk I/O performance...")

	// Test write performance
	start := time.Now()

	// Create multiple files to test disk I/O
	for i := 0; i < 10; i++ {
		content := strings.Repeat(fmt.Sprintf("Disk I/O test content %d ", i), 1000)
		filename := fmt.Sprintf("/app/sync/disk-io-%d.txt", i)
		createFileInContainer(t, projectName, "peer-alpha", filename, content)
	}

	writeTime := time.Since(start)
	t.Logf("Disk write time for 10 files: %v", writeTime)

	// Wait for sync
	time.Sleep(15 * time.Second)

	// Test read performance (by checking file sizes)
	readStart := time.Now()
	totalSize := 0
	for i := 0; i < 10; i++ {
		filename := fmt.Sprintf("/app/sync/disk-io-%d.txt", i)
		size := getFileSizeInContainer(t, projectName, "peer-beta", filename)
		totalSize += size
	}

	readTime := time.Since(readStart)
	t.Logf("Disk read time: %v for %d bytes", readTime, totalSize)
}

// Helper functions

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

// TestScalability tests system scalability with increasing load
func TestScalability(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping scalability test in short mode")
	}

	projectName := fmt.Sprintf("p2p-scale-%d", time.Now().Unix())

	testDir := t.TempDir()
	dockerDir := filepath.Join(testDir, "docker")
	configDir := filepath.Join(dockerDir, "config")

	copyDockerFiles(t, dockerDir)
	createTestConfig(t, configDir)
	createSyncDirectories(t, dockerDir)

	startDockerCompose(t, dockerDir, projectName)
	defer stopDockerCompose(t, dockerDir, projectName)

	waitForServices(t, projectName)

	// Test scalability with increasing numbers of files
	fileCounts := []int{10, 25, 50, 100}

	for _, count := range fileCounts {
		t.Run(fmt.Sprintf("scale_%d_files", count), func(t *testing.T) {
			start := time.Now()

			// Create N files
			for i := 0; i < count; i++ {
				content := fmt.Sprintf("Scalability test file %d", i)
				filename := fmt.Sprintf("/app/sync/scale-%03d.txt", i)
				createFileInContainer(t, projectName, "peer-alpha", filename, content)
			}

			// Wait for sync
			time.Sleep(time.Duration(count/10+5) * time.Second) // Scale wait time with file count

			// Count synced files
			syncedCount := 0
			for i := 0; i < count; i++ {
				filename := fmt.Sprintf("/app/sync/scale-%03d.txt", i)
				size := getFileSizeInContainer(t, projectName, "peer-beta", filename)
				if size > 0 {
					syncedCount++
				}
			}

			elapsed := time.Since(start)

			t.Logf("Scalability test (%d files): synced %d/%d in %v",
				count, syncedCount, count, elapsed)

			if syncedCount != count {
				t.Errorf("Scalability test failed: only %d/%d files synced", syncedCount, count)
			}
		})
	}
}

// TestReliability tests system reliability under stress
func TestReliability(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping reliability test in short mode")
	}

	projectName := fmt.Sprintf("p2p-reliability-%d", time.Now().Unix())

	testDir := t.TempDir()
	dockerDir := filepath.Join(testDir, "docker")
	configDir := filepath.Join(dockerDir, "config")

	copyDockerFiles(t, dockerDir)
	createTestConfig(t, configDir)
	createSyncDirectories(t, dockerDir)

	startDockerCompose(t, dockerDir, projectName)
	defer stopDockerCompose(t, dockerDir, projectName)

	waitForServices(t, projectName)

	t.Log("Testing system reliability...")

	// Create initial baseline file
	createFileInContainer(t, projectName, "peer-alpha", "/app/sync/reliability-baseline.txt", "Baseline content")
	time.Sleep(3 * time.Second)

	// Verify baseline sync
	content := readFileFromContainer(t, projectName, "peer-beta", "/app/sync/reliability-baseline.txt")
	if content != "Baseline content" {
		t.Fatal("Baseline sync failed")
	}

	// Stress test: rapid create/modify/delete cycle
	iterations := 5
	for i := 0; i < iterations; i++ {
		t.Logf("Reliability iteration %d/%d", i+1, iterations)

		// Create files
		for j := 0; j < 5; j++ {
			filename := fmt.Sprintf("/app/sync/stress-%d-%d.txt", i, j)
			content := fmt.Sprintf("Stress content iteration %d file %d", i, j)
			createFileInContainer(t, projectName, "peer-alpha", filename, content)
		}

		// Modify files
		time.Sleep(1 * time.Second)
		for j := 0; j < 5; j++ {
			filename := fmt.Sprintf("/app/sync/stress-%d-%d.txt", i, j)
			content := fmt.Sprintf("Modified stress content iteration %d file %d", i, j)
			createFileInContainer(t, projectName, "peer-alpha", filename, content)
		}

		// Delete files
		time.Sleep(1 * time.Second)
		for j := 0; j < 5; j++ {
			filename := fmt.Sprintf("/app/sync/stress-%d-%d.txt", i, j)
			deleteFileInContainer(t, projectName, "peer-alpha", filename)
		}

		// Wait for operations to sync
		time.Sleep(5 * time.Second)
	}

	// Final verification - baseline file should still exist
	content = readFileFromContainer(t, projectName, "peer-beta", "/app/sync/reliability-baseline.txt")
	if content != "Baseline content" {
		t.Error("Reliability test failed: baseline file corrupted or lost")
	}

	t.Log("Reliability test passed")
}

// Helper function to delete files in containers
func deleteFileInContainer(t *testing.T, projectName, container, filePath string) {
	cmd := exec.Command("docker", "exec", fmt.Sprintf("%s_%s_1", projectName, container),
		"rm", "-f", filePath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Failed to delete file in container %s: %v\nOutput: %s", container, err, output)
	}
}
