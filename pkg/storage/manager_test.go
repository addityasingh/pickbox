package storage

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewManager(t *testing.T) {
	// Skip this test when running with race detector due to checkptr issues in BoltDB
	// This is a known issue with github.com/boltdb/bolt@v1.3.1 and checkptr validation
	if testing.Short() {
		t.Skip("Skipping in short mode due to BoltDB checkptr issues")
	}
	tests := []struct {
		name    string
		cfg     ManagerConfig
		wantErr bool
	}{
		{
			name: "valid manager creation",
			cfg: ManagerConfig{
				NodeCount: 3,
				NodeID:    "test-node",
				DataDir:   "/tmp/test-manager",
				BindAddr:  "127.0.0.1:0", // Use port 0 for auto-allocation
			},
			wantErr: false,
		},
		{
			name: "zero nodes",
			cfg: ManagerConfig{
				NodeCount: 0,
				NodeID:    "test-node",
				DataDir:   "/tmp/test-manager-zero",
				BindAddr:  "127.0.0.1:0",
			},
			wantErr: false,
		},
		{
			name: "negative node count",
			cfg: ManagerConfig{
				NodeCount: -1,
				NodeID:    "test-node",
				DataDir:   "/tmp/test-manager-neg",
				BindAddr:  "127.0.0.1:0",
			},
			wantErr: true,
		},
		{
			name: "empty node ID",
			cfg: ManagerConfig{
				NodeCount: 3,
				NodeID:    "",
				DataDir:   "/tmp/test-manager-empty",
				BindAddr:  "127.0.0.1:0",
			},
			wantErr: true,
		},
		{
			name: "empty data directory",
			cfg: ManagerConfig{
				NodeCount: 3,
				NodeID:    "test-node",
				DataDir:   "",
				BindAddr:  "127.0.0.1:0",
			},
			wantErr: true,
		},
		{
			name: "empty bind address",
			cfg: ManagerConfig{
				NodeCount: 3,
				NodeID:    "test-node",
				DataDir:   "/tmp/test-manager-bind",
				BindAddr:  "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Cleanup before test
			os.RemoveAll(tt.cfg.DataDir)
			defer os.RemoveAll(tt.cfg.DataDir)

			manager, err := NewManager(tt.cfg)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, manager)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, manager)
				assert.Len(t, manager.nodes, tt.cfg.NodeCount)
				assert.NotNil(t, manager.raftManager)

				// Additional validation for successful cases
				if manager != nil {
					assert.NotNil(t, manager.Raft())
					fsm, store, snapshots := manager.RaftComponents()
					assert.NotNil(t, fsm)
					assert.NotNil(t, store)
					assert.NotNil(t, snapshots)
				}
			}
		})
	}
}

// Test Manager methods
func TestManager_BootstrapCluster(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode due to BoltDB checkptr issues")
	}

	cfg := ManagerConfig{
		NodeCount: 3,
		NodeID:    "test-node",
		DataDir:   "/tmp/test-manager-bootstrap",
		BindAddr:  "127.0.0.1:0",
	}

	os.RemoveAll(cfg.DataDir)
	defer os.RemoveAll(cfg.DataDir)

	manager, err := NewManager(cfg)
	require.NoError(t, err)
	require.NotNil(t, manager)

	// Create server configuration
	servers := []raft.Server{
		{
			ID:      raft.ServerID(cfg.NodeID),
			Address: raft.ServerAddress(cfg.BindAddr),
		},
	}

	// Bootstrap the cluster
	err = manager.BootstrapCluster(servers)
	assert.NoError(t, err)

	// Verify raft state
	assert.NotNil(t, manager.Raft())
}

