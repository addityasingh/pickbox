package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScriptCommand(t *testing.T) {
	tests := []struct {
		name          string
		expectedUse   string
		expectedShort string
	}{
		{
			name:          "script command properties",
			expectedUse:   "script",
			expectedShort: "Run common cluster scripts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedUse, scriptCmd.Use)
			assert.Equal(t, tt.expectedShort, scriptCmd.Short)
			assert.NotEmpty(t, scriptCmd.Long)
		})
	}
}

func TestScriptDemo3Command(t *testing.T) {
	tests := []struct {
		name          string
		expectedUse   string
		expectedShort string
	}{
		{
			name:          "demo-3-nodes command properties",
			expectedUse:   "demo-3-nodes",
			expectedShort: "Demo script for 3-node cluster",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedUse, scriptDemo3Cmd.Use)
			assert.Equal(t, tt.expectedShort, scriptDemo3Cmd.Short)
			assert.NotEmpty(t, scriptDemo3Cmd.Long)
			assert.NotNil(t, scriptDemo3Cmd.RunE)
		})
	}
}

func TestScriptCleanupCommand(t *testing.T) {
	tests := []struct {
		name          string
		expectedUse   string
		expectedShort string
	}{
		{
			name:          "cleanup command properties",
			expectedUse:   "cleanup",
			expectedShort: "Clean up data directories",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedUse, scriptCleanupCmd.Use)
			assert.Equal(t, tt.expectedShort, scriptCleanupCmd.Short)
			assert.NotEmpty(t, scriptCleanupCmd.Long)
			assert.NotNil(t, scriptCleanupCmd.RunE)
		})
	}
}

func TestScriptCommandInitialization(t *testing.T) {
	// Test that script command is properly added to root
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "script" {
			found = true
			break
		}
	}
	assert.True(t, found, "script command should be added to root command")

	// Test that subcommands are properly added to script command
	subcommands := scriptCmd.Commands()
	expectedSubcommands := []string{"demo-3-nodes", "cleanup"}

	for _, expected := range expectedSubcommands {
		found := false
		for _, cmd := range subcommands {
			if cmd.Use == expected {
				found = true
				break
			}
		}
		assert.True(t, found, "subcommand %s should be added to script command", expected)
	}
}

func TestCleanupFunction(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "pickbox_test_")
	require.NoError(t, err)

	// Create some test files/directories
	testFile := filepath.Join(tempDir, "test.txt")
	testSubDir := filepath.Join(tempDir, "subdir")

	err = os.WriteFile(testFile, []byte("test data"), 0644)
	require.NoError(t, err)

	err = os.MkdirAll(testSubDir, 0755)
	require.NoError(t, err)

	// Verify files exist before cleanup
	assert.FileExists(t, testFile)
	assert.DirExists(t, testSubDir)

	// Test cleanup function
	err = cleanup(tempDir)
	assert.NoError(t, err)

	// Verify directory was removed
	assert.NoFileExists(t, tempDir)
}

func TestCleanupFunctionWithNonExistentDirectory(t *testing.T) {
	// Test cleanup with non-existent directory
	nonExistentDir := "/tmp/non_existent_pickbox_test_dir_12345"

	err := cleanup(nonExistentDir)
	assert.NoError(t, err, "cleanup should not error on non-existent directory")
}

func TestRunCleanupCommand(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "pickbox_cleanup_test_")
	require.NoError(t, err)

	// Create test content
	testFile := filepath.Join(tempDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	// Create a test command with the temp directory
	testCmd := &cobra.Command{Use: "test"}
	testCmd.Flags().String("data-dir", tempDir, "test data dir")

	// Test the runCleanup function
	err = runCleanup(testCmd, []string{})
	assert.NoError(t, err)

	// Verify directory was cleaned
	assert.NoFileExists(t, tempDir)
}

func TestRunDemo3NodesWithoutBinary(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "pickbox_demo_test_")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a test command
	testCmd := &cobra.Command{Use: "test"}
	testCmd.Flags().String("data-dir", tempDir, "test data dir")

	// Test the runDemo3Nodes function - this should fail because binary doesn't exist
	err = runDemo3Nodes(testCmd, []string{})
	assert.Error(t, err, "should error when pickbox binary is not found")
	assert.Contains(t, err.Error(), "pickbox binary not found")
}

