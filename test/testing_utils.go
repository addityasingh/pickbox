package test

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Common test utilities to reduce code duplication across test files

// TestEnvironment provides a consistent testing environment setup
type TestEnvironment struct {
	TempDir     string
	DataDirs    []string
	Cleanup     []func()
	mutex       sync.Mutex
	initialized bool
}

// NewTestEnvironment creates a new test environment with proper cleanup
func NewTestEnvironment(t *testing.T) *TestEnvironment {
	tempDir := t.TempDir()
	env := &TestEnvironment{
		TempDir:     tempDir,
		DataDirs:    make([]string, 0),
		Cleanup:     make([]func(), 0),
		initialized: true,
	}

	// Register cleanup
	t.Cleanup(func() {
		env.CleanupAll()
	})

	return env
}

// CreateDataDir creates a new data directory for testing
func (env *TestEnvironment) CreateDataDir(name string) string {
	env.mutex.Lock()
	defer env.mutex.Unlock()

	dataDir := filepath.Join(env.TempDir, name)
	err := os.MkdirAll(dataDir, 0755)
	if err != nil {
		panic(fmt.Sprintf("Failed to create data directory: %v", err))
	}

	env.DataDirs = append(env.DataDirs, dataDir)
	return dataDir
}

// CreateTempFile creates a temporary file with given content
func (env *TestEnvironment) CreateTempFile(name, content string) string {
	env.mutex.Lock()
	defer env.mutex.Unlock()

	filePath := filepath.Join(env.TempDir, name)
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		panic(fmt.Sprintf("Failed to create directory: %v", err))
	}

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		panic(fmt.Sprintf("Failed to write file: %v", err))
	}

	return filePath
}

// AddCleanupFunc adds a cleanup function to be called at the end
func (env *TestEnvironment) AddCleanupFunc(f func()) {
	env.mutex.Lock()
	defer env.mutex.Unlock()
	env.Cleanup = append(env.Cleanup, f)
}

// CleanupAll cleans up all resources
func (env *TestEnvironment) CleanupAll() {
	env.mutex.Lock()
	defer env.mutex.Unlock()

	for _, cleanup := range env.Cleanup {
		cleanup()
	}
	env.Cleanup = nil
}

// TestFile represents a test file with metadata
type TestFile struct {
	Path    string
	Content string
	Size    int64
	Hash    string
}

// CreateTestFile creates a test file with specified properties
func CreateTestFile(dir, name, content string) *TestFile {
	filePath := filepath.Join(dir, name)
	dirPath := filepath.Dir(filePath)

	if err := os.MkdirAll(dirPath, 0755); err != nil {
		panic(fmt.Sprintf("Failed to create directory: %v", err))
	}

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		panic(fmt.Sprintf("Failed to write file: %v", err))
	}

	return &TestFile{
		Path:    filePath,
		Content: content,
		Size:    int64(len(content)),
		Hash:    HashContent([]byte(content)),
	}
}

// CreateRandomFile creates a file with random content of specified size
func CreateRandomFile(dir, name string, size int64) *TestFile {
	content := make([]byte, size)
	if _, err := rand.Read(content); err != nil {
		panic(fmt.Sprintf("Failed to generate random content: %v", err))
	}

	filePath := filepath.Join(dir, name)
	dirPath := filepath.Dir(filePath)

	if err := os.MkdirAll(dirPath, 0755); err != nil {
		panic(fmt.Sprintf("Failed to create directory: %v", err))
	}

	if err := os.WriteFile(filePath, content, 0644); err != nil {
		panic(fmt.Sprintf("Failed to write file: %v", err))
	}

	return &TestFile{
		Path:    filePath,
		Content: string(content),
		Size:    size,
		Hash:    HashContent(content),
	}
}

// CreateBinaryFile creates a test file with binary content
func CreateBinaryFile(dir, name string, size int64) *TestFile {
	content := make([]byte, size)
	// Create a pattern for binary content
	for i := range content {
		content[i] = byte(i % 256)
	}

	filePath := filepath.Join(dir, name)
	dirPath := filepath.Dir(filePath)

	if err := os.MkdirAll(dirPath, 0755); err != nil {
		panic(fmt.Sprintf("Failed to create directory: %v", err))
	}

	if err := os.WriteFile(filePath, content, 0644); err != nil {
		panic(fmt.Sprintf("Failed to write file: %v", err))
	}

	return &TestFile{
		Path:    filePath,
		Content: string(content),
		Size:    size,
		Hash:    HashContent(content),
	}
}

// ReadFileContent reads and returns file content
func ReadFileContent(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// FileExists checks if a file exists
func FileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return err == nil
}

// FilesEqual compares two files for equality
func FilesEqual(file1, file2 string) bool {
	content1, err1 := ReadFileContent(file1)
	content2, err2 := ReadFileContent(file2)

	if err1 != nil || err2 != nil {
		return false
	}

	return content1 == content2
}

// HashContent calculates SHA-256 hash of content
func HashContent(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// WaitForCondition waits for a condition to be true with timeout
func WaitForCondition(condition func() bool, timeout time.Duration, interval time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-timer.C:
			return false
		case <-ticker.C:
			if condition() {
				return true
			}
		}
	}
}

// WaitForFile waits for a file to exist with specific content
func WaitForFile(filePath, expectedContent string, timeout time.Duration) bool {
	return WaitForCondition(func() bool {
		content, err := ReadFileContent(filePath)
		return err == nil && content == expectedContent
	}, timeout, 100*time.Millisecond)
}

