package provider

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Cyclone1070/autocmd/internal/domain"
	"github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/cloudwego/eino/components/model"
	"google.golang.org/genai"
)

// GoogleProvider implements the domain.Provider interface for Google Gemini.
type GoogleProvider struct {
	models []domain.LLMInfo
}

// NewGoogleProvider creates a new Google provider factory.
func NewGoogleProvider(models []domain.LLMInfo) *GoogleProvider {
	return &GoogleProvider{models: models}
}

// ID returns the unique identifier for the Google provider.
func (p *GoogleProvider) ID() string {
	return domain.ProviderGoogle
}

// SupportedAuthMethods returns the list of authentication methods supported by Google.
func (p *GoogleProvider) SupportedAuthMethods() []domain.AuthMethod {
	return []domain.AuthMethod{
		domain.APIKeyAuthMethod{
			ID:   domain.AuthMethodAPIKey,
			Name: authMethodNameAPIKey,
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

func (p *GoogleProvider) newClient(ctx context.Context, cred *domain.Credential) (*genai.Client, error) {
	if cred == nil || cred.APIKey == "" {
		return nil, fmt.Errorf("google provider requires an API key")
	}

	cfg := &genai.ClientConfig{
		APIKey: cred.APIKey,
	}

	return genai.NewClient(ctx, cfg)
}

// List returns the available models provided by Google.
func (p *GoogleProvider) List() []domain.LLMInfo {
	return p.models
}

// GetLLM initializes and returns a Google-backed LLM instance.
func (p *GoogleProvider) GetLLM(ctx context.Context, cred *domain.Credential, info domain.LLMInfo) (domain.LLM, error) {
	start := time.Now()
	slog.Info("provider google get llm start", "model_id", info.ID)
	client, err := p.newClient(ctx, cred)
	if err != nil {
		slog.Error("provider google client init failed", "model_id", info.ID, "duration_ms", time.Since(start).Milliseconds(), "error", err)
		return nil, err
	}

	chatModel, err := gemini.NewChatModel(ctx, &gemini.Config{
		Client: client,
		Model:  strings.TrimPrefix(info.ID, domain.ProviderGoogle+modelIDSeparator),
	})
	if err != nil {
		slog.Error("provider google model init failed", "model_id", info.ID, "duration_ms", time.Since(start).Milliseconds(), "error", err)
		return nil, fmt.Errorf("create eino gemini model: %w", err)
	}
	slog.Info("provider google get llm success", "model_id", info.ID, "duration_ms", time.Since(start).Milliseconds())

	return &geminiLLM{
		client:        client,
		model:         chatModel,
		id:            info.ID,
		displayName:   info.DisplayName,
		contextWindow: info.ContextWindow,
	}, nil
}

// geminiLLM implements domain.LLM for Google Gemini models.
type geminiLLM struct {
	client        *genai.Client
	model         model.ToolCallingChatModel
	id            string
	displayName   string
	contextWindow int
}

func (m *geminiLLM) ID() string {
	return m.id
}

func (m *geminiLLM) DisplayName() string {
	return m.displayName
}

func (m *geminiLLM) ContextWindow() int {
	return m.contextWindow
}

func (m *geminiLLM) Model() model.ToolCallingChatModel {
	return m.model
}
