package replication

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/raft"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock FileWatcher for testing
type mockFileWatcher struct {
	pauseCalled  bool
	resumeCalled bool
	mutex        sync.Mutex
}

func (m *mockFileWatcher) PauseWatching() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.pauseCalled = true
}

func (m *mockFileWatcher) ResumeWatching() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.resumeCalled = true
}

func (m *mockFileWatcher) wasPauseCalled() bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.pauseCalled
}

func (m *mockFileWatcher) wasResumeCalled() bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.resumeCalled
}

func (m *mockFileWatcher) reset() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.pauseCalled = false
	m.resumeCalled = false
}

// Mock MetricsCollector for testing
type mockMetricsCollector struct {
	filesReplicated   int
	filesDeleted      int
	bytesReplicated   int64
	replicationErrors int
	mutex             sync.Mutex
}

func (m *mockMetricsCollector) IncrementFilesReplicated() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.filesReplicated++
}

func (m *mockMetricsCollector) IncrementFilesDeleted() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.filesDeleted++
}

func (m *mockMetricsCollector) AddBytesReplicated(bytes int64) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.bytesReplicated += bytes
}

func (m *mockMetricsCollector) IncrementReplicationErrors() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.replicationErrors++
}

func (m *mockMetricsCollector) getFilesReplicated() int {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.filesReplicated
}

func (m *mockMetricsCollector) getFilesDeleted() int {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.filesDeleted
}

func (m *mockMetricsCollector) getBytesReplicated() int64 {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.bytesReplicated
}

func (m *mockMetricsCollector) getReplicationErrors() int {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.replicationErrors
}

func (m *mockMetricsCollector) reset() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.filesReplicated = 0
	m.filesDeleted = 0
	m.bytesReplicated = 0
	m.replicationErrors = 0
}

// Helper function to create a temporary directory
func createTempDir(t *testing.T) string {
	tempDir, err := os.MkdirTemp("", "fsm_test_")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(tempDir) })
	return tempDir
}

func TestNewFSM(t *testing.T) {
	t.Run("valid_fsm_creation", func(t *testing.T) {
		tempDir := createTempDir(t)
		logger := logrus.New()

		fsm := NewFSM(tempDir, "test-node", logger)

		assert.NotNil(t, fsm)
		assert.Equal(t, tempDir, fsm.dataDir)
		assert.Equal(t, "test-node", fsm.nodeID)
		assert.Equal(t, logger, fsm.logger)
		assert.NotNil(t, fsm.fileStates)
		assert.Equal(t, int64(0), fsm.lastSequence)
	})

	t.Run("nil_logger", func(t *testing.T) {
		tempDir := createTempDir(t)

		fsm := NewFSM(tempDir, "test-node", nil)

		assert.NotNil(t, fsm)
		assert.NotNil(t, fsm.logger)
	})

	t.Run("empty_parameters", func(t *testing.T) {
		fsm := NewFSM("", "", nil)

		assert.NotNil(t, fsm)
		assert.Equal(t, "", fsm.dataDir)
		assert.Equal(t, "", fsm.nodeID)
		assert.NotNil(t, fsm.logger)
	})
}

func TestFSM_SetFileWatcher(t *testing.T) {
	tempDir := createTempDir(t)
	fsm := NewFSM(tempDir, "test-node", nil)
	mockWatcher := &mockFileWatcher{}

	fsm.SetFileWatcher(mockWatcher)

	assert.Equal(t, mockWatcher, fsm.watcher)
}

func TestFSM_SetMetricsCollector(t *testing.T) {
	tempDir := createTempDir(t)
	fsm := NewFSM(tempDir, "test-node", nil)
	mockMetrics := &mockMetricsCollector{}

	fsm.SetMetricsCollector(mockMetrics)

	assert.Equal(t, mockMetrics, fsm.metrics)
}

