package picker

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Item is a single selectable row.
type Item struct {
	ID     string
	Label  string // primary text
	Detail string // secondary info
	Active bool   // shows bullet indicator
	Group  string // group header
}

// Action defines a keybinding the caller can register.
type Action struct {
	Key   string
	Label string
	Fn    func(item Item) tea.Cmd
}

// Config configures the picker.
type Config struct {
	Title   string
	Items   []Item
	Actions []Action
}

// Picker is a reusable grouped list selection model.
type Picker struct {
	title    string
	items    []Item
	actions  []Action
	cursor   int
	selected *Item
	quit     bool
	// Internal calculated indices for navigation
	selectableIndices []int
}

func NewPicker(cfg Config) *Picker {
	var indices []int
	for i := range cfg.Items {
		indices = append(indices, i)
	}

	return &Picker{
		title:             cfg.Title,
		items:             cfg.Items,
		actions:           cfg.Actions,
		selectableIndices: indices,
	}
}

func (m *Picker) Init() tea.Cmd {
	return nil
}

func (m *Picker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			if m.cursor < len(m.selectableIndices)-1 {
				m.cursor++
			}

		case "enter":
			if len(m.selectableIndices) > 0 {
				idx := m.selectableIndices[m.cursor]
				m.selected = &m.items[idx]
				m.quit = true
				return m, tea.Quit
			}
		}

		// Handle custom actions
		if len(m.selectableIndices) > 0 {
			idx := m.selectableIndices[m.cursor]
			item := m.items[idx]
			for _, action := range m.actions {
				if msg.String() == action.Key {
					return m, action.Fn(item)
				}
			}
		}
	}
	return m, nil
}

func (m *Picker) View() string {
	if m.quit {
		return ""
	}

	var s strings.Builder

	// Styles
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86")).MarginBottom(1)
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	groupStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("240")).Margin(1, 0, 0, 2)

	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
	cursorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Bold(true)
	inactiveStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	fadedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	helpItem := func(k, d string) string {
		return fmt.Sprintf("%s %s", keyStyle.Render(k), descStyle.Render(d))
	}

	// Title
	s.WriteString(titleStyle.Render("  "+m.title) + "\n")

	// Help lines
	helpParts := []string{
		helpItem("↑/↓", "navigate"),
		helpItem("Enter", "select"),
	}
	for _, a := range m.actions {
		helpParts = append(helpParts, helpItem(a.Key, a.Label))
	}
	helpParts = append(helpParts, helpItem("q", "quit"))

	s.WriteString("  " + strings.Join(helpParts, "   ") + "\n\n")

	if len(m.items) == 0 {
		s.WriteString(descStyle.Render("    No entries found.") + "\n")
		return s.String()
	}

	var lastGroup string
	for i, item := range m.items {
		// Group header
		if item.Group != "" && item.Group != lastGroup {
			displayGroup := item.Group
			if len(displayGroup) > 0 {
				displayGroup = strings.ToUpper(displayGroup[:1]) + displayGroup[1:]
			}
			s.WriteString(groupStyle.Render(displayGroup) + "\n")
			lastGroup = item.Group
		}

		// Find if this specific item index is the one pointed at by m.cursor
		isCursor := false
		if m.cursor < len(m.selectableIndices) && m.selectableIndices[m.cursor] == i {
			isCursor = true
		}

		var icon string
		if isCursor {
			icon = cursorStyle.Render("▸")
		} else {
			icon = " "
		}

		status := " "
		if item.Active {
			status = activeStyle.Render("●")
		}

		var labelText string
		if isCursor {
			labelText = activeStyle.Bold(true).Render(item.Label)
		} else {
			labelText = inactiveStyle.Render(item.Label)
		}

		// Manual padding for alignment: ANSI escape sequences shouldn't count towards width.
		const labelWidth = 40
		currentWidth := lipgloss.Width(labelText)
		padding := 0
		if currentWidth < labelWidth {
			padding = labelWidth - currentWidth
		}

		detailText := fadedStyle.Render(item.Detail)

		line := fmt.Sprintf(" %s %s %s%s  %s\n", icon, status, labelText, strings.Repeat(" ", padding), detailText)
		s.WriteString(line)
	}

	return s.String()
}

func (m *Picker) Selected() (*Item, bool) {
	return m.selected, m.selected != nil
}

func (m *Picker) CursorItem() (Item, bool) {
	if len(m.selectableIndices) == 0 {
		return Item{}, false
	}
	idx := m.selectableIndices[m.cursor]
	return m.items[idx], true
}

// RefreshItems allows the caller to update the list (e.g. after a rename or delete)
func (m *Picker) RefreshItems(items []Item) {
	m.items = items
	// Re-calculate selectable indices
	var indices []int
	for i := range items {
		indices = append(indices, i)
	}
	m.selectableIndices = indices

	// Adjust cursor if out of bounds
	if m.cursor >= len(m.selectableIndices) {
		m.cursor = len(m.selectableIndices) - 1
	}
	if m.cursor < 0 && len(m.selectableIndices) > 0 {
		m.cursor = 0
	}
}
