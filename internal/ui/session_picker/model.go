package session_picker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// Workflow defines the operations needed for session management.
type Workflow interface {
	PrepareSelection(ctx context.Context) (*domain.SessionPickerSnapshot, error)
	ApplySelection(ctx context.Context, id string) error
	CreateSession(ctx context.Context) (string, error)
	RenameSession(ctx context.Context, id, name string) error
	DeleteSession(ctx context.Context, id string) error
}

type prepareResultMsg struct {
	data *domain.SessionPickerSnapshot
	err  error
}

type mutationResultMsg struct {
	err     error
	refresh bool
}

type applyResultMsg struct {
	id  string
	err error
}

// Model is an autonomous UI component for managing chat sessions.
type Model struct {
	picker       *ui.Picker
	textInput    textinput.Model
	wf           Workflow
	fetching     bool
	renaming     bool
	renameItemID string
	quitting     bool
	err          error
	selectedID   string
}

// NewModel creates a new session picker UI with an injected workflow.
func NewModel(wf Workflow) *Model {
	ti := textinput.New()
	ti.Placeholder = "New session name..."

	return &Model{
		wf:        wf,
		textInput: ti,
		fetching:  true,
	}
}

// Init starts the session loading process.
func (m *Model) Init() tea.Cmd {
	return func() tea.Msg {
		res, err := m.wf.PrepareSelection(context.Background())
		return prepareResultMsg{data: res, err: err}
	}
}

// Update handles UI interactions and translates them into workflow calls.
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

	case mutationResultMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, tea.Quit
		}
		if msg.refresh {
			return m, m.Init()
		}
		return m, nil

	case applyResultMsg:
		m.quitting = true
		if msg.err != nil {
			m.err = msg.err
			return m, tea.Quit
		}
		m.selectedID = msg.id
		return m, tea.Sequence(
			tea.Printf("\nSelected session\n"),
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
					return m, func() tea.Msg {
						err := m.wf.RenameSession(context.Background(), id, newName)
						return mutationResultMsg{err: err, refresh: true}
					}
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
				m.quitting = true
				return m, tea.Quit
			}
			return m, nil
		}

		switch msg.String() {
		case "n":
			return m, func() tea.Msg {
				id, err := m.wf.CreateSession(context.Background())
				if err == nil {
					return applyResultMsg{id: id}
				}
				return mutationResultMsg{err: err}
			}
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
				return m, func() tea.Msg {
					err := m.wf.DeleteSession(context.Background(), item.ID)
					return mutationResultMsg{err: err, refresh: true}
				}
			}
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

func (m *Model) initializePicker(data *domain.SessionPickerSnapshot) {
	var items []ui.Item
	for _, s := range data.Sessions {
		name := s.Name
		if name == "" {
			name = "(untitled)"
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
func (m *Model) View() string {
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
func (m *Model) Err() error {
	return m.err
}

// SelectedID returns the ID of the chosen session if any.
func (m *Model) SelectedID() string {
	return m.selectedID
}
