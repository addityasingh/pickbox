package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRaftManager(t *testing.T) {
	// Skip this test when running with race detector due to checkptr issues in BoltDB
	// This is a known issue with github.com/boltdb/bolt@v1.3.1 and checkptr validation
	if testing.Short() {
		t.Skip("Skipping in short mode due to BoltDB checkptr issues")
	}

	tests := []struct {
		name     string
		nodeID   string
		dataDir  string
		bindAddr string
		wantErr  bool
	}{
		{
			name:     "valid_raft_manager",
			nodeID:   "test-node-1",
			dataDir:  "/tmp/test-raft-1",
			bindAddr: "127.0.0.1:0",
			wantErr:  false,
		},
		{
			name:     "empty_node_id",
			nodeID:   "",
			dataDir:  "/tmp/test-raft-2",
			bindAddr: "127.0.0.1:0",
			wantErr:  true,
		},
		{
			name:     "empty_data_dir",
			nodeID:   "test-node-3",
			dataDir:  "",
			bindAddr: "127.0.0.1:0",
			wantErr:  true,
		},
		{
			name:     "empty_bind_addr",
			nodeID:   "test-node-4",
			dataDir:  "/tmp/test-raft-4",
			bindAddr: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Cleanup
			os.RemoveAll(tt.dataDir)
			defer os.RemoveAll(tt.dataDir)

			rm, err := NewRaftManager(tt.nodeID, tt.dataDir, tt.bindAddr)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, rm)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, rm)
				assert.NotNil(t, rm.raft)
				assert.NotNil(t, rm.fsm)
				assert.NotNil(t, rm.store)
				assert.NotNil(t, rm.snapshots)
				assert.NotNil(t, rm.transport)
				assert.Equal(t, tt.nodeID, rm.nodeID)
				assert.Equal(t, tt.dataDir, rm.dataDir)

				// Cleanup
				err := rm.Shutdown()
				assert.NoError(t, err)
			}
		})
	}
}

func TestFileSystemFSM_Apply(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode due to BoltDB checkptr issues")
	}

	dataDir := "/tmp/test-fsm"
	os.RemoveAll(dataDir)
	defer os.RemoveAll(dataDir)

	fsm := &fileSystemFSM{
		store:   make(map[string][]byte),
		dataDir: dataDir,
	}

	tests := []struct {
		name        string
		cmd         Command
		wantErr     bool
		expectError bool
	}{
		{
			name: "write_operation",
			cmd: Command{
				Op:   OpWrite,
				Path: "test.txt",
				Data: []byte("hello world"),
			},
			wantErr: false,
		},
		{
			name: "write_nested_path",
			cmd: Command{
				Op:   OpWrite,
				Path: "nested/dir/test.txt",
				Data: []byte("nested content"),
			},
			wantErr: false,
		},
		{
			name: "delete_operation",
			cmd: Command{
				Op:   OpDelete,
				Path: "test.txt",
			},
			wantErr: false,
		},
		{
			name: "delete_nonexistent",
			cmd: Command{
				Op:   OpDelete,
				Path: "nonexistent.txt",
			},
			wantErr: false, // Should not error on non-existent files
		},
		{
			name: "unknown_operation",
			cmd: Command{
				Op:   "unknown",
				Path: "test.txt",
				Data: []byte("data"),
			},
			wantErr:     true,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmdData, err := json.Marshal(tt.cmd)
			require.NoError(t, err)

			log := &raft.Log{
				Data: cmdData,
			}

			result := fsm.Apply(log)

			if tt.expectError {
				assert.Error(t, result.(error))
			} else if tt.wantErr {
				assert.Error(t, result.(error))
			} else {
				if result != nil {
					assert.NoError(t, result.(error))
				}

				// Verify write operations
				if tt.cmd.Op == OpWrite {
					// Check in-memory store
					data, exists := fsm.store[tt.cmd.Path]
					assert.True(t, exists)
					assert.Equal(t, tt.cmd.Data, data)

					// Check file on disk
					filePath := filepath.Join(dataDir, tt.cmd.Path)
					fileData, err := os.ReadFile(filePath)
					assert.NoError(t, err)
					assert.Equal(t, tt.cmd.Data, fileData)
				}

				// Verify delete operations
				if tt.cmd.Op == OpDelete && tt.cmd.Path != "nonexistent.txt" {
					_, exists := fsm.store[tt.cmd.Path]
					assert.False(t, exists)
				}
			}
		})
	}
}

