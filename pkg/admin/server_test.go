package admin

import (
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/hashicorp/raft"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock Raft for testing
type mockRaft struct {
	state         raft.RaftState
	addVoterError error
	applyError    error
	applyResult   interface{}
}

func newMockRaft() *mockRaft {
	return &mockRaft{
		state: raft.Follower,
	}
}

func (m *mockRaft) State() raft.RaftState {
	return m.state
}

func (m *mockRaft) AddVoter(id raft.ServerID, address raft.ServerAddress, prevIndex uint64, timeout time.Duration) raft.IndexFuture {
	future := &mockIndexFuture{err: m.addVoterError}
	return future
}

func (m *mockRaft) Apply(cmd []byte, timeout time.Duration) raft.ApplyFuture {
	future := &mockApplyFuture{
		err:    m.applyError,
		result: m.applyResult,
	}
	return future
}

func (m *mockRaft) setState(state raft.RaftState) {
	m.state = state
}

func (m *mockRaft) setAddVoterError(err error) {
	m.addVoterError = err
}

func (m *mockRaft) setApplyError(err error) {
	m.applyError = err
}

func (m *mockRaft) setApplyResult(result interface{}) {
	m.applyResult = result
}

// Mock IndexFuture for testing
type mockIndexFuture struct {
	err   error
	index uint64
}

func (f *mockIndexFuture) Error() error {
	return f.err
}

func (f *mockIndexFuture) Index() uint64 {
	return f.index
}

// Mock ApplyFuture for testing
type mockApplyFuture struct {
	err    error
	result interface{}
}

func (f *mockApplyFuture) Error() error {
	return f.err
}

func (f *mockApplyFuture) Response() interface{} {
	return f.result
}

func (f *mockApplyFuture) Index() uint64 {
	return 0
}

// Helper function to get a free port
func getFreePort() int {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// Helper function to create a logger that doesn't output during tests
func createTestLogger() *logrus.Logger {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel) // Suppress logs during tests
	return logger
}

// Test Command JSON serialization
func TestCommand_JSONSerialization(t *testing.T) {
	tests := []struct {
		name string
		cmd  Command
	}{
		{
			name: "write_command",
			cmd: Command{
				Op:       "write",
				Path:     "test/file.txt",
				Data:     []byte("test content"),
				Hash:     "abc123",
				NodeID:   "node1",
				Sequence: 1,
			},
		},
		{
			name: "delete_command",
			cmd: Command{
				Op:       "delete",
				Path:     "test/file.txt",
				Hash:     "",
				NodeID:   "node2",
				Sequence: 2,
			},
		},
		{
			name: "empty_command",
			cmd: Command{
				Op:       "",
				Path:     "",
				Data:     nil,
				Hash:     "",
				NodeID:   "",
				Sequence: 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test marshaling
			data, err := json.Marshal(tt.cmd)
			require.NoError(t, err)
			assert.NotEmpty(t, data)

			// Test unmarshaling
			var unmarshaled Command
			err = json.Unmarshal(data, &unmarshaled)
			require.NoError(t, err)

			// Verify all fields match
			assert.Equal(t, tt.cmd.Op, unmarshaled.Op)
			assert.Equal(t, tt.cmd.Path, unmarshaled.Path)
			assert.Equal(t, tt.cmd.Data, unmarshaled.Data)
			assert.Equal(t, tt.cmd.Hash, unmarshaled.Hash)
			assert.Equal(t, tt.cmd.NodeID, unmarshaled.NodeID)
			assert.Equal(t, tt.cmd.Sequence, unmarshaled.Sequence)
		})
	}
}

// Test NewServer function
func TestNewServer(t *testing.T) {
	t.Run("valid_server_creation", func(t *testing.T) {
		mockRaft := newMockRaft()
		logger := createTestLogger()
		port := getFreePort()

		server := NewServer(mockRaft, port, logger)

		assert.NotNil(t, server)
		assert.Equal(t, mockRaft, server.raft)
		assert.Equal(t, port, server.port)
		assert.Equal(t, logger, server.logger)
	})

	t.Run("nil_logger", func(t *testing.T) {
		mockRaft := newMockRaft()
		port := getFreePort()

		server := NewServer(mockRaft, port, nil)

		assert.NotNil(t, server)
		assert.NotNil(t, server.logger)
	})

	t.Run("zero_port", func(t *testing.T) {
		mockRaft := newMockRaft()
		logger := createTestLogger()

		server := NewServer(mockRaft, 0, logger)

		assert.NotNil(t, server)
		assert.Equal(t, 0, server.port)
	})
}

// Test Server Start method
func TestServer_Start(t *testing.T) {
	t.Run("successful_start", func(t *testing.T) {
		mockRaft := newMockRaft()
		logger := createTestLogger()
		port := getFreePort()

		server := NewServer(mockRaft, port, logger)

		err := server.Start()
		assert.NoError(t, err)

		// Verify server is listening
		time.Sleep(100 * time.Millisecond)
		conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			conn.Close()
		}
		assert.NoError(t, err)
	})

	t.Run("invalid_port", func(t *testing.T) {
		mockRaft := newMockRaft()
		logger := createTestLogger()

		server := NewServer(mockRaft, -1, logger)

		err := server.Start()
		assert.Error(t, err)
	})

	t.Run("port_already_in_use", func(t *testing.T) {
		mockRaft := newMockRaft()
		logger := createTestLogger()
		port := getFreePort()

		// Start first server
		server1 := NewServer(mockRaft, port, logger)
		err := server1.Start()
		require.NoError(t, err)

		// Give it time to start
		time.Sleep(100 * time.Millisecond)

		// Try to start second server on same port
		server2 := NewServer(mockRaft, port, logger)
		err = server2.Start()
		assert.Error(t, err)
	})
}