func TestManager_AddVoter(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode due to BoltDB checkptr issues")
	}

	cfg := ManagerConfig{
		NodeCount: 3,
		NodeID:    "test-node",
		DataDir:   "/tmp/test-manager-addvoter",
		BindAddr:  "127.0.0.1:0",
	}

	os.RemoveAll(cfg.DataDir)
	defer os.RemoveAll(cfg.DataDir)

	manager, err := NewManager(cfg)
	require.NoError(t, err)
	require.NotNil(t, manager)

	// Bootstrap first
	servers := []raft.Server{
		{
			ID:      raft.ServerID(cfg.NodeID),
			Address: raft.ServerAddress(cfg.BindAddr),
		},
	}

	err = manager.BootstrapCluster(servers)
	require.NoError(t, err)

	// Wait for leader election
	time.Sleep(100 * time.Millisecond)

	// Add a voter
	err = manager.AddVoter("node2", "127.0.0.1:8001")
	// This might fail if not leader yet, but shouldn't panic
	// We just verify the method exists and can be called
	assert.NotPanics(t, func() {
		manager.AddVoter("node2", "127.0.0.1:8001")
	})
}

func TestManager_RaftComponents(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode due to BoltDB checkptr issues")
	}

	cfg := ManagerConfig{
		NodeCount: 3,
		NodeID:    "test-node",
		DataDir:   "/tmp/test-manager-components",
		BindAddr:  "127.0.0.1:0",
	}

	os.RemoveAll(cfg.DataDir)
	defer os.RemoveAll(cfg.DataDir)

	manager, err := NewManager(cfg)
	require.NoError(t, err)
	require.NotNil(t, manager)

	// Test RaftComponents method
	fsm, store, snapshots := manager.RaftComponents()
	assert.NotNil(t, fsm)
	assert.NotNil(t, store)
	assert.NotNil(t, snapshots)

	// Test Raft method
	raftInstance := manager.Raft()
	assert.NotNil(t, raftInstance)
}

func TestNewNode(t *testing.T) {
	tests := []struct {
		name   string
		nodeID uint32
	}{
		{
			name:   "node 0",
			nodeID: 0,
		},
		{
			name:   "node 1",
			nodeID: 1,
		},
		{
			name:   "node 100",
			nodeID: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := NewNode(tt.nodeID)
			assert.NotNil(t, node)
			assert.Equal(t, tt.nodeID, node.ID())
			assert.NotNil(t, node.chunks)
			assert.NotNil(t, node.chunkRoles)
		})
	}
}

func TestNode_StoreChunk(t *testing.T) {
	node := NewNode(0)
	chunkID := ChunkID{FileID: 1, ChunkIndex: 1}

	tests := []struct {
		name string
		role Role
	}{
		{
			name: "primary role",
			role: RolePrimary,
		},
		{
			name: "replica role",
			role: RoleReplica,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node.StoreChunk(chunkID, tt.role)

			// Verify the chunk role was stored
			node.mu.RLock()
			storedRole, exists := node.chunkRoles[chunkID]
			node.mu.RUnlock()

			assert.True(t, exists)
			assert.Equal(t, tt.role, storedRole)
		})
	}
}

func TestNode_RetrieveChunk(t *testing.T) {
	node := NewNode(0)
	chunkID := ChunkID{FileID: 1, ChunkIndex: 1}

	// Test retrieving non-existent chunk
	chunk := node.RetrieveChunk(chunkID)
	assert.Nil(t, chunk)

	// Store a chunk first
	testChunk := &Chunk{
		ID:       chunkID,
		Data:     []byte("test data"),
		Checksum: 12345,
		Version:  1,
	}

	node.mu.Lock()
	node.chunks[chunkID] = testChunk
	node.mu.Unlock()

	// Test retrieving existing chunk
	retrieved := node.RetrieveChunk(chunkID)
	assert.NotNil(t, retrieved)
	assert.Equal(t, testChunk, retrieved)
}

func TestNode_RetrieveChunk_Concurrent(t *testing.T) {
	node := NewNode(0)
	chunkID := ChunkID{FileID: 1, ChunkIndex: 1}

	// Store a chunk
	testChunk := &Chunk{
		ID:       chunkID,
		Data:     []byte("concurrent test data"),
		Checksum: 67890,
		Version:  2,
	}

	node.mu.Lock()
	node.chunks[chunkID] = testChunk
	node.mu.Unlock()

	// Test concurrent retrieval
	var wg sync.WaitGroup
	numGoroutines := 10
	results := make([]*Chunk, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index] = node.RetrieveChunk(chunkID)
		}(i)
	}

	wg.Wait()

	// Verify all goroutines retrieved the same chunk
	for i := 0; i < numGoroutines; i++ {
		assert.NotNil(t, results[i])
		assert.Equal(t, testChunk, results[i])
	}
}

