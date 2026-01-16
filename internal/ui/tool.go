package ui

import (
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
)

const indent = "" // No indentation

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
		content = m.viewStringDisplay(d, t)
	case domain.DiffDisplay:
		content = m.viewDiffDisplay(d, t)
	case domain.ShellDisplay:
		content = m.viewShellDisplay(d, t)
	default:
		content = fmt.Sprintf("Unknown display type: %T", d)
	}

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if i == 0 {
			// First line gets prefix and padding
			lines[i] = fmt.Sprintf(" %s %s ", prefix, line)
		} else if strings.Contains(line, "─") {
			// Separator line - no horizontal padding to touch borders
			continue
		} else {
			// Regular content line - manual padding (3 spaces to align with prefix+space)
			lines[i] = "   " + line
		}
	}

	return m.theme.box.Width(boxWidth).Render(strings.Join(lines, "\n"))
}

func (m *model) viewStringDisplay(d domain.StringDisplay, t *toolState) string {
	s := string(d)
	if t.status == statusError {
		sep := m.theme.separator.Render(strings.Repeat("─", boxWidth))
		return fmt.Sprintf("%s\n%s\n%s", s, sep, m.theme.errorSt.Render(t.err))
	}
	return s
}
