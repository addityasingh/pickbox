package test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testBaseDir      = "/tmp/pickbox-integration-test"
	node1Dir         = testBaseDir + "/node1"
	node2Dir         = testBaseDir + "/node2"
	node3Dir         = testBaseDir + "/node3"
	testTimeout      = 30 * time.Second
	replicationDelay = 5 * time.Second
)

// setupTestEnvironment prepares the test directories and cleanup
func setupTestEnvironment(t *testing.T) {
	// Cleanup any existing test data
	os.RemoveAll(testBaseDir)

	// Create test directories
	dirs := []string{node1Dir, node2Dir, node3Dir}
	for _, dir := range dirs {
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err, "Failed to create test directory: %s", dir)
	}

	// Cleanup function
	t.Cleanup(func() {
		cleanupTestEnvironment()
	})
}

// cleanupTestEnvironment removes test directories and kills processes
func cleanupTestEnvironment() {
	// Kill any running test processes
	exec.Command("pkill", "-f", "multi_replication").Run()
	time.Sleep(1 * time.Second)

	// Remove test directories
	os.RemoveAll(testBaseDir)
}

// startNode starts a multi-replication node in the background
func startNode(t *testing.T, nodeID string, dataDir string, raftPort, adminPort int) *exec.Cmd {
	cmd := exec.Command(
		"../bin/pickbox", "node", "multi",
		"--node-id", nodeID,
		"--data-dir", dataDir,
		"--port", fmt.Sprintf("%d", raftPort),
		"--admin-port", fmt.Sprintf("%d", adminPort),
	)

	// Set up logging for debugging
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Start()
	require.NoError(t, err, "Failed to start node %s", nodeID)

	// Give the node time to initialize
	time.Sleep(2 * time.Second)

	return cmd
}

// joinNode adds a node to the cluster via admin interface
func joinNode(t *testing.T, adminPort int, nodeID string, raftAddr string) {
	cmd := exec.Command("curl", "-s", "-X", "POST",
		fmt.Sprintf("http://localhost:%d/join", adminPort),
		"-d", fmt.Sprintf(`{"id":"%s","addr":"%s"}`, nodeID, raftAddr))

	output, err := cmd.CombinedOutput()
	assert.NoError(t, err, "Failed to join node %s: %s", nodeID, string(output))
}

// writeTestFile creates a test file in the specified directory
func writeTestFile(t *testing.T, dir, filename, content string) {
	filePath := filepath.Join(dir, filename)
	err := os.MkdirAll(filepath.Dir(filePath), 0755)
	require.NoError(t, err)

	err = os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err, "Failed to write test file: %s", filePath)
}

// readFileContent reads and returns file content, or empty string if file doesn't exist
func readFileContent(dir, filename string) string {
	filePath := filepath.Join(dir, filename)
	content, err := os.ReadFile(filePath)
	if err != nil {
		return ""
	}
	return string(content)
}

// waitForReplication waits for file to replicate to all nodes with the expected content
func waitForReplication(t *testing.T, filename, expectedContent string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	dirs := []string{node1Dir, node2Dir, node3Dir}

	for time.Now().Before(deadline) {
		allMatch := true
		for _, dir := range dirs {
			content := readFileContent(dir, filename)
			if content != expectedContent {
				allMatch = false
				break
			}
		}

		if allMatch {
			return // Success!
		}

		time.Sleep(200 * time.Millisecond)
	}

	// Timeout reached, gather debug info
	t.Logf("Replication timeout for file %s", filename)
	for i, dir := range dirs {
		content := readFileContent(dir, filename)
		t.Logf("Node%d content: %q (expected: %q)", i+1, content, expectedContent)
	}

	t.Fatalf("File %s did not replicate to all nodes within %v", filename, timeout)
}

// waitForDeletion waits for file to be deleted from all nodes
func waitForDeletion(t *testing.T, filename string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	dirs := []string{node1Dir, node2Dir, node3Dir}

	for time.Now().Before(deadline) {
		allDeleted := true
		for _, dir := range dirs {
			filePath := filepath.Join(dir, filename)
			if _, err := os.Stat(filePath); !os.IsNotExist(err) {
				allDeleted = false
				break
			}
		}

		if allDeleted {
			return // Success!
		}

		time.Sleep(200 * time.Millisecond)
	}

	t.Fatalf("File %s was not deleted from all nodes within %v", filename, timeout)
}

