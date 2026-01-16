package workflow

import (
	"context"

	"github.com/Cyclone1070/iav/internal/domain"
)

// SetLLM sets the current LLM for requests by resolving it via registry.
func (w *Workflow) SetLLM(ctx context.Context, id string) error {
	m, err := w.llmRegistry.Get(ctx, id)
	if err != nil {
		return err
	}
	w.currentLLM = m
	return nil
}

// ListLLMs returns available LLMs from the registry.
func (w *Workflow) ListLLMs(ctx context.Context) ([]domain.LLMInfo, error) {
	return w.llmRegistry.List(ctx)
}