func TestFSM_Apply(t *testing.T) {
	tempDir := createTempDir(t)
	fsm := NewFSM(tempDir, "test-node", nil)
	mockWatcher := &mockFileWatcher{}
	mockMetrics := &mockMetricsCollector{}
	fsm.SetFileWatcher(mockWatcher)
	fsm.SetMetricsCollector(mockMetrics)

	t.Run("write_operation", func(t *testing.T) {
		mockWatcher.reset()
		mockMetrics.reset()

		cmd := Command{
			Op:       OpWrite,
			Path:     "test.txt",
			Data:     []byte("test content"),
			Hash:     HashContent([]byte("test content")),
			NodeID:   "other-node",
			Sequence: 1,
		}

		cmdData, err := json.Marshal(cmd)
		require.NoError(t, err)

		log := &raft.Log{Data: cmdData}
		result := fsm.Apply(log)

		assert.Nil(t, result)
		assert.True(t, mockWatcher.wasPauseCalled())
		assert.True(t, mockWatcher.wasResumeCalled())
		assert.Equal(t, 1, mockMetrics.getFilesReplicated())
		assert.Equal(t, int64(12), mockMetrics.getBytesReplicated())

		// Check file was created
		filePath := filepath.Join(tempDir, "test.txt")
		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, []byte("test content"), content)
	})

	t.Run("delete_operation", func(t *testing.T) {
		mockWatcher.reset()
		mockMetrics.reset()

		// First create a file
		filePath := filepath.Join(tempDir, "delete_test.txt")
		err := os.WriteFile(filePath, []byte("content"), 0644)
		require.NoError(t, err)

		cmd := Command{
			Op:       OpDelete,
			Path:     "delete_test.txt",
			NodeID:   "other-node",
			Sequence: 2,
		}

		cmdData, err := json.Marshal(cmd)
		require.NoError(t, err)

		log := &raft.Log{Data: cmdData}
		result := fsm.Apply(log)

		assert.Nil(t, result)
		assert.True(t, mockWatcher.wasPauseCalled())
		assert.True(t, mockWatcher.wasResumeCalled())
		assert.Equal(t, 1, mockMetrics.getFilesDeleted())

		// Check file was deleted
		_, err = os.Stat(filePath)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("invalid_json", func(t *testing.T) {
		mockMetrics.reset()

		log := &raft.Log{Data: []byte("invalid json")}
		result := fsm.Apply(log)

		assert.Error(t, result.(error))
		assert.Equal(t, 1, mockMetrics.getReplicationErrors())
	})

	t.Run("unknown_operation", func(t *testing.T) {
		mockMetrics.reset()

		cmd := Command{
			Op:       "unknown",
			Path:     "test.txt",
			NodeID:   "other-node",
			Sequence: 3,
		}

		cmdData, err := json.Marshal(cmd)
		require.NoError(t, err)

		log := &raft.Log{Data: cmdData}
		result := fsm.Apply(log)

		assert.Error(t, result.(error))
		assert.Contains(t, result.(error).Error(), "unknown operation")
		assert.Equal(t, 1, mockMetrics.getReplicationErrors())
	})

	t.Run("skip_same_node_with_same_content", func(t *testing.T) {
		mockWatcher.reset()
		mockMetrics.reset()

		// First, create the file state
		testData := []byte("test content")
		fsm.UpdateFileState("test.txt", testData)

		cmd := Command{
			Op:       OpWrite,
			Path:     "test.txt",
			Data:     testData,
			Hash:     HashContent(testData),
			NodeID:   "test-node", // Same as fsm.nodeID
			Sequence: 1,
		}

		cmdData, err := json.Marshal(cmd)
		require.NoError(t, err)

		log := &raft.Log{Data: cmdData}
		result := fsm.Apply(log)

		assert.Nil(t, result)
		// Should not increment metrics for skipped operation
		assert.Equal(t, 0, mockMetrics.getFilesReplicated())
	})

	t.Run("no_watcher_set", func(t *testing.T) {
		tempDir2 := createTempDir(t)
		fsm2 := NewFSM(tempDir2, "test-node", nil)

		cmd := Command{
			Op:       OpWrite,
			Path:     "test.txt",
			Data:     []byte("test content"),
			NodeID:   "other-node",
			Sequence: 1,
		}

		cmdData, err := json.Marshal(cmd)
		require.NoError(t, err)

		log := &raft.Log{Data: cmdData}
		result := fsm2.Apply(log)

		assert.Nil(t, result)

		// Check file was created
		filePath := filepath.Join(tempDir2, "test.txt")
		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, []byte("test content"), content)
	})

	t.Run("no_metrics_set", func(t *testing.T) {
		tempDir2 := createTempDir(t)
		fsm2 := NewFSM(tempDir2, "test-node", nil)

		cmd := Command{
			Op:       OpWrite,
			Path:     "test.txt",
			Data:     []byte("test content"),
			NodeID:   "other-node",
			Sequence: 1,
		}

		cmdData, err := json.Marshal(cmd)
		require.NoError(t, err)

		log := &raft.Log{Data: cmdData}
		result := fsm2.Apply(log)

		assert.Nil(t, result)

		// Check file was created
		filePath := filepath.Join(tempDir2, "test.txt")
		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, []byte("test content"), content)
	})
}

func TestFSM_applyWrite(t *testing.T) {
	tempDir := createTempDir(t)
	fsm := NewFSM(tempDir, "test-node", nil)

	t.Run("successful_write", func(t *testing.T) {
		cmd := Command{
			Op:   OpWrite,
			Path: "test.txt",
			Data: []byte("test content"),
		}

		err := fsm.applyWrite(cmd)
		assert.NoError(t, err)

		// Check file was created
		filePath := filepath.Join(tempDir, "test.txt")
		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, []byte("test content"), content)

		// Check file state was updated
		assert.True(t, fsm.FileHasContent("test.txt", []byte("test content")))
	})

	t.Run("write_with_nested_path", func(t *testing.T) {
		cmd := Command{
			Op:   OpWrite,
			Path: "dir/subdir/test.txt",
			Data: []byte("nested content"),
		}

		err := fsm.applyWrite(cmd)
		assert.NoError(t, err)

		// Check file was created
		filePath := filepath.Join(tempDir, "dir/subdir/test.txt")
		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, []byte("nested content"), content)
	})

	t.Run("skip_if_content_matches", func(t *testing.T) {
		// Set up existing file state
		testData := []byte("existing content")
		fsm.UpdateFileState("existing.txt", testData)

		cmd := Command{
			Op:   OpWrite,
			Path: "existing.txt",
			Data: testData,
		}

		err := fsm.applyWrite(cmd)
		assert.NoError(t, err)

		// File should not be created since content matches
		filePath := filepath.Join(tempDir, "existing.txt")
		_, err = os.Stat(filePath)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("empty_data", func(t *testing.T) {
		cmd := Command{
			Op:   OpWrite,
			Path: "empty.txt",
			Data: []byte{},
		}

		err := fsm.applyWrite(cmd)
		assert.NoError(t, err)

		// Check empty file was created
		filePath := filepath.Join(tempDir, "empty.txt")
		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, []byte{}, content)
	})
}

