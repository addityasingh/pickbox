// Package watcher provides file system monitoring capabilities for distributed replication.
package watcher

import (
	"sync"
	"sync/atomic"
	"time"
)

// DefaultStateManager implements the StateManager interface.
type DefaultStateManager struct {
	fileStates      map[string]*FileState
	fileStatesMutex sync.RWMutex
	lastSequence    int64
	sequenceMutex   sync.Mutex
}

// NewDefaultStateManager creates a new default state manager.
func NewDefaultStateManager() *DefaultStateManager {
	return &DefaultStateManager{
		fileStates: make(map[string]*FileState),
	}
}

// FileHasContent checks if a file already has the expected content.
func (sm *DefaultStateManager) FileHasContent(path string, expectedData []byte) bool {
	sm.fileStatesMutex.RLock()
	defer sm.fileStatesMutex.RUnlock()

	state, exists := sm.fileStates[path]
	if !exists {
		return false
	}

	expectedHash := HashContent(expectedData)
	return state.Hash == expectedHash
}

// UpdateFileState records the current state of a file.
func (sm *DefaultStateManager) UpdateFileState(path string, data []byte) {
	sm.fileStatesMutex.Lock()
	defer sm.fileStatesMutex.Unlock()

	sm.fileStates[path] = &FileState{
		Hash:         HashContent(data),
		LastModified: time.Now(),
		Size:         int64(len(data)),
	}
}

// RemoveFileState removes tracking for a deleted file.
func (sm *DefaultStateManager) RemoveFileState(path string) {
	sm.fileStatesMutex.Lock()
	defer sm.fileStatesMutex.Unlock()
	delete(sm.fileStates, path)
}

// GetNextSequence returns the next sequence number for operations.
func (sm *DefaultStateManager) GetNextSequence() int64 {
	return atomic.AddInt64(&sm.lastSequence, 1)
}

// GetFileStates returns a copy of all file states for monitoring.
func (sm *DefaultStateManager) GetFileStates() map[string]FileState {
	sm.fileStatesMutex.RLock()
	defer sm.fileStatesMutex.RUnlock()

	result := make(map[string]FileState, len(sm.fileStates))
	for path, state := range sm.fileStates {
		result[path] = *state
	}
	return result
}

// GetFileCount returns the number of files being tracked.
func (sm *DefaultStateManager) GetFileCount() int {
	sm.fileStatesMutex.RLock()
	defer sm.fileStatesMutex.RUnlock()
	return len(sm.fileStates)
}
