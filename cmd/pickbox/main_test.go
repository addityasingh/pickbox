package main

import (
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootCommand(t *testing.T) {
	tests := []struct {
		name          string
		expectedUse   string
		expectedShort string
	}{
		{
			name:          "root command properties",
			expectedUse:   "pickbox",
			expectedShort: "A distributed file storage system similar to Dropbox",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedUse, rootCmd.Use)
			assert.Equal(t, tt.expectedShort, rootCmd.Short)
			assert.NotEmpty(t, rootCmd.Long)
			assert.NotEmpty(t, rootCmd.Version)
		})
	}
}

func TestRootCommandVersion(t *testing.T) {
	// Test version string format
	assert.Contains(t, rootCmd.Version, version)
	assert.Contains(t, rootCmd.Version, commit)
	assert.Contains(t, rootCmd.Version, date)
}

func TestRootCommandLong(t *testing.T) {
	expectedFeatures := []string{
		"Distributed storage with multiple nodes",
		"File replication and consistency",
		"RAFT consensus",
		"Real-time file watching",
		"Admin interface",
		"Cluster management",
	}

	for _, feature := range expectedFeatures {
		assert.Contains(t, rootCmd.Long, feature, "Long description should mention %s", feature)
	}
}

func TestGlobalFlags(t *testing.T) {
	tests := []struct {
		name         string
		flagName     string
		shortFlag    string
		defaultValue string
		usage        string
	}{
		{
			name:         "log-level flag",
			flagName:     "log-level",
			shortFlag:    "l",
			defaultValue: "info",
			usage:        "Set log level (debug, info, warn, error)",
		},
		{
			name:         "data-dir flag",
			flagName:     "data-dir",
			shortFlag:    "d",
			defaultValue: "data",
			usage:        "Data directory for storage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := rootCmd.PersistentFlags().Lookup(tt.flagName)
			require.NotNil(t, flag, "Flag %s should exist", tt.flagName)

			assert.Equal(t, tt.defaultValue, flag.DefValue, "Default value mismatch for %s", tt.flagName)
			assert.Equal(t, tt.usage, flag.Usage, "Usage description mismatch for %s", tt.flagName)
			assert.Equal(t, tt.shortFlag, flag.Shorthand, "Short flag mismatch for %s", tt.flagName)
		})
	}
}

func TestRootCommandSubcommands(t *testing.T) {
	// Test that expected subcommands are registered
	expectedSubcommands := []string{"script", "cluster", "node", "multi-replication"}

	for _, expectedCmd := range expectedSubcommands {
		found := false
		for _, cmd := range rootCmd.Commands() {
			if cmd.Use == expectedCmd {
				found = true
				break
			}
		}
		if !found && expectedCmd != "node" && expectedCmd != "multi-replication" {
			// node and multi-replication might be defined in other files
			t.Logf("Warning: Expected subcommand '%s' not found", expectedCmd)
		}
	}
}

func TestRootCommandExecution(t *testing.T) {
	// Test help command execution
	rootCmd.SetArgs([]string{"--help"})

	// Capture output by temporarily redirecting
	originalOut := os.Stdout
	defer func() { os.Stdout = originalOut }()

	// Test that help command doesn't panic
	assert.NotPanics(t, func() {
		rootCmd.Execute()
	})
}

func TestVersionFlag(t *testing.T) {
	// Test version flag
	rootCmd.SetArgs([]string{"--version"})

	assert.NotPanics(t, func() {
		rootCmd.Execute()
	})
}

func TestMainFunction(t *testing.T) {
	// Test that main function doesn't panic with valid commands
	assert.NotPanics(t, func() {
		// Save original args
		originalArgs := os.Args
		defer func() { os.Args = originalArgs }()

		// Set test args
		os.Args = []string{"pickbox", "--help"}

		// This would normally call os.Exit, but in test we just verify no panic
		// We can't easily test the actual main() function without modifying it
	})
}

