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
	nNodeTestBaseDir      = "/tmp/pickbox-n-node-integration-test"
	nNodeTestTimeout      = 60 * time.Second
	clusterStartDelay     = 15 * time.Second
	nNodeReplicationDelay = 5 * time.Second
)

// nNodeTestEnvironment manages test environment for N-node clusters
type nNodeTestEnvironment struct {
	nodeCount   int
	basePort    int
	adminPort   int
	monitorPort int
	baseDir     string
	nodeDirs    []string
	processes   []*exec.Cmd
}

// setupNNodeTestEnvironment prepares test environment for N nodes
func setupNNodeTestEnvironment(t *testing.T, nodeCount int) *nNodeTestEnvironment {
	env := &nNodeTestEnvironment{
		nodeCount:   nodeCount,
		basePort:    8001,
		adminPort:   9001,
		monitorPort: 6001,
		baseDir:     nNodeTestBaseDir,
		nodeDirs:    make([]string, nodeCount),
		processes:   make([]*exec.Cmd, nodeCount),
	}

	// Clean up any existing test data
	os.RemoveAll(env.baseDir)

	// Create directories for all nodes
	for i := 0; i < nodeCount; i++ {
		nodeDir := filepath.Join(env.baseDir, fmt.Sprintf("node%d", i+1))
		env.nodeDirs[i] = nodeDir
		err := os.MkdirAll(nodeDir, 0755)
		require.NoError(t, err, "Failed to create directory for node%d", i+1)
	}

	// Setup cleanup
	t.Cleanup(func() {
		env.cleanup()
	})

	return env
}

// startCluster starts an N-node cluster using the cluster manager
func (env *nNodeTestEnvironment) startCluster(t *testing.T) {
	// Use the cluster manager script to start the cluster
	cmd := exec.Command("./scripts/cluster_manager.sh", "start", "-n", fmt.Sprintf("%d", env.nodeCount))
	cmd.Dir = ".." // Run from project root

	// Capture output for debugging
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Cluster start output: %s", string(output))
		require.NoError(t, err, "Failed to start %d-node cluster", env.nodeCount)
	}

	// Wait for cluster to stabilize
	time.Sleep(clusterStartDelay)
}

// stopCluster stops the cluster using the cluster manager
func (env *nNodeTestEnvironment) stopCluster(t *testing.T) {
	cmd := exec.Command("./scripts/cluster_manager.sh", "stop", "-n", fmt.Sprintf("%d", env.nodeCount))
	cmd.Dir = ".." // Run from project root

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Cluster stop output: %s", string(output))
		// Don't fail test on cleanup errors
		t.Logf("Warning: Failed to stop cluster cleanly: %v", err)
	}
}

// checkClusterStatus verifies all nodes are running and listening
func (env *nNodeTestEnvironment) checkClusterStatus(t *testing.T) {
	cmd := exec.Command("./scripts/cluster_manager.sh", "status", "-n", fmt.Sprintf("%d", env.nodeCount))
	cmd.Dir = ".." // Run from project root

	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed to check cluster status")

	outputStr := string(output)

	// Verify all nodes are reported as RUNNING
	for i := 1; i <= env.nodeCount; i++ {
		nodeStatus := fmt.Sprintf("Node %d", i)
		assert.Contains(t, outputStr, nodeStatus, "Node %d should be in status output", i)

		// Look for RUNNING status for this node
		lines := strings.Split(outputStr, "\n")
		nodeFound := false
		for j, line := range lines {
			if strings.Contains(line, nodeStatus) && j+4 < len(lines) {
				// Check the Status line (should be 4 lines after the node header)
				statusLine := lines[j+4]
				assert.Contains(t, statusLine, "RUNNING", "Node %d should be RUNNING", i)
				nodeFound = true
				break
			}
		}
		assert.True(t, nodeFound, "Could not find status for node %d", i)
	}
}

// writeTestFileToNode creates a test file in a specific node's directory
func (env *nNodeTestEnvironment) writeTestFileToNode(t *testing.T, nodeNum int, filename, content string) {
	require.True(t, nodeNum >= 1 && nodeNum <= env.nodeCount, "Invalid node number %d", nodeNum)

	nodeDataDir := filepath.Join("..", "data", fmt.Sprintf("node%d", nodeNum))
	filePath := filepath.Join(nodeDataDir, filename)

	err := os.MkdirAll(filepath.Dir(filePath), 0755)
	require.NoError(t, err, "Failed to create directory for %s", filePath)

	err = os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err, "Failed to write test file %s", filePath)
}

