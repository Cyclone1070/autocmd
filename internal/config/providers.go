package config

// ModelConfig represents an AI model configuration.
type ModelConfig struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ContextWindow int    `json:"context_window"`
}

// ProviderConfig maps provider names to their available models.
type ProviderConfig map[string][]ModelConfig
