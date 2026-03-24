package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Cyclone1070/iav/internal/domain"
)

// Provider implements the domain.Provider interface for GitHub Copilot.
type Provider struct{}

// NewProvider creates a new GitHub provider factory.
func NewProvider() *Provider {
	return &Provider{}
}

func (p *Provider) ID() string {
	return domain.ProviderGitHub
}

func (p *Provider) SupportedAuthMethods() []domain.AuthMethod {
	return []domain.AuthMethod{
		domain.OAuthMethod{
			ID:            "github_oauth",
			Name:          "GitHub OAuth",
			ClientID:      "Iv1.b507a08c87ecfe98", // GitHub Copilot OAuth App ID
			DeviceAuthURL: "https://github.com/login/device/code",
			TokenURL:      "https://github.com/login/oauth/access_token",
			Scopes:        []string{"read:user"},
		},
	}
}

func (p *Provider) ListLLMs() []domain.LLMInfo {
	return []domain.LLMInfo{
		{ID: "claude-haiku-4.5", DisplayName: "Claude Haiku 4.5 (GitHub)"},
		{ID: "gemini-3-flash-preview", DisplayName: "Gemini 3 Flash (GitHub)"},
		{ID: "gpt-5.1", DisplayName: "GPT-5.1 (GitHub)"},
	}
}

func (p *Provider) GetLLM(ctx context.Context, cred *domain.Credential, modelID string) (domain.LLM, error) {
	tokenSource := NewTokenSource(cred.OAuthToken, "")

	// Dynamic Metadata Discovery for the requested model
	metadata, err := p.fetchMetadata(ctx, tokenSource, modelID)
	if err != nil {
		// Fallback to defaults
		return newCopilotLLM(tokenSource, modelID, "Claude Haiku 4.5", 128000), nil
	}

	return newCopilotLLM(tokenSource, modelID, metadata.Name, metadata.ContextWindow), nil
}

type modelMetadata struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ContextWindow int    `json:"context_window"`
}

func (p *Provider) fetchMetadata(ctx context.Context, ts *TokenSource, modelID string) (*modelMetadata, error) {
	token, err := ts.Get(ctx)
	if err != nil {
		return nil, err
	}

	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.githubcopilot.com/models", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Github-Api-Version", "2023-07-07")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch models: %s", resp.Status)
	}

	var payload struct {
		Data []modelMetadata `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	for _, m := range payload.Data {
		if m.ID == modelID {
			if m.ContextWindow == 0 {
				return nil, fmt.Errorf("model %s has no context window information", modelID)
			}
			return &m, nil
		}
	}

	return nil, fmt.Errorf("model %s not found in catalog", modelID)
}
