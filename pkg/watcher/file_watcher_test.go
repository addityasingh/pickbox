package watcher

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/hashicorp/raft"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock RaftApplier for testing
type mockRaftApplier struct {
	state       raft.RaftState
	leader      raft.ServerAddress
	applyErr    error
	applyResult interface{}
}

func newMockRaftApplier() *mockRaftApplier {
	return &mockRaftApplier{
		state:  raft.Leader,
		leader: "127.0.0.1:8000",
	}
}

func (m *mockRaftApplier) Apply(data []byte, timeout time.Duration) raft.ApplyFuture {
	return &mockApplyFuture{
		err:    m.applyErr,
		result: m.applyResult,
	}
}

func (m *mockRaftApplier) State() raft.RaftState {
	return m.state
}

func (m *mockRaftApplier) Leader() raft.ServerAddress {
	return m.leader
}

func (m *mockRaftApplier) setState(state raft.RaftState) {
	m.state = state
}

func (m *mockRaftApplier) setLeader(leader raft.ServerAddress) {
	m.leader = leader
}

func (m *mockRaftApplier) setApplyError(err error) {
	m.applyErr = err
}

func (m *mockRaftApplier) setApplyResult(result interface{}) {
	m.applyResult = result
}

// Mock ApplyFuture for testing
type mockApplyFuture struct {
	err    error
	result interface{}
}

func (f *mockApplyFuture) Error() error {
	return f.err
}

func (f *mockApplyFuture) Response() interface{} {
	return f.result
}

func (f *mockApplyFuture) Index() uint64 {
	return 0
}

// Mock LeaderForwarder for testing
type mockLeaderForwarder struct {
	forwardErr error
	lastCmd    Command
}

func newMockLeaderForwarder() *mockLeaderForwarder {
	return &mockLeaderForwarder{}
}

func (m *mockLeaderForwarder) ForwardToLeader(leaderAddr string, cmd Command) error {
	m.lastCmd = cmd
	return m.forwardErr
}

func (m *mockLeaderForwarder) setForwardError(err error) {
	m.forwardErr = err
}

func (m *mockLeaderForwarder) getLastCommand() Command {
	return m.lastCmd
}

// Helper function to create a temporary directory
func createTempDir(t *testing.T) string {
	tempDir, err := os.MkdirTemp("", "watcher_test_")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(tempDir) })
	return tempDir
}

// Helper function to create a logger that doesn't output during tests
func createTestLogger() *logrus.Logger {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel) // Suppress logs during tests
	return logger
}

func TestConfig_Fields(t *testing.T) {
	logger := createTestLogger()
	config := Config{
		DataDir:      "/tmp/test",
		NodeID:       "test-node",
		Logger:       logger,
		ApplyTimeout: 5 * time.Second,
	}

	// Test that all fields are accessible
	assert.Equal(t, "/tmp/test", config.DataDir)
	assert.Equal(t, "test-node", config.NodeID)
	assert.Equal(t, logger, config.Logger)
	assert.Equal(t, 5*time.Second, config.ApplyTimeout)
}

func TestDefaultStateManager_Creation(t *testing.T) {
	sm := NewDefaultStateManager()
	assert.NotNil(t, sm)
}

func TestDefaultStateManager_Operations(t *testing.T) {
	sm := NewDefaultStateManager()

	// Test setting and getting file state
	testData := []byte("test content")
	sm.UpdateFileState("test.txt", testData)

	hasContent := sm.FileHasContent("test.txt", testData)
	assert.True(t, hasContent)

	// Test with different content
	differentData := []byte("different content")
	hasContent = sm.FileHasContent("test.txt", differentData)
	assert.False(t, hasContent)

	// Test removing file state
	sm.RemoveFileState("test.txt")
	hasContent = sm.FileHasContent("test.txt", testData)
	assert.False(t, hasContent)
}

func TestCommand_Structure(t *testing.T) {
	cmd := Command{
		Op:       "write",
		Path:     "test.txt",
		Data:     []byte("test content"),
		Hash:     "abc123",
		NodeID:   "test-node",
		Sequence: 1,
	}

	// Test that all fields are accessible
	assert.Equal(t, "write", cmd.Op)
	assert.Equal(t, "test.txt", cmd.Path)
	assert.Equal(t, []byte("test content"), cmd.Data)
	assert.Equal(t, "abc123", cmd.Hash)
	assert.Equal(t, "test-node", cmd.NodeID)
	assert.Equal(t, int64(1), cmd.Sequence)
}

