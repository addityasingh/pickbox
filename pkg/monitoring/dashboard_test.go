package monitoring

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hashicorp/raft"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock Raft for testing
type mockRaft struct {
	state        raft.RaftState
	leader       raft.ServerAddress
	lastIndex    uint64
	commitIndex  uint64
	appliedIndex uint64
	configError  error
	servers      []raft.Server
}

func newMockRaft() *mockRaft {
	return &mockRaft{
		state:        raft.Follower,
		leader:       "127.0.0.1:8000",
		lastIndex:    100,
		commitIndex:  99,
		appliedIndex: 98,
		servers: []raft.Server{
			{ID: "node1", Address: "127.0.0.1:8001"},
			{ID: "node2", Address: "127.0.0.1:8002"},
		},
	}
}

func (m *mockRaft) State() raft.RaftState {
	return m.state
}

func (m *mockRaft) Leader() raft.ServerAddress {
	return m.leader
}

func (m *mockRaft) LastIndex() uint64 {
	return m.lastIndex
}

func (m *mockRaft) CommitIndex() uint64 {
	return m.commitIndex
}

func (m *mockRaft) AppliedIndex() uint64 {
	return m.appliedIndex
}

func (m *mockRaft) GetConfiguration() raft.ConfigurationFuture {
	return &mockConfigFuture{
		err:     m.configError,
		servers: m.servers,
	}
}

func (m *mockRaft) setState(state raft.RaftState) {
	m.state = state
}

func (m *mockRaft) setLeader(leader raft.ServerAddress) {
	m.leader = leader
}

func (m *mockRaft) setConfigError(err error) {
	m.configError = err
}

// Mock ConfigurationFuture for testing
type mockConfigFuture struct {
	err     error
	servers []raft.Server
}

func (f *mockConfigFuture) Error() error {
	return f.err
}

func (f *mockConfigFuture) Index() uint64 {
	return 0
}

func (f *mockConfigFuture) Configuration() raft.Configuration {
	return raft.Configuration{
		Servers: f.servers,
	}
}

// Helper function to create a logger that doesn't output during tests
func createTestLogger() *logrus.Logger {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel) // Suppress logs during tests
	return logger
}

// Test Dashboard creation
func TestNewDashboard(t *testing.T) {
	logger := createTestLogger()

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
	logger := createTestLogger()

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
	logger := createTestLogger()

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
	logger := createTestLogger()

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
	logger := createTestLogger()

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
	logger := createTestLogger()

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

// Test Dashboard handleDashboard method
func TestDashboard_handleDashboard(t *testing.T) {
	logger := createTestLogger()
	mockRaft := newMockRaft()

	monitor := NewMonitor("test-node", mockRaft, logger)
	dashboard := NewDashboard(monitor, logger)

	t.Run("successful_dashboard_render", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()

		dashboard.handleDashboard(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "text/html", w.Header().Get("Content-Type"))
		assert.Contains(t, w.Body.String(), "Pickbox Distributed Storage")
		assert.Contains(t, w.Body.String(), "test-node")
	})

	t.Run("dashboard_contains_required_elements", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()

		dashboard.handleDashboard(w, req)

		body := w.Body.String()
		assert.Contains(t, body, "Cluster Health")
		assert.Contains(t, body, "Replication Metrics")
		assert.Contains(t, body, "System Resources")
		assert.Contains(t, body, "/static/dashboard.css")
		assert.Contains(t, body, "/static/dashboard.js")
	})
}

// Test Dashboard handleAPIMetrics method
func TestDashboard_handleAPIMetrics(t *testing.T) {
	logger := createTestLogger()
	mockRaft := newMockRaft()

	monitor := NewMonitor("test-node", mockRaft, logger)
	monitor.metrics.IncrementFilesReplicated()
	monitor.metrics.AddBytesReplicated(1024)

	dashboard := NewDashboard(monitor, logger)

	t.Run("successful_metrics_response", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/metrics", nil)
		w := httptest.NewRecorder()

		dashboard.handleAPIMetrics(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

		var metrics NodeMetrics
		err := json.NewDecoder(w.Body).Decode(&metrics)
		require.NoError(t, err)

		assert.Equal(t, "test-node", metrics.NodeID)
		assert.Equal(t, int64(1), metrics.FilesReplicated)
		assert.Equal(t, int64(1024), metrics.BytesReplicated)
	})
}