// Test Server handleConnection method
func TestServer_handleConnection(t *testing.T) {
	t.Run("add_voter_command", func(t *testing.T) {
		mockRaft := newMockRaft()
		logger := createTestLogger()
		port := getFreePort()

		server := NewServer(mockRaft, port, logger)
		err := server.Start()
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		// Connect and send ADD_VOTER command
		conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		require.NoError(t, err)
		defer conn.Close()

		_, err = conn.Write([]byte("ADD_VOTER node1 127.0.0.1:8001"))
		require.NoError(t, err)

		// Read response
		buffer := make([]byte, 1024)
		n, err := conn.Read(buffer)
		require.NoError(t, err)

		response := string(buffer[:n])
		assert.Equal(t, "OK\n", response)
	})

	t.Run("forward_command_as_leader", func(t *testing.T) {
		mockRaft := newMockRaft()
		mockRaft.setState(raft.Leader)
		logger := createTestLogger()
		port := getFreePort()

		server := NewServer(mockRaft, port, logger)
		err := server.Start()
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		// Connect and send FORWARD command
		conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		require.NoError(t, err)
		defer conn.Close()

		cmd := Command{
			Op:       "write",
			Path:     "test.txt",
			Data:     []byte("test"),
			NodeID:   "node1",
			Sequence: 1,
		}
		cmdData, _ := json.Marshal(cmd)

		_, err = conn.Write([]byte(fmt.Sprintf("FORWARD %s", string(cmdData))))
		require.NoError(t, err)

		// Read response
		buffer := make([]byte, 1024)
		n, err := conn.Read(buffer)
		require.NoError(t, err)

		response := string(buffer[:n])
		assert.Equal(t, "OK\n", response)
	})

	t.Run("forward_command_as_follower", func(t *testing.T) {
		mockRaft := newMockRaft()
		mockRaft.setState(raft.Follower)
		logger := createTestLogger()
		port := getFreePort()

		server := NewServer(mockRaft, port, logger)
		err := server.Start()
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		// Connect and send FORWARD command
		conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		require.NoError(t, err)
		defer conn.Close()

		cmd := Command{
			Op:       "write",
			Path:     "test.txt",
			Data:     []byte("test"),
			NodeID:   "node1",
			Sequence: 1,
		}
		cmdData, _ := json.Marshal(cmd)

		_, err = conn.Write([]byte(fmt.Sprintf("FORWARD %s", string(cmdData))))
		require.NoError(t, err)

		// Read response
		buffer := make([]byte, 1024)
		n, err := conn.Read(buffer)
		require.NoError(t, err)

		response := string(buffer[:n])
		assert.Equal(t, "ERROR: Not leader\n", response)
	})

	t.Run("unknown_command", func(t *testing.T) {
		mockRaft := newMockRaft()
		logger := createTestLogger()
		port := getFreePort()

		server := NewServer(mockRaft, port, logger)
		err := server.Start()
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		// Connect and send unknown command
		conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		require.NoError(t, err)
		defer conn.Close()

		_, err = conn.Write([]byte("UNKNOWN_COMMAND"))
		require.NoError(t, err)

		// Read response
		buffer := make([]byte, 1024)
		n, err := conn.Read(buffer)
		require.NoError(t, err)

		response := string(buffer[:n])
		assert.Equal(t, "ERROR: Unknown command\n", response)
	})

	t.Run("connection_read_error", func(t *testing.T) {
		mockRaft := newMockRaft()
		logger := createTestLogger()
		port := getFreePort()

		server := NewServer(mockRaft, port, logger)
		err := server.Start()
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		// Connect and immediately close to trigger read error
		conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		require.NoError(t, err)
		conn.Close()

		// Allow time for server to handle the connection
		time.Sleep(100 * time.Millisecond)
	})
}

