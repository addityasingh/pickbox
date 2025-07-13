package monitoring

import (
	"sync"
	"testing"
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

// Test file deletion metrics
func TestMetrics_IncrementFilesDeleted(t *testing.T) {
	logger := logrus.New()
	metrics := NewMetrics("test-node", logger)

	// Test initial state
	initialMetrics := metrics.GetNodeMetrics()
	assert.Equal(t, int64(0), initialMetrics.FilesDeleted)

	// Test increment
	metrics.IncrementFilesDeleted()
	updatedMetrics := metrics.GetNodeMetrics()
	assert.Equal(t, int64(1), updatedMetrics.FilesDeleted)

	// Test multiple increments
	for i := 0; i < 5; i++ {
		metrics.IncrementFilesDeleted()
	}
	finalMetrics := metrics.GetNodeMetrics()
	assert.Equal(t, int64(6), finalMetrics.FilesDeleted)
}

// Test metrics with zero values
func TestMetrics_ZeroValues(t *testing.T) {
	logger := logrus.New()
	metrics := NewMetrics("test-node", logger)

	// Test adding zero bytes
	metrics.AddBytesReplicated(0)
	nodeMetrics := metrics.GetNodeMetrics()
	assert.Equal(t, int64(0), nodeMetrics.BytesReplicated)

	// Test recording zero replication time
	metrics.RecordReplicationTime(0)
	updatedMetrics := metrics.GetNodeMetrics()
	assert.Equal(t, int64(0), updatedMetrics.AvgReplicationTime)
}

// Test metrics with negative values
func TestMetrics_NegativeValues(t *testing.T) {
	logger := logrus.New()
	metrics := NewMetrics("test-node", logger)

	// Test adding negative bytes (should still work)
	metrics.AddBytesReplicated(-100)
	nodeMetrics := metrics.GetNodeMetrics()
	assert.Equal(t, int64(-100), nodeMetrics.BytesReplicated)

	// Add positive bytes to verify it works correctly
	metrics.AddBytesReplicated(200)
	updatedMetrics := metrics.GetNodeMetrics()
	assert.Equal(t, int64(100), updatedMetrics.BytesReplicated)
}

// Test metrics with very large values
func TestMetrics_LargeValues(t *testing.T) {
	logger := logrus.New()
	metrics := NewMetrics("test-node", logger)

	// Test with large byte values
	largeValue := int64(1024 * 1024 * 1024 * 1024) // 1TB
	metrics.AddBytesReplicated(largeValue)
	nodeMetrics := metrics.GetNodeMetrics()
	assert.Equal(t, largeValue, nodeMetrics.BytesReplicated)

	// Test with many file replications
	for i := 0; i < 10000; i++ {
		metrics.IncrementFilesReplicated()
	}
	updatedMetrics := metrics.GetNodeMetrics()
	assert.Equal(t, int64(10000), updatedMetrics.FilesReplicated)
}

// Test LastReplicationTime tracking
func TestMetrics_LastReplicationTime(t *testing.T) {
	logger := logrus.New()
	metrics := NewMetrics("test-node", logger)

	// Initially should be "never"
	initialMetrics := metrics.GetNodeMetrics()
	assert.Equal(t, "never", initialMetrics.LastReplicationTime)

	// After incrementing files replicated, should have a timestamp
	metrics.IncrementFilesReplicated()
	updatedMetrics := metrics.GetNodeMetrics()
	assert.NotEqual(t, "never", updatedMetrics.LastReplicationTime)

	// Should be a valid RFC3339 timestamp
	_, err := time.Parse(time.RFC3339, updatedMetrics.LastReplicationTime)
	assert.NoError(t, err)
}

// Test replication time averaging
func TestMetrics_ReplicationTimeAveraging(t *testing.T) {
	logger := logrus.New()
	metrics := NewMetrics("test-node", logger)

	// Record first replication time
	metrics.RecordReplicationTime(100 * time.Millisecond)
	firstMetrics := metrics.GetNodeMetrics()
	assert.Equal(t, int64(100), firstMetrics.AvgReplicationTime)

	// Record second replication time (should be averaged)
	metrics.RecordReplicationTime(200 * time.Millisecond)
	secondMetrics := metrics.GetNodeMetrics()
	// Should be weighted average: (100*9 + 200) / 10 = 110
	assert.Equal(t, int64(110), secondMetrics.AvgReplicationTime)

	// Record third replication time
	metrics.RecordReplicationTime(300 * time.Millisecond)
	thirdMetrics := metrics.GetNodeMetrics()
	// Should be weighted average: (110*9 + 300) / 10 = 129
	assert.Equal(t, int64(129), thirdMetrics.AvgReplicationTime)
}

// Test metrics fields validation
func TestMetrics_FieldValidation(t *testing.T) {
	logger := logrus.New()
	metrics := NewMetrics("test-node-validation", logger)

	// Add some test data
	metrics.IncrementFilesReplicated()
	metrics.IncrementFilesDeleted()
	metrics.AddBytesReplicated(2048)
	metrics.IncrementReplicationErrors()
	metrics.RecordReplicationTime(150 * time.Millisecond)

	nodeMetrics := metrics.GetNodeMetrics()

	// Validate all fields are present and have correct types
	assert.IsType(t, "", nodeMetrics.NodeID)
	assert.IsType(t, int64(0), nodeMetrics.FilesReplicated)
	assert.IsType(t, int64(0), nodeMetrics.FilesDeleted)
	assert.IsType(t, int64(0), nodeMetrics.BytesReplicated)
	assert.IsType(t, int64(0), nodeMetrics.ReplicationErrors)
	assert.IsType(t, int64(0), nodeMetrics.AvgReplicationTime)
	assert.IsType(t, "", nodeMetrics.LastReplicationTime)
	assert.IsType(t, int64(0), nodeMetrics.MemoryUsage)
	assert.IsType(t, int(0), nodeMetrics.Goroutines)
	assert.IsType(t, "", nodeMetrics.Uptime)
	assert.IsType(t, time.Time{}, nodeMetrics.Timestamp)

	// Validate specific values
	assert.Equal(t, "test-node-validation", nodeMetrics.NodeID)
	assert.Equal(t, int64(1), nodeMetrics.FilesReplicated)
	assert.Equal(t, int64(1), nodeMetrics.FilesDeleted)
	assert.Equal(t, int64(2048), nodeMetrics.BytesReplicated)
	assert.Equal(t, int64(1), nodeMetrics.ReplicationErrors)
	assert.Equal(t, int64(150), nodeMetrics.AvgReplicationTime)
	assert.Greater(t, nodeMetrics.MemoryUsage, int64(0))
	assert.Greater(t, nodeMetrics.Goroutines, 0)
	assert.NotEmpty(t, nodeMetrics.Uptime)
	assert.WithinDuration(t, time.Now(), nodeMetrics.Timestamp, time.Second)
}

// Test metrics concurrent access patterns
func TestMetrics_ConcurrentAccessPatterns(t *testing.T) {
	logger := logrus.New()
	metrics := NewMetrics("test-node", logger)

	numGoroutines := 5
	operationsPerGoroutine := 50

	var wg sync.WaitGroup

	// Start multiple goroutines doing different operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				switch goroutineID % 4 {
				case 0:
					metrics.IncrementFilesReplicated()
				case 1:
					metrics.IncrementFilesDeleted()
				case 2:
					metrics.AddBytesReplicated(int64(j * 10))
				case 3:
					metrics.IncrementReplicationErrors()
				}
			}
		}(i)
	}

	// Start a goroutine that continuously reads metrics (simplified)
	readStop := make(chan bool, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ { // Limited iterations to avoid hanging
			select {
			case <-readStop:
				return
			default:
				metrics.GetNodeMetrics()
				time.Sleep(1 * time.Millisecond)
			}
		}
	}()

	wg.Wait()
	close(readStop)

	// Verify final state
	finalMetrics := metrics.GetNodeMetrics()

	// We should have some increments from goroutines 0 and 1
	expectedFilesReplicated := int64((numGoroutines / 4) * operationsPerGoroutine)
	if numGoroutines%4 > 0 {
		expectedFilesReplicated += int64(operationsPerGoroutine)
	}

	expectedFilesDeleted := int64((numGoroutines / 4) * operationsPerGoroutine)
	if numGoroutines%4 > 1 {
		expectedFilesDeleted += int64(operationsPerGoroutine)
	}

	assert.Equal(t, expectedFilesReplicated, finalMetrics.FilesReplicated)
	assert.Equal(t, expectedFilesDeleted, finalMetrics.FilesDeleted)
	assert.Greater(t, finalMetrics.BytesReplicated, int64(0))
}

// Test metrics with different logger configurations
func TestMetrics_DifferentLoggerConfigurations(t *testing.T) {
	t.Run("with_debug_logger", func(t *testing.T) {
		logger := logrus.New()
		logger.SetLevel(logrus.DebugLevel)

		metrics := NewMetrics("debug-node", logger)
		assert.NotNil(t, metrics)
		assert.Equal(t, "debug-node", metrics.nodeID)
	})

	t.Run("with_error_logger", func(t *testing.T) {
		logger := logrus.New()
		logger.SetLevel(logrus.ErrorLevel)

		metrics := NewMetrics("error-node", logger)
		assert.NotNil(t, metrics)
		assert.Equal(t, "error-node", metrics.nodeID)
	})

	t.Run("with_custom_formatter", func(t *testing.T) {
		logger := logrus.New()
		logger.SetFormatter(&logrus.JSONFormatter{})

		metrics := NewMetrics("json-node", logger)
		assert.NotNil(t, metrics)
		assert.Equal(t, "json-node", metrics.nodeID)
	})
}
