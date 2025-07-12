package admin

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