// Test NewFileWatcher function
func TestNewFileWatcher(t *testing.T) {
	tempDir := createTempDir(t)
	logger := createTestLogger()
	mockRaft := newMockRaftApplier()
	mockStateManager := NewDefaultStateManager()
	mockForwarder := newMockLeaderForwarder()

	t.Run("valid_file_watcher_creation", func(t *testing.T) {
		config := Config{
			DataDir:      tempDir,
			NodeID:       "test-node",
			Logger:       logger,
			ApplyTimeout: 5 * time.Second,
		}

		fw, err := NewFileWatcher(config, mockRaft, mockStateManager, mockForwarder)

		assert.NoError(t, err)
		assert.NotNil(t, fw)
		assert.Equal(t, tempDir, fw.dataDir)
		assert.Equal(t, "test-node", fw.nodeID)
		assert.Equal(t, logger, fw.logger)
		assert.Equal(t, 5*time.Second, fw.applyTimeout)
		assert.NotNil(t, fw.watcher)
		assert.Equal(t, mockRaft, fw.raft)
		assert.Equal(t, mockStateManager, fw.stateManager)
		assert.Equal(t, mockForwarder, fw.forwarder)
	})

	t.Run("empty_data_dir", func(t *testing.T) {
		config := Config{
			DataDir: "",
			NodeID:  "test-node",
			Logger:  logger,
		}

		fw, err := NewFileWatcher(config, mockRaft, mockStateManager, mockForwarder)

		assert.Error(t, err)
		assert.Nil(t, fw)
		assert.Contains(t, err.Error(), "data directory cannot be empty")
	})

	t.Run("empty_node_id", func(t *testing.T) {
		config := Config{
			DataDir: tempDir,
			NodeID:  "",
			Logger:  logger,
		}

		fw, err := NewFileWatcher(config, mockRaft, mockStateManager, mockForwarder)

		assert.Error(t, err)
		assert.Nil(t, fw)
		assert.Contains(t, err.Error(), "node ID cannot be empty")
	})

	t.Run("nil_logger", func(t *testing.T) {
		config := Config{
			DataDir: tempDir,
			NodeID:  "test-node",
			Logger:  nil,
		}

		fw, err := NewFileWatcher(config, mockRaft, mockStateManager, mockForwarder)

		assert.NoError(t, err)
		assert.NotNil(t, fw)
		assert.NotNil(t, fw.logger)
	})

	t.Run("zero_apply_timeout", func(t *testing.T) {
		config := Config{
			DataDir:      tempDir,
			NodeID:       "test-node",
			Logger:       logger,
			ApplyTimeout: 0,
		}

		fw, err := NewFileWatcher(config, mockRaft, mockStateManager, mockForwarder)

		assert.NoError(t, err)
		assert.NotNil(t, fw)
		assert.Equal(t, 5*time.Second, fw.applyTimeout)
	})

	t.Run("invalid_data_dir", func(t *testing.T) {
		config := Config{
			DataDir: "/nonexistent/path/that/does/not/exist",
			NodeID:  "test-node",
			Logger:  logger,
		}

		fw, err := NewFileWatcher(config, mockRaft, mockStateManager, mockForwarder)

		// Should create FileWatcher but fail when trying to start
		assert.NoError(t, err)
		assert.NotNil(t, fw)

		// Starting should fail
		err = fw.Start()
		assert.Error(t, err)
	})
}

// Test FileWatcher Start method
func TestFileWatcher_Start(t *testing.T) {
	tempDir := createTempDir(t)
	logger := createTestLogger()
	mockRaft := newMockRaftApplier()
	mockStateManager := NewDefaultStateManager()
	mockForwarder := newMockLeaderForwarder()

	t.Run("successful_start", func(t *testing.T) {
		config := Config{
			DataDir: tempDir,
			NodeID:  "test-node",
			Logger:  logger,
		}

		fw, err := NewFileWatcher(config, mockRaft, mockStateManager, mockForwarder)
		require.NoError(t, err)

		err = fw.Start()
		assert.NoError(t, err)

		// Clean up
		fw.Stop()
	})

	t.Run("start_with_invalid_dir", func(t *testing.T) {
		config := Config{
			DataDir: "/nonexistent/path/that/does/not/exist",
			NodeID:  "test-node",
			Logger:  logger,
		}

		fw, err := NewFileWatcher(config, mockRaft, mockStateManager, mockForwarder)
		require.NoError(t, err)

		err = fw.Start()
		assert.Error(t, err)
	})
}

