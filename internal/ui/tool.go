package ui

import (
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/charmbracelet/lipgloss"
)

func (m *model) viewTool(t *toolState) string {
	// Status prefix
	var prefix string
	switch t.status {
	case statusRunning:
		prefix = m.spinner.View()
	case statusSuccess:
		prefix = m.theme.Success("✓")
	case statusError:
		prefix = m.theme.Error("✗")
	}

	// Content width = m.width - 2 (for left/right box borders)
	// This ensures total visual width = m.width
	contentWidth := m.width - 2

	// Content
	var content string
	switch d := t.display.(type) {
	case domain.StringDisplay:
		content = renderString(m.theme, d, t.status, t.err, prefix)
	case domain.DiffDisplay:
		content = renderDiff(contentWidth, m.theme, d, t.status, t.err, prefix)
	case domain.ShellDisplay:
		content = renderShell(contentWidth, m.theme, d, t.shellOutput.String(), t.status, t.err, prefix)
	default:
		content = pad(fmt.Sprintf("Unknown display type: %T", d), prefix)
	}

	return m.theme.Box(content, contentWidth, t.status)
}

// pad adds the status prefix to the first line and standard indentation to others.
func pad(s string, prefix string) string {
	lines := strings.Split(s, "\n")

	// Dynamic indentation: Space + Prefix + Space
	// Standard width for prefix (like spinner) is 1.
	w := lipgloss.Width(prefix)
	if w == 0 {
		w = 1 // Default to 1 char width (like space) for alignment if empty
	}
	indent := strings.Repeat(" ", 1+w+1)

	for i, line := range lines {
		if i == 0 && prefix != "" {
			lines[i] = fmt.Sprintf(" %s %s ", prefix, line)
		} else {
			lines[i] = indent + line
		}
	}
	return strings.Join(lines, "\n")
}

func renderString(theme *theme, d domain.StringDisplay, status toolStatus, err string, prefix string) string {
	s := string(d)
	if status == statusError {
		s = formatError(s, err, theme)
	}
	return pad(s, prefix)
}

func formatError(header string, err string, theme *theme) string {
	return fmt.Sprintf("%s — %s", header, theme.Error(err))
}
