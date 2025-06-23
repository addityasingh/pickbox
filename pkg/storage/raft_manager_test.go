package storage

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
			dataDir:  "/tmp/test-raft-empty",
			bindAddr: "127.0.0.1:0",
			wantErr:  true,
		},
		{
			name:     "invalid_bind_addr",
			nodeID:   "test-node-invalid",
			dataDir:  "/tmp/test-raft-invalid",
			bindAddr: "invalid-address",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Cleanup before test
			os.RemoveAll(tt.dataDir)
			defer os.RemoveAll(tt.dataDir)

			manager, err := NewRaftManager(tt.nodeID, tt.dataDir, tt.bindAddr)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, manager)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, manager)
				assert.NotNil(t, manager.raft)
				assert.NotNil(t, manager.fsm)
				assert.NotNil(t, manager.store)
				assert.NotNil(t, manager.snapshots)
				assert.NotNil(t, manager.transport)
				assert.Equal(t, tt.nodeID, manager.nodeID)
				assert.Equal(t, tt.dataDir, manager.dataDir)

				// Verify data directory was created
				_, err := os.Stat(tt.dataDir)
				assert.NoError(t, err)

				// Verify raft.db file is created
				dbPath := filepath.Join(tt.dataDir, "raft.db")
				_, err = os.Stat(dbPath)
				assert.NoError(t, err)

				// Clean up transport
				manager.transport.Close()
			}
		})
	}
}

func TestFileSystemFSM_Apply(t *testing.T) {
	tempDir := "/tmp/test-fsm-apply"
	os.RemoveAll(tempDir)
	defer os.RemoveAll(tempDir)

	fsm := &FileSystemFSM{
		store:   make(map[string][]byte),
		dataDir: tempDir,
	}

	tests := []struct {
		name    string
		command Command
		wantErr bool
	}{
		{
			name: "write_command",
			command: Command{
				Op:    "write",
				Path:  "test/file.txt",
				Data:  []byte("test content"),
				Chunk: ChunkID{FileID: 1, ChunkIndex: 0},
			},
			wantErr: false,
		},
		{
			name: "write_root_file",
			command: Command{
				Op:    "write",
				Path:  "root.txt",
				Data:  []byte("root content"),
				Chunk: ChunkID{FileID: 2, ChunkIndex: 0},
			},
			wantErr: false,
		},
		{
			name: "delete_command",
			command: Command{
				Op:    "delete",
				Path:  "test/file.txt",
				Chunk: ChunkID{FileID: 1, ChunkIndex: 0},
			},
			wantErr: false,
		},
		{
			name: "unknown_operation",
			command: Command{
				Op:   "unknown",
				Path: "test.txt",
				Data: []byte("test"),
			},
			wantErr: false, // Should not error, just ignore
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.command)
			require.NoError(t, err)

			log := &raft.Log{
				Data: data,
			}

			result := fsm.Apply(log)

			if tt.wantErr {
				assert.NotNil(t, result)
			} else {
				if tt.command.Op == "write" {
					// Verify file was written
					filePath := filepath.Join(tempDir, tt.command.Path)
					content, err := os.ReadFile(filePath)
					assert.NoError(t, err)
					assert.Equal(t, tt.command.Data, content)

					// Verify in-memory store
					fsm.mu.RLock()
					storedData := fsm.store[tt.command.Path]
					fsm.mu.RUnlock()
					assert.Equal(t, tt.command.Data, storedData)
				} else if tt.command.Op == "delete" {
					// Verify file was deleted
					filePath := filepath.Join(tempDir, tt.command.Path)
					_, err := os.Stat(filePath)
					assert.True(t, os.IsNotExist(err))

					// Verify removed from in-memory store
					fsm.mu.RLock()
					_, exists := fsm.store[tt.command.Path]
					fsm.mu.RUnlock()
					assert.False(t, exists)
				}
			}
		})
	}
}

func TestFileSystemFSM_Apply_InvalidJSON(t *testing.T) {
	tempDir := "/tmp/test-fsm-invalid"
	os.RemoveAll(tempDir)
	defer os.RemoveAll(tempDir)

	fsm := &FileSystemFSM{
		store:   make(map[string][]byte),
		dataDir: tempDir,
	}

	log := &raft.Log{
		Data: []byte("invalid json"),
	}

	result := fsm.Apply(log)
	assert.NotNil(t, result)
	assert.Contains(t, result.(error).Error(), "failed to unmarshal command")
}

