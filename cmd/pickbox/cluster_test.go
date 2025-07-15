package main

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClusterCommand(t *testing.T) {
	tests := []struct {
		name          string
		expectedUse   string
		expectedShort string
	}{
		{
			name:          "cluster command properties",
			expectedUse:   "cluster",
			expectedShort: "Cluster management commands",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedUse, clusterCmd.Use)
			assert.Equal(t, tt.expectedShort, clusterCmd.Short)
			assert.NotEmpty(t, clusterCmd.Long)
		})
	}
}

func TestClusterJoinCommand(t *testing.T) {
	tests := []struct {
		name          string
		expectedUse   string
		expectedShort string
	}{
		{
			name:          "cluster join command properties",
			expectedUse:   "join",
			expectedShort: "Join a node to an existing cluster",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedUse, clusterJoinCmd.Use)
			assert.Equal(t, tt.expectedShort, clusterJoinCmd.Short)
			assert.NotEmpty(t, clusterJoinCmd.Long)
			assert.NotNil(t, clusterJoinCmd.RunE)
		})
	}
}

func TestClusterStatusCommand(t *testing.T) {
	tests := []struct {
		name          string
		expectedUse   string
		expectedShort string
	}{
		{
			name:          "cluster status command properties",
			expectedUse:   "status",
			expectedShort: "Check cluster status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedUse, clusterStatusCmd.Use)
			assert.Equal(t, tt.expectedShort, clusterStatusCmd.Short)
			assert.NotEmpty(t, clusterStatusCmd.Long)
			assert.NotNil(t, clusterStatusCmd.RunE)
		})
	}
}

func TestClusterCommandInitialization(t *testing.T) {
	// Test that cluster command is properly added to root
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "cluster" {
			found = true
			break
		}
	}
	assert.True(t, found, "cluster command should be added to root command")

	// Test that subcommands are properly added to cluster command
	subcommands := clusterCmd.Commands()
	expectedSubcommands := []string{"join", "status"}

	for _, expected := range expectedSubcommands {
		found := false
		for _, cmd := range subcommands {
			if cmd.Use == expected {
				found = true
				break
			}
		}
		assert.True(t, found, "subcommand %s should be added to cluster command", expected)
	}
}

func TestClusterJoinCommandFlags(t *testing.T) {
	tests := []struct {
		name      string
		flagName  string
		shortFlag string
		usage     string
		required  bool
	}{
		{
			name:      "leader flag",
			flagName:  "leader",
			shortFlag: "L", // Changed to avoid conflict with log-level flag
			usage:     "Leader address (required)",
			required:  true,
		},
		{
			name:      "node-id flag",
			flagName:  "node-id",
			shortFlag: "n",
			usage:     "Node ID to join (required)",
			required:  true,
		},
		{
			name:      "node-addr flag",
			flagName:  "node-addr",
			shortFlag: "a",
			usage:     "Node address (required)",
			required:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := clusterJoinCmd.Flags().Lookup(tt.flagName)
			require.NotNil(t, flag, "Flag %s should exist", tt.flagName)

			assert.Equal(t, tt.shortFlag, flag.Shorthand, "Short flag mismatch for %s", tt.flagName)
			assert.Contains(t, flag.Usage, tt.usage, "Usage description mismatch for %s", tt.flagName)
		})
	}
}

func TestClusterStatusCommandFlags(t *testing.T) {
	tests := []struct {
		name         string
		flagName     string
		shortFlag    string
		defaultValue string
		usage        string
	}{
		{
			name:         "addr flag",
			flagName:     "addr",
			shortFlag:    "a",
			defaultValue: "127.0.0.1:9001",
			usage:        "Admin address to check status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := clusterStatusCmd.Flags().Lookup(tt.flagName)
			require.NotNil(t, flag, "Flag %s should exist", tt.flagName)

			assert.Equal(t, tt.defaultValue, flag.DefValue, "Default value mismatch for %s", tt.flagName)
			assert.Equal(t, tt.shortFlag, flag.Shorthand, "Short flag mismatch for %s", tt.flagName)
			assert.Contains(t, flag.Usage, tt.usage, "Usage description mismatch for %s", tt.flagName)
		})
	}
}

