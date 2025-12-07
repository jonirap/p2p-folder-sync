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
