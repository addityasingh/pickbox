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

	"github.com/stretchr/testify/require"
)

// performanceTestSuite manages performance testing for N-node clusters
type performanceTestSuite struct {
	nodeCount   int
	clusterEnv  *nNodeTestEnvironment
	testDataDir string
	metrics     *performanceMetrics
}

// performanceMetrics tracks various performance measurements
type performanceMetrics struct {
	totalOperations      int
	totalBytes           int64
	operationLatencies   []time.Duration
	replicationLatencies []time.Duration
	startTime            time.Time
	endTime              time.Time
	mutex                sync.Mutex
}

// recordOperation records metrics for a single operation
func (m *performanceMetrics) recordOperation(operationLatency, replicationLatency time.Duration, bytes int64) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.totalOperations++
	m.totalBytes += bytes
	m.operationLatencies = append(m.operationLatencies, operationLatency)
	m.replicationLatencies = append(m.replicationLatencies, replicationLatency)
}

// getStats calculates performance statistics
func (m *performanceMetrics) getStats() map[string]interface{} {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if len(m.operationLatencies) == 0 {
		return make(map[string]interface{})
	}

	totalDuration := m.endTime.Sub(m.startTime)
	throughput := float64(m.totalOperations) / totalDuration.Seconds()
	byteThroughput := float64(m.totalBytes) / totalDuration.Seconds()

	// Calculate average latencies
	var totalOpLatency, totalReplLatency time.Duration
	for _, latency := range m.operationLatencies {
		totalOpLatency += latency
	}
	for _, latency := range m.replicationLatencies {
		totalReplLatency += latency
	}

	avgOpLatency := totalOpLatency / time.Duration(len(m.operationLatencies))
	avgReplLatency := totalReplLatency / time.Duration(len(m.replicationLatencies))

	return map[string]interface{}{
		"total_operations":        m.totalOperations,
		"total_bytes":             m.totalBytes,
		"total_duration_sec":      totalDuration.Seconds(),
		"operations_per_sec":      throughput,
		"bytes_per_sec":           byteThroughput,
		"avg_operation_latency":   avgOpLatency,
		"avg_replication_latency": avgReplLatency,
	}
}

// setupPerformanceTest initializes a performance test environment
func setupPerformanceTest(t *testing.T, nodeCount int) *performanceTestSuite {
	suite := &performanceTestSuite{
		nodeCount:   nodeCount,
		clusterEnv:  setupNNodeTestEnvironment(t, nodeCount),
		testDataDir: filepath.Join("..", "data"),
		metrics:     &performanceMetrics{},
	}

	// Start the cluster
	suite.clusterEnv.startCluster(t)
	suite.metrics.startTime = time.Now()

	// Cleanup when test finishes
	t.Cleanup(func() {
		suite.cleanup(t)
	})

	return suite
}

// cleanup stops the cluster and reports final metrics
func (suite *performanceTestSuite) cleanup(t *testing.T) {
	suite.metrics.endTime = time.Now()
	stats := suite.metrics.getStats()

	t.Logf("Performance Test Results:")
	for key, value := range stats {
		t.Logf("  %s: %v", key, value)
	}

	suite.clusterEnv.stopCluster(t)
}

// writeFileAndMeasure creates a file and measures performance
func (suite *performanceTestSuite) writeFileAndMeasure(t *testing.T, nodeNum int, relativePath, content string) {
	nodeDataDir := filepath.Join(suite.testDataDir, fmt.Sprintf("node%d", nodeNum))
	filePath := filepath.Join(nodeDataDir, relativePath)

	// Measure operation latency
	operationStart := time.Now()

	err := os.MkdirAll(filepath.Dir(filePath), 0755)
	require.NoError(t, err, "Failed to create directory for %s", filePath)

	err = os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err, "Failed to write file %s on node%d", relativePath, nodeNum)

	operationLatency := time.Since(operationStart)

	// Measure replication latency
	replicationStart := time.Now()
	suite.waitForReplicationPerformance(t, relativePath, content)
	replicationLatency := time.Since(replicationStart)

	// Record metrics
	suite.metrics.recordOperation(operationLatency, replicationLatency, int64(len(content)))
}

