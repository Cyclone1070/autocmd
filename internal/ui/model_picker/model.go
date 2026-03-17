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

type prepareResultMsg struct {
	data *domain.ModelPickerResult
	err  error
}

// Model is a domain-specific UI model for selecting LLM models.
type Model struct {
	picker     *ui.Picker
	wf         Workflow
	selectedID string
	err        error
	quitting   bool
	fetching   bool
}

// NewModel creates a new model picker UI model with an injected workflow.
func NewModel(wf Workflow) *Model {
	return &Model{
		wf:       wf,
		fetching: true,
	}
}

// Init initializes the model by fetching the available models.
func (m *Model) Init() tea.Cmd {
	return func() tea.Msg {
		res, err := m.wf.PrepareSelection(context.Background())
		return prepareResultMsg{data: res, err: err}
	}
}

// Update handles UI messages.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case prepareResultMsg:
		m.fetching = false
		if msg.err != nil {
			m.err = msg.err
			return m, tea.Quit
		}
		m.initializePicker(msg.data)
		return m, m.picker.Init()

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
		if m.fetching {
			if msg.String() == "ctrl+c" || msg.String() == "q" || msg.String() == "esc" {
				m.quitting = true
				return m, tea.Quit
			}
			return m, nil
		}
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

	if m.picker != nil {
		newModel, cmd := m.picker.Update(msg)
		m.picker = newModel.(*ui.Picker)
		return m, cmd
	}

	return m, nil
}

func (m *Model) initializePicker(data *domain.ModelPickerResult) {
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
	m.picker = ui.NewPicker(cfg)
}

// View returns the string representation of the UI.
func (m *Model) View() string {
	if m.quitting || m.err != nil {
		return ""
	}
	if m.fetching {
		return "\n  Fetching models...\n"
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
