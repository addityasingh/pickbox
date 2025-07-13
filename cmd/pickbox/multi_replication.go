package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/addityasingh/pickbox/pkg/admin"
	"github.com/addityasingh/pickbox/pkg/monitoring"
	"github.com/addityasingh/pickbox/pkg/storage"
	"github.com/addityasingh/pickbox/pkg/watcher"
	"github.com/hashicorp/raft"
	"github.com/sirupsen/logrus"
)

// MultiConfig holds configuration for multi-directional replication
type MultiConfig struct {
	NodeID           string
	Port             int
	AdminPort        int
	MonitorPort      int
	DashboardPort    int
	JoinAddr         string
	DataDir          string
	LogLevel         string
	BootstrapCluster bool
}

// validateMultiConfig validates the multi-directional replication configuration.
func validateMultiConfig(cfg MultiConfig) error {
	if cfg.DataDir == "" {
		return errors.New("data directory cannot be empty")
	}
	if cfg.NodeID == "" {
		return errors.New("node ID cannot be empty")
	}
	if cfg.Port <= 0 {
		return errors.New("port must be positive")
	}
	if cfg.AdminPort <= 0 {
		return errors.New("admin port must be positive")
	}
	if cfg.MonitorPort <= 0 {
		return errors.New("monitor port must be positive")
	}
	if cfg.DashboardPort <= 0 {
		return errors.New("dashboard port must be positive")
	}
	return nil
}

// MultiApplication represents the multi-directional replication application
type MultiApplication struct {
	config       MultiConfig
	logger       *logrus.Logger
	raftManager  *storage.RaftManager
	stateManager *watcher.DefaultStateManager
	fileWatcher  *watcher.FileWatcher
	adminServer  *admin.Server
	monitor      *monitoring.Monitor
	dashboard    *monitoring.Dashboard
}