func TestFileSystemFSM_Snapshot(t *testing.T) {
	fsm := &FileSystemFSM{
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
	fsSnapshot, ok := snapshot.(*FileSystemSnapshot)
	assert.True(t, ok)
	assert.NotNil(t, fsSnapshot.store)
	assert.Len(t, fsSnapshot.store, 2)
	assert.Equal(t, []byte("content1"), fsSnapshot.store["file1.txt"])
	assert.Equal(t, []byte("content2"), fsSnapshot.store["file2.txt"])
}

func TestFileSystemFSM_Restore(t *testing.T) {
	fsm := &FileSystemFSM{
		store:   make(map[string][]byte),
		dataDir: "/tmp/test-restore",
	}

	// Create test data to restore
	testData := map[string][]byte{
		"restored1.txt": []byte("restored content 1"),
		"restored2.txt": []byte("restored content 2"),
	}

	// Create a reader with JSON data
	data, err := json.Marshal(testData)
	require.NoError(t, err)
	reader := strings.NewReader(string(data))

	// Test restore
	err = fsm.Restore(io.NopCloser(reader))
	assert.NoError(t, err)

	// Verify data was restored
	fsm.mu.RLock()
	assert.Equal(t, testData, fsm.store)
	fsm.mu.RUnlock()
}

func TestFileSystemFSM_Restore_InvalidJSON(t *testing.T) {
	fsm := &FileSystemFSM{
		store:   make(map[string][]byte),
		dataDir: "/tmp/test-restore-invalid",
	}

	reader := strings.NewReader("invalid json")
	err := fsm.Restore(io.NopCloser(reader))
	assert.Error(t, err)
}

func TestFileSystemSnapshot_Persist(t *testing.T) {
	snapshot := &FileSystemSnapshot{
		store: map[string][]byte{
			"persist1.txt": []byte("persist content 1"),
			"persist2.txt": []byte("persist content 2"),
		},
	}

	// Create a mock sink
	var buf strings.Builder
	mockSink := &mockSnapshotSink{writer: &buf}

	err := snapshot.Persist(mockSink)
	assert.NoError(t, err)

	// Verify JSON was written
	var result map[string][]byte
	err = json.Unmarshal([]byte(buf.String()), &result)
	assert.NoError(t, err)
	assert.Equal(t, snapshot.store, result)
}

func TestFileSystemSnapshot_Release(t *testing.T) {
	snapshot := &FileSystemSnapshot{
		store: map[string][]byte{},
	}

	// Should not panic
	assert.NotPanics(t, func() {
		snapshot.Release()
	})
}

func TestRaftManager_ReplicateChunk(t *testing.T) {
	// Skip this test when running with race detector due to checkptr issues in BoltDB
	// This is a known issue with github.com/boltdb/bolt@v1.3.1 and checkptr validation
	if testing.Short() {
		t.Skip("Skipping in short mode due to BoltDB checkptr issues")
	}

	tempDir := "/tmp/test-replicate-chunk"
	os.RemoveAll(tempDir)
	defer os.RemoveAll(tempDir)

	// Create a raft manager but don't start cluster (we're testing the method logic)
	manager, err := NewRaftManager("test-node", tempDir, "127.0.0.1:0")
	require.NoError(t, err)
	if manager != nil && manager.transport != nil {
		defer manager.transport.Close()
	}

	tests := []struct {
		name    string
		chunkID ChunkID
		data    []byte
		wantErr bool
	}{
		{
			name:    "valid_chunk",
			chunkID: ChunkID{FileID: 1, ChunkIndex: 0},
			data:    []byte("test chunk data"),
			wantErr: true, // Will fail because no cluster is formed
		},
		{
			name:    "empty_data",
			chunkID: ChunkID{FileID: 2, ChunkIndex: 0},
			data:    []byte{},
			wantErr: true, // Will fail because no cluster is formed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.ReplicateChunk(tt.chunkID, tt.data)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCommand_JSONMarshaling(t *testing.T) {
	cmd := Command{
		Op:    "write",
		Path:  "test/file.txt",
		Data:  []byte("test data"),
		Chunk: ChunkID{FileID: 1, ChunkIndex: 0},
	}

	// Test marshaling
	data, err := json.Marshal(cmd)
	assert.NoError(t, err)
	assert.NotEmpty(t, data)

	// Test unmarshaling
	var unmarshaled Command
	err = json.Unmarshal(data, &unmarshaled)
	assert.NoError(t, err)
	assert.Equal(t, cmd.Op, unmarshaled.Op)
	assert.Equal(t, cmd.Path, unmarshaled.Path)
	assert.Equal(t, cmd.Data, unmarshaled.Data)
	assert.Equal(t, cmd.Chunk, unmarshaled.Chunk)
}

// Mock implementation for testing
type mockSnapshotSink struct {
	writer io.Writer
	closed bool
}

func (m *mockSnapshotSink) Write(p []byte) (n int, err error) {
	return m.writer.Write(p)
}

func (m *mockSnapshotSink) Close() error {
	m.closed = true
	return nil
}

func (m *mockSnapshotSink) ID() string {
	return "mock-snapshot"
}

func (m *mockSnapshotSink) Cancel() error {
	return nil
}

// Benchmark tests
func BenchmarkFileSystemFSM_Apply_Write(b *testing.B) {
	tempDir := "/tmp/bench-fsm-write"
	os.RemoveAll(tempDir)
	defer os.RemoveAll(tempDir)

	fsm := &FileSystemFSM{
		store:   make(map[string][]byte),
		dataDir: tempDir,
	}

	cmd := Command{
		Op:    "write",
		Path:  "bench/file.txt",
		Data:  []byte("benchmark data"),
		Chunk: ChunkID{FileID: 1, ChunkIndex: 0},
	}

	data, _ := json.Marshal(cmd)
	log := &raft.Log{Data: data}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fsm.Apply(log)
	}
}

func BenchmarkFileSystemFSM_Snapshot(b *testing.B) {
	fsm := &FileSystemFSM{
		store: map[string][]byte{
			"file1.txt": []byte("content1"),
			"file2.txt": []byte("content2"),
			"file3.txt": []byte("content3"),
		},
		dataDir: "/tmp/bench-snapshot",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fsm.Snapshot()
	}
}

func BenchmarkCommand_JSONMarshal(b *testing.B) {
	cmd := Command{
		Op:    "write",
		Path:  "benchmark/file.txt",
		Data:  []byte("benchmark data for marshaling"),
		Chunk: ChunkID{FileID: 1, ChunkIndex: 0},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		json.Marshal(cmd)
	}
}

func BenchmarkCommand_JSONUnmarshal(b *testing.B) {
	cmd := Command{
		Op:    "write",
		Path:  "benchmark/file.txt",
		Data:  []byte("benchmark data for unmarshaling"),
		Chunk: ChunkID{FileID: 1, ChunkIndex: 0},
	}

	data, _ := json.Marshal(cmd)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var unmarshaled Command
		json.Unmarshal(data, &unmarshaled)
	}
}

func TestRaftManager_BootstrapCluster(t *testing.T) {
	// Skip this test when running with race detector due to checkptr issues in BoltDB
	if testing.Short() {
		t.Skip("Skipping in short mode due to BoltDB checkptr issues")
	}

	tempDir := "/tmp/test-raft-bootstrap"
	os.RemoveAll(tempDir)
	defer os.RemoveAll(tempDir)

	manager, err := NewRaftManager("bootstrap-node", tempDir, "127.0.0.1:0")
	require.NoError(t, err)
	defer manager.transport.Close()

	// Test bootstrapping a single-node cluster
	servers := []raft.Server{
		{
			Suffrage: raft.Voter,
			ID:       raft.ServerID("bootstrap-node"),
			Address:  raft.ServerAddress(manager.transport.LocalAddr()),
		},
	}

	err = manager.BootstrapCluster(servers)
	assert.NoError(t, err)

	// Verify the node becomes a leader (single node cluster)
	// Note: This might take a moment in a real cluster, but for testing we just verify no error
}

func TestRaftManager_AddVoter(t *testing.T) {
	// Skip this test when running with race detector due to checkptr issues in BoltDB
	if testing.Short() {
		t.Skip("Skipping in short mode due to BoltDB checkptr issues")
	}

	tempDir := "/tmp/test-raft-add-voter"
	os.RemoveAll(tempDir)
	defer os.RemoveAll(tempDir)

	manager, err := NewRaftManager("voter-test-node", tempDir, "127.0.0.1:0")
	require.NoError(t, err)
	defer manager.transport.Close()

	// Note: AddVoter will fail without a properly bootstrapped cluster
	// This test verifies the method doesn't panic and returns an error appropriately
	err = manager.AddVoter("new-node", "127.0.0.1:9999")
	assert.Error(t, err) // Expected to fail as cluster isn't bootstrapped
}
