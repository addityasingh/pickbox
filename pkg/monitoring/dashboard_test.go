package monitoring

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test Dashboard creation
func TestNewDashboard(t *testing.T) {
	logger := logrus.New()
	
	// Create a simple monitor (we'll skip the raft dependency for this test)
	monitor := &Monitor{
		metrics: NewMetrics("test-node", logger),
		logger:  logger,
	}
	
	dashboard := NewDashboard(monitor, logger)
	
	require.NotNil(t, dashboard)
	assert.NotNil(t, dashboard.monitor)
	assert.Equal(t, logger, dashboard.logger)
}

// Test Dashboard with nil logger
func TestNewDashboard_NilLogger(t *testing.T) {
	logger := logrus.New()
	
	// Create a simple monitor
	monitor := &Monitor{
		metrics: NewMetrics("test-node", logger),
		logger:  logger,
	}
	
	dashboard := NewDashboard(monitor, nil)
	
	require.NotNil(t, dashboard)
	assert.Nil(t, dashboard.logger) // NewDashboard doesn't create default logger
}

// Test Dashboard metrics access
func TestDashboard_MetricsAccess(t *testing.T) {
	logger := logrus.New()
	
	// Create monitor with metrics
	metrics := NewMetrics("test-node", logger)
	metrics.IncrementFilesReplicated()
	metrics.AddBytesReplicated(1024)
	
	monitor := &Monitor{
		metrics: metrics,
		logger:  logger,
	}
	
	dashboard := NewDashboard(monitor, logger)
	
	// Verify dashboard can access metrics through monitor
	assert.NotNil(t, dashboard.monitor)
	assert.NotNil(t, dashboard.monitor.metrics)
	
	// Get metrics and verify they contain expected data
	nodeMetrics := dashboard.monitor.metrics.GetNodeMetrics()
	assert.Equal(t, "test-node", nodeMetrics.NodeID)
	assert.Equal(t, int64(1), nodeMetrics.FilesReplicated)
	assert.Equal(t, int64(1024), nodeMetrics.BytesReplicated)
}

// Test Dashboard logging with valid logger
func TestDashboard_Logging(t *testing.T) {
	logger := logrus.New()
	
	monitor := &Monitor{
		metrics: NewMetrics("test-node", logger),
		logger:  logger,
	}
	
	dashboard := NewDashboard(monitor, logger)
	
	// Verify logger is set
	assert.Equal(t, logger, dashboard.logger)
	
	// Test that dashboard can log (no errors should occur)
	assert.NotPanics(t, func() {
		dashboard.logger.Info("Test log message")
	})
}

// Test Dashboard structure validation
func TestDashboard_Structure(t *testing.T) {
	logger := logrus.New()
	
	monitor := &Monitor{
		metrics: NewMetrics("test-node", logger),
		logger:  logger,
	}
	
	dashboard := NewDashboard(monitor, logger)
	
	// Verify dashboard structure
	assert.NotNil(t, dashboard)
	assert.IsType(t, &Dashboard{}, dashboard)
	assert.IsType(t, &Monitor{}, dashboard.monitor)
	assert.IsType(t, &logrus.Logger{}, dashboard.logger)
}

// Test Monitor GetMetrics method
func TestDashboard_MonitorGetMetrics(t *testing.T) {
	logger := logrus.New()
	
	metrics := NewMetrics("test-node", logger)
	metrics.IncrementFilesReplicated()
	
	monitor := &Monitor{
		metrics: metrics,
		logger:  logger,
	}
	
	dashboard := NewDashboard(monitor, logger)
	
	// Test that we can get metrics through the monitor
	retrievedMetrics := dashboard.monitor.GetMetrics()
	assert.NotNil(t, retrievedMetrics)
	assert.Equal(t, metrics, retrievedMetrics)
	
	// Verify metrics data
	nodeMetrics := retrievedMetrics.GetNodeMetrics()
	assert.Equal(t, int64(1), nodeMetrics.FilesReplicated)
}

// Benchmark dashboard creation
func BenchmarkNewDashboard(b *testing.B) {
	logger := logrus.New()
	
	monitor := &Monitor{
		metrics: NewMetrics("bench-node", logger),
		logger:  logger,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewDashboard(monitor, logger)
	}
}

// Benchmark metrics access through dashboard
func BenchmarkDashboard_MetricsAccess(b *testing.B) {
	logger := logrus.New()
	
	metrics := NewMetrics("bench-node", logger)
	monitor := &Monitor{
		metrics: metrics,
		logger:  logger,
	}
	
	dashboard := NewDashboard(monitor, logger)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dashboard.monitor.GetMetrics().GetNodeMetrics()
	}
}
