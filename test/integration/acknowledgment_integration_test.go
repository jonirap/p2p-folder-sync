package integration

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestFileSyncWithAcknowledgments tests that files sync successfully with proper acknowledgment handling
func TestFileSyncWithAcknowledgments(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	projectName := fmt.Sprintf("p2p-ack-test-%d", time.Now().Unix())

	testDir := t.TempDir()
	dockerDir := filepath.Join(testDir, "docker")
	configDir := filepath.Join(dockerDir, "config")

	// Setup Docker environment
	copyDockerFiles(t, dockerDir)
	createTestConfig(t, configDir)
	createSyncDirectories(t, dockerDir)

	// Start Docker Compose
	startDockerCompose(t, dockerDir, projectName)
	defer stopDockerCompose(t, dockerDir, projectName)

	waitForServices(t, projectName)

	t.Log("Testing file sync with acknowledgment flow...")

	// Test 1: Single small file sync
	t.Run("single_small_file", func(t *testing.T) {
		testSingleFileSync(t, projectName, "ack-test-1.txt", "Hello from ACK test!", 5*time.Second)
	})

	// Test 2: Multiple small files
	t.Run("multiple_small_files", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			filename := fmt.Sprintf("ack-test-%d.txt", i+2)
			content := fmt.Sprintf("ACK test file %d", i+2)
			testSingleFileSync(t, projectName, filename, content, 10*time.Second)
		}
	})

	// Test 3: Medium file (to test chunking with ACKs)
	t.Run("medium_file_with_chunks", func(t *testing.T) {
		mediumContent := strings.Repeat("This is a medium file for ACK testing. ", 5000) // ~200KB
		testSingleFileSync(t, projectName, "ack-medium.txt", mediumContent, 15*time.Second)
	})

	// Test 4: Concurrent file sync
	t.Run("concurrent_files", func(t *testing.T) {
		testConcurrentFileSync(t, projectName, 10, 30*time.Second)
	})
}

// testSingleFileSync tests syncing a single file and verifies it arrives
func testSingleFileSync(t *testing.T, projectName, filename, content string, timeout time.Duration) {
	// Create file on peer-alpha
	createFileInContainer(t, projectName, "peer-alpha", fmt.Sprintf("/app/sync/%s", filename), content)

	// Wait for file to sync to peer-beta
	start := time.Now()
	maxWait := timeout
	timeoutCh := time.After(maxWait)

	for {
		select {
		case <-timeoutCh:
			// Get logs to diagnose
			alphaLogs := getContainerLogsTest(t, projectName, "peer-alpha", 50)
			betaLogs := getContainerLogsTest(t, projectName, "peer-beta", 50)

			t.Fatalf("File %s did not sync within %v\nAlpha logs:\n%s\n\nBeta logs:\n%s",
				filename, maxWait, alphaLogs, betaLogs)

		default:
			actualContent := readFileFromContainer(t, projectName, "peer-beta", fmt.Sprintf("/app/sync/%s", filename))
			if actualContent == content {
				elapsed := time.Since(start)
				t.Logf("✓ File %s synced successfully in %v", filename, elapsed.Round(time.Millisecond))
				return
			}

			time.Sleep(100 * time.Millisecond)
		}
	}
}

// testConcurrentFileSync tests syncing multiple files concurrently
func testConcurrentFileSync(t *testing.T, projectName string, numFiles int, timeout time.Duration) {
	// Create multiple files concurrently
	start := time.Now()

	for i := 0; i < numFiles; i++ {
		filename := fmt.Sprintf("concurrent-%d.txt", i)
		content := fmt.Sprintf("Concurrent test file %d", i)

		go func(fn, cnt string) {
			createFileInContainer(t, projectName, "peer-alpha", fmt.Sprintf("/app/sync/%s", fn), cnt)
		}(filename, content)
	}

	// Wait for all files to sync
	timeoutCh := time.After(timeout)
	checkInterval := 500 * time.Millisecond

	for {
		select {
		case <-timeoutCh:
			syncedCount := countSyncedFiles(t, projectName, "concurrent-", numFiles)
			t.Fatalf("Only %d/%d files synced within %v", syncedCount, numFiles, timeout)

		default:
			syncedCount := countSyncedFiles(t, projectName, "concurrent-", numFiles)

			if syncedCount == numFiles {
				elapsed := time.Since(start)
				rate := float64(numFiles) / elapsed.Seconds()
				t.Logf("✓ All %d files synced in %v (%.2f files/s)",
					numFiles, elapsed.Round(time.Millisecond), rate)
				return
			}

			time.Sleep(checkInterval)
		}
	}
}

// countSyncedFiles counts how many files with a given prefix have synced
func countSyncedFiles(t *testing.T, projectName, prefix string, expectedCount int) int {
	count := 0

	for i := 0; i < expectedCount; i++ {
		filename := fmt.Sprintf("%s%d.txt", prefix, i)
		content := readFileFromContainer(t, projectName, "peer-beta", fmt.Sprintf("/app/sync/%s", filename))

		if len(content) > 0 {
			count++
		}
	}

	return count
}

// getContainerLogsTest retrieves the last N lines of logs from a container
func getContainerLogsTest(t *testing.T, projectName, container string, lines int) string {
	cmd := exec.Command("docker", "logs", "--tail", fmt.Sprintf("%d", lines),
		fmt.Sprintf("%s_%s_1", projectName, container))

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Failed to get logs for %s: %v", container, err)
		return ""
	}

	return string(output)
}

