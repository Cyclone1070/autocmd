package model_picker

import (
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestModelSelection(t *testing.T) {
	data := &domain.ModelPickerResult{
		Models: []domain.LLMInfo{
			{ID: "m1", DisplayName: "Model 1"},
			{ID: "m2", DisplayName: "Model 2"},
		},
		ActiveModelID: "m1",
	}

	m := NewModel(data)

	t.Run("Selection changes SelectedID", func(t *testing.T) {
		m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})

		selectedID, ok := m.SelectedID()
		assert.True(t, ok)
		assert.Equal(t, "m2", selectedID)
	})

	t.Run("Escape clears selection", func(t *testing.T) {
		m := NewModel(data)
		m.Update(tea.KeyMsg{Type: tea.KeyEsc})

		_, ok := m.SelectedID()
		assert.False(t, ok)
	})
}
