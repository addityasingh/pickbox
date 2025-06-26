// Package storage implements a distributed storage system with Raft consensus.
package storage

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
	"github.com/sirupsen/logrus"
)

const (
	// OpWrite represents a file write operation.
	OpWrite = "write"
	// OpDelete represents a file delete operation.
	OpDelete = "delete"

	// Default timeouts and configurations
	defaultApplyTimeout     = 10 * time.Second
	defaultSnapshotRetain   = 3
	defaultTransportTimeout = 10 * time.Second
	defaultTransportMaxPool = 3
)

// RaftManager manages Raft consensus for distributed file system operations.
type RaftManager struct {
	raft      *raft.Raft
	fsm       *fileSystemFSM
	store     *raftboltdb.BoltStore
	snapshots *raft.FileSnapshotStore
	transport *raft.NetworkTransport
	logger    *logrus.Logger
	nodeID    string
	dataDir   string
}

// fileSystemFSM implements the Raft finite state machine for file operations.
type fileSystemFSM struct {
	store   map[string][]byte
	mu      sync.RWMutex
	dataDir string
}

// Command represents a distributed file system operation.
type Command struct {
	Op    string  `json:"op"`    // Operation type: "write" or "delete"
	Path  string  `json:"path"`  // Relative file path
	Data  []byte  `json:"data"`  // File content (for write operations)
	Chunk ChunkID `json:"chunk"` // Associated chunk identifier
}

// fileSystemSnapshot implements raft.FSMSnapshot for persistence.
type fileSystemSnapshot struct {
	store map[string][]byte
}

// NewRaftManager creates a new Raft manager for distributed consensus.
func NewRaftManager(nodeID, dataDir, bindAddr string) (*RaftManager, error) {
	if nodeID == "" {
		return nil, fmt.Errorf("node ID cannot be empty")
	}
	if dataDir == "" {
		return nil, fmt.Errorf("data directory cannot be empty")
	}
	if bindAddr == "" {
		return nil, fmt.Errorf("bind address cannot be empty")
	}

	config := raft.DefaultConfig()
	config.LocalID = raft.ServerID(nodeID)

	if err := os.MkdirAll(dataDir, 0750); err != nil {
		return nil, fmt.Errorf("creating data directory %q: %w", dataDir, err)
	}

	fsm := &fileSystemFSM{
		store:   make(map[string][]byte),
		dataDir: dataDir,
	}

	// Create Raft log store
	logStore, err := raftboltdb.NewBoltStore(filepath.Join(dataDir, "raft-log.db"))
	if err != nil {
		return nil, fmt.Errorf("creating log store: %w", err)
	}

	// Create Raft stable store (can reuse the same BoltDB)
	stableStore := logStore

	// Create snapshot store
	snapshots, err := raft.NewFileSnapshotStore(dataDir, defaultSnapshotRetain, os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("creating snapshot store: %w", err)
	}

	// Create network transport
	transport, err := raft.NewTCPTransport(bindAddr, nil, defaultTransportMaxPool, defaultTransportTimeout, os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("creating transport: %w", err)
	}

	// Create Raft instance
	r, err := raft.NewRaft(config, fsm, logStore, stableStore, snapshots, transport)
	if err != nil {
		return nil, fmt.Errorf("creating raft instance: %w", err)
	}

	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	return &RaftManager{
		raft:      r,
		fsm:       fsm,
		store:     logStore,
		snapshots: snapshots,
		transport: transport,
		logger:    logger,
		nodeID:    nodeID,
		dataDir:   dataDir,
	}, nil
}

// Apply executes a command on the finite state machine.
func (fsm *fileSystemFSM) Apply(log *raft.Log) interface{} {
	var cmd Command
	if err := json.Unmarshal(log.Data, &cmd); err != nil {
		return fmt.Errorf("unmarshaling command: %w", err)
	}

	fsm.mu.Lock()
	defer fsm.mu.Unlock()

	switch cmd.Op {
	case OpWrite:
		return fsm.applyWrite(cmd)
	case OpDelete:
		return fsm.applyDelete(cmd)
	default:
		return fmt.Errorf("unknown operation: %q", cmd.Op)
	}
}

// applyWrite handles file write operations.
func (fsm *fileSystemFSM) applyWrite(cmd Command) error {
	filePath := filepath.Join(fsm.dataDir, cmd.Path)

	if err := os.MkdirAll(filepath.Dir(filePath), 0750); err != nil {
		return fmt.Errorf("creating directory for %q: %w", cmd.Path, err)
	}

	if err := os.WriteFile(filePath, cmd.Data, 0600); err != nil {
		return fmt.Errorf("writing file %q: %w", cmd.Path, err)
	}

	fsm.store[cmd.Path] = cmd.Data
	return nil
}

