package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

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
	dataDir string
}

func (f *FSM) Apply(log *raft.Log) interface{} {
	var cmd Command
	if err := json.Unmarshal(log.Data, &cmd); err != nil {
		return fmt.Errorf("failed to unmarshal command: %v", err)
	}

	switch cmd.Op {
	case "write":
		filePath := filepath.Join(f.dataDir, cmd.Path)
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			return fmt.Errorf("failed to create directory: %v", err)
		}
		if err := os.WriteFile(filePath, cmd.Data, 0644); err != nil {
			return fmt.Errorf("failed to write file: %v", err)
		}
		logrus.Infof("Applied write: %s (%d bytes)", cmd.Path, len(cmd.Data))
	case "delete":
		filePath := filepath.Join(f.dataDir, cmd.Path)
		if err := os.Remove(filePath); err != nil {
			return fmt.Errorf("failed to delete file: %v", err)
		}
		logrus.Infof("Applied delete: %s", cmd.Path)
	}
	return nil
}

func (f *FSM) Snapshot() (raft.FSMSnapshot, error) {
	return &Snapshot{dataDir: f.dataDir}, nil
}

func (f *FSM) Restore(rc io.ReadCloser) error {
	defer rc.Close()
	// For simplicity, we'll just log the restore
	logrus.Info("Restoring from snapshot")
	return nil
}

// Snapshot implements FSMSnapshot
type Snapshot struct {
	dataDir string
}

func (s *Snapshot) Persist(sink raft.SnapshotSink) error {
	defer sink.Close()
	// For simplicity, we'll just write a placeholder
	_, err := sink.Write([]byte("snapshot"))
	return err
}

func (s *Snapshot) Release() {}

func main() {
	var nodeID = flag.String("node", "node1", "Node ID")
	var port = flag.Int("port", 8001, "Port")
	var join = flag.String("join", "", "Address of node to join")
	flag.Parse()

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
	r, err := raft.NewRaft(
		config,
		fsm,
		logStore,
		stableStore,
		snapshots,
		transport,
	)
	if err != nil {
		logrus.Fatal(err)
	}

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
		logrus.Infof("Bootstrapped cluster as %s", *nodeID)
	} else {
		// Join existing cluster (this is handled externally)
		logrus.Infof("Started %s, waiting to join cluster", *nodeID)
	}

	// Start admin server for adding nodes
	go startAdminServer(r, *port+1000)

	// Wait for leadership
	for r.State() != raft.Leader {
		time.Sleep(100 * time.Millisecond)
	}
	logrus.Infof("%s became leader", *nodeID)

	// Add test file after becoming leader
	if *join == "" { // Only on the initial leader
		time.Sleep(2 * time.Second) // Give time for other nodes to start

		// Apply a test command
		cmd := Command{
			Op:   "write",
			Path: "test.txt",
			Data: []byte(fmt.Sprintf("Hello from %s at %s", *nodeID, time.Now().Format(time.RFC3339))),
		}

		data, _ := json.Marshal(cmd)
		future := r.Apply(data, 5*time.Second)
		if err := future.Error(); err != nil {
			logrus.Errorf("Failed to apply command: %v", err)
		} else {
			logrus.Info("Successfully applied test command")
		}
	}

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

	logrus.Infof("Admin server listening on port %d", port)

	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}

		go func(conn net.Conn) {
			defer conn.Close()

			// Read command
			buffer := make([]byte, 1024)
			n, err := conn.Read(buffer)
			if err != nil {
				return
			}

			command := string(buffer[:n])
			logrus.Infof("Received admin command: %s", command)

			// Simple command parsing
			if command[:9] == "ADD_VOTER" {
				// Format: ADD_VOTER nodeID address
				parts := string(buffer[:n])
				var nodeID, address string
				fmt.Sscanf(parts, "ADD_VOTER %s %s", &nodeID, &address)

				future := r.AddVoter(raft.ServerID(nodeID), raft.ServerAddress(address), 0, 0)
				if err := future.Error(); err != nil {
					conn.Write([]byte(fmt.Sprintf("ERROR: %v\n", err)))
				} else {
					conn.Write([]byte("OK\n"))
					logrus.Infof("Added voter: %s at %s", nodeID, address)
				}
			}
		}(conn)
	}
}