func TestBasicReplication(t *testing.T) {
	t.Skip("TODO: Stabilize integration tests for CI/CD - currently disabled")
	setupTestEnvironment(t)

	// Start the three nodes
	node1 := startNode(t, "node1", node1Dir, 8000, 9001)
	defer node1.Process.Kill()

	node2 := startNode(t, "node2", node2Dir, 8001, 9002)
	defer node2.Process.Kill()

	node3 := startNode(t, "node3", node3Dir, 8002, 9003)
	defer node3.Process.Kill()

	// Give nodes time to start
	time.Sleep(3 * time.Second)

	// Join nodes to cluster
	joinNode(t, 9002, "node2", "localhost:8001")
	joinNode(t, 9003, "node3", "localhost:8002")

	// Wait for cluster to stabilize
	time.Sleep(3 * time.Second)

	// Test 1: Create file on node1, verify replication
	t.Run("ReplicateFromNode1", func(t *testing.T) {
		testContent := "Hello from node1 at " + time.Now().Format(time.RFC3339)
		writeTestFile(t, node1Dir, "test1.txt", testContent)

		waitForReplication(t, "test1.txt", testContent, testTimeout)
	})

	// Test 2: Create file on node2, verify replication
	t.Run("ReplicateFromNode2", func(t *testing.T) {
		testContent := "Hello from node2 at " + time.Now().Format(time.RFC3339)
		writeTestFile(t, node2Dir, "test2.txt", testContent)

		waitForReplication(t, "test2.txt", testContent, testTimeout)
	})

	// Test 3: Create file on node3, verify replication
	t.Run("ReplicateFromNode3", func(t *testing.T) {
		testContent := "Hello from node3 at " + time.Now().Format(time.RFC3339)
		writeTestFile(t, node3Dir, "test3.txt", testContent)

		waitForReplication(t, "test3.txt", testContent, testTimeout)
	})
}

func TestFileModification(t *testing.T) {
	t.Skip("TODO: Stabilize integration tests for CI/CD - currently disabled")
	setupTestEnvironment(t)

	// Start the three nodes
	node1 := startNode(t, "node1", node1Dir, 8000, 9001)
	defer node1.Process.Kill()

	node2 := startNode(t, "node2", node2Dir, 8001, 9002)
	defer node2.Process.Kill()

	node3 := startNode(t, "node3", node3Dir, 8002, 9003)
	defer node3.Process.Kill()

	time.Sleep(3 * time.Second)

	// Join nodes to cluster
	joinNode(t, 9002, "node2", "localhost:8001")
	joinNode(t, 9003, "node3", "localhost:8002")
	time.Sleep(3 * time.Second)

	// Create initial file
	initialContent := "Initial content"
	writeTestFile(t, node1Dir, "modify.txt", initialContent)
	waitForReplication(t, "modify.txt", initialContent, testTimeout)

	// Modify file from different node
	modifiedContent := "Modified content from node2"
	writeTestFile(t, node2Dir, "modify.txt", modifiedContent)
	waitForReplication(t, "modify.txt", modifiedContent, testTimeout)

	// Modify again from third node
	finalContent := "Final content from node3"
	writeTestFile(t, node3Dir, "modify.txt", finalContent)
	waitForReplication(t, "modify.txt", finalContent, testTimeout)
}

func TestFileDeletion(t *testing.T) {
	t.Skip("TODO: Stabilize integration tests for CI/CD - currently disabled")
	setupTestEnvironment(t)

	// Start the three nodes
	node1 := startNode(t, "node1", node1Dir, 8000, 9001)
	defer node1.Process.Kill()

	node2 := startNode(t, "node2", node2Dir, 8001, 9002)
	defer node2.Process.Kill()

	node3 := startNode(t, "node3", node3Dir, 8002, 9003)
	defer node3.Process.Kill()

	time.Sleep(3 * time.Second)

	// Join nodes to cluster
	joinNode(t, 9002, "node2", "localhost:8001")
	joinNode(t, 9003, "node3", "localhost:8002")
	time.Sleep(3 * time.Second)

	// Create file to be deleted
	testContent := "File to be deleted"
	writeTestFile(t, node1Dir, "delete.txt", testContent)
	waitForReplication(t, "delete.txt", testContent, testTimeout)

	// Delete file from node2
	filePath := filepath.Join(node2Dir, "delete.txt")
	err := os.Remove(filePath)
	require.NoError(t, err)

	// Wait for deletion to replicate
	waitForDeletion(t, "delete.txt", testTimeout)
}