// Test Server handleAddVoter method
func TestServer_handleAddVoter(t *testing.T) {
	t.Run("valid_add_voter", func(t *testing.T) {
		mockRaft := newMockRaft()
		logger := createTestLogger()
		port := getFreePort()

		server := NewServer(mockRaft, port, logger)
		err := server.Start()
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		require.NoError(t, err)
		defer conn.Close()

		_, err = conn.Write([]byte("ADD_VOTER node1 127.0.0.1:8001"))
		require.NoError(t, err)

		buffer := make([]byte, 1024)
		n, err := conn.Read(buffer)
		require.NoError(t, err)

		response := string(buffer[:n])
		assert.Equal(t, "OK\n", response)
	})

	t.Run("invalid_add_voter_format", func(t *testing.T) {
		mockRaft := newMockRaft()
		logger := createTestLogger()
		port := getFreePort()

		server := NewServer(mockRaft, port, logger)
		err := server.Start()
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		require.NoError(t, err)
		defer conn.Close()

		_, err = conn.Write([]byte("ADD_VOTER node1"))
		require.NoError(t, err)

		buffer := make([]byte, 1024)
		n, err := conn.Read(buffer)
		require.NoError(t, err)

		response := string(buffer[:n])
		assert.Contains(t, response, "ERROR: Invalid ADD_VOTER command format")
	})

	t.Run("add_voter_raft_error", func(t *testing.T) {
		mockRaft := newMockRaft()
		mockRaft.setAddVoterError(fmt.Errorf("raft error"))
		logger := createTestLogger()
		port := getFreePort()

		server := NewServer(mockRaft, port, logger)
		err := server.Start()
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		require.NoError(t, err)
		defer conn.Close()

		_, err = conn.Write([]byte("ADD_VOTER node1 127.0.0.1:8001"))
		require.NoError(t, err)

		buffer := make([]byte, 1024)
		n, err := conn.Read(buffer)
		require.NoError(t, err)

		response := string(buffer[:n])
		assert.Contains(t, response, "ERROR: raft error")
	})
}

// Test Server handleForward method
func TestServer_handleForward(t *testing.T) {
	t.Run("valid_forward_as_leader", func(t *testing.T) {
		mockRaft := newMockRaft()
		mockRaft.setState(raft.Leader)
		logger := createTestLogger()
		port := getFreePort()

		server := NewServer(mockRaft, port, logger)
		err := server.Start()
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		require.NoError(t, err)
		defer conn.Close()

		cmd := Command{
			Op:       "write",
			Path:     "test.txt",
			Data:     []byte("test"),
			NodeID:   "node1",
			Sequence: 1,
		}
		cmdData, _ := json.Marshal(cmd)

		_, err = conn.Write([]byte(fmt.Sprintf("FORWARD %s", string(cmdData))))
		require.NoError(t, err)

		buffer := make([]byte, 1024)
		n, err := conn.Read(buffer)
		require.NoError(t, err)

		response := string(buffer[:n])
		assert.Equal(t, "OK\n", response)
	})

	t.Run("invalid_forward_format", func(t *testing.T) {
		mockRaft := newMockRaft()
		mockRaft.setState(raft.Leader)
		logger := createTestLogger()
		port := getFreePort()

		server := NewServer(mockRaft, port, logger)
		err := server.Start()
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		require.NoError(t, err)
		defer conn.Close()

		_, err = conn.Write([]byte("FORWARD invalid"))
		require.NoError(t, err)

		buffer := make([]byte, 1024)
		n, err := conn.Read(buffer)
		require.NoError(t, err)

		response := string(buffer[:n])
		assert.Contains(t, response, "ERROR: Invalid FORWARD command format")
	})

	t.Run("forward_apply_error", func(t *testing.T) {
		mockRaft := newMockRaft()
		mockRaft.setState(raft.Leader)
		mockRaft.setApplyError(fmt.Errorf("apply error"))
		logger := createTestLogger()
		port := getFreePort()

		server := NewServer(mockRaft, port, logger)
		err := server.Start()
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		require.NoError(t, err)
		defer conn.Close()

		cmd := Command{
			Op:       "write",
			Path:     "test.txt",
			Data:     []byte("test"),
			NodeID:   "node1",
			Sequence: 1,
		}
		cmdData, _ := json.Marshal(cmd)

		_, err = conn.Write([]byte(fmt.Sprintf("FORWARD %s", string(cmdData))))
		require.NoError(t, err)

		buffer := make([]byte, 1024)
		n, err := conn.Read(buffer)
		require.NoError(t, err)

		response := string(buffer[:n])
		assert.Contains(t, response, "ERROR: apply error")
	})
}