// WaitForFileExists waits for a file to exist
func WaitForFileExists(filePath string, timeout time.Duration) bool {
	return WaitForCondition(func() bool {
		return FileExists(filePath)
	}, timeout, 100*time.Millisecond)
}

// GenerateTestData generates test data of various types
type TestDataGenerator struct {
	counter int
	mutex   sync.Mutex
}

func NewTestDataGenerator() *TestDataGenerator {
	return &TestDataGenerator{}
}

func (g *TestDataGenerator) NextFileName() string {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.counter++
	return fmt.Sprintf("test_file_%d.txt", g.counter)
}

func (g *TestDataGenerator) NextContent() string {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.counter++
	return fmt.Sprintf("Test content %d - %s", g.counter, time.Now().Format(time.RFC3339))
}

// CreateNestedStructure creates a nested directory structure for testing
func CreateNestedStructure(baseDir string, depth int, filesPerDir int) []string {
	var files []string

	var createLevel func(dir string, currentDepth int)
	createLevel = func(dir string, currentDepth int) {
		// Create directory
		if err := os.MkdirAll(dir, 0755); err != nil {
			panic(fmt.Sprintf("Failed to create directory: %v", err))
		}

		// Create files at this level
		for i := 0; i < filesPerDir; i++ {
			fileName := fmt.Sprintf("file_%d_%d.txt", currentDepth, i)
			filePath := filepath.Join(dir, fileName)
			content := fmt.Sprintf("Content for file at depth %d, index %d", currentDepth, i)

			if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
				panic(fmt.Sprintf("Failed to write file: %v", err))
			}

			files = append(files, filePath)
		}

		// Create subdirectories
		if currentDepth < depth {
			for i := 0; i < 2; i++ {
				subDir := filepath.Join(dir, fmt.Sprintf("subdir_%d_%d", currentDepth, i))
				createLevel(subDir, currentDepth+1)
			}
		}
	}

	createLevel(baseDir, 1)
	return files
}

// PortManager manages port allocation for tests
type PortManager struct {
	basePort int
	mutex    sync.Mutex
}

func NewPortManager(basePort int) *PortManager {
	return &PortManager{basePort: basePort}
}

func (pm *PortManager) AllocatePort() int {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()
	port := pm.basePort
	pm.basePort++
	return port
}

func (pm *PortManager) AllocatePortRange(count int) []int {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	ports := make([]int, count)
	for i := 0; i < count; i++ {
		ports[i] = pm.basePort + i
	}
	pm.basePort += count
	return ports
}

// TestMetrics provides metrics collection for tests
type TestMetrics struct {
	operations int
	errors     int
	startTime  time.Time
	durations  []time.Duration
	mutex      sync.Mutex
}

func NewTestMetrics() *TestMetrics {
	return &TestMetrics{
		startTime: time.Now(),
		durations: make([]time.Duration, 0),
	}
}

func (m *TestMetrics) RecordOperation(duration time.Duration) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.operations++
	m.durations = append(m.durations, duration)
}

func (m *TestMetrics) RecordError() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.errors++
}

func (m *TestMetrics) GetStats() (int, int, time.Duration, time.Duration) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	totalDuration := time.Since(m.startTime)
	var avgDuration time.Duration

	if len(m.durations) > 0 {
		var sum time.Duration
		for _, d := range m.durations {
			sum += d
		}
		avgDuration = sum / time.Duration(len(m.durations))
	}

	return m.operations, m.errors, totalDuration, avgDuration
}

// Retry helper for flaky tests
func Retry(t *testing.T, attempts int, delay time.Duration, fn func() error) {
	var lastErr error
	for i := 0; i < attempts; i++ {
		lastErr = fn()
		if lastErr == nil {
			return
		}
		if i < attempts-1 {
			time.Sleep(delay)
		}
	}
	require.NoError(t, lastErr, "Operation failed after %d attempts", attempts)
}

// EnsureCleanState ensures a clean state for testing
func EnsureCleanState(t *testing.T, paths ...string) {
	for _, path := range paths {
		if strings.Contains(path, "pickbox") || strings.Contains(path, "test") {
			os.RemoveAll(path)
		}
	}
}

// CopyFile copies a file from src to dst
func CopyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// ValidateTestFile validates a test file meets expected criteria
func ValidateTestFile(t *testing.T, file *TestFile) {
	require.NotEmpty(t, file.Path, "File path should not be empty")
	require.NotEmpty(t, file.Hash, "File hash should not be empty")
	require.True(t, FileExists(file.Path), "File should exist at path: %s", file.Path)

	actualContent, err := ReadFileContent(file.Path)
	require.NoError(t, err, "Should be able to read file content")
	require.Equal(t, file.Content, actualContent, "File content should match expected")

	actualHash := HashContent([]byte(actualContent))
	require.Equal(t, file.Hash, actualHash, "File hash should match expected")
}

// GetTestTimeout returns appropriate timeout for test type
func GetTestTimeout(testType string) time.Duration {
	switch testType {
	case "unit":
		return 30 * time.Second
	case "integration":
		return 5 * time.Minute
	case "performance":
		return 10 * time.Minute
	case "failure":
		return 3 * time.Minute
	default:
		return 2 * time.Minute
	}
}

// SkipIfShort skips test if running in short mode
func SkipIfShort(t *testing.T, reason string) {
	if testing.Short() {
		t.Skip(reason)
	}
}
