package ui

import (
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/charmbracelet/lipgloss"
)

// viewTool renders a tool display box with status indicator.
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
		content = renderShell(contentWidth, m.config.UI.ShellOutputHeight, m.theme, d, t.shellOutput.String(), t.status, t.err, prefix)
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

func formatError(header string, err string, theme *theme) string {
	return fmt.Sprintf("%s — %s", header, theme.Error(err))
}

// --- StringDisplay ---

func renderString(theme *theme, d domain.StringDisplay, status toolStatus, err string, prefix string) string {
	s := string(d)
	if status == statusError {
		s = formatError(s, err, theme)
	}
	return pad(s, prefix)
}

// --- DiffDisplay ---

func renderDiff(width int, theme *theme, d domain.DiffDisplay, status toolStatus, err string, prefix string) string {
	header := d.Header
	// Only show stats on success
	if status == statusSuccess && (d.Added != 0 || d.Removed != 0) {
		header = fmt.Sprintf("%s (%s, %s)",
			d.Header,
			theme.Success(fmt.Sprintf("+%d", d.Added)),
			theme.Error(fmt.Sprintf("-%d", d.Removed)))
	}

	if status == statusError {
		header = formatError(header, err, theme)
		return fmt.Sprintf(" %s %s ", prefix, header)
	}

	diffContent := colorizeDiff(d.Diff, theme)
	paddedDiff := pad(diffContent, "") // Just indent, no prefix for diff body
	sep := theme.Separator(width, status)

	return fmt.Sprintf(" %s %s \n%s\n%s",
		prefix, header, sep, paddedDiff)
}

func colorizeDiff(diff string, theme *theme) string {
	lines := strings.Split(diff, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "+") {
			lines[i] = theme.Success(line)
		} else if strings.HasPrefix(line, "-") {
			lines[i] = theme.Error(line)
		}
	}
	return strings.Join(lines, "\n")
}

// --- ShellDisplay ---

func renderShell(width int, shellOutputHeight int, theme *theme, d domain.ShellDisplay, output string, status toolStatus, err string, prefix string) string {
	sep := theme.Separator(width, status)
	header := d.Header
	cmdLine := pad(fmt.Sprintf("$ %s", d.Command), "")
	if status == statusError {
		header = formatError(header, err, theme)
		// Early return on error - show header and command, hide output
		return fmt.Sprintf(" %s %s \n%s\n%s", prefix, header, sep, cmdLine)
	}

	// Output
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")

	// Calculate visible window using configurable height
	var visibleLines []string
	if len(lines) > shellOutputHeight {
		visibleLines = lines[len(lines)-shellOutputHeight:]
	} else {
		visibleLines = lines
	}

	content := strings.Join(visibleLines, "\n")
	paddedContent := pad(content, "") // Indent lines, no prefix

	return fmt.Sprintf(" %s %s \n%s\n%s\n%s\n%s",
		prefix,
		header,
		sep,
		cmdLine,
		sep,
		paddedContent)
}