// TestAcknowledgmentPerformance tests that ACKs don't significantly slow down sync
func TestAcknowledgmentPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	projectName := fmt.Sprintf("p2p-ack-perf-%d", time.Now().Unix())

	testDir := t.TempDir()
	dockerDir := filepath.Join(testDir, "docker")
	configDir := filepath.Join(dockerDir, "config")

	copyDockerFiles(t, dockerDir)
	createTestConfig(t, configDir)
	createSyncDirectories(t, dockerDir)

	startDockerCompose(t, dockerDir, projectName)
	defer stopDockerCompose(t, dockerDir, projectName)

	waitForServices(t, projectName)

	t.Log("Testing acknowledgment performance...")

	// Sync 50 small files and measure performance
	numFiles := 50
	start := time.Now()

	for i := 0; i < numFiles; i++ {
		filename := fmt.Sprintf("perf-test-%d.txt", i)
		content := fmt.Sprintf("Performance test file %d", i)
		createFileInContainer(t, projectName, "peer-alpha", fmt.Sprintf("/app/sync/%s", filename), content)
	}

	// Wait for all files to sync
	maxWait := 120 * time.Second
	timeoutCh := time.After(maxWait)

	for {
		select {
		case <-timeoutCh:
			syncedCount := countSyncedFiles(t, projectName, "perf-test-", numFiles)
			t.Fatalf("Performance test failed: only %d/%d files synced within %v", syncedCount, numFiles, maxWait)

		default:
			syncedCount := countSyncedFiles(t, projectName, "perf-test-", numFiles)

			if syncedCount == numFiles {
				elapsed := time.Since(start)
				filesPerSec := float64(numFiles) / elapsed.Seconds()

				t.Logf("Performance test results:")
				t.Logf("  Files synced: %d", numFiles)
				t.Logf("  Total time: %v", elapsed.Round(time.Millisecond))
				t.Logf("  Files/sec: %.2f", filesPerSec)

				// Performance expectations:
				// With proper ACKs, we should still be able to sync at a reasonable rate
				// At minimum, we should sync faster than 1 file per 2 seconds
				minExpectedRate := 0.5 // 0.5 files/sec = 1 file per 2 seconds

				if filesPerSec < minExpectedRate {
					t.Errorf("Performance below expectations: %.2f files/s (expected > %.2f files/s)",
						filesPerSec, minExpectedRate)
				} else {
					t.Logf("✓ Performance meets expectations (%.2f files/s > %.2f files/s)",
						filesPerSec, minExpectedRate)
				}

				return
			}

			time.Sleep(500 * time.Millisecond)
		}
	}
}

// TestAcknowledgmentReliability tests that ACKs prevent message loss
func TestAcknowledgmentReliability(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping reliability test in short mode")
	}

	projectName := fmt.Sprintf("p2p-ack-reliability-%d", time.Now().Unix())

	testDir := t.TempDir()
	dockerDir := filepath.Join(testDir, "docker")
	configDir := filepath.Join(dockerDir, "config")

	copyDockerFiles(t, dockerDir)
	createTestConfig(t, configDir)
	createSyncDirectories(t, dockerDir)

	startDockerCompose(t, dockerDir, projectName)
	defer stopDockerCompose(t, dockerDir, projectName)

	waitForServices(t, projectName)

	t.Log("Testing acknowledgment reliability...")

	// Create a sequence of files with known content
	numFiles := 20
	expectedContents := make(map[string]string)

	for i := 0; i < numFiles; i++ {
		filename := fmt.Sprintf("reliability-%03d.txt", i)
		content := fmt.Sprintf("Reliability test %03d: %s", i, strings.Repeat("X", 100))
		expectedContents[filename] = content

		createFileInContainer(t, projectName, "peer-alpha", fmt.Sprintf("/app/sync/%s", filename), content)

		// Small delay between file creations to simulate real usage
		time.Sleep(50 * time.Millisecond)
	}

	// Wait for all files to sync
	maxWait := 60 * time.Second
	current := time.Now()
	timeoutCh := time.After(maxWait)

	for {
		select {
		case <-timeoutCh:
			// Count how many files synced correctly
			correctCount := 0
			for filename, expectedContent := range expectedContents {
				actualContent := readFileFromContainer(t, projectName, "peer-beta", fmt.Sprintf("/app/sync/%s", filename))
				if actualContent == expectedContent {
					correctCount++
				}
			}

			t.Fatalf("Reliability test failed: only %d/%d files synced correctly within %v",
				correctCount, numFiles, maxWait)

		default:
			// Check all files
			allCorrect := true
			for filename, expectedContent := range expectedContents {
				actualContent := readFileFromContainer(t, projectName, "peer-beta", fmt.Sprintf("/app/sync/%s", filename))
				if actualContent != expectedContent {
					allCorrect = false
					break
				}
			}

			if allCorrect {
				elapsed := time.Since(current)
				t.Logf("✓ Reliability test passed: all %d files synced correctly", numFiles)
				t.Logf("  Time taken: %v", elapsed)
				return
			}

			time.Sleep(500 * time.Millisecond)
		}
	}
}
