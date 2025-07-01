// Package replication implements the finite state machine for distributed file replication.
package replication

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hashicorp/raft"
	"github.com/sirupsen/logrus"
)

const (
	// File operations
	OpWrite  = "write"
	OpDelete = "delete"

	// File permissions
	DirPerm  = 0755
	FilePerm = 0644
)

// Command represents a file operation with enhanced metadata for deduplication.
type Command struct {
	Op       string `json:"op"`       // Operation type: "write" or "delete"
	Path     string `json:"path"`     // Relative file path
	Data     []byte `json:"data"`     // File content (for write operations)
	Hash     string `json:"hash"`     // SHA-256 content hash for deduplication
	NodeID   string `json:"node_id"`  // Originating node ID
	Sequence int64  `json:"sequence"` // Sequence number for ordering
}

// FileState tracks file metadata to prevent unnecessary operations.
type FileState struct {
	Hash         string
	LastModified time.Time
	Size         int64
}

// FSM implements the Raft finite state machine with conflict resolution.
type FSM struct {
	dataDir         string
	nodeID          string
	fileStates      map[string]*FileState
	fileStatesMutex sync.RWMutex
	lastSequence    int64
	sequenceMutex   sync.Mutex
	logger          *logrus.Logger

	// Interfaces for external dependencies
	watcher FileWatcher
	metrics MetricsCollector
}

// FileWatcher defines the interface for file watching control.
type FileWatcher interface {
	PauseWatching()
	ResumeWatching()
}

// MetricsCollector defines the interface for metrics collection.
type MetricsCollector interface {
	IncrementFilesReplicated()
	IncrementFilesDeleted()
	AddBytesReplicated(bytes int64)
	IncrementReplicationErrors()
}

// Snapshot implements raft.FSMSnapshot for state persistence.
type Snapshot struct {
	dataDir string
}

// NewFSM creates a new FSM instance.
func NewFSM(dataDir, nodeID string, logger *logrus.Logger) *FSM {
	if logger == nil {
		logger = logrus.New()
	}

	return &FSM{
		dataDir:    dataDir,
		nodeID:     nodeID,
		fileStates: make(map[string]*FileState),
		logger:     logger,
	}
}

// SetFileWatcher sets the file watcher interface.
func (fsm *FSM) SetFileWatcher(watcher FileWatcher) {
	fsm.watcher = watcher
}

// SetMetricsCollector sets the metrics collector interface.
func (fsm *FSM) SetMetricsCollector(metrics MetricsCollector) {
	fsm.metrics = metrics
}

// Apply executes commands on the finite state machine.
func (fsm *FSM) Apply(log *raft.Log) interface{} {
	var cmd Command
	if err := json.Unmarshal(log.Data, &cmd); err != nil {
		fsm.logger.WithError(err).Error("Failed to unmarshal command")
		if fsm.metrics != nil {
			fsm.metrics.IncrementReplicationErrors()
		}
		return fmt.Errorf("unmarshaling command: %w", err)
	}

	// Skip if this command originated from the current node and content matches
	if cmd.NodeID == fsm.nodeID && fsm.FileHasContent(cmd.Path, cmd.Data) {
		return nil // Avoid infinite loops
	}

	// Temporarily disable file watching during application
	if fsm.watcher != nil {
		fsm.watcher.PauseWatching()
		defer fsm.watcher.ResumeWatching()
	}

	switch cmd.Op {
	case OpWrite:
		if err := fsm.applyWrite(cmd); err != nil {
			if fsm.metrics != nil {
				fsm.metrics.IncrementReplicationErrors()
			}
			return err
		}
		if fsm.metrics != nil {
			fsm.metrics.IncrementFilesReplicated()
			fsm.metrics.AddBytesReplicated(int64(len(cmd.Data)))
		}
		return nil
	case OpDelete:
		if err := fsm.applyDelete(cmd); err != nil {
			if fsm.metrics != nil {
				fsm.metrics.IncrementReplicationErrors()
			}
			return err
		}
		if fsm.metrics != nil {
			fsm.metrics.IncrementFilesDeleted()
		}
		return nil
	default:
		fsm.logger.WithField("operation", cmd.Op).Error("Unknown operation")
		if fsm.metrics != nil {
			fsm.metrics.IncrementReplicationErrors()
		}
		return fmt.Errorf("unknown operation: %q", cmd.Op)
	}
}