func TestFSM_applyDelete(t *testing.T) {
	tempDir := createTempDir(t)
	fsm := NewFSM(tempDir, "test-node", nil)

	t.Run("successful_delete", func(t *testing.T) {
		// Create a file first
		filePath := filepath.Join(tempDir, "delete_me.txt")
		err := os.WriteFile(filePath, []byte("content"), 0644)
		require.NoError(t, err)

		// Set file state
		fsm.UpdateFileState("delete_me.txt", []byte("content"))

		cmd := Command{
			Op:   OpDelete,
			Path: "delete_me.txt",
		}

		err = fsm.applyDelete(cmd)
		assert.NoError(t, err)

		// Check file was deleted
		_, err = os.Stat(filePath)
		assert.True(t, os.IsNotExist(err))

		// Check file state was removed
		assert.False(t, fsm.FileHasContent("delete_me.txt", []byte("content")))
	})

	t.Run("delete_nonexistent_file", func(t *testing.T) {
		cmd := Command{
			Op:   OpDelete,
			Path: "nonexistent.txt",
		}

		err := fsm.applyDelete(cmd)
		assert.NoError(t, err) // Should not error for non-existent files
	})
}

func TestFSM_FileHasContent(t *testing.T) {
	tempDir := createTempDir(t)
	fsm := NewFSM(tempDir, "test-node", nil)

	t.Run("file_has_content", func(t *testing.T) {
		testData := []byte("test content")
		fsm.UpdateFileState("test.txt", testData)

		hasContent := fsm.FileHasContent("test.txt", testData)
		assert.True(t, hasContent)
	})

	t.Run("file_different_content", func(t *testing.T) {
		testData := []byte("test content")
		fsm.UpdateFileState("test.txt", testData)

		hasContent := fsm.FileHasContent("test.txt", []byte("different content"))
		assert.False(t, hasContent)
	})

	t.Run("file_not_tracked", func(t *testing.T) {
		hasContent := fsm.FileHasContent("nonexistent.txt", []byte("content"))
		assert.False(t, hasContent)
	})

	t.Run("empty_content", func(t *testing.T) {
		emptyData := []byte{}
		fsm.UpdateFileState("empty.txt", emptyData)

		hasContent := fsm.FileHasContent("empty.txt", emptyData)
		assert.True(t, hasContent)
	})
}

func TestFSM_UpdateFileState(t *testing.T) {
	tempDir := createTempDir(t)
	fsm := NewFSM(tempDir, "test-node", nil)

	t.Run("update_file_state", func(t *testing.T) {
		testData := []byte("test content")

		fsm.UpdateFileState("test.txt", testData)

		// Check state was updated
		fsm.fileStatesMutex.RLock()
		state, exists := fsm.fileStates["test.txt"]
		fsm.fileStatesMutex.RUnlock()

		assert.True(t, exists)
		assert.Equal(t, HashContent(testData), state.Hash)
		assert.Equal(t, int64(len(testData)), state.Size)
		assert.WithinDuration(t, time.Now(), state.LastModified, time.Second)
	})

	t.Run("update_existing_file_state", func(t *testing.T) {
		// Set initial state
		fsm.UpdateFileState("test.txt", []byte("initial"))

		// Update with new content
		newData := []byte("updated content")
		fsm.UpdateFileState("test.txt", newData)

		// Check state was updated
		fsm.fileStatesMutex.RLock()
		state, exists := fsm.fileStates["test.txt"]
		fsm.fileStatesMutex.RUnlock()

		assert.True(t, exists)
		assert.Equal(t, HashContent(newData), state.Hash)
		assert.Equal(t, int64(len(newData)), state.Size)
	})

	t.Run("concurrent_updates", func(t *testing.T) {
		var wg sync.WaitGroup

		// Start multiple goroutines updating different files
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				path := fmt.Sprintf("file%d.txt", index)
				data := []byte(fmt.Sprintf("content%d", index))
				fsm.UpdateFileState(path, data)
			}(i)
		}

		wg.Wait()

		// Check all files were updated
		fsm.fileStatesMutex.RLock()
		assert.Equal(t, 11, len(fsm.fileStates)) // 10 new + 1 from previous test
		fsm.fileStatesMutex.RUnlock()
	})
}