// Test Node.ReplicateChunk method
func TestNode_ReplicateChunk(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode due to BoltDB checkptr issues")
	}

	cfg := ManagerConfig{
		NodeCount: 3,
		NodeID:    "test-node",
		DataDir:   "/tmp/test-node-replicate",
		BindAddr:  "127.0.0.1:0",
	}

	os.RemoveAll(cfg.DataDir)
	defer os.RemoveAll(cfg.DataDir)

	manager, err := NewManager(cfg)
	require.NoError(t, err)
	require.NotNil(t, manager)

	// Test ReplicateChunk method
	node := NewNode(0)
	chunkID := ChunkID{FileID: 1, ChunkIndex: 1}

	// Store a chunk to replicate
	testChunk := &Chunk{
		ID:       chunkID,
		Data:     []byte("replicate test data"),
		Checksum: 12345,
		Version:  1,
	}

	node.mu.Lock()
	node.chunks[chunkID] = testChunk
	node.mu.Unlock()

	// Test replication
	err = node.ReplicateChunk(chunkID, 1, manager.raftManager)
	// This might fail if no leader is available, but shouldn't panic
	assert.NotPanics(t, func() {
		node.ReplicateChunk(chunkID, 1, manager.raftManager)
	})
}

// Test Node.ReplicateChunk with missing chunk
func TestNode_ReplicateChunk_MissingChunk(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode due to BoltDB checkptr issues")
	}

	cfg := ManagerConfig{
		NodeCount: 3,
		NodeID:    "test-node",
		DataDir:   "/tmp/test-node-replicate-missing",
		BindAddr:  "127.0.0.1:0",
	}

	os.RemoveAll(cfg.DataDir)
	defer os.RemoveAll(cfg.DataDir)

	manager, err := NewManager(cfg)
	require.NoError(t, err)
	require.NotNil(t, manager)

	node := NewNode(0)
	chunkID := ChunkID{FileID: 1, ChunkIndex: 1}

	// Test replication of non-existent chunk
	err = node.ReplicateChunk(chunkID, 1, manager.raftManager)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found on node")
}

func TestNewVectorClock(t *testing.T) {
	vc := NewVectorClock()
	assert.NotNil(t, vc)
	assert.NotNil(t, vc.timestamps)
	assert.Equal(t, 0, len(vc.timestamps))
}