// Test FileWatcher Stop method
func TestFileWatcher_Stop(t *testing.T) {
	tempDir := createTempDir(t)
	logger := createTestLogger()
	mockRaft := newMockRaftApplier()
	mockStateManager := NewDefaultStateManager()
	mockForwarder := newMockLeaderForwarder()

	t.Run("successful_stop", func(t *testing.T) {
		config := Config{
			DataDir: tempDir,
			NodeID:  "test-node",
			Logger:  logger,
		}

		fw, err := NewFileWatcher(config, mockRaft, mockStateManager, mockForwarder)
		require.NoError(t, err)

		err = fw.Start()
		require.NoError(t, err)

		err = fw.Stop()
		assert.NoError(t, err)
	})

	t.Run("stop_without_start", func(t *testing.T) {
		config := Config{
			DataDir: tempDir,
			NodeID:  "test-node",
			Logger:  logger,
		}

		fw, err := NewFileWatcher(config, mockRaft, mockStateManager, mockForwarder)
		require.NoError(t, err)

		err = fw.Stop()
		assert.NoError(t, err)
	})

	t.Run("stop_with_nil_watcher", func(t *testing.T) {
		config := Config{
			DataDir: tempDir,
			NodeID:  "test-node",
			Logger:  logger,
		}

		fw, err := NewFileWatcher(config, mockRaft, mockStateManager, mockForwarder)
		require.NoError(t, err)

		fw.watcher = nil
		err = fw.Stop()
		assert.NoError(t, err)
	})
}

// Test FileWatcher pause/resume functionality
func TestFileWatcher_PauseResumeWatching(t *testing.T) {
	tempDir := createTempDir(t)
	logger := createTestLogger()
	mockRaft := newMockRaftApplier()
	mockStateManager := NewDefaultStateManager()
	mockForwarder := newMockLeaderForwarder()

	config := Config{
		DataDir: tempDir,
		NodeID:  "test-node",
		Logger:  logger,
	}

	fw, err := NewFileWatcher(config, mockRaft, mockStateManager, mockForwarder)
	require.NoError(t, err)

	t.Run("initial_state", func(t *testing.T) {
		assert.False(t, fw.IsWatchingPaused())
	})

	t.Run("pause_watching", func(t *testing.T) {
		fw.PauseWatching()
		assert.True(t, fw.IsWatchingPaused())
	})

	t.Run("resume_watching", func(t *testing.T) {
		fw.ResumeWatching()
		assert.False(t, fw.IsWatchingPaused())
	})

	t.Run("multiple_pause_resume", func(t *testing.T) {
		fw.PauseWatching()
		assert.True(t, fw.IsWatchingPaused())

		fw.PauseWatching()
		assert.True(t, fw.IsWatchingPaused())

		fw.ResumeWatching()
		assert.False(t, fw.IsWatchingPaused())
	})
}

// Test FileWatcher handleFileEvent method
func TestFileWatcher_handleFileEvent(t *testing.T) {
	tempDir := createTempDir(t)
	logger := createTestLogger()
	mockRaft := newMockRaftApplier()
	mockStateManager := NewDefaultStateManager()
	mockForwarder := newMockLeaderForwarder()

	config := Config{
		DataDir: tempDir,
		NodeID:  "test-node",
		Logger:  logger,
	}

	fw, err := NewFileWatcher(config, mockRaft, mockStateManager, mockForwarder)
	require.NoError(t, err)

	t.Run("skip_when_paused", func(t *testing.T) {
		fw.PauseWatching()

		testFile := filepath.Join(tempDir, "test.txt")
		event := fsnotify.Event{
			Name: testFile,
			Op:   fsnotify.Write,
		}

		// This should not process the event
		assert.NotPanics(t, func() {
			fw.handleFileEvent(event)
		})

		fw.ResumeWatching()
	})

	t.Run("skip_raft_files", func(t *testing.T) {
		raftFile := filepath.Join(tempDir, "raft-log.db")
		event := fsnotify.Event{
			Name: raftFile,
			Op:   fsnotify.Write,
		}

		// This should not process the event
		assert.NotPanics(t, func() {
			fw.handleFileEvent(event)
		})
	})

	t.Run("handle_write_event", func(t *testing.T) {
		testFile := filepath.Join(tempDir, "test.txt")

		// Create the file first
		err := os.WriteFile(testFile, []byte("test content"), 0644)
		require.NoError(t, err)

		event := fsnotify.Event{
			Name: testFile,
			Op:   fsnotify.Write,
		}

		// This should process the event
		assert.NotPanics(t, func() {
			fw.handleFileEvent(event)
		})
	})

	t.Run("handle_create_event", func(t *testing.T) {
		testFile := filepath.Join(tempDir, "create_test.txt")

		// Create the file first
		err := os.WriteFile(testFile, []byte("create content"), 0644)
		require.NoError(t, err)

		event := fsnotify.Event{
			Name: testFile,
			Op:   fsnotify.Create,
		}

		// This should process the event
		assert.NotPanics(t, func() {
			fw.handleFileEvent(event)
		})
	})

	t.Run("skip_directory_events", func(t *testing.T) {
		testDir := filepath.Join(tempDir, "testdir")
		err := os.Mkdir(testDir, 0755)
		require.NoError(t, err)

		event := fsnotify.Event{
			Name: testDir,
			Op:   fsnotify.Write,
		}

		// This should not process the event
		assert.NotPanics(t, func() {
			fw.handleFileEvent(event)
		})
	})

	t.Run("skip_nonexistent_files", func(t *testing.T) {
		nonExistentFile := filepath.Join(tempDir, "nonexistent.txt")
		event := fsnotify.Event{
			Name: nonExistentFile,
			Op:   fsnotify.Write,
		}

		// This should not process the event
		assert.NotPanics(t, func() {
			fw.handleFileEvent(event)
		})
	})
}