// Test Server writeResponse method
func TestServer_writeResponse(t *testing.T) {
	t.Run("successful_write", func(t *testing.T) {
		mockRaft := newMockRaft()
		logger := createTestLogger()
		port := getFreePort()

		server := NewServer(mockRaft, port, logger)
		err := server.Start()
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		require.NoError(t, err)
		defer conn.Close()

		// The writeResponse method is tested indirectly through other tests
		// but we can test it directly by triggering a command
		_, err = conn.Write([]byte("UNKNOWN_COMMAND"))
		require.NoError(t, err)

		buffer := make([]byte, 1024)
		n, err := conn.Read(buffer)
		require.NoError(t, err)

		response := string(buffer[:n])
		assert.Equal(t, "ERROR: Unknown command\n", response)
	})
}

// Test ForwardToLeader function with invalid address
func TestForwardToLeader_InvalidAddress(t *testing.T) {
	cmd := Command{
		Op:       "write",
		Path:     "test.txt",
		Data:     []byte("test"),
		Hash:     "abc123",
		NodeID:   "node1",
		Sequence: 1,
	}

	// Test with invalid address (should fail)
	err := ForwardToLeader("invalid-address", cmd)
	assert.Error(t, err)
}

// Test ForwardToLeader function with valid address but no server
func TestForwardToLeader_NoServer(t *testing.T) {
	cmd := Command{
		Op:       "write",
		Path:     "test.txt",
		Data:     []byte("test"),
		Hash:     "abc123",
		NodeID:   "node1",
		Sequence: 1,
	}

	// Test with valid address but no server running (should fail)
	err := ForwardToLeader("127.0.0.1:8000", cmd)
	assert.Error(t, err)
}

// Test ForwardToLeader function with working server
func TestForwardToLeader_WorkingServer(t *testing.T) {
	mockRaft := newMockRaft()
	mockRaft.setState(raft.Leader)
	logger := createTestLogger()
	port := 9001 // Use the expected admin port

	server := NewServer(mockRaft, port, logger)
	err := server.Start()
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	cmd := Command{
		Op:       "write",
		Path:     "test.txt",
		Data:     []byte("test"),
		Hash:     "abc123",
		NodeID:   "node1",
		Sequence: 1,
	}

	// Test with working server
	err = ForwardToLeader("127.0.0.1:8000", cmd)
	assert.NoError(t, err)
}

// Test sendForwardCommand function with invalid address
func TestSendForwardCommand_InvalidAddress(t *testing.T) {
	cmd := Command{
		Op:       "write",
		Path:     "test.txt",
		Data:     []byte("test"),
		Hash:     "abc123",
		NodeID:   "node1",
		Sequence: 1,
	}

	// Test with invalid address (should fail)
	err := sendForwardCommand("invalid-address", cmd)
	assert.Error(t, err)
}

// Test sendForwardCommand function with valid address but no server
func TestSendForwardCommand_NoServer(t *testing.T) {
	cmd := Command{
		Op:       "write",
		Path:     "test.txt",
		Data:     []byte("test"),
		Hash:     "abc123",
		NodeID:   "node1",
		Sequence: 1,
	}

	// Test with valid address but no server running (should fail)
	err := sendForwardCommand("127.0.0.1:9999", cmd)
	assert.Error(t, err)
}

// Test sendForwardCommand function with working server
func TestSendForwardCommand_WorkingServer(t *testing.T) {
	mockRaft := newMockRaft()
	mockRaft.setState(raft.Leader)
	logger := createTestLogger()
	port := getFreePort()

	server := NewServer(mockRaft, port, logger)
	err := server.Start()
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	cmd := Command{
		Op:       "write",
		Path:     "test.txt",
		Data:     []byte("test"),
		Hash:     "abc123",
		NodeID:   "node1",
		Sequence: 1,
	}

	// Test with working server
	err = sendForwardCommand(fmt.Sprintf("127.0.0.1:%d", port), cmd)
	assert.NoError(t, err)
}

