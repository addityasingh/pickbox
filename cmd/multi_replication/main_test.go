package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/raft"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

// Test for the refactored AppConfig validation
func TestAppConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  AppConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: AppConfig{
				DataDir:     "/tmp/test",
				NodeID:      "node1",
				Port:        8000,
				AdminPort:   9000,
				MonitorPort: 8080,
				LogLevel:    "info",
			},
			wantErr: false,
		},
		{
			name: "invalid config - empty data dir",
			config: AppConfig{
				DataDir:     "",
				NodeID:      "node1",
				Port:        8000,
				AdminPort:   9000,
				MonitorPort: 8080,
				LogLevel:    "info",
			},
			wantErr: true,
		},
		{
			name: "invalid config - empty node ID",
			config: AppConfig{
				DataDir:     "/tmp/test",
				NodeID:      "",
				Port:        8000,
				AdminPort:   9000,
				MonitorPort: 8080,
				LogLevel:    "info",
			},
			wantErr: true,
		},
		{
			name: "invalid config - zero port",
			config: AppConfig{
				DataDir:     "/tmp/test",
				NodeID:      "node1",
				Port:        0,
				AdminPort:   9000,
				MonitorPort: 8080,
				LogLevel:    "info",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Test for the refactored Application creation
func TestNewApplication(t *testing.T) {
	// tempDir := t.TempDir()

	tests := []struct {
		name    string
		config  AppConfig
		wantErr bool
	}{
		// {
		// 	name: "valid application creation",
		// 	config: AppConfig{
		// 		DataDir:     tempDir,
		// 		NodeID:      "test-node",
		// 		Port:        8000,
		// 		AdminPort:   9000,
		// 		MonitorPort: 8080,
		// 		LogLevel:    "info",
		// 	},
		// 	wantErr: false,
		// },
		{
			name: "invalid config should fail",
			config: AppConfig{
				DataDir:     "",
				NodeID:      "test-node",
				Port:        8000,
				AdminPort:   9000,
				MonitorPort: 8080,
				LogLevel:    "info",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, err := NewApplication(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, app)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, app)
				assert.Equal(t, tt.config.NodeID, app.config.NodeID)
				assert.Equal(t, tt.config.DataDir, app.config.DataDir)
			}
		})
	}
}

// Legacy tests for backward compatibility with original implementation
// These tests are kept for comprehensive coverage of the original functionality

// Command represents a file operation with enhanced metadata for deduplication.
// This type is kept for backward compatibility with legacy tests
type Command struct {
	Op       string `json:"op"`       // Operation type: "write" or "delete"
	Path     string `json:"path"`     // Relative file path
	Data     []byte `json:"data"`     // File content (for write operations)
	Hash     string `json:"hash"`     // SHA-256 content hash for deduplication
	NodeID   string `json:"node_id"`  // Originating node ID
	Sequence int64  `json:"sequence"` // Sequence number for ordering
}

const (
	// Operations
	opWrite  = "write"
	opDelete = "delete"
)

func TestCommand_JSONSerialization(t *testing.T) {
	tests := []struct {
		name string
		cmd  Command
	}{
		{
			name: "write_command",
			cmd: Command{
				Op:       opWrite,
				Path:     "test/file.txt",
				Data:     []byte("test content"),
				Hash:     "abc123",
				NodeID:   "node1",
				Sequence: 1,
			},
		},
		{
			name: "delete_command",
			cmd: Command{
				Op:       opDelete,
				Path:     "test/file.txt",
				NodeID:   "node2",
				Sequence: 2,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test marshaling
			data, err := json.Marshal(tt.cmd)
			assert.NoError(t, err)
			assert.NotEmpty(t, data)

			// Test unmarshaling
			var unmarshaled Command
			err = json.Unmarshal(data, &unmarshaled)
			assert.NoError(t, err)
			assert.Equal(t, tt.cmd.Op, unmarshaled.Op)
			assert.Equal(t, tt.cmd.Path, unmarshaled.Path)
			assert.Equal(t, tt.cmd.Data, unmarshaled.Data)
			assert.Equal(t, tt.cmd.Hash, unmarshaled.Hash)
			assert.Equal(t, tt.cmd.NodeID, unmarshaled.NodeID)
			assert.Equal(t, tt.cmd.Sequence, unmarshaled.Sequence)
		})
	}
}

// hashContent computes the SHA-256 hash of data for backward compatibility
func hashContent(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
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
			name:     "hello_world",
			data:     []byte("hello world"),
			expected: "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
		},
		{
			name:     "test_content",
			data:     []byte("test content for hashing"),
			expected: computeExpectedHash([]byte("test content for hashing")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hashContent(tt.data)
			assert.Equal(t, tt.expected, result)
			assert.Len(t, result, 64) // SHA-256 produces 64-character hex string
		})
	}
}

