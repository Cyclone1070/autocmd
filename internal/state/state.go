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

// FileSystem abstracts file operations for testability
type FileSystem interface {
	UserHomeDir() (string, error)
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm os.FileMode) error
	MkdirAll(path string, perm os.FileMode) error
}

// OSFileSystem implements FileSystem using the real OS
type OSFileSystem struct{}

func (OSFileSystem) UserHomeDir() (string, error) { return os.UserHomeDir() }
func (OSFileSystem) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }
func (OSFileSystem) WriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}
func (OSFileSystem) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

// State holds application persistent state that is managed by the app.
type State struct {
	CurrentSessionID string `json:"current_session_id"`
	Model            string `json:"model"`
}

// Loader handles state loading with injected dependencies
type Loader struct {
	fs FileSystem
}

func NewLoader() *Loader {
	return &Loader{fs: OSFileSystem{}}
}

func NewLoaderWithFS(fs FileSystem) *Loader {
	return &Loader{fs: fs}
}

// Default returns the default application state
func Default() *State {
	return &State{
		Model: "google/gemini-2.5-flash",
	}
}

// Validate checks state values for correctness.
func (s *State) Validate() error {
	if s.Model == "" {
		return fmt.Errorf("state validation failed: model must not be empty")
	}
	return nil
}

// Load reads application state from ~/.config/iav/state.json
func (l *Loader) Load() (*State, error) {
	s := Default()

	homeDir, err := l.fs.UserHomeDir()
	if err != nil {
		return s, nil
	}

	statePath := filepath.Join(homeDir, ".config", ConfigDir, StateFile)

	data, err := l.fs.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, s); err != nil {
		return nil, err
	}

	if err := s.Validate(); err != nil {
		return nil, err
	}

	return s, nil
}

// Save writes application state to ~/.config/iav/state.json
func (l *Loader) Save(s *State) error {
	if s == nil {
		return fmt.Errorf("state is nil")
	}

	homeDir, err := l.fs.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	configDir := filepath.Join(homeDir, ".config", ConfigDir)
	statePath := filepath.Join(configDir, StateFile)

	if err := l.fs.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	if err := l.fs.WriteFile(statePath, data, 0644); err != nil {
		return fmt.Errorf("write state file: %w", err)
	}

	return nil
}

// Convenience functions using the default loader
func Load() (*State, error) { return NewLoader().Load() }
func Save(s *State) error   { return NewLoader().Save(s) }