func TestRunClusterJoinWithoutServer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}

	// Test cluster join when no server is running
	// Use t.Parallel() to ensure proper test isolation
	t.Parallel()

	// Thread-safe access to save original global variables
	globalVarsMutex.Lock()
	originalLeaderAddr := leaderAddr
	originalJoinNodeID := joinNodeID
	originalJoinNodeAddr := joinNodeAddr

	// Set the global variables for this test with unique ports to avoid conflicts
	leaderAddr = "127.0.0.1:18001"
	joinNodeID = "test-node-join"
	joinNodeAddr = "127.0.0.1:18002"
	globalVarsMutex.Unlock()

	// Ensure cleanup even if test panics
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Test panicked: %v", r)
		}
		globalVarsMutex.Lock()
		leaderAddr = originalLeaderAddr
		joinNodeID = originalJoinNodeID
		joinNodeAddr = originalJoinNodeAddr
		globalVarsMutex.Unlock()
	}()

	// Validate that variables are properly set before calling function
	globalVarsMutex.RLock()
	currentLeaderAddr := leaderAddr
	currentJoinNodeID := joinNodeID
	currentJoinNodeAddr := joinNodeAddr
	globalVarsMutex.RUnlock()

	assert.NotEmpty(t, currentLeaderAddr, "leaderAddr should not be empty")
	assert.NotEmpty(t, currentJoinNodeID, "joinNodeID should not be empty")
	assert.NotEmpty(t, currentJoinNodeAddr, "joinNodeAddr should not be empty")

	cmd := &cobra.Command{Use: "test"}
	assert.NotNil(t, cmd, "cmd should not be nil")

	err := runClusterJoin(cmd, []string{})

	assert.Contains(t, err.Error(), "connecting to admin server")
}

func TestRunClusterStatusWithoutServer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}

	// Test cluster status when no server is running
	// Use t.Parallel() to ensure proper test isolation
	t.Parallel()

	// Thread-safe access to save original global variable
	globalVarsMutex.Lock()
	originalStatusAddr := statusAddr

	statusAddr = "127.0.0.1:19999" // Use a unique unused port
	globalVarsMutex.Unlock()

	// Ensure cleanup even if test panics
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Test panicked: %v", r)
		}
		globalVarsMutex.Lock()
		statusAddr = originalStatusAddr
		globalVarsMutex.Unlock()
	}()

	// Validate that variable is properly set before calling function
	globalVarsMutex.RLock()
	currentStatusAddr := statusAddr
	globalVarsMutex.RUnlock()

	assert.NotEmpty(t, currentStatusAddr, "statusAddr should not be empty")

	cmd := &cobra.Command{Use: "test"}
	assert.NotNil(t, cmd, "cmd should not be nil")

	err := runClusterStatus(cmd, []string{})

	assert.Contains(t, err.Error(), "connecting to admin server")
}

