package workflow

import (
	"context"

	"github.com/Cyclone1070/iav/internal/domain"
)

// modelLLMRegistry defines the interface for model discovery.
type modelLLMRegistry interface {
	List(ctx context.Context) ([]domain.LLMInfo, error)
}

// modelState defines the interface for managing current model state.
type modelState interface {
	Model() string
	SetModel(id string)
	Save() error
}

// ModelPickerWorkflow orchestrates the model selection use case.
type ModelPickerWorkflow struct {
	registry modelLLMRegistry
	state    modelState
}

// NewModelPickerWorkflow creates a new instance of the model picker workflow.
func NewModelPickerWorkflow(registry modelLLMRegistry, state modelState) *ModelPickerWorkflow {
	return &ModelPickerWorkflow{
		registry: registry,
		state:    state,
	}
}

// PrepareSelection gathers the current model state and available models.
func (w *ModelPickerWorkflow) PrepareSelection(ctx context.Context) (*domain.ModelPickerResult, error) {
	models, err := w.registry.List(ctx)
	if err != nil {
		return nil, err
	}

	return &domain.ModelPickerResult{
		Models:        models,
		ActiveModelID: w.state.Model(),
	}, nil
}

// ApplySelection updates the current model in the application state.
func (w *ModelPickerWorkflow) ApplySelection(ctx context.Context, id string) error {
	w.state.SetModel(id)
	return w.state.Save()
}
