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

type modelPickerBus interface {
	SendUIUpdate(domain.UIUpdate)
	WorkflowActions() <-chan domain.Action
}

// ModelPickerDeps contains the dependencies for the model selection workflow.
type ModelPickerDeps struct {
	Bus      modelPickerBus
	Registry modelLLMRegistry
	State    modelState
}

// RunModelPicker starts the model selection workflow asynchronously.
func RunModelPicker(ctx context.Context, deps *ModelPickerDeps) <-chan error {
	done := make(chan error, 1)
	go func() {
		defer close(done)
		wf := newModelPickerWorkflow(deps.Registry, deps.State)

		// 1. Send initial snapshot
		snapshot, err := wf.prepareSelection(ctx)
		if err != nil {
			done <- err
			return
		}
		deps.Bus.SendUIUpdate(snapshot)

		// 2. Action loop
		for {
			select {
			case <-ctx.Done():
				done <- ctx.Err()
				return
			case act, ok := <-deps.Bus.WorkflowActions():
				if !ok {
					done <- nil
					return
				}

				switch a := act.(type) {
				case domain.SelectModelAction:
					if err := wf.applySelection(a.ID); err != nil {
						done <- err
						return
					}
					deps.Bus.SendUIUpdate(domain.DoneEvent{})
					done <- nil
					return

				case domain.StopAction:
					deps.Bus.SendUIUpdate(domain.DoneEvent{})
					done <- nil
					return
				}
			}
		}
	}()
	return done
}

type modelPickerWorkflow struct {
	registry modelLLMRegistry
	state    modelState
}

func newModelPickerWorkflow(registry modelLLMRegistry, state modelState) *modelPickerWorkflow {
	return &modelPickerWorkflow{
		registry: registry,
		state:    state,
	}
}

func (w *modelPickerWorkflow) prepareSelection(ctx context.Context) (domain.ModelListEvent, error) {
	models, err := w.registry.List(ctx)
	if err != nil {
		return domain.ModelListEvent{}, err
	}

	return domain.ModelListEvent{
		Models:        models,
		ActiveModelID: w.state.Model(),
	}, nil
}

func (w *modelPickerWorkflow) applySelection(id string) error {
	w.state.SetModel(id)
	return w.state.Save()
}