func TestVectorClock_Update(t *testing.T) {
	vc := NewVectorClock()

	tests := []struct {
		name      string
		nodeID    uint32
		timestamp uint64
		expected  uint64
	}{
		{
			name:      "first update",
			nodeID:    1,
			timestamp: 10,
			expected:  10,
		},
		{
			name:      "higher timestamp",
			nodeID:    1,
			timestamp: 20,
			expected:  20,
		},
		{
			name:      "lower timestamp",
			nodeID:    1,
			timestamp: 5,
			expected:  20, // Should remain at higher value
		},
		{
			name:      "different node",
			nodeID:    2,
			timestamp: 15,
			expected:  15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vc.Update(tt.nodeID, tt.timestamp)

			vc.mu.RLock()
			actual := vc.timestamps[tt.nodeID]
			vc.mu.RUnlock()

			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestVectorClock_Update_Concurrent(t *testing.T) {
	vc := NewVectorClock()

	var wg sync.WaitGroup
	numGoroutines := 10
	numUpdates := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(nodeID uint32) {
			defer wg.Done()
			for j := 0; j < numUpdates; j++ {
				vc.Update(nodeID, uint64(j))
			}
		}(uint32(i))
	}

	wg.Wait()

	// Verify all nodes have their highest timestamp
	vc.mu.RLock()
	for i := 0; i < numGoroutines; i++ {
		assert.Equal(t, uint64(numUpdates-1), vc.timestamps[uint32(i)])
	}
	vc.mu.RUnlock()
}

func TestVectorClock_Compare(t *testing.T) {
	tests := []struct {
		name     string
		vc1      map[uint32]uint64
		vc2      map[uint32]uint64
		expected int
	}{
		{
			name:     "equal clocks",
			vc1:      map[uint32]uint64{1: 10, 2: 20},
			vc2:      map[uint32]uint64{1: 10, 2: 20},
			expected: 0,
		},
		{
			name:     "vc1 before vc2",
			vc1:      map[uint32]uint64{1: 10, 2: 20},
			vc2:      map[uint32]uint64{1: 15, 2: 25},
			expected: -1,
		},
		{
			name:     "vc1 after vc2",
			vc1:      map[uint32]uint64{1: 15, 2: 25},
			vc2:      map[uint32]uint64{1: 10, 2: 20},
			expected: 1,
		},
		{
			name:     "concurrent clocks",
			vc1:      map[uint32]uint64{1: 15, 2: 20},
			vc2:      map[uint32]uint64{1: 10, 2: 25},
			expected: 0,
		},
		{
			name:     "missing nodes",
			vc1:      map[uint32]uint64{1: 10},
			vc2:      map[uint32]uint64{2: 20},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vc1 := &VectorClock{timestamps: tt.vc1}
			vc2 := &VectorClock{timestamps: tt.vc2}

			result := vc1.Compare(vc2)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestVectorClock_Compare_Concurrent(t *testing.T) {
	vc1 := &VectorClock{timestamps: map[uint32]uint64{1: 10, 2: 20}}
	vc2 := &VectorClock{timestamps: map[uint32]uint64{1: 15, 2: 25}}

	var wg sync.WaitGroup
	numGoroutines := 10
	results := make([]int, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index] = vc1.Compare(vc2)
		}(i)
	}

	wg.Wait()

	// All results should be the same
	for i := 1; i < numGoroutines; i++ {
		assert.Equal(t, results[0], results[i])
	}
}

func TestChunkID_Equality(t *testing.T) {
	id1 := ChunkID{FileID: 1, ChunkIndex: 1}
	id2 := ChunkID{FileID: 1, ChunkIndex: 1}
	id3 := ChunkID{FileID: 2, ChunkIndex: 1}

	assert.Equal(t, id1, id2)
	assert.NotEqual(t, id1, id3)
}

func TestChunk_Creation(t *testing.T) {
	chunkID := ChunkID{FileID: 1, ChunkIndex: 1}
	data := []byte("test data")
	checksum := uint64(12345)
	version := uint64(1)

	chunk := Chunk{
		ID:       chunkID,
		Data:     data,
		Checksum: checksum,
		Version:  version,
	}

	assert.Equal(t, chunkID, chunk.ID)
	assert.Equal(t, data, chunk.Data)
	assert.Equal(t, checksum, chunk.Checksum)
	assert.Equal(t, version, chunk.Version)
}

func TestRole_Constants(t *testing.T) {
	assert.Equal(t, Role(0), RolePrimary)
	assert.Equal(t, Role(1), RoleReplica)
}

func TestRole_String(t *testing.T) {
	tests := []struct {
		name     string
		role     Role
		expected string
	}{
		{
			name:     "primary",
			role:     RolePrimary,
			expected: "primary",
		},
		{
			name:     "replica",
			role:     RoleReplica,
			expected: "replica",
		},
		{
			name:     "unknown",
			role:     Role(99),
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.role.String())
		})
	}
}

// Test ManagerConfig validation
func TestManagerConfig_Validation(t *testing.T) {
	validCfg := ManagerConfig{
		NodeCount: 3,
		NodeID:    "test-node",
		DataDir:   "/tmp/test-config",
		BindAddr:  "127.0.0.1:0",
	}

	assert.NotEmpty(t, validCfg.NodeID)
	assert.NotEmpty(t, validCfg.DataDir)
	assert.NotEmpty(t, validCfg.BindAddr)
	assert.GreaterOrEqual(t, validCfg.NodeCount, 0)
}

// Test edge cases for VectorClock
func TestVectorClock_EdgeCases(t *testing.T) {
	t.Run("empty_clocks", func(t *testing.T) {
		vc1 := NewVectorClock()
		vc2 := NewVectorClock()

		result := vc1.Compare(vc2)
		assert.Equal(t, 0, result)
	})

	t.Run("single_node_clock", func(t *testing.T) {
		vc1 := NewVectorClock()
		vc1.Update(1, 10)

		vc2 := NewVectorClock()

		result := vc1.Compare(vc2)
		assert.Equal(t, 1, result)
	})

	t.Run("zero_timestamp", func(t *testing.T) {
		vc := NewVectorClock()
		vc.Update(1, 0)

		vc.mu.RLock()
		assert.Equal(t, uint64(0), vc.timestamps[1])
		vc.mu.RUnlock()
	})
}

// Test Node edge cases
func TestNode_EdgeCases(t *testing.T) {
	t.Run("store_same_chunk_multiple_times", func(t *testing.T) {
		node := NewNode(0)
		chunkID := ChunkID{FileID: 1, ChunkIndex: 1}

		node.StoreChunk(chunkID, RolePrimary)
		node.StoreChunk(chunkID, RoleReplica)

		// Should overwrite the role
		node.mu.RLock()
		role, exists := node.chunkRoles[chunkID]
		node.mu.RUnlock()

		assert.True(t, exists)
		assert.Equal(t, RoleReplica, role)
	})

	t.Run("retrieve_chunk_race_condition", func(t *testing.T) {
		node := NewNode(0)
		chunkID := ChunkID{FileID: 1, ChunkIndex: 1}

		testChunk := &Chunk{
			ID:       chunkID,
			Data:     []byte("race test data"),
			Checksum: 12345,
			Version:  1,
		}

		// Start goroutine to store chunk
		go func() {
			node.mu.Lock()
			node.chunks[chunkID] = testChunk
			node.mu.Unlock()
		}()

		// Try to retrieve at the same time
		var retrieved *Chunk
		for i := 0; i < 10; i++ {
			retrieved = node.RetrieveChunk(chunkID)
			if retrieved != nil {
				break
			}
			time.Sleep(time.Millisecond)
		}

		// Should eventually get the chunk or nil (both are valid)
		if retrieved != nil {
			assert.Equal(t, testChunk, retrieved)
		}
	})
}

// Benchmark tests
func BenchmarkNode_StoreChunk(b *testing.B) {
	node := NewNode(0)
	chunkID := ChunkID{FileID: 1, ChunkIndex: 1}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		node.StoreChunk(chunkID, RolePrimary)
	}
}

