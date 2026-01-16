package google

import (
	"context"
	"fmt"

	"github.com/Cyclone1070/iav/internal/domain"
	"google.golang.org/genai"
)

// geminiLLM implements domain.LLM for Google Gemini models.
type geminiLLM struct {
	client        *genai.Client
	id            string
	displayName   string
	contextWindow int
}

func (m *geminiLLM) ID() string {
	return "google/" + m.id
}

func (m *geminiLLM) DisplayName() string {
	return m.displayName
}

func (m *geminiLLM) ContextWindow() int {
	return m.contextWindow
}

func (m *geminiLLM) ComputeTokens(ctx context.Context, msgs []domain.Message) (int, error) {
	hist, err := toHistory(msgs)
	if err != nil {
		return 0, fmt.Errorf("convert history: %w", err)
	}

	result, err := m.client.Models.CountTokens(ctx, m.id, hist.Contents, nil)
	if err != nil {
		return 0, fmt.Errorf("count tokens: %w", err)
	}

	return int(result.TotalTokens), nil
}

func (m *geminiLLM) Stream(ctx context.Context, msgs []domain.Message, tools []domain.Declaration) (domain.Stream, error) {
	hist, err := toHistory(msgs)
	if err != nil {
		return nil, fmt.Errorf("convert history: %w", err)
	}

	config := &genai.GenerateContentConfig{
		Tools: toTools(tools),
	}
	if hist.SystemPrompt != "" {
		config.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{{Text: hist.SystemPrompt}},
		}
	}

	iter := m.client.Models.GenerateContentStream(ctx, m.id, hist.Contents, config)

	return &googleStream{
		iter: iter,
	}, nil
}
