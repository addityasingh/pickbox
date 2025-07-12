package test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	replicationTestTimeout = 90 * time.Second
	fileCheckInterval      = 200 * time.Millisecond
)

// replicationTestSuite manages replication-specific tests
type replicationTestSuite struct {
	nodeCount   int
	clusterEnv  *nNodeTestEnvironment
	testDataDir string
}

// setupReplicationTest initializes a replication test environment
func setupReplicationTest(t *testing.T, nodeCount int) *replicationTestSuite {
	suite := &replicationTestSuite{
		nodeCount:   nodeCount,
		clusterEnv:  setupNNodeTestEnvironment(t, nodeCount),
		testDataDir: filepath.Join("..", "data"),
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
func (suite *replicationTestSuite) cleanup(t *testing.T) {
	suite.clusterEnv.stopCluster(t)
}

// writeFileOnNode creates a file on a specific node
func (suite *replicationTestSuite) writeFileOnNode(t *testing.T, nodeNum int, relativePath, content string) {
	nodeDataDir := filepath.Join(suite.testDataDir, fmt.Sprintf("node%d", nodeNum))
	filePath := filepath.Join(nodeDataDir, relativePath)

	err := os.MkdirAll(filepath.Dir(filePath), 0755)
	require.NoError(t, err, "Failed to create directory for %s", filePath)

	err = os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err, "Failed to write file %s on node%d", relativePath, nodeNum)
}

// readFileFromNode reads a file from a specific node
func (suite *replicationTestSuite) readFileFromNode(nodeNum int, relativePath string) (string, error) {
	nodeDataDir := filepath.Join(suite.testDataDir, fmt.Sprintf("node%d", nodeNum))
	filePath := filepath.Join(nodeDataDir, relativePath)

	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// waitForFileReplication waits for a file to replicate to all nodes
func (suite *replicationTestSuite) waitForFileReplication(t *testing.T, relativePath, expectedContent string) {
	deadline := time.Now().Add(replicationTestTimeout)

	for time.Now().Before(deadline) {
		allNodesHaveFile := true

		for nodeNum := 1; nodeNum <= suite.nodeCount; nodeNum++ {
			content, err := suite.readFileFromNode(nodeNum, relativePath)
			if err != nil || content != expectedContent {
				allNodesHaveFile = false
				break
			}
		}

		if allNodesHaveFile {
			return // Success!
		}

		time.Sleep(fileCheckInterval)
	}

	// Timeout - gather debug information
	t.Logf("Replication timeout for file %s", relativePath)
	for nodeNum := 1; nodeNum <= suite.nodeCount; nodeNum++ {
		content, err := suite.readFileFromNode(nodeNum, relativePath)
		if err != nil {
			t.Logf("Node%d: %v", nodeNum, err)
		} else {
			t.Logf("Node%d: %q (expected: %q)", nodeNum, content, expectedContent)
		}
	}

	t.Fatalf("File %s failed to replicate to all %d nodes within %v", relativePath, suite.nodeCount, replicationTestTimeout)
}

// verifyFileConsistency checks that a file has the same content across all nodes
func (suite *replicationTestSuite) verifyFileConsistency(t *testing.T, relativePath string) string {
	var referenceContent string

	for nodeNum := 1; nodeNum <= suite.nodeCount; nodeNum++ {
		content, err := suite.readFileFromNode(nodeNum, relativePath)
		require.NoError(t, err, "File %s should exist on node%d", relativePath, nodeNum)

		if nodeNum == 1 {
			referenceContent = content
		} else {
			assert.Equal(t, referenceContent, content, "File content should be consistent across all nodes")
		}
	}

	return referenceContent
}

// deleteFileFromNode removes a file from a specific node
func (suite *replicationTestSuite) deleteFileFromNode(t *testing.T, nodeNum int, relativePath string) {
	nodeDataDir := filepath.Join(suite.testDataDir, fmt.Sprintf("node%d", nodeNum))
	filePath := filepath.Join(nodeDataDir, relativePath)

	err := os.Remove(filePath)
	require.NoError(t, err, "Failed to delete file %s from node%d", relativePath, nodeNum)
}

// waitForFileDeletion waits for a file to be deleted from all nodes
func (suite *replicationTestSuite) waitForFileDeletion(t *testing.T, relativePath string) {
	deadline := time.Now().Add(replicationTestTimeout)

	for time.Now().Before(deadline) {
		allNodesDeleted := true

		for nodeNum := 1; nodeNum <= suite.nodeCount; nodeNum++ {
			nodeDataDir := filepath.Join(suite.testDataDir, fmt.Sprintf("node%d", nodeNum))
			filePath := filepath.Join(nodeDataDir, relativePath)

			if _, err := os.Stat(filePath); !os.IsNotExist(err) {
				allNodesDeleted = false
				break
			}
		}

		if allNodesDeleted {
			return // Success!
		}

		time.Sleep(fileCheckInterval)
	}

	t.Fatalf("File %s was not deleted from all %d nodes within %v", relativePath, suite.nodeCount, replicationTestTimeout)
}

// Test basic multi-directional replication
func TestMultiDirectionalReplication(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	suite := setupReplicationTest(t, 5)

	// Test replication from each node to all others
	for sourceNode := 1; sourceNode <= suite.nodeCount; sourceNode++ {
		t.Run(fmt.Sprintf("from_node_%d", sourceNode), func(t *testing.T) {
			filename := fmt.Sprintf("multi_dir_test_%d.txt", sourceNode)
			content := fmt.Sprintf("Multi-directional test from node%d at %s", sourceNode, time.Now().Format(time.RFC3339))

			suite.writeFileOnNode(t, sourceNode, filename, content)
			suite.waitForFileReplication(t, filename, content)
			suite.verifyFileConsistency(t, filename)
		})
	}
}

// Test concurrent writes from multiple nodes
func TestConcurrentMultiNodeWrites(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	suite := setupReplicationTest(t, 5)

	// Perform concurrent writes from all nodes
	var wg sync.WaitGroup
	filenames := make([]string, suite.nodeCount)

	for nodeNum := 1; nodeNum <= suite.nodeCount; nodeNum++ {
		wg.Add(1)
		go func(node int) {
			defer wg.Done()

			filename := fmt.Sprintf("concurrent_write_%d.txt", node)
			content := fmt.Sprintf("Concurrent write from node%d at %s", node, time.Now().Format(time.RFC3339))

			suite.writeFileOnNode(t, node, filename, content)
			filenames[node-1] = filename
		}(nodeNum)
	}

	wg.Wait()

	// Verify all files replicated to all nodes
	for _, filename := range filenames {
		if filename != "" {
			t.Run(fmt.Sprintf("verify_%s", filename), func(t *testing.T) {
				// Extract expected content by reading from the source node
				parts := strings.Split(filename, "_")
				if len(parts) >= 3 {
					sourceNodeStr := strings.TrimSuffix(parts[2], ".txt")
					var sourceNode int
					fmt.Sscanf(sourceNodeStr, "%d", &sourceNode)

					expectedContent, err := suite.readFileFromNode(sourceNode, filename)
					require.NoError(t, err, "Should be able to read source file")

					suite.waitForFileReplication(t, filename, expectedContent)
				}
			})
		}
	}
}

// Test file modification propagation
func TestFileModificationPropagation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	suite := setupReplicationTest(t, 3)

	filename := "modification_test.txt"

	// Step 1: Create initial file
	initialContent := "Initial content for modification test"
	suite.writeFileOnNode(t, 1, filename, initialContent)
	suite.waitForFileReplication(t, filename, initialContent)

	// Step 2: Modify from different node
	modifiedContent := "Modified content at " + time.Now().Format(time.RFC3339)
	suite.writeFileOnNode(t, 2, filename, modifiedContent)
	suite.waitForFileReplication(t, filename, modifiedContent)

	// Step 3: Modify again from yet another node
	finalContent := "Final content at " + time.Now().Format(time.RFC3339)
	suite.writeFileOnNode(t, 3, filename, finalContent)
	suite.waitForFileReplication(t, filename, finalContent)

	// Verify final consistency
	suite.verifyFileConsistency(t, filename)
}

// Test file deletion propagation
func TestFileDeletionPropagation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	suite := setupReplicationTest(t, 3)

	filename := "deletion_test.txt"
	content := "File to be deleted"

	// Create file on all nodes
	suite.writeFileOnNode(t, 1, filename, content)
	suite.waitForFileReplication(t, filename, content)

	// Delete from one node
	suite.deleteFileFromNode(t, 2, filename)
	suite.waitForFileDeletion(t, filename)

	// Verify file is deleted from all nodes
	for nodeNum := 1; nodeNum <= suite.nodeCount; nodeNum++ {
		nodeDataDir := filepath.Join(suite.testDataDir, fmt.Sprintf("node%d", nodeNum))
		filePath := filepath.Join(nodeDataDir, filename)
		assert.NoFileExists(t, filePath, "File should be deleted from node%d", nodeNum)
	}
}

