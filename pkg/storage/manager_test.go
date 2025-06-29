package storage

import (
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
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

				// Verify all nodes are properly initialized
				for i, node := range manager.nodes {
					assert.Equal(t, uint32(i), node.ID())
					assert.NotNil(t, node.chunks)
					assert.NotNil(t, node.chunkRoles)
				}
			}
		})
	}
}

func TestNewNode(t *testing.T) {
	tests := []struct {
		name   string
		nodeID uint32
	}{
		{name: "node_0", nodeID: 0},
		{name: "node_1", nodeID: 1},
		{name: "node_100", nodeID: 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := NewNode(tt.nodeID)

			assert.NotNil(t, node)
			assert.Equal(t, tt.nodeID, node.ID())
			assert.NotNil(t, node.chunks)
			assert.NotNil(t, node.chunkRoles)
			assert.Empty(t, node.chunks)
			assert.Empty(t, node.chunkRoles)
		})
	}
}

func TestNode_StoreChunk(t *testing.T) {
	node := NewNode(1)
	chunkID := ChunkID{FileID: 1, ChunkIndex: 0}

	tests := []struct {
		name string
		role Role
	}{
		{name: "primary_role", role: RolePrimary},
		{name: "replica_role", role: RoleReplica},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node.StoreChunk(chunkID, tt.role)

			role, exists := node.chunkRoles[chunkID]
			assert.True(t, exists)
			assert.Equal(t, tt.role, role)
		})
	}
}

func TestNode_RetrieveChunk(t *testing.T) {
	node := NewNode(1)
	chunkID := ChunkID{FileID: 1, ChunkIndex: 0}

	// Test retrieving non-existent chunk
	chunk := node.RetrieveChunk(chunkID)
	assert.Nil(t, chunk)

	// Add a chunk and test retrieval
	expectedChunk := &Chunk{
		ID:       chunkID,
		Data:     []byte("test data"),
		Checksum: 12345,
		Version:  1,
	}

	node.mu.Lock()
	node.chunks[chunkID] = expectedChunk
	node.mu.Unlock()

	retrievedChunk := node.RetrieveChunk(chunkID)
	assert.NotNil(t, retrievedChunk)
	assert.Equal(t, expectedChunk, retrievedChunk)
}

func TestNode_RetrieveChunk_Concurrent(t *testing.T) {
	node := NewNode(1)
	chunkID := ChunkID{FileID: 1, ChunkIndex: 0}

	expectedChunk := &Chunk{
		ID:       chunkID,
		Data:     []byte("concurrent test data"),
		Checksum: 54321,
		Version:  1,
	}

	// Add chunk
	node.mu.Lock()
	node.chunks[chunkID] = expectedChunk
	node.mu.Unlock()

	// Test concurrent reads
	var wg sync.WaitGroup
	results := make([]*Chunk, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index] = node.RetrieveChunk(chunkID)
		}(i)
	}

	wg.Wait()

	// All results should be identical
	for _, result := range results {
		assert.Equal(t, expectedChunk, result)
	}
}

func TestNewVectorClock(t *testing.T) {
	vc := NewVectorClock()

	assert.NotNil(t, vc)
	assert.NotNil(t, vc.timestamps)
	assert.Empty(t, vc.timestamps)
}

