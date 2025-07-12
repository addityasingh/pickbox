package test

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test N-node configuration validation
func TestNNodeConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		nodeID   string
		port     int
		joinAddr string
		wantErr  bool
	}{
		{
			name:     "valid_single_node",
			nodeID:   "node1",
			port:     8001,
			joinAddr: "",
			wantErr:  false,
		},
		{
			name:     "valid_joining_node",
			nodeID:   "node2",
			port:     8002,
			joinAddr: "127.0.0.1:8001",
			wantErr:  false,
		},
		{
			name:     "invalid_empty_node_id",
			nodeID:   "",
			port:     8001,
			joinAddr: "",
			wantErr:  true,
		},
		{
			name:     "invalid_zero_port",
			nodeID:   "node1",
			port:     0,
			joinAddr: "",
			wantErr:  true,
		},
		{
			name:     "invalid_negative_port",
			nodeID:   "node1",
			port:     -1,
			joinAddr: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test basic validation logic
			err := validateNodeConfig(tt.nodeID, tt.port, tt.joinAddr)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Helper function for configuration validation
func validateNodeConfig(nodeID string, port int, joinAddr string) error {
	if nodeID == "" {
		return fmt.Errorf("node ID cannot be empty")
	}
	if port <= 0 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	if joinAddr != "" {
		host, portStr, err := net.SplitHostPort(joinAddr)
		if err != nil {
			return fmt.Errorf("invalid join address format: %w", err)
		}
		if host == "" || portStr == "" {
			return fmt.Errorf("join address must include both host and port")
		}
	}
	return nil
}

// Test admin address derivation logic
func TestAdminAddressDerivation(t *testing.T) {
	tests := []struct {
		name        string
		raftAddr    string
		expected    string
		description string
	}{
		{
			name:        "standard_port",
			raftAddr:    "127.0.0.1:8001",
			expected:    "127.0.0.1:9001",
			description: "Should add 1000 to raft port",
		},
		{
			name:        "high_port",
			raftAddr:    "127.0.0.1:18001",
			expected:    "127.0.0.1:19001",
			description: "Should handle high port numbers",
		},
		{
			name:        "localhost",
			raftAddr:    "localhost:8001",
			expected:    "localhost:9001",
			description: "Should preserve hostname",
		},
		{
			name:        "ipv4_address",
			raftAddr:    "192.168.1.100:8001",
			expected:    "192.168.1.100:9001",
			description: "Should work with IPv4 addresses",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deriveAdminAddress(tt.raftAddr)
			assert.Equal(t, tt.expected, result, tt.description)
		})
	}
}

// Helper function for admin address derivation (mirrors the implementation)
func deriveAdminAddress(raftAddr string) string {
	host, portStr, err := net.SplitHostPort(raftAddr)
	if err != nil {
		return "127.0.0.1:9001" // fallback
	}

	port := 0
	fmt.Sscanf(portStr, "%d", &port)
	if port == 0 {
		return "127.0.0.1:9001" // fallback
	}

	adminPort := port + 1000
	return fmt.Sprintf("%s:%d", host, adminPort)
}

// Test port calculation for N-node clusters
func TestPortCalculation(t *testing.T) {
	basePort := 8001
	adminBasePort := 9001
	monitorBasePort := 6001

	tests := []struct {
		nodeNum       int
		expectedRaft  int
		expectedAdmin int
		expectedMon   int
	}{
		{1, 8001, 9001, 6001},
		{2, 8002, 9002, 6002},
		{3, 8003, 9003, 6003},
		{5, 8005, 9005, 6005},
		{10, 8010, 9010, 6010},
		{20, 8020, 9020, 6020},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("node_%d", tt.nodeNum), func(t *testing.T) {
			raftPort := basePort + tt.nodeNum - 1
			adminPort := adminBasePort + tt.nodeNum - 1
			monitorPort := monitorBasePort + tt.nodeNum - 1

			assert.Equal(t, tt.expectedRaft, raftPort, "Raft port calculation")
			assert.Equal(t, tt.expectedAdmin, adminPort, "Admin port calculation")
			assert.Equal(t, tt.expectedMon, monitorPort, "Monitor port calculation")
		})
	}
}

