// Package tool provides rendering for tool outputs (StringDisplay, DiffDisplay, ShellDisplay).
// Used by compose when wiring engine.Deps.ViewTool.

package ui

import (
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/charmbracelet/lipgloss"
)

type gater interface {
	Gate(content string) string
}

// ToolRenderer provides rendering for tool outputs (StringDisplay, DiffDisplay, ShellDisplay).
type ToolRenderer struct {
	Theme *Theme
	Width int
	gater gater
}

// NewToolRenderer creates a new ToolRenderer.
func NewToolRenderer(theme *Theme, width int, g gater) *ToolRenderer {
	return &ToolRenderer{
		Theme: theme,
		Width: width,
		gater: g,
	}
}

func (r *ToolRenderer) SetShortToolbox(b bool) {
	r.Theme.ShortToolbox = b
}

// StatusPrefix returns a styled and padded status indicator.
func (r *ToolRenderer) StatusPrefix(status ToolStatus, frame string) string {
	return r.Theme.StatusPrefix(status, frame)
}

// Pad adds the status prefix to the first line and standard indentation to others.
func (r *ToolRenderer) Pad(text, prefix string) string {
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	indent := strings.Repeat(" ", lipgloss.Width(prefix))
	for i := range lines {
		if i == 0 {
			lines[i] = prefix + lines[i]
		} else {
			lines[i] = indent + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}

func (r *ToolRenderer) formatError(header string, err string) string {
	return fmt.Sprintf("%s — %s", header, r.Theme.Error(err))
}

// RenderString renders StringDisplay.
func (r *ToolRenderer) RenderString(d domain.StringDisplay, status ToolStatus, err string, prefix string) string {
	s := d.Content
	if status == StatusError {
		s = r.formatError(s, err)
	}
	return r.gater.Gate(r.Pad(s, prefix))
}

// RenderDiff renders DiffDisplay.
func (r *ToolRenderer) RenderDiff(d domain.DiffDisplay, status ToolStatus, err string, prefix string) string {
	header := d.Comment
	target := d.Target
	if target == "" {
		target = header
	}

	if status == StatusError {
		header = r.formatError(header, err)
		header = r.Theme.Muted("# " + header)
		parts := []string{header, target}
		return r.Pad(strings.Join(parts, "\n\n"), prefix)
	}

	// Add stats to target if success
	if status == StatusSuccess && (d.Added != 0 || d.Removed != 0) {
		target = fmt.Sprintf("%s (%s, %s)",
			target,
			r.Theme.Success(fmt.Sprintf("+%d", d.Added)),
			r.Theme.Error(fmt.Sprintf("-%d", d.Removed)))
	}

	header = r.Theme.Muted("# " + header)
	diffContent := r.colorizeDiff(d.Diff)
	diffContent = r.gater.Gate(diffContent)

	// Build parts with blank line separation
	parts := []string{
		header,
		target,
	}
	if !r.Theme.ShortToolbox {
		parts = append(parts, diffContent)
	}

	content := strings.Join(parts, "\n\n")
	return r.Pad(content, prefix)
}

func (r *ToolRenderer) colorizeDiff(diff string) string {
	lines := strings.Split(diff, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "+") {
			lines[i] = r.Theme.Success(line)
		} else if strings.HasPrefix(line, "-") {
			lines[i] = r.Theme.Error(line)
		}
	}
	return strings.Join(lines, "\n")
}

// RenderShell renders ShellDisplay.
func (r *ToolRenderer) RenderShell(d domain.ShellDisplay, output string, status ToolStatus, err string, prefix string) string {
	header := d.Comment
	if status == StatusError {
		header = r.formatError(header, err)
		header = r.Theme.Muted("# " + header)
		cmdLine := fmt.Sprintf("$ %s", d.Command)
		content := strings.Join([]string{header, cmdLine}, "\n\n")
		return r.Pad(content, prefix)
	}

	header = r.Theme.Muted("# " + header)
	cmdLine := fmt.Sprintf("$ %s", d.Command)

	shellOutput := r.gater.Gate(strings.TrimRight(output, "\n"))

	// Build parts with blank line separation
	parts := []string{
		header,
		cmdLine,
	}
	if shellOutput != "" && !r.Theme.ShortToolbox {
		parts = append(parts, shellOutput)
	}

	content := strings.Join(parts, "\n\n")
	return r.Pad(content, prefix)
}

// Box wraps content in a themed box.
func (r *ToolRenderer) Box(content string, width int, status ToolStatus) string {
	return r.Theme.Box(content, width, status)
}
