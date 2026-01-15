package google

import (
	"context"
	"fmt"

	"github.com/Cyclone1070/iav/internal/domain"
	"google.golang.org/genai"
)

// Provider implements the internal provider interface for Google Gemini.
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

func (p *Provider) ListModels(ctx context.Context) ([]domain.ModelInfo, error) {
	return []domain.ModelInfo{
		{ID: "gemini-2.5-flash-lite", DisplayName: "Gemini 2.5 Flash Lite"},
		{ID: "gemini-2.5-flash", DisplayName: "Gemini 2.5 Flash"},
		{ID: "gemini-3.0-flash", DisplayName: "Gemini 3.0 Flash"},
		{ID: "gemini-2.5-pro", DisplayName: "Gemini 2.5 Pro"},
		{ID: "gemini-3.0-pro-preview", DisplayName: "Gemini 3.0 Pro Preview"},
	}, nil
}

func (p *Provider) GetModel(ctx context.Context, id string) (domain.Model, error) {
	// Get model info from API for context window
	info, err := p.client.Models.Get(ctx, id, nil)
	if err != nil {
		return nil, fmt.Errorf("get model info: %w", err)
	}

	return &googleModel{
		client:        p.client,
		id:            id,
		displayName:   info.DisplayName,
		contextWindow: int(info.InputTokenLimit),
	}, nil
}
