// Package tool provides rendering for tool outputs (StringDisplay, DiffDisplay, ShellDisplay).
// Used by compose when wiring engine.Deps.ViewTool.

package tool

import (
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui/theme"
	"github.com/charmbracelet/lipgloss"
)

// Pad adds the status prefix to the first line and standard indentation to others.
func Pad(s string, prefix string) string {
	return pad(s, prefix)
}

func pad(s string, prefix string) string {
	lines := strings.Split(s, "\n")
	w := lipgloss.Width(prefix)
	if w == 0 {
		w = 1
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

func formatError(header string, err string, th *theme.Theme) string {
	return fmt.Sprintf("%s — %s", header, th.Error(err))
}

// RenderString renders StringDisplay.
func RenderString(th *theme.Theme, d domain.StringDisplay, status theme.ToolStatus, err string, prefix string) string {
	s := string(d)
	if status == theme.StatusError {
		s = formatError(s, err, th)
	}
	return pad(s, prefix)
}

// RenderDiff renders DiffDisplay.
func RenderDiff(width int, th *theme.Theme, d domain.DiffDisplay, status theme.ToolStatus, err string, prefix string) string {
	header := d.Header
	if status == theme.StatusSuccess && (d.Added != 0 || d.Removed != 0) {
		header = fmt.Sprintf("%s (%s, %s)",
			d.Header,
			th.Success(fmt.Sprintf("+%d", d.Added)),
			th.Error(fmt.Sprintf("-%d", d.Removed)))
	}

	if status == theme.StatusError {
		header = formatError(header, err, th)
		return fmt.Sprintf(" %s %s ", prefix, header)
	}

	diffContent := colorizeDiff(d.Diff, th)
	paddedDiff := pad(diffContent, "")
	sep := th.Separator(width, status)

	return fmt.Sprintf(" %s %s \n%s\n%s",
		prefix, header, sep, paddedDiff)
}

func colorizeDiff(diff string, th *theme.Theme) string {
	lines := strings.Split(diff, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "+") {
			lines[i] = th.Success(line)
		} else if strings.HasPrefix(line, "-") {
			lines[i] = th.Error(line)
		}
	}
	return strings.Join(lines, "\n")
}

// RenderShell renders ShellDisplay.
func RenderShell(width, shellOutputHeight int, th *theme.Theme, d domain.ShellDisplay, output string, status theme.ToolStatus, err string, prefix string) string {
	sep := th.Separator(width, status)
	header := d.Header
	cmdLine := pad(fmt.Sprintf("$ %s", d.Command), "")
	if status == theme.StatusError {
		header = formatError(header, err, th)
		return fmt.Sprintf(" %s %s \n%s\n%s", prefix, header, sep, cmdLine)
	}

	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	var visibleLines []string
	if len(lines) > shellOutputHeight {
		visibleLines = lines[len(lines)-shellOutputHeight:]
	} else {
		visibleLines = lines
	}

	content := strings.Join(visibleLines, "\n")
	paddedContent := pad(content, "")

	return fmt.Sprintf(" %s %s \n%s\n%s\n%s\n%s",
		prefix,
		header,
		sep,
		cmdLine,
		sep,
		paddedContent)
}