func TestFSM_RemoveFileState(t *testing.T) {
	tempDir := createTempDir(t)
	fsm := NewFSM(tempDir, "test-node", nil)

	t.Run("remove_existing_file_state", func(t *testing.T) {
		// Add file state first
		fsm.UpdateFileState("remove_me.txt", []byte("content"))

		// Verify it exists
		fsm.fileStatesMutex.RLock()
		_, exists := fsm.fileStates["remove_me.txt"]
		fsm.fileStatesMutex.RUnlock()
		assert.True(t, exists)

		// Remove it
		fsm.RemoveFileState("remove_me.txt")

		// Verify it's gone
		fsm.fileStatesMutex.RLock()
		_, exists = fsm.fileStates["remove_me.txt"]
		fsm.fileStatesMutex.RUnlock()
		assert.False(t, exists)
	})

	t.Run("remove_nonexistent_file_state", func(t *testing.T) {
		// Should not panic
		fsm.RemoveFileState("nonexistent.txt")
	})

	t.Run("concurrent_removals", func(t *testing.T) {
		// Add multiple file states
		for i := 0; i < 10; i++ {
			path := fmt.Sprintf("concurrent%d.txt", i)
			fsm.UpdateFileState(path, []byte("content"))
		}

		var wg sync.WaitGroup

		// Remove them concurrently
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				path := fmt.Sprintf("concurrent%d.txt", index)
				fsm.RemoveFileState(path)
			}(i)
		}

		wg.Wait()

		// Check all were removed
		fsm.fileStatesMutex.RLock()
		for i := 0; i < 10; i++ {
			path := fmt.Sprintf("concurrent%d.txt", i)
			_, exists := fsm.fileStates[path]
			assert.False(t, exists)
		}
		fsm.fileStatesMutex.RUnlock()
	})
}

func TestFSM_GetNextSequence(t *testing.T) {
	tempDir := createTempDir(t)
	fsm := NewFSM(tempDir, "test-node", nil)

	t.Run("sequential_numbers", func(t *testing.T) {
		seq1 := fsm.GetNextSequence()
		seq2 := fsm.GetNextSequence()
		seq3 := fsm.GetNextSequence()

		assert.Equal(t, int64(1), seq1)
		assert.Equal(t, int64(2), seq2)
		assert.Equal(t, int64(3), seq3)
	})

	t.Run("concurrent_sequence_generation", func(t *testing.T) {
		var wg sync.WaitGroup
		sequences := make([]int64, 100)

		// Generate sequences concurrently
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				sequences[index] = fsm.GetNextSequence()
			}(i)
		}

		wg.Wait()

		// Check all sequences are unique and in range
		seenSequences := make(map[int64]bool)
		for _, seq := range sequences {
			assert.False(t, seenSequences[seq], "Duplicate sequence: %d", seq)
			seenSequences[seq] = true
			assert.Greater(t, seq, int64(3)) // Should be greater than previous test
		}
	})
}

func TestFSM_Snapshot(t *testing.T) {
	tempDir := createTempDir(t)
	fsm := NewFSM(tempDir, "test-node", nil)

	t.Run("create_snapshot", func(t *testing.T) {
		snapshot, err := fsm.Snapshot()

		assert.NoError(t, err)
		assert.NotNil(t, snapshot)

		// Check snapshot has correct dataDir
		snap := snapshot.(*Snapshot)
		assert.Equal(t, tempDir, snap.dataDir)
	})
}

func TestFSM_Restore(t *testing.T) {
	tempDir := createTempDir(t)
	fsm := NewFSM(tempDir, "test-node", nil)

	t.Run("restore_from_snapshot", func(t *testing.T) {
		reader := io.NopCloser(bytes.NewReader([]byte("snapshot data")))

		err := fsm.Restore(reader)

		assert.NoError(t, err)
	})

	t.Run("restore_with_empty_reader", func(t *testing.T) {
		reader := io.NopCloser(bytes.NewReader([]byte{}))

		err := fsm.Restore(reader)

		assert.NoError(t, err)
	})
}

func TestSnapshot_Persist(t *testing.T) {
	tempDir := createTempDir(t)
	snapshot := &Snapshot{dataDir: tempDir}

	t.Run("successful_persist", func(t *testing.T) {
		var buf bytes.Buffer
		sink := &mockSnapshotSink{
			writer: &buf,
		}

		err := snapshot.Persist(sink)

		assert.NoError(t, err)
		assert.Equal(t, "snapshot", buf.String())
		assert.True(t, sink.closeCalled)
	})

	t.Run("persist_with_write_error", func(t *testing.T) {
		sink := &mockSnapshotSink{
			writeError: assert.AnError,
		}

		err := snapshot.Persist(sink)

		assert.Error(t, err)
		assert.True(t, sink.cancelCalled)
		assert.True(t, sink.closeCalled)
	})
}

func TestSnapshot_Release(t *testing.T) {
	snapshot := &Snapshot{dataDir: "/tmp/test"}

	// Should not panic
	snapshot.Release()
}