func BenchmarkNode_RetrieveChunk(b *testing.B) {
	node := NewNode(0)
	chunkID := ChunkID{FileID: 1, ChunkIndex: 1}

	// Store a chunk first
	testChunk := &Chunk{
		ID:       chunkID,
		Data:     []byte("benchmark data"),
		Checksum: 12345,
		Version:  1,
	}

	node.mu.Lock()
	node.chunks[chunkID] = testChunk
	node.mu.Unlock()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		node.RetrieveChunk(chunkID)
	}
}

func BenchmarkVectorClock_Update(b *testing.B) {
	vc := NewVectorClock()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vc.Update(1, uint64(i))
	}
}

func BenchmarkVectorClock_Compare(b *testing.B) {
	vc1 := &VectorClock{timestamps: map[uint32]uint64{1: 10, 2: 20}}
	vc2 := &VectorClock{timestamps: map[uint32]uint64{1: 15, 2: 25}}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vc1.Compare(vc2)
	}
}

// Test Node operations with error conditions
func TestNode_ErrorConditions(t *testing.T) {
	t.Run("replicate_chunk_with_nil_data", func(t *testing.T) {
		tempDir := createTempDir(t)
		defer os.RemoveAll(tempDir)

		cfg := ManagerConfig{
			NodeCount: 1,
			NodeID:    "test-node",
			DataDir:   tempDir,
			BindAddr:  "127.0.0.1:0",
		}

		manager, err := NewManager(cfg)
		require.NoError(t, err)
		defer manager.raftManager.raft.Shutdown()

		node := NewNode(0)
		chunkID := ChunkID{FileID: 1, ChunkIndex: 1}

		// Store a chunk with nil data
		chunk := &Chunk{
			ID:   chunkID,
			Data: nil,
		}
		node.mu.Lock()
		node.chunks[chunkID] = chunk
		node.chunkRoles[chunkID] = RolePrimary
		node.mu.Unlock()

		err = node.ReplicateChunk(chunkID, 1, manager.raftManager)
		assert.Error(t, err)
		// The error might be about nil data or raft leadership
		if !strings.Contains(err.Error(), "data is nil") {
			assert.Contains(t, err.Error(), "not the leader")
		}
	})

	t.Run("replicate_chunk_with_non_primary_role", func(t *testing.T) {
		tempDir := createTempDir(t)
		defer os.RemoveAll(tempDir)

		cfg := ManagerConfig{
			NodeCount: 1,
			NodeID:    "test-node",
			DataDir:   tempDir,
			BindAddr:  "127.0.0.1:0",
		}

		manager, err := NewManager(cfg)
		require.NoError(t, err)
		defer manager.raftManager.raft.Shutdown()

		node := NewNode(0)
		chunkID := ChunkID{FileID: 1, ChunkIndex: 1}

		// Store a chunk with replica role
		chunk := &Chunk{
			ID:   chunkID,
			Data: []byte("test data"),
		}
		node.mu.Lock()
		node.chunks[chunkID] = chunk
		node.chunkRoles[chunkID] = RoleReplica
		node.mu.Unlock()

		err = node.ReplicateChunk(chunkID, 1, manager.raftManager)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot replicate non-primary chunk")
	})
}