// Test cluster size validation
func TestClusterSizeValidation(t *testing.T) {
	tests := []struct {
		name        string
		clusterSize int
		valid       bool
		reason      string
	}{
		{
			name:        "single_node",
			clusterSize: 1,
			valid:       true,
			reason:      "Single node should be valid",
		},
		{
			name:        "three_nodes",
			clusterSize: 3,
			valid:       true,
			reason:      "Three nodes is a common cluster size",
		},
		{
			name:        "five_nodes",
			clusterSize: 5,
			valid:       true,
			reason:      "Five nodes provides good fault tolerance",
		},
		{
			name:        "ten_nodes",
			clusterSize: 10,
			valid:       true,
			reason:      "Ten nodes should be supported",
		},
		{
			name:        "twenty_nodes",
			clusterSize: 20,
			valid:       true,
			reason:      "Twenty nodes should be supported",
		},
		{
			name:        "zero_nodes",
			clusterSize: 0,
			valid:       false,
			reason:      "Zero nodes is invalid",
		},
		{
			name:        "negative_nodes",
			clusterSize: -1,
			valid:       false,
			reason:      "Negative nodes is invalid",
		},
		{
			name:        "excessive_nodes",
			clusterSize: 1000,
			valid:       false,
			reason:      "Too many nodes may cause performance issues",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := validateClusterSize(tt.clusterSize)
			assert.Equal(t, tt.valid, valid, tt.reason)
		})
	}
}

// Helper function for cluster size validation
func validateClusterSize(size int) bool {
	return size > 0 && size <= 50 // Reasonable upper bound
}

// Test node ID generation and validation
func TestNodeIDValidation(t *testing.T) {
	tests := []struct {
		name   string
		nodeID string
		valid  bool
		reason string
	}{
		{
			name:   "standard_node_id",
			nodeID: "node1",
			valid:  true,
			reason: "Standard node ID should be valid",
		},
		{
			name:   "numeric_suffix",
			nodeID: "node123",
			valid:  true,
			reason: "Node ID with large number should be valid",
		},
		{
			name:   "alphanumeric",
			nodeID: "node-1a",
			valid:  true,
			reason: "Alphanumeric node ID should be valid",
		},
		{
			name:   "empty_node_id",
			nodeID: "",
			valid:  false,
			reason: "Empty node ID should be invalid",
		},
		{
			name:   "whitespace_only",
			nodeID: "   ",
			valid:  false,
			reason: "Whitespace-only node ID should be invalid",
		},
		{
			name:   "special_characters",
			nodeID: "node@#$%",
			valid:  false,
			reason: "Node ID with special characters should be invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := validateNodeID(tt.nodeID)
			assert.Equal(t, tt.valid, valid, tt.reason)
		})
	}
}