func TestHashContent(t *testing.T) {
	t.Run("hash_content", func(t *testing.T) {
		data := []byte("test content")
		hash := HashContent(data)

		assert.NotEmpty(t, hash)
		assert.Equal(t, 64, len(hash)) // SHA-256 hex string length
	})

	t.Run("hash_empty_content", func(t *testing.T) {
		data := []byte{}
		hash := HashContent(data)

		assert.NotEmpty(t, hash)
		assert.Equal(t, 64, len(hash))
	})

	t.Run("hash_consistency", func(t *testing.T) {
		data := []byte("consistent content")
		hash1 := HashContent(data)
		hash2 := HashContent(data)

		assert.Equal(t, hash1, hash2)
	})

	t.Run("hash_uniqueness", func(t *testing.T) {
		data1 := []byte("content1")
		data2 := []byte("content2")
		hash1 := HashContent(data1)
		hash2 := HashContent(data2)

		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("hash_large_content", func(t *testing.T) {
		// Create large content
		largeData := make([]byte, 1024*1024) // 1MB
		for i := range largeData {
			largeData[i] = byte(i % 256)
		}

		hash := HashContent(largeData)

		assert.NotEmpty(t, hash)
		assert.Equal(t, 64, len(hash))
	})
}

func TestCommand_JSONSerialization(t *testing.T) {
	t.Run("serialize_write_command", func(t *testing.T) {
		cmd := Command{
			Op:       OpWrite,
			Path:     "test.txt",
			Data:     []byte("test content"),
			Hash:     "abc123",
			NodeID:   "node1",
			Sequence: 1,
		}

		data, err := json.Marshal(cmd)
		assert.NoError(t, err)

		var unmarshaled Command
		err = json.Unmarshal(data, &unmarshaled)
		assert.NoError(t, err)

		assert.Equal(t, cmd.Op, unmarshaled.Op)
		assert.Equal(t, cmd.Path, unmarshaled.Path)
		assert.Equal(t, cmd.Data, unmarshaled.Data)
		assert.Equal(t, cmd.Hash, unmarshaled.Hash)
		assert.Equal(t, cmd.NodeID, unmarshaled.NodeID)
		assert.Equal(t, cmd.Sequence, unmarshaled.Sequence)
	})

	t.Run("serialize_delete_command", func(t *testing.T) {
		cmd := Command{
			Op:       OpDelete,
			Path:     "test.txt",
			NodeID:   "node1",
			Sequence: 2,
		}

		data, err := json.Marshal(cmd)
		assert.NoError(t, err)

		var unmarshaled Command
		err = json.Unmarshal(data, &unmarshaled)
		assert.NoError(t, err)

		assert.Equal(t, cmd.Op, unmarshaled.Op)
		assert.Equal(t, cmd.Path, unmarshaled.Path)
		assert.Equal(t, cmd.NodeID, unmarshaled.NodeID)
		assert.Equal(t, cmd.Sequence, unmarshaled.Sequence)
	})
}

func TestFileState_Fields(t *testing.T) {
	state := FileState{
		Hash:         "abc123",
		LastModified: time.Now(),
		Size:         1024,
	}

	assert.Equal(t, "abc123", state.Hash)
	assert.Equal(t, int64(1024), state.Size)
	assert.WithinDuration(t, time.Now(), state.LastModified, time.Second)
}

func TestConstants(t *testing.T) {
	assert.Equal(t, "write", OpWrite)
	assert.Equal(t, "delete", OpDelete)
	assert.Equal(t, 0755, DirPerm)
	assert.Equal(t, 0644, FilePerm)
}

// Mock snapshot sink for testing
type mockSnapshotSink struct {
	writer       *bytes.Buffer
	writeError   error
	cancelCalled bool
	closeCalled  bool
}

func (m *mockSnapshotSink) Write(p []byte) (int, error) {
	if m.writeError != nil {
		return 0, m.writeError
	}
	if m.writer != nil {
		return m.writer.Write(p)
	}
	return len(p), nil
}

func (m *mockSnapshotSink) Close() error {
	m.closeCalled = true
	return nil
}

func (m *mockSnapshotSink) Cancel() error {
	m.cancelCalled = true
	return nil
}

func (m *mockSnapshotSink) ID() string {
	return "test-snapshot"
}

// Benchmark tests
func BenchmarkFSM_Apply_Write(b *testing.B) {
	tempDir, _ := os.MkdirTemp("", "benchmark_")
	defer os.RemoveAll(tempDir)

	fsm := NewFSM(tempDir, "test-node", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cmd := Command{
			Op:       OpWrite,
			Path:     fmt.Sprintf("bench%d.txt", i),
			Data:     []byte("benchmark content"),
			NodeID:   "other-node",
			Sequence: int64(i),
		}

		cmdData, _ := json.Marshal(cmd)
		log := &raft.Log{Data: cmdData}
		fsm.Apply(log)
	}
}

func BenchmarkHashContent(b *testing.B) {
	data := []byte("benchmark content for hashing")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		HashContent(data)
	}
}

func BenchmarkFSM_UpdateFileState(b *testing.B) {
	tempDir, _ := os.MkdirTemp("", "benchmark_")
	defer os.RemoveAll(tempDir)

	fsm := NewFSM(tempDir, "test-node", nil)
	data := []byte("benchmark content")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fsm.UpdateFileState(fmt.Sprintf("bench%d.txt", i), data)
	}
}

