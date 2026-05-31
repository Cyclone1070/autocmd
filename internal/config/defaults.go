// Package config handles application configuration loading and defaults.
package config

import (
	"github.com/Cyclone1070/autocmd/internal/domain"
)

const (
	defaultChatWindowWidth  = 0
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
			textColor:        ColorConfig{light: "#1C1C1C", dark: "#D0D0D0"},
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