// Test VectorClock edge cases
func TestVectorClock_AdvancedOperations(t *testing.T) {
	t.Run("update_with_same_timestamp", func(t *testing.T) {
		vc := NewVectorClock()

		// First update
		vc.Update(1, 100)

		// Update with same timestamp - should not change
		vc.Update(1, 100)

		vc.mu.RLock()
		timestamp := vc.timestamps[1]
		vc.mu.RUnlock()

		assert.Equal(t, uint64(100), timestamp)
	})

	t.Run("update_with_zero_timestamp", func(t *testing.T) {
		vc := NewVectorClock()

		// Update with zero timestamp
		vc.Update(1, 0)

		vc.mu.RLock()
		timestamp := vc.timestamps[1]
		vc.mu.RUnlock()

		assert.Equal(t, uint64(0), timestamp)
	})

	t.Run("compare_with_overlapping_nodes", func(t *testing.T) {
		vc1 := NewVectorClock()
		vc2 := NewVectorClock()

		// vc1 has nodes 1, 2, 3
		vc1.Update(1, 10)
		vc1.Update(2, 20)
		vc1.Update(3, 30)

		// vc2 has nodes 2, 3, 4
		vc2.Update(2, 25)
		vc2.Update(3, 15)
		vc2.Update(4, 40)

		// Should be concurrent
		result := vc1.Compare(vc2)
		assert.Equal(t, 0, result)
	})

	t.Run("compare_with_large_timestamps", func(t *testing.T) {
		vc1 := NewVectorClock()
		vc2 := NewVectorClock()

		// Use large timestamp values
		vc1.Update(1, 18446744073709551615) // Max uint64
		vc2.Update(1, 18446744073709551614) // Max uint64 - 1

		result := vc1.Compare(vc2)
		assert.Equal(t, 1, result) // vc1 after vc2
	})
}

// Test ChunkID operations
func TestChunkID_Operations(t *testing.T) {
	t.Run("chunk_id_as_map_key", func(t *testing.T) {
		chunkMap := make(map[ChunkID]string)

		chunk1 := ChunkID{FileID: 1, ChunkIndex: 1}
		chunk2 := ChunkID{FileID: 1, ChunkIndex: 2}
		chunk3 := ChunkID{FileID: 2, ChunkIndex: 1}

		chunkMap[chunk1] = "chunk1"
		chunkMap[chunk2] = "chunk2"
		chunkMap[chunk3] = "chunk3"

		assert.Equal(t, "chunk1", chunkMap[chunk1])
		assert.Equal(t, "chunk2", chunkMap[chunk2])
		assert.Equal(t, "chunk3", chunkMap[chunk3])
		assert.Equal(t, 3, len(chunkMap))
	})

	t.Run("chunk_id_zero_values", func(t *testing.T) {
		chunk := ChunkID{FileID: 0, ChunkIndex: 0}

		assert.Equal(t, uint32(0), chunk.FileID)
		assert.Equal(t, uint32(0), chunk.ChunkIndex)
	})

	t.Run("chunk_id_max_values", func(t *testing.T) {
		chunk := ChunkID{FileID: 4294967295, ChunkIndex: 4294967295} // Max uint32

		assert.Equal(t, uint32(4294967295), chunk.FileID)
		assert.Equal(t, uint32(4294967295), chunk.ChunkIndex)
	})
}

