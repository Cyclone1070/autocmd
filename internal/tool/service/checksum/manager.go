// Package checksum provides utilities for computing and managing file checksums.
package checksum

import (
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"sync"
)

// SessionState defines the interface for retrieving the current session ID.
type SessionState interface {
	CurrentSessionID() string
}

// Store defines the interface for loading and saving checksums.
type Store interface {
	LoadChecksums(sessionID string) (map[string]string, error)
	SaveChecksums(sessionID string, checksums map[string]string) error
}

// Manager handles in-memory and persistent checksum verification.
type Manager struct {
	store         map[string]string
	mu            sync.RWMutex
	checksumStore Store
	state         SessionState
	loadedSession string
}

// NewManager creates a new Checksum Manager.
func NewManager(store Store, state SessionState) *Manager {
	return &Manager{
		store:         make(map[string]string),
		checksumStore: store,
		state:         state,
	}
}

// Compute computes the SHA-256 checksum of data and returns it as a hex string.
func (m *Manager) Compute(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// Get retrieves the cached checksum for a file path.
// Returns the checksum and true if found, or empty string and false if not cached.
func (m *Manager) Get(path string) (checksum string, ok bool) {
	m.mu.Lock()
	m.ensureLoaded()
	m.mu.Unlock()

	m.mu.RLock()
	defer m.mu.RUnlock()
	checksum, ok = m.store[path]
	return checksum, ok
}

// Update stores or updates the checksum for a file path in the cache.
func (m *Manager) Update(path string, checksum string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureLoaded()
	m.store[path] = checksum
	m.save()
}

func (m *Manager) ensureLoaded() {
	if m.state == nil || m.checksumStore == nil {
		return
	}
	sessionID := m.state.CurrentSessionID()
	if sessionID == "" {
		return
	}
	if m.loadedSession == sessionID {
		return
	}
	m.loadedSession = sessionID
	m.store = make(map[string]string)
	loaded, err := m.checksumStore.LoadChecksums(sessionID)
	if err == nil {
		maps.Copy(m.store, loaded)
	}
}

func (m *Manager) save() {
	if m.state == nil || m.checksumStore == nil {
		return
	}
	sessionID := m.state.CurrentSessionID()
	if sessionID == "" {
		return
	}
	_ = m.checksumStore.SaveChecksums(sessionID, m.store)
}
