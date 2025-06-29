package monitoring

import (
	"testing"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

// Test Metrics creation
func TestNewMetrics(t *testing.T) {
	nodeID := "test-node"
	logger := logrus.New()
	metrics := NewMetrics(nodeID, logger)

	assert.NotNil(t, metrics)
	assert.Equal(t, nodeID, metrics.nodeID)
	assert.WithinDuration(t, time.Now(), metrics.startTime, time.Second)
	assert.Equal(t, int64(0), metrics.filesReplicated)
	assert.Equal(t, int64(0), metrics.filesDeleted)
	assert.Equal(t, int64(0), metrics.bytesReplicated)
	assert.Equal(t, int64(0), metrics.replicationErrors)
}

// Test file replication metrics
func TestMetrics_IncrementFilesReplicated(t *testing.T) {
	logger := logrus.New()
	metrics := NewMetrics("test-node", logger)

	// Test initial state
	initialMetrics := metrics.GetNodeMetrics()
	assert.Equal(t, int64(0), initialMetrics.FilesReplicated)

	// Test increment
	metrics.IncrementFilesReplicated()
	updatedMetrics := metrics.GetNodeMetrics()
	assert.Equal(t, int64(1), updatedMetrics.FilesReplicated)

	// Test multiple increments
	for i := 0; i < 10; i++ {
		metrics.IncrementFilesReplicated()
	}
	finalMetrics := metrics.GetNodeMetrics()
	assert.Equal(t, int64(11), finalMetrics.FilesReplicated)
}

// Test bytes replication metrics
func TestMetrics_AddBytesReplicated(t *testing.T) {
	logger := logrus.New()
	metrics := NewMetrics("test-node", logger)

	// Test initial state
	initialMetrics := metrics.GetNodeMetrics()
	assert.Equal(t, int64(0), initialMetrics.BytesReplicated)

	// Test adding bytes
	metrics.AddBytesReplicated(100)
	updatedMetrics := metrics.GetNodeMetrics()
	assert.Equal(t, int64(100), updatedMetrics.BytesReplicated)

	// Test adding more bytes
	metrics.AddBytesReplicated(250)
	finalMetrics := metrics.GetNodeMetrics()
	assert.Equal(t, int64(350), finalMetrics.BytesReplicated)
}

// Test error tracking
func TestMetrics_IncrementReplicationErrors(t *testing.T) {
	logger := logrus.New()
	metrics := NewMetrics("test-node", logger)

	// Test initial state
	initialMetrics := metrics.GetNodeMetrics()
	assert.Equal(t, int64(0), initialMetrics.ReplicationErrors)

	// Test increment
	metrics.IncrementReplicationErrors()
	updatedMetrics := metrics.GetNodeMetrics()
	assert.Equal(t, int64(1), updatedMetrics.ReplicationErrors)
}

// Test concurrent metrics updates
func TestMetrics_ConcurrentUpdates(t *testing.T) {
	logger := logrus.New()
	metrics := NewMetrics("test-node", logger)

	numGoroutines := 10
	incrementsPerGoroutine := 100

	var wg sync.WaitGroup

	// Concurrent file replication increments
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < incrementsPerGoroutine; j++ {
				metrics.IncrementFilesReplicated()
				metrics.AddBytesReplicated(10)
				metrics.IncrementReplicationErrors()
			}
		}()
	}

	wg.Wait()

	expectedFiles := int64(numGoroutines * incrementsPerGoroutine)
	expectedBytes := int64(numGoroutines * incrementsPerGoroutine * 10)
	expectedErrors := int64(numGoroutines * incrementsPerGoroutine)

	finalMetrics := metrics.GetNodeMetrics()
	assert.Equal(t, expectedFiles, finalMetrics.FilesReplicated)
	assert.Equal(t, expectedBytes, finalMetrics.BytesReplicated)
	assert.Equal(t, expectedErrors, finalMetrics.ReplicationErrors)
}

// Test replication time tracking
func TestMetrics_RecordReplicationTime(t *testing.T) {
	logger := logrus.New()
	metrics := NewMetrics("test-node", logger)

	// Test recording replication time
	metrics.RecordReplicationTime(100 * time.Millisecond)
	nodeMetrics := metrics.GetNodeMetrics()
	assert.Greater(t, nodeMetrics.AvgReplicationTime, int64(0))

	// Test multiple recordings
	metrics.RecordReplicationTime(200 * time.Millisecond)
	updatedMetrics := metrics.GetNodeMetrics()
	assert.Greater(t, updatedMetrics.AvgReplicationTime, int64(0))
}

// Test uptime calculation
func TestMetrics_Uptime(t *testing.T) {
	logger := logrus.New()
	metrics := NewMetrics("test-node", logger)

	// Wait a bit to ensure uptime > 0
	time.Sleep(10 * time.Millisecond)

	nodeMetrics := metrics.GetNodeMetrics()
	assert.NotEmpty(t, nodeMetrics.Uptime)
}

// Test NodeMetrics structure
func TestMetrics_GetNodeMetrics(t *testing.T) {
	logger := logrus.New()
	metrics := NewMetrics("test-node", logger)

	// Add some data
	metrics.IncrementFilesReplicated()
	metrics.IncrementFilesDeleted()
	metrics.AddBytesReplicated(1024)
	metrics.IncrementReplicationErrors()
	metrics.RecordReplicationTime(50 * time.Millisecond)

	nodeMetrics := metrics.GetNodeMetrics()

	// Verify all fields are populated
	assert.Equal(t, "test-node", nodeMetrics.NodeID)
	assert.Equal(t, int64(1), nodeMetrics.FilesReplicated)
	assert.Equal(t, int64(1), nodeMetrics.FilesDeleted)
	assert.Equal(t, int64(1024), nodeMetrics.BytesReplicated)
	assert.Equal(t, int64(1), nodeMetrics.ReplicationErrors)
	assert.NotEmpty(t, nodeMetrics.Uptime)
	assert.Greater(t, nodeMetrics.MemoryUsage, int64(0))
	assert.Greater(t, nodeMetrics.Goroutines, 0)
	assert.WithinDuration(t, time.Now(), nodeMetrics.Timestamp, time.Second)
}

// Test Metrics with nil logger
func TestNewMetrics_NilLogger(t *testing.T) {
	metrics := NewMetrics("test-node", nil)
	assert.NotNil(t, metrics)
	assert.NotNil(t, metrics.logger) // Should create default logger
}

// Benchmark metrics operations
func BenchmarkMetrics_IncrementFilesReplicated(b *testing.B) {
	logger := logrus.New()
	metrics := NewMetrics("test-node", logger)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics.IncrementFilesReplicated()
	}
}

func BenchmarkMetrics_AddBytesReplicated(b *testing.B) {
	logger := logrus.New()
	metrics := NewMetrics("test-node", logger)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics.AddBytesReplicated(1024)
	}
}

func BenchmarkMetrics_GetNodeMetrics(b *testing.B) {
	logger := logrus.New()
	metrics := NewMetrics("test-node", logger)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics.GetNodeMetrics()
	}
}
