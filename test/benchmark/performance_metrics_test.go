//go:build benchmark
// +build benchmark

package benchmark

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
