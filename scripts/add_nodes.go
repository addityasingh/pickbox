package main

import (
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
	// Wait for node1 to be ready
	fmt.Println("Waiting for node1 to be ready...")
	time.Sleep(5 * time.Second)

	// Try to add node2 and node3 to the cluster
	nodes := []struct {
		ID   string
		Addr string
	}{
		{"node2", "127.0.0.1:8002"},
		{"node3", "127.0.0.1:8003"},
	}

	leaderAddr := "127.0.0.1:8001"

	for _, node := range nodes {
		fmt.Printf("Adding %s to cluster...\n", node.ID)

		err := addNodeToCluster(leaderAddr, node.ID, node.Addr)
		if err != nil {
			fmt.Printf("Failed to add %s: %v\n", node.ID, err)
		} else {
			fmt.Printf("Successfully added %s to cluster\n", node.ID)
		}

		time.Sleep(1 * time.Second)
	}
}
