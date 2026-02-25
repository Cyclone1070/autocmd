package workflow

import (
	"context"
	"fmt"

	"github.com/Cyclone1070/iav/internal/domain"
)

// ListLLMs returns available LLMs from the registry.
func (w *Workflow) ListLLMs(ctx context.Context) ([]domain.LLMInfo, error) {
	return w.llmRegistry.List(ctx)
}

// GetModel returns the current model ID from config.
func (w *Workflow) GetModel() string {
	return w.cfg.Model
}

// SetModel validates the model ID against the registry, then updates
// both the config field and the resolved LLM instance.
func (w *Workflow) SetModel(ctx context.Context, id string) error {
	_, err := w.llmRegistry.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("unknown model %q: %w", id, err)
	}
	w.cfg.Model = id
	return nil
}
