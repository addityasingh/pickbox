package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommand_JSONMarshaling(t *testing.T) {
	cmd := Command{
		Op:       "write",
		Path:     "test/file.txt",
		Data:     []byte("test data"),
		Hash:     "testhash",
		NodeID:   "node1",
		Sequence: 123,
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
	assert.Equal(t, cmd.Hash, unmarshaled.Hash)
	assert.Equal(t, cmd.NodeID, unmarshaled.NodeID)
	assert.Equal(t, cmd.Sequence, unmarshaled.Sequence)
}

func TestHashContent(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected string
	}{
		{
			name:     "empty_data",
			data:     []byte{},
			expected: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name:     "simple_string",
			data:     []byte("hello"),
			expected: "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
		},
		{
			name: "binary_data",
			data: []byte{0x00, 0x01, 0x02, 0x03},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hashContent(tt.data)

			// Verify it's a valid SHA-256 hash (64 hex characters)
			assert.Len(t, result, 64)

			// Verify it's deterministic
			result2 := hashContent(tt.data)
			assert.Equal(t, result, result2)

			if tt.expected != "" {
				assert.Equal(t, tt.expected, result)
			}

			// Verify it matches manual SHA-256 calculation
			hash := sha256.Sum256(tt.data)
			expected := hex.EncodeToString(hash[:])
			assert.Equal(t, expected, result)
		})
	}
}