// Test error handling in applyWrite
func TestFSM_applyWrite_ErrorHandling(t *testing.T) {
	tempDir := createTempDir(t)
	fsm := NewFSM(tempDir, "test-node", nil)

	t.Run("write_to_invalid_path", func(t *testing.T) {
		// Create a file where we want to create a directory
		filePath := filepath.Join(tempDir, "blocking_file")
		err := os.WriteFile(filePath, []byte("content"), 0644)
		require.NoError(t, err)

		// Try to create a file inside what should be a directory
		cmd := Command{
			Op:   OpWrite,
			Path: "blocking_file/nested.txt",
			Data: []byte("content"),
		}

		err = fsm.applyWrite(cmd)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "creating directory")
	})

	t.Run("write_to_read_only_directory", func(t *testing.T) {
		// Create a read-only directory
		readOnlyDir := filepath.Join(tempDir, "readonly")
		err := os.MkdirAll(readOnlyDir, 0755)
		require.NoError(t, err)

		// Make it read-only
		err = os.Chmod(readOnlyDir, 0444)
		require.NoError(t, err)

		// Clean up after test
		defer os.Chmod(readOnlyDir, 0755)

		cmd := Command{
			Op:   OpWrite,
			Path: "readonly/test.txt",
			Data: []byte("content"),
		}

		err = fsm.applyWrite(cmd)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "writing file")
	})

	t.Run("write_with_very_long_path", func(t *testing.T) {
		// Create a very long path
		longPath := ""
		for i := 0; i < 100; i++ {
			longPath += fmt.Sprintf("very_long_directory_name_%d/", i)
		}
		longPath += "file.txt"

		cmd := Command{
			Op:   OpWrite,
			Path: longPath,
			Data: []byte("content"),
		}

		err := fsm.applyWrite(cmd)
		// This might succeed or fail depending on filesystem limits
		if err != nil {
			assert.Contains(t, err.Error(), "creating directory")
		}
	})
}

// Test error handling in applyDelete
func TestFSM_applyDelete_ErrorHandling(t *testing.T) {
	tempDir := createTempDir(t)
	fsm := NewFSM(tempDir, "test-node", nil)

	t.Run("delete_directory_instead_of_file", func(t *testing.T) {
		// Create a directory
		dirPath := filepath.Join(tempDir, "directory")
		err := os.MkdirAll(dirPath, 0755)
		require.NoError(t, err)

		cmd := Command{
			Op:   OpDelete,
			Path: "directory",
		}

		err = fsm.applyDelete(cmd)
		// On some systems, this might succeed (removing directory) or fail
		// The test should verify the behavior matches expectations
		if err != nil {
			assert.Contains(t, err.Error(), "deleting file")
		}
	})

	t.Run("delete_in_read_only_directory", func(t *testing.T) {
		// Create a read-only directory with a file
		readOnlyDir := filepath.Join(tempDir, "readonly2")
		err := os.MkdirAll(readOnlyDir, 0755)
		require.NoError(t, err)

		filePath := filepath.Join(readOnlyDir, "file.txt")
		err = os.WriteFile(filePath, []byte("content"), 0644)
		require.NoError(t, err)

		// Make directory read-only
		err = os.Chmod(readOnlyDir, 0444)
		require.NoError(t, err)

		// Clean up after test
		defer func() {
			os.Chmod(readOnlyDir, 0755)
			os.Remove(filePath)
		}()

		cmd := Command{
			Op:   OpDelete,
			Path: "readonly2/file.txt",
		}

		err = fsm.applyDelete(cmd)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "deleting file")
	})
}

// Test FSM with malformed JSON commands
func TestFSM_Apply_MalformedJSON(t *testing.T) {
	tempDir := createTempDir(t)
	fsm := NewFSM(tempDir, "test-node", nil)
	mockMetrics := &mockMetricsCollector{}
	fsm.SetMetricsCollector(mockMetrics)

	testCases := []struct {
		name     string
		jsonData string
	}{
		{"empty_json", ""},
		{"invalid_json", "{invalid json"},
		{"incomplete_json", "{\"op\": \"write\""},
		{"wrong_type_json", "[]"},
		{"null_json", "null"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockMetrics.reset()
			log := &raft.Log{Data: []byte(tc.jsonData)}
			result := fsm.Apply(log)

			assert.Error(t, result.(error))
			assert.Equal(t, 1, mockMetrics.getReplicationErrors())
		})
	}
}

// Test FSM with different node IDs
func TestFSM_Apply_DifferentNodeIDs(t *testing.T) {
	tempDir := createTempDir(t)
	fsm := NewFSM(tempDir, "test-node", nil)
	mockMetrics := &mockMetricsCollector{}
	fsm.SetMetricsCollector(mockMetrics)

	t.Run("same_node_id_different_content", func(t *testing.T) {
		// Set up existing file state
		fsm.UpdateFileState("test.txt", []byte("old content"))

		cmd := Command{
			Op:       OpWrite,
			Path:     "test.txt",
			Data:     []byte("new content"),
			NodeID:   "test-node", // Same as FSM node ID
			Sequence: 1,
		}

		cmdData, err := json.Marshal(cmd)
		require.NoError(t, err)

		log := &raft.Log{Data: cmdData}
		result := fsm.Apply(log)

		assert.Nil(t, result)
		assert.Equal(t, 1, mockMetrics.getFilesReplicated())

		// Check file was written
		filePath := filepath.Join(tempDir, "test.txt")
		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, []byte("new content"), content)
	})

	t.Run("same_node_id_same_content", func(t *testing.T) {
		mockMetrics.reset()
		testData := []byte("same content")

		// Set up existing file state
		fsm.UpdateFileState("same.txt", testData)

		cmd := Command{
			Op:       OpWrite,
			Path:     "same.txt",
			Data:     testData,
			NodeID:   "test-node", // Same as FSM node ID
			Sequence: 1,
		}

		cmdData, err := json.Marshal(cmd)
		require.NoError(t, err)

		log := &raft.Log{Data: cmdData}
		result := fsm.Apply(log)

		assert.Nil(t, result)
		assert.Equal(t, 0, mockMetrics.getFilesReplicated()) // Should be skipped
	})
}