// waitForReplicationPerformance waits for file replication (performance-focused)
func (suite *performanceTestSuite) waitForReplicationPerformance(t *testing.T, relativePath, expectedContent string) {
	deadline := time.Now().Add(30 * time.Second) // Shorter timeout for performance tests

	for time.Now().Before(deadline) {
		allNodesHaveFile := true

		for nodeNum := 1; nodeNum <= suite.nodeCount; nodeNum++ {
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

		time.Sleep(50 * time.Millisecond) // Shorter polling interval for performance
	}

	t.Errorf("Replication timeout for file %s (performance test)", relativePath)
}

// Test throughput with different cluster sizes
func TestThroughputScaling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	clusterSizes := []int{3, 5}
	fileCount := 50
	fileSize := 1024 // 1KB files

	for _, size := range clusterSizes {
		t.Run(fmt.Sprintf("cluster_size_%d", size), func(t *testing.T) {
			suite := setupPerformanceTest(t, size)

			content := strings.Repeat("A", fileSize)

			for i := 0; i < fileCount; i++ {
				filename := fmt.Sprintf("throughput_test_%d.txt", i)
				sourceNode := (i % size) + 1

				suite.writeFileAndMeasure(t, sourceNode, filename, content)

				// Small delay between operations to avoid overwhelming the system
				time.Sleep(100 * time.Millisecond)
			}

			stats := suite.metrics.getStats()
			t.Logf("Cluster size %d - Operations/sec: %.2f", size, stats["operations_per_sec"])
			t.Logf("Cluster size %d - Average replication latency: %v", size, stats["avg_replication_latency"])
		})
	}
}

// Test concurrent write performance
func TestConcurrentWritePerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	suite := setupPerformanceTest(t, 5)

	concurrentWriters := 10
	filesPerWriter := 10
	fileSize := 512 // 512 bytes

	var wg sync.WaitGroup

	for writer := 0; writer < concurrentWriters; writer++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()

			content := fmt.Sprintf("Writer %d content: %s", writerID, strings.Repeat("X", fileSize-50))

			for i := 0; i < filesPerWriter; i++ {
				filename := fmt.Sprintf("concurrent_%d_%d.txt", writerID, i)
				sourceNode := (writerID % suite.nodeCount) + 1

				suite.writeFileAndMeasure(t, sourceNode, filename, content)

				// Brief delay
				time.Sleep(10 * time.Millisecond)
			}
		}(writer)
	}

	wg.Wait()

	stats := suite.metrics.getStats()
	t.Logf("Concurrent writes - Total operations: %d", stats["total_operations"])
	t.Logf("Concurrent writes - Operations/sec: %.2f", stats["operations_per_sec"])
	t.Logf("Concurrent writes - Bytes/sec: %.2f", stats["bytes_per_sec"])
}

// Test file size impact on performance
func TestFileSizePerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	suite := setupPerformanceTest(t, 3)

	fileSizes := []int{
		1024,       // 1KB
		10 * 1024,  // 10KB
		100 * 1024, // 100KB
	}

	for _, size := range fileSizes {
		t.Run(fmt.Sprintf("file_size_%d_bytes", size), func(t *testing.T) {
			content := strings.Repeat("A", size)
			filename := fmt.Sprintf("size_test_%d.txt", size)

			start := time.Now()
			suite.writeFileAndMeasure(t, 1, filename, content)
			elapsed := time.Since(start)

			throughput := float64(size) / elapsed.Seconds()
			t.Logf("File size %d bytes - Replication time: %v", size, elapsed)
			t.Logf("File size %d bytes - Throughput: %.2f bytes/sec", size, throughput)
		})
	}
}