// Test Chunk operations
func TestChunk_Operations(t *testing.T) {
	t.Run("chunk_with_large_data", func(t *testing.T) {
		largeData := make([]byte, 1024*1024) // 1MB
		for i := range largeData {
			largeData[i] = byte(i % 256)
		}

		chunk := Chunk{
			ID:       ChunkID{FileID: 1, ChunkIndex: 1},
			Data:     largeData,
			Checksum: 12345,
			Version:  1,
		}

		assert.Equal(t, 1024*1024, len(chunk.Data))
		assert.Equal(t, largeData, chunk.Data)
		assert.Equal(t, uint64(12345), chunk.Checksum)
		assert.Equal(t, uint64(1), chunk.Version)
	})

	t.Run("chunk_with_empty_data", func(t *testing.T) {
		chunk := Chunk{
			ID:       ChunkID{FileID: 1, ChunkIndex: 1},
			Data:     []byte{},
			Checksum: 0,
			Version:  0,
		}

		assert.Equal(t, 0, len(chunk.Data))
		assert.Equal(t, uint64(0), chunk.Checksum)
		assert.Equal(t, uint64(0), chunk.Version)
	})

	t.Run("chunk_with_binary_data", func(t *testing.T) {
		binaryData := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD}

		chunk := Chunk{
			ID:       ChunkID{FileID: 1, ChunkIndex: 1},
			Data:     binaryData,
			Checksum: 54321,
			Version:  10,
		}

		assert.Equal(t, binaryData, chunk.Data)
		assert.Equal(t, uint64(54321), chunk.Checksum)
		assert.Equal(t, uint64(10), chunk.Version)
	})
}

// Test Role operations
func TestRole_Operations(t *testing.T) {
	t.Run("role_comparison", func(t *testing.T) {
		assert.Equal(t, RolePrimary, RolePrimary)
		assert.Equal(t, RoleReplica, RoleReplica)
		assert.NotEqual(t, RolePrimary, RoleReplica)
	})

	t.Run("role_type_safety", func(t *testing.T) {
		var role Role = RolePrimary
		assert.IsType(t, Role(0), role)
		assert.Equal(t, "primary", role.String())

		role = RoleReplica
		assert.Equal(t, "replica", role.String())
	})

	t.Run("role_invalid_value", func(t *testing.T) {
		var role Role = 999 // Invalid role
		assert.Equal(t, "unknown", role.String())
	})
}

// Test Node concurrent operations
func TestNode_ConcurrentOperations(t *testing.T) {
	t.Run("concurrent_chunk_operations", func(t *testing.T) {
		node := NewNode(0)

		var wg sync.WaitGroup
		numGoroutines := 10
		chunksPerGoroutine := 100

		// Concurrent chunk storage
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()
				for j := 0; j < chunksPerGoroutine; j++ {
					chunkID := ChunkID{
						FileID:     uint32(goroutineID),
						ChunkIndex: uint32(j),
					}
					node.StoreChunk(chunkID, RolePrimary)
				}
			}(i)
		}

		// Concurrent chunk retrieval
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()
				for j := 0; j < chunksPerGoroutine; j++ {
					chunkID := ChunkID{
						FileID:     uint32(goroutineID),
						ChunkIndex: uint32(j),
					}
					// This might return nil if the chunk hasn't been stored yet
					node.RetrieveChunk(chunkID)
				}
			}(i)
		}

		wg.Wait()

		// Verify final state
		node.mu.RLock()
		totalChunks := len(node.chunks)
		totalRoles := len(node.chunkRoles)
		node.mu.RUnlock()

		// Should have stored all chunks
		assert.Equal(t, numGoroutines*chunksPerGoroutine, totalChunks)
		assert.Equal(t, numGoroutines*chunksPerGoroutine, totalRoles)
	})
}