// Test FileWatcher handleFileWrite method
func TestFileWatcher_handleFileWrite(t *testing.T) {
	tempDir := createTempDir(t)
	logger := createTestLogger()
	mockRaft := newMockRaftApplier()
	mockStateManager := NewDefaultStateManager()
	mockForwarder := newMockLeaderForwarder()

	config := Config{
		DataDir: tempDir,
		NodeID:  "test-node",
		Logger:  logger,
	}

	fw, err := NewFileWatcher(config, mockRaft, mockStateManager, mockForwarder)
	require.NoError(t, err)

	t.Run("handle_write_as_leader", func(t *testing.T) {
		mockRaft.setState(raft.Leader)

		testFile := filepath.Join(tempDir, "leader_test.txt")
		err := os.WriteFile(testFile, []byte("leader content"), 0644)
		require.NoError(t, err)

		// This should apply the command directly
		assert.NotPanics(t, func() {
			fw.handleFileWrite(testFile)
		})
	})

	t.Run("handle_write_as_follower", func(t *testing.T) {
		mockRaft.setState(raft.Follower)
		mockRaft.setLeader("127.0.0.1:8000")

		testFile := filepath.Join(tempDir, "follower_test.txt")
		err := os.WriteFile(testFile, []byte("follower content"), 0644)
		require.NoError(t, err)

		// This should forward the command to leader
		assert.NotPanics(t, func() {
			fw.handleFileWrite(testFile)
		})

		// Verify the command was forwarded
		lastCmd := mockForwarder.getLastCommand()
		assert.Equal(t, OpWrite, lastCmd.Op)
		assert.Equal(t, "follower_test.txt", lastCmd.Path)
		assert.Equal(t, []byte("follower content"), lastCmd.Data)
	})

	t.Run("skip_unchanged_content", func(t *testing.T) {
		testFile := filepath.Join(tempDir, "unchanged_test.txt")
		testData := []byte("unchanged content")

		err := os.WriteFile(testFile, testData, 0644)
		require.NoError(t, err)

		// Set up state manager to indicate file already has this content
		mockStateManager.UpdateFileState("unchanged_test.txt", testData)

		// This should not trigger replication
		assert.NotPanics(t, func() {
			fw.handleFileWrite(testFile)
		})
	})

	t.Run("handle_file_outside_data_dir", func(t *testing.T) {
		// Create a file outside the data directory
		outsideFile := filepath.Join(os.TempDir(), "outside_test.txt")
		err := os.WriteFile(outsideFile, []byte("outside content"), 0644)
		require.NoError(t, err)
		t.Cleanup(func() { os.Remove(outsideFile) })

		// This should not panic but should return early
		assert.NotPanics(t, func() {
			fw.handleFileWrite(outsideFile)
		})
	})

	t.Run("handle_nonexistent_file", func(t *testing.T) {
		nonExistentFile := filepath.Join(tempDir, "nonexistent.txt")

		// This should not panic but should return early
		assert.NotPanics(t, func() {
			fw.handleFileWrite(nonExistentFile)
		})
	})
}

