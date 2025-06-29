// Package watcher provides file system monitoring capabilities for distributed replication.
package watcher

import (
	"crypto/sha256"
	"encoding/hex"
)

// HashContent computes the SHA-256 hash of data.
func HashContent(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
