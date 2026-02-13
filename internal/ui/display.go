package ui

import (
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/charmbracelet/lipgloss"
)

// Pad adds the status prefix to the first line and standard indentation to others.
// Exported for compose/engine deps.
func Pad(s string, prefix string) string {
	return pad(s, prefix)
}

// pad is the internal implementation.
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

func formatError(header string, err string, theme *Theme) string {
	return fmt.Sprintf("%s — %s", header, theme.Error(err))
}

// --- StringDisplay ---

// RenderString renders StringDisplay for compose/engine deps.
func RenderString(theme *Theme, d domain.StringDisplay, status ToolStatus, err string, prefix string) string {
	return renderString(theme, d, status, err, prefix)
}

func renderString(theme *Theme, d domain.StringDisplay, status ToolStatus, err string, prefix string) string {
	s := string(d)
	if status == StatusError {
		s = formatError(s, err, theme)
	}
	return pad(s, prefix)
}

// --- DiffDisplay ---

// RenderDiff renders DiffDisplay for compose/engine deps.
func RenderDiff(width int, theme *Theme, d domain.DiffDisplay, status ToolStatus, err string, prefix string) string {
	return renderDiff(width, theme, d, status, err, prefix)
}

func renderDiff(width int, theme *Theme, d domain.DiffDisplay, status ToolStatus, err string, prefix string) string {
	header := d.Header
	// Only show stats on success
	if status == StatusSuccess && (d.Added != 0 || d.Removed != 0) {
		header = fmt.Sprintf("%s (%s, %s)",
			d.Header,
			theme.Success(fmt.Sprintf("+%d", d.Added)),
			theme.Error(fmt.Sprintf("-%d", d.Removed)))
	}

	if status == StatusError {
		header = formatError(header, err, theme)
		return fmt.Sprintf(" %s %s ", prefix, header)
	}

	diffContent := colorizeDiff(d.Diff, theme)
	paddedDiff := pad(diffContent, "") // Just indent, no prefix for diff body
	sep := theme.Separator(width, status)

	return fmt.Sprintf(" %s %s \n%s\n%s",
		prefix, header, sep, paddedDiff)
}

func colorizeDiff(diff string, theme *Theme) string {
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

// RenderShell renders ShellDisplay for compose/engine deps.
func RenderShell(width, shellOutputHeight int, theme *Theme, d domain.ShellDisplay, output string, status ToolStatus, err string, prefix string) string {
	return renderShell(width, shellOutputHeight, theme, d, output, status, err, prefix)
}

func renderShell(width int, shellOutputHeight int, theme *Theme, d domain.ShellDisplay, output string, status ToolStatus, err string, prefix string) string {
	sep := theme.Separator(width, status)
	header := d.Header
	cmdLine := pad(fmt.Sprintf("$ %s", d.Command), "")
	if status == StatusError {
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
