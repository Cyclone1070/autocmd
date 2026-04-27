package workflow

import (
	"context"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockModelLLMRegistry struct {
	mock.Mock
}

func (m *mockModelLLMRegistry) List(ctx context.Context) ([]domain.LLMInfo, error) {
	args := m.Called(ctx)
	return args.Get(0).([]domain.LLMInfo), args.Error(1)
}

type mockModelState struct {
	mock.Mock
}

func (m *mockModelState) Model() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockModelState) SetModel(id string) {
	m.Called(id)
}

func (m *mockModelState) Save() error {
	args := m.Called()
	return args.Error(0)
}

type mockModelPickerBus struct {
	mock.Mock
}

func (m *mockModelPickerBus) SendUIUpdate(update domain.UIUpdate) {
	m.Called(update)
}

func (m *mockModelPickerBus) WorkflowActions() <-chan domain.Action {
	args := m.Called()
	return args.Get(0).(<-chan domain.Action)
}

func TestRunModelPicker(t *testing.T) {
	ctx := t.Context()

	registry := new(mockModelLLMRegistry)
	state := new(mockModelState)
	bus := new(mockModelPickerBus)

	models := []domain.LLMInfo{{ID: "m1", DisplayName: "Model 1"}}
	registry.On("List", mock.Anything).Return(models, nil)
	state.On("Model").Return("m1")

	// Expect initial snapshot
	bus.On("SendUIUpdate", mock.MatchedBy(func(ev domain.UIUpdate) bool {
		snapshot, ok := ev.(domain.ModelListEvent)
		return ok && snapshot.ActiveModelID == "m1"
	})).Return()

	actions := make(chan domain.Action, 1)
	bus.On("WorkflowActions").Return((<-chan domain.Action)(actions))

	t.Run("Selection success", func(t *testing.T) {
		state.On("SetModel", "m2").Return()
		state.On("Save").Return(nil)
		bus.On("SendUIUpdate", domain.DoneEvent{}).Return()

		done := RunModelPicker(ctx, &ModelPickerDeps{
			Bus:      bus,
			Registry: registry,
			State:    state,
		})

		actions <- domain.SelectModelAction{ID: "m2"}

		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(200 * time.Millisecond):
			t.Fatal("workflow timed out")
		}

		state.AssertExpectations(t)
		bus.AssertExpectations(t)
	})
}
