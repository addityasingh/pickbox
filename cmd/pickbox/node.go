package main

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/addityasingh/pickbox/pkg/admin"
	"github.com/addityasingh/pickbox/pkg/monitoring"
	"github.com/addityasingh/pickbox/pkg/storage"
	"github.com/addityasingh/pickbox/pkg/watcher"
	"github.com/hashicorp/raft"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var nodeCmd = &cobra.Command{
	Use:   "node",
	Short: "Node management commands",
	Long:  `Commands for managing Pickbox nodes including starting, stopping, and configuration`,
}

var nodeStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start a Pickbox node with full features",
	Long: `Start a Pickbox node with all features including:
- Multi-directional file replication
- Admin interface
- Monitoring and dashboard
- Cluster management`,
	RunE: runNodeStart,
}

var nodeMultiCmd = &cobra.Command{
	Use:   "multi",
	Short: "Start a node with multi-directional replication",
	Long: `Start a node with multi-directional replication capabilities.
This mode provides real-time file watching and multi-directional replication across all nodes.`,
	RunE: runNodeMulti,
}

// Node start command flags
var (
	nodeID           string
	port             int
	adminPort        int
	monitorPort      int
	dashboardPort    int
	joinAddr         string
	bootstrapCluster bool
)

// Live node command flags
var (
	liveNodeID string
	livePort   int
	liveJoin   string
)

func init() {
	rootCmd.AddCommand(nodeCmd)
	nodeCmd.AddCommand(nodeStartCmd)
	nodeCmd.AddCommand(nodeMultiCmd)

	// Node start command flags
	nodeStartCmd.Flags().StringVarP(&nodeID, "node-id", "n", "", "Node ID (required)")
	nodeStartCmd.Flags().IntVarP(&port, "port", "p", 8001, "Raft port")
	nodeStartCmd.Flags().IntVar(&adminPort, "admin-port", 9001, "Admin API port")
	nodeStartCmd.Flags().IntVar(&monitorPort, "monitor-port", 9002, "Monitor port")
	nodeStartCmd.Flags().IntVar(&dashboardPort, "dashboard-port", 9003, "Dashboard port")
	nodeStartCmd.Flags().StringVarP(&joinAddr, "join", "j", "", "Address of node to join")
	nodeStartCmd.Flags().BoolVarP(&bootstrapCluster, "bootstrap", "b", false, "Bootstrap new cluster")
	nodeStartCmd.MarkFlagRequired("node-id")

	// Multi-directional replication command flags
	nodeMultiCmd.Flags().StringVarP(&liveNodeID, "node-id", "n", "", "Node ID (required)")
	nodeMultiCmd.Flags().IntVarP(&livePort, "port", "p", 8001, "Port")
	nodeMultiCmd.Flags().StringVarP(&liveJoin, "join", "j", "", "Address of node to join")
	nodeMultiCmd.MarkFlagRequired("node-id")
}

func runNodeStart(cmd *cobra.Command, args []string) error {
	// Get global flags
	logLevel, _ := cmd.Flags().GetString("log-level")
	dataDir, _ := cmd.Flags().GetString("data-dir")

	// Create configuration
	config := AppConfig{
		NodeID:           nodeID,
		Port:             port,
		AdminPort:        adminPort,
		MonitorPort:      monitorPort,
		DashboardPort:    dashboardPort,
		JoinAddr:         joinAddr,
		DataDir:          filepath.Join(dataDir, nodeID),
		LogLevel:         logLevel,
		BootstrapCluster: bootstrapCluster,
	}

	// Create and start application
	app, err := NewApplication(config)
	if err != nil {
		return fmt.Errorf("creating application: %w", err)
	}

	// Start application
	if err := app.Start(); err != nil {
		return fmt.Errorf("starting application: %w", err)
	}

	// Setup graceful shutdown
	setupSignalHandling(app)

	// Keep running
	select {}
}

func runNodeMulti(cmd *cobra.Command, args []string) error {
	// Get global flags
	logLevel, _ := cmd.Flags().GetString("log-level")
	dataDir, _ := cmd.Flags().GetString("data-dir")

	// Set up logging
	logger := logrus.New()
	level, err := logrus.ParseLevel(logLevel)
	if err != nil {
		level = logrus.InfoLevel
	}
	logger.SetLevel(level)
	logger.Infof("Starting multi-directional replication node %s on port %d", liveNodeID, livePort)

	// Setup data directory
	nodeDataDir := filepath.Join(dataDir, liveNodeID)
	if err := os.MkdirAll(nodeDataDir, 0755); err != nil {
		return fmt.Errorf("creating data directory: %w", err)
	}

	// Start multi-directional replication node
	return runMultiReplication(liveNodeID, livePort, liveJoin, nodeDataDir, logger)
}

