package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const (
	// ConfigDir is the directory name under ~/.config
	ConfigDir = "iav"
	// ConfigFile is the config file name
	ConfigFile = "config.json"
)

// FileSystem abstracts file operations for testability
type FileSystem interface {
	UserHomeDir() (string, error)
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm os.FileMode) error
	MkdirAll(path string, perm os.FileMode) error
}

// ConfigFileReader implements FileSystem using the real OS for config loading
type ConfigFileReader struct{}

func (ConfigFileReader) UserHomeDir() (string, error) {
	return os.UserHomeDir()
}

func (ConfigFileReader) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (ConfigFileReader) WriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}

func (ConfigFileReader) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

// Loader handles configuration loading with injected dependencies
type Loader struct {
	fs FileSystem
}

// NewLoader creates a production Loader using the real filesystem
func NewLoader() *Loader {
	return &Loader{fs: ConfigFileReader{}}
}

// NewLoaderWithFS creates a Loader with a custom filesystem (for testing)
func NewLoaderWithFS(fs FileSystem) *Loader {
	return &Loader{fs: fs}
}

// Load reads configuration and merges it with defaults.
// Returns error only for parse errors, permission issues, or validation failures.
func (l *Loader) Load() (*Config, error) {
	cfg := DefaultConfig()

	// Load User Config
	homeDir, err := l.fs.UserHomeDir()
	if err != nil {
		return cfg, nil // Use defaults if can't get home dir
	}

	configPath := filepath.Join(homeDir, ".config", ConfigDir, ConfigFile)
	data, err := l.fs.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, cfg.Validate()
		}
		return nil, err // Return error for permission issues
	}

	// Parse JSON directly into the config struct.
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	// Validate the final merged configuration
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Load is a convenience function using the default loader
func Load() (*Config, error) {
	return NewLoader().Load()
}
