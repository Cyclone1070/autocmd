package config

import (
	"os"
	"path/filepath"
)

// Config holds all application configuration values.
type Config struct {
	tools     ToolsConfig
	session   SessionConfig
	ui        UIConfig
	providers ProviderConfig
}

func (c *Config) Tools() ToolsConfig        { return c.tools }
func (c *Config) Session() SessionConfig    { return c.session }
func (c *Config) UI() UIConfig              { return c.ui }
func (c *Config) Providers() ProviderConfig { return c.providers }

type SessionConfig struct {
	storageDir string
}

func (c SessionConfig) StorageDir() string { return c.storageDir }

type ToolsConfig struct {
	maxFileSize   int64
	maxIterations int
}

func (c ToolsConfig) MaxFileSize() int64 { return c.maxFileSize }
func (c ToolsConfig) MaxIterations() int { return c.maxIterations }

type ModelConfig struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ContextWindow int    `json:"context_window"`
}

type ProviderConfig map[string][]ModelConfig

// DTOs for JSON persistence
type sessionDTO struct {
	StorageDir string `json:"storage_dir"`
}

type toolsDTO struct {
	MaxFileSize   int64 `json:"max_file_size"`
	MaxIterations int   `json:"max_iterations"`
}

type configDTO struct {
	Tools     toolsDTO       `json:"tools"`
	Session   sessionDTO     `json:"session"`
	UI        uiDTO          `json:"ui"`
	Providers ProviderConfig `json:"providers,omitempty"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		tools: ToolsConfig{
			maxFileSize:   20 * 1024 * 1024,
			maxIterations: 20,
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
		providers: ProviderConfig{
			"google": {
				{ID: "google/gemini-2.5-flash-lite", Name: "Gemini 2.5 Flash Lite", ContextWindow: 1048576},
				{ID: "google/gemini-2.5-flash", Name: "Gemini 2.5 Flash", ContextWindow: 1048576},
				{ID: "google/gemini-3-flash-preview", Name: "Gemini 3.0 Flash", ContextWindow: 2097152},
				{ID: "google/gemini-2.5-pro", Name: "Gemini 2.5 Pro", ContextWindow: 2097152},
				{ID: "google/gemini-3-pro-preview", Name: "Gemini 3.0 Pro", ContextWindow: 2097152},
			},
			"github": {
				{ID: "github/claude-haiku-4.5", Name: "Claude Haiku 4.5", ContextWindow: 128000},
				{ID: "github/gemini-3-flash-preview", Name: "Gemini 3 Flash", ContextWindow: 2097152},
				{ID: "github/gpt-5.1", Name: "GPT 5.1", ContextWindow: 128000},
			},
		},
	}
}