func TestFSM_fileHasContent(t *testing.T) {
	fsm := &FSM{
		fileStates: make(map[string]*FileState),
		nodeID:     "test-node",
	}

	testData := []byte("test content")
	testHash := hashContent(testData)

	tests := []struct {
		name     string
		path     string
		data     []byte
		setup    func()
		expected bool
	}{
		{
			name:     "no_state_exists",
			path:     "nonexistent.txt",
			data:     testData,
			setup:    func() {},
			expected: false,
		},
		{
			name: "matching_hash",
			path: "matching.txt",
			data: testData,
			setup: func() {
				fsm.fileStates["matching.txt"] = &FileState{
					Hash:         testHash,
					LastModified: time.Now(),
					Size:         int64(len(testData)),
				}
			},
			expected: true,
		},
		{
			name: "different_hash",
			path: "different.txt",
			data: []byte("different content"),
			setup: func() {
				fsm.fileStates["different.txt"] = &FileState{
					Hash:         testHash,
					LastModified: time.Now(),
					Size:         int64(len(testData)),
				}
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			result := fsm.fileHasContent(tt.path, tt.data)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFSM_updateFileState(t *testing.T) {
	fsm := &FSM{
		fileStates: make(map[string]*FileState),
		nodeID:     "test-node",
	}

	testData := []byte("test content for state update")
	testPath := "test/file.txt"

	// Test initial update
	fsm.updateFileState(testPath, testData)

	state, exists := fsm.fileStates[testPath]
	assert.True(t, exists)
	assert.NotNil(t, state)
	assert.Equal(t, hashContent(testData), state.Hash)
	assert.Equal(t, int64(len(testData)), state.Size)
	assert.True(t, time.Since(state.LastModified) < time.Second)

	// Test update with different content
	newData := []byte("updated content")
	fsm.updateFileState(testPath, newData)

	updatedState, exists := fsm.fileStates[testPath]
	assert.True(t, exists)
	assert.NotNil(t, updatedState)
	assert.Equal(t, hashContent(newData), updatedState.Hash)
	assert.Equal(t, int64(len(newData)), updatedState.Size)
	assert.NotEqual(t, state.Hash, updatedState.Hash)
}

func TestFSM_removeFileState(t *testing.T) {
	fsm := &FSM{
		fileStates: make(map[string]*FileState),
		nodeID:     "test-node",
	}

	testPath := "test/remove.txt"
	testData := []byte("content to remove")

	// Add state first
	fsm.updateFileState(testPath, testData)
	_, exists := fsm.fileStates[testPath]
	assert.True(t, exists)

	// Remove state
	fsm.removeFileState(testPath)
	_, exists = fsm.fileStates[testPath]
	assert.False(t, exists)

	// Removing non-existent state should not panic
	assert.NotPanics(t, func() {
		fsm.removeFileState("nonexistent.txt")
	})
}

func TestFSM_getNextSequence(t *testing.T) {
	fsm := &FSM{
		lastSequence: 0,
	}

	// Test sequential generation
	for i := int64(1); i <= 10; i++ {
		seq := fsm.getNextSequence()
		assert.Equal(t, i, seq)
	}

	// Test concurrency
	results := make(chan int64, 100)
	for i := 0; i < 100; i++ {
		go func() {
			results <- fsm.getNextSequence()
		}()
	}

	sequences := make([]int64, 100)
	for i := 0; i < 100; i++ {
		sequences[i] = <-results
	}

	// Verify all sequences are unique
	seen := make(map[int64]bool)
	for _, seq := range sequences {
		assert.False(t, seen[seq], "Duplicate sequence: %d", seq)
		seen[seq] = true
		assert.True(t, seq > 10) // Should be after our initial 10
	}
}

func TestFSM_watchingControls(t *testing.T) {
	fsm := &FSM{
		nodeID: "test-node",
	}

	// Initial state should not be paused
	assert.False(t, fsm.isWatchingPaused())

	// Test pause
	fsm.pauseWatching()
	assert.True(t, fsm.isWatchingPaused())

	// Test resume (note: has built-in delay)
	go fsm.resumeWatching()

	// Should still be paused briefly
	assert.True(t, fsm.isWatchingPaused())

	// Wait for resume to complete
	time.Sleep(250 * time.Millisecond)
	assert.False(t, fsm.isWatchingPaused())
}

func TestFSM_Apply(t *testing.T) {
	tempDir := "/tmp/test-multi-fsm-apply"
	os.RemoveAll(tempDir)
	defer os.RemoveAll(tempDir)

	fsm := &FSM{
		dataDir:    tempDir,
		nodeID:     "test-node",
		fileStates: make(map[string]*FileState),
	}

	tests := []struct {
		name    string
		command Command
		setup   func()
		verify  func(t *testing.T)
	}{
		{
			name: "write_new_file",
			command: Command{
				Op:       "write",
				Path:     "test/new.txt",
				Data:     []byte("new file content"),
				Hash:     hashContent([]byte("new file content")),
				NodeID:   "other-node",
				Sequence: 1,
			},
			setup: func() {},
			verify: func(t *testing.T) {
				// Verify file was created
				filePath := filepath.Join(tempDir, "test/new.txt")
				content, err := os.ReadFile(filePath)
				assert.NoError(t, err)
				assert.Equal(t, []byte("new file content"), content)

				// Verify file state was updated
				state, exists := fsm.fileStates["test/new.txt"]
				assert.True(t, exists)
				assert.Equal(t, hashContent([]byte("new file content")), state.Hash)
			},
		},
		{
			name: "write_existing_file_same_content",
			command: Command{
				Op:       "write",
				Path:     "test/existing.txt",
				Data:     []byte("existing content"),
				Hash:     hashContent([]byte("existing content")),
				NodeID:   "test-node", // Same node
				Sequence: 2,
			},
			setup: func() {
				// Pre-create file and state
				filePath := filepath.Join(tempDir, "test/existing.txt")
				os.MkdirAll(filepath.Dir(filePath), 0755)
				os.WriteFile(filePath, []byte("existing content"), 0644)
				fsm.updateFileState("test/existing.txt", []byte("existing content"))
			},
			verify: func(t *testing.T) {
				// Should have been skipped
				filePath := filepath.Join(tempDir, "test/existing.txt")
				content, err := os.ReadFile(filePath)
				assert.NoError(t, err)
				assert.Equal(t, []byte("existing content"), content)
			},
		},
		{
			name: "delete_file",
			command: Command{
				Op:       "delete",
				Path:     "test/delete.txt",
				NodeID:   "other-node",
				Sequence: 3,
			},
			setup: func() {
				// Pre-create file
				filePath := filepath.Join(tempDir, "test/delete.txt")
				os.MkdirAll(filepath.Dir(filePath), 0755)
				os.WriteFile(filePath, []byte("to be deleted"), 0644)
				fsm.updateFileState("test/delete.txt", []byte("to be deleted"))
			},
			verify: func(t *testing.T) {
				// Verify file was deleted
				filePath := filepath.Join(tempDir, "test/delete.txt")
				_, err := os.Stat(filePath)
				assert.True(t, os.IsNotExist(err))

				// Verify state was removed
				_, exists := fsm.fileStates["test/delete.txt"]
				assert.False(t, exists)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			data, err := json.Marshal(tt.command)
			require.NoError(t, err)

			log := &raft.Log{Data: data}
			result := fsm.Apply(log)
			assert.Nil(t, result) // No error expected

			tt.verify(t)
		})
	}
}

func TestFSM_Apply_InvalidJSON(t *testing.T) {
	fsm := &FSM{
		dataDir:    "/tmp/test-invalid-json",
		nodeID:     "test-node",
		fileStates: make(map[string]*FileState),
	}

	log := &raft.Log{
		Data: []byte("invalid json"),
	}

	result := fsm.Apply(log)
	assert.NotNil(t, result)
	assert.Contains(t, result.(error).Error(), "failed to unmarshal command")
}

func TestSnapshot_PersistAndRelease(t *testing.T) {
	snapshot := &Snapshot{
		dataDir: "/tmp/test-snapshot",
	}

	// Test Release (should not panic)
	assert.NotPanics(t, func() {
		snapshot.Release()
	})

	// Test Persist with mock sink
	mockSink := &mockSnapshotSink{}
	err := snapshot.Persist(mockSink)
	assert.NoError(t, err)
	assert.True(t, mockSink.closed)
}

func TestIsRaftFile(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		expected bool
	}{
		{
			name:     "raft_prefix",
			filename: "/data/raft-logs.db",
			expected: true,
		},
		{
			name:     "db_suffix",
			filename: "/data/store.db",
			expected: true,
		},
		{
			name:     "snapshots_dir",
			filename: "/data/snapshots/snapshot.dat",
			expected: true,
		},
		{
			name:     "snapshots_basename",
			filename: "/data/snapshots",
			expected: true,
		},
		{
			name:     "regular_file",
			filename: "/data/user/document.txt",
			expected: false,
		},
		{
			name:     "file_with_raft_in_name",
			filename: "/data/my-raft-document.txt",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRaftFile(tt.filename)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFileState_Creation(t *testing.T) {
	testData := []byte("test file state")
	state := &FileState{
		Hash:         hashContent(testData),
		LastModified: time.Now(),
		Size:         int64(len(testData)),
	}

	assert.Equal(t, hashContent(testData), state.Hash)
	assert.Equal(t, int64(len(testData)), state.Size)
	assert.True(t, time.Since(state.LastModified) < time.Second)
}

// Mock implementation for testing
type mockSnapshotSink struct {
	closed bool
}

func (m *mockSnapshotSink) Write(p []byte) (n int, err error) {
	return len(p), nil
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
func BenchmarkHashContent(b *testing.B) {
	data := []byte("benchmark test data for hashing performance")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hashContent(data)
	}
}

func BenchmarkFSM_fileHasContent(b *testing.B) {
	fsm := &FSM{
		fileStates: make(map[string]*FileState),
		nodeID:     "bench-node",
	}

	testData := []byte("benchmark data")
	fsm.updateFileState("bench.txt", testData)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fsm.fileHasContent("bench.txt", testData)
	}
}

func BenchmarkFSM_updateFileState(b *testing.B) {
	fsm := &FSM{
		fileStates: make(map[string]*FileState),
		nodeID:     "bench-node",
	}

	testData := []byte("benchmark data for state update")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fsm.updateFileState("bench.txt", testData)
	}
}

func BenchmarkFSM_getNextSequence(b *testing.B) {
	fsm := &FSM{
		lastSequence: 0,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fsm.getNextSequence()
	}
}

func BenchmarkCommand_JSONMarshal(b *testing.B) {
	cmd := Command{
		Op:       "write",
		Path:     "benchmark/file.txt",
		Data:     []byte("benchmark data for marshaling"),
		Hash:     "benchmarkhash",
		NodeID:   "bench-node",
		Sequence: 123456,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		json.Marshal(cmd)
	}
}
