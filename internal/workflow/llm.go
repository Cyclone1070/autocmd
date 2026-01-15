package workflow

import (
	"context"

	"github.com/Cyclone1070/iav/internal/domain"
)

// SetModel sets the current model for LLM requests by resolving it via registry.
func (w *Workflow) SetModel(ctx context.Context, id string) error {
	m, err := w.modelRegistry.Get(ctx, id)
	if err != nil {
		return err
	}
	w.currentModel = m
	return nil
}

// ListModels returns available models from the registry.
func (w *Workflow) ListModels(ctx context.Context) ([]domain.ModelInfo, error) {
	return w.modelRegistry.List(ctx)
}
