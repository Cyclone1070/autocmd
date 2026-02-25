package cmd

import (
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/session"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type sessionPickerModel struct {
	cfg       *config.Config
	store     *session.Store
	sessions  []domain.SessionSummary
	cursor    int
	quit      bool
	err       error
	selected  string
	renaming  bool
	textInput textinput.Model
}

func newSessionPickerModel(cfg *config.Config, store *session.Store, sessions []domain.SessionSummary) *sessionPickerModel {
	ti := textinput.New()
	ti.Placeholder = "Session name..."
	ti.CharLimit = 50
	ti.Width = 30

	return &sessionPickerModel{
		cfg:       cfg,
		store:     store,
		sessions:  sessions,
		textInput: ti,
	}
}

func (m *sessionPickerModel) Init() tea.Cmd {
	return nil
}

func (m *sessionPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.renaming {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "enter":
				id := m.sessions[m.cursor].ID
				newName := strings.TrimSpace(m.textInput.Value())
				if newName != "" {
					_ = m.store.Rename(id, newName)
					m.sessions, _ = m.store.List()
				}
				m.renaming = false
				m.textInput.Blur()
				return m, nil
			case "esc":
				m.renaming = false
				m.textInput.Blur()
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}

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

		case "r":
			// Rename session
			if len(m.sessions) > 0 {
				m.renaming = true
				m.textInput.SetValue(m.sessions[m.cursor].Name)
				m.textInput.Focus()
				return m, textinput.Blink
			}
		}
	}
	return m, nil
}

func (m *sessionPickerModel) View() string {
	if m.quit {
		if m.selected != "" {
			return lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Render(fmt.Sprintf("\n  Selected session: %s\n", m.selected))
		}
		return "\n  Cancelled.\n"
	}

	if m.err != nil {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(fmt.Sprintf("\n  Error: %v\n", m.err))
	}

	var s strings.Builder

	// Styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86")).
		MarginBottom(1)

	keyStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86"))

	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	helpItem := func(k, d string) string {
		return fmt.Sprintf("%s %s", keyStyle.Render(k), descStyle.Render(d))
	}

	// Header
	s.WriteString(titleStyle.Render("  SESSIONS"))
	s.WriteString("\n")

	if m.renaming {
		s.WriteString("  " + descStyle.Render("Rename current session:") + "\n\n")
		s.WriteString("  " + m.textInput.View() + "\n\n")
		s.WriteString(fmt.Sprintf("  %s %s\n", helpItem("Enter", "save"), helpItem("Esc", "cancel")))
		return s.String()
	}

	// Help line (styled)
	help := fmt.Sprintf("  %s   %s   %s   %s   %s   %s\n\n",
		helpItem("↑/↓", "navigate"),
		helpItem("Enter", "select"),
		helpItem("n", "new"),
		helpItem("r", "rename"),
		helpItem("d", "delete"),
		helpItem("q", "quit"),
	)
	s.WriteString(help)

	if len(m.sessions) == 0 {
		s.WriteString(lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("240")).Render("    No sessions found. Press 'n' to create one.") + "\n")
	}

	// List
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
	cursorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Bold(true)
	inactiveStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	fadedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	for i, sess := range m.sessions {
		isCursor := m.cursor == i
		isActive := sess.ID == m.cfg.Session.CurrentSessionID

		var icon string
		if isCursor {
			icon = cursorStyle.Render("▸")
		} else {
			icon = " "
		}

		status := " "
		if isActive {
			status = activeStyle.Render("●")
		}

		name := sess.Name
		if name == "" {
			name = "(untitled)"
		}

		// Truncate name
		if len(name) > 40 {
			name = name[:37] + "..."
		}

		var nameText string
		if isCursor {
			nameText = activeStyle.Bold(true).Render(name)
		} else {
			nameText = inactiveStyle.Render(name)
		}

		msgCount := sess.MessageCount
		msgs := fadedStyle.Render(fmt.Sprintf("%d msg%s", msgCount, plural(msgCount)))
		date := fadedStyle.Render(sess.Updated.Format("2.Jan 15:04"))

		line := fmt.Sprintf(" %s %s %-40s  %s  %s", icon, status, nameText, msgs, date)
		s.WriteString(line + "\n")
	}

	return s.String()
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