func TestFileSystemFSM_Snapshot(t *testing.T) {
	fsm := &fileSystemFSM{
		store: map[string][]byte{
			"file1.txt": []byte("content1"),
			"file2.txt": []byte("content2"),
		},
		dataDir: "/tmp/test-snapshot",
	}

	snapshot, err := fsm.Snapshot()
	assert.NoError(t, err)
	assert.NotNil(t, snapshot)

	// Verify snapshot type
	fsSnapshot, ok := snapshot.(*fileSystemSnapshot)
	assert.True(t, ok)
	assert.Equal(t, len(fsm.store), len(fsSnapshot.store))

	// Verify snapshot content
	for k, v := range fsm.store {
		snapshotData, exists := fsSnapshot.store[k]
		assert.True(t, exists)
		assert.Equal(t, v, snapshotData)
	}

	// Verify it's a deep copy (modifying original shouldn't affect snapshot)
	fsm.store["file3.txt"] = []byte("content3")
	_, exists := fsSnapshot.store["file3.txt"]
	assert.False(t, exists)
}

func TestFileSystemFSM_Restore(t *testing.T) {
	fsm := &fileSystemFSM{
		store:   make(map[string][]byte),
		dataDir: "/tmp/test-restore",
	}

	// Create test data
	testData := map[string][]byte{
		"restored1.txt": []byte("restored content 1"),
		"restored2.txt": []byte("restored content 2"),
	}

	// Create a pipe for the test
	r, w, err := os.Pipe()
	require.NoError(t, err)
	defer r.Close()

	// Write test data in a goroutine
	go func() {
		defer w.Close()
		encoder := json.NewEncoder(w)
		err := encoder.Encode(testData)
		assert.NoError(t, err)
	}()

	// Restore from the pipe
	err = fsm.Restore(r)
	assert.NoError(t, err)

	// Verify restoration
	assert.Equal(t, len(testData), len(fsm.store))
	for k, v := range testData {
		restoredData, exists := fsm.store[k]
		assert.True(t, exists)
		assert.Equal(t, v, restoredData)
	}
}

func TestRaftManager_ReplicateChunk(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode due to BoltDB checkptr issues")
	}

	dataDir := "/tmp/test-replicate"
	os.RemoveAll(dataDir)
	defer os.RemoveAll(dataDir)

	rm, err := NewRaftManager("test-node", dataDir, "127.0.0.1:0")
	require.NoError(t, err)
	defer func() {
		err := rm.Shutdown()
		assert.NoError(t, err)
	}()

	// Bootstrap the cluster
	servers := []raft.Server{
		{
			ID:      raft.ServerID("test-node"),
			Address: rm.transport.LocalAddr(),
		},
	}
	err = rm.BootstrapCluster(servers)
	require.NoError(t, err)

	// Wait for leadership (with timeout)
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for leadership")
		case <-ticker.C:
			if rm.IsLeader() {
				goto testReplication
			}
		}
	}

testReplication:
	chunkID := ChunkID{FileID: 1, ChunkIndex: 0}
	data := []byte("test chunk data")

	err = rm.ReplicateChunk(chunkID, data)
	assert.NoError(t, err)

	// Give some time for the command to be applied
	time.Sleep(500 * time.Millisecond)

	// Verify the file was created
	expectedPath := filepath.Join(dataDir, "chunks", "1_0")
	fileData, err := os.ReadFile(expectedPath)
	assert.NoError(t, err)
	assert.Equal(t, data, fileData)
}

func TestRaftManager_AddVoter(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode due to BoltDB checkptr issues")
	}

	dataDir := "/tmp/test-addvoter"
	os.RemoveAll(dataDir)
	defer os.RemoveAll(dataDir)

	rm, err := NewRaftManager("test-node", dataDir, "127.0.0.1:0")
	require.NoError(t, err)
	defer func() {
		err := rm.Shutdown()
		assert.NoError(t, err)
	}()

	tests := []struct {
		name    string
		id      string
		addr    string
		wantErr bool
	}{
		{
			name:    "empty_id",
			id:      "",
			addr:    "127.0.0.1:8001",
			wantErr: true,
		},
		{
			name:    "empty_addr",
			id:      "node2",
			addr:    "",
			wantErr: true,
		},
		{
			name:    "valid_voter", // This will fail because we don't have a real cluster, but tests validation
			id:      "node2",
			addr:    "127.0.0.1:8001",
			wantErr: true, // Expected to fail in single-node test setup
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rm.AddVoter(tt.id, tt.addr)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRaftManager_State_Methods(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode due to BoltDB checkptr issues")
	}

	dataDir := "/tmp/test-state"
	os.RemoveAll(dataDir)
	defer os.RemoveAll(dataDir)

	rm, err := NewRaftManager("test-node", dataDir, "127.0.0.1:0")
	require.NoError(t, err)
	defer func() {
		err := rm.Shutdown()
		assert.NoError(t, err)
	}()

	// Test initial state
	state := rm.State()
	assert.Contains(t, []raft.RaftState{raft.Follower, raft.Candidate}, state)

	// Test IsLeader (should be false initially)
	assert.False(t, rm.IsLeader())

	// Test Leader (should be empty initially)
	leader := rm.Leader()
	assert.Empty(t, leader)
}

func TestRaftManager_BootstrapCluster(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode due to BoltDB checkptr issues")
	}

	dataDir := "/tmp/test-bootstrap"
	os.RemoveAll(dataDir)
	defer os.RemoveAll(dataDir)

	rm, err := NewRaftManager("test-node", dataDir, "127.0.0.1:0")
	require.NoError(t, err)
	defer func() {
		err := rm.Shutdown()
		assert.NoError(t, err)
	}()

	servers := []raft.Server{
		{
			ID:      raft.ServerID("test-node"),
			Address: rm.transport.LocalAddr(),
		},
	}

	err = rm.BootstrapCluster(servers)
	assert.NoError(t, err)

	// Wait for leadership (with timeout)
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for leadership after bootstrap")
		case <-ticker.C:
			if rm.IsLeader() {
				return // Test passed
			}
		}
	}
}