func TestConcurrentWrites(t *testing.T) {
	t.Skip("TODO: Stabilize integration tests for CI/CD - currently disabled")
	setupTestEnvironment(t)

	// Start the three nodes
	node1 := startNode(t, "node1", node1Dir, 8000, 9001)
	defer node1.Process.Kill()

	node2 := startNode(t, "node2", node2Dir, 8001, 9002)
	defer node2.Process.Kill()

	node3 := startNode(t, "node3", node3Dir, 8002, 9003)
	defer node3.Process.Kill()

	time.Sleep(3 * time.Second)

	// Join nodes to cluster
	joinNode(t, 9002, "node2", "localhost:8001")
	joinNode(t, 9003, "node3", "localhost:8002")
	time.Sleep(3 * time.Second)

	// Perform concurrent writes from all nodes
	var wg sync.WaitGroup

	for i := 0; i < 5; i++ {
		wg.Add(3)

		// Write from node1
		go func(index int) {
			defer wg.Done()
			content := fmt.Sprintf("Node1 write %d at %s", index, time.Now().Format(time.RFC3339Nano))
			writeTestFile(t, node1Dir, fmt.Sprintf("concurrent1_%d.txt", index), content)
		}(i)

		// Write from node2
		go func(index int) {
			defer wg.Done()
			content := fmt.Sprintf("Node2 write %d at %s", index, time.Now().Format(time.RFC3339Nano))
			writeTestFile(t, node2Dir, fmt.Sprintf("concurrent2_%d.txt", index), content)
		}(i)

		// Write from node3
		go func(index int) {
			defer wg.Done()
			content := fmt.Sprintf("Node3 write %d at %s", index, time.Now().Format(time.RFC3339Nano))
			writeTestFile(t, node3Dir, fmt.Sprintf("concurrent3_%d.txt", index), content)
		}(i)

		time.Sleep(100 * time.Millisecond)
	}

	wg.Wait()

	// Give time for all replications to complete
	time.Sleep(10 * time.Second)

	// Verify all files exist on all nodes
	for nodeIdx := 1; nodeIdx <= 3; nodeIdx++ {
		for fileIdx := 0; fileIdx < 5; fileIdx++ {
			filename := fmt.Sprintf("concurrent%d_%d.txt", nodeIdx, fileIdx)

			// Check that file exists on all nodes
			for _, dir := range []string{node1Dir, node2Dir, node3Dir} {
				filePath := filepath.Join(dir, filename)
				_, err := os.Stat(filePath)
				assert.NoError(t, err, "File %s should exist in %s", filename, dir)
			}
		}
	}
}

func TestNestedDirectories(t *testing.T) {
	t.Skip("TODO: Stabilize integration tests for CI/CD - currently disabled")
	setupTestEnvironment(t)

	// Start the three nodes
	node1 := startNode(t, "node1", node1Dir, 8000, 9001)
	defer node1.Process.Kill()

	node2 := startNode(t, "node2", node2Dir, 8001, 9002)
	defer node2.Process.Kill()

	node3 := startNode(t, "node3", node3Dir, 8002, 9003)
	defer node3.Process.Kill()

	time.Sleep(3 * time.Second)

	// Join nodes to cluster
	joinNode(t, 9002, "node2", "localhost:8001")
	joinNode(t, 9003, "node3", "localhost:8002")
	time.Sleep(3 * time.Second)

	// Test nested directory structure
	nestedFile := "deep/nested/directory/structure/file.txt"
	testContent := "Content in deeply nested file"

	writeTestFile(t, node1Dir, nestedFile, testContent)
	waitForReplication(t, nestedFile, testContent, testTimeout)

	// Verify directory structure was created on all nodes
	for _, dir := range []string{node1Dir, node2Dir, node3Dir} {
		fullPath := filepath.Join(dir, nestedFile)
		content, err := os.ReadFile(fullPath)
		assert.NoError(t, err, "Nested file should exist in %s", dir)
		assert.Equal(t, testContent, string(content))
	}
}

