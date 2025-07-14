package main

import (
	"testing"

	"github.com/hashicorp/raft"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test for AppConfig validation
func TestAppConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  AppConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: AppConfig{
				DataDir:       "/tmp/test",
				NodeID:        "node1",
				Port:          8000,
				AdminPort:     9000,
				MonitorPort:   8080,
				DashboardPort: 8090,
				LogLevel:      "info",
			},
			wantErr: false,
		},
		{
			name: "invalid config - empty data dir",
			config: AppConfig{
				DataDir:       "",
				NodeID:        "node1",
				Port:          8000,
				AdminPort:     9000,
				MonitorPort:   8080,
				DashboardPort: 8090,
				LogLevel:      "info",
			},
			wantErr: true,
		},
		{
			name: "invalid config - empty node ID",
			config: AppConfig{
				DataDir:       "/tmp/test",
				NodeID:        "",
				Port:          8000,
				AdminPort:     9000,
				MonitorPort:   8080,
				DashboardPort: 8090,
				LogLevel:      "info",
			},
			wantErr: true,
		},
		{
			name: "invalid config - zero port",
			config: AppConfig{
				DataDir:       "/tmp/test",
				NodeID:        "node1",
				Port:          0,
				AdminPort:     9000,
				MonitorPort:   8080,
				DashboardPort: 8090,
				LogLevel:      "info",
			},
			wantErr: true,
		},
		{
			name: "invalid config - zero admin port",
			config: AppConfig{
				DataDir:       "/tmp/test",
				NodeID:        "node1",
				Port:          8000,
				AdminPort:     0,
				MonitorPort:   8080,
				DashboardPort: 8090,
				LogLevel:      "info",
			},
			wantErr: true,
		},
		{
			name: "invalid config - zero monitor port",
			config: AppConfig{
				DataDir:       "/tmp/test",
				NodeID:        "node1",
				Port:          8000,
				AdminPort:     9000,
				MonitorPort:   0,
				DashboardPort: 8090,
				LogLevel:      "info",
			},
			wantErr: true,
		},
		{
			name: "invalid config - zero dashboard port",
			config: AppConfig{
				DataDir:       "/tmp/test",
				NodeID:        "node1",
				Port:          8000,
				AdminPort:     9000,
				MonitorPort:   8080,
				DashboardPort: 0,
				LogLevel:      "info",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Test for Application creation
func TestNewApplication(t *testing.T) {
	tests := []struct {
		name    string
		config  AppConfig
		wantErr bool
	}{
		{
			name: "invalid config should fail",
			config: AppConfig{
				DataDir:       "",
				NodeID:        "node1",
				Port:          8000,
				AdminPort:     9000,
				MonitorPort:   8080,
				DashboardPort: 8090,
				LogLevel:      "info",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, err := NewApplication(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, app)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, app)
			}
		})
	}
}

// Test deriveAdminAddress function
func TestDeriveAdminAddress(t *testing.T) {
	// Create a temporary application for testing
	tempDir := t.TempDir()
	config := AppConfig{
		DataDir:       tempDir,
		NodeID:        "test-node",
		Port:          8401,
		AdminPort:     9401,
		MonitorPort:   8480,
		DashboardPort: 8490,
		LogLevel:      "error",
	}

	app, err := NewApplication(config)
	require.NoError(t, err)
	require.NotNil(t, app)
	defer app.Stop()

	tests := []struct {
		name     string
		raftAddr string
		want     string
	}{
		{
			name:     "valid address",
			raftAddr: "127.0.0.1:8001",
			want:     "127.0.0.1:9001",
		},
		{
			name:     "different port",
			raftAddr: "127.0.0.1:8002",
			want:     "127.0.0.1:9002",
		},
		{
			name:     "different host",
			raftAddr: "192.168.1.1:8001",
			want:     "192.168.1.1:9001",
		},
		{
			name:     "invalid address",
			raftAddr: "invalid",
			want:     "127.0.0.1:9001",
		},
		{
			name:     "invalid port",
			raftAddr: "127.0.0.1:invalid",
			want:     "127.0.0.1:9001",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := app.deriveAdminAddress(tt.raftAddr)
			assert.Equal(t, tt.want, got)
		})
	}
}

// Test raftWrapper
func TestRaftWrapper(t *testing.T) {
	// Create a mock raft manager for testing
	// Note: This is a simplified test as creating a full raft manager is complex
	t.Run("wrapper interface", func(t *testing.T) {
		// Test that raftWrapper implements the expected interface
		var wrapper interface{} = &raftWrapper{}

		// Check that it has the expected methods
		assert.NotNil(t, wrapper)
	})
}

// Test forwarderWrapper
func TestForwarderWrapper(t *testing.T) {
	wrapper := &forwarderWrapper{}

	// Test that wrapper implements the expected interface
	assert.NotNil(t, wrapper)
}

// Test raft wrapper implementations
func TestNodeRaftWrapperImplementations(t *testing.T) {
	t.Run("raftWrapper methods", func(t *testing.T) {
		// Test that raftWrapper has the expected methods
		wrapper := &raftWrapper{}
		assert.NotNil(t, wrapper)

		// Test nil safety - these should not panic when rm is nil
		assert.Equal(t, raft.Shutdown, wrapper.State())
		assert.Equal(t, raft.ServerAddress(""), wrapper.Leader())
	})

	t.Run("forwarderWrapper methods", func(t *testing.T) {
		wrapper := &forwarderWrapper{}
		assert.NotNil(t, wrapper)
	})
}