// applyWrite handles file write operations.
func (fsm *FSM) applyWrite(cmd Command) error {
	filePath := filepath.Join(fsm.dataDir, cmd.Path)

	// Check if content already matches to avoid unnecessary writes
	if fsm.FileHasContent(cmd.Path, cmd.Data) {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(filePath), DirPerm); err != nil {
		return fmt.Errorf("creating directory for %q: %w", cmd.Path, err)
	}

	if err := os.WriteFile(filePath, cmd.Data, FilePerm); err != nil {
		return fmt.Errorf("writing file %q: %w", cmd.Path, err)
	}

	fsm.UpdateFileState(cmd.Path, cmd.Data)
	fsm.logger.Infof("✓ Replicated: %s (%d bytes) from %s", cmd.Path, len(cmd.Data), cmd.NodeID)
	return nil
}

// applyDelete handles file deletion operations.
func (fsm *FSM) applyDelete(cmd Command) error {
	filePath := filepath.Join(fsm.dataDir, cmd.Path)

	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("deleting file %q: %w", cmd.Path, err)
	}

	fsm.RemoveFileState(cmd.Path)
	fsm.logger.Infof("✓ Deleted: %s from %s", cmd.Path, cmd.NodeID)
	return nil
}

// FileHasContent checks if a file already has the expected content.
func (fsm *FSM) FileHasContent(path string, expectedData []byte) bool {
	fsm.fileStatesMutex.RLock()
	defer fsm.fileStatesMutex.RUnlock()

	state, exists := fsm.fileStates[path]
	if !exists {
		return false
	}

	expectedHash := HashContent(expectedData)
	return state.Hash == expectedHash
}

// UpdateFileState records the current state of a file.
func (fsm *FSM) UpdateFileState(path string, data []byte) {
	fsm.fileStatesMutex.Lock()
	defer fsm.fileStatesMutex.Unlock()

	fsm.fileStates[path] = &FileState{
		Hash:         HashContent(data),
		LastModified: time.Now(),
		Size:         int64(len(data)),
	}
}

// RemoveFileState removes tracking for a deleted file.
func (fsm *FSM) RemoveFileState(path string) {
	fsm.fileStatesMutex.Lock()
	defer fsm.fileStatesMutex.Unlock()

	delete(fsm.fileStates, path)
}

// GetNextSequence returns a monotonically increasing sequence number.
func (fsm *FSM) GetNextSequence() int64 {
	fsm.sequenceMutex.Lock()
	defer fsm.sequenceMutex.Unlock()
	fsm.lastSequence++
	return fsm.lastSequence
}

// Snapshot creates a point-in-time snapshot of the FSM state.
func (fsm *FSM) Snapshot() (raft.FSMSnapshot, error) {
	return &Snapshot{dataDir: fsm.dataDir}, nil
}

// Restore reconstructs the FSM state from a snapshot.
func (fsm *FSM) Restore(rc io.ReadCloser) error {
	defer func() {
		if err := rc.Close(); err != nil {
			fsm.logger.WithError(err).Warn("Error closing restore reader")
		}
	}()

	fsm.logger.Info("Restoring from snapshot")
	return nil
}

// Persist writes the snapshot to persistent storage.
func (s *Snapshot) Persist(sink raft.SnapshotSink) error {
	defer func() {
		if err := sink.Close(); err != nil {
			log.Printf("Error closing snapshot sink: %v", err)
		}
	}()

	if _, err := sink.Write([]byte("snapshot")); err != nil {
		if cancelErr := sink.Cancel(); cancelErr != nil {
			log.Printf("Error canceling snapshot: %v", cancelErr)
		}
		return fmt.Errorf("writing snapshot: %w", err)
	}

	return nil
}

// Release is called when the snapshot is no longer needed.
func (s *Snapshot) Release() {
	// No resources to clean up
}

// HashContent computes the SHA-256 hash of data.
func HashContent(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
