package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const groupLeftMargin = 2

// Item is a single selectable row.
type Item struct {
	ID     string
	Label  string
	Detail string
	Group  string
	Active bool
	Faded  bool
}

// Action defines a keybinding the caller can register.
type Action struct {
	Fn    func(item Item) tea.Cmd
	Key   string
	Label string
	Quit  bool
}

// Config configures the picker.
type Config struct {
	Title string
	Items []Item
	// Theme controls the visual styling of the picker.
	// If nil, a default style that roughly matches the global theme is used.
	Theme   *Theme
	Actions []Action
}

// Picker is a reusable grouped list selection model.
type Picker struct {
	theme             *Theme
	selected          *Item
	title             string
	items             []Item
	actions           []Action
	selectableIndices []int
	cursor            int
	quit              bool
}

// NewPicker creates a new Picker with the given configuration.
func NewPicker(cfg Config) *Picker {
	if cfg.Theme == nil {
		panic("ui.Picker: theme is required and must not be nil")
	}
	indices := make([]int, 0, len(cfg.Items))
	for i := range cfg.Items {
		indices = append(indices, i)
	}

	return &Picker{
		title:             cfg.Title,
		items:             cfg.Items,
		actions:           cfg.Actions,
		theme:             cfg.Theme,
		selectableIndices: indices,
	}
}

// Init initializes the picker model.
func (m *Picker) Init() tea.Cmd {
	return nil
}

// Update handles keyboard messages for navigation and selection.
func (m *Picker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
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

		case "enter", " ":
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
					if action.Quit {
						m.quit = true
						_ = action.Fn(item)
						return m, tea.Quit
					}
					return m, action.Fn(item)
				}
			}
		}
	}
	return m, nil
}

// View renders the picker as a string.
func (m *Picker) View() string {
	if m.quit {
		return ""
	}

	var s strings.Builder

	// Resolve colors from theme.
	primary := m.theme.PrimaryColor()
	muted := m.theme.MutedColor()
	active := m.theme.SuccessColor()
	text := m.theme.TextColor()

	// Styles
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(primary).MarginBottom(1)
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(primary)
	descStyle := lipgloss.NewStyle().Foreground(muted)
	blueColor := lipgloss.AdaptiveColor{Light: "#005FDF", Dark: "#38BDF8"}
	groupStyle := lipgloss.NewStyle().Bold(true).Foreground(blueColor).Margin(1, 0, 0, groupLeftMargin)

	activeStyle := lipgloss.NewStyle().Foreground(active)
	cursorStyle := lipgloss.NewStyle().Foreground(primary).Bold(true)
	inactiveStyle := lipgloss.NewStyle().Foreground(text)
	fadedStyle := lipgloss.NewStyle().Foreground(muted)

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

	helpRow := "  " + strings.Join(helpParts, "   ")
	s.WriteString(helpRow + "\n\n")

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
			if item.Faded {
				fadedGroupStyle := lipgloss.NewStyle().Italic(true).Foreground(muted).Margin(1, 0, 0, groupLeftMargin)
				s.WriteString(fadedGroupStyle.Render(displayGroup) + "\n")
			} else {
				s.WriteString(groupStyle.Render(displayGroup) + "\n")
			}
			lastGroup = item.Group
		}

		// Find if this specific item index is the one pointed at by m.cursor
		isCursor := false
		if m.cursor < len(m.selectableIndices) && m.selectableIndices[m.cursor] == i {
			isCursor = true
		}

		var icon string
		if isCursor {
			icon = cursorStyle.Render("●")
		} else {
			icon = " "
		}

		var labelText string
		if item.Active {
			labelText = activeStyle.Bold(true).Render(item.Label)
		} else if item.Faded {
			labelText = fadedStyle.Render(item.Label)
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

		var line string
		if isCursor {
			line = fmt.Sprintf("  %s  %s%s  %s\n", icon, labelText, strings.Repeat(" ", padding), detailText)
		} else {
			line = fmt.Sprintf("     %s%s  %s\n", labelText, strings.Repeat(" ", padding), detailText)
		}
		s.WriteString(line)
	}

	return s.String()
}

// Selected returns the item that was selected by the user.
func (m *Picker) Selected() (*Item, bool) {
	return m.selected, m.selected != nil
}

// CursorItem returns the item currently under the cursor.
func (m *Picker) CursorItem() (Item, bool) {
	if len(m.selectableIndices) == 0 {
		return Item{}, false
	}
	idx := m.selectableIndices[m.cursor]
	return m.items[idx], true
}

// RefreshItems allows the caller to update the list (e.g. after a rename or delete).
func (m *Picker) RefreshItems(items []Item) {
	m.items = items
	// Re-calculate selectable indices
	indices := make([]int, 0, len(items))
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