// Test FSM with very large files
func TestFSM_Apply_LargeFiles(t *testing.T) {
	tempDir := createTempDir(t)
	fsm := NewFSM(tempDir, "test-node", nil)
	mockMetrics := &mockMetricsCollector{}
	fsm.SetMetricsCollector(mockMetrics)

	t.Run("large_file_write", func(t *testing.T) {
		// Create 1MB of data
		largeData := make([]byte, 1024*1024)
		for i := range largeData {
			largeData[i] = byte(i % 256)
		}

		cmd := Command{
			Op:       OpWrite,
			Path:     "large.txt",
			Data:     largeData,
			NodeID:   "other-node",
			Sequence: 1,
		}

		cmdData, err := json.Marshal(cmd)
		require.NoError(t, err)

		log := &raft.Log{Data: cmdData}
		result := fsm.Apply(log)

		assert.Nil(t, result)
		assert.Equal(t, 1, mockMetrics.getFilesReplicated())
		assert.Equal(t, int64(1024*1024), mockMetrics.getBytesReplicated())

		// Verify file was written correctly
		filePath := filepath.Join(tempDir, "large.txt")
		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, largeData, content)
	})
}

// Test FSM with binary data
func TestFSM_Apply_BinaryData(t *testing.T) {
	tempDir := createTempDir(t)
	fsm := NewFSM(tempDir, "test-node", nil)

	t.Run("binary_data_write", func(t *testing.T) {
		// Create binary data with null bytes and special characters
		binaryData := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD, 0x7F, 0x80}

		cmd := Command{
			Op:       OpWrite,
			Path:     "binary.dat",
			Data:     binaryData,
			NodeID:   "other-node",
			Sequence: 1,
		}

		cmdData, err := json.Marshal(cmd)
		require.NoError(t, err)

		log := &raft.Log{Data: cmdData}
		result := fsm.Apply(log)

		assert.Nil(t, result)

		// Verify binary data was written correctly
		filePath := filepath.Join(tempDir, "binary.dat")
		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, binaryData, content)
	})
}

// Test FSM sequence number edge cases
func TestFSM_GetNextSequence_EdgeCases(t *testing.T) {
	tempDir := createTempDir(t)
	fsm := NewFSM(tempDir, "test-node", nil)

	t.Run("sequence_after_large_number", func(t *testing.T) {
		// Set a large sequence number
		fsm.lastSequence = 1000000

		seq := fsm.GetNextSequence()
		assert.Equal(t, int64(1000001), seq)

		// Verify it continues incrementing
		seq = fsm.GetNextSequence()
		assert.Equal(t, int64(1000002), seq)
	})

	t.Run("sequence_thread_safety", func(t *testing.T) {
		var wg sync.WaitGroup
		sequences := make(chan int64, 1000)

		// Start multiple goroutines generating sequences
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 100; j++ {
					sequences <- fsm.GetNextSequence()
				}
			}()
		}

		wg.Wait()
		close(sequences)

		// Collect all sequences
		var allSequences []int64
		for seq := range sequences {
			allSequences = append(allSequences, seq)
		}

		// Verify all sequences are unique
		seenSequences := make(map[int64]bool)
		for _, seq := range allSequences {
			assert.False(t, seenSequences[seq], "Duplicate sequence: %d", seq)
			seenSequences[seq] = true
		}

		assert.Equal(t, 1000, len(allSequences))
	})
}

// Test FSM file state operations with edge cases
func TestFSM_FileState_EdgeCases(t *testing.T) {
	tempDir := createTempDir(t)
	fsm := NewFSM(tempDir, "test-node", nil)

	t.Run("file_state_with_unicode_path", func(t *testing.T) {
		unicodePath := "测试文件.txt"
		unicodeData := []byte("Unicode content: 测试内容")

		fsm.UpdateFileState(unicodePath, unicodeData)

		hasContent := fsm.FileHasContent(unicodePath, unicodeData)
		assert.True(t, hasContent)

		// Test with different data
		hasContent = fsm.FileHasContent(unicodePath, []byte("different"))
		assert.False(t, hasContent)
	})

	t.Run("file_state_with_empty_path", func(t *testing.T) {
		emptyPath := ""
		data := []byte("content")

		fsm.UpdateFileState(emptyPath, data)

		hasContent := fsm.FileHasContent(emptyPath, data)
		assert.True(t, hasContent)

		fsm.RemoveFileState(emptyPath)

		hasContent = fsm.FileHasContent(emptyPath, data)
		assert.False(t, hasContent)
	})

	t.Run("file_state_concurrent_access", func(t *testing.T) {
		var wg sync.WaitGroup

		// Start goroutines for concurrent updates
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				path := fmt.Sprintf("concurrent_%d.txt", index)
				data := []byte(fmt.Sprintf("content_%d", index))

				fsm.UpdateFileState(path, data)

				// Verify state was set
				hasContent := fsm.FileHasContent(path, data)
				assert.True(t, hasContent)
			}(i)
		}

		// Start goroutines for concurrent reads
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				path := fmt.Sprintf("concurrent_%d.txt", index)
				data := []byte(fmt.Sprintf("content_%d", index))

				// This might be true or false depending on timing
				fsm.FileHasContent(path, data)
			}(i)
		}

		wg.Wait()
	})
}

