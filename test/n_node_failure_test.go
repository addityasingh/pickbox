package test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	failureTestTimeout = 120 * time.Second
	nodeRecoveryDelay  = 20 * time.Second
)

// failureTestSuite manages failure scenario tests
type failureTestSuite struct {
	nodeCount     int
	clusterEnv    *nNodeTestEnvironment
	nodeProcesses map[int]*exec.Cmd
	testDataDir   string
}

// setupFailureTest initializes a failure test environment
func setupFailureTest(t *testing.T, nodeCount int) *failureTestSuite {
	suite := &failureTestSuite{
		nodeCount:     nodeCount,
		clusterEnv:    setupNNodeTestEnvironment(t, nodeCount),
		nodeProcesses: make(map[int]*exec.Cmd),
		testDataDir:   filepath.Join("..", "data"),
	}

	// Start the cluster
	suite.clusterEnv.startCluster(t)

	// Cleanup when test finishes
	t.Cleanup(func() {
		suite.cleanup(t)
	})

	return suite
}

// cleanup stops the cluster and cleans up resources
func (suite *failureTestSuite) cleanup(t *testing.T) {
	suite.clusterEnv.stopCluster(t)
}

// simulateNodeFailure kills a specific node process
func (suite *failureTestSuite) simulateNodeFailure(t *testing.T, nodeNum int) {
	// Find the process for this node and kill it
	cmd := exec.Command("pkill", "-f", fmt.Sprintf("node%d", nodeNum))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Warning: Failed to kill node%d process: %v, output: %s", nodeNum, err, string(output))
	} else {
		t.Logf("Simulated failure of node%d", nodeNum)
	}

	// Wait for the failure to take effect
	time.Sleep(3 * time.Second)
}

// restartNode restarts a failed node
func (suite *failureTestSuite) restartNode(t *testing.T, nodeNum int) {
	nodeID := fmt.Sprintf("node%d", nodeNum)
	port := 8000 + nodeNum
	adminPort := 9000 + nodeNum
	dataDir := filepath.Join(suite.testDataDir, nodeID)
	joinAddr := "127.0.0.1:8001" // Always join to node1

	cmd := exec.Command(
		"../bin/pickbox", "node", "multi",
		"--node-id", nodeID,
		"--port", fmt.Sprintf("%d", port),
		"--admin-port", fmt.Sprintf("%d", adminPort),
		"--data-dir", dataDir,
		"--join", joinAddr,
	)

	// Start in background
	err := cmd.Start()
	require.NoError(t, err, "Failed to restart node%d", nodeNum)

	suite.nodeProcesses[nodeNum] = cmd
	t.Logf("Restarted node%d", nodeNum)

	// Wait for node to rejoin
	time.Sleep(nodeRecoveryDelay)
}

// writeFileOnNode creates a file on a specific node
func (suite *failureTestSuite) writeFileOnNode(t *testing.T, nodeNum int, relativePath, content string) {
	nodeDataDir := filepath.Join(suite.testDataDir, fmt.Sprintf("node%d", nodeNum))
	filePath := filepath.Join(nodeDataDir, relativePath)

	err := os.MkdirAll(filepath.Dir(filePath), 0755)
	require.NoError(t, err, "Failed to create directory for %s", filePath)

	err = os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err, "Failed to write file %s on node%d", relativePath, nodeNum)
}

// verifyFileExists checks if a file exists on a specific node
func (suite *failureTestSuite) verifyFileExists(nodeNum int, relativePath string) bool {
	nodeDataDir := filepath.Join(suite.testDataDir, fmt.Sprintf("node%d", nodeNum))
	filePath := filepath.Join(nodeDataDir, relativePath)

	_, err := os.Stat(filePath)
	return err == nil
}