// AppConfig holds all configuration for the application.
type AppConfig struct {
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

// validateConfig validates the application configuration.
func validateConfig(cfg AppConfig) error {
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

// Application represents the main application with all components.
type Application struct {
	config       AppConfig
	logger       *logrus.Logger
	raftManager  *storage.RaftManager
	stateManager *watcher.DefaultStateManager
	fileWatcher  *watcher.FileWatcher
	adminServer  *admin.Server
	monitor      *monitoring.Monitor
	dashboard    *monitoring.Dashboard
}

// NewApplication creates a new application instance with all components.
func NewApplication(cfg AppConfig) (*Application, error) {
	// Validate configuration
	if err := validateConfig(cfg); err != nil {
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

	app := &Application{
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
func (app *Application) initializeComponents() error {
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

	// Initialize file watcher with simplified approach
	watcherConfig := watcher.Config{
		DataDir:      app.config.DataDir,
		NodeID:       app.config.NodeID,
		Logger:       app.logger,
		ApplyTimeout: 5 * time.Second,
	}

	app.fileWatcher, err = watcher.NewFileWatcher(
		watcherConfig,
		&raftWrapper{app.raftManager},
		app.stateManager,
		&forwarderWrapper{},
	)
	if err != nil {
		return fmt.Errorf("creating file watcher: %w", err)
	}

	return nil
}

// getRaftInstance provides access to the underlying raft instance
func (app *Application) getRaftInstance() *raft.Raft {
	if app.raftManager == nil {
		return nil
	}
	return app.raftManager.GetRaft()
}

// raftWrapper adapts RaftManager to the watcher.RaftApplier interface.
type raftWrapper struct {
	rm *storage.RaftManager
}

func (rw *raftWrapper) Apply(data []byte, timeout time.Duration) raft.ApplyFuture {
	if rw.rm == nil {
		return nil
	}
	return rw.rm.GetRaft().Apply(data, timeout)
}

func (rw *raftWrapper) State() raft.RaftState {
	if rw.rm == nil {
		return raft.Shutdown
	}
	return rw.rm.State()
}

func (rw *raftWrapper) Leader() raft.ServerAddress {
	if rw.rm == nil {
		return ""
	}
	return rw.rm.Leader()
}

// forwarderWrapper implements the watcher.LeaderForwarder interface.
type forwarderWrapper struct{}

func (fw *forwarderWrapper) ForwardToLeader(leaderAddr string, cmd watcher.Command) error {
	adminCmd := admin.Command{
		Op:       cmd.Op,
		Path:     cmd.Path,
		Data:     cmd.Data,
		Hash:     cmd.Hash,
		NodeID:   cmd.NodeID,
		Sequence: cmd.Sequence,
	}
	return admin.ForwardToLeader(leaderAddr, adminCmd)
}

// Start starts all application components.
func (app *Application) Start() error {
	app.logger.Infof("🚀 Starting Pickbox node %s", app.config.NodeID)

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

	app.logger.Infof("✅ Node %s started successfully", app.config.NodeID)
	app.logAccessURLs()

	return nil
}

// startRaftCluster initializes the Raft cluster.
func (app *Application) startRaftCluster() error {
	if app.config.BootstrapCluster {
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

// handleClusterMembership handles joining cluster if join address is provided.
func (app *Application) handleClusterMembership() {
	if app.config.JoinAddr == "" {
		return
	}

	app.logger.Info("⏳ Waiting for cluster membership...")

	// Wait briefly for the node to be ready
	time.Sleep(2 * time.Second)

	// Derive admin address from Raft address
	leaderAdminAddr := app.deriveAdminAddress(app.config.JoinAddr)
	nodeAddr := fmt.Sprintf("127.0.0.1:%d", app.config.Port)

	// Try to join the cluster
	if err := app.requestJoinCluster(leaderAdminAddr, app.config.NodeID, nodeAddr); err != nil {
		app.logger.Errorf("❌ Failed to join cluster: %v", err)
		return
	}

	app.logger.Infof("🤝 Successfully joined cluster via %s", leaderAdminAddr)

	// Monitor leadership changes
	go app.monitorLeadership()
}

// deriveAdminAddress converts a Raft address to an admin address.
func (app *Application) deriveAdminAddress(raftAddr string) string {
	parts := strings.Split(raftAddr, ":")
	if len(parts) != 2 {
		return "127.0.0.1:9001" // Fallback to default admin port
	}

	raftPort, err := strconv.Atoi(parts[1])
	if err != nil {
		return "127.0.0.1:9001" // Fallback to default admin port
	}

	// Assume admin port is raftPort + 1000
	adminPort := raftPort + 1000
	return fmt.Sprintf("%s:%d", parts[0], adminPort)
}

// requestJoinCluster requests to join the cluster via admin API.
func (app *Application) requestJoinCluster(leaderAdminAddr, nodeID, nodeAddr string) error {
	return admin.RequestJoinCluster(leaderAdminAddr, nodeID, nodeAddr)
}

// monitorLeadership monitors leadership changes and logs them.
func (app *Application) monitorLeadership() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	var lastLeader raft.ServerAddress

	for range ticker.C {
		currentLeader := app.raftManager.Leader()
		if currentLeader != lastLeader {
			if currentLeader == "" {
				app.logger.Warn("👑 No leader elected")
			} else {
				app.logger.Infof("👑 Leader: %s", currentLeader)
			}
			lastLeader = currentLeader
		}
	}
}

// logAccessURLs logs the access URLs for the various services.
func (app *Application) logAccessURLs() {
	app.logger.Info("📊 Access URLs:")
	app.logger.Infof("  Admin API: http://localhost:%d", app.config.AdminPort)
	app.logger.Infof("  Monitoring: http://localhost:%d/metrics", app.config.MonitorPort)
	app.logger.Infof("  Dashboard: http://localhost:%d", app.config.DashboardPort)
	app.logger.Infof("  Data Directory: %s", app.config.DataDir)
}

// Stop stops all application components.
func (app *Application) Stop() error {
	app.logger.Info("🛑 Stopping Pickbox node...")

	// Stop file watcher
	if app.fileWatcher != nil {
		app.fileWatcher.Stop()
	}

	// Note: Admin server doesn't have a Stop method, it will be cleaned up
	// when the application exits

	// Stop Raft
	if app.raftManager != nil {
		app.raftManager.Shutdown()
	}

	app.logger.Info("✅ Node stopped successfully")
	return nil
}

// setupSignalHandling sets up graceful shutdown on SIGINT and SIGTERM.
func setupSignalHandling(app *Application) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		app.logger.Info("🛑 Received shutdown signal...")
		app.Stop()
		os.Exit(0)
	}()
}
