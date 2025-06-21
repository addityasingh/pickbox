package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
	"github.com/sirupsen/logrus"
)

// Command represents a file operation
type Command struct {
	Op   string `json:"op"`
	Path string `json:"path"`
	Data []byte `json:"data"`
}

// FSM implements the Raft finite state machine
type FSM struct {
	dataDir  string
	watcher  *fsnotify.Watcher
	raft     *raft.Raft
	isLeader bool
}

func (f *FSM) Apply(log *raft.Log) interface{} {
	var cmd Command
	if err := json.Unmarshal(log.Data, &cmd); err != nil {
		return fmt.Errorf("failed to unmarshal command: %v", err)
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
		if err := os.WriteFile(filePath, cmd.Data, 0644); err != nil {
			return fmt.Errorf("failed to write file: %v", err)
		}
		logrus.Infof("✓ Replicated: %s (%d bytes)", cmd.Path, len(cmd.Data))
	case "delete":
		filePath := filepath.Join(f.dataDir, cmd.Path)
		if err := os.Remove(filePath); err != nil {
			return fmt.Errorf("failed to delete file: %v", err)
		}
		logrus.Infof("✓ Deleted: %s", cmd.Path)
	}
	return nil
}

func (f *FSM) Snapshot() (raft.FSMSnapshot, error) {
	return &Snapshot{dataDir: f.dataDir}, nil
}

func (f *FSM) Restore(rc io.ReadCloser) error {
	defer rc.Close()
	logrus.Info("Restoring from snapshot")
	return nil
}

var watchingPaused = false

func (f *FSM) pauseWatching() {
	watchingPaused = true
}

func (f *FSM) resumeWatching() {
	time.Sleep(100 * time.Millisecond) // Brief pause to avoid race conditions
	watchingPaused = false
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

	// Start watching in a goroutine
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				// Skip if watching is paused or if this is a Raft file
				if watchingPaused || isRaftFile(event.Name) {
					continue
				}

				// Only handle regular files, not directories
				if info, err := os.Stat(event.Name); err != nil || info.IsDir() {
					continue
				}

				// Only react to writes and creates, and only if we're the leader
				if (event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create) && r.State() == raft.Leader {
					// Get relative path
					relPath, err := filepath.Rel(fsm.dataDir, event.Name)
					if err != nil {
						continue
					}

					// Read file content
					data, err := os.ReadFile(event.Name)
					if err != nil {
						logrus.Errorf("Failed to read file %s: %v", event.Name, err)
						continue
					}

					// Create command
					cmd := Command{
						Op:   "write",
						Path: relPath,
						Data: data,
					}

					// Apply through Raft
					cmdData, _ := json.Marshal(cmd)
					future := r.Apply(cmdData, 5*time.Second)
					if err := future.Error(); err != nil {
						logrus.Errorf("Failed to replicate %s: %v", relPath, err)
					} else {
						logrus.Infof("📡 Detected change in %s, replicating...", relPath)
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

	// Create FSM
	fsm := &FSM{dataDir: dataDir}

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

	// Wait for leadership
	go func() {
		for {
			if r.State() == raft.Leader {
				if !fsm.isLeader {
					fsm.isLeader = true
					logrus.Infof("👑 %s became leader - file watching enabled", *nodeID)

					// Setup file watcher
					if err := setupFileWatcher(fsm, r); err != nil {
						logrus.Errorf("Failed to setup file watcher: %v", err)
					}
				}
			} else {
				if fsm.isLeader {
					fsm.isLeader = false
					logrus.Infof("👥 %s is now a follower - file watching disabled", *nodeID)
				}
			}
			time.Sleep(1 * time.Second)
		}
	}()

	// Create a welcome file for testing
	if *join == "" {
		time.Sleep(3 * time.Second) // Wait for leadership
		welcomeFile := filepath.Join(dataDir, "welcome.txt")
		welcomeContent := fmt.Sprintf("Welcome to %s!\nThis file was created at %s\n\nTry editing this file and watch it replicate to other nodes!", *nodeID, time.Now().Format(time.RFC3339))
		if err := os.WriteFile(welcomeFile, []byte(welcomeContent), 0644); err == nil {
			logrus.Info("📝 Created welcome.txt - try editing it!")
		}
	}

	logrus.Info("🟢 Node is running! Try editing files in the data directory.")
	logrus.Info("📁 Data directory:", dataDir)
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
			}
		}(conn)
	}
}