// waitForReplicationToAllNodes waits for a file to replicate to all nodes
func (env *nNodeTestEnvironment) waitForReplicationToAllNodes(t *testing.T, filename, expectedContent string) {
	deadline := time.Now().Add(nNodeTestTimeout)

	for time.Now().Before(deadline) {
		allMatch := true

		for i := 1; i <= env.nodeCount; i++ {
			nodeDataDir := filepath.Join("..", "data", fmt.Sprintf("node%d", i))
			filePath := filepath.Join(nodeDataDir, filename)

			content, err := os.ReadFile(filePath)
			if err != nil || string(content) != expectedContent {
				allMatch = false
				break
			}
		}

		if allMatch {
			return // Success!
		}

		time.Sleep(500 * time.Millisecond)
	}

	// Timeout reached, gather debug info
	t.Logf("Replication timeout for file %s", filename)
	for i := 1; i <= env.nodeCount; i++ {
		nodeDataDir := filepath.Join("..", "data", fmt.Sprintf("node%d", i))
		filePath := filepath.Join(nodeDataDir, filename)

		content, err := os.ReadFile(filePath)
		if err != nil {
			t.Logf("Node%d: File not found or error: %v", i, err)
		} else {
			t.Logf("Node%d content: %q (expected: %q)", i, string(content), expectedContent)
		}
	}

	t.Fatalf("File %s did not replicate to all %d nodes within %v", filename, env.nodeCount, nNodeTestTimeout)
}

// verifyFileExistsOnAllNodes checks that a file exists on all nodes with the same content
func (env *nNodeTestEnvironment) verifyFileExistsOnAllNodes(t *testing.T, filename string) string {
	var firstContent string

	for i := 1; i <= env.nodeCount; i++ {
		nodeDataDir := filepath.Join("..", "data", fmt.Sprintf("node%d", i))
		filePath := filepath.Join(nodeDataDir, filename)

		content, err := os.ReadFile(filePath)
		require.NoError(t, err, "File %s should exist on node%d", filename, i)

		if i == 1 {
			firstContent = string(content)
		} else {
			assert.Equal(t, firstContent, string(content), "File content should be identical on all nodes")
		}
	}

	return firstContent
}

// cleanup removes test directories and kills processes
func (env *nNodeTestEnvironment) cleanup() {
	// Stop cluster
	exec.Command("./scripts/cluster_manager.sh", "stop", "-n", fmt.Sprintf("%d", env.nodeCount)).Run()
	time.Sleep(2 * time.Second)

	// Remove test directories
	os.RemoveAll(env.baseDir)
}

// Test 3-node cluster formation and basic replication
func TestThreeNodeCluster(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupNNodeTestEnvironment(t, 3)

	// Start cluster
	env.startCluster(t)
	defer env.stopCluster(t)

	// Verify cluster status
	env.checkClusterStatus(t)

	// Test replication from each node
	for nodeNum := 1; nodeNum <= 3; nodeNum++ {
		t.Run(fmt.Sprintf("replication_from_node%d", nodeNum), func(t *testing.T) {
			filename := fmt.Sprintf("test_from_node%d.txt", nodeNum)
			content := fmt.Sprintf("Hello from node%d at %s", nodeNum, time.Now().Format(time.RFC3339))

			env.writeTestFileToNode(t, nodeNum, filename, content)
			env.waitForReplicationToAllNodes(t, filename, content)
		})
	}
}

