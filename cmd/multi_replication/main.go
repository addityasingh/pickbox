// Package main implements a multi-directional distributed file replication system.
// Files changed on any node are automatically replicated to all other nodes in the cluster.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
	"github.com/sirupsen/logrus"
)

const (
	// Operations
	opWrite  = "write"
	opDelete = "delete"

	// Admin commands
	adminAddVoter = "ADD_VOTER"
	adminForward  = "FORWARD"

	// Default timeouts
	defaultApplyTimeout = 5 * time.Second
	defaultWatchDelay   = 50 * time.Millisecond
	defaultPauseDelay   = 200 * time.Millisecond

	// File permissions
	dirPerm  = 0755
	filePerm = 0644
)

// Config holds the configuration for a replication node.
type Config struct {
	NodeID           string
	Port             int
	JoinAddr         string
	DataDir          string
	AdminPort        int
	RaftAddr         string
	BootstrapCluster bool
}

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

// ReplicationFSM implements the Raft finite state machine with conflict resolution.
type ReplicationFSM struct {
	dataDir         string
	nodeID          string
	watcher         *fsnotify.Watcher
	raft            *raft.Raft
	watchingMutex   sync.RWMutex
	watchingPaused  bool
	fileStates      map[string]*FileState
	fileStatesMutex sync.RWMutex
	lastSequence    int64
	sequenceMutex   sync.Mutex
	logger          *logrus.Logger
}

// Snapshot implements raft.FSMSnapshot for state persistence.
type Snapshot struct {
	dataDir string
}

// NewReplicationFSM creates a new FSM instance.
func NewReplicationFSM(dataDir, nodeID string, logger *logrus.Logger) *ReplicationFSM {
	return &ReplicationFSM{
		dataDir:    dataDir,
		nodeID:     nodeID,
		fileStates: make(map[string]*FileState),
		logger:     logger,
	}
}

// Apply executes commands on the finite state machine.
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

// applyWrite handles file write operations.
func (fsm *ReplicationFSM) applyWrite(cmd Command) error {
	filePath := filepath.Join(fsm.dataDir, cmd.Path)

	// Check if content already matches to avoid unnecessary writes
	if fsm.fileHasContent(cmd.Path, cmd.Data) {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(filePath), dirPerm); err != nil {
		return fmt.Errorf("creating directory for %q: %w", cmd.Path, err)
	}

	if err := os.WriteFile(filePath, cmd.Data, filePerm); err != nil {
		return fmt.Errorf("writing file %q: %w", cmd.Path, err)
	}

	fsm.updateFileState(cmd.Path, cmd.Data)
	fsm.logger.Infof("✓ Replicated: %s (%d bytes) from %s", cmd.Path, len(cmd.Data), cmd.NodeID)
	return nil
}

// applyDelete handles file deletion operations.
func (fsm *ReplicationFSM) applyDelete(cmd Command) error {
	filePath := filepath.Join(fsm.dataDir, cmd.Path)

	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("deleting file %q: %w", cmd.Path, err)
	}

	fsm.removeFileState(cmd.Path)
	fsm.logger.Infof("✓ Deleted: %s from %s", cmd.Path, cmd.NodeID)
	return nil
}

// fileHasContent checks if a file already has the expected content.
func (fsm *ReplicationFSM) fileHasContent(path string, expectedData []byte) bool {
	fsm.fileStatesMutex.RLock()
	defer fsm.fileStatesMutex.RUnlock()

	state, exists := fsm.fileStates[path]
	if !exists {
		return false
	}

	expectedHash := hashContent(expectedData)
	return state.Hash == expectedHash
}

// updateFileState records the current state of a file.
func (fsm *ReplicationFSM) updateFileState(path string, data []byte) {
	fsm.fileStatesMutex.Lock()
	defer fsm.fileStatesMutex.Unlock()

	fsm.fileStates[path] = &FileState{
		Hash:         hashContent(data),
		LastModified: time.Now(),
		Size:         int64(len(data)),
	}
}

// removeFileState removes tracking for a deleted file.
func (fsm *ReplicationFSM) removeFileState(path string) {
	fsm.fileStatesMutex.Lock()
	defer fsm.fileStatesMutex.Unlock()

	delete(fsm.fileStates, path)
}

// Snapshot creates a point-in-time snapshot of the FSM state.
func (fsm *ReplicationFSM) Snapshot() (raft.FSMSnapshot, error) {
	return &Snapshot{dataDir: fsm.dataDir}, nil
}

