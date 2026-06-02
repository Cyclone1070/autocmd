package provider

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Cyclone1070/autocmd/internal/domain"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
)

type OpenCodeProvider struct {
	models []domain.LLMInfo
}

func NewOpenCodeProvider(models []domain.LLMInfo) *OpenCodeProvider {
	return &OpenCodeProvider{models: models}
}

func (p *OpenCodeProvider) ID() string {
	return domain.ProviderOpenCode
}

func (p *OpenCodeProvider) SupportedAuthMethods() []domain.AuthMethod {
	return []domain.AuthMethod{
		domain.APIKeyAuthMethod{
			ID:   domain.AuthMethodAPIKey,
			Name: authMethodNameAPIKey,
			Fields: []domain.AuthField{
				{
					ID:          domain.AuthFieldAPIKey,
					Label:       "API Key",
					Placeholder: "Enter your OpenCode Zen API Key",
					EnvVar:      "OPENCODE_API_KEY",
					IsSecret:    true,
				},
			},
		},
		domain.EnvVarAuthMethod{
			ID:      domain.AuthMethodEnv,
			Name:    "Environment Variables",
			EnvVars: []string{"OPENCODE_API_KEY"},
		},
	}
}

func (p *OpenCodeProvider) List() []domain.LLMInfo {
	return p.models
}

func (p *OpenCodeProvider) GetLLM(ctx context.Context, cred *domain.Credential, info domain.LLMInfo) (domain.LLM, error) {
	start := time.Now()
	slog.Info("provider opencode get llm start", "model_id", info.ID)

	if cred == nil || cred.APIKey == "" {
		return nil, fmt.Errorf("opencode provider requires an API key")
	}

	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		Model:      strings.TrimPrefix(info.ID, domain.ProviderOpenCode+modelIDSeparator),
		BaseURL:    "https://opencode.ai/zen/v1",
		HTTPClient: http.DefaultClient,
		APIKey:     cred.APIKey,
	})
	if err != nil {
		slog.Error("provider opencode model init failed", "model_id", info.ID, "duration_ms", time.Since(start).Milliseconds(), "error", err)
		return nil, fmt.Errorf("create eino openai model: %w", err)
	}
	slog.Info("provider opencode get llm success", "model_id", info.ID, "duration_ms", time.Since(start).Milliseconds())

	return &opencodeLLM{
		model:         chatModel,
		id:            info.ID,
		displayName:   info.DisplayName,
		contextWindow: info.ContextWindow,
	}, nil
}

type opencodeLLM struct {
	model         model.ToolCallingChatModel
	id            string
	displayName   string
	contextWindow int
}

func (l *opencodeLLM) ID() string                        { return l.id }
func (l *opencodeLLM) DisplayName() string               { return l.displayName }
func (l *opencodeLLM) ContextWindow() int                { return l.contextWindow }
func (l *opencodeLLM) Model() model.ToolCallingChatModel { return l.model }
