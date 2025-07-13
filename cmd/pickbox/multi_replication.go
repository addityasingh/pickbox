package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/addityasingh/pickbox/pkg/admin"
	"github.com/addityasingh/pickbox/pkg/storage"
	"github.com/addityasingh/pickbox/pkg/watcher"
	"github.com/hashicorp/raft"
	"github.com/sirupsen/logrus"
)

// MultiApplication represents the multi-directional replication application
type MultiApplication struct {
	config       MultiConfig
	logger       *logrus.Logger
	raftManager  *storage.RaftManager
	stateManager *watcher.DefaultStateManager
	fileWatcher  *watcher.FileWatcher
	adminServer  *admin.Server
}

// MultiConfig holds configuration for multi-directional replication
type MultiConfig struct {
	NodeID    string
	Port      int
	AdminPort int
	JoinAddr  string
	DataDir   string
	LogLevel  string
}

// NewMultiApplication creates a new multi-directional replication application instance
func NewMultiApplication(cfg MultiConfig) (*MultiApplication, error) {
	// Setup logger
	logger := logrus.New()
	level, err := logrus.ParseLevel(cfg.LogLevel)
	if err != nil {
		level = logrus.InfoLevel
	}
	logger.SetLevel(level)
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		ForceColors:   true,
	})

	// Create data directory
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		return nil, fmt.Errorf("creating data directory: %w", err)
	}

	app := &MultiApplication{
		config: cfg,
		logger: logger,
	}

	// Initialize components
	if err := app.initializeComponents(); err != nil {
		return nil, fmt.Errorf("initializing components: %w", err)
	}

	return app, nil
}

// initializeComponents sets up all application components for multi-directional replication
func (app *MultiApplication) initializeComponents() error {
	var err error

	// Initialize Raft manager
	bindAddr := fmt.Sprintf("127.0.0.1:%d", app.config.Port)
	app.raftManager, err = storage.NewRaftManager(
		app.config.NodeID,
		app.config.DataDir,
		bindAddr,
	)
	if err != nil {
		return fmt.Errorf("creating raft manager: %w", err)
	}

	// Initialize state manager
	app.stateManager = watcher.NewDefaultStateManager()

	// Initialize admin server
	app.adminServer = admin.NewServer(
		app.raftManager.GetRaft(),
		app.config.AdminPort,
		app.logger,
	)

	// Initialize file watcher with multi-directional support
	watcherConfig := watcher.Config{
		DataDir:      app.config.DataDir,
		NodeID:       app.config.NodeID,
		Logger:       app.logger,
		ApplyTimeout: 5 * time.Second,
	}

	app.fileWatcher, err = watcher.NewFileWatcher(
		watcherConfig,
		&multiRaftWrapper{app.raftManager},
		app.stateManager,
		&multiForwarderWrapper{app.logger},
	)
	if err != nil {
		return fmt.Errorf("creating file watcher: %w", err)
	}

	return nil
}

// multiRaftWrapper adapts RaftManager to the watcher.RaftApplier interface
type multiRaftWrapper struct {
	rm *storage.RaftManager
}

func (rw *multiRaftWrapper) Apply(data []byte, timeout time.Duration) raft.ApplyFuture {
	return rw.rm.GetRaft().Apply(data, timeout)
}

func (rw *multiRaftWrapper) State() raft.RaftState {
	return rw.rm.State()
}

func (rw *multiRaftWrapper) Leader() raft.ServerAddress {
	return rw.rm.Leader()
}

// multiForwarderWrapper implements the watcher.LeaderForwarder interface
type multiForwarderWrapper struct {
	logger *logrus.Logger
}

func (fw *multiForwarderWrapper) ForwardToLeader(leaderAddr string, cmd watcher.Command) error {
	// Convert to admin command
	adminCmd := admin.Command{
		Op:       cmd.Op,
		Path:     cmd.Path,
		Data:     cmd.Data,
		Hash:     cmd.Hash,
		NodeID:   cmd.NodeID,
		Sequence: cmd.Sequence,
	}

	// Forward to leader via admin interface
	adminAddr := deriveMultiAdminAddress(string(leaderAddr))
	fw.logger.Debugf("📡 Forwarding command to leader at %s", adminAddr)

	return admin.ForwardToLeader(adminAddr, adminCmd)
}

// Start starts the multi-directional replication application
func (app *MultiApplication) Start() error {
	app.logger.Infof("🚀 Starting multi-directional replication node %s", app.config.NodeID)

	// Start Raft cluster
	if err := app.startRaftCluster(); err != nil {
		return fmt.Errorf("starting raft cluster: %w", err)
	}

	// Start admin server
	if err := app.adminServer.Start(); err != nil {
		return fmt.Errorf("starting admin server: %w", err)
	}

	// Start file watcher (multi-directional)
	if err := app.fileWatcher.Start(); err != nil {
		return fmt.Errorf("starting file watcher: %w", err)
	}

	// Handle cluster membership
	go app.handleClusterMembership()

	// Monitor leadership changes
	go app.monitorLeadership()

	app.logger.Infof("✅ Multi-directional replication node %s started successfully", app.config.NodeID)
	app.logAccessURLs()

	return nil
}

// startRaftCluster initializes the Raft cluster
func (app *MultiApplication) startRaftCluster() error {
	if app.config.JoinAddr == "" {
		app.logger.Info("🏗️  Bootstrapping new cluster...")

		// Create server configuration for bootstrap
		server := raft.Server{
			ID:      raft.ServerID(app.config.NodeID),
			Address: raft.ServerAddress(fmt.Sprintf("127.0.0.1:%d", app.config.Port)),
		}

		if err := app.raftManager.BootstrapCluster([]raft.Server{server}); err != nil {
			return fmt.Errorf("bootstrapping cluster: %w", err)
		}

		app.logger.Infof("🏗️  Cluster bootstrapped with node %s", app.config.NodeID)
	}

	return nil
}