// Restore reconstructs the FSM state from a snapshot.
func (fsm *ReplicationFSM) Restore(rc io.ReadCloser) error {
	defer func() {
		if err := rc.Close(); err != nil {
			fsm.logger.WithError(err).Warn("Error closing restore reader")
		}
	}()

	fsm.logger.Info("Restoring from snapshot")
	return nil
}

// pauseWatching temporarily disables file system watching.
func (fsm *ReplicationFSM) pauseWatching() {
	fsm.watchingMutex.Lock()
	defer fsm.watchingMutex.Unlock()
	fsm.watchingPaused = true
}

// resumeWatching re-enables file system watching after a brief delay.
func (fsm *ReplicationFSM) resumeWatching() {
	time.Sleep(defaultPauseDelay)
	fsm.watchingMutex.Lock()
	defer fsm.watchingMutex.Unlock()
	fsm.watchingPaused = false
}

// isWatchingPaused returns true if file watching is currently paused.
func (fsm *ReplicationFSM) isWatchingPaused() bool {
	fsm.watchingMutex.RLock()
	defer fsm.watchingMutex.RUnlock()
	return fsm.watchingPaused
}

// getNextSequence returns a monotonically increasing sequence number.
func (fsm *ReplicationFSM) getNextSequence() int64 {
	fsm.sequenceMutex.Lock()
	defer fsm.sequenceMutex.Unlock()
	fsm.lastSequence++
	return fsm.lastSequence
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

// hashContent computes the SHA-256 hash of data.
func hashContent(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// forwardToLeader sends a command to the leader via admin interface.
func forwardToLeader(leaderAddr string, cmd Command) error {
	// Extract host from raft address and construct admin address
	host, _, err := net.SplitHostPort(leaderAddr)
	if err != nil {
		return fmt.Errorf("parsing leader address %q: %w", leaderAddr, err)
	}

	// Try different admin ports (incremental from 9001)
	adminPorts := []int{9001, 9002, 9003}
	for _, port := range adminPorts {
		adminAddr := fmt.Sprintf("%s:%d", host, port)
		if err := sendForwardCommand(adminAddr, cmd); err == nil {
			return nil
		}
	}

	return fmt.Errorf("failed to forward command to leader at %s", leaderAddr)
}

// sendForwardCommand sends a FORWARD command to the specified admin address.
func sendForwardCommand(adminAddr string, cmd Command) error {
	conn, err := net.DialTimeout("tcp", adminAddr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("connecting to admin server: %w", err)
	}
	defer conn.Close()

	cmdData, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("marshaling command: %w", err)
	}

	message := fmt.Sprintf("%s %s", adminForward, string(cmdData))
	if _, err := conn.Write([]byte(message)); err != nil {
		return fmt.Errorf("sending command: %w", err)
	}

	return nil
}

// setupFileWatcher configures and starts the file system watcher.
func setupFileWatcher(fsm *ReplicationFSM, r *raft.Raft) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("creating file watcher: %w", err)
	}

	fsm.watcher = watcher
	fsm.raft = r

	if err := watcher.Add(fsm.dataDir); err != nil {
		return fmt.Errorf("adding directory to watcher: %w", err)
	}

	fsm.logger.Infof("🔍 File watcher enabled for %s", fsm.nodeID)

	// Start watching in a separate goroutine
	go fsm.watchFiles()
	return nil
}

// watchFiles monitors file system events and triggers replication.
func (fsm *ReplicationFSM) watchFiles() {
	for {
		select {
		case event, ok := <-fsm.watcher.Events:
			if !ok {
				return
			}
			fsm.handleFileEvent(event)

		case err, ok := <-fsm.watcher.Errors:
			if !ok {
				return
			}
			fsm.logger.WithError(err).Error("File watcher error")
		}
	}
}

// handleFileEvent processes a single file system event.
func (fsm *ReplicationFSM) handleFileEvent(event fsnotify.Event) {
	// Skip if watching is paused or this is a Raft-related file
	if fsm.isWatchingPaused() || isRaftFile(event.Name) {
		return
	}

	// Only handle regular files, not directories
	if info, err := os.Stat(event.Name); err != nil || info.IsDir() {
		return
	}

	// Handle writes and creates from any node
	if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
		fsm.handleFileWrite(event.Name)
	}
}

