package config

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"

	"github.com/Cyclone1070/iav/internal/domain"
)

const (
	configFile = "config.json"
)

// FileSystem abstracts file operations for testability.
type FileSystem interface {
	UserHomeDir() (string, error)
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm os.FileMode) error
	MkdirAll(path string, perm os.FileMode) error
}

// FileReader implements FileSystem using the real OS for config loading.
type FileReader struct{}

// UserHomeDir returns the current user's home directory.
func (FileReader) UserHomeDir() (string, error) {
	return os.UserHomeDir()
}

// ReadFile reads the file named by path and returns the contents.
func (FileReader) ReadFile(path string) ([]byte, error) {
	// #nosec G304 - Intentional file loading from path
	return os.ReadFile(path)
}

// WriteFile writes data to a file named by path.
func (FileReader) WriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}

// MkdirAll creates a directory named path, along with any necessary parents.
func (FileReader) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

// Manager handles configuration loading with injected dependencies.
type Manager struct {
	fs FileSystem
}

func copyStringMap(in map[string]string) map[string]string {
	if in == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	maps.Copy(out, in)
	return out
}

// NewManager creates a Manager with the provided filesystem.
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
			maxFileSize:       dto.Tools.MaxFileSize,
			maxIterations:     dto.Tools.MaxIterations,
			permissionDefault: dto.Tools.Permissions.Default,
			toolPermissions:   copyStringMap(dto.Tools.Permissions.ByTool),
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
			chatWindowWidth:  dto.UI.ChatWindowWidth,
			bashOutputHeight: dto.UI.BashOutputHeight,
			thinkingHeight:   dto.UI.ThinkingHeight,
			shortToolBlock:   dto.UI.ShortToolBlock,
		},
		providers: dto.Providers,
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
			MaxFileSize:   defaults.tools.maxFileSize,
			MaxIterations: defaults.tools.maxIterations,
			Permissions: permissionsDTO{
				Default: defaults.tools.permissionDefault,
				ByTool:  copyStringMap(defaults.tools.toolPermissions),
			},
		},
		UI: uiDTO{
			PrimaryColor:     colorDTO{Light: defaults.ui.primaryColor.light, Dark: defaults.ui.primaryColor.dark},
			SuccessColor:     colorDTO{Light: defaults.ui.successColor.light, Dark: defaults.ui.successColor.dark},
			ErrorColor:       colorDTO{Light: defaults.ui.errorColor.light, Dark: defaults.ui.errorColor.dark},
			MutedColor:       colorDTO{Light: defaults.ui.mutedColor.light, Dark: defaults.ui.mutedColor.dark},
			ChatWindowWidth:  defaults.ui.chatWindowWidth,
			BashOutputHeight: defaults.ui.bashOutputHeight,
			ThinkingHeight:   defaults.ui.thinkingHeight,
			ShortToolBlock:   defaults.ui.shortToolBlock,
		},
		Providers: defaults.providers,
	}

	homeDir, err := m.fs.UserHomeDir()
	if err == nil {
		configPath := filepath.Join(homeDir, domain.ConfigBaseDir, domain.AppName, configFile)
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