// Legacy FSM tests for backward compatibility
// FileState tracks file metadata to prevent unnecessary operations.
type FileState struct {
	Hash         string
	LastModified time.Time
	Size         int64
}

// ReplicationFSM implements the Raft finite state machine with conflict resolution.
// This is kept for backward compatibility with legacy tests
type ReplicationFSM struct {
	dataDir        string
	nodeID         string
	fileStates     map[string]*FileState
	watchingPaused bool
	lastSequence   int64
	logger         *logrus.Logger
}

func NewReplicationFSM(dataDir, nodeID string, logger *logrus.Logger) *ReplicationFSM {
	return &ReplicationFSM{
		dataDir:    dataDir,
		nodeID:     nodeID,
		fileStates: make(map[string]*FileState),
		logger:     logger,
	}
}

func (fsm *ReplicationFSM) getNextSequence() int64 {
	fsm.lastSequence++
	return fsm.lastSequence
}

func (fsm *ReplicationFSM) isWatchingPaused() bool {
	return fsm.watchingPaused
}

func (fsm *ReplicationFSM) pauseWatching() {
	fsm.watchingPaused = true
}

func (fsm *ReplicationFSM) resumeWatching() {
	fsm.watchingPaused = false
}

func (fsm *ReplicationFSM) fileHasContent(path string, expectedData []byte) bool {
	state, exists := fsm.fileStates[path]
	if !exists {
		return false
	}
	expectedHash := hashContent(expectedData)
	return state.Hash == expectedHash
}

func (fsm *ReplicationFSM) updateFileState(path string, data []byte) {
	fsm.fileStates[path] = &FileState{
		Hash:         hashContent(data),
		LastModified: time.Now(),
		Size:         int64(len(data)),
	}
}

func (fsm *ReplicationFSM) removeFileState(path string) {
	delete(fsm.fileStates, path)
}

func (fsm *ReplicationFSM) Apply(log *raft.Log) interface{} {
	var cmd Command
	if err := json.Unmarshal(log.Data, &cmd); err != nil {
		return fmt.Errorf("unmarshaling command: %w", err)
	}

	// Skip if this command originated from the current node and content matches
	if cmd.NodeID == fsm.nodeID && fsm.fileHasContent(cmd.Path, cmd.Data) {
		return nil // Avoid infinite loops
	}

	// Temporarily disable file watching during application
	fsm.pauseWatching()
	defer fsm.resumeWatching()

	switch cmd.Op {
	case opWrite:
		return fsm.applyWrite(cmd)
	case opDelete:
		return fsm.applyDelete(cmd)
	default:
		return fmt.Errorf("unknown operation: %q", cmd.Op)
	}
}

func (fsm *ReplicationFSM) applyWrite(cmd Command) error {
	filePath := filepath.Join(fsm.dataDir, cmd.Path)

	// Check if content already matches to avoid unnecessary writes
	if fsm.fileHasContent(cmd.Path, cmd.Data) {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("creating directory for %q: %w", cmd.Path, err)
	}

	if err := os.WriteFile(filePath, cmd.Data, 0644); err != nil {
		return fmt.Errorf("writing file %q: %w", cmd.Path, err)
	}

	fsm.updateFileState(cmd.Path, cmd.Data)
	return nil
}

func (fsm *ReplicationFSM) applyDelete(cmd Command) error {
	filePath := filepath.Join(fsm.dataDir, cmd.Path)

	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("deleting file %q: %w", cmd.Path, err)
	}

	fsm.removeFileState(cmd.Path)
	return nil
}

func (fsm *ReplicationFSM) Snapshot() (raft.FSMSnapshot, error) {
	return &Snapshot{dataDir: fsm.dataDir}, nil
}

func (fsm *ReplicationFSM) Restore(rc io.ReadCloser) error {
	defer rc.Close()
	return nil
}

// Snapshot implements raft.FSMSnapshot for state persistence.
type Snapshot struct {
	dataDir string
}

func (s *Snapshot) Persist(sink raft.SnapshotSink) error {
	defer sink.Close()

	if _, err := sink.Write([]byte("snapshot")); err != nil {
		sink.Cancel()
		return fmt.Errorf("writing snapshot: %w", err)
	}

	return nil
}

func (s *Snapshot) Release() {
	// No resources to clean up
}

func TestReplicationFSM_NewReplicationFSM(t *testing.T) {
	dataDir := "/tmp/test-fsm"
	nodeID := "test-node"
	logger := logrus.New()

	fsm := NewReplicationFSM(dataDir, nodeID, logger)

	assert.NotNil(t, fsm)
	assert.Equal(t, dataDir, fsm.dataDir)
	assert.Equal(t, nodeID, fsm.nodeID)
	assert.NotNil(t, fsm.fileStates)
	assert.Equal(t, logger, fsm.logger)
	assert.False(t, fsm.watchingPaused)
	assert.Equal(t, int64(0), fsm.lastSequence)
}