// handleFileWrite processes a file write/create event.
func (fsm *ReplicationFSM) handleFileWrite(filename string) {
	relPath, err := filepath.Rel(fsm.dataDir, filename)
	if err != nil {
		return
	}

	// Add delay to ensure file write is complete
	time.Sleep(defaultWatchDelay)

	data, err := os.ReadFile(filename)
	if err != nil {
		fsm.logger.WithError(err).Errorf("Failed to read file %s", filename)
		return
	}

	// Check if content actually changed
	if fsm.fileHasContent(relPath, data) {
		return
	}

	// Create command with metadata
	cmd := Command{
		Op:       opWrite,
		Path:     relPath,
		Data:     data,
		Hash:     hashContent(data),
		NodeID:   fsm.nodeID,
		Sequence: fsm.getNextSequence(),
	}

	cmdData, err := json.Marshal(cmd)
	if err != nil {
		fsm.logger.WithError(err).Error("Failed to marshal command")
		return
	}

	if fsm.raft.State() == raft.Leader {
		// Apply directly if this node is the leader
		if err := fsm.applyAsLeader(cmdData, relPath); err != nil {
			fsm.logger.WithError(err).Errorf("Failed to replicate %s", relPath)
		}
	} else {
		// Forward to leader if this node is a follower
		fsm.forwardToLeader(cmd, relPath)
	}
}

// applyAsLeader applies a command when this node is the Raft leader.
func (fsm *ReplicationFSM) applyAsLeader(cmdData []byte, relPath string) error {
	future := fsm.raft.Apply(cmdData, defaultApplyTimeout)
	if err := future.Error(); err != nil {
		return err
	}

	fsm.logger.Infof("📡 %s (leader) detected change in %s, replicating...", fsm.nodeID, relPath)
	return nil
}

// forwardToLeader forwards a command to the current Raft leader.
func (fsm *ReplicationFSM) forwardToLeader(cmd Command, relPath string) {
	leader := fsm.raft.Leader()
	if leader == "" {
		fsm.logger.Errorf("No leader available to forward %s", relPath)
		return
	}

	if err := forwardToLeader(string(leader), cmd); err != nil {
		fsm.logger.WithError(err).Errorf("Failed to forward %s to leader", relPath)
	} else {
		fsm.logger.Infof("📡 %s (follower) forwarded change in %s to leader", fsm.nodeID, relPath)
		// Update local file state to prevent re-detection
		fsm.updateFileState(relPath, cmd.Data)
	}
}

// isRaftFile checks if a file is related to Raft internals.
func isRaftFile(filename string) bool {
	base := filepath.Base(filename)
	return strings.HasPrefix(base, "raft-") ||
		strings.HasSuffix(base, ".db") ||
		base == "snapshots" ||
		strings.Contains(filename, "snapshots")
}

// createRaftNode creates and configures a new Raft node.
func createRaftNode(cfg Config, fsm *ReplicationFSM) (*raft.Raft, error) {
	config := raft.DefaultConfig()
	config.LocalID = raft.ServerID(cfg.NodeID)

	// Create Raft stores
	logStore, err := raftboltdb.NewBoltStore(filepath.Join(cfg.DataDir, "raft-log.db"))
	if err != nil {
		return nil, fmt.Errorf("creating log store: %w", err)
	}

	stableStore, err := raftboltdb.NewBoltStore(filepath.Join(cfg.DataDir, "raft-stable.db"))
	if err != nil {
		return nil, fmt.Errorf("creating stable store: %w", err)
	}

	snapshots, err := raft.NewFileSnapshotStore(cfg.DataDir, 3, os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("creating snapshot store: %w", err)
	}

	// Create transport
	transport, err := raft.NewTCPTransport(cfg.RaftAddr, nil, 3, 10*time.Second, os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("creating transport: %w", err)
	}

	// Create Raft node
	r, err := raft.NewRaft(config, fsm, logStore, stableStore, snapshots, transport)
	if err != nil {
		return nil, fmt.Errorf("creating raft node: %w", err)
	}

	return r, nil
}

// bootstrapCluster initializes the Raft cluster if this is the first node.
func bootstrapCluster(r *raft.Raft, cfg Config) error {
	if !cfg.BootstrapCluster {
		return nil
	}

	configuration := raft.Configuration{
		Servers: []raft.Server{
			{
				ID:      raft.ServerID(cfg.NodeID),
				Address: raft.ServerAddress(cfg.RaftAddr),
			},
		},
	}

	future := r.BootstrapCluster(configuration)
	if err := future.Error(); err != nil {
		return fmt.Errorf("bootstrapping cluster: %w", err)
	}

	logrus.Infof("🚀 Bootstrapped cluster as %s", cfg.NodeID)
	return nil
}

// startAdminServer starts the admin interface for cluster management.
func startAdminServer(r *raft.Raft, port int) {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		logrus.WithError(err).Error("Failed to start admin server")
		return
	}
	defer listener.Close()

	logrus.Infof("🔧 Admin server listening on port %d", port)

	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}

		go handleAdminConnection(r, conn)
	}
}

