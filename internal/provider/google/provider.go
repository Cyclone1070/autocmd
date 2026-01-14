package google

import (
	"context"
	"fmt"

	"github.com/Cyclone1070/iav/internal/domain"
	"google.golang.org/genai"
)

// Provider implements domain.Provider for Google Gemini.
type Provider struct {
	client *genai.Client
}

// NewProvider creates a new Google provider using the given API key.
func NewProvider(ctx context.Context, apiKey string) (*Provider, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}
	return &Provider{client: client}, nil
}

func (p *Provider) Name() string {
	return "google"
}

func (p *Provider) ListModels(ctx context.Context) ([]domain.Model, error) {
	return []domain.Model{
		{ID: "gemini-2.5-flash-lite", DisplayName: "Gemini 2.5 Flash Lite"},
		{ID: "gemini-2.5-flash", DisplayName: "Gemini 2.5 Flash"},
		{ID: "gemini-3.0-flash", DisplayName: "Gemini 3.0 Flash"},
		{ID: "gemini-2.5-pro", DisplayName: "Gemini 2.5 Pro"},
		{ID: "gemini-3.0-pro-preview", DisplayName: "Gemini 3.0 Pro Preview"},
	}, nil
}

func (p *Provider) Stream(ctx context.Context, model string, msgs []domain.Message, tools []domain.Declaration) (domain.Stream, error) {
	// The SDK uses "model" role for assistant and handles system instructions via config.
	hist, err := toHistory(msgs)
	if err != nil {
		return nil, err
	}

	config := &genai.GenerateContentConfig{
		Tools: toTools(tools),
	}
	if hist.SystemPrompt != "" {
		config.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{{Text: hist.SystemPrompt}},
		}
	}

	// Combine history + last message into one contents slice
	contents := hist.Contents

	iter := p.client.Models.GenerateContentStream(ctx, model, contents, config)

	return &googleStream{
		iter: iter,
	}, nil
}
