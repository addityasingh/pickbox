package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/aditya/pickbox/pkg/storage"
	"github.com/hashicorp/raft"
	"github.com/sirupsen/logrus"
)

func waitForLeadership(r *raft.Raft, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if r.State() == raft.Leader {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for leadership")
}

func isPortAvailable(port int) bool {
	addr := fmt.Sprintf(":%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

func main() {
	// Parse command line flags
	nodeID := flag.String("node-id", "", "Node ID (node1, node2, or node3)")
	port := flag.Int("port", 8001, "Port to listen on")
	flag.Parse()

	if *nodeID == "" {
		fmt.Println("Error: node-id is required")
		flag.Usage()
		os.Exit(1)
	}

	// Set up logging
	logrus.SetLevel(logrus.InfoLevel)
	logger := logrus.WithField("node", *nodeID)

	// Create data directory
	dataDir := filepath.Join("data", *nodeID)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		logger.WithError(err).Fatal("Failed to create data directory")
	}

	// Create storage manager
	manager, err := storage.NewManager(3, *nodeID, dataDir, fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		logger.WithError(err).Fatal("Failed to create storage manager")
	}

	// Bootstrap or join cluster
	if *nodeID == "node1" {
		// Wait for port to be available
		for !isPortAvailable(*port) {
			logger.Infof("Waiting for port %d to be available...", *port)
			time.Sleep(time.Second)
		}

		// Bootstrap the cluster with the first node
		servers := []raft.Server{
			{
				Suffrage: raft.Voter,
				ID:       raft.ServerID("node1"),
				Address:  raft.ServerAddress("127.0.0.1:8001"),
			},
		}
		if err := manager.BootstrapCluster(servers); err != nil {
			logger.WithError(err).Fatal("Failed to bootstrap cluster")
		}
		logger.Info("Bootstrapped cluster")

		// Wait for node1 to become leader
		if err := waitForLeadership(manager.GetRaft(), 10*time.Second); err != nil {
			logger.WithError(err).Fatal("Failed to become leader")
		}
		logger.Info("Became leader")

		// Create a channel to signal when node1 is ready to accept joins
		readyFile := filepath.Join("data", "node1_ready")
		if err := os.WriteFile(readyFile, []byte("ready"), 0644); err != nil {
			logger.WithError(err).Fatal("Failed to create ready file")
		}
	} else {
		// For follower nodes, just wait and let add_nodes.go handle joining
		logger.Info("Follower node started, waiting to be added to cluster...")
	}

	// Create a test file in the data directory
	testFile := filepath.Join(dataDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("Hello from "+*nodeID), 0644); err != nil {
		logger.WithError(err).Fatal("Failed to create test file")
	}

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("Shutting down...")
}