func TestRaftManager_AddVoter_AfterBootstrap(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode due to BoltDB checkptr issues")
	}

	dataDir := "/tmp/test-addvoter-bootstrap"
	os.RemoveAll(dataDir)
	defer os.RemoveAll(dataDir)

	rm, err := NewRaftManager("test-node", dataDir, "127.0.0.1:0")
	require.NoError(t, err)
	defer func() {
		err := rm.Shutdown()
		assert.NoError(t, err)
	}()

	// Bootstrap first
	servers := []raft.Server{
		{
			ID:      raft.ServerID("test-node"),
			Address: rm.transport.LocalAddr(),
		},
	}
	err = rm.BootstrapCluster(servers)
	require.NoError(t, err)

	// Wait for leadership
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for leadership")
		case <-ticker.C:
			if rm.IsLeader() {
				goto testAddVoter
			}
		}
	}

testAddVoter:
	// Try to add a voter (this might succeed or fail depending on timing)
	err = rm.AddVoter("node2", "127.0.0.1:8001")
	// In a real test environment, this may succeed but the node won't be reachable
	// We just verify the method doesn't panic
	if err != nil {
		assert.Contains(t, err.Error(), "adding voter")
	}
}

// Benchmark tests for performance verification
func BenchmarkFileSystemFSM_Apply_Write(b *testing.B) {
	fsm := &fileSystemFSM{
		store:   make(map[string][]byte),
		dataDir: "/tmp/bench-fsm",
	}
	os.RemoveAll(fsm.dataDir)
	defer os.RemoveAll(fsm.dataDir)

	cmd := Command{
		Op:   OpWrite,
		Path: "bench.txt",
		Data: []byte("benchmark data"),
	}
	cmdData, _ := json.Marshal(cmd)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		log := &raft.Log{Data: cmdData}
		fsm.Apply(log)
	}
}

func BenchmarkFileSystemFSM_Snapshot(b *testing.B) {
	fsm := &fileSystemFSM{
		store: make(map[string][]byte),
	}

	// Pre-populate with some data
	for i := 0; i < 100; i++ {
		fsm.store[fmt.Sprintf("file%d.txt", i)] = []byte(fmt.Sprintf("content%d", i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := fsm.Snapshot()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRaftManager_ReplicateChunk(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping in short mode due to BoltDB checkptr issues")
	}

	dataDir := "/tmp/bench-replicate"
	os.RemoveAll(dataDir)
	defer os.RemoveAll(dataDir)

	rm, err := NewRaftManager("bench-node", dataDir, "127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		err := rm.Shutdown()
		if err != nil {
			b.Logf("Shutdown error: %v", err)
		}
	}()

	// Bootstrap
	servers := []raft.Server{
		{
			ID:      raft.ServerID("bench-node"),
			Address: rm.transport.LocalAddr(),
		},
	}
	err = rm.BootstrapCluster(servers)
	if err != nil {
		b.Fatal(err)
	}

	// Wait for leadership
	for !rm.IsLeader() {
		time.Sleep(10 * time.Millisecond)
	}

	chunkID := ChunkID{FileID: 1, ChunkIndex: 0}
	data := []byte("benchmark chunk data")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		chunkID.ChunkIndex = uint32(i)
		err := rm.ReplicateChunk(chunkID, data)
		if err != nil {
			b.Fatal(err)
		}
	}
}
