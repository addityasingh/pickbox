package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
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

// Command represents a file operation with metadata
type Command struct {
	Op       string `json:"op"`
	Path     string `json:"path"`
	Data     []byte `json:"data"`
	Hash     string `json:"hash"`     // Content hash for deduplication
	NodeID   string `json:"node_id"`  // Which node initiated the change
	Sequence int64  `json:"sequence"` // Sequence number for ordering
}

// FileState tracks file content hashes to prevent unnecessary replication
type FileState struct {
	Hash         string
	LastModified time.Time
	Size         int64
}

// FSM implements the Raft finite state machine with improved conflict resolution
type FSM struct {
	dataDir         string
	nodeID          string
	watcher         *fsnotify.Watcher
	raft            *raft.Raft
	watchingMutex   sync.RWMutex
	watchingPaused  bool
	fileStates      map[string]*FileState // Track file hashes
	fileStatesMutex sync.RWMutex
	lastSequence    int64
	sequenceMutex   sync.Mutex
}

func (f *FSM) Apply(log *raft.Log) interface{} {
	var cmd Command
	if err := json.Unmarshal(log.Data, &cmd); err != nil {
		return fmt.Errorf("failed to unmarshal command: %v", err)
	}

	// Skip if this command came from the current node and the file already matches
	if cmd.NodeID == f.nodeID {
		if f.fileHasContent(cmd.Path, cmd.Data) {
			return nil // Skip applying to avoid infinite loops
		}
	}

	// Temporarily disable file watching to avoid infinite loops
	f.pauseWatching()
	defer f.resumeWatching()

	switch cmd.Op {
	case "write":
		filePath := filepath.Join(f.dataDir, cmd.Path)
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			return fmt.Errorf("failed to create directory: %v", err)
		}

		// Check if content already matches to avoid unnecessary writes
		if f.fileHasContent(cmd.Path, cmd.Data) {
			return nil
		}

		if err := os.WriteFile(filePath, cmd.Data, 0644); err != nil {
			return fmt.Errorf("failed to write file: %v", err)
		}

		// Update file state tracking
		f.updateFileState(cmd.Path, cmd.Data)

		logrus.Infof("✓ Replicated: %s (%d bytes) from %s", cmd.Path, len(cmd.Data), cmd.NodeID)

	case "delete":
		filePath := filepath.Join(f.dataDir, cmd.Path)
		if err := os.Remove(filePath); err != nil {
			return fmt.Errorf("failed to delete file: %v", err)
		}

		// Remove from file state tracking
		f.removeFileState(cmd.Path)

		logrus.Infof("✓ Deleted: %s from %s", cmd.Path, cmd.NodeID)
	}
	return nil
}

func (f *FSM) fileHasContent(path string, expectedData []byte) bool {
	f.fileStatesMutex.RLock()
	defer f.fileStatesMutex.RUnlock()

	state, exists := f.fileStates[path]
	if !exists {
		return false
	}

	expectedHash := hashContent(expectedData)
	return state.Hash == expectedHash
}

func (f *FSM) updateFileState(path string, data []byte) {
	f.fileStatesMutex.Lock()
	defer f.fileStatesMutex.Unlock()

	f.fileStates[path] = &FileState{
		Hash:         hashContent(data),
		LastModified: time.Now(),
		Size:         int64(len(data)),
	}
}

func (f *FSM) removeFileState(path string) {
	f.fileStatesMutex.Lock()
	defer f.fileStatesMutex.Unlock()

	delete(f.fileStates, path)
}

func (f *FSM) Snapshot() (raft.FSMSnapshot, error) {
	return &Snapshot{dataDir: f.dataDir}, nil
}

func (f *FSM) Restore(rc io.ReadCloser) error {
	defer rc.Close()
	logrus.Info("Restoring from snapshot")
	return nil
}

func (f *FSM) pauseWatching() {
	f.watchingMutex.Lock()
	defer f.watchingMutex.Unlock()
	f.watchingPaused = true
}

func (f *FSM) resumeWatching() {
	// Brief pause to avoid race conditions
	time.Sleep(200 * time.Millisecond)
	f.watchingMutex.Lock()
	defer f.watchingMutex.Unlock()
	f.watchingPaused = false
}

func (f *FSM) isWatchingPaused() bool {
	f.watchingMutex.RLock()
	defer f.watchingMutex.RUnlock()
	return f.watchingPaused
}

func (f *FSM) getNextSequence() int64 {
	f.sequenceMutex.Lock()
	defer f.sequenceMutex.Unlock()
	f.lastSequence++
	return f.lastSequence
}

// Snapshot implements FSMSnapshot
type Snapshot struct {
	dataDir string
}

func (s *Snapshot) Persist(sink raft.SnapshotSink) error {
	defer sink.Close()
	_, err := sink.Write([]byte("snapshot"))
	return err
}

func (s *Snapshot) Release() {}