func TestVectorClock_Update(t *testing.T) {
	vc := NewVectorClock()

	tests := []struct {
		name      string
		nodeID    uint32
		timestamp uint64
		expected  uint64
	}{
		{name: "first_update", nodeID: 1, timestamp: 10, expected: 10},
		{name: "higher_timestamp", nodeID: 1, timestamp: 20, expected: 20},
		{name: "lower_timestamp", nodeID: 1, timestamp: 15, expected: 20}, // Should not update
		{name: "different_node", nodeID: 2, timestamp: 5, expected: 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vc.Update(tt.nodeID, tt.timestamp)

			actual, exists := vc.timestamps[tt.nodeID]
			assert.True(t, exists)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestVectorClock_Update_Concurrent(t *testing.T) {
	vc := NewVectorClock()
	nodeID := uint32(1)

	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(timestamp uint64) {
			defer wg.Done()
			vc.Update(nodeID, timestamp)
		}(uint64(i))
	}

	wg.Wait()

	// The final timestamp should be the highest one
	final, exists := vc.timestamps[nodeID]
	assert.True(t, exists)
	assert.Equal(t, uint64(numGoroutines-1), final)
}

func TestVectorClock_Compare(t *testing.T) {
	tests := []struct {
		name     string
		vc1      map[uint32]uint64
		vc2      map[uint32]uint64
		expected int
	}{
		{
			name:     "equal_clocks",
			vc1:      map[uint32]uint64{1: 5, 2: 3},
			vc2:      map[uint32]uint64{1: 5, 2: 3},
			expected: 0,
		},
		{
			name:     "vc1_before_vc2",
			vc1:      map[uint32]uint64{1: 3, 2: 2},
			vc2:      map[uint32]uint64{1: 5, 2: 3},
			expected: -1,
		},
		{
			name:     "vc1_after_vc2",
			vc1:      map[uint32]uint64{1: 7, 2: 4},
			vc2:      map[uint32]uint64{1: 5, 2: 3},
			expected: 1,
		},
		{
			name:     "concurrent_clocks",
			vc1:      map[uint32]uint64{1: 7, 2: 2},
			vc2:      map[uint32]uint64{1: 5, 2: 4},
			expected: 0, // concurrent
		},
		{
			name:     "missing_nodes",
			vc1:      map[uint32]uint64{1: 5},
			vc2:      map[uint32]uint64{1: 5, 2: 3},
			expected: -1, // vc1 is before vc2
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
	vc1 := &VectorClock{timestamps: map[uint32]uint64{1: 5, 2: 3}}
	vc2 := &VectorClock{timestamps: map[uint32]uint64{1: 5, 2: 3}}

	var wg sync.WaitGroup
	results := make([]int, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index] = vc1.Compare(vc2)
		}(i)
	}

	wg.Wait()

	// All results should be 0 (equal)
	for _, result := range results {
		assert.Equal(t, 0, result)
	}
}

func TestChunkID_Equality(t *testing.T) {
	chunk1 := ChunkID{FileID: 1, ChunkIndex: 0}
	chunk2 := ChunkID{FileID: 1, ChunkIndex: 0}
	chunk3 := ChunkID{FileID: 2, ChunkIndex: 0}

	assert.Equal(t, chunk1, chunk2)
	assert.NotEqual(t, chunk1, chunk3)
}

func TestChunk_Creation(t *testing.T) {
	chunkID := ChunkID{FileID: 1, ChunkIndex: 0}
	data := []byte("test chunk data")

	chunk := &Chunk{
		ID:       chunkID,
		Data:     data,
		Checksum: 12345,
		Version:  1,
	}

	assert.Equal(t, chunkID, chunk.ID)
	assert.Equal(t, data, chunk.Data)
	assert.Equal(t, uint64(12345), chunk.Checksum)
	assert.Equal(t, uint64(1), chunk.Version)
}

func TestRole_Constants(t *testing.T) {
	assert.Equal(t, Role(0), RolePrimary)
	assert.Equal(t, Role(1), RoleReplica)
}

func TestRole_String(t *testing.T) {
	tests := []struct {
		role     Role
		expected string
	}{
		{RolePrimary, "primary"},
		{RoleReplica, "replica"},
		{Role(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.role.String())
		})
	}
}

func BenchmarkNode_StoreChunk(b *testing.B) {
	node := NewNode(1)
	chunkID := ChunkID{FileID: 1, ChunkIndex: 0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		node.StoreChunk(chunkID, RolePrimary)
	}
}

func BenchmarkNode_RetrieveChunk(b *testing.B) {
	node := NewNode(1)
	chunkID := ChunkID{FileID: 1, ChunkIndex: 0}

	// Pre-populate with a chunk
	chunk := &Chunk{
		ID:       chunkID,
		Data:     []byte("benchmark data"),
		Checksum: 12345,
		Version:  1,
	}
	node.mu.Lock()
	node.chunks[chunkID] = chunk
	node.mu.Unlock()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		node.RetrieveChunk(chunkID)
	}
}

func BenchmarkVectorClock_Update(b *testing.B) {
	vc := NewVectorClock()
	nodeID := uint32(1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vc.Update(nodeID, uint64(i))
	}
}

func BenchmarkVectorClock_Compare(b *testing.B) {
	vc1 := &VectorClock{timestamps: map[uint32]uint64{1: 5, 2: 3, 3: 7}}
	vc2 := &VectorClock{timestamps: map[uint32]uint64{1: 4, 2: 6, 3: 2}}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vc1.Compare(vc2)
	}
}
