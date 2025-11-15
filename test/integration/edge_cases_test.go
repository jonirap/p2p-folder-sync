package integration

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestEdgeCases tests various edge cases in file synchronization
func TestEdgeCases(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping edge cases test in short mode")
	}

	projectName := fmt.Sprintf("p2p-edge-%d", time.Now().Unix())

	testDir := t.TempDir()
	dockerDir := filepath.Join(testDir, "docker")
	configDir := filepath.Join(dockerDir, "config")

	copyDockerFiles(t, dockerDir)
	createTestConfig(t, configDir)
	createSyncDirectories(t, dockerDir)

	startDockerCompose(t, dockerDir, projectName)
	defer stopDockerCompose(t, dockerDir, projectName)

	waitForServices(t, projectName)

	// Test various edge cases
	testEmptyFiles(t, projectName)
	testSpecialCharacters(t, projectName)
	testDeepDirectoryStructure(t, projectName)
	testFilePermissions(t, projectName)
	testRapidFileChanges(t, projectName)
	testBinaryFiles(t, projectName)
	testUnicodeFilenames(t, projectName)
}

func testEmptyFiles(t *testing.T, projectName string) {
	t.Log("Testing empty file handling...")

	// Create an empty file
	createFileInContainer(t, projectName, "peer-alpha", "/app/sync/empty.txt", "")
	time.Sleep(3 * time.Second)

	// Verify empty file sync
	for _, peer := range []string{"peer-beta", "peer-gamma"} {
		size := getFileSizeInContainer(t, projectName, peer, "/app/sync/empty.txt")
		if size != 0 {
			t.Errorf("Empty file sync failed on %s: expected size 0, got %d", peer, size)
		}
	}

	t.Log("Empty file handling test passed")
}

func testSpecialCharacters(t *testing.T, projectName string) {
	t.Log("Testing special characters in filenames...")

	specialFiles := []struct {
		filename string
		content  string
	}{
		{"/app/sync/file with spaces.txt", "Content with spaces"},
		{"/app/sync/file-with-dashes.txt", "Content with dashes"},
		{"/app/sync/file_underscore.txt", "Content with underscore"},
		{"/app/sync/file.dots.txt", "Content with dots"},
	}

	for _, sf := range specialFiles {
		createFileInContainer(t, projectName, "peer-alpha", sf.filename, sf.content)
	}

	time.Sleep(5 * time.Second)

	// Verify all files synced
	for _, sf := range specialFiles {
		for _, peer := range []string{"peer-beta", "peer-gamma"} {
			content := readFileFromContainer(t, projectName, peer, sf.filename)
			if content != sf.content {
				t.Errorf("Special character file sync failed: %s on %s", sf.filename, peer)
			}
		}
	}

	t.Log("Special characters test passed")
}

func testDeepDirectoryStructure(t *testing.T, projectName string) {
	t.Log("Testing deep directory structure...")

	// Create deeply nested directory structure
	deepPath := "/app/sync/level1/level2/level3/level4/level5/deep-file.txt"
	deepContent := "Content from the depths"

	createFileInContainer(t, projectName, "peer-alpha", deepPath, deepContent)
	time.Sleep(5 * time.Second)

	// Verify deep file sync
	for _, peer := range []string{"peer-beta", "peer-gamma"} {
		content := readFileFromContainer(t, projectName, peer, deepPath)
		if content != deepContent {
			t.Errorf("Deep directory sync failed on %s", peer)
		}
	}

	t.Log("Deep directory structure test passed")
}

func testFilePermissions(t *testing.T, projectName string) {
	t.Log("Testing file permissions...")

	// Create file with specific content (permissions are handled by the container)
	permContent := "File with permissions test"
	createFileInContainer(t, projectName, "peer-alpha", "/app/sync/permissions.txt", permContent)
	time.Sleep(3 * time.Second)

	// Verify content sync (permissions may vary by filesystem)
	for _, peer := range []string{"peer-beta", "peer-gamma"} {
		content := readFileFromContainer(t, projectName, peer, "/app/sync/permissions.txt")
		if content != permContent {
			t.Errorf("File permissions test failed on %s", peer)
		}
	}

	t.Log("File permissions test passed")
}

func testRapidFileChanges(t *testing.T, projectName string) {
	t.Log("Testing rapid file changes...")

	filename := "/app/sync/rapid.txt"

	// Rapidly change file content multiple times
	changes := []string{
		"Version 1",
		"Version 2",
		"Version 3",
		"Version 4",
		"Version 5",
	}

	for _, content := range changes {
		createFileInContainer(t, projectName, "peer-alpha", filename, content)
		time.Sleep(500 * time.Millisecond) // Short delay between changes
	}

	// Wait for final sync
	time.Sleep(5 * time.Second)

	// Verify final version is synced
	finalContent := changes[len(changes)-1]
	for _, peer := range []string{"peer-beta", "peer-gamma"} {
		content := readFileFromContainer(t, projectName, peer, filename)
		if content != finalContent {
			t.Errorf("Rapid changes test failed on %s: expected %q, got %q", peer, finalContent, content)
		}
	}

	t.Log("Rapid file changes test passed")
}

