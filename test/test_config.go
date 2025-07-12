package test

import (
	"os"
	"strconv"
	"time"
)

// TestConfig contains configuration for different test environments
type TestConfig struct {
	// General settings
	Environment    string
	Verbose        bool
	EnableCoverage bool

	// Timing settings
	DefaultTimeout     time.Duration
	ReplicationTimeout time.Duration
	StartupTimeout     time.Duration
	ShutdownTimeout    time.Duration

	// Resource settings
	MaxNodes        int
	BasePort        int
	BaseAdminPort   int
	BaseMonitorPort int
	DashboardPort   int

	// Test data settings
	TestDataDir      string
	CleanupAfterTest bool
	PreserveTestData bool

	// Performance settings
	MaxConcurrentOps int
	MaxFileSize      int64
	MaxTestDuration  time.Duration

	// Network settings
	Host             string
	EnableNetworking bool
	NetworkDelay     time.Duration

	// Debug settings
	DebugMode       bool
	LogLevel        string
	EnableMetrics   bool
	EnableProfiling bool

	// CI/CD settings
	CIMode         bool
	ShortMode      bool
	StressTestMode bool

	// Retry settings
	MaxRetries   int
	RetryDelay   time.Duration
	RetryBackoff float64
}

// TestEnvironments defines different test environment configurations
var TestEnvironments = map[string]TestConfig{
	"development": {
		Environment:        "development",
		Verbose:            true,
		EnableCoverage:     false,
		DefaultTimeout:     5 * time.Minute,
		ReplicationTimeout: 30 * time.Second,
		StartupTimeout:     10 * time.Second,
		ShutdownTimeout:    5 * time.Second,
		MaxNodes:           10,
		BasePort:           8000,
		BaseAdminPort:      9000,
		BaseMonitorPort:    6000,
		DashboardPort:      8080,
		TestDataDir:        "/tmp/pickbox-dev-test",
		CleanupAfterTest:   true,
		PreserveTestData:   false,
		MaxConcurrentOps:   10,
		MaxFileSize:        10 * 1024 * 1024, // 10MB
		MaxTestDuration:    10 * time.Minute,
		Host:               "127.0.0.1",
		EnableNetworking:   true,
		NetworkDelay:       0,
		DebugMode:          true,
		LogLevel:           "debug",
		EnableMetrics:      true,
		EnableProfiling:    false,
		CIMode:             false,
		ShortMode:          false,
		StressTestMode:     false,
		MaxRetries:         3,
		RetryDelay:         1 * time.Second,
		RetryBackoff:       1.5,
	},
	"ci": {
		Environment:        "ci",
		Verbose:            false,
		EnableCoverage:     true,
		DefaultTimeout:     3 * time.Minute,
		ReplicationTimeout: 15 * time.Second,
		StartupTimeout:     5 * time.Second,
		ShutdownTimeout:    3 * time.Second,
		MaxNodes:           7,
		BasePort:           18000,
		BaseAdminPort:      19000,
		BaseMonitorPort:    16000,
		DashboardPort:      18080,
		TestDataDir:        "/tmp/pickbox-ci-test",
		CleanupAfterTest:   true,
		PreserveTestData:   false,
		MaxConcurrentOps:   5,
		MaxFileSize:        5 * 1024 * 1024, // 5MB
		MaxTestDuration:    5 * time.Minute,
		Host:               "127.0.0.1",
		EnableNetworking:   true,
		NetworkDelay:       0,
		DebugMode:          false,
		LogLevel:           "info",
		EnableMetrics:      false,
		EnableProfiling:    false,
		CIMode:             true,
		ShortMode:          false,
		StressTestMode:     false,
		MaxRetries:         2,
		RetryDelay:         500 * time.Millisecond,
		RetryBackoff:       2.0,
	},
	"short": {
		Environment:        "short",
		Verbose:            false,
		EnableCoverage:     false,
		DefaultTimeout:     30 * time.Second,
		ReplicationTimeout: 5 * time.Second,
		StartupTimeout:     2 * time.Second,
		ShutdownTimeout:    1 * time.Second,
		MaxNodes:           3,
		BasePort:           28000,
		BaseAdminPort:      29000,
		BaseMonitorPort:    26000,
		DashboardPort:      28080,
		TestDataDir:        "/tmp/pickbox-short-test",
		CleanupAfterTest:   true,
		PreserveTestData:   false,
		MaxConcurrentOps:   3,
		MaxFileSize:        1024 * 1024, // 1MB
		MaxTestDuration:    1 * time.Minute,
		Host:               "127.0.0.1",
		EnableNetworking:   false,
		NetworkDelay:       0,
		DebugMode:          false,
		LogLevel:           "warn",
		EnableMetrics:      false,
		EnableProfiling:    false,
		CIMode:             false,
		ShortMode:          true,
		StressTestMode:     false,
		MaxRetries:         1,
		RetryDelay:         100 * time.Millisecond,
		RetryBackoff:       1.0,
	},
	"performance": {
		Environment:        "performance",
		Verbose:            false,
		EnableCoverage:     false,
		DefaultTimeout:     15 * time.Minute,
		ReplicationTimeout: 2 * time.Minute,
		StartupTimeout:     30 * time.Second,
		ShutdownTimeout:    10 * time.Second,
		MaxNodes:           20,
		BasePort:           38000,
		BaseAdminPort:      39000,
		BaseMonitorPort:    36000,
		DashboardPort:      38080,
		TestDataDir:        "/tmp/pickbox-perf-test",
		CleanupAfterTest:   true,
		PreserveTestData:   false,
		MaxConcurrentOps:   100,
		MaxFileSize:        100 * 1024 * 1024, // 100MB
		MaxTestDuration:    30 * time.Minute,
		Host:               "127.0.0.1",
		EnableNetworking:   true,
		NetworkDelay:       0,
		DebugMode:          false,
		LogLevel:           "info",
		EnableMetrics:      true,
		EnableProfiling:    true,
		CIMode:             false,
		ShortMode:          false,
		StressTestMode:     false,
		MaxRetries:         5,
		RetryDelay:         2 * time.Second,
		RetryBackoff:       1.2,
	},
	"stress": {
		Environment:        "stress",
		Verbose:            false,
		EnableCoverage:     false,
		DefaultTimeout:     30 * time.Minute,
		ReplicationTimeout: 5 * time.Minute,
		StartupTimeout:     1 * time.Minute,
		ShutdownTimeout:    30 * time.Second,
		MaxNodes:           50,
		BasePort:           48000,
		BaseAdminPort:      49000,
		BaseMonitorPort:    46000,
		DashboardPort:      48080,
		TestDataDir:        "/tmp/pickbox-stress-test",
		CleanupAfterTest:   true,
		PreserveTestData:   false,
		MaxConcurrentOps:   1000,
		MaxFileSize:        1024 * 1024 * 1024, // 1GB
		MaxTestDuration:    60 * time.Minute,
		Host:               "127.0.0.1",
		EnableNetworking:   true,
		NetworkDelay:       10 * time.Millisecond,
		DebugMode:          false,
		LogLevel:           "warn",
		EnableMetrics:      true,
		EnableProfiling:    true,
		CIMode:             false,
		ShortMode:          false,
		StressTestMode:     true,
		MaxRetries:         10,
		RetryDelay:         5 * time.Second,
		RetryBackoff:       1.1,
	},
	"local": {
		Environment:        "local",
		Verbose:            true,
		EnableCoverage:     true,
		DefaultTimeout:     10 * time.Minute,
		ReplicationTimeout: 1 * time.Minute,
		StartupTimeout:     15 * time.Second,
		ShutdownTimeout:    5 * time.Second,
		MaxNodes:           10,
		BasePort:           58000,
		BaseAdminPort:      59000,
		BaseMonitorPort:    56000,
		DashboardPort:      58080,
		TestDataDir:        "/tmp/pickbox-local-test",
		CleanupAfterTest:   false,
		PreserveTestData:   true,
		MaxConcurrentOps:   20,
		MaxFileSize:        50 * 1024 * 1024, // 50MB
		MaxTestDuration:    20 * time.Minute,
		Host:               "127.0.0.1",
		EnableNetworking:   true,
		NetworkDelay:       0,
		DebugMode:          true,
		LogLevel:           "debug",
		EnableMetrics:      true,
		EnableProfiling:    true,
		CIMode:             false,
		ShortMode:          false,
		StressTestMode:     false,
		MaxRetries:         3,
		RetryDelay:         1 * time.Second,
		RetryBackoff:       1.5,
	},
}