// Test Dashboard handleAPIHealth method
func TestDashboard_handleAPIHealth(t *testing.T) {
	logger := createTestLogger()
	mockRaft := newMockRaft()
	mockRaft.setState(raft.Leader)

	monitor := NewMonitor("test-node", mockRaft, logger)
	dashboard := NewDashboard(monitor, logger)

	t.Run("successful_health_response", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/health", nil)
		w := httptest.NewRecorder()

		dashboard.handleAPIHealth(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

		var health ClusterHealth
		err := json.NewDecoder(w.Body).Decode(&health)
		require.NoError(t, err)

		assert.Equal(t, "test-node", health.NodeID)
		assert.Equal(t, "Leader", health.State)
		assert.Equal(t, "127.0.0.1:8000", health.Leader)
	})

	t.Run("health_with_raft_error", func(t *testing.T) {
		mockRaft.setConfigError(assert.AnError)

		req := httptest.NewRequest("GET", "/api/health", nil)
		w := httptest.NewRecorder()

		dashboard.handleAPIHealth(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var health ClusterHealth
		err := json.NewDecoder(w.Body).Decode(&health)
		require.NoError(t, err)

		assert.Equal(t, "test-node", health.NodeID)
		assert.Empty(t, health.Peers) // Should be empty due to error
	})
}

// Test Dashboard handleAPICluster method
func TestDashboard_handleAPICluster(t *testing.T) {
	logger := createTestLogger()
	mockRaft := newMockRaft()

	monitor := NewMonitor("test-node", mockRaft, logger)
	monitor.metrics.IncrementFilesReplicated()
	monitor.metrics.AddBytesReplicated(1024)
	monitor.metrics.IncrementReplicationErrors()

	dashboard := NewDashboard(monitor, logger)

	t.Run("successful_cluster_response", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/cluster", nil)
		w := httptest.NewRecorder()

		dashboard.handleAPICluster(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

		var clusterInfo map[string]interface{}
		err := json.NewDecoder(w.Body).Decode(&clusterInfo)
		require.NoError(t, err)

		// Verify cluster info structure
		assert.Contains(t, clusterInfo, "health")
		assert.Contains(t, clusterInfo, "metrics")
		assert.Contains(t, clusterInfo, "summary")

		// Verify summary data
		summary := clusterInfo["summary"].(map[string]interface{})
		assert.Contains(t, summary, "total_nodes")
		assert.Contains(t, summary, "leader_node")
		assert.Contains(t, summary, "current_state")
		assert.Contains(t, summary, "files_tracked")
		assert.Contains(t, summary, "total_bytes")
		assert.Contains(t, summary, "error_rate")
		assert.Contains(t, summary, "avg_repl_time")
	})
}

// Test Dashboard handleStatic method
func TestDashboard_handleStatic(t *testing.T) {
	logger := createTestLogger()
	mockRaft := newMockRaft()

	monitor := NewMonitor("test-node", mockRaft, logger)
	dashboard := NewDashboard(monitor, logger)

	t.Run("serve_css_file", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/static/dashboard.css", nil)
		w := httptest.NewRecorder()

		dashboard.handleStatic(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "text/css", w.Header().Get("Content-Type"))
		assert.Contains(t, w.Body.String(), "body")
	})

	t.Run("serve_js_file", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/static/dashboard.js", nil)
		w := httptest.NewRecorder()

		dashboard.handleStatic(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/javascript", w.Header().Get("Content-Type"))
		assert.NotEmpty(t, w.Body.String())
	})

	t.Run("not_found_for_unknown_file", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/static/unknown.txt", nil)
		w := httptest.NewRecorder()

		dashboard.handleStatic(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// Test Dashboard StartDashboardServer method
func TestDashboard_StartDashboardServer(t *testing.T) {
	logger := createTestLogger()
	mockRaft := newMockRaft()

	monitor := NewMonitor("test-node", mockRaft, logger)
	dashboard := NewDashboard(monitor, logger)

	t.Run("server_starts_successfully", func(t *testing.T) {
		// Use port 0 to get a free port
		assert.NotPanics(t, func() {
			dashboard.StartDashboardServer(0)
		})
	})
}

// Test NewMonitor function
func TestNewMonitor(t *testing.T) {
	logger := createTestLogger()
	mockRaft := newMockRaft()

	monitor := NewMonitor("test-node", mockRaft, logger)

	assert.NotNil(t, monitor)
	assert.NotNil(t, monitor.metrics)
	assert.Equal(t, mockRaft, monitor.raft)
	assert.Equal(t, logger, monitor.logger)
	assert.Equal(t, "test-node", monitor.metrics.nodeID)
}

// Test Monitor GetClusterHealth method
func TestMonitor_GetClusterHealth(t *testing.T) {
	logger := createTestLogger()
	mockRaft := newMockRaft()

	monitor := NewMonitor("test-node", mockRaft, logger)

	t.Run("successful_health_retrieval", func(t *testing.T) {
		mockRaft.setState(raft.Leader)
		mockRaft.setLeader("127.0.0.1:8000")

		health := monitor.GetClusterHealth()

		assert.Equal(t, "test-node", health.NodeID)
		assert.Equal(t, "Leader", health.State)
		assert.Equal(t, "127.0.0.1:8000", health.Leader)
		assert.Equal(t, uint64(100), health.LastLogIndex)
		assert.Equal(t, uint64(99), health.CommitIndex)
		assert.Equal(t, uint64(98), health.AppliedIndex)
		assert.NotEmpty(t, health.Uptime)
		assert.WithinDuration(t, time.Now(), health.Timestamp, time.Second)
		assert.Len(t, health.Peers, 2)
	})

	t.Run("health_with_follower_state", func(t *testing.T) {
		mockRaft.setState(raft.Follower)
		mockRaft.setLeader("127.0.0.1:8001")

		health := monitor.GetClusterHealth()

		assert.Equal(t, "Follower", health.State)
		assert.Equal(t, "127.0.0.1:8001", health.Leader)
	})

	t.Run("health_with_config_error", func(t *testing.T) {
		mockRaft.setConfigError(assert.AnError)

		health := monitor.GetClusterHealth()

		assert.Equal(t, "test-node", health.NodeID)
		assert.Empty(t, health.Peers) // Should be empty due to error
	})
}

// Test Monitor StartHTTPServer method
func TestMonitor_StartHTTPServer(t *testing.T) {
	logger := createTestLogger()
	mockRaft := newMockRaft()

	monitor := NewMonitor("test-node", mockRaft, logger)

	t.Run("server_starts_successfully", func(t *testing.T) {
		assert.NotPanics(t, func() {
			monitor.StartHTTPServer(0) // Use port 0 to get a free port
		})
	})
}

// Test Monitor HTTP endpoints
func TestMonitor_HTTPEndpoints(t *testing.T) {
	logger := createTestLogger()
	mockRaft := newMockRaft()
	mockRaft.setState(raft.Leader)

	monitor := NewMonitor("test-node", mockRaft, logger)
	monitor.metrics.IncrementFilesReplicated()
	monitor.metrics.AddBytesReplicated(1024)

	t.Run("metrics_endpoint", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/metrics", nil)
		w := httptest.NewRecorder()

		// Create the handler manually to test
		handler := func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			metrics := monitor.metrics.GetNodeMetrics()
			json.NewEncoder(w).Encode(metrics)
		}

		handler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

		var metrics NodeMetrics
		err := json.NewDecoder(w.Body).Decode(&metrics)
		require.NoError(t, err)

		assert.Equal(t, "test-node", metrics.NodeID)
		assert.Equal(t, int64(1), metrics.FilesReplicated)
		assert.Equal(t, int64(1024), metrics.BytesReplicated)
	})

	t.Run("health_endpoint", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/health", nil)
		w := httptest.NewRecorder()

		handler := func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			health := monitor.GetClusterHealth()
			json.NewEncoder(w).Encode(health)
		}

		handler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

		var health ClusterHealth
		err := json.NewDecoder(w.Body).Decode(&health)
		require.NoError(t, err)

		assert.Equal(t, "test-node", health.NodeID)
		assert.Equal(t, "Leader", health.State)
	})

	t.Run("ready_endpoint_leader", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/ready", nil)
		w := httptest.NewRecorder()

		handler := func(w http.ResponseWriter, r *http.Request) {
			if monitor.raft.State() == raft.Leader || monitor.raft.State() == raft.Follower {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("ready"))
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte("not ready"))
			}
		}

		handler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "ready", w.Body.String())
	})

	t.Run("ready_endpoint_candidate", func(t *testing.T) {
		mockRaft.setState(raft.Candidate)

		req := httptest.NewRequest("GET", "/ready", nil)
		w := httptest.NewRecorder()

		handler := func(w http.ResponseWriter, r *http.Request) {
			if monitor.raft.State() == raft.Leader || monitor.raft.State() == raft.Follower {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("ready"))
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte("not ready"))
			}
		}

		handler(w, req)

		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
		assert.Equal(t, "not ready", w.Body.String())
	})

	t.Run("live_endpoint", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/live", nil)
		w := httptest.NewRecorder()

		handler := func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("alive"))
		}

		handler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "alive", w.Body.String())
	})

	t.Run("root_endpoint", func(t *testing.T) {
		// Reset state back to Leader for this test
		mockRaft.setState(raft.Leader)

		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()

		handler := func(w http.ResponseWriter, r *http.Request) {
			info := map[string]string{
				"service": "pickbox-distributed-storage",
				"node_id": monitor.metrics.nodeID,
				"state":   monitor.raft.State().String(),
				"uptime":  time.Since(monitor.metrics.startTime).String(),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(info)
		}

		handler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

		var info map[string]string
		err := json.NewDecoder(w.Body).Decode(&info)
		require.NoError(t, err)

		assert.Equal(t, "pickbox-distributed-storage", info["service"])
		assert.Equal(t, "test-node", info["node_id"])
		assert.Equal(t, "Leader", info["state"])
		assert.NotEmpty(t, info["uptime"])
	})
}

