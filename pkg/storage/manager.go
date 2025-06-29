// Package storage implements a distributed storage system with Raft consensus.
package storage

import (
	"fmt"
	"sync"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
)

// Manager coordinates storage operations across multiple nodes with Raft consensus.
type Manager struct {
	nodes       []*Node
	raftManager *RaftManager
}

// Node represents a single storage node in the distributed system.
type Node struct {
	id         uint32
	chunks     map[ChunkID]*Chunk
	chunkRoles map[ChunkID]Role
	mu         sync.RWMutex
}

// ChunkID uniquely identifies a data chunk within the system.
type ChunkID struct {
	FileID     uint32
	ChunkIndex uint32
}

// Chunk represents a unit of data with associated metadata.
type Chunk struct {
	ID       ChunkID
	Data     []byte
	Checksum uint64
	Version  uint64
}

// Role defines whether a chunk is primary or replica.
type Role int

const (
	RolePrimary Role = iota
	RoleReplica
)

// String returns a human-readable representation of the role.
func (r Role) String() string {
	switch r {
	case RolePrimary:
		return "primary"
	case RoleReplica:
		return "replica"
	default:
		return "unknown"
	}
}

// VectorClock provides logical time for conflict resolution in distributed systems.
type VectorClock struct {
	timestamps map[uint32]uint64
	mu         sync.RWMutex
}

// ManagerConfig holds configuration for creating a new Manager.
type ManagerConfig struct {
	NodeCount int
	NodeID    string
	DataDir   string
	BindAddr  string
}

// NewManager creates a new storage manager with the specified configuration.
func NewManager(cfg ManagerConfig) (*Manager, error) {
	if cfg.NodeCount < 0 {
		return nil, fmt.Errorf("node count cannot be negative: %d", cfg.NodeCount)
	}
	if cfg.NodeID == "" {
		return nil, fmt.Errorf("node ID cannot be empty")
	}
	if cfg.DataDir == "" {
		return nil, fmt.Errorf("data directory cannot be empty")
	}
	if cfg.BindAddr == "" {
		return nil, fmt.Errorf("bind address cannot be empty")
	}

	nodes := make([]*Node, cfg.NodeCount)
	for i := range nodes {
		nodes[i] = NewNode(uint32(i))
	}

	raftManager, err := NewRaftManager(cfg.NodeID, cfg.DataDir, cfg.BindAddr)
	if err != nil {
		return nil, fmt.Errorf("creating raft manager: %w", err)
	}

	return &Manager{
		nodes:       nodes,
		raftManager: raftManager,
	}, nil
}

// NewNode creates a new storage node with the given ID.
func NewNode(id uint32) *Node {
	return &Node{
		id:         id,
		chunks:     make(map[ChunkID]*Chunk),
		chunkRoles: make(map[ChunkID]Role),
	}
}

// NewVectorClock creates a new vector clock for logical time tracking.
func NewVectorClock() *VectorClock {
	return &VectorClock{
		timestamps: make(map[uint32]uint64),
	}
}

// ID returns the node's unique identifier.
func (n *Node) ID() uint32 {
	return n.id
}

// StoreChunk assigns a role to a chunk on this node.
func (n *Node) StoreChunk(chunkID ChunkID, role Role) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.chunkRoles[chunkID] = role
}

// RetrieveChunk returns the chunk with the given ID, or nil if not found.
func (n *Node) RetrieveChunk(chunkID ChunkID) *Chunk {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.chunks[chunkID]
}

// ReplicateChunk initiates replication of a chunk to other nodes via Raft.
// It only replicates chunks that this node owns as primary.
func (n *Node) ReplicateChunk(chunkID ChunkID, _ uint32, raftManager *RaftManager) error {
	n.mu.RLock()
	role, exists := n.chunkRoles[chunkID]
	chunk := n.chunks[chunkID]
	n.mu.RUnlock()

	if !exists {
		return fmt.Errorf("chunk %v not found on node %d", chunkID, n.id)
	}
	if role != RolePrimary {
		return fmt.Errorf("cannot replicate non-primary chunk %v from node %d", chunkID, n.id)
	}
	if chunk == nil {
		return fmt.Errorf("chunk %v data is nil on node %d", chunkID, n.id)
	}

	if err := raftManager.ReplicateChunk(chunkID, chunk.Data); err != nil {
		return fmt.Errorf("replicating chunk %v: %w", chunkID, err)
	}

	return nil
}

// Update modifies the timestamp for the given node ID if the new timestamp is greater.
func (vc *VectorClock) Update(nodeID uint32, timestamp uint64) {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	if current, exists := vc.timestamps[nodeID]; !exists || timestamp > current {
		vc.timestamps[nodeID] = timestamp
	}
}

// Compare returns the relationship between this vector clock and another.
// Returns -1 if this clock is before other, 1 if after, 0 if concurrent or equal.
func (vc *VectorClock) Compare(other *VectorClock) int {
	vc.mu.RLock()
	defer vc.mu.RUnlock()
	other.mu.RLock()
	defer other.mu.RUnlock()

	var thisBeforeOther, thisAfterOther bool

	// Check all timestamps in this clock
	for nodeID, timestamp := range vc.timestamps {
		otherTimestamp, exists := other.timestamps[nodeID]
		if !exists {
			thisAfterOther = true
			continue
		}
		if timestamp < otherTimestamp {
			thisBeforeOther = true
		} else if timestamp > otherTimestamp {
			thisAfterOther = true
		}
	}

	// Check timestamps that only exist in the other clock
	for nodeID := range other.timestamps {
		if _, exists := vc.timestamps[nodeID]; !exists {
			thisBeforeOther = true
		}
	}

	switch {
	case thisBeforeOther && thisAfterOther:
		return 0 // Concurrent
	case thisBeforeOther:
		return -1 // This is before other
	case thisAfterOther:
		return 1 // This is after other
	default:
		return 0 // Equal
	}
}

// BootstrapCluster initializes a new Raft cluster with the given servers.
func (m *Manager) BootstrapCluster(servers []raft.Server) error {
	return m.raftManager.BootstrapCluster(servers)
}

// AddVoter adds a new voting member to the Raft cluster.
func (m *Manager) AddVoter(id, addr string) error {
	return m.raftManager.AddVoter(id, addr)
}

// Raft returns the underlying Raft instance for advanced operations.
func (m *Manager) Raft() *raft.Raft {
	return m.raftManager.raft
}

// RaftComponents returns the Raft components for cluster joining operations.
func (m *Manager) RaftComponents() (raft.FSM, *raftboltdb.BoltStore, *raft.FileSnapshotStore) {
	return m.raftManager.fsm, m.raftManager.store, m.raftManager.snapshots
}