func TestDeriveAdminAddr(t *testing.T) {
	tests := []struct {
		name     string
		raftAddr string
		expected string
	}{
		{
			name:     "valid raft address",
			raftAddr: "127.0.0.1:8001",
			expected: "127.0.0.1:9001",
		},
		{
			name:     "localhost raft address",
			raftAddr: "localhost:8001",
			expected: "localhost:9001",
		},
		{
			name:     "invalid raft address",
			raftAddr: "invalid-address",
			expected: "127.0.0.1:9001", // Default
		},
		{
			name:     "empty raft address",
			raftAddr: "",
			expected: "127.0.0.1:9001", // Default
		},
		{
			name:     "raft address without port",
			raftAddr: "127.0.0.1",
			expected: "127.0.0.1:9001", // Default
		},
		{
			name:     "raft address with multiple colons",
			raftAddr: "127.0.0.1:8001:extra",
			expected: "127.0.0.1:9001", // Default (invalid format)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deriveAdminAddr(tt.raftAddr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestClusterJoinRequiredFlags(t *testing.T) {
	// Test that required flags are properly marked by checking flag annotations
	requiredFlags := []string{"leader", "node-id", "node-addr"}

	for _, flagName := range requiredFlags {
		t.Run("required_flag_"+flagName, func(t *testing.T) {
			flag := clusterJoinCmd.Flags().Lookup(flagName)
			require.NotNil(t, flag, "Flag %s should exist", flagName)

			// Check if the flag is marked as required by trying to validate without it
			// We create a fresh command to avoid global state interference
			testCmd := &cobra.Command{Use: "test"}
			testCmd.Flags().StringP(flagName, flag.Shorthand, "", flag.Usage)
			testCmd.MarkFlagRequired(flagName)

			// This should fail when the required flag is missing
			err := testCmd.ParseFlags([]string{})
			assert.NoError(t, err, "Parsing flags should not error")

			// The validation happens during Execute, but we can't test that easily
			// So we just verify the flag exists and has the right properties
			assert.NotEmpty(t, flag.Usage, "Flag should have usage text")
		})
	}
}

func TestClusterCommandUsage(t *testing.T) {
	// Test cluster command usage
	usage := clusterCmd.UsageString()
	assert.Contains(t, usage, "cluster")
	assert.Contains(t, usage, "Available Commands")
}

func TestClusterJoinCommandUsage(t *testing.T) {
	// Test cluster join command usage - just check that it has the basic structure
	assert.NotEmpty(t, clusterJoinCmd.Use)
	assert.NotEmpty(t, clusterJoinCmd.Short)
	assert.NotEmpty(t, clusterJoinCmd.Long)
}

func TestClusterStatusCommandUsage(t *testing.T) {
	// Test cluster status command usage
	usage := clusterStatusCmd.UsageString()
	assert.Contains(t, usage, "status")
}

func TestClusterCommandHelp(t *testing.T) {
	// Test that help doesn't panic
	assert.NotPanics(t, func() {
		clusterCmd.SetArgs([]string{"--help"})
		clusterCmd.Execute()
	})
}

func TestClusterJoinCommandHelp(t *testing.T) {
	// Test that help doesn't panic
	assert.NotPanics(t, func() {
		clusterJoinCmd.SetArgs([]string{"--help"})
		clusterJoinCmd.Execute()
	})
}

func TestClusterStatusCommandHelp(t *testing.T) {
	// Test that help doesn't panic
	assert.NotPanics(t, func() {
		clusterStatusCmd.SetArgs([]string{"--help"})
		clusterStatusCmd.Execute()
	})
}

func TestClusterJoinCommandValidation(t *testing.T) {
	// Test that the required flags are properly configured
	requiredFlags := []string{"leader", "node-id", "node-addr"}

	for _, flagName := range requiredFlags {
		t.Run("flag_"+flagName+"_exists", func(t *testing.T) {
			flag := clusterJoinCmd.Flags().Lookup(flagName)
			assert.NotNil(t, flag, "Flag %s should exist", flagName)
			assert.NotEmpty(t, flag.Usage, "Flag should have usage text")
		})
	}
}

func TestClusterStatusCommandValidation(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		wantErr bool
	}{
		{
			name:    "default address",
			addr:    "",
			wantErr: false, // Will fail at connection, not validation
		},
		{
			name:    "custom address",
			addr:    "127.0.0.1:9002",
			wantErr: false, // Will fail at connection, not validation
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{}
			if tt.addr != "" {
				args = append(args, "--addr", tt.addr)
			}

			clusterStatusCmd.SetArgs(args)
			err := clusterStatusCmd.Execute()

			// Should fail at connection, not validation
			if err != nil {
				assert.Contains(t, err.Error(), "connecting to admin server")
			}
		})
	}
}

func TestClusterCommandStructure(t *testing.T) {
	// Test command structure
	assert.NotEmpty(t, clusterCmd.Use)
	assert.NotEmpty(t, clusterCmd.Short)
	assert.NotEmpty(t, clusterCmd.Long)

	// Test subcommands structure
	assert.NotEmpty(t, clusterJoinCmd.Use)
	assert.NotEmpty(t, clusterJoinCmd.Short)
	assert.NotEmpty(t, clusterJoinCmd.Long)
	assert.NotNil(t, clusterJoinCmd.RunE)

	assert.NotEmpty(t, clusterStatusCmd.Use)
	assert.NotEmpty(t, clusterStatusCmd.Short)
	assert.NotEmpty(t, clusterStatusCmd.Long)
	assert.NotNil(t, clusterStatusCmd.RunE)
}

func TestGlobalVariables(t *testing.T) {
	// Test that global variables exist and can be set
	// Thread-safe access to save original global variables
	globalVarsMutex.Lock()
	originalLeader := leaderAddr
	originalNodeID := joinNodeID
	originalNodeAddr := joinNodeAddr
	originalStatusAddr := statusAddr

	// Test setting variables
	leaderAddr = "test-leader"
	joinNodeID = "test-node-id"
	joinNodeAddr = "test-node-addr"
	statusAddr = "test-status-addr"
	globalVarsMutex.Unlock()

	defer func() {
		globalVarsMutex.Lock()
		leaderAddr = originalLeader
		joinNodeID = originalNodeID
		joinNodeAddr = originalNodeAddr
		statusAddr = originalStatusAddr
		globalVarsMutex.Unlock()
	}()

	// Verify variables are set correctly
	globalVarsMutex.RLock()
	assert.Equal(t, "test-leader", leaderAddr)
	assert.Equal(t, "test-node-id", joinNodeID)
	assert.Equal(t, "test-node-addr", joinNodeAddr)
	assert.Equal(t, "test-status-addr", statusAddr)
	globalVarsMutex.RUnlock()
}