// GetTestConfig returns the test configuration for the specified environment
func GetTestConfig(environment string) TestConfig {
	// Default to development if environment is not specified
	if environment == "" {
		environment = "development"
	}

	// Check for environment override
	if envOverride := os.Getenv("TEST_ENVIRONMENT"); envOverride != "" {
		environment = envOverride
	}

	// Get base config
	config, exists := TestEnvironments[environment]
	if !exists {
		// Return development config as fallback
		config = TestEnvironments["development"]
		config.Environment = environment
	}

	// Apply environment variable overrides
	config = applyEnvironmentOverrides(config)

	return config
}

// applyEnvironmentOverrides applies environment variable overrides to config
func applyEnvironmentOverrides(config TestConfig) TestConfig {
	// Verbose mode
	if verbose := os.Getenv("TEST_VERBOSE"); verbose != "" {
		config.Verbose = verbose == "true" || verbose == "1"
	}

	// Coverage
	if coverage := os.Getenv("TEST_COVERAGE"); coverage != "" {
		config.EnableCoverage = coverage == "true" || coverage == "1"
	}

	// Timeout settings
	if timeout := os.Getenv("TEST_TIMEOUT"); timeout != "" {
		if duration, err := time.ParseDuration(timeout); err == nil {
			config.DefaultTimeout = duration
		}
	}

	if replicationTimeout := os.Getenv("TEST_REPLICATION_TIMEOUT"); replicationTimeout != "" {
		if duration, err := time.ParseDuration(replicationTimeout); err == nil {
			config.ReplicationTimeout = duration
		}
	}

	// Port settings
	if basePort := os.Getenv("TEST_BASE_PORT"); basePort != "" {
		if port, err := strconv.Atoi(basePort); err == nil {
			config.BasePort = port
		}
	}

	if adminPort := os.Getenv("TEST_ADMIN_PORT"); adminPort != "" {
		if port, err := strconv.Atoi(adminPort); err == nil {
			config.BaseAdminPort = port
		}
	}

	// Max nodes
	if maxNodes := os.Getenv("TEST_MAX_NODES"); maxNodes != "" {
		if nodes, err := strconv.Atoi(maxNodes); err == nil {
			config.MaxNodes = nodes
		}
	}

	// Test data directory
	if testDataDir := os.Getenv("TEST_DATA_DIR"); testDataDir != "" {
		config.TestDataDir = testDataDir
	}

	// Debug mode
	if debugMode := os.Getenv("TEST_DEBUG"); debugMode != "" {
		config.DebugMode = debugMode == "true" || debugMode == "1"
	}

	// Log level
	if logLevel := os.Getenv("TEST_LOG_LEVEL"); logLevel != "" {
		config.LogLevel = logLevel
	}

	// CI mode
	if ciMode := os.Getenv("CI"); ciMode != "" {
		config.CIMode = ciMode == "true" || ciMode == "1"
	}

	// Short mode
	if shortMode := os.Getenv("TEST_SHORT"); shortMode != "" {
		config.ShortMode = shortMode == "true" || shortMode == "1"
	}

	// Stress test mode
	if stressMode := os.Getenv("TEST_STRESS"); stressMode != "" {
		config.StressTestMode = stressMode == "true" || stressMode == "1"
	}

	// Host
	if host := os.Getenv("TEST_HOST"); host != "" {
		config.Host = host
	}

	// Max concurrent operations
	if maxConcurrentOps := os.Getenv("TEST_MAX_CONCURRENT_OPS"); maxConcurrentOps != "" {
		if ops, err := strconv.Atoi(maxConcurrentOps); err == nil {
			config.MaxConcurrentOps = ops
		}
	}

	// Max file size
	if maxFileSize := os.Getenv("TEST_MAX_FILE_SIZE"); maxFileSize != "" {
		if size, err := strconv.ParseInt(maxFileSize, 10, 64); err == nil {
			config.MaxFileSize = size
		}
	}

	// Cleanup after test
	if cleanupAfterTest := os.Getenv("TEST_CLEANUP_AFTER"); cleanupAfterTest != "" {
		config.CleanupAfterTest = cleanupAfterTest == "true" || cleanupAfterTest == "1"
	}

	// Preserve test data
	if preserveTestData := os.Getenv("TEST_PRESERVE_DATA"); preserveTestData != "" {
		config.PreserveTestData = preserveTestData == "true" || preserveTestData == "1"
	}

	// Max retries
	if maxRetries := os.Getenv("TEST_MAX_RETRIES"); maxRetries != "" {
		if retries, err := strconv.Atoi(maxRetries); err == nil {
			config.MaxRetries = retries
		}
	}

	// Retry delay
	if retryDelay := os.Getenv("TEST_RETRY_DELAY"); retryDelay != "" {
		if delay, err := time.ParseDuration(retryDelay); err == nil {
			config.RetryDelay = delay
		}
	}

	// Network delay
	if networkDelay := os.Getenv("TEST_NETWORK_DELAY"); networkDelay != "" {
		if delay, err := time.ParseDuration(networkDelay); err == nil {
			config.NetworkDelay = delay
		}
	}

	return config
}