// Test 5-node cluster formation and replication
func TestFiveNodeCluster(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupNNodeTestEnvironment(t, 5)

	// Start cluster
	env.startCluster(t)
	defer env.stopCluster(t)

	// Verify cluster status
	env.checkClusterStatus(t)

	// Test multi-directional replication
	testContent := fmt.Sprintf("5-node cluster test at %s", time.Now().Format(time.RFC3339))
	env.writeTestFileToNode(t, 3, "five_node_test.txt", testContent) // Write to middle node
	env.waitForReplicationToAllNodes(t, "five_node_test.txt", testContent)

	// Test concurrent writes from multiple nodes
	t.Run("concurrent_writes", func(t *testing.T) {
		var wg sync.WaitGroup

		for i := 1; i <= 5; i++ {
			wg.Add(1)
			go func(nodeNum int) {
				defer wg.Done()

				filename := fmt.Sprintf("concurrent_%d.txt", nodeNum)
				content := fmt.Sprintf("Concurrent write from node%d at %s", nodeNum, time.Now().Format(time.RFC3339))

				env.writeTestFileToNode(t, nodeNum, filename, content)
			}(i)
		}

		wg.Wait()

		// Wait for all files to replicate
		time.Sleep(nNodeReplicationDelay)

		// Verify all files exist on all nodes
		for i := 1; i <= 5; i++ {
			filename := fmt.Sprintf("concurrent_%d.txt", i)
			env.verifyFileExistsOnAllNodes(t, filename)
		}
	})
}

// Test 7-node cluster formation
func TestSevenNodeCluster(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupNNodeTestEnvironment(t, 7)

	// Start cluster
	env.startCluster(t)
	defer env.stopCluster(t)

	// Verify cluster status
	env.checkClusterStatus(t)

	// Test replication with larger cluster
	testContent := fmt.Sprintf("7-node cluster test at %s", time.Now().Format(time.RFC3339))
	env.writeTestFileToNode(t, 1, "seven_node_test.txt", testContent)
	env.waitForReplicationToAllNodes(t, "seven_node_test.txt", testContent)
}

// Test large file replication across N nodes
func TestLargeFileReplication(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupNNodeTestEnvironment(t, 5)

	// Start cluster
	env.startCluster(t)
	defer env.stopCluster(t)

	// Create a large test file (10KB)
	largeContent := strings.Repeat("This is a large test file for replication testing. ", 200)
	filename := "large_file_test.txt"

	env.writeTestFileToNode(t, 2, filename, largeContent)
	env.waitForReplicationToAllNodes(t, filename, largeContent)

	// Verify file size on all nodes
	for i := 1; i <= 5; i++ {
		nodeDataDir := filepath.Join("..", "data", fmt.Sprintf("node%d", i))
		filePath := filepath.Join(nodeDataDir, filename)

		info, err := os.Stat(filePath)
		require.NoError(t, err, "Large file should exist on node%d", i)
		assert.Equal(t, int64(len(largeContent)), info.Size(), "File size should match on node%d", i)
	}
}

// Test nested directory replication
func TestNestedDirectoryReplication(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupNNodeTestEnvironment(t, 3)

	// Start cluster
	env.startCluster(t)
	defer env.stopCluster(t)

	// Create nested directory structure
	nestedPath := "deep/nested/directory/structure/test.txt"
	content := "Nested directory test content"

	env.writeTestFileToNode(t, 1, nestedPath, content)
	env.waitForReplicationToAllNodes(t, nestedPath, content)
}

// Test file modification replication
func TestFileModificationReplication(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupNNodeTestEnvironment(t, 3)

	// Start cluster
	env.startCluster(t)
	defer env.stopCluster(t)

	filename := "modification_test.txt"

	// Create initial file
	initialContent := "Initial content"
	env.writeTestFileToNode(t, 1, filename, initialContent)
	env.waitForReplicationToAllNodes(t, filename, initialContent)

	// Modify the file
	modifiedContent := "Modified content at " + time.Now().Format(time.RFC3339)
	env.writeTestFileToNode(t, 1, filename, modifiedContent)
	env.waitForReplicationToAllNodes(t, filename, modifiedContent)
}

// Test cluster scaling (simulated by testing different sizes)
func TestClusterScaling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Test different cluster sizes sequentially
	clusterSizes := []int{1, 3, 5}

	for _, size := range clusterSizes {
		t.Run(fmt.Sprintf("cluster_size_%d", size), func(t *testing.T) {
			env := setupNNodeTestEnvironment(t, size)

			// Start cluster
			env.startCluster(t)
			defer env.stopCluster(t)

			// Verify cluster status
			env.checkClusterStatus(t)

			// Test basic replication
			filename := fmt.Sprintf("scale_test_%d.txt", size)
			content := fmt.Sprintf("Scaling test for %d nodes", size)

			env.writeTestFileToNode(t, 1, filename, content)
			env.waitForReplicationToAllNodes(t, filename, content)
		})
	}
}

