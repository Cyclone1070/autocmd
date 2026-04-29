// Package auth provides functionality for managing authentication credentials.
package auth

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sync"

	"github.com/Cyclone1070/iav/internal/domain"
)

// FileSystem abstracts filesystem operations for the auth package.
type FileSystem interface {
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, perm os.FileMode) error
	MkdirAll(path string, perm os.FileMode) error
}

// Manager handles persistent storage of authentication credentials.
type Manager struct {
	fs        FileSystem
	cache     map[string]domain.Credential
	storePath string
	mu        sync.RWMutex
}

// NewManager creates a new Manager with the given filesystem and storage path.
func NewManager(fs FileSystem, storePath string) *Manager {
	return &Manager{
		fs:        fs,
		storePath: storePath,
	}
}

// DefaultStorePath returns the standard ~/.config/iav/auth.json path.
func DefaultStorePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, domain.ConfigBaseDir, domain.AppName, "auth.json"), nil
}

// Get returns the credential for the given provider.
func (m *Manager) Get(providerID string) (*domain.Credential, error) {
	all, err := m.All()
	if err != nil {
		return nil, err
	}
	cred, ok := all[providerID]
	if !ok {
		return nil, nil
	}
	return &cred, nil
}

// GetWithFallback returns the credential for the given provider,
// prioritizing stored credentials over environment variable fallbacks.
func (m *Manager) GetWithFallback(p domain.Provider) (*domain.Credential, error) {
	// 1. Try stored credentials first
	cred, err := m.Get(p.ID())
	if err != nil {
		return nil, err
	}
	if cred != nil && (cred.APIKey != "" || cred.OAuthToken != "") {
		return cred, nil
	}

	// 2. Try environment variable fallbacks from metadata
	fallback := &domain.Credential{Type: domain.AuthMethodEnv}
	found := false

	for _, method := range p.SupportedAuthMethods() {
		switch m := method.(type) {
		case domain.APIKeyAuthMethod:
			for _, field := range m.Fields {
				if field.EnvVar == "" {
					continue
				}
				val := os.Getenv(field.EnvVar)
				if val == "" {
					continue
				}

				found = true
				if field.ID == domain.AuthFieldAPIKey {
					fallback.APIKey = val
				}
			}
		case domain.EnvVarAuthMethod:
			for _, envVar := range m.EnvVars {
				val := os.Getenv(envVar)
				if val == "" {
					continue
				}
				found = true
				fallback.APIKey = val
				break
			}
		}
	}

	if found {
		return fallback, nil
	}

	return nil, nil
}

// Set stores the credential for the given provider.
func (m *Manager) Set(providerID string, cred domain.Credential) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	all, err := m.loadAll()
	if err != nil {
		all = make(map[string]domain.Credential)
	}

	all[providerID] = cred

	if err := m.saveAll(all); err != nil {
		return err
	}

	m.cache = all
	return nil
}

// All returns all stored credentials.
func (m *Manager) All() (map[string]domain.Credential, error) {
	m.mu.RLock()
	if m.cache != nil {
		res := make(map[string]domain.Credential, len(m.cache))
		maps.Copy(res, m.cache)
		m.mu.RUnlock()
		return res, nil
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cache != nil {
		res := make(map[string]domain.Credential, len(m.cache))
		maps.Copy(res, m.cache)
		return res, nil
	}

	all, err := m.loadAll()
	if err != nil {
		return nil, err
	}
	m.cache = all

	res := make(map[string]domain.Credential, len(all))
	maps.Copy(res, all)
	return res, nil
}

// Remove deletes the credential for the given provider.
func (m *Manager) Remove(providerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	all, err := m.loadAll()
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if _, ok := all[providerID]; !ok {
		return nil
	}

	delete(all, providerID)
	if err := m.saveAll(all); err != nil {
		return err
	}

	m.cache = all
	return nil
}

func (m *Manager) loadAll() (map[string]domain.Credential, error) {
	data, err := m.fs.ReadFile(m.storePath)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]domain.Credential), nil
		}
		return nil, fmt.Errorf("read auth file: %w", err)
	}

	var all map[string]domain.Credential
	if err := json.Unmarshal(data, &all); err != nil {
		return nil, fmt.Errorf("unmarshal auth file: %w", err)
	}

	return all, nil
}

func (m *Manager) saveAll(all map[string]domain.Credential) error {
	if err := m.fs.MkdirAll(filepath.Dir(m.storePath), domain.PrivateDirPerm); err != nil {
		return fmt.Errorf("create auth dir: %w", err)
	}

	// #nosec G117
	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal auth: %w", err)
	}

	if err := m.fs.WriteFile(m.storePath, data, domain.PrivateFilePerm); err != nil {
		return fmt.Errorf("write auth file: %w", err)
	}

	return nil
}