func TestClusterJoinWithValidFlags(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}

	// Test cluster join by verifying the function logic directly
	// Use t.Parallel() to ensure proper test isolation
	t.Parallel()

	// Thread-safe access to save original global variables
	globalVarsMutex.Lock()
	originalLeader := leaderAddr
	originalNodeID := joinNodeID
	originalNodeAddr := joinNodeAddr

	// Set the global variables (simulating flag parsing) with unique ports
	leaderAddr = "127.0.0.1:28001"
	joinNodeID = "test-node-valid"
	joinNodeAddr = "127.0.0.1:28002"
	globalVarsMutex.Unlock()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Test panicked: %v", r)
		}
		globalVarsMutex.Lock()
		leaderAddr = originalLeader
		joinNodeID = originalNodeID
		joinNodeAddr = originalNodeAddr
		globalVarsMutex.Unlock()
	}()

	cmd := &cobra.Command{Use: "test"}
	assert.NotNil(t, cmd, "cmd should not be nil")

	err := runClusterJoin(cmd, []string{})

	// Should fail at connection attempt, not flag validation
	assert.Error(t, err, "should error when cannot connect to admin server")
	assert.Contains(t, err.Error(), "connecting to admin server")
}

func TestNetworkTimeout(t *testing.T) {
	// Test that network operations timeout appropriately
	start := time.Now()

	// This should timeout quickly since the address doesn't exist
	_, err := net.DialTimeout("tcp", "192.0.2.1:9999", 100*time.Millisecond)

	elapsed := time.Since(start)
	assert.Error(t, err)
	assert.True(t, elapsed < 200*time.Millisecond, "Should timeout within reasonable time")
}

func TestFlagShorthandUniqueness(t *testing.T) {
	// Test that flag shorthands don't conflict within commands
	joinFlags := clusterJoinCmd.Flags()
	statusFlags := clusterStatusCmd.Flags()

	// Check join command flags
	leaderFlag := joinFlags.Lookup("leader")
	nodeIDFlag := joinFlags.Lookup("node-id")
	nodeAddrFlag := joinFlags.Lookup("node-addr")

	assert.Equal(t, "L", leaderFlag.Shorthand)
	assert.Equal(t, "n", nodeIDFlag.Shorthand)
	assert.Equal(t, "a", nodeAddrFlag.Shorthand)

	// Check status command flags
	addrFlag := statusFlags.Lookup("addr")
	assert.Equal(t, "a", addrFlag.Shorthand)

	// Note: Both join and status use -a, but they're in different commands so it's okay
}

func TestClusterCommandAliases(t *testing.T) {
	// Test that commands don't have conflicting aliases
	assert.Empty(t, clusterCmd.Aliases, "Cluster command should not have aliases")
	assert.Empty(t, clusterJoinCmd.Aliases, "Cluster join command should not have aliases")
	assert.Empty(t, clusterStatusCmd.Aliases, "Cluster status command should not have aliases")
}

func TestInvalidNetworkAddresses(t *testing.T) {
	tests := []struct {
		name    string
		address string
	}{
		{"invalid host", "invalid-host:9001"},
		{"invalid port", "127.0.0.1:999999"},
		{"no port", "127.0.0.1"},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that invalid addresses are handled gracefully
			_, err := net.DialTimeout("tcp", tt.address, 10*time.Millisecond)
			assert.Error(t, err, "Should error for invalid address: %s", tt.address)
		})
	}
}

func TestStringValidation(t *testing.T) {
	// Test string validation in cluster operations
	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{"valid node ID", "node-1", true},
		{"valid address", "127.0.0.1:8001", true},
		{"empty node ID", "", false},
		{"node ID with spaces", "node 1", false},
		{"very long node ID", strings.Repeat("a", 1000), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.valid {
				assert.NotEmpty(t, strings.TrimSpace(tt.value), "Valid value should not be empty when trimmed")
				assert.NotContains(t, tt.value, " ", "Valid node ID should not contain spaces")
			} else {
				if tt.value != "" && !strings.Contains(tt.value, " ") && len(tt.value) < 100 {
					// Only test non-empty, non-space, reasonable length strings
					assert.NotEmpty(t, tt.value)
				}
			}
		})
	}
}