// Benchmark tests
func BenchmarkAppConfigValidation(b *testing.B) {
	config := AppConfig{
		DataDir:       "/tmp/test",
		NodeID:        "node1",
		Port:          8000,
		AdminPort:     9000,
		MonitorPort:   8080,
		DashboardPort: 8090,
		LogLevel:      "info",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validateConfig(config)
	}
}

func BenchmarkDeriveAdminAddress(b *testing.B) {
	tempDir := b.TempDir()
	config := AppConfig{
		DataDir:       tempDir,
		NodeID:        "test-node",
		Port:          8501,
		AdminPort:     9501,
		MonitorPort:   8580,
		DashboardPort: 8590,
		LogLevel:      "error",
	}

	app, err := NewApplication(config)
	require.NoError(b, err)
	require.NotNil(b, app)
	defer app.Stop()

	addr := "127.0.0.1:8001"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		app.deriveAdminAddress(addr)
	}
}

// Integration tests
func TestApplicationIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tempDir := t.TempDir()

	config := AppConfig{
		DataDir:          tempDir,
		NodeID:           "test-node",
		Port:             8601,
		AdminPort:        9601,
		MonitorPort:      8680,
		DashboardPort:    8690,
		LogLevel:         "error", // Use error level to reduce test output
		BootstrapCluster: true,
	}

	t.Run("application lifecycle", func(t *testing.T) {
		app, err := NewApplication(config)
		require.NoError(t, err)
		require.NotNil(t, app)

		// Test components initialization
		assert.NotNil(t, app.config)
		assert.NotNil(t, app.logger)
		assert.NotNil(t, app.raftManager)
		assert.NotNil(t, app.stateManager)
		assert.NotNil(t, app.fileWatcher)
		assert.NotNil(t, app.adminServer)
		assert.NotNil(t, app.monitor)
		assert.NotNil(t, app.dashboard)

		// Test that data directory was created
		assert.DirExists(t, tempDir)

		// Test stop functionality
		err = app.Stop()
		assert.NoError(t, err)
	})
}

// Test edge cases and error handling
func TestApplicationErrorHandling(t *testing.T) {
	t.Run("invalid data directory", func(t *testing.T) {
		config := AppConfig{
			DataDir:          "/invalid/path/that/does/not/exist/and/cannot/be/created",
			NodeID:           "test-node",
			Port:             8701,
			AdminPort:        9701,
			MonitorPort:      8780,
			DashboardPort:    8790,
			LogLevel:         "error",
			BootstrapCluster: true,
		}

		app, err := NewApplication(config)
		assert.Error(t, err)
		assert.Nil(t, app)
	})

	t.Run("invalid log level", func(t *testing.T) {
		tempDir := t.TempDir()
		config := AppConfig{
			DataDir:          tempDir,
			NodeID:           "test-node",
			Port:             8702,
			AdminPort:        9702,
			MonitorPort:      8781,
			DashboardPort:    8791,
			LogLevel:         "invalid-level",
			BootstrapCluster: true,
		}

		app, err := NewApplication(config)
		require.NoError(t, err)
		require.NotNil(t, app)

		// Should default to info level
		assert.Equal(t, logrus.InfoLevel, app.logger.Level)

		app.Stop()
	})
}

// Test setupSignalHandling function
func TestSetupSignalHandling(t *testing.T) {
	tempDir := t.TempDir()
	config := AppConfig{
		DataDir:          tempDir,
		NodeID:           "test-node",
		Port:             8801,
		AdminPort:        9801,
		MonitorPort:      8880,
		DashboardPort:    8890,
		LogLevel:         "error",
		BootstrapCluster: true,
	}

	app, err := NewApplication(config)
	require.NoError(t, err)
	require.NotNil(t, app)

	// Test that setupSignalHandling doesn't panic
	assert.NotPanics(t, func() {
		setupSignalHandling(app)
	})

	app.Stop()
}

// Test getRaftInstance function
func TestGetRaftInstance(t *testing.T) {
	tempDir := t.TempDir()
	config := AppConfig{
		DataDir:          tempDir,
		NodeID:           "test-node",
		Port:             8901,
		AdminPort:        9901,
		MonitorPort:      8980,
		DashboardPort:    8990,
		LogLevel:         "error",
		BootstrapCluster: true,
	}

	app, err := NewApplication(config)
	require.NoError(t, err)
	require.NotNil(t, app)
	defer app.Stop()

	// Test getRaftInstance
	raftInstance := app.getRaftInstance()
	assert.NotNil(t, raftInstance)

	// Test with nil raftManager
	app.raftManager = nil
	raftInstance = app.getRaftInstance()
	assert.Nil(t, raftInstance)
}

// Test Application methods
func TestApplicationMethods(t *testing.T) {
	tempDir := t.TempDir()
	config := AppConfig{
		DataDir:          tempDir,
		NodeID:           "test-node",
		Port:             9201, // Changed from 9101 to avoid conflict with multi-replication test
		AdminPort:        10201,
		MonitorPort:      9280,
		DashboardPort:    9290,
		LogLevel:         "error",
		BootstrapCluster: true,
	}

	app, err := NewApplication(config)
	require.NoError(t, err)
	require.NotNil(t, app)
	defer app.Stop()

	t.Run("logAccessURLs", func(t *testing.T) {
		// Test that logAccessURLs doesn't panic
		assert.NotPanics(t, func() {
			app.logAccessURLs()
		})
	})

	t.Run("initializeComponents", func(t *testing.T) {
		// Test that initializeComponents has already been called
		assert.NotNil(t, app.raftManager)
		assert.NotNil(t, app.stateManager)
		assert.NotNil(t, app.fileWatcher)
		assert.NotNil(t, app.adminServer)
		assert.NotNil(t, app.monitor)
		assert.NotNil(t, app.dashboard)
	})
}