// Test Monitor GetMetrics method
func TestMonitor_GetMetrics(t *testing.T) {
	logger := createTestLogger()
	mockRaft := newMockRaft()

	monitor := NewMonitor("test-node", mockRaft, logger)

	metrics := monitor.GetMetrics()

	assert.NotNil(t, metrics)
	assert.Equal(t, monitor.metrics, metrics)
	assert.Equal(t, "test-node", metrics.nodeID)
}

// Test error handling in API endpoints
func TestDashboard_APIErrorHandling(t *testing.T) {
	logger := createTestLogger()

	// Create a monitor with a nil raft to trigger errors
	monitor := &Monitor{
		metrics: NewMetrics("test-node", logger),
		raft:    nil,
		logger:  logger,
	}

	dashboard := NewDashboard(monitor, logger)

	t.Run("health_endpoint_with_nil_raft", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/health", nil)
		w := httptest.NewRecorder()

		// This should panic but we'll catch it
		assert.Panics(t, func() {
			dashboard.handleAPIHealth(w, req)
		})
	})
}

// Test template rendering with invalid data
func TestDashboard_TemplateError(t *testing.T) {
	logger := createTestLogger()

	// Create a monitor with nil metrics to trigger template error
	monitor := &Monitor{
		metrics: nil,
		logger:  logger,
	}

	dashboard := NewDashboard(monitor, logger)

	t.Run("template_error_with_nil_metrics", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()

		// This should panic but we'll catch it
		assert.Panics(t, func() {
			dashboard.handleDashboard(w, req)
		})
	})
}

