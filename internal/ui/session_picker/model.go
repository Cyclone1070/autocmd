package session_picker

import (
	"fmt"
	"strings"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type bus interface {
	UIUpdates() <-chan domain.UIUpdate
	SendAction(domain.Action)
}

// model is an autonomous UI component for managing chat sessions.
type model struct {
	picker       *ui.Picker
	textInput    textinput.Model
	bus          bus
	theme        *ui.Theme
	fetching     bool
	renaming     bool
	renameItemID string
	quitting     bool
	err          error
	selectedID   string
	selectedName string
}

// NewModel creates a new session picker UI with a bus and theme.
func NewModel(b bus, theme *ui.Theme) *model {
	ti := textinput.New()
	ti.Placeholder = "New session name..."

	return &model{
		bus:       b,
		theme:     theme,
		textInput: ti,
		fetching:  true,
	}
}

// Init starts the session loading process.
func (m *model) Init() tea.Cmd {
	return m.pollBus()
}

func (m *model) pollBus() tea.Cmd {
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
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case domain.SessionListEvent:
		m.fetching = false
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
			case "esc":
				m.renaming = false
				return m, nil
			}
			var cmd tea.Cmd
			m.textInput, cmd = m.textInput.Update(msg)
			return m, cmd
		}

		if m.fetching {
			if msg.String() == "ctrl+c" || msg.String() == "q" || msg.String() == "esc" {
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
		case "enter":
			if item, ok := m.picker.CursorItem(); ok {
				m.selectedID = item.ID
				m.selectedName = item.Label
				m.bus.SendAction(domain.SelectSessionAction{ID: item.ID})
				return m, nil
			}
		case "q", "esc", "ctrl+c":
			m.selectedID = "" // signal cancellation
			m.selectedName = "Cancelled"
			m.bus.SendAction(domain.StopAction{})
			return m, nil
		}
	}

	if m.picker != nil {
		newModel, cmd := m.picker.Update(msg)
		m.picker = newModel.(*ui.Picker)
		return m, cmd
	}

	return m, nil
}

func (m *model) initializePicker(data *domain.SessionListEvent) {
	var items []ui.Item
	for _, s := range data.Sessions {
		name := s.Name
		if name == "" {
			name = "(untitled)"
		}

		if s.ID == data.CurrentSessionID {
			m.selectedID = s.ID
			m.selectedName = name
		}

		items = append(items, ui.Item{
			ID:     s.ID,
			Label:  name,
			Detail: fmt.Sprintf("%d msgs  %s", s.MessageCount, s.Updated.Format("2.Jan 15:04")),
			Active: s.ID == data.CurrentSessionID,
			Group:  getDateGroup(s.Updated),
		})
	}

	cfg := ui.Config{
		Title: "SESSIONS",
		Items: items,
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
func (m *model) View() string {
	if m.quitting || m.err != nil {
		return ""
	}
	if m.renaming {
		return fmt.Sprintf("\n  Rename session:\n\n  %s\n\n  (Enter to save, Esc to cancel)\n", m.textInput.View())
	}
	if m.fetching && m.picker == nil {
		return "\n  Fetching sessions...\n"
	}
	if m.picker != nil {
		return m.picker.View()
	}
	return ""
}

// Err returns any error encountered during session management.
func (m *model) Err() error {
	return m.err
}

// SelectedID returns the ID of the chosen session if any.
func (m *model) SelectedID() string {
	return m.selectedID
}
