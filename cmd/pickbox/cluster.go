package main

import (
	"fmt"
	"net"
	"strings"
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
	// Validate required global variables are set
	if leaderAddr == "" {
		return fmt.Errorf("leader address is required")
	}
	if joinNodeID == "" {
		return fmt.Errorf("node ID is required")
	}
	if joinNodeAddr == "" {
		return fmt.Errorf("node address is required")
	}

	// Derive admin address from leader address
	adminAddr := deriveAdminAddr(leaderAddr)

	fmt.Printf("Attempting to join node %s (%s) to cluster via %s...\n", joinNodeID, joinNodeAddr, adminAddr)

	// Use the admin API to join the cluster
	conn, err := net.DialTimeout("tcp", adminAddr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("connecting to admin server: %w", err)
	}
	defer conn.Close()

	message := fmt.Sprintf("ADD_VOTER %s %s", joinNodeID, joinNodeAddr)
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

	fmt.Printf("✅ Successfully joined node %s to cluster\n", joinNodeID)
	return nil
}

func runClusterStatus(cmd *cobra.Command, args []string) error {
	// Validate required global variable is set
	if statusAddr == "" {
		return fmt.Errorf("status address is required")
	}

	// This is a simple implementation - in a real system you'd query more cluster info
	conn, err := net.DialTimeout("tcp", statusAddr, 2*time.Second)
	if err != nil {
		fmt.Printf("❌ Cannot connect to admin server at %s\n", statusAddr)
		return fmt.Errorf("connecting to admin server: %w", err)
	}
	defer conn.Close()

	fmt.Printf("✅ Admin server is reachable at %s\n", statusAddr)
	fmt.Printf("🔍 For detailed cluster status, check the monitoring dashboard\n")
	return nil
}

func deriveAdminAddr(raftAddr string) string {
	parts := strings.Split(raftAddr, ":")
	if len(parts) != 2 {
		return "127.0.0.1:9001" // Default admin port
	}

	// Convert raft port to admin port (typically raft_port + 1000)
	host := parts[0]
	return fmt.Sprintf("%s:9001", host) // Default admin port
}