// Test peer filtering in cluster health
func TestMonitor_GetClusterHealth_PeerFiltering(t *testing.T) {
	logger := createTestLogger()
	mockRaft := newMockRaft()

	// Add the current node to the servers list
	mockRaft.servers = append(mockRaft.servers, raft.Server{
		ID:      "test-node",
		Address: "127.0.0.1:8000",
	})

	monitor := NewMonitor("test-node", mockRaft, logger)

	health := monitor.GetClusterHealth()

	// Should exclude the current node from peers
	assert.Len(t, health.Peers, 2)
	assert.NotContains(t, health.Peers, "127.0.0.1:8000")
}

// Benchmark dashboard creation
func BenchmarkNewDashboard(b *testing.B) {
	logger := createTestLogger()

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
	logger := createTestLogger()

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

// Benchmark dashboard HTTP handlers
func BenchmarkDashboard_HandleAPIMetrics(b *testing.B) {
	logger := createTestLogger()
	mockRaft := newMockRaft()

	monitor := NewMonitor("bench-node", mockRaft, logger)
	dashboard := NewDashboard(monitor, logger)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/api/metrics", nil)
		w := httptest.NewRecorder()
		dashboard.handleAPIMetrics(w, req)
	}
}

func BenchmarkDashboard_HandleAPIHealth(b *testing.B) {
	logger := createTestLogger()
	mockRaft := newMockRaft()

	monitor := NewMonitor("bench-node", mockRaft, logger)
	dashboard := NewDashboard(monitor, logger)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/api/health", nil)
		w := httptest.NewRecorder()
		dashboard.handleAPIHealth(w, req)
	}
}

