package config

import (
	"os"
	"path/filepath"
)

// Config holds all application configuration values.
type Config struct {
	tools   ToolsConfig
	session SessionConfig
	ui      UIConfig
}

func (c *Config) Tools() ToolsConfig     { return c.tools }
func (c *Config) Session() SessionConfig { return c.session }
func (c *Config) UI() UIConfig           { return c.ui }

type SessionConfig struct {
	storageDir string
}

func (c SessionConfig) StorageDir() string { return c.storageDir }

type ToolsConfig struct {
	maxFileSize         int64
	defaultShellTimeout int
	maxIterations       int
}

func (c ToolsConfig) MaxFileSize() int64         { return c.maxFileSize }
func (c ToolsConfig) DefaultShellTimeout() int   { return c.defaultShellTimeout }
func (c ToolsConfig) MaxIterations() int         { return c.maxIterations }

// DTOs for JSON persistence
type sessionDTO struct {
	StorageDir string `json:"storage_dir"`
}

type toolsDTO struct {
	MaxFileSize         int64 `json:"max_file_size"`
	DefaultShellTimeout int   `json:"default_shell_timeout"`
	MaxIterations       int   `json:"max_iterations"`
}

type configDTO struct {
	Tools   toolsDTO   `json:"tools"`
	Session sessionDTO `json:"session"`
	UI      uiDTO      `json:"ui"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		tools: ToolsConfig{
			maxFileSize:         20 * 1024 * 1024,
			defaultShellTimeout: 600,
			maxIterations:       20,
		},
		session: SessionConfig{
			storageDir: filepath.Join(os.Getenv("HOME"), ".config", "iav", "sessions"),
		},
		ui: UIConfig{
			primaryColor:      ColorConfig{light: "#0EA5E9", dark: "#38BDF8"},
			successColor:      ColorConfig{light: "#43BF6D", dark: "#73F59F"},
			errorColor:        ColorConfig{light: "#F05D5E", dark: "#FF6666"},
			mutedColor:        ColorConfig{light: "#D9DCCF", dark: "#888888"},
			chatWindowWidth:   80,
			shellOutputHeight: 12,
			shortToolbox:      false,
		},
	}
}
