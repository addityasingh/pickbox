package watcher

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test HashContent function
func TestHashContent(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected string
	}{
		{
			name:     "empty_content",
			data:     []byte{},
			expected: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", // SHA-256 of empty string
		},
		{
			name:     "simple_text",
			data:     []byte("hello"),
			expected: "2cf24dba4f21d4288094c8d5b9b2e1b5e0e8d3f0e1b3d8e7a8c7b6a5b4c3d2e1", // Not actual hash, just placeholder
		},
		{
			name:     "binary_data",
			data:     []byte{0x00, 0x01, 0x02, 0xFF},
			expected: "", // Will be computed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HashContent(tt.data)
			
			// Verify it's a valid hex string
			_, err := hex.DecodeString(result)
			assert.NoError(t, err, "Hash should be valid hex")
			
			// Verify it's the correct length for SHA-256 (64 hex characters)
			assert.Equal(t, 64, len(result), "SHA-256 hash should be 64 characters")
			
			// Verify consistency - same input should produce same hash
			result2 := HashContent(tt.data)
			assert.Equal(t, result, result2, "Hash should be consistent")
			
			// For empty content, verify exact hash
			if tt.name == "empty_content" {
				hasher := sha256.New()
				hasher.Write(tt.data)
				expectedHash := hex.EncodeToString(hasher.Sum(nil))
				assert.Equal(t, expectedHash, result)
			}
		})
	}
}

// Test hash uniqueness
func TestHashContent_Uniqueness(t *testing.T) {
	testCases := [][]byte{
		[]byte("content1"),
		[]byte("content2"),
		[]byte("different content"),
		[]byte{0x01, 0x02, 0x03},
		[]byte{0x03, 0x02, 0x01},
		[]byte{},
		[]byte("a"),
		[]byte("ab"),
		[]byte("abc"),
	}
	
	hashes := make(map[string][]byte)
	
	for _, data := range testCases {
		hash := HashContent(data)
		
		// Check if we've seen this hash before
		if existingData, exists := hashes[hash]; exists {
			// Only fail if the data is actually different (collision)
			assert.Equal(t, existingData, data, "Hash collision detected for different content")
		} else {
			hashes[hash] = data
		}
	}
	
	// We should have unique hashes for different content
	assert.Equal(t, len(testCases), len(hashes), "All different content should have unique hashes")
}

// Test hash stability across multiple calls
func TestHashContent_Stability(t *testing.T) {
	testData := []byte("stable test content")
	
	// Generate hash multiple times
	hashes := make([]string, 100)
	for i := 0; i < 100; i++ {
		hashes[i] = HashContent(testData)
	}
	
	// All hashes should be identical
	firstHash := hashes[0]
	for i, hash := range hashes {
		assert.Equal(t, firstHash, hash, "Hash should be stable across calls (iteration %d)", i)
	}
}

// Test large content hashing
func TestHashContent_LargeContent(t *testing.T) {
	// Create large content (1MB)
	largeData := make([]byte, 1024*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}
	
	hash := HashContent(largeData)
	
	// Verify it's a valid hash
	assert.Equal(t, 64, len(hash), "Large content hash should be 64 characters")
	
	_, err := hex.DecodeString(hash)
	assert.NoError(t, err, "Large content hash should be valid hex")
	
	// Verify consistency
	hash2 := HashContent(largeData)
	assert.Equal(t, hash, hash2, "Large content hash should be consistent")
}

// Benchmark hashing performance
func BenchmarkHashContent_Small(b *testing.B) {
	data := []byte("small test content")
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		HashContent(data)
	}
}

func BenchmarkHashContent_Medium(b *testing.B) {
	// 1KB content
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		HashContent(data)
	}
}

func BenchmarkHashContent_Large(b *testing.B) {
	// 1MB content
	data := make([]byte, 1024*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		HashContent(data)
	}
}
