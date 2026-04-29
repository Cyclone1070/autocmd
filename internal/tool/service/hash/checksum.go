// Package hash provides utilities for computing and managing file checksums.
package hash

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

// ChecksumManager is a thread-safe checksum manager.
// It uses SHA-256 for checksum computation and stores checksums in an in-memory map.
type ChecksumManager struct {
	store map[string]string
	mu    sync.RWMutex
}

// NewChecksumManager creates a new thread-safe checksum manager instance.
func NewChecksumManager() *ChecksumManager {
	return &ChecksumManager{
		store: make(map[string]string),
	}
}

// Compute computes the SHA-256 checksum of data and returns it as a hex string.
func (m *ChecksumManager) Compute(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// Get retrieves the cached checksum for a file path.
// Returns the checksum and true if found, or empty string and false if not cached.
func (m *ChecksumManager) Get(path string) (checksum string, ok bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	checksum, ok = m.store[path]
	return checksum, ok
}

// Update stores or updates the checksum for a file path in the cache.
func (m *ChecksumManager) Update(path string, checksum string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.store[path] = checksum
}

// Clear removes all cached checksums from the manager.
func (m *ChecksumManager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.store = make(map[string]string)
}