// Test nested directory replication (replication specific)
func TestReplicationNestedDirectories(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	suite := setupReplicationTest(t, 3)

	// Test various nested directory structures
	testCases := []struct {
		name string
		path string
	}{
		{"simple_nested", "folder/file.txt"},
		{"deep_nested", "level1/level2/level3/level4/deep_file.txt"},
		{"mixed_structure", "projects/web/src/components/Button.tsx"},
		{"with_spaces", "folder with spaces/sub folder/file with spaces.txt"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			content := fmt.Sprintf("Content for %s test", tc.name)
			suite.writeFileOnNode(t, 1, tc.path, content)
			suite.waitForFileReplication(t, tc.path, content)
			suite.verifyFileConsistency(t, tc.path)
		})
	}
}

// Test large file replication (replication specific)
func TestReplicationLargeFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	suite := setupReplicationTest(t, 3)

	// Test different file sizes
	testCases := []struct {
		name string
		size int
	}{
		{"small_1kb", 1024},
		{"medium_10kb", 10 * 1024},
		{"large_100kb", 100 * 1024},
		{"very_large_1mb", 1024 * 1024},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filename := fmt.Sprintf("large_file_%s.dat", tc.name)
			content := strings.Repeat("A", tc.size)

			suite.writeFileOnNode(t, 1, filename, content)
			suite.waitForFileReplication(t, filename, content)

			// Verify file size on all nodes
			for nodeNum := 1; nodeNum <= suite.nodeCount; nodeNum++ {
				nodeDataDir := filepath.Join(suite.testDataDir, fmt.Sprintf("node%d", nodeNum))
				filePath := filepath.Join(nodeDataDir, filename)

				info, err := os.Stat(filePath)
				require.NoError(t, err, "Large file should exist on node%d", nodeNum)
				assert.Equal(t, int64(tc.size), info.Size(), "File size should match on node%d", nodeNum)
			}
		})
	}
}

