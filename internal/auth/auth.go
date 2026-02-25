package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Cyclone1070/iav/internal/domain"
)

var (
	// storePath is a var so it can be overridden in tests
	storePath = func() string {
		return filepath.Join(os.Getenv("HOME"), ".iav", "auth.json")
	}
	mu sync.RWMutex
)

// Get returns the credential for the given provider.
func Get(providerID string) (*domain.Credential, error) {
	all, err := All()
	if err != nil {
		return nil, err
	}
	cred, ok := all[providerID]
	if !ok {
		return nil, nil
	}
	return &cred, nil
}

// Set stores the credential for the given provider.
func Set(providerID string, cred domain.Credential) error {
	mu.Lock()
	defer mu.Unlock()

	all, err := loadAll()
	if err != nil {
		all = make(map[string]domain.Credential)
	}

	all[providerID] = cred

	return saveAll(all)
}

// All returns all stored credentials.
func All() (map[string]domain.Credential, error) {
	mu.RLock()
	defer mu.RUnlock()
	return loadAll()
}

// Remove deletes the credential for the given provider.
func Remove(providerID string) error {
	mu.Lock()
	defer mu.Unlock()

	all, err := loadAll()
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
	return saveAll(all)
}

func loadAll() (map[string]domain.Credential, error) {
	path := storePath()
	data, err := os.ReadFile(path)
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

func saveAll(all map[string]domain.Credential) error {
	path := storePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create auth dir: %w", err)
	}

	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal auth: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write auth file: %w", err)
	}

	return nil
}
