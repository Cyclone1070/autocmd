package model_picker

import (
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

// Model is a domain-specific UI model for selecting LLM models.
type Model struct {
	picker *ui.Picker
}

// NewModel creates a new model picker UI model.
func NewModel(data *domain.ModelPickerResult) *Model {
	var items []ui.Item
	for _, m := range data.Models {
		items = append(items, ui.Item{
			ID:     m.ID,
			Label:  m.DisplayName,
			Detail: m.ID,
			Active: m.ID == data.ActiveModelID,
		})
	}

	cfg := ui.Config{
		Title: "MODELS",
		Items: items,
	}

	return &Model{
		picker: ui.NewPicker(cfg),
	}
}

// Init initializes the model.
func (m *Model) Init() tea.Cmd {
	return m.picker.Init()
}

// Update handles UI messages.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	newModel, cmd := m.picker.Update(msg)
	m.picker = newModel.(*ui.Picker)
	return m, cmd
}

// View returns the string representation of the UI.
func (m *Model) View() string {
	return m.picker.View()
}

// SelectedID returns the ID of the selected model and true, or empty string and false if cancelled.
func (m *Model) SelectedID() (string, bool) {
	if item, ok := m.picker.Selected(); ok {
		return item.ID, true
	}
	return "", false
}

// RenderSuccess prints a success message after a model is selected.
func (m *Model) RenderSuccess(cmd *cobra.Command, modelID string) {
	cmd.Printf("\nSelected model: %s\n", modelID)
}