// Test FileWatcher applyAsLeader method
func TestFileWatcher_applyAsLeader(t *testing.T) {
	tempDir := createTempDir(t)
	logger := createTestLogger()
	mockRaft := newMockRaftApplier()
	mockStateManager := NewDefaultStateManager()
	mockForwarder := newMockLeaderForwarder()

	config := Config{
		DataDir: tempDir,
		NodeID:  "test-node",
		Logger:  logger,
	}

	fw, err := NewFileWatcher(config, mockRaft, mockStateManager, mockForwarder)
	require.NoError(t, err)

	t.Run("successful_apply", func(t *testing.T) {
		cmd := Command{
			Op:       OpWrite,
			Path:     "apply_test.txt",
			Data:     []byte("apply content"),
			NodeID:   "test-node",
			Sequence: 1,
		}

		cmdData, err := json.Marshal(cmd)
		require.NoError(t, err)

		err = fw.applyAsLeader(cmdData, "apply_test.txt")
		assert.NoError(t, err)
	})

	t.Run("apply_with_error", func(t *testing.T) {
		mockRaft.setApplyError(fmt.Errorf("apply error"))

		cmd := Command{
			Op:       OpWrite,
			Path:     "error_test.txt",
			Data:     []byte("error content"),
			NodeID:   "test-node",
			Sequence: 1,
		}

		cmdData, err := json.Marshal(cmd)
		require.NoError(t, err)

		err = fw.applyAsLeader(cmdData, "error_test.txt")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "apply error")

		// Reset for other tests
		mockRaft.setApplyError(nil)
	})
}

// Test FileWatcher forwardToLeader method
func TestFileWatcher_forwardToLeader(t *testing.T) {
	tempDir := createTempDir(t)
	logger := createTestLogger()
	mockRaft := newMockRaftApplier()
	mockStateManager := NewDefaultStateManager()
	mockForwarder := newMockLeaderForwarder()

	config := Config{
		DataDir: tempDir,
		NodeID:  "test-node",
		Logger:  logger,
	}

	fw, err := NewFileWatcher(config, mockRaft, mockStateManager, mockForwarder)
	require.NoError(t, err)

	t.Run("successful_forward", func(t *testing.T) {
		mockRaft.setLeader("127.0.0.1:8000")

		cmd := Command{
			Op:       OpWrite,
			Path:     "forward_test.txt",
			Data:     []byte("forward content"),
			NodeID:   "test-node",
			Sequence: 1,
		}

		assert.NotPanics(t, func() {
			fw.forwardToLeader(cmd, "forward_test.txt")
		})

		// Verify the command was forwarded
		lastCmd := mockForwarder.getLastCommand()
		assert.Equal(t, cmd.Op, lastCmd.Op)
		assert.Equal(t, cmd.Path, lastCmd.Path)
		assert.Equal(t, cmd.Data, lastCmd.Data)
	})

	t.Run("forward_with_no_leader", func(t *testing.T) {
		mockRaft.setLeader("")

		cmd := Command{
			Op:       OpWrite,
			Path:     "no_leader_test.txt",
			Data:     []byte("no leader content"),
			NodeID:   "test-node",
			Sequence: 1,
		}

		assert.NotPanics(t, func() {
			fw.forwardToLeader(cmd, "no_leader_test.txt")
		})
	})

	t.Run("forward_with_error", func(t *testing.T) {
		mockRaft.setLeader("127.0.0.1:8000")
		mockForwarder.setForwardError(fmt.Errorf("forward error"))

		cmd := Command{
			Op:       OpWrite,
			Path:     "error_forward_test.txt",
			Data:     []byte("error forward content"),
			NodeID:   "test-node",
			Sequence: 1,
		}

		assert.NotPanics(t, func() {
			fw.forwardToLeader(cmd, "error_forward_test.txt")
		})

		// Reset for other tests
		mockForwarder.setForwardError(nil)
	})
}

// Test IsRaftFile utility function
func TestIsRaftFile(t *testing.T) {
	testCases := []struct {
		filename string
		expected bool
	}{
		{"raft-log.db", true},
		{"raft-snapshot.db", true},
		{"data.db", true},
		{"snapshots", true},
		{"data/snapshots/snapshot.db", true},
		{"regular.txt", false},
		{"data.txt", false},
		{"normal/file.txt", false},
		{"my-raft-file.txt", false},
		{"file.raft", false},
	}

	for _, tc := range testCases {
		t.Run(tc.filename, func(t *testing.T) {
			result := IsRaftFile(tc.filename)
			assert.Equal(t, tc.expected, result, "IsRaftFile(%q) should be %v", tc.filename, tc.expected)
		})
	}
}

