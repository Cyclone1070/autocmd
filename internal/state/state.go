package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	ConfigDir = "iav"
	StateFile = "state.json"
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

// State holds application persistent state.
type State struct {
	currentSessionID string
	model            string
	saveFn           func() error
}

// Model returns the current model.
func (s *State) Model() string {
	return s.model
}

// SetModel sets the current model.
func (s *State) SetModel(m string) {
	s.model = m
}

// CurrentSessionID returns the current session ID.
func (s *State) CurrentSessionID() string {
	return s.currentSessionID
}

// SetCurrentSessionID sets the current session ID.
func (s *State) SetCurrentSessionID(id string) {
	s.currentSessionID = id
}

// Save persists the state using the manager it was loaded from.
func (s *State) Save() error {
	if s.saveFn == nil {
		return fmt.Errorf("state not loaded via manager")
	}
	return s.saveFn()
}

// stateDTO is used for JSON persistence.
type stateDTO struct {
	CurrentSessionID string `json:"current_session_id"`
	Model            string `json:"model"`
}

// Default returns the default application state.
func Default() *State {
	return &State{
		model: "",
	}
}

// Load reads application state using the injected filesystem.
func (m *Manager) Load() (*State, error) {
	s := Default()

	homeDir, err := m.fs.UserHomeDir()
	if err != nil {
		return s, nil
	}

	statePath := filepath.Join(homeDir, ".config", ConfigDir, StateFile)

	data, err := m.fs.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}

	var dto stateDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return nil, err
	}

	s.currentSessionID = dto.CurrentSessionID
	s.model = dto.Model
	s.saveFn = func() error { return m.Save(s) }

	return s, nil
}

// Save writes application state using the injected filesystem.
func (m *Manager) Save(s *State) error {
	if s == nil {
		return fmt.Errorf("state is nil")
	}

	homeDir, err := m.fs.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	configDir := filepath.Join(homeDir, ".config", ConfigDir)
	statePath := filepath.Join(configDir, StateFile)

	if err := m.fs.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	dto := stateDTO{
		CurrentSessionID: s.currentSessionID,
		Model:            s.model,
	}

	data, err := json.MarshalIndent(dto, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	if err := m.fs.WriteFile(statePath, data, 0644); err != nil {
		return fmt.Errorf("write state file: %w", err)
	}

	return nil
}
