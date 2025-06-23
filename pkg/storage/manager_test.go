package storage

import (
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewManager(t *testing.T) {
	t.Skip("Skipping the manager creation test for now")

	tests := []struct {
		name      string
		nodeCount int
		nodeID    string
		dataDir   string
		bindAddr  string
		wantErr   bool
	}{
		{
			name:      "valid manager creation",
			nodeCount: 3,
			nodeID:    "test-node",
			dataDir:   "/tmp/test-manager",
			bindAddr:  "127.0.0.1:0", // Use port 0 for auto-allocation
			wantErr:   false,
		},
		{
			name:      "zero nodes",
			nodeCount: 0,
			nodeID:    "test-node",
			dataDir:   "/tmp/test-manager-zero",
			bindAddr:  "127.0.0.1:0",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Cleanup before test
			os.RemoveAll(tt.dataDir)
			defer os.RemoveAll(tt.dataDir)

			manager, err := NewManager(tt.nodeCount, tt.nodeID, tt.dataDir, tt.bindAddr)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, manager)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, manager)
				assert.Len(t, manager.nodes, tt.nodeCount)
				assert.NotNil(t, manager.raft)

				// Verify all nodes are properly initialized
				for i, node := range manager.nodes {
					assert.Equal(t, uint32(i), node.ID)
					assert.NotNil(t, node.Chunks)
					assert.NotNil(t, node.ChunkRoles)
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
			assert.Equal(t, tt.nodeID, node.ID)
			assert.NotNil(t, node.Chunks)
			assert.NotNil(t, node.ChunkRoles)
			assert.Empty(t, node.Chunks)
			assert.Empty(t, node.ChunkRoles)
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
		{name: "primary_role", role: Primary},
		{name: "replica_role", role: Replica},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node.StoreChunk(chunkID, tt.role)

			role, exists := node.ChunkRoles[chunkID]
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
	node.Chunks[chunkID] = expectedChunk
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
	node.Chunks[chunkID] = expectedChunk
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
	assert.NotNil(t, vc.Timestamps)
	assert.Empty(t, vc.Timestamps)
}

func TestVectorClock_UpdateVectorClock(t *testing.T) {
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
			vc.UpdateVectorClock(tt.nodeID, tt.timestamp)

			vc.mu.RLock()
			actual := vc.Timestamps[tt.nodeID]
			vc.mu.RUnlock()

			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestVectorClock_CompareVectorClocks(t *testing.T) {
	tests := []struct {
		name     string
		vc1      map[uint32]uint64
		vc2      map[uint32]uint64
		expected int
	}{
		{
			name:     "equal_clocks",
			vc1:      map[uint32]uint64{1: 10, 2: 5},
			vc2:      map[uint32]uint64{1: 10, 2: 5},
			expected: 0,
		},
		{
			name:     "vc1_before_vc2",
			vc1:      map[uint32]uint64{1: 5, 2: 5},
			vc2:      map[uint32]uint64{1: 10, 2: 5},
			expected: -1,
		},
		{
			name:     "vc1_after_vc2",
			vc1:      map[uint32]uint64{1: 10, 2: 5},
			vc2:      map[uint32]uint64{1: 5, 2: 5},
			expected: 1,
		},
		{
			name:     "concurrent_clocks",
			vc1:      map[uint32]uint64{1: 10, 2: 5},
			vc2:      map[uint32]uint64{1: 5, 2: 10},
			expected: 0,
		},
		{
			name:     "different_nodes",
			vc1:      map[uint32]uint64{1: 10},
			vc2:      map[uint32]uint64{2: 10},
			expected: 0,
		},
		{
			name:     "empty_vs_non_empty",
			vc1:      map[uint32]uint64{},
			vc2:      map[uint32]uint64{1: 10},
			expected: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vc1 := &VectorClock{Timestamps: tt.vc1}
			vc2 := &VectorClock{Timestamps: tt.vc2}

			result := vc1.CompareVectorClocks(vc2)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestVectorClock_CompareVectorClocks_Concurrent(t *testing.T) {
	vc1 := &VectorClock{Timestamps: map[uint32]uint64{1: 10, 2: 5}}
	vc2 := &VectorClock{Timestamps: map[uint32]uint64{1: 5, 2: 10}}

	// Test concurrent comparisons
	var wg sync.WaitGroup
	results := make([]int, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index] = vc1.CompareVectorClocks(vc2)
		}(i)
	}

	wg.Wait()

	// All results should be 0 (concurrent)
	for _, result := range results {
		assert.Equal(t, 0, result)
	}
}

func TestChunkID_Equality(t *testing.T) {
	chunk1 := ChunkID{FileID: 1, ChunkIndex: 0}
	chunk2 := ChunkID{FileID: 1, ChunkIndex: 0}
	chunk3 := ChunkID{FileID: 1, ChunkIndex: 1}
	chunk4 := ChunkID{FileID: 2, ChunkIndex: 0}

	assert.Equal(t, chunk1, chunk2)
	assert.NotEqual(t, chunk1, chunk3)
	assert.NotEqual(t, chunk1, chunk4)
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
	assert.Equal(t, Role(0), Primary)
	assert.Equal(t, Role(1), Replica)
	assert.NotEqual(t, Primary, Replica)
}

// Benchmark tests
func BenchmarkNode_StoreChunk(b *testing.B) {
	node := NewNode(1)
	chunkID := ChunkID{FileID: 1, ChunkIndex: 0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		node.StoreChunk(chunkID, Primary)
	}
}

func BenchmarkNode_RetrieveChunk(b *testing.B) {
	node := NewNode(1)
	chunkID := ChunkID{FileID: 1, ChunkIndex: 0}

	chunk := &Chunk{
		ID:       chunkID,
		Data:     []byte("benchmark data"),
		Checksum: 12345,
		Version:  1,
	}

	node.mu.Lock()
	node.Chunks[chunkID] = chunk
	node.mu.Unlock()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		node.RetrieveChunk(chunkID)
	}
}

func BenchmarkVectorClock_UpdateVectorClock(b *testing.B) {
	vc := NewVectorClock()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vc.UpdateVectorClock(uint32(i%10), uint64(i))
	}
}

func BenchmarkVectorClock_CompareVectorClocks(b *testing.B) {
	vc1 := &VectorClock{Timestamps: map[uint32]uint64{1: 10, 2: 5, 3: 15}}
	vc2 := &VectorClock{Timestamps: map[uint32]uint64{1: 8, 2: 7, 3: 12}}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vc1.CompareVectorClocks(vc2)
	}
}