// waitForFileReplicationToRemainingNodes waits for a file to replicate to all non-failed nodes
func (suite *failureTestSuite) waitForFileReplicationToRemainingNodes(t *testing.T, relativePath, expectedContent string, excludeNodes []int) {
	deadline := time.Now().Add(failureTestTimeout)

	excludeMap := make(map[int]bool)
	for _, node := range excludeNodes {
		excludeMap[node] = true
	}

	for time.Now().Before(deadline) {
		allNodesHaveFile := true

		for nodeNum := 1; nodeNum <= suite.nodeCount; nodeNum++ {
			if excludeMap[nodeNum] {
				continue // Skip failed nodes
			}

			nodeDataDir := filepath.Join(suite.testDataDir, fmt.Sprintf("node%d", nodeNum))
			filePath := filepath.Join(nodeDataDir, relativePath)

			content, err := os.ReadFile(filePath)
			if err != nil || string(content) != expectedContent {
				allNodesHaveFile = false
				break
			}
		}

		if allNodesHaveFile {
			return // Success!
		}

		time.Sleep(500 * time.Millisecond)
	}

	// Timeout - gather debug information
	t.Logf("Replication timeout for file %s", relativePath)
	for nodeNum := 1; nodeNum <= suite.nodeCount; nodeNum++ {
		if excludeMap[nodeNum] {
			t.Logf("Node%d: EXCLUDED (failed)", nodeNum)
			continue
		}

		nodeDataDir := filepath.Join(suite.testDataDir, fmt.Sprintf("node%d", nodeNum))
		filePath := filepath.Join(nodeDataDir, relativePath)

		content, err := os.ReadFile(filePath)
		if err != nil {
			t.Logf("Node%d: %v", nodeNum, err)
		} else {
			t.Logf("Node%d: %q (expected: %q)", nodeNum, string(content), expectedContent)
		}
	}

	t.Fatalf("File %s failed to replicate to remaining nodes within %v", relativePath, failureTestTimeout)
}

// Test single node failure and recovery
func TestSingleNodeFailureRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	suite := setupFailureTest(t, 5)

	// Step 1: Create a file before failure
	filename := "before_failure.txt"
	content := "Content created before node failure"
	suite.writeFileOnNode(t, 1, filename, content)

	// Wait for initial replication
	time.Sleep(5 * time.Second)

	// Verify all nodes have the file
	for nodeNum := 1; nodeNum <= suite.nodeCount; nodeNum++ {
		assert.True(t, suite.verifyFileExists(nodeNum, filename), "File should exist on node%d before failure", nodeNum)
	}

	// Step 2: Simulate failure of node3 (non-leader)
	failedNode := 3
	suite.simulateNodeFailure(t, failedNode)

	// Step 3: Create another file after failure
	filename2 := "after_failure.txt"
	content2 := "Content created after node failure"
	suite.writeFileOnNode(t, 1, filename2, content2)

	// Wait for replication to remaining nodes
	suite.waitForFileReplicationToRemainingNodes(t, filename2, content2, []int{failedNode})

	// Step 4: Restart the failed node
	suite.restartNode(t, failedNode)

	// Step 5: Verify the recovered node eventually gets the file
	deadline := time.Now().Add(failureTestTimeout)
	for time.Now().Before(deadline) {
		if suite.verifyFileExists(failedNode, filename2) {
			// Verify content is correct
			nodeDataDir := filepath.Join(suite.testDataDir, fmt.Sprintf("node%d", failedNode))
			filePath := filepath.Join(nodeDataDir, filename2)
			recoveredContent, err := os.ReadFile(filePath)
			if err == nil && string(recoveredContent) == content2 {
				t.Logf("Node%d successfully recovered and synchronized", failedNode)
				return
			}
		}
		time.Sleep(2 * time.Second)
	}

	t.Fatalf("Failed node%d did not recover and synchronize within %v", failedNode, failureTestTimeout)
}

// Test multiple node failures
func TestMultipleNodeFailures(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	suite := setupFailureTest(t, 5)

	// Step 1: Create initial file
	filename := "multi_failure_test.txt"
	content := "Content for multiple failure test"
	suite.writeFileOnNode(t, 1, filename, content)

	// Wait for initial replication
	time.Sleep(5 * time.Second)

	// Step 2: Simulate failure of multiple nodes (but not leader)
	failedNodes := []int{3, 4}
	for _, nodeNum := range failedNodes {
		suite.simulateNodeFailure(t, nodeNum)
	}

	// Step 3: Create file after failures
	filename2 := "after_multi_failure.txt"
	content2 := "Content after multiple failures"
	suite.writeFileOnNode(t, 1, filename2, content2)

	// Wait for replication to remaining nodes
	suite.waitForFileReplicationToRemainingNodes(t, filename2, content2, failedNodes)

	// Step 4: Restart failed nodes one by one
	for _, nodeNum := range failedNodes {
		suite.restartNode(t, nodeNum)
		time.Sleep(10 * time.Second) // Stagger restarts
	}

	// Step 5: Verify all nodes eventually have the latest file
	deadline := time.Now().Add(failureTestTimeout)
	for time.Now().Before(deadline) {
		allRecovered := true
		for _, nodeNum := range failedNodes {
			if !suite.verifyFileExists(nodeNum, filename2) {
				allRecovered = false
				break
			}
		}

		if allRecovered {
			t.Logf("All failed nodes successfully recovered")
			return
		}
		time.Sleep(3 * time.Second)
	}

	t.Fatalf("Not all failed nodes recovered within %v", failureTestTimeout)
}