// applyDelete handles file deletion operations.
func (fsm *fileSystemFSM) applyDelete(cmd Command) error {
	filePath := filepath.Join(fsm.dataDir, cmd.Path)

	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("deleting file %q: %w", cmd.Path, err)
	}

	delete(fsm.store, cmd.Path)
	return nil
}

// Snapshot creates a point-in-time snapshot of the file system state.
func (fsm *fileSystemFSM) Snapshot() (raft.FSMSnapshot, error) {
	fsm.mu.RLock()
	defer fsm.mu.RUnlock()

	// Create a deep copy of the current state
	snapshot := make(map[string][]byte, len(fsm.store))
	for k, v := range fsm.store {
		// Copy the byte slice to avoid sharing memory
		data := make([]byte, len(v))
		copy(data, v)
		snapshot[k] = data
	}

	return &fileSystemSnapshot{store: snapshot}, nil
}

// Restore reconstructs the file system state from a snapshot.
func (fsm *fileSystemFSM) Restore(rc io.ReadCloser) error {
	defer func() {
		if err := rc.Close(); err != nil {
			logrus.WithError(err).Warn("Error closing restore reader")
		}
	}()

	fsm.mu.Lock()
	defer fsm.mu.Unlock()

	var snapshot map[string][]byte
	if err := json.NewDecoder(rc).Decode(&snapshot); err != nil {
		return fmt.Errorf("decoding snapshot: %w", err)
	}

	fsm.store = snapshot
	return nil
}

// Persist writes the snapshot to persistent storage.
func (s *fileSystemSnapshot) Persist(sink raft.SnapshotSink) error {
	defer func() {
		if err := sink.Close(); err != nil {
			logrus.WithError(err).Warn("Error closing snapshot sink")
		}
	}()

	if err := json.NewEncoder(sink).Encode(s.store); err != nil {
		if cancelErr := sink.Cancel(); cancelErr != nil {
			logrus.WithError(cancelErr).Warn("Error canceling snapshot")
		}
		return fmt.Errorf("encoding snapshot: %w", err)
	}

	return nil
}

// Release is called when the snapshot is no longer needed.
func (s *fileSystemSnapshot) Release() {
	// No resources to clean up for this implementation
}

// ReplicateChunk replicates a data chunk across the Raft cluster.
func (rm *RaftManager) ReplicateChunk(chunkID ChunkID, data []byte) error {
	cmd := Command{
		Op:    OpWrite,
		Path:  fmt.Sprintf("chunks/%d_%d", chunkID.FileID, chunkID.ChunkIndex),
		Data:  data,
		Chunk: chunkID,
	}

	cmdData, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("marshaling command: %w", err)
	}

	future := rm.raft.Apply(cmdData, defaultApplyTimeout)
	if err := future.Error(); err != nil {
		return fmt.Errorf("applying command via raft: %w", err)
	}

	return nil
}

// BootstrapCluster initializes a new single-node Raft cluster.
func (rm *RaftManager) BootstrapCluster(servers []raft.Server) error {
	config := raft.Configuration{Servers: servers}

	future := rm.raft.BootstrapCluster(config)
	if err := future.Error(); err != nil {
		return fmt.Errorf("bootstrapping cluster: %w", err)
	}

	return nil
}

// AddVoter adds a new voting member to the Raft cluster.
func (rm *RaftManager) AddVoter(id, addr string) error {
	if id == "" {
		return fmt.Errorf("voter ID cannot be empty")
	}
	if addr == "" {
		return fmt.Errorf("voter address cannot be empty")
	}

	future := rm.raft.AddVoter(raft.ServerID(id), raft.ServerAddress(addr), 0, 0)
	if err := future.Error(); err != nil {
		return fmt.Errorf("adding voter %q at %q: %w", id, addr, err)
	}

	return nil
}

// Shutdown gracefully shuts down the Raft manager.
func (rm *RaftManager) Shutdown() error {
	if rm.raft != nil {
		future := rm.raft.Shutdown()
		if err := future.Error(); err != nil {
			return fmt.Errorf("shutting down raft: %w", err)
		}
	}

	if rm.transport != nil {
		if err := rm.transport.Close(); err != nil {
			return fmt.Errorf("closing transport: %w", err)
		}
	}

	return nil
}

// State returns the current Raft state (Leader, Follower, etc.).
func (rm *RaftManager) State() raft.RaftState {
	return rm.raft.State()
}

// IsLeader returns true if this node is the current Raft leader.
func (rm *RaftManager) IsLeader() bool {
	return rm.raft.State() == raft.Leader
}

// Leader returns the address of the current leader, if known.
func (rm *RaftManager) Leader() raft.ServerAddress {
	return rm.raft.Leader()
}
