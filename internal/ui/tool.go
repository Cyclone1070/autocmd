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
		content = renderString(m.width, m.theme, d, t.status, t.err, prefix)
	case domain.DiffDisplay:
		content = renderDiff(m.width, m.theme, d, t.status, t.err, prefix)
	case domain.ShellDisplay:
		content = renderShell(m.width, m.theme, d, t.shellOutput.String(), t.status, t.err, prefix)
	default:
		content = pad(fmt.Sprintf("Unknown display type: %T", d), prefix)
	}

	return m.theme.box.Width(m.width).Render(content)
}

// pad adds the status prefix to the first line and standard indentation to others.
func pad(s string, prefix string) string {
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

func renderString(width int, theme *theme, d domain.StringDisplay, status toolStatus, err string, prefix string) string {
	s := string(d)
	if status == statusError {
		sep := theme.separator.Render(strings.Repeat("\u2500", width))
		return pad(fmt.Sprintf("%s\n%s\n%s", s, sep, theme.errorSt.Render(err)), prefix)
	}
	return pad(s, prefix)
}
