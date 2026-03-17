package model_picker

import (
	"context"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockWorkflow struct {
	mock.Mock
}

func (m *mockWorkflow) PrepareSelection(ctx context.Context) (*domain.ModelPickerResult, error) {
	args := m.Called(ctx)
	return args.Get(0).(*domain.ModelPickerResult), args.Error(1)
}

func (m *mockWorkflow) ApplySelection(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func TestModelSelection(t *testing.T) {
	data := &domain.ModelPickerResult{
		Models: []domain.LLMInfo{
			{ID: "m1", DisplayName: "Model 1"},
			{ID: "m2", DisplayName: "Model 2"},
		},
		ActiveModelID: "m1",
	}

	t.Run("Selection triggers ApplySelection and returns SelectedID", func(t *testing.T) {
		wf := new(mockWorkflow)
		wf.On("ApplySelection", mock.Anything, "m2").Return(nil)
		
		m := NewModel(data, wf)
		
		// Move cursor down to m2
		m.Update(tea.KeyMsg{Type: tea.KeyDown})
		
		// Press Enter - this should return a Cmd
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		assert.NotNil(t, cmd)
		
		// Execute the cmd
		msg := cmd()
		
		// Send the result back to the model
		m.Update(msg)

		selectedID, ok := m.SelectedID()
		assert.True(t, ok)
		assert.Equal(t, "m2", selectedID)
		
		// CRITICAL: The view must be empty after selection to allow clean terminal output
		assert.Empty(t, m.View(), "View should be empty after selection to clear the picker")
		
		wf.AssertExpectations(t)
	})

	t.Run("Escape clears selection without calling ApplySelection", func(t *testing.T) {
		wf := new(mockWorkflow)
		m := NewModel(data, wf)
		m.Update(tea.KeyMsg{Type: tea.KeyEsc})

		_, ok := m.SelectedID()
		assert.False(t, ok)
		assert.Empty(t, m.View(), "View should be empty after escape")
		wf.AssertNotCalled(t, "ApplySelection", mock.Anything, mock.Anything)
	})
}