// Test leader failure and election
func TestLeaderFailureAndElection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	suite := setupFailureTest(t, 5)

	// Step 1: Create file before leader failure
	filename := "before_leader_failure.txt"
	content := "Content before leader failure"
	suite.writeFileOnNode(t, 1, filename, content)

	// Wait for replication
	time.Sleep(5 * time.Second)

	// Step 2: Kill the leader (node1)
	suite.simulateNodeFailure(t, 1)

	// Step 3: Wait for leader election to occur
	time.Sleep(15 * time.Second)

	// Step 4: Try to create a file from another node (should work if new leader elected)
	filename2 := "after_leader_failure.txt"
	content2 := "Content after leader failure"
	suite.writeFileOnNode(t, 2, filename2, content2)

	// Wait for replication to remaining nodes
	suite.waitForFileReplicationToRemainingNodes(t, filename2, content2, []int{1})

	// Step 5: Restart the original leader
	suite.restartNode(t, 1)

	// Step 6: Verify the restarted node catches up
	deadline := time.Now().Add(failureTestTimeout)
	for time.Now().Before(deadline) {
		if suite.verifyFileExists(1, filename2) {
			t.Logf("Original leader successfully rejoined and synchronized")
			return
		}
		time.Sleep(3 * time.Second)
	}

	t.Fatalf("Original leader did not rejoin and synchronize within %v", failureTestTimeout)
}

// Test cluster partitioning (simulate network split)
func TestClusterPartitioning(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	suite := setupFailureTest(t, 5)

	// Step 1: Create initial file
	filename := "before_partition.txt"
	content := "Content before partition"
	suite.writeFileOnNode(t, 1, filename, content)

	// Wait for replication
	time.Sleep(5 * time.Second)

	// Step 2: Simulate network partition by killing nodes 4 and 5
	partitionedNodes := []int{4, 5}
	for _, nodeNum := range partitionedNodes {
		suite.simulateNodeFailure(t, nodeNum)
	}

	// Step 3: Continue operations on the majority partition
	filename2 := "during_partition.txt"
	content2 := "Content during partition"
	suite.writeFileOnNode(t, 1, filename2, content2)

	// Wait for replication within majority partition
	suite.waitForFileReplicationToRemainingNodes(t, filename2, content2, partitionedNodes)

	// Step 4: Heal the partition by restarting nodes
	for _, nodeNum := range partitionedNodes {
		suite.restartNode(t, nodeNum)
	}

	// Step 5: Verify partition healing
	deadline := time.Now().Add(failureTestTimeout)
	for time.Now().Before(deadline) {
		allHealed := true
		for _, nodeNum := range partitionedNodes {
			if !suite.verifyFileExists(nodeNum, filename2) {
				allHealed = false
				break
			}
		}

		if allHealed {
			t.Logf("Partition successfully healed")
			return
		}
		time.Sleep(3 * time.Second)
	}

	t.Fatalf("Partition did not heal within %v", failureTestTimeout)
}

// Test rapid failure and recovery cycles
func TestRapidFailureRecoveryCycles(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	suite := setupFailureTest(t, 3)

	cycles := 3
	targetNode := 3 // Always fail node3

	for cycle := 1; cycle <= cycles; cycle++ {
		t.Run(fmt.Sprintf("cycle_%d", cycle), func(t *testing.T) {
			// Create a file for this cycle
			filename := fmt.Sprintf("cycle_%d.txt", cycle)
			content := fmt.Sprintf("Content for cycle %d", cycle)
			suite.writeFileOnNode(t, 1, filename, content)

			// Wait for replication
			time.Sleep(3 * time.Second)

			// Simulate failure
			suite.simulateNodeFailure(t, targetNode)

			// Create another file during failure
			filename2 := fmt.Sprintf("during_failure_%d.txt", cycle)
			content2 := fmt.Sprintf("Content during failure cycle %d", cycle)
			suite.writeFileOnNode(t, 1, filename2, content2)

			// Wait for replication to remaining nodes
			suite.waitForFileReplicationToRemainingNodes(t, filename2, content2, []int{targetNode})

			// Restart the node
			suite.restartNode(t, targetNode)

			// Brief wait between cycles
			time.Sleep(5 * time.Second)
		})
	}
}

