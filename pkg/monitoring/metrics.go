// Package monitoring provides metrics collection and health monitoring for the distributed storage system.
package monitoring

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/hashicorp/raft"
	"github.com/sirupsen/logrus"
)

// RaftInterface defines the interface for Raft operations needed by the monitor
type RaftInterface interface {
	State() raft.RaftState
	Leader() raft.ServerAddress
	LastIndex() uint64
	CommitIndex() uint64
	AppliedIndex() uint64
	GetConfiguration() raft.ConfigurationFuture
}

// Metrics tracks various operational metrics for the storage system.
type Metrics struct {
	// File operation counters
	filesReplicated   int64
	filesDeleted      int64
	bytesReplicated   int64
	replicationErrors int64

	// Performance metrics
	avgReplicationTime    int64 // in milliseconds
	lastReplicationTimeNs int64 // Unix nanoseconds, accessed atomically

	// System metrics
	startTime time.Time
	nodeID    string

	logger *logrus.Logger
}

// ClusterHealth represents the health status of the entire cluster.
type ClusterHealth struct {
	NodeID       string    `json:"node_id"`
	State        string    `json:"state"`
	Leader       string    `json:"leader"`
	Peers        []string  `json:"peers"`
	LastLogIndex uint64    `json:"last_log_index"`
	CommitIndex  uint64    `json:"commit_index"`
	AppliedIndex uint64    `json:"applied_index"`
	Uptime       string    `json:"uptime"`
	Timestamp    time.Time `json:"timestamp"`
}

// NodeMetrics represents metrics for a single node.
type NodeMetrics struct {
	NodeID              string    `json:"node_id"`
	FilesReplicated     int64     `json:"files_replicated"`
	FilesDeleted        int64     `json:"files_deleted"`
	BytesReplicated     int64     `json:"bytes_replicated"`
	ReplicationErrors   int64     `json:"replication_errors"`
	AvgReplicationTime  int64     `json:"avg_replication_time_ms"`
	LastReplicationTime string    `json:"last_replication_time"`
	MemoryUsage         int64     `json:"memory_usage_bytes"`
	Goroutines          int       `json:"goroutines"`
	Uptime              string    `json:"uptime"`
	Timestamp           time.Time `json:"timestamp"`
}

// Monitor provides monitoring capabilities for the distributed storage system.
type Monitor struct {
	metrics *Metrics
	raft    RaftInterface
	logger  *logrus.Logger
}

// NewMetrics creates a new metrics instance.
func NewMetrics(nodeID string, logger *logrus.Logger) *Metrics {
	if logger == nil {
		logger = logrus.New()
	}

	return &Metrics{
		nodeID:    nodeID,
		startTime: time.Now(),
		logger:    logger,
	}
}

// NewMonitor creates a new monitoring instance.
func NewMonitor(nodeID string, raftNode RaftInterface, logger *logrus.Logger) *Monitor {
	return &Monitor{
		metrics: NewMetrics(nodeID, logger),
		raft:    raftNode,
		logger:  logger,
	}
}

// IncrementFilesReplicated increments the files replicated counter.
func (m *Metrics) IncrementFilesReplicated() {
	atomic.AddInt64(&m.filesReplicated, 1)
	atomic.StoreInt64(&m.lastReplicationTimeNs, time.Now().UnixNano())
}

// IncrementFilesDeleted increments the files deleted counter.
func (m *Metrics) IncrementFilesDeleted() {
	atomic.AddInt64(&m.filesDeleted, 1)
}

// AddBytesReplicated adds to the bytes replicated counter.
func (m *Metrics) AddBytesReplicated(bytes int64) {
	atomic.AddInt64(&m.bytesReplicated, bytes)
}

// IncrementReplicationErrors increments the replication errors counter.
func (m *Metrics) IncrementReplicationErrors() {
	atomic.AddInt64(&m.replicationErrors, 1)
}

// RecordReplicationTime records the time taken for a replication operation.
func (m *Metrics) RecordReplicationTime(duration time.Duration) {
	ms := duration.Milliseconds()
	// Simple moving average (could be improved with a proper sliding window)
	current := atomic.LoadInt64(&m.avgReplicationTime)
	if current == 0 {
		atomic.StoreInt64(&m.avgReplicationTime, ms)
	} else {
		// Weighted average: 90% old value, 10% new value
		newAvg := (current*9 + ms) / 10
		atomic.StoreInt64(&m.avgReplicationTime, newAvg)
	}
}