// Test cluster with different port ranges
func TestClusterWithCustomPorts(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Clean up any existing processes
	exec.Command("./scripts/cluster_manager.sh", "stop", "-n", "3").Run()
	time.Sleep(2 * time.Second)

	// Start cluster with custom ports
	cmd := exec.Command("./scripts/cluster_manager.sh", "start", "-n", "3", "-p", "18001", "-a", "19001")
	cmd.Dir = ".."

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Custom port cluster output: %s", string(output))
		require.NoError(t, err, "Failed to start cluster with custom ports")
	}

	defer func() {
		// Cleanup with custom ports
		stopCmd := exec.Command("./scripts/cluster_manager.sh", "stop", "-n", "3")
		stopCmd.Dir = ".."
		stopCmd.Run()
	}()

	// Wait for cluster to start
	time.Sleep(clusterStartDelay)

	// Verify cluster is running with custom ports
	statusCmd := exec.Command("./scripts/cluster_manager.sh", "status", "-n", "3")
	statusCmd.Dir = ".."

	statusOutput, err := statusCmd.CombinedOutput()
	require.NoError(t, err, "Failed to check custom port cluster status")

	statusStr := string(statusOutput)

	// Verify custom ports are being used
	assert.Contains(t, statusStr, "18001", "Should use custom raft port")
	assert.Contains(t, statusStr, "19001", "Should use custom admin port")
}

// Test rapid cluster start/stop cycles
func TestClusterStartStopCycles(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	nodeCount := 3
	cycles := 3

	for cycle := 1; cycle <= cycles; cycle++ {
		t.Run(fmt.Sprintf("cycle_%d", cycle), func(t *testing.T) {
			// Start cluster
			cmd := exec.Command("./scripts/cluster_manager.sh", "start", "-n", fmt.Sprintf("%d", nodeCount))
			cmd.Dir = ".."

			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Logf("Cycle %d start output: %s", cycle, string(output))
				require.NoError(t, err, "Failed to start cluster in cycle %d", cycle)
			}

			// Wait for startup
			time.Sleep(10 * time.Second)

			// Quick test
			testFile := filepath.Join("..", "data", "node1", fmt.Sprintf("cycle_%d.txt", cycle))
			testContent := fmt.Sprintf("Cycle %d test", cycle)
			err = os.WriteFile(testFile, []byte(testContent), 0644)
			require.NoError(t, err, "Failed to write test file in cycle %d", cycle)

			// Stop cluster
			stopCmd := exec.Command("./scripts/cluster_manager.sh", "stop", "-n", fmt.Sprintf("%d", nodeCount))
			stopCmd.Dir = ".."

			stopOutput, err := stopCmd.CombinedOutput()
			if err != nil {
				t.Logf("Cycle %d stop output: %s", cycle, string(stopOutput))
				// Don't fail on stop errors, just log them
				t.Logf("Warning: Failed to stop cluster cleanly in cycle %d: %v", cycle, err)
			}

			// Wait between cycles
			time.Sleep(3 * time.Second)
		})
	}
}

// Benchmark cluster startup time for different sizes
func BenchmarkClusterStartup(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	clusterSizes := []int{3, 5, 7}

	for _, size := range clusterSizes {
		b.Run(fmt.Sprintf("nodes_%d", size), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				b.StopTimer()

				// Ensure clean state
				exec.Command("./scripts/cluster_manager.sh", "stop", "-n", fmt.Sprintf("%d", size)).Run()
				time.Sleep(2 * time.Second)

				b.StartTimer()

				// Start cluster and measure time
				cmd := exec.Command("./scripts/cluster_manager.sh", "start", "-n", fmt.Sprintf("%d", size))
				cmd.Dir = ".."

				start := time.Now()
				err := cmd.Run()
				if err != nil {
					b.Fatalf("Failed to start %d-node cluster: %v", size, err)
				}

				// Wait for cluster to be ready (simplified readiness check)
				time.Sleep(10 * time.Second)
				elapsed := time.Since(start)

				b.StopTimer()

				b.Logf("Cluster startup time for %d nodes: %v", size, elapsed)

				// Cleanup
				exec.Command("./scripts/cluster_manager.sh", "stop", "-n", fmt.Sprintf("%d", size)).Run()
				time.Sleep(2 * time.Second)
			}
		})
	}
}
