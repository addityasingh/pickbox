package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/assert"
)

func TestRaftReplication(t *testing.T) {
	// Create test directories for each node
	baseDir := "testdata"
	nodeDirs := []string{
		filepath.Join(baseDir, "node1"),
		filepath.Join(baseDir, "node2"),
		filepath.Join(baseDir, "node3"),
	}

	// Clean up any existing test data
	os.RemoveAll(baseDir)
	for _, dir := range nodeDirs {
		assert.NoError(t, os.MkdirAll(dir, 0755))
	}

	// Create Raft managers for each node
	managers := make([]*RaftManager, 3)
	ports := []string{":8001", ":8002", ":8003"}

	for i := 0; i < 3; i++ {
		var err error
		managers[i], err = NewRaftManager(
			fmt.Sprintf("node%d", i+1),
			nodeDirs[i],
			fmt.Sprintf("127.0.0.1%s", ports[i]),
		)
		assert.NoError(t, err)
	}

	// Bootstrap the cluster with the first node
	servers := []raft.Server{
		{
			Suffrage: raft.Voter,
			ID:       raft.ServerID("node1"),
			Address:  raft.ServerAddress("127.0.0.1:8001"),
		},
	}
	assert.NoError(t, managers[0].BootstrapCluster(servers))

	// Add other nodes to the cluster
	for i := 1; i < 3; i++ {
		assert.NoError(t, managers[0].AddVoter(
			fmt.Sprintf("node%d", i+1),
			fmt.Sprintf("127.0.0.1%s", ports[i]),
		))
	}

	// Wait for cluster to stabilize
	time.Sleep(2 * time.Second)

	// Test replication by writing a file through the leader
	testData := []byte("Hello, Raft!")
	chunkID := ChunkID{FileID: 1, ChunkIndex: 1}

	// Find the leader
	var leader *RaftManager
	for _, m := range managers {
		if m.raft.State() == raft.Leader {
			leader = m
			break
		}
	}
	assert.NotNil(t, leader, "No leader found in the cluster")

	// Write data through the leader
	err := leader.ReplicateChunk(chunkID, testData)
	assert.NoError(t, err)

	// Wait for replication
	time.Sleep(1 * time.Second)

	// Verify data is replicated to all nodes
	for i, dir := range nodeDirs {
		expectedPath := filepath.Join(dir, "chunks", "1_1")
		data, err := os.ReadFile(expectedPath)
		assert.NoError(t, err, "Failed to read file from node %d", i+1)
		assert.Equal(t, testData, data, "Data mismatch in node %d", i+1)
	}

	// Clean up
	for _, m := range managers {
		if m.raft != nil {
			m.raft.Shutdown().Error()
		}
	}
	os.RemoveAll(baseDir)
}
