// Package tool provides rendering for tool outputs (StringDisplay, DiffDisplay, ShellDisplay).
// Used by compose when wiring engine.Deps.ViewTool.

package ui

import (
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/charmbracelet/lipgloss"
)

// Pad adds the status prefix to the first line and standard indentation to others.
func Pad(s string, prefix string) string {
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

func formatError(header string, err string, th *Theme) string {
	return fmt.Sprintf("%s — %s", header, th.Error(err))
}

// RenderString renders StringDisplay.
func RenderString(th *Theme, d domain.StringDisplay, status ToolStatus, err string, prefix string) string {
	s := d.Content
	if status == StatusError {
		s = formatError(s, err, th)
	}
	return Pad(s, prefix)
}

// RenderDiff renders DiffDisplay.
func RenderDiff(width, maxDiffHeight int, th *Theme, d domain.DiffDisplay, status ToolStatus, err string, prefix string) string {
	header := d.Comment
	target := d.Target
	if target == "" {
		target = header
	}

	if status == StatusError {
		header = formatError(header, err, th)
		header = th.Muted("# " + header)
		parts := []string{header, target}
		return Pad(strings.Join(parts, "\n\n"), prefix)
	}

	// Add stats to target if success
	if status == StatusSuccess && (d.Added != 0 || d.Removed != 0) {
		target = fmt.Sprintf("%s (%s, %s)",
			target,
			th.Success(fmt.Sprintf("+%d", d.Added)),
			th.Error(fmt.Sprintf("-%d", d.Removed)))
	}

	header = th.Muted("# " + header)
	diffContent := colorizeDiff(d.Diff, th)

	// Apply truncation if needed
	lines := strings.Split(diffContent, "\n")
	if len(lines) > maxDiffHeight && maxDiffHeight > 0 {
		overflow := len(lines) - maxDiffHeight
		visible := lines[overflow:]
		indicator := fmt.Sprintf("  ▲ [%d lines truncated]", overflow)
		diffContent = indicator + "\n" + strings.Join(visible, "\n")
	}

	// Build parts with blank line separation
	parts := []string{
		header,
		target,
		diffContent,
	}

	content := strings.Join(parts, "\n\n")
	return Pad(content, prefix)
}

func colorizeDiff(diff string, th *Theme) string {
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
func RenderShell(width, shellOutputHeight int, th *Theme, d domain.ShellDisplay, output string, status ToolStatus, err string, prefix string) string {
	header := d.Comment
	if status == StatusError {
		header = formatError(header, err, th)
		header = th.Muted("# " + header)
		cmdLine := fmt.Sprintf("$ %s", d.Command)
		content := strings.Join([]string{header, cmdLine}, "\n\n")
		return Pad(content, prefix)
	}

	header = th.Muted("# " + header)
	cmdLine := fmt.Sprintf("$ %s", d.Command)

	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	var visibleLines []string
	var indicator string
	if len(lines) > shellOutputHeight && shellOutputHeight > 0 {
		overflow := len(lines) - shellOutputHeight
		visibleLines = lines[overflow:]
		indicator = fmt.Sprintf("  ▲ [%d lines truncated]", overflow)
	} else {
		visibleLines = lines
	}
	shellOutput := strings.Join(visibleLines, "\n")
	if indicator != "" {
		shellOutput = indicator + "\n" + shellOutput
	}

	// Build parts with blank line separation
	parts := []string{
		header,
		cmdLine,
	}
	if shellOutput != "" {
		parts = append(parts, shellOutput)
	}

	content := strings.Join(parts, "\n\n")
	return Pad(content, prefix)
}