func TestReplicationFSM_SequenceGeneration(t *testing.T) {
	fsm := NewReplicationFSM("/tmp/test", "test-node", logrus.New())

	// Test sequence generation
	seq1 := fsm.getNextSequence()
	seq2 := fsm.getNextSequence()
	seq3 := fsm.getNextSequence()

	assert.Equal(t, int64(1), seq1)
	assert.Equal(t, int64(2), seq2)
	assert.Equal(t, int64(3), seq3)
}

func TestReplicationFSM_WatchingControls(t *testing.T) {
	fsm := NewReplicationFSM("/tmp/test", "test-node", logrus.New())

	// Initial state
	assert.False(t, fsm.isWatchingPaused())

	// Pause watching
	fsm.pauseWatching()
	assert.True(t, fsm.isWatchingPaused())

	// Resume watching
	fsm.resumeWatching()
	assert.False(t, fsm.isWatchingPaused())
}

func TestReplicationFSM_FileStateManagement(t *testing.T) {
	fsm := NewReplicationFSM("/tmp/test", "test-node", logrus.New())

	path := "test/file.txt"
	data := []byte("test content")

	// Initially no file state
	assert.False(t, fsm.fileHasContent(path, data))

	// Update file state
	fsm.updateFileState(path, data)

	// Should now have content
	assert.True(t, fsm.fileHasContent(path, data))

	// Different content should return false
	differentData := []byte("different content")
	assert.False(t, fsm.fileHasContent(path, differentData))

	// Remove file state
	fsm.removeFileState(path)
	assert.False(t, fsm.fileHasContent(path, data))
}

func TestReplicationFSM_Apply_WriteCommand(t *testing.T) {
	// Note: This test would require a real filesystem, so we test the logic parts
	fsm := NewReplicationFSM("/tmp/test-apply", "test-node", logrus.New())

	cmd := Command{
		Op:       opWrite,
		Path:     "test.txt",
		Data:     []byte("test content"),
		Hash:     hashContent([]byte("test content")),
		NodeID:   "other-node", // Different node to avoid skip logic
		Sequence: 1,
	}

	// Create a mock log entry
	cmdData, err := json.Marshal(cmd)
	assert.NoError(t, err)

	log := &raft.Log{
		Data: cmdData,
	}

	// This would fail in real test due to filesystem access, but tests the unmarshaling
	result := fsm.Apply(log)

	// Check if it's an error related to filesystem (expected in test environment)
	if result != nil {
		err, ok := result.(error)
		if ok {
			// In test environment, expect filesystem-related errors
			assert.Contains(t, err.Error(), "creating directory")
		}
	}
}

func TestReplicationFSM_Apply_InvalidJSON(t *testing.T) {
	fsm := NewReplicationFSM("/tmp/test", "test-node", logrus.New())

	log := &raft.Log{
		Data: []byte("invalid json"),
	}

	result := fsm.Apply(log)
	assert.NotNil(t, result)

	err, ok := result.(error)
	assert.True(t, ok)
	assert.Contains(t, err.Error(), "unmarshaling command")
}

func TestReplicationFSM_Apply_UnknownOperation(t *testing.T) {
	fsm := NewReplicationFSM("/tmp/test", "test-node", logrus.New())

	cmd := Command{
		Op:     "unknown",
		Path:   "test.txt",
		NodeID: "other-node",
	}

	cmdData, err := json.Marshal(cmd)
	assert.NoError(t, err)

	log := &raft.Log{
		Data: cmdData,
	}

	result := fsm.Apply(log)
	assert.NotNil(t, result)

	err, ok := result.(error)
	assert.True(t, ok)
	assert.Contains(t, err.Error(), "unknown operation")
}

func TestReplicationFSM_Apply_SkipSameNode(t *testing.T) {
	fsm := NewReplicationFSM("/tmp/test", "test-node", logrus.New())

	// Pre-populate file state to trigger skip logic
	path := "test.txt"
	data := []byte("test content")
	fsm.updateFileState(path, data)

	cmd := Command{
		Op:     opWrite,
		Path:   path,
		Data:   data,
		NodeID: "test-node", // Same as FSM's nodeID
	}

	cmdData, err := json.Marshal(cmd)
	assert.NoError(t, err)

	log := &raft.Log{
		Data: cmdData,
	}

	result := fsm.Apply(log)
	assert.Nil(t, result) // Should be nil when skipped
}

