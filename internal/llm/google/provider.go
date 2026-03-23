package google

import (
	"context"
	"fmt"

	"github.com/Cyclone1070/iav/internal/domain"
	"google.golang.org/genai"
)

// Provider implements the domain.Provider interface for Google Gemini.
type Provider struct{}

// NewProvider creates a new Google provider factory.
func NewProvider() *Provider {
	return &Provider{}
}

func (p *Provider) ID() string {
	return domain.ProviderGoogle
}

func (p *Provider) SupportedAuthMethods() []domain.AuthMethod {
	return []domain.AuthMethod{
		domain.APIKeyAuthMethod{
			ID:   domain.AuthMethodAPIKey,
			Name: "API Key",
			Fields: []domain.AuthField{
				{
					ID:          domain.AuthFieldAPIKey,
					Label:       "API Key",
					Placeholder: "Enter your Gemini API Key",
					IsSecret:    true,
				},
			},
		},
		domain.EnvVarAuthMethod{
			ID:      domain.AuthMethodEnv,
			Name:    "Environment Variables",
			EnvVars: []string{"GEMINI_API_KEY"},
		},
	}
}

func (p *Provider) newClient(ctx context.Context, cred *domain.Credential) (*genai.Client, error) {
	if cred == nil || cred.APIKey == "" {
		return nil, fmt.Errorf("google provider requires an API key")
	}

	cfg := &genai.ClientConfig{
		APIKey: cred.APIKey,
	}

	if cred.Project != "" || cred.Location != "" {
		cfg.Backend = genai.BackendVertexAI
		cfg.Project = cred.Project
		cfg.Location = cred.Location
	}

	return genai.NewClient(ctx, cfg)
}

func (p *Provider) ListLLMs() []domain.LLMInfo {
	return []domain.LLMInfo{
		{ID: "gemini-2.5-flash-lite", DisplayName: "Gemini 2.5 Flash-Lite"},
		{ID: "gemini-2.5-flash", DisplayName: "Gemini 2.5 Flash"},
		{ID: "gemini-3-flash-preview", DisplayName: "Gemini 3.0 Flash Preview"},
		{ID: "gemini-2.5-pro", DisplayName: "Gemini 2.5 Pro"},
		{ID: "gemini-3-pro-preview", DisplayName: "Gemini 3.0 Pro Preview"},
	}
}

func (p *Provider) GetLLM(ctx context.Context, cred *domain.Credential, id string) (domain.LLM, error) {
	client, err := p.newClient(ctx, cred)
	if err != nil {
		return nil, err
	}

	// Get LLM info from API for context window
	info, err := client.Models.Get(ctx, id, nil)
	if err != nil {
		return nil, fmt.Errorf("get LLM info: %w", err)
	}

	return &geminiLLM{
		client:        client,
		id:            id,
		displayName:   info.DisplayName,
		contextWindow: int(info.InputTokenLimit),
	}, nil
}