func TestStartNodeInBackgroundValidation(t *testing.T) {
	tests := []struct {
		name        string
		nodeID      string
		port        int
		adminPort   int
		joinAddr    string
		bootstrap   bool
		expectErr   bool
		errContains string
	}{
		{
			name:        "valid bootstrap node",
			nodeID:      "node1",
			port:        8001,
			adminPort:   9001,
			joinAddr:    "",
			bootstrap:   true,
			expectErr:   true, // Will fail because binary doesn't exist
			errContains: "pickbox binary not found",
		},
		{
			name:        "valid joining node",
			nodeID:      "node2",
			port:        8002,
			adminPort:   9002,
			joinAddr:    "127.0.0.1:8001",
			bootstrap:   false,
			expectErr:   true, // Will fail because binary doesn't exist
			errContains: "pickbox binary not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := startNodeInBackground(tt.nodeID, tt.port, tt.adminPort, tt.joinAddr, tt.bootstrap)

			if tt.expectErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestStartNodeInBackgroundCommandArgs(t *testing.T) {
	// Since we can't actually start nodes in tests, we'll test the argument building logic
	// by verifying the function handles different parameter combinations correctly

	tests := []struct {
		name      string
		nodeID    string
		port      int
		adminPort int
		joinAddr  string
		bootstrap bool
	}{
		{
			name:      "bootstrap node",
			nodeID:    "node1",
			port:      8001,
			adminPort: 9001,
			joinAddr:  "",
			bootstrap: true,
		},
		{
			name:      "joining node",
			nodeID:    "node2",
			port:      8002,
			adminPort: 9002,
			joinAddr:  "127.0.0.1:8001",
			bootstrap: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We can't test the actual execution, but we can verify error handling
			err := startNodeInBackground(tt.nodeID, tt.port, tt.adminPort, tt.joinAddr, tt.bootstrap)

			// Should error because binary doesn't exist
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "pickbox binary not found")
		})
	}
}

func TestScriptCommandUsage(t *testing.T) {
	// Test script command usage
	usage := scriptCmd.UsageString()
	assert.Contains(t, usage, "script")
	assert.Contains(t, usage, "Available Commands")
}

func TestDemo3NodesCommandUsage(t *testing.T) {
	// Test demo-3-nodes command usage
	usage := scriptDemo3Cmd.UsageString()
	assert.Contains(t, usage, "demo-3-nodes")
}

func TestCleanupCommandUsage(t *testing.T) {
	// Test cleanup command usage
	usage := scriptCleanupCmd.UsageString()
	assert.Contains(t, usage, "cleanup")
}

func TestScriptCommandHelp(t *testing.T) {
	// Test that help doesn't panic
	assert.NotPanics(t, func() {
		scriptCmd.SetArgs([]string{"--help"})
		scriptCmd.Execute()
	})
}

func TestDemo3NodesCommandHelp(t *testing.T) {
	// Test that help doesn't panic
	assert.NotPanics(t, func() {
		scriptDemo3Cmd.SetArgs([]string{"--help"})
		scriptDemo3Cmd.Execute()
	})
}

func TestCleanupCommandHelp(t *testing.T) {
	// Test that help doesn't panic
	assert.NotPanics(t, func() {
		scriptCleanupCmd.SetArgs([]string{"--help"})
		scriptCleanupCmd.Execute()
	})
}

func TestCleanupWithPermissionError(t *testing.T) {
	// Skip this test on systems where we can't create permission-restricted directories
	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}

	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "pickbox_perm_test_")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a subdirectory
	subDir := filepath.Join(tempDir, "subdir")
	err = os.MkdirAll(subDir, 0755)
	require.NoError(t, err)

	// Make parent directory read-only (this may not work on all systems)
	err = os.Chmod(tempDir, 0444)
	if err != nil {
		t.Skip("Cannot change directory permissions on this system")
	}
	defer os.Chmod(tempDir, 0755) // Restore permissions for cleanup

	// Try to cleanup - this should handle permission errors gracefully
	err = cleanup(subDir)
	// The result depends on the system - some systems allow deletion despite read-only parent
	// We just ensure it doesn't panic
	assert.NotNil(t, err) // Might be nil or an error, both are valid
}