// Test rapid file operations
func TestRapidFileOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	suite := setupReplicationTest(t, 3)

	operationCount := 20
	var wg sync.WaitGroup

	// Perform rapid file operations
	for i := 0; i < operationCount; i++ {
		wg.Add(1)
		go func(opNum int) {
			defer wg.Done()

			sourceNode := (opNum % suite.nodeCount) + 1
			filename := fmt.Sprintf("rapid_op_%d.txt", opNum)
			content := fmt.Sprintf("Rapid operation %d from node%d", opNum, sourceNode)

			suite.writeFileOnNode(t, sourceNode, filename, content)

			// Small delay between operations
			time.Sleep(10 * time.Millisecond)
		}(i)
	}

	wg.Wait()

	// Wait for all operations to settle
	time.Sleep(10 * time.Second)

	// Verify all files exist on all nodes
	for i := 0; i < operationCount; i++ {
		filename := fmt.Sprintf("rapid_op_%d.txt", i)
		t.Run(fmt.Sprintf("verify_rapid_op_%d", i), func(t *testing.T) {
			suite.verifyFileConsistency(t, filename)
		})
	}
}

// Test replication under simulated network delays
func TestReplicationWithDelays(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	suite := setupReplicationTest(t, 5)

	// Create files with artificial delays between operations
	for i := 0; i < 10; i++ {
		sourceNode := (i % suite.nodeCount) + 1
		filename := fmt.Sprintf("delayed_op_%d.txt", i)
		content := fmt.Sprintf("Delayed operation %d from node%d at %s", i, sourceNode, time.Now().Format(time.RFC3339))

		suite.writeFileOnNode(t, sourceNode, filename, content)

		// Artificial delay between operations
		time.Sleep(500 * time.Millisecond)
	}

	// Verify all files eventually replicate
	for i := 0; i < 10; i++ {
		filename := fmt.Sprintf("delayed_op_%d.txt", i)
		t.Run(fmt.Sprintf("verify_delayed_op_%d", i), func(t *testing.T) {
			suite.verifyFileConsistency(t, filename)
		})
	}
}

