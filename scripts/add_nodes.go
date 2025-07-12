package main

import (
	"flag"
	"fmt"
	"net"
	"time"
)

// addNodeToCluster adds a node to the cluster via the leader
func addNodeToCluster(leaderAddr, nodeID, nodeAddr string) error {
	// Connect to the leader
	conn, err := net.Dial("tcp", leaderAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to leader: %v", err)
	}
	defer conn.Close()

	// Send add voter command
	command := fmt.Sprintf("ADD_VOTER %s %s\n", nodeID, nodeAddr)
	_, err = conn.Write([]byte(command))
	if err != nil {
		return fmt.Errorf("failed to send command: %v", err)
	}

	// Read response
	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil {
		return fmt.Errorf("failed to read response: %v", err)
	}

	response := string(buffer[:n])
	if response != "OK\n" {
		return fmt.Errorf("failed to add node: %s", response)
	}

	return nil
}

func main() {
	// Parse command line arguments for flexibility
	var nodeCount int
	var basePort int
	var leaderAdminPort int
	var host string
	var startNode int
	var help bool

	flag.IntVar(&nodeCount, "nodes", 2, "Number of nodes to add")
	flag.IntVar(&basePort, "base-port", 8001, "Base port for Raft")
	flag.IntVar(&leaderAdminPort, "admin-port", 9001, "Leader admin port")
	flag.StringVar(&host, "host", "127.0.0.1", "Host address")
	flag.IntVar(&startNode, "start", 2, "Starting node number")
	flag.BoolVar(&help, "help", false, "Show help")
	flag.Parse()

	if help {
		fmt.Println("Generic Node Adder for Pickbox")
		fmt.Println("Usage: go run add_nodes.go [options]")
		fmt.Println("Options:")
		fmt.Println("  -nodes N       Number of nodes to add (default: 2)")
		fmt.Println("  -base-port P   Base port for Raft (default: 8001)")
		fmt.Println("  -admin-port P  Leader admin port (default: 9001)")
		fmt.Println("  -host H        Host address (default: 127.0.0.1)")
		fmt.Println("  -start N       Starting node number (default: 2)")
		fmt.Println("  -help          Show this help")
		fmt.Println("\nExamples:")
		fmt.Println("  go run add_nodes.go                    # Add node2, node3")
		fmt.Println("  go run add_nodes.go -nodes 5           # Add node2-node6")
		fmt.Println("  go run add_nodes.go -nodes 2 -start 4  # Add node4, node5")
		return
	}

	// Wait for leader to be ready
	fmt.Printf("Waiting for leader to be ready...\n")
	time.Sleep(5 * time.Second)

	leaderAddr := fmt.Sprintf("%s:%d", host, leaderAdminPort)

	fmt.Printf("Adding %d nodes starting from node%d...\n", nodeCount, startNode)

	// Generate and add nodes dynamically
	for i := 0; i < nodeCount; i++ {
		nodeNum := startNode + i
		nodeID := fmt.Sprintf("node%d", nodeNum)
		nodePort := basePort + nodeNum - 1
		nodeAddr := fmt.Sprintf("%s:%d", host, nodePort)

		fmt.Printf("Adding %s (%s) to cluster...\n", nodeID, nodeAddr)

		err := addNodeToCluster(leaderAddr, nodeID, nodeAddr)
		if err != nil {
			fmt.Printf("Failed to add %s: %v\n", nodeID, err)
		} else {
			fmt.Printf("Successfully added %s to cluster\n", nodeID)
		}

		time.Sleep(1 * time.Second)
	}

	fmt.Printf("Node addition completed!\n")
}