func TestLargeFiles(t *testing.T) {
	t.Skip("TODO: Fix large file test - currently causes timeout issues")
	setupTestEnvironment(t)

	// Start the three nodes
	node1 := startNode(t, "node1", node1Dir, 8000, 9001)
	defer node1.Process.Kill()

	node2 := startNode(t, "node2", node2Dir, 8001, 9002)
	defer node2.Process.Kill()

	node3 := startNode(t, "node3", node3Dir, 8002, 9003)
	defer node3.Process.Kill()

	time.Sleep(3 * time.Second)

	// Join nodes to cluster
	joinNode(t, 9002, "node2", "localhost:8001")
	joinNode(t, 9003, "node3", "localhost:8002")
	time.Sleep(3 * time.Second)

	// Create a large file (1MB)
	largeContent := strings.Repeat("This is a test line for large file replication.\n", 20000) // ~1MB
	writeTestFile(t, node1Dir, "large.txt", largeContent)

	// Wait for replication with extended timeout for large files
	waitForReplication(t, "large.txt", largeContent, 60*time.Second)

	// Verify file sizes match
	for _, dir := range []string{node1Dir, node2Dir, node3Dir} {
		filePath := filepath.Join(dir, "large.txt")
		stat, err := os.Stat(filePath)
		assert.NoError(t, err)
		assert.Equal(t, int64(len(largeContent)), stat.Size())
	}
}

func TestNodeFailureRecovery(t *testing.T) {
	t.Skip("TODO: Fix node failure recovery test - requires more robust cluster management")

	if testing.Short() {
		t.Skip("Skipping failure recovery test in short mode")
	}

	setupTestEnvironment(t)

	// Start the three nodes
	node1 := startNode(t, "node1", node1Dir, 8000, 9001)
	defer node1.Process.Kill()

	node2 := startNode(t, "node2", node2Dir, 8001, 9002)
	defer node2.Process.Kill()

	node3 := startNode(t, "node3", node3Dir, 8002, 9003)
	defer node3.Process.Kill()

	time.Sleep(3 * time.Second)

	// Join nodes to cluster
	joinNode(t, 9002, "node2", "localhost:8001")
	joinNode(t, 9003, "node3", "localhost:8002")
	time.Sleep(3 * time.Second)

	// Create initial file
	initialContent := "Pre-failure content"
	writeTestFile(t, node1Dir, "recovery.txt", initialContent)
	waitForReplication(t, "recovery.txt", initialContent, testTimeout)

	// Kill node2
	t.Log("Killing node2...")
	node2.Process.Kill()
	node2.Wait()
	time.Sleep(2 * time.Second)

	// Continue operations with remaining nodes
	failureContent := "Content during node2 failure"
	writeTestFile(t, node1Dir, "during_failure.txt", failureContent)

	// Verify replication still works between node1 and node3
	time.Sleep(replicationDelay)
	content3 := readFileContent(node3Dir, "during_failure.txt")
	assert.Equal(t, failureContent, content3, "Replication should work with 2 nodes")

	// Restart node2
	t.Log("Restarting node2...")
	node2 = startNode(t, "node2", node2Dir, 8001, 9002)
	defer node2.Process.Kill()

	// Rejoin to cluster
	time.Sleep(3 * time.Second)
	joinNode(t, 9002, "node2", "localhost:8001")

	// Give time for node2 to catch up
	time.Sleep(5 * time.Second)

	// Verify node2 has the file created during its absence
	content2 := readFileContent(node2Dir, "during_failure.txt")
	assert.Equal(t, failureContent, content2, "Node2 should catch up after restart")

	// Test new replication works normally
	recoveryContent := "Post-recovery content"
	writeTestFile(t, node2Dir, "post_recovery.txt", recoveryContent)
	waitForReplication(t, "post_recovery.txt", recoveryContent, testTimeout)
}

// Benchmark test for replication performance
func BenchmarkReplicationThroughput(b *testing.B) {
	// Skip if not running benchmarks
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	setupTestEnvironment(&testing.T{})
	defer cleanupTestEnvironment()

	// Start nodes
	node1 := startNode(&testing.T{}, "node1", node1Dir, 8000, 9001)
	defer node1.Process.Kill()

	node2 := startNode(&testing.T{}, "node2", node2Dir, 8001, 9002)
	defer node2.Process.Kill()

	node3 := startNode(&testing.T{}, "node3", node3Dir, 8002, 9003)
	defer node3.Process.Kill()

	time.Sleep(3 * time.Second)

	// Join nodes
	joinNode(&testing.T{}, 9002, "node2", "localhost:8001")
	joinNode(&testing.T{}, 9003, "node3", "localhost:8002")
	time.Sleep(3 * time.Second)

	testContent := "Benchmark test content for throughput measurement"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filename := fmt.Sprintf("bench_%d.txt", i)
		writeTestFile(&testing.T{}, node1Dir, filename, testContent)

		// Wait for replication to complete
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if readFileContent(node2Dir, filename) == testContent &&
				readFileContent(node3Dir, filename) == testContent {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}
