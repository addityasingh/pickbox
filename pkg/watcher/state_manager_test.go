package watcher

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test DefaultStateManager creation
func TestNewDefaultStateManager(t *testing.T) {
	sm := NewDefaultStateManager()

	assert.NotNil(t, sm)
	assert.NotNil(t, sm.fileStates)
	assert.Equal(t, int64(0), sm.lastSequence)
}

// Test file content checking
func TestDefaultStateManager_FileHasContent(t *testing.T) {
	sm := NewDefaultStateManager()

	testPath := "test/file.txt"
	testData := []byte("test content")

	// Initially, file should not have content
	assert.False(t, sm.FileHasContent(testPath, testData))

	// Update file state
	sm.UpdateFileState(testPath, testData)

	// Now file should have the content
	assert.True(t, sm.FileHasContent(testPath, testData))

	// Different content should return false
	differentData := []byte("different content")
	assert.False(t, sm.FileHasContent(testPath, differentData))
}

// Test file state updates
func TestDefaultStateManager_UpdateFileState(t *testing.T) {
	sm := NewDefaultStateManager()

	testPath := "test/file.txt"
	testData := []byte("test content")

	// Update file state
	sm.UpdateFileState(testPath, testData)

	// Verify state was stored
	sm.fileStatesMutex.RLock()
	state, exists := sm.fileStates[testPath]
	sm.fileStatesMutex.RUnlock()

	require.True(t, exists)
	assert.Equal(t, HashContent(testData), state.Hash)
	assert.Equal(t, int64(len(testData)), state.Size)
	assert.WithinDuration(t, time.Now(), state.LastModified, time.Second)
}

// Test file state removal
func TestDefaultStateManager_RemoveFileState(t *testing.T) {
	sm := NewDefaultStateManager()

	testPath := "test/file.txt"
	testData := []byte("test content")

	// Add file state
	sm.UpdateFileState(testPath, testData)

	// Verify it exists
	assert.True(t, sm.FileHasContent(testPath, testData))

	// Remove file state
	sm.RemoveFileState(testPath)

	// Verify it was removed
	assert.False(t, sm.FileHasContent(testPath, testData))
}

// Test sequence generation
func TestDefaultStateManager_GetNextSequence(t *testing.T) {
	sm := NewDefaultStateManager()

	// Initial sequence should be 1
	seq1 := sm.GetNextSequence()
	assert.Equal(t, int64(1), seq1)

	// Next sequence should be 2
	seq2 := sm.GetNextSequence()
	assert.Equal(t, int64(2), seq2)

	// Sequences should be monotonically increasing
	for i := 3; i <= 10; i++ {
		seq := sm.GetNextSequence()
		assert.Equal(t, int64(i), seq)
	}
}

// Test concurrent sequence generation
func TestDefaultStateManager_ConcurrentSequenceGeneration(t *testing.T) {
	sm := NewDefaultStateManager()

	numGoroutines := 10
	sequencesPerGoroutine := 100
	totalSequences := numGoroutines * sequencesPerGoroutine

	sequences := make([]int64, totalSequences)
	var wg sync.WaitGroup

	// Generate sequences concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < sequencesPerGoroutine; j++ {
				idx := goroutineID*sequencesPerGoroutine + j
				sequences[idx] = sm.GetNextSequence()
			}
		}(i)
	}

	wg.Wait()

	// Verify all sequences are unique
	sequenceMap := make(map[int64]bool)
	for _, seq := range sequences {
		assert.False(t, sequenceMap[seq], "Duplicate sequence found: %d", seq)
		sequenceMap[seq] = true
	}

	// Verify we have all expected sequences
	assert.Equal(t, totalSequences, len(sequenceMap))
}

// Test empty file content
func TestDefaultStateManager_EmptyFileContent(t *testing.T) {
	sm := NewDefaultStateManager()

	testPath := "empty/file.txt"
	emptyData := []byte{}

	// Update with empty data
	sm.UpdateFileState(testPath, emptyData)

	// Should recognize empty content
	assert.True(t, sm.FileHasContent(testPath, emptyData))

	// Should not match non-empty content
	nonEmptyData := []byte("not empty")
	assert.False(t, sm.FileHasContent(testPath, nonEmptyData))
}

// Benchmark sequence generation
func BenchmarkDefaultStateManager_GetNextSequence(b *testing.B) {
	sm := NewDefaultStateManager()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sm.GetNextSequence()
	}
}
