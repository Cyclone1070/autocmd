// Package config handles application configuration loading and defaults.
package config

import (
	"maps"

	"github.com/Cyclone1070/iav/internal/domain"
)

const (
	defaultChatWindowWidth  = 80
	defaultBashOutputHeight = 12
	defaultThinkingHeight   = 5
	defaultMaxFileSize      = 20 * 1024 * 1024
	defaultMaxIterations    = 50

	permissionModeAsk   = "ask"
	permissionModeAllow = "allow"
	permissionModeDeny  = "deny"

	contextWindow128k = 128000
	contextWindow256k = 256000
	contextWindow2M   = 2000000
)

// Config holds all application configuration values.
type Config struct {
	tools     ToolsConfig
	providers ProviderConfig
	ui        UIConfig
}

// Tools returns the configuration for tools.
func (c *Config) Tools() ToolsConfig { return c.tools }


// UI returns the configuration for the terminal UI.
func (c *Config) UI() UIConfig { return c.ui }

// Providers returns the configuration for AI providers.
func (c *Config) Providers() ProviderConfig { return c.providers }


// ToolsConfig holds tool-specific configuration.
type ToolsConfig struct {
	toolPermissions   map[string]string
	permissionDefault string
	maxFileSize       int64
	maxIterations     int
}

// MaxFileSize returns the maximum file size allowed for tool operations.
func (c ToolsConfig) MaxFileSize() int64 { return c.maxFileSize }

// MaxIterations returns the maximum number of tool iterations per request.
func (c ToolsConfig) MaxIterations() int { return c.maxIterations }

// PermissionDefault returns the default permission level for tools.
func (c ToolsConfig) PermissionDefault() string { return c.permissionDefault }

// ToolPermissions returns a copy of the tool-specific permission levels.
func (c ToolsConfig) ToolPermissions() map[string]string {
	out := make(map[string]string, len(c.toolPermissions))
	maps.Copy(out, c.toolPermissions)
	return out
}

// ModelConfig represents an AI model configuration.
type ModelConfig struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ContextWindow int    `json:"context_window"`
}

// ProviderConfig maps provider names to their available models.
type ProviderConfig map[string][]ModelConfig

// DTOs for JSON persistence.

type toolsDTO struct {
	Permissions   permissionsDTO `json:"permissions"`
	MaxFileSize   int64          `json:"max_file_size"`
	MaxIterations int            `json:"max_iterations"`
}

type permissionsDTO struct {
	ByTool  map[string]string `json:"by_tool"`
	Default string            `json:"default"`
}

type configDTO struct {
	Tools     toolsDTO       `json:"tools"`
	Providers ProviderConfig `json:"providers,omitempty"`
	UI        uiDTO          `json:"ui"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		tools: ToolsConfig{
			maxFileSize:       defaultMaxFileSize,
			maxIterations:     defaultMaxIterations,
			permissionDefault: permissionModeAllow,
			toolPermissions: map[string]string{
				"edit_file":  permissionModeAsk,
				"write_file": permissionModeAsk,
				"bash":       permissionModeAsk,
			},
		},
		ui: UIConfig{
			primaryColor:     ColorConfig{light: "#0EA5E9", dark: "#38BDF8"},
			successColor:     ColorConfig{light: "#43BF6D", dark: "#73F59F"},
			errorColor:       ColorConfig{light: "#F05D5E", dark: "#FF6666"},
			mutedColor:       ColorConfig{light: "#D9DCCF", dark: "#888888"},
			chatWindowWidth:  defaultChatWindowWidth,
			bashOutputHeight: defaultBashOutputHeight,
			thinkingHeight:   defaultThinkingHeight,
			shortToolBlock:   false,
		},
		providers: ProviderConfig{
			domain.ProviderGoogle: {
				{ID: "google/gemma-4-31b-it", Name: "Gemma 4", ContextWindow: contextWindow256k},
				{ID: "google/gemma-4-26b-a4b-it", Name: "Gemma 4 MoE", ContextWindow: contextWindow256k},
				{ID: "google/gemini-3.1-flash-lite", Name: "Gemini 3.0 Flash Lite", ContextWindow: contextWindow2M},
				{ID: "google/gemini-3-flash-preview", Name: "Gemini 3.0 Flash", ContextWindow: contextWindow2M},
				{ID: "google/gemini-3-pro-preview", Name: "Gemini 3.0 Pro", ContextWindow: contextWindow2M},
			},
			domain.ProviderGitHub: {
				{ID: "github/claude-haiku-4.5", Name: "Claude Haiku 4.5", ContextWindow: contextWindow128k},
				{ID: "github/gemini-3-flash-preview", Name: "Gemini 3 Flash", ContextWindow: contextWindow2M},
				{ID: "github/gpt-5.1", Name: "GPT 5.1", ContextWindow: contextWindow128k},
			},
		},
	}
}