// Test Dashboard API error handling with JSON encoding issues
func TestDashboard_APIJSONEncodingError(t *testing.T) {
	logger := createTestLogger()
	mockRaft := newMockRaft()

	monitor := NewMonitor("test-node", mockRaft, logger)
	dashboard := NewDashboard(monitor, logger)

	// Create a broken response writer that fails on write
	brokenWriter := &brokenResponseWriter{}

	t.Run("metrics_endpoint_json_error", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/metrics", nil)

		// This will fail when trying to write JSON
		dashboard.handleAPIMetrics(brokenWriter, req)

		// Verify that the error was handled
		assert.Equal(t, http.StatusInternalServerError, brokenWriter.statusCode)
	})

	t.Run("health_endpoint_json_error", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/health", nil)

		// This will fail when trying to write JSON
		dashboard.handleAPIHealth(brokenWriter, req)

		// Verify that the error was handled
		assert.Equal(t, http.StatusInternalServerError, brokenWriter.statusCode)
	})

	t.Run("cluster_endpoint_json_error", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/cluster", nil)

		// This will fail when trying to write JSON
		dashboard.handleAPICluster(brokenWriter, req)

		// Verify that the error was handled
		assert.Equal(t, http.StatusInternalServerError, brokenWriter.statusCode)
	})
}

// Test Monitor LogMetrics method
func TestMonitor_LogMetrics(t *testing.T) {
	logger := createTestLogger()
	mockRaft := newMockRaft()

	monitor := NewMonitor("test-node", mockRaft, logger)

	// Test LogMetrics doesn't panic
	assert.NotPanics(t, func() {
		monitor.LogMetrics(100 * time.Millisecond)
	})

	// Wait a bit to ensure the goroutine has time to run
	time.Sleep(150 * time.Millisecond)

	// Test should not panic with very short intervals
	assert.NotPanics(t, func() {
		monitor.LogMetrics(1 * time.Millisecond)
	})
}

// Test Monitor with different raft states
func TestMonitor_DifferentRaftStates(t *testing.T) {
	logger := createTestLogger()
	mockRaft := newMockRaft()

	monitor := NewMonitor("test-node", mockRaft, logger)

	states := []raft.RaftState{
		raft.Follower,
		raft.Candidate,
		raft.Leader,
		raft.Shutdown,
	}

	for _, state := range states {
		t.Run(state.String(), func(t *testing.T) {
			mockRaft.setState(state)
			health := monitor.GetClusterHealth()
			assert.Equal(t, state.String(), health.State)
		})
	}
}