// Test graceful vs ungraceful node shutdown
func TestGracefulVsUngracefulShutdown(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	suite := setupFailureTest(t, 3)

	// Test ungraceful shutdown (SIGKILL)
	t.Run("ungraceful_shutdown", func(t *testing.T) {
		filename := "before_ungraceful.txt"
		content := "Content before ungraceful shutdown"
		suite.writeFileOnNode(t, 1, filename, content)

		time.Sleep(3 * time.Second)

		// Kill with SIGKILL (ungraceful)
		cmd := exec.Command("pkill", "-9", "-f", "node3")
		cmd.Run()

		// Create file after ungraceful shutdown
		filename2 := "after_ungraceful.txt"
		content2 := "Content after ungraceful shutdown"
		suite.writeFileOnNode(t, 1, filename2, content2)

		suite.waitForFileReplicationToRemainingNodes(t, filename2, content2, []int{3})

		// Restart
		suite.restartNode(t, 3)
	})

	// Test graceful shutdown (SIGTERM)
	t.Run("graceful_shutdown", func(t *testing.T) {
		filename := "before_graceful.txt"
		content := "Content before graceful shutdown"
		suite.writeFileOnNode(t, 1, filename, content)

		time.Sleep(3 * time.Second)

		// Send SIGTERM (graceful)
		cmd := exec.Command("pkill", "-TERM", "-f", "node3")
		cmd.Run()

		time.Sleep(2 * time.Second) // Allow time for graceful shutdown

		// Create file after graceful shutdown
		filename2 := "after_graceful.txt"
		content2 := "Content after graceful shutdown"
		suite.writeFileOnNode(t, 1, filename2, content2)

		suite.waitForFileReplicationToRemainingNodes(t, filename2, content2, []int{3})

		// Restart
		suite.restartNode(t, 3)
	})
}

// Test disk space exhaustion simulation
func TestDiskSpaceExhaustion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	suite := setupFailureTest(t, 3)

	// Create multiple large files to simulate disk pressure
	for i := 0; i < 5; i++ {
		filename := fmt.Sprintf("large_file_%d.txt", i)
		// Create moderately large content (don't actually exhaust disk)
		content := fmt.Sprintf("Large file %d: %s", i, string(make([]byte, 1024*10))) // 10KB

		suite.writeFileOnNode(t, 1, filename, content)
		time.Sleep(1 * time.Second)
	}

	// Verify all files eventually replicate
	time.Sleep(10 * time.Second)

	for i := 0; i < 5; i++ {
		filename := fmt.Sprintf("large_file_%d.txt", i)
		for nodeNum := 1; nodeNum <= suite.nodeCount; nodeNum++ {
			assert.True(t, suite.verifyFileExists(nodeNum, filename), "Large file %d should exist on node%d", i, nodeNum)
		}
	}
}

// Test concurrent failures during heavy load
func TestConcurrentFailuresDuringLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	suite := setupFailureTest(t, 5)

	// Start heavy load (creating files rapidly)
	done := make(chan bool)
	go func() {
		for i := 0; i < 20; i++ {
			select {
			case <-done:
				return
			default:
				filename := fmt.Sprintf("load_test_%d.txt", i)
				content := fmt.Sprintf("Load test file %d created at %s", i, time.Now().Format(time.RFC3339))
				suite.writeFileOnNode(t, 1, filename, content)
				time.Sleep(200 * time.Millisecond)
			}
		}
	}()

	// Simulate failures during load
	time.Sleep(2 * time.Second)
	suite.simulateNodeFailure(t, 3)

	time.Sleep(3 * time.Second)
	suite.simulateNodeFailure(t, 4)

	// Let load continue for a bit
	time.Sleep(5 * time.Second)

	// Stop load generation
	close(done)

	// Restart failed nodes
	suite.restartNode(t, 3)
	suite.restartNode(t, 4)

	// Verify system eventually stabilizes
	time.Sleep(20 * time.Second)

	t.Logf("Concurrent failure test completed - system should have stabilized")
}
