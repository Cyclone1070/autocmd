package model_picker

import (
	"context"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

// Workflow defines the operations needed for model selection.
type Workflow interface {
	PrepareSelection(ctx context.Context) (*domain.ModelPickerResult, error)
	ApplySelection(ctx context.Context, id string) error
}

type applyResultMsg struct {
	id  string
	err error
}

// Model is a domain-specific UI model for selecting LLM models.
type Model struct {
	picker     *ui.Picker
	wf         Workflow
	selectedID string
	err        error
	quitting   bool
}

// NewModel creates a new model picker UI model with an injected workflow.
func NewModel(data *domain.ModelPickerResult, wf Workflow) *Model {
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
		wf:     wf,
	}
}

// Init initializes the model.
func (m *Model) Init() tea.Cmd {
	return m.picker.Init()
}

// Update handles UI messages.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case applyResultMsg:
		m.quitting = true
		if msg.err != nil {
			m.err = msg.err
			return m, tea.Quit
		}
		m.selectedID = msg.id
		// Return tea.Printf followed by tea.Quit to show the persistent message
		return m, tea.Sequence(
			tea.Printf("\nSelected model: %s\n", msg.id),
			tea.Quit,
		)

	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if item, ok := m.picker.CursorItem(); ok {
				return m, func() tea.Msg {
					err := m.wf.ApplySelection(context.Background(), item.ID)
					return applyResultMsg{id: item.ID, err: err}
				}
			}
		case "q", "esc", "ctrl+c":
			m.quitting = true
		}
	}

	newModel, cmd := m.picker.Update(msg)
	m.picker = newModel.(*ui.Picker)
	return m, cmd
}

// View returns the string representation of the UI.
func (m *Model) View() string {
	if m.quitting || m.err != nil {
		return ""
	}
	return m.picker.View()
}

// SelectedID returns the ID of the selected model and true, or empty string and false if cancelled.
func (m *Model) SelectedID() (string, bool) {
	if m.selectedID != "" {
		return m.selectedID, true
	}
	return "", false
}

// Err returns any error encountered during the selection process.
func (m *Model) Err() error {
	return m.err
}
