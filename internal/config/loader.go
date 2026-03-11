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

// Manager handles configuration loading with injected dependencies
type Manager struct {
	fs FileSystem
}

// NewManager creates a Manager with the provided filesystem
func NewManager(fs FileSystem) *Manager {
	if fs == nil {
		panic("fs is required")
	}
	return &Manager{fs: fs}
}

// newConfig creates a Config from a DTO and validates it.
func newConfig(dto configDTO) (*Config, error) {
	cfg := &Config{
		tools: ToolsConfig{
			maxFileSize:         dto.Tools.MaxFileSize,
			defaultShellTimeout: dto.Tools.DefaultShellTimeout,
			maxIterations:       dto.Tools.MaxIterations,
		},
		session: SessionConfig{
			storageDir: dto.Session.StorageDir,
		},
		ui: UIConfig{
			primaryColor: ColorConfig{
				light: dto.UI.PrimaryColor.Light,
				dark:  dto.UI.PrimaryColor.Dark,
			},
			successColor: ColorConfig{
				light: dto.UI.SuccessColor.Light,
				dark:  dto.UI.SuccessColor.Dark,
			},
			errorColor: ColorConfig{
				light: dto.UI.ErrorColor.Light,
				dark:  dto.UI.ErrorColor.Dark,
			},
			mutedColor: ColorConfig{
				light: dto.UI.MutedColor.Light,
				dark:  dto.UI.MutedColor.Dark,
			},
			chatWindowWidth:   dto.UI.ChatWindowWidth,
			shellOutputHeight: dto.UI.ShellOutputHeight,
			shortToolbox:      dto.UI.ShortToolbox,
		},
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

// Load reads configuration and merges it with defaults.
func (m *Manager) Load() (*Config, error) {
	// Start with default DTO values
	defaults := DefaultConfig()
	dto := configDTO{
		Tools: toolsDTO{
			MaxFileSize:         defaults.tools.maxFileSize,
			DefaultShellTimeout: defaults.tools.defaultShellTimeout,
			MaxIterations:       defaults.tools.maxIterations,
		},
		Session: sessionDTO{
			StorageDir: defaults.session.storageDir,
		},
		UI: uiDTO{
			PrimaryColor:      colorDTO{Light: defaults.ui.primaryColor.light, Dark: defaults.ui.primaryColor.dark},
			SuccessColor:      colorDTO{Light: defaults.ui.successColor.light, Dark: defaults.ui.successColor.dark},
			ErrorColor:        colorDTO{Light: defaults.ui.errorColor.light, Dark: defaults.ui.errorColor.dark},
			MutedColor:        colorDTO{Light: defaults.ui.mutedColor.light, Dark: defaults.ui.mutedColor.dark},
			ChatWindowWidth:   defaults.ui.chatWindowWidth,
			ShellOutputHeight: defaults.ui.shellOutputHeight,
			ShortToolbox:      defaults.ui.shortToolbox,
		},
	}

	homeDir, err := m.fs.UserHomeDir()
	if err == nil {
		configPath := filepath.Join(homeDir, ".config", ConfigDir, ConfigFile)
		data, err := m.fs.ReadFile(configPath)
		if err == nil {
			if err := json.Unmarshal(data, &dto); err != nil {
				return nil, fmt.Errorf("failed to parse config file: %w", err)
			}
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}

	return newConfig(dto)
}