// Test FileState structure
func TestFileState_Fields(t *testing.T) {
	now := time.Now()
	state := FileState{
		Hash:         "abc123",
		LastModified: now,
		Size:         1024,
	}

	assert.Equal(t, "abc123", state.Hash)
	assert.Equal(t, now, state.LastModified)
	assert.Equal(t, int64(1024), state.Size)
}

// Test Command JSON serialization
func TestCommand_JSONSerialization(t *testing.T) {
	cmd := Command{
		Op:       OpWrite,
		Path:     "test.txt",
		Data:     []byte("test content"),
		Hash:     "abc123",
		NodeID:   "test-node",
		Sequence: 1,
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

// Test DefaultStateManager additional methods
func TestDefaultStateManager_AdditionalMethods(t *testing.T) {
	sm := NewDefaultStateManager()

	t.Run("get_file_states", func(t *testing.T) {
		testData1 := []byte("test content 1")
		testData2 := []byte("test content 2")

		sm.UpdateFileState("test1.txt", testData1)
		sm.UpdateFileState("test2.txt", testData2)

		states := sm.GetFileStates()
		assert.Len(t, states, 2)

		assert.Contains(t, states, "test1.txt")
		assert.Contains(t, states, "test2.txt")

		assert.Equal(t, HashContent(testData1), states["test1.txt"].Hash)
		assert.Equal(t, HashContent(testData2), states["test2.txt"].Hash)
	})

	t.Run("get_file_count", func(t *testing.T) {
		sm.UpdateFileState("count1.txt", []byte("content1"))
		sm.UpdateFileState("count2.txt", []byte("content2"))
		sm.UpdateFileState("count3.txt", []byte("content3"))

		count := sm.GetFileCount()
		assert.Equal(t, 5, count) // 3 new + 2 from previous test
	})

	t.Run("get_next_sequence", func(t *testing.T) {
		seq1 := sm.GetNextSequence()
		seq2 := sm.GetNextSequence()
		seq3 := sm.GetNextSequence()

		assert.Greater(t, seq2, seq1)
		assert.Greater(t, seq3, seq2)
		assert.Equal(t, seq1+1, seq2)
		assert.Equal(t, seq2+1, seq3)
	})
}

// Test constants
func TestConstants(t *testing.T) {
	assert.Equal(t, "write", OpWrite)
	assert.Equal(t, "delete", OpDelete)
	assert.Equal(t, 50*time.Millisecond, defaultWatchDelay)
	assert.Equal(t, 200*time.Millisecond, defaultPauseDelay)
}

// Benchmark tests
func BenchmarkFileWatcher_NewFileWatcher(b *testing.B) {
	tempDir, _ := os.MkdirTemp("", "benchmark_")
	defer os.RemoveAll(tempDir)

	logger := createTestLogger()
	mockRaft := newMockRaftApplier()
	mockStateManager := NewDefaultStateManager()
	mockForwarder := newMockLeaderForwarder()

	config := Config{
		DataDir: tempDir,
		NodeID:  "benchmark-node",
		Logger:  logger,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fw, _ := NewFileWatcher(config, mockRaft, mockStateManager, mockForwarder)
		fw.Stop()
	}
}

func BenchmarkFileWatcher_handleFileWrite(b *testing.B) {
	tempDir, _ := os.MkdirTemp("", "benchmark_")
	defer os.RemoveAll(tempDir)

	logger := createTestLogger()
	mockRaft := newMockRaftApplier()
	mockStateManager := NewDefaultStateManager()
	mockForwarder := newMockLeaderForwarder()

	config := Config{
		DataDir: tempDir,
		NodeID:  "benchmark-node",
		Logger:  logger,
	}

	fw, _ := NewFileWatcher(config, mockRaft, mockStateManager, mockForwarder)
	defer fw.Stop()

	// Create test file
	testFile := filepath.Join(tempDir, "benchmark.txt")
	os.WriteFile(testFile, []byte("benchmark content"), 0644)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fw.handleFileWrite(testFile)
	}
}

func BenchmarkIsRaftFile(b *testing.B) {
	filenames := []string{
		"raft-log.db",
		"regular.txt",
		"data.db",
		"snapshots",
		"normal/file.txt",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, filename := range filenames {
			IsRaftFile(filename)
		}
	}
}
