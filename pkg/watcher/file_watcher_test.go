package watcher

import (
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestConfig_Fields(t *testing.T) {
	logger := logrus.New()
	config := Config{
		DataDir:      "/tmp/test",
		NodeID:       "test-node",
		Logger:       logger,
		ApplyTimeout: 5 * time.Second,
	}

	// Test that all fields are accessible
	assert.Equal(t, "/tmp/test", config.DataDir)
	assert.Equal(t, "test-node", config.NodeID)
	assert.Equal(t, logger, config.Logger)
	assert.Equal(t, 5*time.Second, config.ApplyTimeout)
}

func TestDefaultStateManager_Creation(t *testing.T) {
	sm := NewDefaultStateManager()
	assert.NotNil(t, sm)
}

func TestDefaultStateManager_Operations(t *testing.T) {
	sm := NewDefaultStateManager()

	// Test setting and getting file state
	testData := []byte("test content")
	sm.UpdateFileState("test.txt", testData)

	hasContent := sm.FileHasContent("test.txt", testData)
	assert.True(t, hasContent)

	// Test with different content
	differentData := []byte("different content")
	hasContent = sm.FileHasContent("test.txt", differentData)
	assert.False(t, hasContent)

	// Test removing file state
	sm.RemoveFileState("test.txt")
	hasContent = sm.FileHasContent("test.txt", testData)
	assert.False(t, hasContent)
}

func TestCommand_Structure(t *testing.T) {
	cmd := Command{
		Op:       "write",
		Path:     "test.txt",
		Data:     []byte("test content"),
		Hash:     "abc123",
		NodeID:   "test-node",
		Sequence: 1,
	}

	// Test that all fields are accessible
	assert.Equal(t, "write", cmd.Op)
	assert.Equal(t, "test.txt", cmd.Path)
	assert.Equal(t, []byte("test content"), cmd.Data)
	assert.Equal(t, "abc123", cmd.Hash)
	assert.Equal(t, "test-node", cmd.NodeID)
	assert.Equal(t, int64(1), cmd.Sequence)
}
