package config

import (
	"os"
	"path/filepath"
)

// Config holds all application configuration values.
// Defaults are set in DefaultConfig() and can be overridden via dotfile.
// NOTE: Values in config files override defaults, including explicit zero values.
// Missing keys are left at their default values.
type Config struct {
	Tools   ToolsConfig   `json:"tools"`
	Session SessionConfig `json:"session"`
	UI      UIConfig      `json:"ui"`
}


type SessionConfig struct {
	StorageDir string `json:"storage_dir"` // Default: ~/.config/iav/sessions
}

type ToolsConfig struct {
	// File Operations
	MaxFileSize int64 `json:"max_file_size"` // Default: 20 * 1024 * 1024 (20MB)

	// Command Execution
	DefaultShellTimeout int `json:"default_shell_timeout"` // Default: 600 (10 minutes, in seconds)

	// Agent
	MaxIterations int `json:"max_iterations"` // Default: 20
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		Tools: ToolsConfig{
			MaxFileSize:         20 * 1024 * 1024,
			DefaultShellTimeout: 600,
			MaxIterations:       20,
		},
		Session: SessionConfig{
			StorageDir: filepath.Join(os.Getenv("HOME"), ".config", "iav", "sessions"),
		},
		UI: UIConfig{
			PrimaryColor:      ColorConfig{Light: "#0EA5E9", Dark: "#38BDF8"}, // Sky Blue (Tailwind 500/400)
			SuccessColor:      ColorConfig{Light: "#43BF6D", Dark: "#73F59F"},
			ErrorColor:        ColorConfig{Light: "#F05D5E", Dark: "#FF6666"},
			MutedColor:        ColorConfig{Light: "#D9DCCF", Dark: "#888888"},
			ChatWindowWidth:   80,
			ShellOutputHeight: 12,
			ShortToolbox:      false,
		},
	}
}