// NewMultiApplication creates a new multi-directional replication application instance
func NewMultiApplication(cfg MultiConfig) (*MultiApplication, error) {
	// Validate configuration
	if err := validateMultiConfig(cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

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

// initializeComponents sets up all application components.
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

	// Access the raft instance through the manager for admin server
	raftInstance := app.getRaftInstance()

	// Initialize admin server
	app.adminServer = admin.NewServer(
		raftInstance,
		app.config.AdminPort,
		app.logger,
	)

	// Initialize monitoring
	app.monitor = monitoring.NewMonitor(
		app.config.NodeID,
		raftInstance,
		app.logger,
	)

	// Initialize dashboard
	app.dashboard = monitoring.NewDashboard(app.monitor, app.logger)

	// Initialize file watcher
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

// getRaftInstance provides access to the underlying raft instance
func (app *MultiApplication) getRaftInstance() *raft.Raft {
	if app.raftManager == nil {
		return nil
	}
	return app.raftManager.GetRaft()
}

// multiRaftWrapper adapts RaftManager to the watcher.RaftApplier interface.
type multiRaftWrapper struct {
	rm *storage.RaftManager
}

func (rw *multiRaftWrapper) Apply(data []byte, timeout time.Duration) raft.ApplyFuture {
	if rw.rm == nil {
		return nil
	}
	return rw.rm.GetRaft().Apply(data, timeout)
}

func (rw *multiRaftWrapper) State() raft.RaftState {
	if rw.rm == nil {
		return raft.Shutdown
	}
	return rw.rm.State()
}

func (rw *multiRaftWrapper) Leader() raft.ServerAddress {
	if rw.rm == nil {
		return ""
	}
	return rw.rm.Leader()
}

// multiForwarderWrapper implements the watcher.LeaderForwarder interface.
type multiForwarderWrapper struct {
	logger *logrus.Logger
}

func (fw *multiForwarderWrapper) ForwardToLeader(leaderAddr string, cmd watcher.Command) error {
	adminCmd := admin.Command{
		Op:       cmd.Op,
		Path:     cmd.Path,
		Data:     cmd.Data,
		Hash:     cmd.Hash,
		NodeID:   cmd.NodeID,
		Sequence: cmd.Sequence,
	}

	// Convert raft address to admin address
	adminAddr := deriveMultiAdminAddress(leaderAddr)

	if fw.logger != nil {
		fw.logger.WithFields(logrus.Fields{
			"leader_addr": leaderAddr,
			"admin_addr":  adminAddr,
			"operation":   cmd.Op,
			"path":        cmd.Path,
		}).Debug("Forwarding command to leader")
	}

	return admin.ForwardToLeader(adminAddr, adminCmd)
}

// Start starts all application components.
func (app *MultiApplication) Start() error {
	app.logger.Infof("🚀 Starting Pickbox multi-directional replication node %s", app.config.NodeID)

	// Start Raft cluster
	if err := app.startRaftCluster(); err != nil {
		return fmt.Errorf("starting raft cluster: %w", err)
	}

	// Start admin server
	if err := app.adminServer.Start(); err != nil {
		return fmt.Errorf("starting admin server: %w", err)
	}

	// Start monitoring
	app.monitor.StartHTTPServer(app.config.MonitorPort)
	app.monitor.LogMetrics(30 * time.Second)

	// Start dashboard
	app.dashboard.StartDashboardServer(app.config.DashboardPort)

	// Start file watcher
	if err := app.fileWatcher.Start(); err != nil {
		return fmt.Errorf("starting file watcher: %w", err)
	}

	// Wait for leadership and join cluster if needed
	go app.handleClusterMembership()

	app.logger.Infof("✅ Multi-directional replication node %s started successfully", app.config.NodeID)
	app.logAccessURLs()

	return nil
}

// startRaftCluster initializes the Raft cluster.
func (app *MultiApplication) startRaftCluster() error {
	if app.config.BootstrapCluster {
		app.logger.Info("🏗️  Bootstrapping new cluster...")

		// Create server configuration for bootstrap
		servers := []raft.Server{
			{
				ID:      raft.ServerID(app.config.NodeID),
				Address: raft.ServerAddress(fmt.Sprintf("127.0.0.1:%d", app.config.Port)),
			},
		}

		if err := app.raftManager.BootstrapCluster(servers); err != nil {
			return fmt.Errorf("bootstrapping cluster: %w", err)
		}
		app.logger.Info("✅ Cluster bootstrapped successfully")
	} else if app.config.JoinAddr != "" {
		app.logger.Infof("🔗 Joining cluster at %s", app.config.JoinAddr)
		// Join logic will be handled in handleClusterMembership
	}

	return nil
}

// handleClusterMembership manages cluster joining and leadership monitoring.
func (app *MultiApplication) handleClusterMembership() {
	if app.config.JoinAddr != "" && !app.config.BootstrapCluster {
		// Wait a bit for bootstrap node to be ready
		time.Sleep(5 * time.Second)

		// Request to join cluster via admin interface
		app.logger.Infof("Requesting to join cluster at %s", app.config.JoinAddr)

		nodeAddr := fmt.Sprintf("127.0.0.1:%d", app.config.Port)
		leaderAdminAddr := deriveMultiAdminAddress(app.config.JoinAddr)

		if err := app.requestJoinCluster(leaderAdminAddr, app.config.NodeID, nodeAddr); err != nil {
			app.logger.WithError(err).Warn("Failed to join cluster via admin interface")
		} else {
			app.logger.Info("Successfully joined cluster")
		}
	}

	// Monitor leadership changes
	go app.monitorLeadership()
}

// requestJoinCluster sends an ADD_VOTER command to the leader's admin interface.
func (app *MultiApplication) requestJoinCluster(leaderAdminAddr, nodeID, nodeAddr string) error {
	conn, err := net.DialTimeout("tcp", leaderAdminAddr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("connecting to leader admin at %s: %w", leaderAdminAddr, err)
	}
	defer conn.Close()

	command := fmt.Sprintf("ADD_VOTER %s %s", nodeID, nodeAddr)
	if _, err := conn.Write([]byte(command)); err != nil {
		return fmt.Errorf("sending ADD_VOTER command: %w", err)
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

	return nil
}

// monitorLeadership monitors Raft leadership changes and adjusts file watching.
func (app *MultiApplication) monitorLeadership() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	var wasLeader bool

	for range ticker.C {
		isLeader := app.raftManager.State() == raft.Leader

		if isLeader && !wasLeader {
			app.logger.Infof("👑 %s became leader - multi-directional file watching active", app.config.NodeID)
			app.monitor.GetMetrics().IncrementFilesReplicated() // Example metric update
		} else if !isLeader && wasLeader {
			app.logger.Infof("👥 %s is now a follower", app.config.NodeID)
		}

		wasLeader = isLeader
	}
}

// logAccessURLs logs the access URLs for the various interfaces.
func (app *MultiApplication) logAccessURLs() {
	app.logger.Info("🌐 Access URLs:")
	app.logger.Infof("   Admin Interface: http://localhost:%d", app.config.AdminPort)
	app.logger.Infof("   Monitoring API:  http://localhost:%d", app.config.MonitorPort)
	app.logger.Infof("   Dashboard:       http://localhost:%d", app.config.DashboardPort)
	app.logger.Infof("   Health Check:    http://localhost:%d/health", app.config.MonitorPort)
	app.logger.Infof("   Metrics:         http://localhost:%d/metrics", app.config.MonitorPort)
	app.logger.Info("📁 Data Directory:", app.config.DataDir)
}

// Stop gracefully shuts down all components.
func (app *MultiApplication) Stop() error {
	app.logger.Info("🛑 Shutting down multi-directional replication node...")

	// Stop file watcher
	if err := app.fileWatcher.Stop(); err != nil {
		app.logger.WithError(err).Warn("Error stopping file watcher")
	}

	// Stop Raft manager
	if err := app.raftManager.Shutdown(); err != nil {
		app.logger.WithError(err).Warn("Error stopping Raft manager")
	}

	app.logger.Info("✅ Multi-directional replication shutdown completed")
	return nil
}

// deriveMultiAdminAddress converts a Raft address to an admin address.
// Assumes admin port is 1000 higher than raft port.
func deriveMultiAdminAddress(raftAddr string) string {
	host, portStr, err := net.SplitHostPort(raftAddr)
	if err != nil {
		// Fallback to localhost:9001 if parsing fails
		return "127.0.0.1:9001"
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "127.0.0.1:9001"
	}

	adminPort := port + 1000 // Default admin port offset
	return fmt.Sprintf("%s:%d", host, adminPort)
}

// runMultiReplication runs the multi-directional replication with the given parameters.
func runMultiReplication(nodeID string, port int, join string, dataDir string, logger *logrus.Logger) error {
	// Create configuration
	cfg := MultiConfig{
		NodeID:           nodeID,
		Port:             port,
		AdminPort:        port + 1000,
		MonitorPort:      port + 2000,
		DashboardPort:    port + 3000,
		JoinAddr:         join,
		DataDir:          dataDir,
		LogLevel:         "info",
		BootstrapCluster: join == "", // Bootstrap if not joining
	}

	// Create application
	app, err := NewMultiApplication(cfg)
	if err != nil {
		return fmt.Errorf("creating multi-directional replication application: %w", err)
	}

	// Start application
	if err := app.Start(); err != nil {
		return fmt.Errorf("starting multi-directional replication application: %w", err)
	}

	// Create welcome file for bootstrap node
	if cfg.BootstrapCluster {
		go func() {
			time.Sleep(10 * time.Second) // Wait for cluster to be ready
			createMultiWelcomeFile(cfg.DataDir, cfg.NodeID, logger)
		}()
	}

	return nil
}

// createMultiWelcomeFile creates a test file for demonstration.
func createMultiWelcomeFile(dataDir, nodeID string, logger *logrus.Logger) {
	welcomeFile := filepath.Join(dataDir, "welcome.txt")
	welcomeContent := fmt.Sprintf(`Welcome to Pickbox Multi-Directional Distributed Storage!

This file was created by %s at %s

🚀 Features:
- Multi-directional file replication
- Raft consensus for consistency
- Real-time file monitoring
- Web dashboard and monitoring
- Auto-discovery and healing

📝 Try editing this file and watch it replicate to other nodes!
🔍 Check the dashboard at http://localhost:8080
📊 View metrics at http://localhost:6001/metrics

Happy distributed computing! 🎉
`, nodeID, time.Now().Format(time.RFC3339))

	if err := os.WriteFile(welcomeFile, []byte(welcomeContent), 0644); err == nil {
		logger.Info("📝 Created welcome.txt - try editing it to see multi-directional replication in action!")
	} else {
		logger.WithError(err).Warn("Failed to create welcome file")
	}
}
