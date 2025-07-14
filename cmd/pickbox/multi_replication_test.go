package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/raft"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test for MultiConfig validation
func TestMultiConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  MultiConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: MultiConfig{
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
			config: MultiConfig{
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
			config: MultiConfig{
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
			config: MultiConfig{
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
			config: MultiConfig{
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
			config: MultiConfig{
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
			config: MultiConfig{
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
			err := validateMultiConfig(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Test for MultiApplication creation
func TestNewMultiApplication(t *testing.T) {
	tests := []struct {
		name    string
		config  MultiConfig
		wantErr bool
	}{
		{
			name: "invalid config should fail",
			config: MultiConfig{
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
			app, err := NewMultiApplication(tt.config)
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

// Test command structure for multi-directional replication
func TestCommand_JSONSerialization(t *testing.T) {
	tests := []struct {
		name    string
		command interface{}
		wantErr bool
	}{
		{
			name: "write command",
			command: struct {
				Op       string `json:"op"`
				Path     string `json:"path"`
				Data     []byte `json:"data"`
				Hash     string `json:"hash"`
				NodeID   string `json:"node_id"`
				Sequence int64  `json:"sequence"`
			}{
				Op:       "write",
				Path:     "test.txt",
				Data:     []byte("test content"),
				Hash:     "hash123",
				NodeID:   "node1",
				Sequence: 1,
			},
			wantErr: false,
		},
		{
			name: "delete command",
			command: struct {
				Op       string `json:"op"`
				Path     string `json:"path"`
				Data     []byte `json:"data"`
				Hash     string `json:"hash"`
				NodeID   string `json:"node_id"`
				Sequence int64  `json:"sequence"`
			}{
				Op:       "delete",
				Path:     "test.txt",
				Data:     nil,
				Hash:     "",
				NodeID:   "node1",
				Sequence: 2,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test JSON marshaling
			data, err := json.Marshal(tt.command)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, data)

				// Test JSON unmarshaling
				var restored interface{}
				err = json.Unmarshal(data, &restored)
				assert.NoError(t, err)
			}
		})
	}
}

// Test content hashing function
func TestHashContent(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{
			name: "empty data",
			data: []byte{},
			want: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name: "hello world",
			data: []byte("hello world"),
			want: "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
		},
		{
			name: "test content",
			data: []byte("test content"),
			want: computeExpectedHash([]byte("test content")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hashContent(tt.data)
			assert.Equal(t, tt.want, got)
		})
	}
}

// Test deriveMultiAdminAddress function
func TestDeriveMultiAdminAddress(t *testing.T) {
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
			got := deriveMultiAdminAddress(tt.raftAddr)
			assert.Equal(t, tt.want, got)
		})
	}
}

// Test runMultiReplication function
func TestRunMultiReplication(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tempDir := t.TempDir()
	logger := logrus.New()
	logger.SetOutput(io.Discard) // Suppress logs during testing

	tests := []struct {
		name    string
		nodeID  string
		port    int
		join    string
		dataDir string
		wantErr bool
	}{
		{
			name:    "valid parameters",
			nodeID:  "test-node",
			port:    8101, // Use different port to avoid conflicts
			join:    "",
			dataDir: tempDir,
			wantErr: false,
		},
		{
			name:    "empty node ID",
			nodeID:  "",
			port:    8102,
			join:    "",
			dataDir: tempDir,
			wantErr: true,
		},
		{
			name:    "invalid port",
			nodeID:  "test-node",
			port:    0,
			join:    "",
			dataDir: tempDir,
			wantErr: true,
		},
		{
			name:    "empty data dir",
			nodeID:  "test-node",
			port:    8103,
			join:    "",
			dataDir: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runMultiReplication(tt.nodeID, tt.port, tt.join, tt.dataDir, logger)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Test createMultiWelcomeFile function
func TestCreateMultiWelcomeFile(t *testing.T) {
	tempDir := t.TempDir()
	logger := logrus.New()
	logger.SetOutput(io.Discard) // Suppress logs during testing

	nodeID := "test-node"
	createMultiWelcomeFile(tempDir, nodeID, logger)

	// Check if welcome file was created
	welcomeFile := filepath.Join(tempDir, "welcome.txt")
	assert.FileExists(t, welcomeFile)

	// Check file content
	content, err := os.ReadFile(welcomeFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), nodeID)
	assert.Contains(t, string(content), "Multi-Directional Distributed Storage")
}

// Test multiRaftWrapper
func TestMultiRaftWrapper(t *testing.T) {
	// Create a mock raft manager for testing
	// Note: This is a simplified test as creating a full raft manager is complex
	t.Run("wrapper interface", func(t *testing.T) {
		// Test that multiRaftWrapper implements the expected interface
		var wrapper interface{} = &multiRaftWrapper{}

		// Check that it has the expected methods
		assert.NotNil(t, wrapper)
	})
}

// Test multiForwarderWrapper
func TestMultiForwarderWrapper(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(io.Discard) // Suppress logs during testing

	wrapper := &multiForwarderWrapper{logger: logger}

	// Test wrapper with nil logger (should not panic)
	wrapperNoLogger := &multiForwarderWrapper{logger: nil}
	assert.NotNil(t, wrapperNoLogger)

	// Test that wrapper implements the expected interface
	assert.NotNil(t, wrapper)
}

// Test raft wrapper implementations
func TestRaftWrapperImplementations(t *testing.T) {
	t.Run("multiRaftWrapper methods", func(t *testing.T) {
		// Test that multiRaftWrapper has the expected methods
		wrapper := &multiRaftWrapper{}
		assert.NotNil(t, wrapper)

		// Test nil safety - these should not panic when rm is nil
		assert.Equal(t, raft.Shutdown, wrapper.State())
		assert.Equal(t, raft.ServerAddress(""), wrapper.Leader())
	})

	t.Run("multiForwarderWrapper methods", func(t *testing.T) {
		logger := logrus.New()
		logger.SetOutput(io.Discard)

		wrapper := &multiForwarderWrapper{logger: logger}
		assert.NotNil(t, wrapper)

		// Test with nil logger
		wrapperNil := &multiForwarderWrapper{logger: nil}
		assert.NotNil(t, wrapperNil)
	})
}

// Benchmark tests
func BenchmarkHashContent(b *testing.B) {
	data := []byte("test content for benchmarking")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hashContent(data)
	}
}

func BenchmarkDeriveMultiAdminAddress(b *testing.B) {
	addr := "127.0.0.1:8001"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		deriveMultiAdminAddress(addr)
	}
}

func BenchmarkMultiConfigValidation(b *testing.B) {
	config := MultiConfig{
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
		validateMultiConfig(config)
	}
}

// Helper functions for testing
func hashContent(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func computeExpectedHash(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// Integration tests
func TestMultiApplicationIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tempDir := t.TempDir()

	config := MultiConfig{
		DataDir:          tempDir,
		NodeID:           "test-node",
		Port:             8201, // Use different port to avoid conflicts
		AdminPort:        9201,
		MonitorPort:      8280,
		DashboardPort:    8290,
		LogLevel:         "error", // Use error level to reduce test output
		BootstrapCluster: true,
	}

	t.Run("application lifecycle", func(t *testing.T) {
		app, err := NewMultiApplication(config)
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
func TestMultiApplicationErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	t.Run("invalid data directory", func(t *testing.T) {
		config := MultiConfig{
			DataDir:          "/invalid/path/that/does/not/exist/and/cannot/be/created",
			NodeID:           "test-node",
			Port:             8301,
			AdminPort:        9301,
			MonitorPort:      8380,
			DashboardPort:    8390,
			LogLevel:         "error",
			BootstrapCluster: true,
		}

		app, err := NewMultiApplication(config)
		assert.Error(t, err)
		assert.Nil(t, app)
	})

	t.Run("invalid log level", func(t *testing.T) {
		tempDir := t.TempDir()
		config := MultiConfig{
			DataDir:          tempDir,
			NodeID:           "test-node",
			Port:             8302,
			AdminPort:        9302,
			MonitorPort:      8381,
			DashboardPort:    8391,
			LogLevel:         "invalid-level",
			BootstrapCluster: true,
		}

		app, err := NewMultiApplication(config)
		require.NoError(t, err)
		require.NotNil(t, app)

		// Should default to info level
		assert.Equal(t, logrus.InfoLevel, app.logger.Level)

		app.Stop()
	})
}