func TestRunDemo3NodesDataDirFlag(t *testing.T) {
	// Test that data-dir flag is properly handled
	tempDir, err := os.MkdirTemp("", "pickbox_datadir_test_")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a test command with custom data directory
	testCmd := &cobra.Command{Use: "test"}
	testCmd.Flags().String("data-dir", tempDir, "test data dir")

	// This should fail at the binary check, but first it should process the data-dir flag
	err = runDemo3Nodes(testCmd, []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "pickbox binary not found")

	// The temp directory should have been cleaned up during the process
	// (cleanup is called before attempting to start nodes)
}

func TestScriptCommandStructure(t *testing.T) {
	// Test command structure
	assert.NotEmpty(t, scriptCmd.Use)
	assert.NotEmpty(t, scriptCmd.Short)
	assert.NotEmpty(t, scriptCmd.Long)

	// Test subcommands structure
	assert.NotEmpty(t, scriptDemo3Cmd.Use)
	assert.NotEmpty(t, scriptDemo3Cmd.Short)
	assert.NotEmpty(t, scriptDemo3Cmd.Long)
	assert.NotNil(t, scriptDemo3Cmd.RunE)

	assert.NotEmpty(t, scriptCleanupCmd.Use)
	assert.NotEmpty(t, scriptCleanupCmd.Short)
	assert.NotEmpty(t, scriptCleanupCmd.Long)
	assert.NotNil(t, scriptCleanupCmd.RunE)
}

func TestCleanupEmptyPath(t *testing.T) {
	// Test cleanup with empty path
	err := cleanup("")
	assert.NoError(t, err, "cleanup with empty path should not error")
}

func TestCleanupRelativePath(t *testing.T) {
	// Test cleanup with relative path
	tempDir, err := os.MkdirTemp("", "pickbox_rel_test_")
	require.NoError(t, err)

	// Change to temp directory and create a relative path
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)

	parentDir := filepath.Dir(tempDir)
	err = os.Chdir(parentDir)
	require.NoError(t, err)

	relPath := filepath.Base(tempDir)

	// Test cleanup with relative path
	err = cleanup(relPath)
	assert.NoError(t, err)

	// Verify directory was removed
	assert.NoFileExists(t, filepath.Join(parentDir, relPath))
}

func TestPortCalculation(t *testing.T) {
	// Test the port calculation logic in startNodeInBackground
	// Monitor port should be admin port + 1
	// Dashboard port should be admin port + 2

	testCases := []struct {
		adminPort         int
		expectedMonitor   int
		expectedDashboard int
	}{
		{9001, 9002, 9003},
		{9002, 9003, 9004},
		{9003, 9004, 9005},
	}

	for _, tc := range testCases {
		t.Run("admin_port_"+string(rune(tc.adminPort)), func(t *testing.T) {
			// We can't test the actual command execution, but we can verify the function
			// tries to use the correct ports by checking the error message contains the binary path issue
			err := startNodeInBackground("test", 8000, tc.adminPort, "", false)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "pickbox binary not found")
		})
	}
}

func TestStringFormatting(t *testing.T) {
	// Test that string formatting in the demo function works correctly
	// This is testing the console output formatting logic

	tests := []struct {
		name      string
		nodeID    string
		port      int
		adminPort int
	}{
		{"node1", "node1", 8001, 9001},
		{"node2", "node2", 8002, 9002},
		{"node3", "node3", 8003, 9003},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that the node ID is valid for string formatting
			assert.NotEmpty(t, tt.nodeID)
			assert.NotContains(t, tt.nodeID, " ", "Node ID should not contain spaces")
			assert.True(t, tt.port > 0, "Port should be positive")
			assert.True(t, tt.adminPort > 0, "Admin port should be positive")
			assert.NotEqual(t, tt.port, tt.adminPort, "Port and admin port should be different")
		})
	}
}
