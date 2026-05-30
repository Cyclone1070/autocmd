package workflow

import (
	"context"
	"testing"
	"time"

	"github.com/Cyclone1070/autocmd/internal/domain"
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

type mockStateSaver struct {
	mock.Mock
}

func (m *mockStateSaver) Save(s *domain.State) error {
	args := m.Called(s)
	return args.Error(0)
}

func TestRunModelPicker(t *testing.T) {
	ctx := t.Context()

	registry := new(mockModelLLMRegistry)
	st := &domain.State{Model: "m1"}
	bus := new(mockModelPickerBus)

	models := []domain.LLMInfo{{ID: "m1", DisplayName: "Model 1"}}
	registry.On("List", mock.Anything).Return(models, nil)

	// Expect initial snapshot
	bus.On("SendUIUpdate", mock.MatchedBy(func(ev domain.UIUpdate) bool {
		snapshot, ok := ev.(domain.ModelListEvent)
		return ok && snapshot.ActiveModelID == "m1"
	})).Return()

	actions := make(chan domain.Action, 1)
	bus.On("WorkflowActions").Return((<-chan domain.Action)(actions))

	t.Run("Selection success", func(t *testing.T) {
		saver := new(mockStateSaver)
		saver.On("Save", mock.MatchedBy(func(s *domain.State) bool {
			return s.Model == "m2"
		})).Return(nil)
		bus.On("SendUIUpdate", domain.DoneEvent{}).Return()

		done := RunModelPicker(ctx, &ModelPickerDeps{
			Bus:      bus,
			Registry: registry,
			State:    st,
			Saver:    saver,
		})

		actions <- domain.SelectModelAction{ID: "m2"}

		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(200 * time.Millisecond):
			t.Fatal("workflow timed out")
		}

		saver.AssertExpectations(t)
		bus.AssertExpectations(t)
	})
}