func hashContent(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// forwardToLeader sends a command to the leader for processing
func forwardToLeader(leaderAddr string, cmd Command) error {
	// Convert Raft address to admin port (leader's admin port is +1000)
	parts := strings.Split(leaderAddr, ":")
	if len(parts) != 2 {
		return fmt.Errorf("invalid leader address: %s", leaderAddr)
	}

	port := parts[1]
	var adminPort string
	switch port {
	case "8001":
		adminPort = "9001"
	case "8002":
		adminPort = "9002"
	case "8003":
		adminPort = "9003"
	default:
		return fmt.Errorf("unknown port for leader forwarding: %s", port)
	}

	adminAddr := parts[0] + ":" + adminPort

	// Connect to leader's admin interface
	conn, err := net.Dial("tcp", adminAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to leader admin: %v", err)
	}
	defer conn.Close()

	// Send FORWARD command with the file change
	cmdData, _ := json.Marshal(cmd)
	message := fmt.Sprintf("FORWARD %s", string(cmdData))

	_, err = conn.Write([]byte(message))
	if err != nil {
		return fmt.Errorf("failed to send forward command: %v", err)
	}

	// Read response
	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil {
		return fmt.Errorf("failed to read response: %v", err)
	}

	response := string(buffer[:n])
	if !strings.HasPrefix(response, "OK") {
		return fmt.Errorf("leader rejected command: %s", response)
	}

	return nil
}

func setupFileWatcher(fsm *FSM, r *raft.Raft) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	fsm.watcher = watcher
	fsm.raft = r

	// Watch the data directory
	err = watcher.Add(fsm.dataDir)
	if err != nil {
		return err
	}

	logrus.Infof("🔍 File watcher enabled for %s", fsm.nodeID)

	// Start watching in a goroutine
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				// Skip if watching is paused or if this is a Raft file
				if fsm.isWatchingPaused() || isRaftFile(event.Name) {
					continue
				}

				// Only handle regular files, not directories
				if info, err := os.Stat(event.Name); err != nil || info.IsDir() {
					continue
				}

				// Handle writes and creates from ANY node (not just leader)
				if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
					// Get relative path
					relPath, err := filepath.Rel(fsm.dataDir, event.Name)
					if err != nil {
						continue
					}

					// Add small delay to ensure file write is complete
					time.Sleep(50 * time.Millisecond)

					// Read file content
					data, err := os.ReadFile(event.Name)
					if err != nil {
						logrus.Errorf("Failed to read file %s: %v", event.Name, err)
						continue
					}

					// Check if content actually changed
					if fsm.fileHasContent(relPath, data) {
						continue // Skip if content hasn't changed
					}

					// Create command with hash and metadata
					contentHash := hashContent(data)
					cmd := Command{
						Op:       "write",
						Path:     relPath,
						Data:     data,
						Hash:     contentHash,
						NodeID:   fsm.nodeID,
						Sequence: fsm.getNextSequence(),
					}

					// Apply through Raft (leader only) or forward to leader
					cmdData, _ := json.Marshal(cmd)

					if r.State() == raft.Leader {
						// If this node is the leader, apply directly
						future := r.Apply(cmdData, 5*time.Second)
						if err := future.Error(); err != nil {
							logrus.Errorf("Failed to replicate %s: %v", relPath, err)
						} else {
							logrus.Infof("📡 %s (leader) detected change in %s, replicating...", fsm.nodeID, relPath)
							// Update local file state to prevent re-detection
							fsm.updateFileState(relPath, data)
						}
					} else {
						// If this node is a follower, forward to leader
						leader := r.Leader()
						if leader != "" {
							if err := forwardToLeader(string(leader), cmd); err != nil {
								logrus.Errorf("Failed to forward %s to leader: %v", relPath, err)
							} else {
								logrus.Infof("📡 %s (follower) forwarded change in %s to leader", fsm.nodeID, relPath)
								// Update local file state to prevent re-detection
								fsm.updateFileState(relPath, data)
							}
						} else {
							logrus.Errorf("No leader available to forward %s", relPath)
						}
					}
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				logrus.Errorf("File watcher error: %v", err)
			}
		}
	}()

	return nil
}

func isRaftFile(filename string) bool {
	base := filepath.Base(filename)
	return strings.HasPrefix(base, "raft-") ||
		strings.HasSuffix(base, ".db") ||
		base == "snapshots" ||
		strings.Contains(filename, "snapshots")
}

