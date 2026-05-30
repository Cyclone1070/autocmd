package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Cyclone1070/autocmd/internal/domain"
)

const (
	stateFile = "state.json"
)

// FileSystem abstracts file operations for testability.
type FileSystem interface {
	UserHomeDir() (string, error)
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm os.FileMode) error
	MkdirAll(path string, perm os.FileMode) error
}

// Manager handles state loading and saving with injected dependencies.
type Manager struct {
	fs FileSystem
}

// NewManager creates a new Manager with the provided filesystem.
func NewManager(fs FileSystem) *Manager {
	if fs == nil {
		panic("fs is required")
	}
	return &Manager{fs: fs}
}

// Load reads application state from disk using the injected filesystem.
func (m *Manager) Load() (*domain.State, error) {
	homeDir, _ := m.fs.UserHomeDir()
	if homeDir == "" {
		return &domain.State{}, nil
	}

	statePath := filepath.Join(homeDir, domain.ConfigBaseDir, domain.AppName, stateFile)

	data, err := m.fs.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &domain.State{}, nil
		}
		return nil, err
	}

	var s domain.State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}

	return &s, nil
}

// Save writes application state to disk using the injected filesystem.
func (m *Manager) Save(s *domain.State) error {
	if s == nil {
		return fmt.Errorf("state is nil")
	}

	homeDir, err := m.fs.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	configDir := filepath.Join(homeDir, domain.ConfigBaseDir, domain.AppName)
	statePath := filepath.Join(configDir, stateFile)

	if err := m.fs.MkdirAll(configDir, domain.DefaultDirPerm); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	if err := m.fs.WriteFile(statePath, data, domain.DefaultFilePerm); err != nil {
		return fmt.Errorf("write state file: %w", err)
	}

	return nil
}
