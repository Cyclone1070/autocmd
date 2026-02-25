package config

import (
	"encoding/json"
	"fmt"
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

// Load reads configuration from ~/.config/iav/config.json
// and merges it with defaults. Dotfile values override defaults.
// Returns default config if dotfile doesn't exist.
// Returns error only for parse errors, permission issues, or validation failures.
//
// NOTE: This implementation unmarshals JSON keys directly over the default configuration.
// This allows explicit zero values (e.g., 0, false, "") in the config file to override defaults.
func (l *Loader) Load() (*Config, error) {
	cfg := DefaultConfig()

	homeDir, err := l.fs.UserHomeDir()
	if err != nil {
		return cfg, nil // Use defaults if can't get home dir
	}

	configPath := filepath.Join(homeDir, ".config", ConfigDir, ConfigFile)

	data, err := l.fs.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil // Use defaults if file doesn't exist
		}
		return nil, err // Return error for permission issues
	}

	// Parse JSON directly into the default config struct.
	// This ensures that present keys overwrite defaults (even if zero),
	// while missing keys leave the defaults untouched.
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err // Return error for malformed JSON
	}

	// Validate the merged configuration
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Load is a convenience function using the default loader
func Load() (*Config, error) {
	return NewLoader().Load()
}

// Save writes configuration to ~/.config/iav/config.json.
func (l *Loader) Save(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}

	homeDir, err := l.fs.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	configDir := filepath.Join(homeDir, ".config", ConfigDir)
	configPath := filepath.Join(configDir, ConfigFile)

	// Ensure directory exists
	if err := l.fs.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := l.fs.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	return nil
}

// Save is a convenience function using the default loader
func Save(cfg *Config) error {
	return NewLoader().Save(cfg)
}
