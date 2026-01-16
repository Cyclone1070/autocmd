package ui

import (
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
)

// No global indent constant needed, handled by pad helper

func (m *model) viewTool(t *toolState) string {
	// Status prefix
	var prefix string
	switch t.status {
	case statusRunning:
		prefix = m.spinner.View()
	case statusSuccess:
		prefix = m.theme.successSt.Render("✓")
	case statusError:
		prefix = m.theme.errorSt.Render("✗")
	}

	// Content
	var content string
	switch d := t.display.(type) {
	case domain.StringDisplay:
		content = m.viewStringDisplay(d, t, prefix)
	case domain.DiffDisplay:
		content = m.viewDiffDisplay(d, t, prefix)
	case domain.ShellDisplay:
		content = m.viewShellDisplay(d, t, prefix)
	default:
		content = m.pad(fmt.Sprintf("Unknown display type: %T", d), prefix)
	}

	return m.theme.box.Width(m.width).Render(content)
}

// pad adds the status prefix to the first line and standard indentation to others.
func (m *model) pad(s string, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if i == 0 && prefix != "" {
			lines[i] = fmt.Sprintf(" %s %s ", prefix, line)
		} else {
			lines[i] = "   " + line
		}
	}
	return strings.Join(lines, "\n")
}

func (m *model) viewStringDisplay(d domain.StringDisplay, t *toolState, prefix string) string {
	s := string(d)
	if t.status == statusError {
		sep := m.theme.separator.Render(strings.Repeat("─", m.width-2)) // -2 for borders
		return m.pad(fmt.Sprintf("%s\n%s\n%s", s, sep, m.theme.errorSt.Render(t.err)), prefix)
	}
	return m.pad(s, prefix)
}
