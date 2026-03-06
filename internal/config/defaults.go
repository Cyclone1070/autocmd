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
	Model   string        `json:"model"` // e.g., "google/gemini-2.5-flash"
}

// State holds application persistent state that is managed by the app.
type State struct {
	CurrentSessionID string `json:"current_session_id" github:"current_session_id"`
}

type SessionConfig struct {
	StorageDir string `json:"storage_dir"` // Default: ~/.iav/sessions
}

type ToolsConfig struct {
	// File Operations
	MaxFileSize          int64 `json:"max_file_size"`           // Default: 20 * 1024 * 1024 (20MB)
	DefaultReadFileLimit int   `json:"default_read_file_limit"` // Default: 2000

	// Directory Listing
	DefaultListDirectoryLimit int `json:"default_list_directory_limit"` // Default: 1000
	MaxListDirectoryLimit     int `json:"max_list_directory_limit"`     // Default: 10000
	MaxListDirectoryResults   int `json:"max_list_directory_results"`   // Default: 50000

	// Command Execution
	DefaultMaxCommandOutputSize int64 `json:"default_max_command_output_size"` // Default: 10 * 1024 * 1024 (10MB)
	DefaultShellTimeout         int   `json:"default_shell_timeout"`           // Default: 600 (10 minutes, in seconds)

	// Search
	MaxSearchContentResults   int `json:"max_search_content_results"`   // Default: 10000
	MaxLineLength             int `json:"max_line_length"`              // Default: 10000
	DefaultSearchContentLimit int `json:"default_search_content_limit"` // Default: 100
	MaxSearchContentLimit     int `json:"max_search_content_limit"`     // Default: 1000
	MaxFindFileResults        int `json:"max_find_file_results"`        // Default: 10000
	DefaultFindFileLimit      int `json:"default_find_file_limit"`      // Default: 100
	MaxFindFileLimit          int `json:"max_find_file_limit"`          // Default: 1000

	// Docker
	DockerRetryAttempts      int `json:"docker_retry_attempts"`       // Default: 10
	DockerRetryIntervalMs    int `json:"docker_retry_interval_ms"`    // Default: 1000
	DockerGracefulShutdownMs int `json:"docker_graceful_shutdown_ms"` // Default: 2000

	// Agent
	MaxIterations int `json:"max_iterations"` // Default: 20
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		Tools: ToolsConfig{
			MaxFileSize:                 20 * 1024 * 1024,
			DefaultReadFileLimit:        2000,
			DefaultListDirectoryLimit:   1000,
			MaxListDirectoryLimit:       10000,
			MaxListDirectoryResults:     50000,
			DefaultMaxCommandOutputSize: 10 * 1024 * 1024,
			DefaultShellTimeout:         600,
			MaxSearchContentResults:     10000,
			MaxLineLength:               10000,
			DefaultSearchContentLimit:   100,
			MaxSearchContentLimit:       1000,
			MaxFindFileResults:          10000,
			DefaultFindFileLimit:        100,
			MaxFindFileLimit:            1000,
			DockerRetryAttempts:         10,
			DockerRetryIntervalMs:       1000,
			DockerGracefulShutdownMs:    2000,
			MaxIterations:               20,
		},
		Session: SessionConfig{
			StorageDir: filepath.Join(os.Getenv("HOME"), ".iav", "sessions"),
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
		Model: "google/gemini-2.5-flash",
	}
}