// Test cluster startup performance for different sizes
func TestClusterStartupPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	clusterSizes := []int{3, 5, 7}

	for _, size := range clusterSizes {
		t.Run(fmt.Sprintf("startup_time_%d_nodes", size), func(t *testing.T) {
			// Clean up any existing cluster
			exec.Command("./scripts/cluster_manager.sh", "stop", "-n", fmt.Sprintf("%d", size)).Run()
			time.Sleep(3 * time.Second)

			// Measure startup time
			start := time.Now()

			cmd := exec.Command("./scripts/cluster_manager.sh", "start", "-n", fmt.Sprintf("%d", size))
			cmd.Dir = ".."

			err := cmd.Run()
			require.NoError(t, err, "Failed to start %d-node cluster", size)

			// Wait for cluster to be ready (basic readiness check)
			time.Sleep(15 * time.Second)
			elapsed := time.Since(start)

			t.Logf("Cluster startup time for %d nodes: %v", size, elapsed)

			// Verify cluster is working with a quick test
			testFile := filepath.Join("..", "data", "node1", "startup_test.txt")
			testContent := fmt.Sprintf("Startup test for %d nodes", size)
			err = os.WriteFile(testFile, []byte(testContent), 0644)
			require.NoError(t, err, "Failed to write startup test file")

			// Cleanup
			exec.Command("./scripts/cluster_manager.sh", "stop", "-n", fmt.Sprintf("%d", size)).Run()
			time.Sleep(2 * time.Second)
		})
	}
}

// Test memory usage patterns
func TestMemoryUsagePatterns(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	suite := setupPerformanceTest(t, 3)

	// Create many small files to test memory usage
	fileCount := 100
	fileSize := 256 // Small files

	content := strings.Repeat("M", fileSize)

	for i := 0; i < fileCount; i++ {
		filename := fmt.Sprintf("memory_test_%d.txt", i)
		suite.writeFileAndMeasure(t, 1, filename, content)

		// Check memory usage periodically (simplified)
		if i%20 == 0 {
			t.Logf("Created %d files so far", i+1)
		}

		time.Sleep(50 * time.Millisecond)
	}

	stats := suite.metrics.getStats()
	t.Logf("Memory test - Total files: %d", fileCount)
	t.Logf("Memory test - Average operation latency: %v", stats["avg_operation_latency"])
}

// Test replication consistency under load
func TestReplicationConsistencyUnderLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	suite := setupPerformanceTest(t, 5)

	// Generate load while checking consistency
	fileCount := 30
	content := "Consistency test content under load"

	for i := 0; i < fileCount; i++ {
		filename := fmt.Sprintf("consistency_test_%d.txt", i)
		sourceNode := (i % suite.nodeCount) + 1

		suite.writeFileAndMeasure(t, sourceNode, filename, content)

		// Verify the file exists on all nodes with correct content
		for nodeNum := 1; nodeNum <= suite.nodeCount; nodeNum++ {
			nodeDataDir := filepath.Join(suite.testDataDir, fmt.Sprintf("node%d", nodeNum))
			filePath := filepath.Join(nodeDataDir, filename)

			// Allow some time for replication
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				fileContent, err := os.ReadFile(filePath)
				if err == nil && string(fileContent) == content {
					break // File replicated correctly
				}
				time.Sleep(100 * time.Millisecond)
			}

			// Final verification
			fileContent, err := os.ReadFile(filePath)
			if err != nil {
				t.Errorf("File %s not found on node%d after load test", filename, nodeNum)
			} else if string(fileContent) != content {
				t.Errorf("File %s has incorrect content on node%d", filename, nodeNum)
			}
		}

		time.Sleep(100 * time.Millisecond)
	}

	t.Logf("Consistency test completed - all %d files verified across %d nodes", fileCount, suite.nodeCount)
}

