// Package main implements a multi-directional distributed file replication system.
// This version uses modular components for better maintainability and testing.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
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
)

// Config holds all configuration for the application.
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
	// We'll need to add a getter method to RaftManager
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
	// Apply the command directly through the Raft instance
	return rw.rm.GetRaft().Apply(data, timeout)
}

func (rw *raftWrapper) State() raft.RaftState {
	return rw.rm.State()
}

func (rw *raftWrapper) Leader() raft.ServerAddress {
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
func (app *Application) handleClusterMembership() {
	if app.config.JoinAddr != "" && !app.config.BootstrapCluster {
		// Wait a bit for bootstrap node to be ready
		time.Sleep(5 * time.Second)

		// Request to join cluster via admin interface
		app.logger.Infof("Requesting to join cluster at %s", app.config.JoinAddr)

		nodeAddr := fmt.Sprintf("127.0.0.1:%d", app.config.Port)
		leaderAdminAddr := app.deriveAdminAddress(app.config.JoinAddr)

		if err := app.requestJoinCluster(leaderAdminAddr, app.config.NodeID, nodeAddr); err != nil {
			app.logger.WithError(err).Warn("Failed to join cluster via admin interface")
		} else {
			app.logger.Info("Successfully joined cluster")
		}
	}

	// Monitor leadership changes
	go app.monitorLeadership()
}

// deriveAdminAddress converts a Raft address to an admin address.
// Assumes admin port is 1000 higher than raft port.
func (app *Application) deriveAdminAddress(raftAddr string) string {
	host, portStr, err := net.SplitHostPort(raftAddr)
	if err != nil {
		// Fallback to default admin port
		return fmt.Sprintf("127.0.0.1:%d", app.config.AdminPort)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Sprintf("127.0.0.1:%d", app.config.AdminPort)
	}

	adminPort := port + 1000 // Default admin port offset
	return fmt.Sprintf("%s:%d", host, adminPort)
}

// requestJoinCluster sends an ADD_VOTER command to the leader's admin interface.
func (app *Application) requestJoinCluster(leaderAdminAddr, nodeID, nodeAddr string) error {
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
func (app *Application) monitorLeadership() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	var wasLeader bool

	for range ticker.C {
		isLeader := app.raftManager.State() == raft.Leader

		if isLeader && !wasLeader {
			app.logger.Infof("👑 %s became leader - file watching active", app.config.NodeID)
			app.monitor.GetMetrics().IncrementFilesReplicated() // Example metric update
		} else if !isLeader && wasLeader {
			app.logger.Infof("👥 %s is now a follower", app.config.NodeID)
		}

		wasLeader = isLeader
	}
}

// logAccessURLs logs the access URLs for the various interfaces.
func (app *Application) logAccessURLs() {
	app.logger.Info("🌐 Access URLs:")
	app.logger.Infof("   Admin Interface: http://localhost:%d", app.config.AdminPort)
	app.logger.Infof("   Monitoring API:  http://localhost:%d", app.config.MonitorPort)
	app.logger.Infof("   Dashboard:       http://localhost:%d", app.config.DashboardPort)
	app.logger.Infof("   Health Check:    http://localhost:%d/health", app.config.MonitorPort)
	app.logger.Infof("   Metrics:         http://localhost:%d/metrics", app.config.MonitorPort)
	app.logger.Info("📁 Data Directory:", app.config.DataDir)
}

// Stop gracefully shuts down all components.
func (app *Application) Stop() error {
	app.logger.Info("🛑 Shutting down Pickbox node...")

	// Stop file watcher
	if err := app.fileWatcher.Stop(); err != nil {
		app.logger.WithError(err).Warn("Error stopping file watcher")
	}

	// Stop Raft manager
	if err := app.raftManager.Shutdown(); err != nil {
		app.logger.WithError(err).Warn("Error stopping Raft manager")
	}

	app.logger.Info("✅ Shutdown completed")
	return nil
}

// parseFlags parses command line flags and returns configuration.
func parseFlags() AppConfig {
	var cfg AppConfig

	flag.StringVar(&cfg.NodeID, "node", "node1", "Node ID")
	flag.IntVar(&cfg.Port, "port", 8001, "Raft port")
	flag.IntVar(&cfg.AdminPort, "admin-port", 9001, "Admin server port")
	flag.IntVar(&cfg.MonitorPort, "monitor-port", 6001, "Monitoring server port")
	flag.IntVar(&cfg.DashboardPort, "dashboard-port", 8080, "Dashboard server port")
	flag.StringVar(&cfg.JoinAddr, "join", "", "Address of node to join")
	flag.StringVar(&cfg.DataDir, "data-dir", "", "Data directory (default: data/<node-id>)")
	flag.StringVar(&cfg.LogLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	flag.BoolVar(&cfg.BootstrapCluster, "bootstrap", false, "Bootstrap new cluster")

	flag.Parse()

	// Set default data directory if not provided
	if cfg.DataDir == "" {
		cfg.DataDir = filepath.Join("data", cfg.NodeID)
	}

	// Only bootstrap if explicitly requested
	// This prevents multiple nodes from trying to bootstrap simultaneously
	// The cluster manager should explicitly set -bootstrap for the first node

	return cfg
}

// setupSignalHandling sets up graceful shutdown on SIGINT/SIGTERM.
func setupSignalHandling(app *Application) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		app.logger.Info("🔔 Received shutdown signal")
		if err := app.Stop(); err != nil {
			app.logger.WithError(err).Error("Error during shutdown")
			os.Exit(1)
		}
		os.Exit(0)
	}()
}

func main() {
	// Parse configuration
	config := parseFlags()

	// Create application
	app, err := NewApplication(config)
	if err != nil {
		log.Fatalf("Failed to create application: %v", err)
	}

	// Setup signal handling
	setupSignalHandling(app)

	// Start application
	if err := app.Start(); err != nil {
		log.Fatalf("Failed to start application: %v", err)
	}

	// Create a welcome file for testing (only for bootstrap node)
	if config.BootstrapCluster {
		go func() {
			time.Sleep(10 * time.Second) // Wait for cluster to be ready
			createWelcomeFile(config.DataDir, config.NodeID, app.logger)
		}()
	}

	// Keep running
	app.logger.Info("🟢 Node is running! Try editing files in the data directory.")
	app.logger.Info("🛑 Press Ctrl+C to stop")

	select {} // Block forever
}

// createWelcomeFile creates a test file for demonstration.
func createWelcomeFile(dataDir, nodeID string, logger *logrus.Logger) {
	welcomeFile := filepath.Join(dataDir, "welcome.txt")
	welcomeContent := fmt.Sprintf(`Welcome to Pickbox Distributed Storage!

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
		logger.Info("📝 Created welcome.txt - try editing it to see replication in action!")
	} else {
		logger.WithError(err).Warn("Failed to create welcome file")
	}
}