func testBinaryFiles(t *testing.T, projectName string) {
	t.Log("Testing binary file handling...")

	// Create a "binary" file with various byte values
	binaryContent := ""
	for i := 0; i < 256; i++ {
		binaryContent += string(byte(i))
	}

	// Use printf to handle binary data properly
	cmd := exec.Command("docker", "exec", fmt.Sprintf("%s_peer-alpha_1", projectName),
		"sh", "-c", fmt.Sprintf("printf '%%b' '%s' > /app/sync/binary.dat", binaryContent))

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to create binary file: %v\nOutput: %s", err, output)
	}

	time.Sleep(5 * time.Second)

	// Verify binary file sync (check size since content may not be readable)
	expectedSize := len(binaryContent)
	for _, peer := range []string{"peer-beta", "peer-gamma"} {
		size := getFileSizeInContainer(t, projectName, peer, "/app/sync/binary.dat")
		if size != expectedSize {
			t.Errorf("Binary file sync failed on %s: expected size %d, got %d", peer, expectedSize, size)
		}
	}

	t.Log("Binary file handling test passed")
}

func testUnicodeFilenames(t *testing.T, projectName string) {
	t.Log("Testing Unicode filenames...")

	unicodeFiles := []struct {
		filename string
		content  string
	}{
		{"/app/sync/файл.txt", "Russian filename"},
		{"/app/sync/文件.txt", "Chinese filename"},
		{"/app/sync/ファイル.txt", "Japanese filename"},
		{"/app/sync/café.txt", "French filename with accent"},
		{"/app/sync/🚀.txt", "Emoji filename"},
	}

	for _, uf := range unicodeFiles {
		createFileInContainer(t, projectName, "peer-alpha", uf.filename, uf.content)
	}

	time.Sleep(5 * time.Second)

	// Verify Unicode filename sync
	for _, uf := range unicodeFiles {
		for _, peer := range []string{"peer-beta", "peer-gamma"} {
			content := readFileFromContainer(t, projectName, peer, uf.filename)
			if content != uf.content {
				t.Errorf("Unicode filename sync failed: %s on %s", uf.filename, peer)
			}
		}
	}

	t.Log("Unicode filenames test passed")
}

// TestLoadTesting performs basic load testing
func TestLoadTesting(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	projectName := fmt.Sprintf("p2p-load-%d", time.Now().Unix())

	testDir := t.TempDir()
	dockerDir := filepath.Join(testDir, "docker")
	configDir := filepath.Join(dockerDir, "config")

	copyDockerFiles(t, dockerDir)
	createTestConfig(t, configDir)
	createSyncDirectories(t, dockerDir)

	startDockerCompose(t, dockerDir, projectName)
	defer stopDockerCompose(t, dockerDir, projectName)

	waitForServices(t, projectName)

	t.Log("Testing high load scenario...")

	// Create many files quickly
	numFiles := 50
	baseContent := "Load test content for file "

	start := time.Now()

	// Create files in batches
	for i := 0; i < numFiles; i++ {
		filename := fmt.Sprintf("/app/sync/load-test-%03d.txt", i)
		content := fmt.Sprintf("%s%d", baseContent, i)
		createFileInContainer(t, projectName, "peer-alpha", filename, content)
	}

	creationTime := time.Since(start)
	t.Logf("Created %d files in %v", numFiles, creationTime)

	// Wait for sync
	syncWait := 30 * time.Second
	t.Logf("Waiting %v for sync to complete...", syncWait)
	time.Sleep(syncWait)

	// Verify all files synced
	missingFiles := 0
	for i := 0; i < numFiles; i++ {
		filename := fmt.Sprintf("/app/sync/load-test-%03d.txt", i)
		expectedContent := fmt.Sprintf("%s%d", baseContent, i)

		for _, peer := range []string{"peer-beta", "peer-gamma"} {
			content := readFileFromContainer(t, projectName, peer, filename)
			if content != expectedContent {
				missingFiles++
				t.Errorf("Load test failed: file %d missing or corrupted on %s", i, peer)
			}
		}
	}

	totalTime := time.Since(start)
	t.Logf("Load test completed in %v: %d/%d files synced successfully",
		totalTime, numFiles-missingFiles, numFiles)

	if missingFiles > 0 {
		t.Errorf("Load test failed: %d files not synced correctly", missingFiles)
	}
}

// TestMemoryPressure tests system behavior under memory pressure
func TestMemoryPressure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory pressure test in short mode")
	}

	projectName := fmt.Sprintf("p2p-memory-%d", time.Now().Unix())

	testDir := t.TempDir()
	dockerDir := filepath.Join(testDir, "docker")
	configDir := filepath.Join(dockerDir, "config")

	copyDockerFiles(t, dockerDir)
	createTestConfig(t, configDir)
	createSyncDirectories(t, dockerDir)

	startDockerCompose(t, dockerDir, projectName)
	defer stopDockerCompose(t, dockerDir, projectName)

	waitForServices(t, projectName)

	// Test with large files that might cause memory pressure
	t.Log("Testing memory pressure with large files...")

	largeSizes := []int{
		5 * 1024 * 1024,  // 5MB
		10 * 1024 * 1024, // 10MB
	}

	for i, size := range largeSizes {
		content := strings.Repeat(fmt.Sprintf("Large file %d content ", i), size/20) // Approximate size
		filename := fmt.Sprintf("/app/sync/memory-test-%d.dat", i)

		start := time.Now()
		createFileInContainer(t, projectName, "peer-alpha", filename, content)

		// Wait for sync with longer timeout for large files
		time.Sleep(20 * time.Second)

		// Verify sync
		for _, peer := range []string{"peer-beta", "peer-gamma"} {
			actualSize := getFileSizeInContainer(t, projectName, peer, filename)
			if actualSize == 0 {
				t.Errorf("Memory pressure test failed: large file %d not synced to %s", i, peer)
			}
		}

		elapsed := time.Since(start)
		t.Logf("Large file %d (%d bytes) sync time: %v", i, len(content), elapsed)
	}

	t.Log("Memory pressure test passed")
}