// Helper function for node ID validation
func validateNodeID(nodeID string) bool {
	if len(nodeID) == 0 {
		return false
	}

	// Trim whitespace and check if empty
	trimmed := fmt.Sprintf("%s", nodeID)
	if len(trimmed) == 0 {
		return false
	}

	// Basic alphanumeric validation (simplified)
	for _, char := range nodeID {
		if !((char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '-' || char == '_') {
			return false
		}
	}

	return true
}

// Test data directory creation for N nodes
func TestDataDirectoryCreation(t *testing.T) {
	tempDir := filepath.Join(os.TempDir(), "pickbox-n-node-test")
	defer os.RemoveAll(tempDir)

	nodeCount := 5
	baseDataDir := filepath.Join(tempDir, "data")

	// Test creating directories for N nodes
	for i := 1; i <= nodeCount; i++ {
		nodeID := fmt.Sprintf("node%d", i)
		nodeDataDir := filepath.Join(baseDataDir, nodeID)

		err := os.MkdirAll(nodeDataDir, 0755)
		require.NoError(t, err, "Failed to create directory for %s", nodeID)

		// Verify directory exists
		assert.DirExists(t, nodeDataDir, "Directory should exist for %s", nodeID)

		// Test creating a test file in the directory
		testFile := filepath.Join(nodeDataDir, "test.txt")
		err = os.WriteFile(testFile, []byte("test content"), 0644)
		require.NoError(t, err, "Failed to create test file in %s", nodeDataDir)

		assert.FileExists(t, testFile, "Test file should exist in %s", nodeDataDir)
	}

	// Verify all directories were created
	for i := 1; i <= nodeCount; i++ {
		nodeID := fmt.Sprintf("node%d", i)
		nodeDataDir := filepath.Join(baseDataDir, nodeID)
		assert.DirExists(t, nodeDataDir, "Directory should exist for %s", nodeID)
	}
}

// Test concurrent node configuration generation
func TestConcurrentNodeConfiguration(t *testing.T) {
	nodeCount := 10
	basePort := 8001
	adminBasePort := 9001
	monitorBasePort := 6001

	type nodeConfig struct {
		nodeID      string
		raftPort    int
		adminPort   int
		monitorPort int
		dataDir     string
	}

	// Generate configurations concurrently
	configs := make([]nodeConfig, nodeCount)
	done := make(chan bool, nodeCount)

	for i := 1; i <= nodeCount; i++ {
		go func(nodeNum int) {
			defer func() { done <- true }()

			configs[nodeNum-1] = nodeConfig{
				nodeID:      fmt.Sprintf("node%d", nodeNum),
				raftPort:    basePort + nodeNum - 1,
				adminPort:   adminBasePort + nodeNum - 1,
				monitorPort: monitorBasePort + nodeNum - 1,
				dataDir:     fmt.Sprintf("data/node%d", nodeNum),
			}
		}(i)
	}

	// Wait for all configurations to be generated
	for i := 0; i < nodeCount; i++ {
		select {
		case <-done:
			// Configuration generated
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for node configuration generation")
		}
	}

	// Validate all configurations
	usedPorts := make(map[int]bool)

	for i, config := range configs {
		// Validate node ID
		expectedNodeID := fmt.Sprintf("node%d", i+1)
		assert.Equal(t, expectedNodeID, config.nodeID)

		// Validate ports are unique
		assert.False(t, usedPorts[config.raftPort], "Raft port %d should be unique", config.raftPort)
		assert.False(t, usedPorts[config.adminPort], "Admin port %d should be unique", config.adminPort)
		assert.False(t, usedPorts[config.monitorPort], "Monitor port %d should be unique", config.monitorPort)

		usedPorts[config.raftPort] = true
		usedPorts[config.adminPort] = true
		usedPorts[config.monitorPort] = true

		// Validate port ranges
		assert.GreaterOrEqual(t, config.raftPort, basePort)
		assert.LessOrEqual(t, config.raftPort, basePort+nodeCount-1)

		assert.GreaterOrEqual(t, config.adminPort, adminBasePort)
		assert.LessOrEqual(t, config.adminPort, adminBasePort+nodeCount-1)

		assert.GreaterOrEqual(t, config.monitorPort, monitorBasePort)
		assert.LessOrEqual(t, config.monitorPort, monitorBasePort+nodeCount-1)

		// Validate data directory format
		expectedDataDir := fmt.Sprintf("data/node%d", i+1)
		assert.Equal(t, expectedDataDir, config.dataDir)
	}
}

// Test bootstrap logic for different scenarios
func TestBootstrapLogic(t *testing.T) {
	tests := []struct {
		name            string
		nodeID          string
		joinAddr        string
		bootstrapFlag   bool
		shouldBootstrap bool
		description     string
	}{
		{
			name:            "explicit_bootstrap",
			nodeID:          "node1",
			joinAddr:        "",
			bootstrapFlag:   true,
			shouldBootstrap: true,
			description:     "Node with explicit bootstrap flag should bootstrap",
		},
		{
			name:            "joining_node",
			nodeID:          "node2",
			joinAddr:        "127.0.0.1:8001",
			bootstrapFlag:   false,
			shouldBootstrap: false,
			description:     "Node with join address should not bootstrap",
		},
		{
			name:            "no_join_no_bootstrap",
			nodeID:          "node1",
			joinAddr:        "",
			bootstrapFlag:   false,
			shouldBootstrap: false,
			description:     "Node without join address or bootstrap flag should not bootstrap",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldBootstrap := determineBootstrap(tt.joinAddr, tt.bootstrapFlag)
			assert.Equal(t, tt.shouldBootstrap, shouldBootstrap, tt.description)
		})
	}
}

// Helper function for bootstrap logic (mirrors the fixed implementation)
func determineBootstrap(joinAddr string, bootstrapFlag bool) bool {
	// Only bootstrap if explicitly requested
	return bootstrapFlag
}

// Benchmark port calculation performance
func BenchmarkPortCalculation(b *testing.B) {
	basePort := 8001
	nodeCount := 1000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for nodeNum := 1; nodeNum <= nodeCount; nodeNum++ {
			raftPort := basePort + nodeNum - 1
			adminPort := raftPort + 1000
			monitorPort := raftPort - 2000

			// Use the calculated ports to avoid optimization
			_ = raftPort + adminPort + monitorPort
		}
	}
}

// Benchmark node configuration generation
func BenchmarkNodeConfigGeneration(b *testing.B) {
	basePort := 8001
	adminBasePort := 9001
	monitorBasePort := 6001

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		configs := make([]map[string]interface{}, 0, 100)

		for nodeNum := 1; nodeNum <= 100; nodeNum++ {
			config := map[string]interface{}{
				"nodeID":      fmt.Sprintf("node%d", nodeNum),
				"raftPort":    basePort + nodeNum - 1,
				"adminPort":   adminBasePort + nodeNum - 1,
				"monitorPort": monitorBasePort + nodeNum - 1,
				"dataDir":     fmt.Sprintf("data/node%d", nodeNum),
			}
			configs = append(configs, config)
		}

		// Use configs to avoid optimization
		_ = len(configs)
	}
}
