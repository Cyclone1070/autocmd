// Package session_picker provides UI components for selecting and managing chat sessions.
package session_picker

import (
	"fmt"
	"strings"

	"github.com/Cyclone1070/autocmd/internal/domain"
	"github.com/Cyclone1070/autocmd/internal/ui"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// pathResolver formats paths for user-facing display.
type pathResolver interface {
	DisplayPath(path string) string
}

const keyEsc = "esc"

type bus interface {
	UIUpdates() <-chan domain.UIUpdate
	SendAction(domain.Action)
}

// Model is an autonomous UI component for managing chat sessions.
type Model struct {
	bus             bus
	pathResolver    pathResolver
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
	switchRequired  bool
	targetDir       string
}

// NewModel creates a new session picker UI Model with a bus, theme, and path resolver.
func NewModel(b bus, theme *ui.Theme, pr pathResolver) *Model {
	ti := textinput.New()
	ti.Placeholder = "New session name..."

	return &Model{
		bus:          b,
		pathResolver: pr,
		theme:        theme,
		textInput:    ti,
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

	case domain.SessionSelectedEvent:
		m.switchRequired = msg.SwitchRequired
		m.targetDir = msg.TargetDir
		return m, m.pollBus()

	case domain.DoneEvent:
		m.quitting = true
		if m.selectedID == "" {
			return m, tea.Quit
		}
		if m.switchRequired {
			displayPath := m.pathResolver.DisplayPath(m.targetDir)
			msgStr := fmt.Sprintf("\nUnable to select session. To continue this session, switch to its directory:\n  cd %s\n\n", displayPath)
			return m, tea.Sequence(
				tea.Printf("%s", msgStr),
				tea.Quit,
			)
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
			m.selectedName = "Untitled"
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
			name = "Untitled"
		}

		if s.ID == data.CurrentSessionID {
			m.selectedID = s.ID
			m.selectedName = name
		}

		groupName := s.WorkingDir
		if groupName == "" {
			groupName = "(global)"
		} else {
			groupName = m.pathResolver.DisplayPath(groupName)
		}

		detail := fmt.Sprintf("%s tokens  %s", ui.ShortNum(s.TokenCount), s.Updated.Format("2.Jan 2006"))

		items = append(items, ui.Item{
			ID:     s.ID,
			Label:  name,
			Detail: detail,
			Active: s.ID == data.CurrentSessionID,
			Group:  groupName,
			Faded:  s.WorkingDir != data.WorkingDir && s.WorkingDir != "",
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
