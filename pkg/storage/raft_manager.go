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

// RaftManager implements replication using HashiCorp's Raft
type RaftManager struct {
	raft      *raft.Raft
	fsm       *FileSystemFSM
	store     *raftboltdb.BoltStore
	snapshots *raft.FileSnapshotStore
	transport *raft.NetworkTransport
	logger    *logrus.Logger
	nodeID    string
	dataDir   string
}

// FileSystemFSM implements the Raft FSM interface for file system operations
type FileSystemFSM struct {
	store   map[string][]byte
	mu      sync.RWMutex
	dataDir string
}

// Command represents a file system operation
type Command struct {
	Op    string  `json:"op"`    // "write" or "delete"
	Path  string  `json:"path"`  // file path
	Data  []byte  `json:"data"`  // file data
	Chunk ChunkID `json:"chunk"` // chunk identifier
}

// NewRaftManager creates a new Raft-based replication manager
func NewRaftManager(nodeID string, dataDir string, bindAddr string) (*RaftManager, error) {
	config := raft.DefaultConfig()
	config.LocalID = raft.ServerID(nodeID)

	// Create data directory if it doesn't exist
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %v", err)
	}

	// Create FSM
	fsm := &FileSystemFSM{
		store:   make(map[string][]byte),
		dataDir: dataDir,
	}

	// Create store for logs
	store, err := raftboltdb.NewBoltStore(filepath.Join(dataDir, "raft.db"))
	if err != nil {
		return nil, fmt.Errorf("failed to create bolt store: %v", err)
	}

	// Create snapshot store
	snapshots, err := raft.NewFileSnapshotStore(dataDir, 3, os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("failed to create snapshot store: %v", err)
	}

	// Create transport
	transport, err := raft.NewTCPTransport(bindAddr, nil, 3, 10*time.Second, os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport: %v", err)
	}

	// Create Raft instance
	r, err := raft.NewRaft(config, fsm, store, store, snapshots, transport)
	if err != nil {
		return nil, fmt.Errorf("failed to create raft: %v", err)
	}

	return &RaftManager{
		raft:      r,
		fsm:       fsm,
		store:     store,
		snapshots: snapshots,
		transport: transport,
		logger:    logrus.New(),
		nodeID:    nodeID,
		dataDir:   dataDir,
	}, nil
}

// Apply implements the FSM interface
func (f *FileSystemFSM) Apply(log *raft.Log) interface{} {
	var cmd Command
	if err := json.Unmarshal(log.Data, &cmd); err != nil {
		return fmt.Errorf("failed to unmarshal command: %v", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	switch cmd.Op {
	case "write":
		filePath := filepath.Join(f.dataDir, cmd.Path)
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			return fmt.Errorf("failed to create directory: %v", err)
		}
		if err := os.WriteFile(filePath, cmd.Data, 0644); err != nil {
			return fmt.Errorf("failed to write file: %v", err)
		}
		f.store[cmd.Path] = cmd.Data
	case "delete":
		filePath := filepath.Join(f.dataDir, cmd.Path)
		if err := os.Remove(filePath); err != nil {
			return fmt.Errorf("failed to delete file: %v", err)
		}
		delete(f.store, cmd.Path)
	}
	return nil
}

// Snapshot implements the FSM interface
func (f *FileSystemFSM) Snapshot() (raft.FSMSnapshot, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	snapshot := make(map[string][]byte)
	for k, v := range f.store {
		snapshot[k] = v
	}
	return &FileSystemSnapshot{store: snapshot}, nil
}

// Restore implements the FSM interface
func (f *FileSystemFSM) Restore(rc io.ReadCloser) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	defer rc.Close()

	var snapshot map[string][]byte
	if err := json.NewDecoder(rc).Decode(&snapshot); err != nil {
		return err
	}

	f.store = snapshot
	return nil
}

// FileSystemSnapshot implements the FSMSnapshot interface
type FileSystemSnapshot struct {
	store map[string][]byte
}

func (f *FileSystemSnapshot) Persist(sink raft.SnapshotSink) error {
	return json.NewEncoder(sink).Encode(f.store)
}

func (f *FileSystemSnapshot) Release() {}

// ReplicateChunk implements replication using Raft
func (rm *RaftManager) ReplicateChunk(chunkID ChunkID, data []byte) error {
	cmd := Command{
		Op:    "write",
		Path:  fmt.Sprintf("chunks/%d_%d", chunkID.FileID, chunkID.ChunkIndex),
		Data:  data,
		Chunk: chunkID,
	}

	data, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("failed to marshal command: %v", err)
	}

	future := rm.raft.Apply(data, 5*time.Second)
	if err := future.Error(); err != nil {
		return fmt.Errorf("failed to apply command: %v", err)
	}

	return nil
}

// BootstrapCluster initializes a new Raft cluster
func (rm *RaftManager) BootstrapCluster(servers []raft.Server) error {
	config := raft.Configuration{
		Servers: servers,
	}
	return rm.raft.BootstrapCluster(config).Error()
}

// AddVoter adds a new voter to the cluster
func (rm *RaftManager) AddVoter(id string, addr string) error {
	return rm.raft.AddVoter(raft.ServerID(id), raft.ServerAddress(addr), 0, 0).Error()
}