// GetCurrentTestConfig returns the current test configuration based on environment
func GetCurrentTestConfig() TestConfig {
	environment := "development"

	// Determine environment from context
	if os.Getenv("CI") == "true" {
		environment = "ci"
	} else if os.Getenv("TEST_SHORT") == "true" {
		environment = "short"
	} else if os.Getenv("TEST_STRESS") == "true" {
		environment = "stress"
	} else if os.Getenv("TEST_PERFORMANCE") == "true" {
		environment = "performance"
	} else if os.Getenv("TEST_LOCAL") == "true" {
		environment = "local"
	}

	return GetTestConfig(environment)
}

// ValidateConfig validates the test configuration
func ValidateConfig(config TestConfig) error {
	if config.MaxNodes <= 0 {
		return os.ErrInvalid
	}

	if config.BasePort <= 0 || config.BasePort > 65535 {
		return os.ErrInvalid
	}

	if config.BaseAdminPort <= 0 || config.BaseAdminPort > 65535 {
		return os.ErrInvalid
	}

	if config.DefaultTimeout <= 0 {
		return os.ErrInvalid
	}

	if config.ReplicationTimeout <= 0 {
		return os.ErrInvalid
	}

	if config.Host == "" {
		return os.ErrInvalid
	}

	if config.TestDataDir == "" {
		return os.ErrInvalid
	}

	return nil
}