// Test constants
func TestConstants(t *testing.T) {
	assert.Equal(t, "ADD_VOTER", AddVoterCmd)
	assert.Equal(t, "FORWARD", ForwardCmd)
	assert.Equal(t, 5*time.Second, DefaultApplyTimeout)
}

// Test Command validation
func TestCommand_Validation(t *testing.T) {
	tests := []struct {
		name  string
		cmd   Command
		valid bool
	}{
		{
			name: "valid_write_command",
			cmd: Command{
				Op:       "write",
				Path:     "valid/path.txt",
				Data:     []byte("content"),
				Hash:     "hash123",
				NodeID:   "node1",
				Sequence: 1,
			},
			valid: true,
		},
		{
			name: "valid_delete_command",
			cmd: Command{
				Op:       "delete",
				Path:     "valid/path.txt",
				NodeID:   "node1",
				Sequence: 2,
			},
			valid: true,
		},
		{
			name: "empty_operation",
			cmd: Command{
				Op:       "",
				Path:     "path.txt",
				NodeID:   "node1",
				Sequence: 1,
			},
			valid: false,
		},
		{
			name: "empty_path",
			cmd: Command{
				Op:       "write",
				Path:     "",
				NodeID:   "node1",
				Sequence: 1,
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For now, we just check that the command can be marshaled
			_, err := json.Marshal(tt.cmd)
			assert.NoError(t, err)

			// In a real implementation, we would have a Validate() method
			// For now, we just check basic field presence
			if tt.valid {
				assert.NotEmpty(t, tt.cmd.Op)
				assert.NotEmpty(t, tt.cmd.Path)
				assert.NotEmpty(t, tt.cmd.NodeID)
			}
		})
	}
}

// Test Command struct fields
func TestCommand_Fields(t *testing.T) {
	cmd := Command{
		Op:       "write",
		Path:     "test.txt",
		Data:     []byte("content"),
		Hash:     "hash123",
		NodeID:   "node1",
		Sequence: 42,
	}

	assert.Equal(t, "write", cmd.Op)
	assert.Equal(t, "test.txt", cmd.Path)
	assert.Equal(t, []byte("content"), cmd.Data)
	assert.Equal(t, "hash123", cmd.Hash)
	assert.Equal(t, "node1", cmd.NodeID)
	assert.Equal(t, int64(42), cmd.Sequence)
}

// Test error handling in ForwardToLeader
func TestForwardToLeader_ErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		leaderAddr  string
		expectError bool
	}{
		{
			name:        "invalid_address_format",
			leaderAddr:  "invalid-address",
			expectError: true,
		},
		{
			name:        "valid_address_no_server",
			leaderAddr:  "192.168.255.255:8000", // Non-routable address
			expectError: true,
		},
		{
			name:        "empty_address",
			leaderAddr:  "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := Command{
				Op:       "write",
				Path:     "test.txt",
				Data:     []byte("test"),
				Hash:     "abc123",
				NodeID:   "node1",
				Sequence: 1,
			}

			err := ForwardToLeader(tt.leaderAddr, cmd)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Benchmark Command JSON operations
func BenchmarkCommand_JSONMarshal(b *testing.B) {
	cmd := Command{
		Op:       "write",
		Path:     "benchmark/file.txt",
		Data:     []byte("benchmark data for JSON marshaling performance"),
		Hash:     "abc123",
		NodeID:   "bench-node",
		Sequence: 1,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		json.Marshal(cmd)
	}
}

func BenchmarkCommand_JSONUnmarshal(b *testing.B) {
	cmd := Command{
		Op:       "write",
		Path:     "benchmark/file.txt",
		Data:     []byte("benchmark data for JSON unmarshaling performance"),
		Hash:     "abc123",
		NodeID:   "bench-node",
		Sequence: 1,
	}

	data, _ := json.Marshal(cmd)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var unmarshaled Command
		json.Unmarshal(data, &unmarshaled)
	}
}

// Test large command data
func TestCommand_LargeData(t *testing.T) {
	// Create a command with large data
	largeData := make([]byte, 1024*1024) // 1MB
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	cmd := Command{
		Op:       "write",
		Path:     "large/file.txt",
		Data:     largeData,
		Hash:     "large_hash",
		NodeID:   "node1",
		Sequence: 1,
	}

	// Test marshaling large command
	data, err := json.Marshal(cmd)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Test unmarshaling large command
	var unmarshaled Command
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, cmd.Data, unmarshaled.Data)
}