// handleClusterMembership handles joining cluster if join address is provided
func (app *MultiApplication) handleClusterMembership() {
	if app.config.JoinAddr == "" {
		return
	}

	app.logger.Info("⏳ Waiting for cluster membership...")

	// Wait briefly for the node to be ready
	time.Sleep(2 * time.Second)

	// Derive admin address from Raft address
	leaderAdminAddr := deriveMultiAdminAddress(app.config.JoinAddr)
	nodeAddr := fmt.Sprintf("127.0.0.1:%d", app.config.Port)

	// Try to join the cluster
	if err := app.requestJoinCluster(leaderAdminAddr, app.config.NodeID, nodeAddr); err != nil {
		app.logger.Errorf("❌ Failed to join cluster: %v", err)
		return
	}

	app.logger.Infof("🤝 Successfully joined cluster via %s", leaderAdminAddr)
}

// requestJoinCluster requests to join the cluster via admin API
func (app *MultiApplication) requestJoinCluster(leaderAdminAddr, nodeID, nodeAddr string) error {
	return admin.RequestJoinCluster(leaderAdminAddr, nodeID, nodeAddr)
}

// monitorLeadership monitors leadership changes
func (app *MultiApplication) monitorLeadership() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	var lastLeader raft.ServerAddress
	var wasLeader bool

	for range ticker.C {
		currentLeader := app.raftManager.Leader()
		isLeader := app.raftManager.State() == raft.Leader

		if currentLeader != lastLeader {
			if currentLeader == "" {
				app.logger.Warn("👑 No leader elected")
			} else {
				app.logger.Infof("👑 Leader: %s", currentLeader)
			}
			lastLeader = currentLeader
		}

		if isLeader && !wasLeader {
			app.logger.Infof("👑 %s became leader - multi-directional replication active", app.config.NodeID)
		} else if !isLeader && wasLeader {
			app.logger.Infof("👥 %s is now a follower - forwarding changes to leader", app.config.NodeID)
		}

		wasLeader = isLeader
	}
}

// logAccessURLs logs the access URLs for the services
func (app *MultiApplication) logAccessURLs() {
	app.logger.Info("📊 Access URLs:")
	app.logger.Infof("  Admin API: http://localhost:%d", app.config.AdminPort)
	app.logger.Infof("  Data Directory: %s", app.config.DataDir)
	app.logger.Info("📝 Multi-directional replication: Edit files in any node's data directory!")
}

// Stop stops the multi-directional replication application
func (app *MultiApplication) Stop() error {
	app.logger.Info("🛑 Stopping multi-directional replication node...")

	// Stop file watcher
	if app.fileWatcher != nil {
		app.fileWatcher.Stop()
	}

	// Stop Raft
	if app.raftManager != nil {
		app.raftManager.Shutdown()
	}

	app.logger.Info("✅ Multi-directional replication node stopped successfully")
	return nil
}

// deriveMultiAdminAddress converts a Raft address to an admin address
func deriveMultiAdminAddress(raftAddr string) string {
	parts := strings.Split(raftAddr, ":")
	if len(parts) != 2 {
		return "127.0.0.1:9001" // Default admin port
	}

	host := parts[0]
	portStr := parts[1]

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "127.0.0.1:9001" // Default admin port
	}

	// Admin port is typically raft port + 1000
	adminPort := port + 1000
	return fmt.Sprintf("%s:%d", host, adminPort)
}

// runMultiReplication runs the multi-directional replication
func runMultiReplication(nodeID string, port int, join string, dataDir string, logger *logrus.Logger) error {
	// Create configuration
	config := MultiConfig{
		NodeID:    nodeID,
		Port:      port,
		AdminPort: port + 1000, // Admin port is raft port + 1000
		JoinAddr:  join,
		DataDir:   dataDir,
		LogLevel:  "info",
	}

	// Create application
	app, err := NewMultiApplication(config)
	if err != nil {
		return fmt.Errorf("creating multi-directional replication application: %w", err)
	}

	// Start application
	if err := app.Start(); err != nil {
		return fmt.Errorf("starting multi-directional replication application: %w", err)
	}

	// Create welcome file for testing (only if bootstrapping)
	if join == "" {
		go func() {
			time.Sleep(3 * time.Second) // Wait for leadership
			welcomeFile := filepath.Join(dataDir, "welcome.txt")
			welcomeContent := fmt.Sprintf(`Welcome to %s - Multi-Directional Replication!

This file was created at %s

🚀 Features:
- Multi-directional file replication (edit files on any node!)
- Real-time file watching and replication
- Automatic leader forwarding
- Raft consensus for consistency

📝 Try editing this file on any node and watch it replicate to others!
📁 Data directory: %s

Happy distributed computing! 🎉
`, nodeID, time.Now().Format(time.RFC3339), dataDir)

			if err := os.WriteFile(welcomeFile, []byte(welcomeContent), 0644); err == nil {
				logger.Info("📝 Created welcome.txt - edit it on any node to see multi-directional replication!")
			}
		}()
	}

	logger.Info("🟢 Multi-directional replication is running!")
	logger.Info("📁 Data directory:", dataDir)
	logger.Info("🔄 Files can be edited on any node and will replicate to all others")
	logger.Info("🛑 Press Ctrl+C to stop")

	// Keep running
	select {}
}
