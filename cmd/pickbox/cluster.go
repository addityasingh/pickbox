package main

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
)

var clusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "Cluster management commands",
	Long:  `Commands for managing Pickbox clusters including joining nodes and cluster operations`,
}

var clusterJoinCmd = &cobra.Command{
	Use:   "join",
	Short: "Join a node to an existing cluster",
	Long:  `Join a node to an existing Pickbox cluster by specifying the leader address`,
	RunE:  runClusterJoin,
}

var clusterStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check cluster status",
	Long:  `Check the status of a Pickbox cluster`,
	RunE:  runClusterStatus,
}

// Mutex to protect global variables from concurrent access
var globalVarsMutex sync.RWMutex

// Cluster join command flags
var (
	leaderAddr   string
	joinNodeID   string
	joinNodeAddr string
)

// Cluster status command flags
var (
	statusAddr string
)

func init() {
	rootCmd.AddCommand(clusterCmd)
	clusterCmd.AddCommand(clusterJoinCmd)
	clusterCmd.AddCommand(clusterStatusCmd)

	// Cluster join command flags
	clusterJoinCmd.Flags().StringVarP(&leaderAddr, "leader", "L", "", "Leader address (required)")
	clusterJoinCmd.Flags().StringVarP(&joinNodeID, "node-id", "n", "", "Node ID to join (required)")
	clusterJoinCmd.Flags().StringVarP(&joinNodeAddr, "node-addr", "a", "", "Node address (required)")
	clusterJoinCmd.MarkFlagRequired("leader")
	clusterJoinCmd.MarkFlagRequired("node-id")
	clusterJoinCmd.MarkFlagRequired("node-addr")

	// Cluster status command flags
	clusterStatusCmd.Flags().StringVarP(&statusAddr, "addr", "a", "127.0.0.1:9001", "Admin address to check status")
}

func runClusterJoin(cmd *cobra.Command, args []string) error {
	// Validate cmd parameter
	if cmd == nil {
		return fmt.Errorf("command is nil")
	}

	// Thread-safe access to global variables
	globalVarsMutex.RLock()
	leader := leaderAddr
	nodeID := joinNodeID
	nodeAddr := joinNodeAddr
	globalVarsMutex.RUnlock()

	// Validate required global variables are set
	if leader == "" {
		return fmt.Errorf("leader address is required")
	}
	if nodeID == "" {
		return fmt.Errorf("node ID is required")
	}
	if nodeAddr == "" {
		return fmt.Errorf("node address is required")
	}

	// Derive admin address from leader address
	adminAddr := deriveAdminAddr(leader)

	fmt.Printf("Attempting to join node %s (%s) to cluster via %s...\n", nodeID, nodeAddr, adminAddr)

	// Use the admin API to join the cluster
	conn, err := net.DialTimeout("tcp", adminAddr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("connecting to admin server: %w", err)
	}
	defer conn.Close()

	message := fmt.Sprintf("ADD_VOTER %s %s", nodeID, nodeAddr)
	if _, err := conn.Write([]byte(message)); err != nil {
		return fmt.Errorf("sending join request: %w", err)
	}

	// Read response
	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	response := strings.TrimSpace(string(buffer[:n]))
	if response != "OK" {
		return fmt.Errorf("join request failed: %s", response)
	}

	fmt.Printf("✅ Successfully joined node %s to cluster\n", nodeID)
	return nil
}

func runClusterStatus(cmd *cobra.Command, args []string) error {
	// Validate cmd parameter
	if cmd == nil {
		return fmt.Errorf("command is nil")
	}

	// Thread-safe access to global variables
	globalVarsMutex.RLock()
	statusAddress := statusAddr
	globalVarsMutex.RUnlock()

	// Validate required global variable is set
	if statusAddress == "" {
		return fmt.Errorf("status address is required")
	}

	// This is a simple implementation - in a real system you'd query more cluster info
	conn, err := net.DialTimeout("tcp", statusAddress, 2*time.Second)
	if err != nil {
		fmt.Printf("❌ Cannot connect to admin server at %s\n", statusAddress)
		return fmt.Errorf("connecting to admin server: %w", err)
	}
	defer conn.Close()

	fmt.Printf("✅ Admin server is reachable at %s\n", statusAddress)
	fmt.Printf("🔍 For detailed cluster status, check the monitoring dashboard\n")
	return nil
}

func deriveAdminAddr(raftAddr string) string {
	// Handle empty or invalid input
	if raftAddr == "" {
		return "127.0.0.1:9001" // Default admin port
	}

	parts := strings.Split(raftAddr, ":")
	if len(parts) != 2 || parts[0] == "" {
		return "127.0.0.1:9001" // Default admin port
	}

	// Convert raft port to admin port (typically raft_port + 1000)
	host := strings.TrimSpace(parts[0])
	if host == "" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("%s:9001", host) // Default admin port
}
