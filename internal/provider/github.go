// Package provider implements various LLM providers (Google, GitHub) for the application.
package provider

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
)

// GitHubProvider implements the domain.Provider interface for GitHub Copilot.
type GitHubProvider struct {
	models []domain.LLMInfo
}

// NewGitHubProvider creates a new GitHub provider factory.
func NewGitHubProvider(models []domain.LLMInfo) *GitHubProvider {
	return &GitHubProvider{models: models}
}

// ID returns the unique identifier for the GitHub provider.
func (p *GitHubProvider) ID() string {
	return domain.ProviderGitHub
}

// SupportedAuthMethods returns the list of authentication methods supported by GitHub.
func (p *GitHubProvider) SupportedAuthMethods() []domain.AuthMethod {
	return []domain.AuthMethod{
		domain.OAuthMethod{
			ID:            authMethodGitHubOAuth,
			Name:          "GitHub OAuth",
			ClientID:      "Iv1.b507a08c87ecfe98", // #nosec G101 - Public GitHub Copilot OAuth App ID
			DeviceAuthURL: "https://github.com/login/device/code",
			TokenURL:      "https://github.com/login/oauth/access_token",
			Scopes:        []string{"read:user"},
		},
	}
}

// List returns the available models provided by GitHub.
func (p *GitHubProvider) List() []domain.LLMInfo {
	return p.models
}

// GetLLM initializes and returns a GitHub-backed LLM instance.
func (p *GitHubProvider) GetLLM(ctx context.Context, cred *domain.Credential, info domain.LLMInfo) (domain.LLM, error) {
	start := time.Now()
	slog.Info("provider github get llm start", "model_id", info.ID)
	tokenSource := NewTokenSource(cred.OAuthToken, "")

	httpClient := &http.Client{
		Transport: &tokenTransport{source: tokenSource},
	}
	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		Model:      strings.TrimPrefix(info.ID, domain.ProviderGitHub+modelIDSeparator),
		BaseURL:    "https://api.githubcopilot.com/",
		HTTPClient: httpClient,
	})
	if err != nil {
		slog.Error("provider github model init failed", "model_id", info.ID, "duration_ms", time.Since(start).Milliseconds(), "error", err)
		return nil, fmt.Errorf("create eino openai model: %w", err)
	}
	slog.Info("provider github get llm success", "model_id", info.ID, "duration_ms", time.Since(start).Milliseconds())

	return &copilotLLM{
		tokenSource:   tokenSource,
		model:         chatModel,
		id:            info.ID,
		displayName:   info.DisplayName,
		contextWindow: info.ContextWindow,
	}, nil
}

type tokenTransport struct {
	source *TokenSource
}

func (t *tokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	token, err := t.source.Get(req.Context())
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Github-Api-Version", "2023-07-07")
	req.Header.Set("User-Agent", "GithubCopilot/1.155.0")
	req.Header.Set("Editor-Version", "vscode/1.85.1")
	req.Header.Set("Editor-Plugin-Version", "copilot/1.155.0")
	return http.DefaultTransport.RoundTrip(req)
}

type copilotLLM struct {
	tokenSource   *TokenSource
	model         model.ToolCallingChatModel
	id            string
	displayName   string
	contextWindow int
}

func (l *copilotLLM) ID() string {
	return l.id
}

func (l *copilotLLM) DisplayName() string {
	return l.displayName
}

func (l *copilotLLM) ContextWindow() int {
	return l.contextWindow
}

func (l *copilotLLM) Model() model.ToolCallingChatModel {
	return l.model
}
