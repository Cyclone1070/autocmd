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

func (r *ToolRenderer) formatError(prefix string, err string, isMuted bool) string {
	if prefix == "" {
		return r.Theme.Error(err)
	}
	separator := " - "
	if isMuted {
		return r.Theme.Muted(prefix+separator) + r.Theme.Error(err)
	}
	return prefix + separator + r.Theme.Error(err)
}

// RenderString renders StringDisplay.
func (r *ToolRenderer) RenderString(d domain.StringDisplay, status ToolStatus, err string, prefix string) string {
	var parts []string
	contentLines := strings.Split(d.Content, "\n")

	// 1. Header (Comment or first line of Content)
	if d.Comment != "" {
		header := "# " + d.Comment
		if status == StatusError && err != "" {
			header = r.formatError(header, err, true)
		} else {
			header = r.Theme.Muted(header)
		}
		parts = append(parts, header)
		// Body: all of content
		if d.Content != "" {
			parts = append(parts, d.Content)
		}
	} else if d.Content != "" {
		// No comment, error label goes on the first line of content
		firstLine := contentLines[0]
		if status == StatusError && err != "" {
			firstLine = r.formatError(firstLine, err, false)
		}
		parts = append(parts, firstLine)
		// Body: remaining lines
		if len(contentLines) > 1 {
			parts = append(parts, strings.Join(contentLines[1:], "\n"))
		}
	} else if status == StatusError && err != "" {
		// No comment AND no content (edge case), just show the error
		parts = append(parts, r.Theme.Error(err))
	}

	if len(parts) == 0 {
		return ""
	}

	// Join parts with a blank line ONLY IF we have both a comment and content
	// or if we have multi-line content following a labeled first line.
	// Actually, standard behavior is a blank line between header and content.
	content := strings.Join(parts, "\n\n")
	return r.Pad(r.gater.Gate(content), prefix)
}

// RenderDiff renders DiffDisplay.
func (r *ToolRenderer) RenderDiff(d domain.DiffDisplay, status ToolStatus, err string, prefix string) string {
	header := d.Comment
	target := d.Target
	if target == "" {
		target = header
	}

	if status == StatusError {
		header = r.formatError("# "+header, err, true)
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
		header = r.formatError("# "+header, err, true)
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

// RenderErrorLine returns a themed failure message with a status prefix.
func (r *ToolRenderer) RenderErrorLine(s string) string {
	return "\n " + r.Theme.StatusPrefix(StatusError, "✘") + r.Theme.Error(s)
}
