package cmd

import (
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/session"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type sessionPickerModel struct {
	cfg      *config.Config
	store    *session.Store
	sessions []domain.SessionSummary
	cursor   int
	quit     bool
	err      error
	selected string
}

func newSessionPickerModel(cfg *config.Config, store *session.Store, sessions []domain.SessionSummary) *sessionPickerModel {
	return &sessionPickerModel{
		cfg:      cfg,
		store:    store,
		sessions: sessions,
	}
}

func (m *sessionPickerModel) Init() tea.Cmd {
	return nil
}

func (m *sessionPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.quit = true
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(m.sessions)-1 {
				m.cursor++
			}

		case "enter":
			if len(m.sessions) > 0 {
				id := m.sessions[m.cursor].ID
				m.cfg.Session.CurrentSessionID = id
				_ = config.Save(m.cfg)
				m.selected = id
				m.quit = true
				return m, tea.Quit
			}

		case "n":
			// Create new session
			sess, err := m.store.Create()
			if err != nil {
				m.err = err
				return m, nil
			}
			m.cfg.Session.CurrentSessionID = sess.ID
			_ = config.Save(m.cfg)
			m.selected = sess.ID
			m.quit = true
			return m, tea.Quit

		case "d":
			// Delete session
			if len(m.sessions) > 0 {
				id := m.sessions[m.cursor].ID
				_ = m.store.Delete(id)
				// Refresh list
				m.sessions, _ = m.store.List()
				if m.cursor >= len(m.sessions) && m.cursor > 0 {
					m.cursor = len(m.sessions) - 1
				}
			}
		}
	}
	return m, nil
}

func (m *sessionPickerModel) View() string {
	if m.quit {
		if m.selected != "" {
			return fmt.Sprintf("Selected session: %s\n", m.selected)
		}
		return "Cancelled.\n"
	}

	if m.err != nil {
		return fmt.Sprintf("Error: %v\n", m.err)
	}

	var s strings.Builder
	s.WriteString("Select a session (↑/↓ to navigate, Enter to select, n for new, d to delete, q to quit):\n\n")

	if len(m.sessions) == 0 {
		s.WriteString("  No sessions found. Press 'n' to create one.\n")
	}

	for i, sess := range m.sessions {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}

		active := " "
		if sess.ID == m.cfg.Session.CurrentSessionID {
			active = "*"
		}

		name := sess.Name
		if name == "" {
			name = "(untitled)"
		}

		// Truncate name if too long
		if len(name) > 40 {
			name = name[:37] + "..."
		}

		date := sess.Updated.Format("2006-01-02 15:04")
		msgs := fmt.Sprintf("%d msgs", sess.MessageCount)

		line := fmt.Sprintf("%s %s %-40s %s (%s)", cursor, active, name, msgs, date)

		if m.cursor == i {
			line = lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Render(line)
		}

		s.WriteString(line + "\n")
	}

	return s.String()
}