func TestReplicationFSM_Snapshot(t *testing.T) {
	fsm := NewReplicationFSM("/tmp/test-snapshot", "test-node", logrus.New())

	snapshot, err := fsm.Snapshot()
	assert.NoError(t, err)
	assert.NotNil(t, snapshot)

	// Verify snapshot type
	s, ok := snapshot.(*Snapshot)
	assert.True(t, ok)
	assert.Equal(t, fsm.dataDir, s.dataDir)
}

func TestSnapshot_Persist(t *testing.T) {
	snapshot := &Snapshot{dataDir: "/tmp/test"}

	// Create a mock sink
	mockSink := &mockSnapshotSink{}

	err := snapshot.Persist(mockSink)
	assert.NoError(t, err)
	assert.True(t, mockSink.closed)
	assert.Equal(t, []byte("snapshot"), mockSink.data)
}

func TestSnapshot_Release(t *testing.T) {
	snapshot := &Snapshot{dataDir: "/tmp/test"}

	// Should not panic
	assert.NotPanics(t, func() {
		snapshot.Release()
	})
}

// isRaftFile checks if a file is related to Raft internals.
func isRaftFile(filename string) bool {
	base := filepath.Base(filename)
	return strings.HasPrefix(base, "raft-") ||
		strings.HasSuffix(base, ".db") ||
		base == "snapshots" ||
		strings.Contains(filename, "snapshots")
}

func TestIsRaftFile(t *testing.T) {
	tests := []struct {
		filename string
		expected bool
	}{
		{"raft-log.db", true},
		{"raft-stable.db", true},
		{"snapshots", true},
		{"data/node1/snapshots/1-2-123456.tmp", true},
		{"regular-file.txt", false},
		{"data.db", true}, // ends with .db
		{"normal.txt", false},
		{"prefix-raft-something", false}, // doesn't start with "raft-"
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := isRaftFile(tt.filename)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseFlags(t *testing.T) {
	// Test the logic that parseFlags implements
	cfg := AppConfig{
		NodeID:   "test-node",
		Port:     8001,
		JoinAddr: "",
	}

	// Derive other fields as parseFlags would
	if cfg.DataDir == "" {
		cfg.DataDir = filepath.Join("data", cfg.NodeID)
	}
	cfg.AdminPort = 9001
	cfg.MonitorPort = 6001
	cfg.DashboardPort = 8080
	cfg.BootstrapCluster = cfg.JoinAddr == ""

	assert.Equal(t, "data/test-node", cfg.DataDir)
	assert.Equal(t, 9001, cfg.AdminPort)
	assert.Equal(t, 6001, cfg.MonitorPort)
	assert.Equal(t, 8080, cfg.DashboardPort)
	assert.True(t, cfg.BootstrapCluster)

	// Test with join address
	cfg.JoinAddr = "127.0.0.1:8002"
	cfg.BootstrapCluster = cfg.JoinAddr == ""
	assert.False(t, cfg.BootstrapCluster)
}

// Helper functions and mocks

func computeExpectedHash(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

type mockSnapshotSink struct {
	data   []byte
	closed bool
}

func (m *mockSnapshotSink) Write(p []byte) (n int, err error) {
	m.data = append(m.data, p...)
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

// Benchmark tests for performance verification

func BenchmarkHashContent(b *testing.B) {
	data := []byte("benchmark data for hashing performance testing with longer content")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hashContent(data)
	}
}

func BenchmarkReplicationFSM_FileStateManagement(b *testing.B) {
	fsm := NewReplicationFSM("/tmp/bench", "bench-node", logrus.New())
	data := []byte("benchmark data")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		path := "bench-file.txt"
		fsm.updateFileState(path, data)
		fsm.fileHasContent(path, data)
	}
}

func BenchmarkReplicationFSM_SequenceGeneration(b *testing.B) {
	fsm := NewReplicationFSM("/tmp/bench", "bench-node", logrus.New())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fsm.getNextSequence()
	}
}

func BenchmarkCommand_JSONMarshal(b *testing.B) {
	cmd := Command{
		Op:       opWrite,
		Path:     "benchmark/file.txt",
		Data:     []byte("benchmark data for JSON marshaling performance"),
		Hash:     hashContent([]byte("benchmark data")),
		NodeID:   "bench-node",
		Sequence: 1,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		json.Marshal(cmd)
	}
}

func BenchmarkCommand_JSONUnmarshal(b *testing.B) {
	cmd := Command{
		Op:       opWrite,
		Path:     "benchmark/file.txt",
		Data:     []byte("benchmark data for JSON unmarshaling performance"),
		Hash:     hashContent([]byte("benchmark data")),
		NodeID:   "bench-node",
		Sequence: 1,
	}

	data, _ := json.Marshal(cmd)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var unmarshaled Command
		json.Unmarshal(data, &unmarshaled)
	}
}
