// Package watcher provides file system monitoring capabilities for distributed replication.
package watcher

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/hashicorp/raft"
	"github.com/sirupsen/logrus"
)

const (
	// Default timeouts and delays
	defaultWatchDelay = 50 * time.Millisecond
	defaultPauseDelay = 200 * time.Millisecond

	// File operations
	OpWrite  = "write"
	OpDelete = "delete"
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

// StateManager manages file state tracking for deduplication.
type StateManager interface {
	FileHasContent(path string, expectedData []byte) bool
	UpdateFileState(path string, data []byte)
	RemoveFileState(path string)
	GetNextSequence() int64
}

// RaftApplier defines the interface for applying Raft operations.
type RaftApplier interface {
	Apply(data []byte, timeout time.Duration) raft.ApplyFuture
	State() raft.RaftState
	Leader() raft.ServerAddress
}

// LeaderForwarder defines the interface for forwarding commands to the leader.
type LeaderForwarder interface {
	ForwardToLeader(leaderAddr string, cmd Command) error
}

// FileWatcher monitors file system changes and triggers replication.
type FileWatcher struct {
	dataDir        string
	nodeID         string
	watcher        *fsnotify.Watcher
	raft           RaftApplier
	stateManager   StateManager
	forwarder      LeaderForwarder
	watchingMutex  sync.RWMutex
	watchingPaused bool
	logger         *logrus.Logger
	applyTimeout   time.Duration
}

// Config holds configuration for the FileWatcher.
type Config struct {
	DataDir      string
	NodeID       string
	Logger       *logrus.Logger
	ApplyTimeout time.Duration
}

// NewFileWatcher creates a new file system watcher.
func NewFileWatcher(cfg Config, raft RaftApplier, stateManager StateManager, forwarder LeaderForwarder) (*FileWatcher, error) {
	if cfg.DataDir == "" {
		return nil, fmt.Errorf("data directory cannot be empty")
	}
	if cfg.NodeID == "" {
		return nil, fmt.Errorf("node ID cannot be empty")
	}
	if cfg.Logger == nil {
		cfg.Logger = logrus.New()
	}
	if cfg.ApplyTimeout == 0 {
		cfg.ApplyTimeout = 5 * time.Second
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("creating file watcher: %w", err)
	}

	fw := &FileWatcher{
		dataDir:      cfg.DataDir,
		nodeID:       cfg.NodeID,
		watcher:      watcher,
		raft:         raft,
		stateManager: stateManager,
		forwarder:    forwarder,
		logger:       cfg.Logger,
		applyTimeout: cfg.ApplyTimeout,
	}

	return fw, nil
}

// Start begins monitoring the configured directory for file changes.
func (fw *FileWatcher) Start() error {
	if err := fw.watcher.Add(fw.dataDir); err != nil {
		return fmt.Errorf("adding directory to watcher: %w", err)
	}

	fw.logger.Infof("🔍 File watcher started for %s", fw.nodeID)

	// Start watching in a separate goroutine
	go fw.watchFiles()
	return nil
}

// Stop stops the file watcher and cleans up resources.
func (fw *FileWatcher) Stop() error {
	if fw.watcher != nil {
		return fw.watcher.Close()
	}
	return nil
}

// PauseWatching temporarily disables file system watching.
func (fw *FileWatcher) PauseWatching() {
	fw.watchingMutex.Lock()
	defer fw.watchingMutex.Unlock()
	fw.watchingPaused = true
}

// ResumeWatching re-enables file system watching after a brief delay.
func (fw *FileWatcher) ResumeWatching() {
	time.Sleep(defaultPauseDelay)
	fw.watchingMutex.Lock()
	defer fw.watchingMutex.Unlock()
	fw.watchingPaused = false
}

// IsWatchingPaused returns true if file watching is currently paused.
func (fw *FileWatcher) IsWatchingPaused() bool {
	fw.watchingMutex.RLock()
	defer fw.watchingMutex.RUnlock()
	return fw.watchingPaused
}

// watchFiles monitors file system events and triggers replication.
func (fw *FileWatcher) watchFiles() {
	for {
		select {
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}
			fw.handleFileEvent(event)

		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			fw.logger.WithError(err).Error("File watcher error")
		}
	}
}

// handleFileEvent processes a single file system event.
func (fw *FileWatcher) handleFileEvent(event fsnotify.Event) {
	// Skip if watching is paused or this is a Raft-related file
	if fw.IsWatchingPaused() || IsRaftFile(event.Name) {
		return
	}

	// Only handle regular files, not directories
	if info, err := os.Stat(event.Name); err != nil || info.IsDir() {
		return
	}

	// Handle writes and creates from any node
	if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
		fw.handleFileWrite(event.Name)
	}
}

// handleFileWrite processes a file write/create event.
func (fw *FileWatcher) handleFileWrite(filename string) {
	relPath, err := filepath.Rel(fw.dataDir, filename)
	if err != nil {
		return
	}

	// Add delay to ensure file write is complete
	time.Sleep(defaultWatchDelay)

	data, err := os.ReadFile(filename)
	if err != nil {
		fw.logger.WithError(err).Errorf("Failed to read file %s", filename)
		return
	}

	// Check if content actually changed
	if fw.stateManager.FileHasContent(relPath, data) {
		return
	}

	// Create command with metadata
	cmd := Command{
		Op:       OpWrite,
		Path:     relPath,
		Data:     data,
		Hash:     HashContent(data),
		NodeID:   fw.nodeID,
		Sequence: fw.stateManager.GetNextSequence(),
	}

	cmdData, err := json.Marshal(cmd)
	if err != nil {
		fw.logger.WithError(err).Error("Failed to marshal command")
		return
	}

	if fw.raft.State() == raft.Leader {
		// Apply directly if this node is the leader
		if err := fw.applyAsLeader(cmdData, relPath); err != nil {
			fw.logger.WithError(err).Errorf("Failed to replicate %s", relPath)
		}
	} else {
		// Forward to leader if this node is a follower
		fw.forwardToLeader(cmd, relPath)
	}
}

// applyAsLeader applies a command when this node is the Raft leader.
func (fw *FileWatcher) applyAsLeader(cmdData []byte, relPath string) error {
	future := fw.raft.Apply(cmdData, fw.applyTimeout)
	if err := future.Error(); err != nil {
		return err
	}

	fw.logger.Infof("📡 %s (leader) detected change in %s, replicating...", fw.nodeID, relPath)
	return nil
}

// forwardToLeader forwards a command to the current Raft leader.
func (fw *FileWatcher) forwardToLeader(cmd Command, relPath string) {
	leader := fw.raft.Leader()
	if leader == "" {
		fw.logger.Errorf("No leader available to forward %s", relPath)
		return
	}

	if err := fw.forwarder.ForwardToLeader(string(leader), cmd); err != nil {
		fw.logger.WithError(err).Errorf("Failed to forward %s to leader", relPath)
	} else {
		fw.logger.Infof("📡 %s (follower) forwarded change in %s to leader", fw.nodeID, relPath)
		// Update local file state to prevent re-detection
		fw.stateManager.UpdateFileState(relPath, cmd.Data)
	}
}

// IsRaftFile checks if a file is related to Raft internals.
func IsRaftFile(filename string) bool {
	base := filepath.Base(filename)
	return strings.HasPrefix(base, "raft-") ||
		strings.HasSuffix(base, ".db") ||
		base == "snapshots" ||
		strings.Contains(filename, "snapshots")
}
