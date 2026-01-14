package workflow

import (
	"context"
	"fmt"

	"github.com/Cyclone1070/iav/internal/domain"
)

// SetModel sets the current model for LLM requests.
func (w *Workflow) SetModel(model string) {
	w.currentModel = model
}

// ListModels returns available models from the current provider.
func (w *Workflow) ListModels(ctx context.Context) ([]domain.Model, error) {
	if w.currentProvider == nil {
		return nil, fmt.Errorf("no provider selected")
	}
	return w.currentProvider.ListModels(ctx)
}