// CreateDirectories creates necessary directories for testing
func CreateDirectories(config TestConfig) error {
	directories := []string{
		config.TestDataDir,
		config.TestDataDir + "/logs",
		config.TestDataDir + "/data",
		config.TestDataDir + "/coverage",
	}

	for _, dir := range directories {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	return nil
}

// CleanupDirectories cleans up test directories
func CleanupDirectories(config TestConfig) error {
	if config.PreserveTestData {
		return nil
	}

	return os.RemoveAll(config.TestDataDir)
}

// GetNodeConfig returns configuration for a specific node
func GetNodeConfig(config TestConfig, nodeID int) NodeConfig {
	return NodeConfig{
		NodeID:      nodeID,
		RaftPort:    config.BasePort + nodeID,
		AdminPort:   config.BaseAdminPort + nodeID,
		MonitorPort: config.BaseMonitorPort + nodeID,
		Host:        config.Host,
		DataDir:     config.TestDataDir + "/node" + strconv.Itoa(nodeID),
	}
}

// NodeConfig represents configuration for a single node
type NodeConfig struct {
	NodeID      int
	RaftPort    int
	AdminPort   int
	MonitorPort int
	Host        string
	DataDir     string
}

// GetAddress returns the full address for the node
func (nc NodeConfig) GetAddress() string {
	return nc.Host + ":" + strconv.Itoa(nc.RaftPort)
}

// GetAdminAddress returns the admin address for the node
func (nc NodeConfig) GetAdminAddress() string {
	return nc.Host + ":" + strconv.Itoa(nc.AdminPort)
}

// GetMonitorAddress returns the monitor address for the node
func (nc NodeConfig) GetMonitorAddress() string {
	return nc.Host + ":" + strconv.Itoa(nc.MonitorPort)
}

// GetClusterConfig returns configuration for an entire cluster
func GetClusterConfig(config TestConfig, nodeCount int) ClusterConfig {
	nodes := make([]NodeConfig, nodeCount)
	for i := 0; i < nodeCount; i++ {
		nodes[i] = GetNodeConfig(config, i)
	}

	return ClusterConfig{
		Nodes:       nodes,
		NodeCount:   nodeCount,
		Environment: config.Environment,
	}
}

// ClusterConfig represents configuration for an entire cluster
type ClusterConfig struct {
	Nodes       []NodeConfig
	NodeCount   int
	Environment string
}

// GetNodeAddresses returns all node addresses
func (cc ClusterConfig) GetNodeAddresses() []string {
	addresses := make([]string, len(cc.Nodes))
	for i, node := range cc.Nodes {
		addresses[i] = node.GetAddress()
	}
	return addresses
}

// GetAdminAddresses returns all admin addresses
func (cc ClusterConfig) GetAdminAddresses() []string {
	addresses := make([]string, len(cc.Nodes))
	for i, node := range cc.Nodes {
		addresses[i] = node.GetAdminAddress()
	}
	return addresses
}