// GetNodeMetrics returns the current metrics for this node.
func (m *Metrics) GetNodeMetrics() NodeMetrics {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	lastRepl := "never"
	lastReplNs := atomic.LoadInt64(&m.lastReplicationTimeNs)
	if lastReplNs != 0 {
		lastRepl = time.Unix(0, lastReplNs).Format(time.RFC3339)
	}

	return NodeMetrics{
		NodeID:              m.nodeID,
		FilesReplicated:     atomic.LoadInt64(&m.filesReplicated),
		FilesDeleted:        atomic.LoadInt64(&m.filesDeleted),
		BytesReplicated:     atomic.LoadInt64(&m.bytesReplicated),
		ReplicationErrors:   atomic.LoadInt64(&m.replicationErrors),
		AvgReplicationTime:  atomic.LoadInt64(&m.avgReplicationTime),
		LastReplicationTime: lastRepl,
		MemoryUsage:         int64(memStats.Alloc),
		Goroutines:          runtime.NumGoroutine(),
		Uptime:              time.Since(m.startTime).String(),
		Timestamp:           time.Now(),
	}
}

// GetClusterHealth returns the health status of the cluster.
func (mon *Monitor) GetClusterHealth() ClusterHealth {
	state := mon.raft.State().String()
	leader := string(mon.raft.Leader())

	// Get cluster configuration to find peers
	future := mon.raft.GetConfiguration()
	var peers []string
	if err := future.Error(); err == nil {
		config := future.Configuration()
		for _, server := range config.Servers {
			if string(server.ID) != mon.metrics.nodeID {
				peers = append(peers, string(server.Address))
			}
		}
	}

	return ClusterHealth{
		NodeID:       mon.metrics.nodeID,
		State:        state,
		Leader:       leader,
		Peers:        peers,
		LastLogIndex: mon.raft.LastIndex(),
		CommitIndex:  mon.raft.CommitIndex(),
		AppliedIndex: mon.raft.AppliedIndex(),
		Uptime:       time.Since(mon.metrics.startTime).String(),
		Timestamp:    time.Now(),
	}
}

// StartHTTPServer starts an HTTP server for exposing metrics and health endpoints.
func (mon *Monitor) StartHTTPServer(port int) {
	mux := http.NewServeMux()

	// Metrics endpoint
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		metrics := mon.metrics.GetNodeMetrics()
		if err := json.NewEncoder(w).Encode(metrics); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	// Health endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		health := mon.GetClusterHealth()
		if err := json.NewEncoder(w).Encode(health); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	// Readiness endpoint
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		if mon.raft.State() == raft.Leader || mon.raft.State() == raft.Follower {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ready"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("not ready"))
		}
	})

	// Liveness endpoint
	mux.HandleFunc("/live", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("alive"))
	})

	// Root endpoint with basic info
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		info := map[string]string{
			"service": "pickbox-distributed-storage",
			"node_id": mon.metrics.nodeID,
			"state":   mon.raft.State().String(),
			"uptime":  time.Since(mon.metrics.startTime).String(),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(info)
	})

	addr := fmt.Sprintf(":%d", port)
	mon.logger.Infof("📊 Monitoring server starting on http://localhost%s", addr)
	mon.logger.Infof("   Metrics: http://localhost%s/metrics", addr)
	mon.logger.Infof("   Health:  http://localhost%s/health", addr)
	mon.logger.Infof("   Ready:   http://localhost%s/ready", addr)
	mon.logger.Infof("   Live:    http://localhost%s/live", addr)

	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil {
			mon.logger.WithError(err).Error("Monitoring server failed")
		}
	}()
}

// LogMetrics periodically logs key metrics.
func (mon *Monitor) LogMetrics(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			metrics := mon.metrics.GetNodeMetrics()
			health := mon.GetClusterHealth()

			mon.logger.WithFields(logrus.Fields{
				"files_replicated":   metrics.FilesReplicated,
				"bytes_replicated":   metrics.BytesReplicated,
				"replication_errors": metrics.ReplicationErrors,
				"state":              health.State,
				"leader":             health.Leader,
				"peers":              len(health.Peers),
				"memory_mb":          metrics.MemoryUsage / 1024 / 1024,
				"goroutines":         metrics.Goroutines,
			}).Info("Node metrics")
		}
	}()
}

// GetMetrics returns the underlying metrics instance.
func (mon *Monitor) GetMetrics() *Metrics {
	return mon.metrics
}