// handleAdminConnection processes a single admin connection.
func handleAdminConnection(r *raft.Raft, conn net.Conn) {
	defer conn.Close()

	buffer := make([]byte, 4096)
	n, err := conn.Read(buffer)
	if err != nil {
		return
	}

	command := strings.TrimSpace(string(buffer[:n]))

	if strings.HasPrefix(command, adminAddVoter) {
		handleAddVoter(r, conn, command)
	} else if strings.HasPrefix(command, adminForward) {
		handleForward(r, conn, command)
	}
}

// handleAddVoter processes ADD_VOTER admin commands.
func handleAddVoter(r *raft.Raft, conn net.Conn, command string) {
	parts := strings.Fields(command)
	if len(parts) != 3 {
		writeResponse(conn, "ERROR: Invalid ADD_VOTER command format")
		return
	}

	nodeID, address := parts[1], parts[2]
	future := r.AddVoter(raft.ServerID(nodeID), raft.ServerAddress(address), 0, 0)

	if err := future.Error(); err != nil {
		writeResponse(conn, fmt.Sprintf("ERROR: %v", err))
	} else {
		writeResponse(conn, "OK")
		logrus.Infof("➕ Added voter: %s at %s", nodeID, address)
	}
}

// handleForward processes FORWARD admin commands.
func handleForward(r *raft.Raft, conn net.Conn, command string) {
	if r.State() != raft.Leader {
		writeResponse(conn, "ERROR: Not leader")
		return
	}

	// Extract JSON command from "FORWARD {...}"
	jsonStart := strings.Index(command, "{")
	if jsonStart == -1 {
		writeResponse(conn, "ERROR: Invalid FORWARD command format")
		return
	}

	cmdData := []byte(command[jsonStart:])
	future := r.Apply(cmdData, defaultApplyTimeout)

	if err := future.Error(); err != nil {
		writeResponse(conn, fmt.Sprintf("ERROR: %v", err))
	} else {
		writeResponse(conn, "OK")
	}
}

// writeResponse writes a response to an admin connection.
func writeResponse(conn net.Conn, response string) {
	if _, err := conn.Write([]byte(response + "\n")); err != nil {
		logrus.WithError(err).Warn("Failed to write admin response")
	}
}

// parseConfig parses command-line flags into a Config struct.
func parseConfig() Config {
	var cfg Config
	flag.StringVar(&cfg.NodeID, "node", "node1", "Node ID")
	flag.IntVar(&cfg.Port, "port", 8001, "Raft port")
	flag.StringVar(&cfg.JoinAddr, "join", "", "Address of node to join")
	flag.Parse()

	cfg.DataDir = filepath.Join("data", cfg.NodeID)
	cfg.AdminPort = cfg.Port + 1000
	cfg.RaftAddr = fmt.Sprintf("127.0.0.1:%d", cfg.Port)
	cfg.BootstrapCluster = cfg.JoinAddr == ""

	return cfg
}

func main() {
	cfg := parseConfig()

	logrus.SetLevel(logrus.InfoLevel)
	logrus.Infof("Starting %s on port %d", cfg.NodeID, cfg.Port)

	// Create data directory
	if err := os.MkdirAll(cfg.DataDir, dirPerm); err != nil {
		logrus.WithError(err).Fatal("Failed to create data directory")
	}

	// Create FSM
	logger := logrus.New()
	fsm := NewReplicationFSM(cfg.DataDir, cfg.NodeID, logger)

	// Create Raft node
	r, err := createRaftNode(cfg, fsm)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to create Raft node")
	}

	fsm.raft = r

	// Bootstrap cluster if needed
	if err := bootstrapCluster(r, cfg); err != nil {
		logrus.WithError(err).Fatal("Failed to bootstrap cluster")
	}

	if cfg.JoinAddr != "" {
		logrus.Infof("🔗 Started %s, waiting to join cluster", cfg.NodeID)
	}

	// Start admin server
	go startAdminServer(r, cfg.AdminPort)

	// Set up file watcher for all nodes
	if err := setupFileWatcher(fsm, r); err != nil {
		logrus.WithError(err).Fatal("Failed to setup file watcher")
	}

	logrus.Info("🟢 Node is running! Try editing files in the data directory.")
	logrus.Infof("📁 Data directory: %s", cfg.DataDir)
	logrus.Info("🛑 Press Ctrl+C to stop")

	// Keep running
	select {}
}
