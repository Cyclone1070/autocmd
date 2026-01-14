package workflow

import "context"

// SetModel sets the current model for LLM requests.
func (w *Workflow) SetModel(model string) {
	w.currentModel = model
}

// ListModels returns available models from the provider.
func (w *Workflow) ListModels(ctx context.Context) ([]string, error) {
	return w.provider.ListModels(ctx)
}