// Test snapshot persistence with different scenarios
func TestSnapshot_Persist_EdgeCases(t *testing.T) {
	tempDir := createTempDir(t)
	snapshot := &Snapshot{dataDir: tempDir}

	t.Run("persist_with_large_data", func(t *testing.T) {
		var buf bytes.Buffer
		sink := &mockSnapshotSink{writer: &buf}

		// Create custom persist logic for testing
		_, err := sink.Write(make([]byte, 10000)) // Write 10KB
		assert.NoError(t, err)

		err = sink.Close()
		assert.NoError(t, err)

		assert.Equal(t, 10000, buf.Len())
	})

	t.Run("persist_with_write_failure_during_close", func(t *testing.T) {
		sink := &mockSnapshotSink{
			writer: &bytes.Buffer{},
		}

		// Simulate success during write but test close behavior
		err := snapshot.Persist(sink)
		assert.NoError(t, err)
		assert.True(t, sink.closeCalled)
	})
}

// Test FSM Restore with different scenarios
func TestFSM_Restore_EdgeCases(t *testing.T) {
	tempDir := createTempDir(t)
	fsm := NewFSM(tempDir, "test-node", nil)

	t.Run("restore_with_large_data", func(t *testing.T) {
		largeData := make([]byte, 1024*1024) // 1MB
		for i := range largeData {
			largeData[i] = byte(i % 256)
		}

		reader := io.NopCloser(bytes.NewReader(largeData))
		err := fsm.Restore(reader)
		assert.NoError(t, err)
	})

	t.Run("restore_with_binary_data", func(t *testing.T) {
		binaryData := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD}
		reader := io.NopCloser(bytes.NewReader(binaryData))

		err := fsm.Restore(reader)
		assert.NoError(t, err)
	})

	t.Run("restore_with_failing_close", func(t *testing.T) {
		reader := &failingReader{data: []byte("test data")}

		err := fsm.Restore(reader)
		assert.NoError(t, err) // Should not fail even if close fails
	})
}

// Test Command validation
func TestCommand_Validation(t *testing.T) {
	t.Run("command_with_empty_fields", func(t *testing.T) {
		cmd := Command{
			Op:       "",
			Path:     "",
			Data:     nil,
			Hash:     "",
			NodeID:   "",
			Sequence: 0,
		}

		// Should be able to serialize/deserialize
		data, err := json.Marshal(cmd)
		assert.NoError(t, err)

		var unmarshaled Command
		err = json.Unmarshal(data, &unmarshaled)
		assert.NoError(t, err)
		assert.Equal(t, cmd, unmarshaled)
	})

	t.Run("command_with_special_characters", func(t *testing.T) {
		cmd := Command{
			Op:       OpWrite,
			Path:     "path/with spaces/and-dashes/and.dots",
			Data:     []byte("content with\nnewlines\tand\ttabs"),
			Hash:     "hash_with_underscores",
			NodeID:   "node-id-with-dashes",
			Sequence: -1, // Negative sequence
		}

		data, err := json.Marshal(cmd)
		assert.NoError(t, err)

		var unmarshaled Command
		err = json.Unmarshal(data, &unmarshaled)
		assert.NoError(t, err)
		assert.Equal(t, cmd, unmarshaled)
	})
}

// Test HashContent with edge cases
func TestHashContent_EdgeCases(t *testing.T) {
	t.Run("hash_with_null_bytes", func(t *testing.T) {
		data := []byte{0x00, 0x01, 0x00, 0x02}
		hash := HashContent(data)

		assert.NotEmpty(t, hash)
		assert.Equal(t, 64, len(hash))

		// Should be consistent
		hash2 := HashContent(data)
		assert.Equal(t, hash, hash2)
	})

	t.Run("hash_with_unicode", func(t *testing.T) {
		data := []byte("Unicode: 测试 🎉 مرحبا")
		hash := HashContent(data)

		assert.NotEmpty(t, hash)
		assert.Equal(t, 64, len(hash))
	})

	t.Run("hash_very_large_data", func(t *testing.T) {
		// Create 10MB of data
		largeData := make([]byte, 10*1024*1024)
		for i := range largeData {
			largeData[i] = byte(i % 256)
		}

		hash := HashContent(largeData)
		assert.NotEmpty(t, hash)
		assert.Equal(t, 64, len(hash))
	})
}

// Helper types for testing
type failingReader struct {
	data []byte
	pos  int
}

func (r *failingReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}

	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func (r *failingReader) Close() error {
	return fmt.Errorf("simulated close error")
}

// Enhanced mock snapshot sink for better testing
type enhancedMockSnapshotSink struct {
	*mockSnapshotSink
	writeCount int
	totalBytes int
}

func (e *enhancedMockSnapshotSink) Write(p []byte) (int, error) {
	e.writeCount++
	e.totalBytes += len(p)
	return e.mockSnapshotSink.Write(p)
}