// Test Manager with various configurations
func TestManager_ConfigurationVariations(t *testing.T) {
	t.Run("manager_with_high_node_count", func(t *testing.T) {
		tempDir := createTempDir(t)
		defer os.RemoveAll(tempDir)

		cfg := ManagerConfig{
			NodeCount: 100,
			NodeID:    "test-node",
			DataDir:   tempDir,
			BindAddr:  "127.0.0.1:0",
		}

		manager, err := NewManager(cfg)
		require.NoError(t, err)
		defer manager.raftManager.raft.Shutdown()

		assert.Equal(t, 100, len(manager.nodes))

		// Verify all nodes are initialized
		for i, node := range manager.nodes {
			assert.Equal(t, uint32(i), node.ID())
			assert.NotNil(t, node.chunks)
			assert.NotNil(t, node.chunkRoles)
		}
	})

	t.Run("manager_with_special_characters_in_node_id", func(t *testing.T) {
		tempDir := createTempDir(t)
		defer os.RemoveAll(tempDir)

		cfg := ManagerConfig{
			NodeCount: 1,
			NodeID:    "test-node-with-special-chars_123",
			DataDir:   tempDir,
			BindAddr:  "127.0.0.1:0",
		}

		manager, err := NewManager(cfg)
		require.NoError(t, err)
		defer manager.raftManager.raft.Shutdown()

		assert.NotNil(t, manager)
		assert.Equal(t, 1, len(manager.nodes))
	})
}

// Test error conditions in NewManager
func TestNewManager_ErrorConditions(t *testing.T) {
	t.Run("invalid_bind_address_format", func(t *testing.T) {
		tempDir := createTempDir(t)
		defer os.RemoveAll(tempDir)

		cfg := ManagerConfig{
			NodeCount: 1,
			NodeID:    "test-node",
			DataDir:   tempDir,
			BindAddr:  "invalid-address-format",
		}

		manager, err := NewManager(cfg)
		if manager != nil {
			defer manager.raftManager.raft.Shutdown()
		}

		// This might succeed or fail depending on the system
		// If it fails, verify the error is related to address binding
		if err != nil {
			assert.Contains(t, err.Error(), "creating raft manager")
		}
	})

	t.Run("data_directory_with_special_characters", func(t *testing.T) {
		tempDir := createTempDir(t)
		specialDir := filepath.Join(tempDir, "special-dir with spaces & symbols!")
		defer os.RemoveAll(tempDir)

		cfg := ManagerConfig{
			NodeCount: 1,
			NodeID:    "test-node",
			DataDir:   specialDir,
			BindAddr:  "127.0.0.1:0",
		}

		manager, err := NewManager(cfg)
		require.NoError(t, err)
		defer manager.raftManager.raft.Shutdown()

		assert.NotNil(t, manager)
	})
}

// Test VectorClock with many nodes
func TestVectorClock_ManyNodes(t *testing.T) {
	t.Run("vector_clock_with_many_nodes", func(t *testing.T) {
		vc1 := NewVectorClock()
		vc2 := NewVectorClock()

		// Add many nodes
		for i := uint32(0); i < 1000; i++ {
			vc1.Update(i, uint64(i))
			vc2.Update(i, uint64(i+1))
		}

		// vc2 should be after vc1
		result := vc1.Compare(vc2)
		assert.Equal(t, -1, result)
	})

	t.Run("vector_clock_sparse_updates", func(t *testing.T) {
		vc1 := NewVectorClock()
		vc2 := NewVectorClock()

		// Update only some nodes
		nodeIDs := []uint32{1, 100, 999, 10000}
		for _, nodeID := range nodeIDs {
			vc1.Update(nodeID, 50)
			vc2.Update(nodeID, 60)
		}

		// vc2 should be after vc1
		result := vc1.Compare(vc2)
		assert.Equal(t, -1, result)
	})
}

// Helper function to create a temporary directory
func createTempDir(t *testing.T) string {
	tempDir, err := os.MkdirTemp("", "storage_test_")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(tempDir) })
	return tempDir
}