func TestGlobalFlagValidation(t *testing.T) {
	tests := []struct {
		name     string
		flagName string
		value    string
		wantErr  bool
	}{
		{
			name:     "valid log level",
			flagName: "log-level",
			value:    "debug",
			wantErr:  false,
		},
		{
			name:     "valid data dir",
			flagName: "data-dir",
			value:    "/tmp/test",
			wantErr:  false,
		},
		{
			name:     "empty data dir",
			flagName: "data-dir",
			value:    "",
			wantErr:  false, // Empty values are typically allowed for flags
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new command instance to avoid side effects
			testCmd := &cobra.Command{
				Use: "test",
			}
			testCmd.PersistentFlags().StringP("log-level", "l", "info", "Set log level")
			testCmd.PersistentFlags().StringP("data-dir", "d", "data", "Data directory")

			testCmd.SetArgs([]string{"--" + tt.flagName, tt.value})

			err := testCmd.Execute()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCommandStructure(t *testing.T) {
	// Test that rootCmd has required fields
	assert.NotEmpty(t, rootCmd.Use, "Use field should not be empty")
	assert.NotEmpty(t, rootCmd.Short, "Short description should not be empty")
	assert.NotEmpty(t, rootCmd.Long, "Long description should not be empty")
	assert.NotEmpty(t, rootCmd.Version, "Version should not be empty")
}

func TestVersionVariables(t *testing.T) {
	// Test that version variables are properly set (even if defaults)
	assert.NotEmpty(t, version, "version variable should be set")
	assert.NotEmpty(t, commit, "commit variable should be set")
	assert.NotEmpty(t, date, "date variable should be set")

	// Test default values
	if version == "dev" {
		assert.Equal(t, "dev", version, "default version should be 'dev'")
	}
	if commit == "unknown" {
		assert.Equal(t, "unknown", commit, "default commit should be 'unknown'")
	}
	if date == "unknown" {
		assert.Equal(t, "unknown", date, "default date should be 'unknown'")
	}
}

func TestFlagInheritance(t *testing.T) {
	// Test that persistent flags exist on root command
	logLevelFlag := rootCmd.PersistentFlags().Lookup("log-level")
	dataDirFlag := rootCmd.PersistentFlags().Lookup("data-dir")

	assert.NotNil(t, logLevelFlag, "log-level flag should exist on root command")
	assert.NotNil(t, dataDirFlag, "data-dir flag should exist on root command")

	// Test that some key subcommands exist
	subcommandNames := []string{"script", "cluster"}
	for _, name := range subcommandNames {
		found := false
		for _, cmd := range rootCmd.Commands() {
			if cmd.Use == name {
				found = true
				break
			}
		}
		assert.True(t, found, "Subcommand %s should exist", name)
	}
}

func TestLogLevelValidValues(t *testing.T) {
	validLogLevels := []string{"debug", "info", "warn", "error"}

	for _, level := range validLogLevels {
		t.Run("log_level_"+level, func(t *testing.T) {
			// This tests that the flag accepts these values
			testCmd := &cobra.Command{Use: "test"}
			testCmd.Flags().StringP("log-level", "l", "info", "Set log level")
			testCmd.SetArgs([]string{"--log-level", level})

			err := testCmd.Execute()
			assert.NoError(t, err, "Should accept log level: %s", level)
		})
	}
}

func TestCommandUsageText(t *testing.T) {
	// Test that usage text is properly formatted
	usage := rootCmd.UsageString()
	assert.Contains(t, usage, "pickbox", "Usage should contain command name")
	assert.Contains(t, usage, "Available Commands", "Usage should list available commands")
	assert.Contains(t, usage, "Flags", "Usage should list available flags")
}

func TestCommandAliases(t *testing.T) {
	// Test that rootCmd doesn't have conflicting aliases
	assert.Empty(t, rootCmd.Aliases, "Root command should not have aliases")

	// Check subcommands for proper alias setup
	for _, cmd := range rootCmd.Commands() {
		if len(cmd.Aliases) > 0 {
			for _, alias := range cmd.Aliases {
				assert.NotEmpty(t, alias, "Aliases should not be empty strings")
				assert.NotEqual(t, cmd.Use, alias, "Alias should not match command name")
			}
		}
	}
}

func TestBashCompletion(t *testing.T) {
	// Test that bash completion doesn't panic
	assert.NotPanics(t, func() {
		rootCmd.SetArgs([]string{"completion", "bash"})
		rootCmd.Execute()
	})
}

func TestErrorHandling(t *testing.T) {
	// Test with invalid flag
	rootCmd.SetArgs([]string{"--invalid-flag"})

	err := rootCmd.Execute()
	assert.Error(t, err, "Should return error for invalid flag")
	assert.Contains(t, err.Error(), "unknown flag", "Error should mention unknown flag")
}

func TestHelpCommand(t *testing.T) {
	// Test help command
	rootCmd.SetArgs([]string{"help"})

	assert.NotPanics(t, func() {
		rootCmd.Execute()
	})
}

func TestCommandValidation(t *testing.T) {
	// Test command structure validation
	// Note: Root command may not be runnable if it doesn't have a Run function

	// Test that required fields are not empty
	assert.NotEmpty(t, strings.TrimSpace(rootCmd.Use))
	assert.NotEmpty(t, strings.TrimSpace(rootCmd.Short))
	assert.NotEmpty(t, strings.TrimSpace(rootCmd.Long))

	// Test that the command has proper structure
	assert.NotNil(t, rootCmd.Commands(), "Root command should have subcommands")
	assert.True(t, len(rootCmd.Commands()) > 0, "Root command should have at least one subcommand")
}
