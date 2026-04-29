// Package model_picker provides UI components for selecting LLM models.
package model_picker

import (
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

type bus interface {
	UIUpdates() <-chan domain.UIUpdate
	SendAction(domain.Action)
}

// Model is a domain-specific UI Model for selecting LLM models.
type Model struct {
	bus             bus
	err             error
	picker          *ui.Picker
	theme           *ui.Theme
	selectedName    string
	quitting        bool
	cancelRequested bool
}

// NewModel creates a new Model picker UI Model with a bus and theme.
func NewModel(b bus, theme *ui.Theme) *Model {
	return &Model{
		bus:   b,
		theme: theme,
	}
}

// Init initializes the model by starting the bus polling.
func (m *Model) Init() tea.Cmd {
	return m.pollBus()
}

func (m *Model) pollBus() tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-m.bus.UIUpdates()
		if !ok {
			return tea.Sequence(
				tea.Printf("\n %s\n", m.theme.Error("Error: bus closed unexpectedly")),
				tea.Quit,
			)()
		}
		return ev
	}
}

// Update handles UI messages.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.cancelRequested {
		switch msg.(type) {
		case domain.DoneEvent:
			// handled below
		default:
			return m, m.pollBus()
		}
	}

	switch msg := msg.(type) {
	case domain.ModelListEvent:
		m.initializePicker(&msg)
		return m, m.pollBus()

	case domain.DoneEvent:
		m.quitting = true
		if m.selectedName == "" {
			return m, tea.Quit
		}
		return m, tea.Sequence(
			tea.Printf("\nSelected model: %s\n", m.theme.Success(m.selectedName)),
			tea.Quit,
		)

	case tea.KeyMsg:
		if m.picker == nil {
			if msg.String() == "ctrl+c" || msg.String() == "q" || msg.String() == "esc" {
				m.cancelRequested = true
				m.bus.SendAction(domain.StopAction{})
				m.selectedName = ""
				return m, m.pollBus()
			}
			return m, nil
		}
		switch msg.String() {
		case "enter", " ":
			if item, ok := m.picker.CursorItem(); ok {
				m.selectedName = item.Label
				m.bus.SendAction(domain.SelectModelAction{ID: item.ID})
				return m, nil
			}
		case "q", "esc", "ctrl+c":
			m.cancelRequested = true
			m.selectedName = "" // Signal cancellation (suppresses DoneEvent print)
			m.bus.SendAction(domain.StopAction{})
			return m, m.pollBus()
		}
	}

	if m.picker != nil {
		newModel, cmd := m.picker.Update(msg)
		m.picker = newModel.(*ui.Picker)
		return m, cmd
	}

	return m, nil
}

func (m *Model) initializePicker(data *domain.ModelListEvent) {
	items := make([]ui.Item, 0, len(data.Models))
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
		Theme: m.theme,
	}
	m.picker = ui.NewPicker(cfg)
}

// View returns the string representation of the UI.
func (m *Model) View() string {
	if m.quitting || m.err != nil {
		return ""
	}
	if m.picker != nil {
		return m.picker.View()
	}
	return ""
}

// Err returns any error encountered during the selection process.
func (m *Model) Err() error {
	return m.err
}
