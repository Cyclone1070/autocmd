// Package session_picker provides UI components for selecting and managing chat sessions.
package session_picker

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

const keyEsc = "esc"

type bus interface {
	UIUpdates() <-chan domain.UIUpdate
	SendAction(domain.Action)
}

// Model is an autonomous UI component for managing chat sessions.
type Model struct {
	bus             bus
	err             error
	picker          *ui.Picker
	theme           *ui.Theme
	renameItemID    string
	selectedID      string
	selectedName    string
	textInput       textinput.Model
	renaming        bool
	quitting        bool
	cancelRequested bool
}

// NewModel creates a new session picker UI Model with a bus and theme.
func NewModel(b bus, theme *ui.Theme) *Model {
	ti := textinput.New()
	ti.Placeholder = "New session name..."

	return &Model{
		bus:       b,
		theme:     theme,
		textInput: ti,
	}
}

// Init starts the session loading process.
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

// Update handles UI interactions and translates them into workflow calls.
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
	case domain.SessionListEvent:
		m.initializePicker(&msg)
		return m, m.pollBus()

	case domain.DoneEvent:
		m.quitting = true
		if m.selectedID == "" {
			return m, tea.Quit
		}
		return m, tea.Sequence(
			tea.Printf("\nSelected session: %s\n", m.theme.Success(m.selectedName)),
			tea.Quit,
		)

	case tea.KeyMsg:
		if m.renaming {
			switch msg.String() {
			case "enter":
				newName := strings.TrimSpace(m.textInput.Value())
				if newName != "" && m.renameItemID != "" {
					id := m.renameItemID
					m.renaming = false
					m.renameItemID = ""
					m.bus.SendAction(domain.RenameSessionAction{ID: id, Name: newName})
					return m, nil
				}
				m.renaming = false
				return m, nil
			case keyEsc:
				m.renaming = false
				return m, nil
			}
			var cmd tea.Cmd
			m.textInput, cmd = m.textInput.Update(msg)
			return m, cmd
		}

		if m.picker == nil {
			if msg.String() == "ctrl+c" || msg.String() == "q" || msg.String() == "esc" {
				m.cancelRequested = true
				m.selectedID = ""
				m.selectedName = ""
				m.bus.SendAction(domain.StopAction{})
				return m, m.pollBus()
			}
			return m, nil
		}

		switch msg.String() {
		case "n":
			m.selectedName = "(new session)"
			m.bus.SendAction(domain.CreateSessionAction{})
			return m, nil
		case "r":
			if item, ok := m.picker.CursorItem(); ok {
				m.renaming = true
				m.renameItemID = item.ID
				m.textInput.SetValue(item.Label)
				m.textInput.Focus()
				return m, textinput.Blink
			}
		case "d":
			if item, ok := m.picker.CursorItem(); ok {
				m.bus.SendAction(domain.DeleteSessionAction{ID: item.ID})
				return m, nil
			}
		case "enter", " ":
			if item, ok := m.picker.CursorItem(); ok {
				m.selectedID = item.ID
				m.selectedName = item.Label
				m.bus.SendAction(domain.SelectSessionAction{ID: item.ID})
				return m, nil
			}
		case "q", "esc", "ctrl+c":
			m.cancelRequested = true
			m.selectedID = "" // signal cancellation (suppresses DoneEvent print)
			m.selectedName = "Cancelled"
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

func (m *Model) initializePicker(data *domain.SessionListEvent) {
	items := make([]ui.Item, 0, len(data.Sessions))
	for _, s := range data.Sessions {
		name := s.Name
		if name == "" {
			name = "(new session)"
		}

		if s.ID == data.CurrentSessionID {
			m.selectedID = s.ID
			m.selectedName = name
		}

		groupName := s.WorkingDir
		if groupName == "" {
			groupName = "(global)"
		} else {
			home, err := os.UserHomeDir()
			if err == nil && strings.HasPrefix(groupName, home) {
				groupName = "~" + strings.TrimPrefix(groupName, home)
			}
		}

		items = append(items, ui.Item{
			ID:     s.ID,
			Label:  name,
			Detail: fmt.Sprintf("%d msgs  %s", s.MessageCount, s.Updated.Format("2.Jan 15:04")),
			Active: s.ID == data.CurrentSessionID,
			Group:  groupName,
		})
	}

	cfg := ui.Config{
		Title: "SESSIONS",
		Items: items,
		Theme: m.theme,
		Actions: []ui.Action{
			{Key: "n", Label: "new"},
			{Key: "r", Label: "rename"},
			{Key: "d", Label: "delete"},
		},
	}
	m.picker = ui.NewPicker(cfg)
}

func getDateGroup(t time.Time) string {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	yesterday := today.AddDate(0, 0, -1)
	thisWeek := today.AddDate(0, 0, -7)

	if t.After(today) {
		return "Today"
	}
	if t.After(yesterday) {
		return "Yesterday"
	}
	if t.After(thisWeek) {
		return "This Week"
	}
	return "Earlier"
}

// View determines what content to display based on the internal state.
func (m *Model) View() string {
	if m.quitting || m.err != nil {
		return ""
	}
	if m.renaming {
		return fmt.Sprintf("\n  Rename session:\n\n  %s\n\n  (Enter to save, Esc to cancel)\n", m.textInput.View())
	}
	if m.picker != nil {
		return m.picker.View()
	}
	return ""
}

// Err returns any error encountered during session management.
func (m *Model) Err() error {
	return m.err
}

// SelectedID returns the ID of the chosen session if any.
func (m *Model) SelectedID() string {
	return m.selectedID
}
