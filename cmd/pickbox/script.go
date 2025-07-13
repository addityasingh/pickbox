package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/spf13/cobra"
)

var scriptCmd = &cobra.Command{
	Use:   "script",
	Short: "Run common cluster scripts",
	Long:  `Run common cluster scripts for testing and demonstration`,
}

var scriptDemo3Cmd = &cobra.Command{
	Use:   "demo-3-nodes",
	Short: "Demo script for 3-node cluster",
	Long:  `Demonstrates setting up a 3-node cluster with bootstrap and joining`,
	RunE:  runDemo3Nodes,
}

var scriptCleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Clean up data directories",
	Long:  `Clean up data directories from previous runs`,
	RunE:  runCleanup,
}

func init() {
	rootCmd.AddCommand(scriptCmd)
	scriptCmd.AddCommand(scriptDemo3Cmd)
	scriptCmd.AddCommand(scriptCleanupCmd)
}

func runDemo3Nodes(cmd *cobra.Command, args []string) error {
	fmt.Println("🚀 Starting 3-node cluster demo...")

	// Get data directory from global flags
	dataDir, _ := cmd.Flags().GetString("data-dir")

	// Clean up first
	if err := cleanup(dataDir); err != nil {
		fmt.Printf("Warning: cleanup failed: %v\n", err)
	}

	fmt.Println("📋 Starting nodes...")

	// Start node1 as bootstrap
	fmt.Println("Starting node1 (bootstrap)...")
	if err := startNodeInBackground("node1", 8001, 9001, "", true); err != nil {
		return fmt.Errorf("starting node1: %w", err)
	}

	// Wait for node1 to be ready
	time.Sleep(3 * time.Second)

	// Start node2
	fmt.Println("Starting node2...")
	if err := startNodeInBackground("node2", 8002, 9002, "127.0.0.1:8001", false); err != nil {
		return fmt.Errorf("starting node2: %w", err)
	}

	// Start node3
	fmt.Println("Starting node3...")
	if err := startNodeInBackground("node3", 8003, 9003, "127.0.0.1:8001", false); err != nil {
		return fmt.Errorf("starting node3: %w", err)
	}

	fmt.Println("✅ 3-node cluster started!")
	fmt.Println("📊 Access URLs:")
	fmt.Println("  Node1 Admin: http://localhost:9001")
	fmt.Println("  Node2 Admin: http://localhost:9002")
	fmt.Println("  Node3 Admin: http://localhost:9003")
	fmt.Println("  Node1 Dashboard: http://localhost:9003")
	fmt.Println("  Node2 Dashboard: http://localhost:9006")
	fmt.Println("  Node3 Dashboard: http://localhost:9009")
	fmt.Println("📁 Data directories:")
	fmt.Println("  Node1: data/node1")
	fmt.Println("  Node2: data/node2")
	fmt.Println("  Node3: data/node3")
	fmt.Println("🛑 To stop all nodes, run: pkill pickbox")

	return nil
}

func runCleanup(cmd *cobra.Command, args []string) error {
	// Get data directory from global flags
	dataDir, _ := cmd.Flags().GetString("data-dir")

	fmt.Println("🧹 Cleaning up data directories...")
	return cleanup(dataDir)
}

func cleanup(dataDir string) error {
	if err := os.RemoveAll(dataDir); err != nil {
		return fmt.Errorf("removing data directory: %w", err)
	}

	fmt.Println("✅ Cleanup completed")
	return nil
}

func startNodeInBackground(nodeID string, port, adminPort int, joinAddr string, bootstrap bool) error {
	// Build command arguments
	args := []string{
		"node", "start",
		"--node-id", nodeID,
		"--port", strconv.Itoa(port),
		"--admin-port", strconv.Itoa(adminPort),
		"--monitor-port", strconv.Itoa(adminPort + 1),
		"--dashboard-port", strconv.Itoa(adminPort + 2),
	}

	if bootstrap {
		args = append(args, "--bootstrap")
	}

	if joinAddr != "" {
		args = append(args, "--join", joinAddr)
	}

	// Get the current executable path
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("getting executable path: %w", err)
	}

	// Start the command in background
	cmd := exec.Command(executable, args...)
	cmd.Dir = filepath.Dir(executable)

	// Start the process
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting node %s: %w", nodeID, err)
	}

	fmt.Printf("✅ Node %s started (PID: %d)\n", nodeID, cmd.Process.Pid)
	return nil
}