func main() {
	var nodeID = flag.String("node", "node1", "Node ID")
	var port = flag.Int("port", 8001, "Port")
	var join = flag.String("join", "", "Address of node to join")
	flag.Parse()

	logrus.SetLevel(logrus.InfoLevel)
	logrus.Infof("Starting %s on port %d", *nodeID, *port)

	// Setup
	dataDir := filepath.Join("data", *nodeID)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		logrus.Fatal(err)
	}

	// Create Raft configuration
	config := raft.DefaultConfig()
	config.LocalID = raft.ServerID(*nodeID)

	// Create FSM with improved tracking
	fsm := &FSM{
		dataDir:    dataDir,
		nodeID:     *nodeID,
		fileStates: make(map[string]*FileState),
	}

	// Create stores
	logStore, err := raftboltdb.NewBoltStore(filepath.Join(dataDir, "raft-log.db"))
	if err != nil {
		logrus.Fatal(err)
	}

	stableStore, err := raftboltdb.NewBoltStore(filepath.Join(dataDir, "raft-stable.db"))
	if err != nil {
		logrus.Fatal(err)
	}

	snapshots, err := raft.NewFileSnapshotStore(dataDir, 3, os.Stderr)
	if err != nil {
		logrus.Fatal(err)
	}

	// Create transport
	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	transport, err := raft.NewTCPTransport(addr, nil, 3, 10*time.Second, os.Stderr)
	if err != nil {
		logrus.Fatal(err)
	}

	// Create Raft node
	r, err := raft.NewRaft(config, fsm, logStore, stableStore, snapshots, transport)
	if err != nil {
		logrus.Fatal(err)
	}

	fsm.raft = r

	if *join == "" {
		// Bootstrap cluster
		configuration := raft.Configuration{
			Servers: []raft.Server{
				{
					ID:      config.LocalID,
					Address: transport.LocalAddr(),
				},
			},
		}
		r.BootstrapCluster(configuration)
		logrus.Infof("🚀 Bootstrapped cluster as %s", *nodeID)
	} else {
		logrus.Infof("🔗 Started %s, waiting to join cluster", *nodeID)
	}

	// Start admin server for adding nodes
	go startAdminServer(r, *port+1000)

	// Set up file watcher for ALL nodes (not just leader)
	// This enables multi-directional replication
	go func() {
		time.Sleep(3 * time.Second) // Wait for initial setup
		logrus.Infof("🔍 Setting up file watcher for %s", *nodeID)
		if err := setupFileWatcher(fsm, r); err != nil {
			logrus.Errorf("Failed to setup file watcher: %v", err)
		}
	}()

	// Monitor leadership for logging purposes
	go func() {
		var wasLeader bool
		for {
			isLeader := r.State() == raft.Leader
			if isLeader && !wasLeader {
				logrus.Infof("👑 %s became leader", *nodeID)
				wasLeader = true
			} else if !isLeader && wasLeader {
				logrus.Infof("👥 %s is now a follower", *nodeID)
				wasLeader = false
			}
			time.Sleep(2 * time.Second)
		}
	}()

	// Create a welcome file for testing
	if *join == "" {
		time.Sleep(5 * time.Second) // Wait for setup
		welcomeFile := filepath.Join(dataDir, "welcome.txt")
		welcomeContent := fmt.Sprintf("Welcome to %s!\nThis file was created at %s\n\nYou can now edit files in ANY node directory and they will replicate to all others!", *nodeID, time.Now().Format(time.RFC3339))
		if err := os.WriteFile(welcomeFile, []byte(welcomeContent), 0644); err == nil {
			logrus.Info("📝 Created welcome.txt")
		}
	}

	logrus.Info("🟢 Node is running with multi-directional replication!")
	logrus.Info("📁 Data directory:", dataDir)
	logrus.Info("🔄 Edit files in ANY node directory to see replication")
	logrus.Info("🛑 Press Ctrl+C to stop")

	// Keep running
	select {}
}

// startAdminServer starts a simple admin server for managing the cluster
func startAdminServer(r *raft.Raft, port int) {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		logrus.Errorf("Failed to start admin server: %v", err)
		return
	}
	defer listener.Close()

	logrus.Infof("🔧 Admin server listening on port %d", port)

	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}

		go func(conn net.Conn) {
			defer conn.Close()

			buffer := make([]byte, 1024)
			n, err := conn.Read(buffer)
			if err != nil {
				return
			}

			command := string(buffer[:n])
			if len(command) > 9 && command[:9] == "ADD_VOTER" {
				var nodeID, address string
				fmt.Sscanf(command, "ADD_VOTER %s %s", &nodeID, &address)

				future := r.AddVoter(raft.ServerID(nodeID), raft.ServerAddress(address), 0, 0)
				if err := future.Error(); err != nil {
					conn.Write([]byte(fmt.Sprintf("ERROR: %v\n", err)))
				} else {
					conn.Write([]byte("OK\n"))
					logrus.Infof("➕ Added voter: %s at %s", nodeID, address)
				}
			} else if len(command) > 8 && command[:7] == "FORWARD" {
				// Handle forwarded file changes from followers
				cmdJson := command[8:] // Skip "FORWARD "
				var cmd Command
				if err := json.Unmarshal([]byte(cmdJson), &cmd); err != nil {
					conn.Write([]byte(fmt.Sprintf("ERROR: Invalid command format: %v\n", err)))
					return
				}

				// Apply the command through Raft (only leaders can do this)
				if r.State() == raft.Leader {
					cmdData, _ := json.Marshal(cmd)
					future := r.Apply(cmdData, 5*time.Second)
					if err := future.Error(); err != nil {
						conn.Write([]byte(fmt.Sprintf("ERROR: %v\n", err)))
					} else {
						conn.Write([]byte("OK\n"))
						logrus.Infof("🔄 Applied forwarded command: %s from %s", cmd.Path, cmd.NodeID)
					}
				} else {
					conn.Write([]byte("ERROR: Not the leader\n"))
				}
			}
		}(conn)
	}
}
