package storage

import (
	"sync"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
	"github.com/sirupsen/logrus"
)

// Manager handles the storage operations and replication
type Manager struct {
	nodes []*Node
	raft  *RaftManager
}

// Node represents a storage node in the system
type Node struct {
	ID         uint32
	Chunks     map[ChunkID]*Chunk
	ChunkRoles map[ChunkID]Role
	mu         sync.RWMutex
}

// ChunkID uniquely identifies a chunk of data
type ChunkID struct {
	FileID     uint32
	ChunkIndex uint32
}

// Chunk represents a chunk of data with metadata
type Chunk struct {
	ID       ChunkID
	Data     []byte
	Checksum uint64
	Version  uint64
}

// Role represents the role of a chunk (Primary or Replica)
type Role int

const (
	Primary Role = iota
	Replica
)

// VectorClock implements vector clock for conflict resolution
type VectorClock struct {
	Timestamps map[uint32]uint64
	mu         sync.RWMutex
}

// NewManager creates a new storage manager
func NewManager(nodeCount int, nodeID string, dataDir string, bindAddr string) (*Manager, error) {
	nodes := make([]*Node, nodeCount)
	for i := range nodes {
		nodes[i] = NewNode(uint32(i))
	}

	raft, err := NewRaftManager(nodeID, dataDir, bindAddr)
	if err != nil {
		return nil, err
	}

	return &Manager{
		nodes: nodes,
		raft:  raft,
	}, nil
}

// NewNode creates a new storage node
func NewNode(id uint32) *Node {
	return &Node{
		ID:         id,
		Chunks:     make(map[ChunkID]*Chunk),
		ChunkRoles: make(map[ChunkID]Role),
	}
}

// NewVectorClock creates a new vector clock
func NewVectorClock() *VectorClock {
	return &VectorClock{
		Timestamps: make(map[uint32]uint64),
	}
}

// StoreChunk stores a chunk in the specified node
func (n *Node) StoreChunk(chunkID ChunkID, role Role) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.ChunkRoles[chunkID] = role
}

// RetrieveChunk retrieves a chunk from the node
func (n *Node) RetrieveChunk(chunkID ChunkID) *Chunk {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.Chunks[chunkID]
}

// ReplicateChunk replicates a chunk to another node using Raft
func (n *Node) ReplicateChunk(chunkID ChunkID, targetNodeID uint32, raftManager *RaftManager) {
	n.mu.RLock()
	role, exists := n.ChunkRoles[chunkID]
	chunk := n.Chunks[chunkID]
	n.mu.RUnlock()

	if !exists || role != Primary || chunk == nil {
		return
	}

	// Use Raft to replicate the chunk
	if err := raftManager.ReplicateChunk(chunkID, chunk.Data); err != nil {
		// Log error but don't fail - Raft will handle retries
		logrus.WithError(err).Error("Failed to replicate chunk")
	}
}

// UpdateVectorClock updates the vector clock for a node
func (vc *VectorClock) UpdateVectorClock(nodeID uint32, timestamp uint64) {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	if current, exists := vc.Timestamps[nodeID]; !exists || timestamp > current {
		vc.Timestamps[nodeID] = timestamp
	}
}

// CompareVectorClocks compares two vector clocks for conflict detection
func (vc *VectorClock) CompareVectorClocks(other *VectorClock) int {
	vc.mu.RLock()
	defer vc.mu.RUnlock()
	other.mu.RLock()
	defer other.mu.RUnlock()

	var less, greater bool

	// Check all timestamps in both clocks
	for nodeID, timestamp := range vc.Timestamps {
		otherTimestamp, exists := other.Timestamps[nodeID]
		if !exists {
			greater = true
			continue
		}
		if timestamp < otherTimestamp {
			less = true
		} else if timestamp > otherTimestamp {
			greater = true
		}
	}

	// Check timestamps that only exist in the other clock
	for nodeID := range other.Timestamps {
		if _, exists := vc.Timestamps[nodeID]; !exists {
			less = true
			continue
		}
	}

	if less && greater {
		return 0 // Concurrent
	} else if less {
		return -1 // Before
	} else if greater {
		return 1 // After
	}
	return 0 // Equal
}

// BootstrapCluster initializes the Raft cluster
func (m *Manager) BootstrapCluster(servers []raft.Server) error {
	return m.raft.BootstrapCluster(servers)
}

// AddVoter adds a new voter to the Raft cluster
func (m *Manager) AddVoter(id string, addr string) error {
	return m.raft.AddVoter(id, addr)
}

// GetRaft returns the underlying Raft instance
func (m *Manager) GetRaft() *raft.Raft {
	return m.raft.raft
}

// GetRaftComponents returns the Raft components needed for joining
func (m *Manager) GetRaftComponents() (raft.FSM, *raftboltdb.BoltStore, *raft.FileSnapshotStore) {
	return m.raft.fsm, m.raft.store, m.raft.snapshots
}
