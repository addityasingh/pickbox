package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "pickbox",
	Short: "A distributed file storage system similar to Dropbox",
	Long: `Pickbox is a distributed file storage system with replication and consistency guarantees.
It supports file operations (OPEN, READ, WRITE, CLOSE) across multiple nodes using RAFT consensus.

Features:
- Distributed storage with multiple nodes
- File replication and consistency
- RAFT consensus for distributed coordination
- Real-time file watching and replication
- Admin interface and monitoring
- Cluster management`,
	Version: fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date),
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	// Add global flags
	rootCmd.PersistentFlags().StringP("log-level", "l", "info", "Set log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().StringP("data-dir", "d", "data", "Data directory for storage")
}