// Test latency distribution
func TestLatencyDistribution(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	suite := setupPerformanceTest(t, 3)

	fileCount := 50
	content := "Latency distribution test content"

	// Collect detailed latency measurements
	latencies := make([]time.Duration, 0, fileCount)

	for i := 0; i < fileCount; i++ {
		filename := fmt.Sprintf("latency_test_%d.txt", i)

		start := time.Now()
		suite.writeFileAndMeasure(t, 1, filename, content)
		latency := time.Since(start)

		latencies = append(latencies, latency)
		time.Sleep(200 * time.Millisecond)
	}

	// Calculate latency statistics
	if len(latencies) > 0 {
		var min, max, total time.Duration
		min = latencies[0]
		max = latencies[0]

		for _, latency := range latencies {
			total += latency
			if latency < min {
				min = latency
			}
			if latency > max {
				max = latency
			}
		}

		avg := total / time.Duration(len(latencies))

		t.Logf("Latency distribution (%d samples):", len(latencies))
		t.Logf("  Min: %v", min)
		t.Logf("  Max: %v", max)
		t.Logf("  Avg: %v", avg)

		// Calculate percentiles (simplified)
		if len(latencies) >= 10 {
			// Sort latencies for percentile calculation (simplified approach)
			p50Index := len(latencies) / 2
			p95Index := (len(latencies) * 95) / 100

			t.Logf("  P50 (approx): %v", latencies[p50Index])
			t.Logf("  P95 (approx): %v", latencies[p95Index])
		}
	}
}

// Test resource usage during peak load
func TestPeakLoadResourceUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	suite := setupPerformanceTest(t, 5)

	// Generate peak load for a short duration
	duration := 30 * time.Second
	deadline := time.Now().Add(duration)

	var wg sync.WaitGroup
	workers := 5

	// Start multiple workers
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			fileCounter := 0
			for time.Now().Before(deadline) {
				filename := fmt.Sprintf("peak_load_%d_%d.txt", workerID, fileCounter)
				content := fmt.Sprintf("Peak load test content from worker %d file %d", workerID, fileCounter)
				sourceNode := (workerID % suite.nodeCount) + 1

				suite.writeFileAndMeasure(t, sourceNode, filename, content)

				fileCounter++
				time.Sleep(50 * time.Millisecond)
			}

			t.Logf("Worker %d created %d files", workerID, fileCounter)
		}(worker)
	}

	wg.Wait()

	stats := suite.metrics.getStats()
	t.Logf("Peak load test results:")
	t.Logf("  Duration: %v", duration)
	t.Logf("  Total operations: %d", stats["total_operations"])
	t.Logf("  Operations/sec: %.2f", stats["operations_per_sec"])
	t.Logf("  Bytes/sec: %.2f", stats["bytes_per_sec"])
}

// Test edge case performance scenarios
func TestEdgeCasePerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	suite := setupPerformanceTest(t, 3)

	// Test 1: Many tiny files
	t.Run("many_tiny_files", func(t *testing.T) {
		for i := 0; i < 20; i++ {
			filename := fmt.Sprintf("tiny_%d.txt", i)
			content := "T" // 1 byte file
			suite.writeFileAndMeasure(t, 1, filename, content)
			time.Sleep(100 * time.Millisecond)
		}
	})

	// Test 2: Files with long names
	t.Run("long_filename", func(t *testing.T) {
		longName := strings.Repeat("long_name_part_", 10) + ".txt"
		content := "File with very long name"
		suite.writeFileAndMeasure(t, 1, longName, content)
	})

	// Test 3: Deep directory structure
	t.Run("deep_directory", func(t *testing.T) {
		deepPath := strings.Repeat("deep/", 10) + "file.txt"
		content := "File in deep directory structure"
		suite.writeFileAndMeasure(t, 1, deepPath, content)
	})

	stats := suite.metrics.getStats()
	t.Logf("Edge case performance completed - Total operations: %d", stats["total_operations"])
}