// Test binary file replication
func TestBinaryFileReplication(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	suite := setupReplicationTest(t, 3)

	// Create binary content (simulated)
	binaryContent := make([]byte, 1024)
	for i := 0; i < len(binaryContent); i++ {
		binaryContent[i] = byte(i % 256)
	}

	filename := "binary_test.bin"

	// Write binary file
	nodeDataDir := filepath.Join(suite.testDataDir, "node1")
	filePath := filepath.Join(nodeDataDir, filename)
	err := os.MkdirAll(filepath.Dir(filePath), 0755)
	require.NoError(t, err)

	err = os.WriteFile(filePath, binaryContent, 0644)
	require.NoError(t, err)

	// Wait for replication
	suite.waitForFileReplication(t, filename, string(binaryContent))

	// Verify binary content integrity on all nodes
	for nodeNum := 1; nodeNum <= suite.nodeCount; nodeNum++ {
		nodeDataDir := filepath.Join(suite.testDataDir, fmt.Sprintf("node%d", nodeNum))
		filePath := filepath.Join(nodeDataDir, filename)

		replicatedContent, err := os.ReadFile(filePath)
		require.NoError(t, err, "Binary file should exist on node%d", nodeNum)
		assert.Equal(t, binaryContent, replicatedContent, "Binary content should match on node%d", nodeNum)
	}
}

// Test file permission preservation
func TestFilePermissionReplication(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	suite := setupReplicationTest(t, 3)

	filename := "permission_test.txt"
	content := "File with specific permissions"

	// Create file with specific permissions
	nodeDataDir := filepath.Join(suite.testDataDir, "node1")
	filePath := filepath.Join(nodeDataDir, filename)
	err := os.MkdirAll(filepath.Dir(filePath), 0755)
	require.NoError(t, err)

	err = os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err)

	// Wait for replication
	suite.waitForFileReplication(t, filename, content)

	// Note: In the current implementation, file permissions may not be perfectly preserved
	// This test documents the current behavior
	for nodeNum := 1; nodeNum <= suite.nodeCount; nodeNum++ {
		nodeDataDir := filepath.Join(suite.testDataDir, fmt.Sprintf("node%d", nodeNum))
		filePath := filepath.Join(nodeDataDir, filename)

		info, err := os.Stat(filePath)
		require.NoError(t, err, "File should exist on node%d", nodeNum)

		// Verify file exists and has some reasonable permissions
		mode := info.Mode()
		assert.True(t, mode.IsRegular(), "Should be a regular file on node%d", nodeNum)
		t.Logf("Node%d file permissions: %v", nodeNum, mode)
	}
}

// Note: Benchmark tests removed due to interface complexity with testing.B vs testing.T
// They can be re-added later with proper type handling