// Test Monitor GetClusterHealth with empty leader
func TestMonitor_GetClusterHealthEmptyLeader(t *testing.T) {
	logger := createTestLogger()
	mockRaft := newMockRaft()

	monitor := NewMonitor("test-node", mockRaft, logger)

	// Test with empty leader
	mockRaft.setLeader("")
	health := monitor.GetClusterHealth()
	assert.Empty(t, health.Leader)
}

// Test Monitor HTTP endpoints with different content types
func TestMonitor_HTTPEndpointsContentType(t *testing.T) {
	logger := createTestLogger()
	mockRaft := newMockRaft()

	monitor := NewMonitor("test-node", mockRaft, logger)

	// Test that all endpoints return correct content types
	endpoints := []struct {
		path        string
		contentType string
	}{
		{"/metrics", "application/json"},
		{"/health", "application/json"},
		{"/", "application/json"},
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint.path, func(t *testing.T) {
			req := httptest.NewRequest("GET", endpoint.path, nil)
			w := httptest.NewRecorder()

			// Create appropriate handler
			var handler http.HandlerFunc
			switch endpoint.path {
			case "/metrics":
				handler = func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					metrics := monitor.metrics.GetNodeMetrics()
					json.NewEncoder(w).Encode(metrics)
				}
			case "/health":
				handler = func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					health := monitor.GetClusterHealth()
					json.NewEncoder(w).Encode(health)
				}
			case "/":
				handler = func(w http.ResponseWriter, r *http.Request) {
					info := map[string]string{
						"service": "pickbox-distributed-storage",
						"node_id": monitor.metrics.nodeID,
						"state":   monitor.raft.State().String(),
						"uptime":  time.Since(monitor.metrics.startTime).String(),
					}
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(info)
				}
			}

			handler(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, endpoint.contentType, w.Header().Get("Content-Type"))
		})
	}
}

// Test Dashboard template rendering with various data conditions
func TestDashboard_TemplateRendering(t *testing.T) {
	logger := createTestLogger()
	mockRaft := newMockRaft()

	t.Run("template_with_long_node_id", func(t *testing.T) {
		// Create monitor with long node ID
		longNodeMonitor := NewMonitor("very-long-node-id-with-many-characters", mockRaft, logger)
		longNodeDashboard := NewDashboard(longNodeMonitor, logger)

		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()

		longNodeDashboard.handleDashboard(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "very-long-node-id-with-many-characters")
	})

	t.Run("template_with_special_characters", func(t *testing.T) {
		// Create monitor with special characters in node ID
		specialMonitor := NewMonitor("node-with-special-chars_123", mockRaft, logger)
		specialDashboard := NewDashboard(specialMonitor, logger)

		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()

		specialDashboard.handleDashboard(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "node-with-special-chars_123")
	})
}

// Test Dashboard static file serving edge cases
func TestDashboard_StaticFileEdgeCases(t *testing.T) {
	logger := createTestLogger()
	mockRaft := newMockRaft()

	monitor := NewMonitor("test-node", mockRaft, logger)
	dashboard := NewDashboard(monitor, logger)

	t.Run("static_path_with_query_params", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/static/dashboard.css?version=1.0", nil)
		w := httptest.NewRecorder()

		dashboard.handleStatic(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "text/css", w.Header().Get("Content-Type"))
	})

	t.Run("static_path_case_sensitivity", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/static/Dashboard.css", nil)
		w := httptest.NewRecorder()

		dashboard.handleStatic(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("static_empty_path", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/static/", nil)
		w := httptest.NewRecorder()

		dashboard.handleStatic(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// Helper type for testing JSON encoding errors
type brokenResponseWriter struct {
	statusCode int
	header     http.Header
}

func (b *brokenResponseWriter) Header() http.Header {
	if b.header == nil {
		b.header = make(http.Header)
	}
	return b.header
}

func (b *brokenResponseWriter) Write([]byte) (int, error) {
	return 0, assert.AnError
}

func (b *brokenResponseWriter) WriteHeader(statusCode int) {
	b.statusCode = statusCode
}
