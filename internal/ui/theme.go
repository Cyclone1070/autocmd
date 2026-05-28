// Package theme provides styling and status types for the UI.
// Theme and ToolStatus are used by tool display, status bar, and compose wiring.

package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ToolStatus represents tool lifecycle state for display rendering.
type ToolStatus int

const (
	// StatusRunning indicates that a tool or process is currently active.
	StatusRunning ToolStatus = iota
	// StatusAwaitingApproval indicates that a tool is blocked waiting for user consent.
	StatusAwaitingApproval
	// StatusSuccess indicates that a tool or process completed successfully.
	StatusSuccess
	// StatusError indicates that a tool or process failed.
	StatusError
)

// ColorInfo matches the methods provided by config.ColorConfig.
type ColorInfo interface {
	Light() string
	Dark() string
}

// ToAdaptiveColor converts a ColorInfo (e.g. from config) to lipgloss.AdaptiveColor.
func ToAdaptiveColor(c ColorInfo) lipgloss.AdaptiveColor {
	return lipgloss.AdaptiveColor{Light: c.Light(), Dark: c.Dark()}
}

// Theme provides styling and configuration for the UI.
type Theme struct {
	PrimaryCol     lipgloss.AdaptiveColor
	SuccessCol     lipgloss.AdaptiveColor
	ErrorCol       lipgloss.AdaptiveColor
	MutedCol       lipgloss.AdaptiveColor
	TextCol        lipgloss.AdaptiveColor
	ShortToolBlock bool
}

// StatusPrefix returns a styled icon with a trailing space.
func (t *Theme) StatusPrefix(status ToolStatus, frame string) string {
	switch status {
	case StatusRunning, StatusAwaitingApproval:
		return t.Primary(frame) + " "
	case StatusSuccess:
		return t.Success("✔") + " "
	case StatusError:
		return t.Error("✘") + " "
	default:
		return t.Muted("○") + " "
	}
}

func (t *Theme) Success(s string) string {
	if s == "" {
		return ""
	}
	return lipgloss.NewStyle().Foreground(t.SuccessCol).Render(s)
}

func (t *Theme) Error(s string) string {
	if s == "" {
		return ""
	}
	return lipgloss.NewStyle().Foreground(t.ErrorCol).Render(s)
}

// Muted styles a string with the theme's muted color.
func (t *Theme) Muted(s string) string {
	if s == "" {
		return ""
	}
	return lipgloss.NewStyle().Foreground(t.MutedCol).Render(s)
}

// Primary styles a string with the theme's primary color.
func (t *Theme) Primary(s string) string {
	if s == "" {
		return ""
	}
	return lipgloss.NewStyle().Foreground(t.PrimaryCol).Render(s)
}

// Separator returns a styled horizontal line.
func (t *Theme) Separator(width int, status ToolStatus) string {
	color := t.colorForStatus(status)
	return lipgloss.NewStyle().Foreground(color).Render(strings.Repeat("─", width))
}

// RenderActionBlock renders an action execution block using status markers and colors.
func (t *Theme) RenderActionBlock(spec ActionBlockSpec) string {
	header := trimEmptyLines(spec.HeaderLines)
	content := trimEmptyLines(spec.ContentLines)
	footer := trimEmptyLines(spec.FooterLines)
	if len(header) == 0 && len(content) == 0 && len(footer) == 0 {
		return ""
	}

	prefix := t.StatusPrefix(spec.Status, spec.Frame)
	headerContinuationPrefix := ToolInsetPrefix + strings.Repeat(" ", lipgloss.Width(prefix))
	if len(header) > 0 {
		header[0] = ToolInsetPrefix + prefix + header[0]
		for i := 1; i < len(header); i++ {
			header[i] = headerContinuationPrefix + header[i]
		}
	} else {
		// Do not promote content into the header. Empty header stays empty.
		header = []string{ToolInsetPrefix + prefix}
	}

	var out []string
	out = append(out, header...)
	for i, line := range content {
		if i == 0 {
			out = append(out, t.Muted(ToolInsetPrefix+ToolFirstContentGutterPrefix)+line)
			continue
		}
		out = append(out, t.Muted(ToolInsetPrefix+ToolContentGutterPrefix)+line)
	}
	if len(footer) > 0 {
		out = append(out, "")
		for _, line := range footer {
			out = append(out, headerContinuationPrefix+line)
		}
	}

	return "\n\n" + strings.Join(out, "\n")
}

// ActionBlock renders a simple action block with header and content.
func (t *Theme) ActionBlock(content string, status ToolStatus, _ string) string {
	content = strings.Trim(content, "\n")
	if content == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	spec := ActionBlockSpec{
		Status: status,
	}
	if len(lines) > 0 {
		spec.HeaderLines = []string{lines[0]}
	}
	if len(lines) > 1 {
		spec.ContentLines = lines[1:]
	}
	return t.RenderActionBlock(spec)
}

// Box exists for compatibility while tests migrate from box-style assertions.
func (t *Theme) Box(content string, _ int, status ToolStatus) string {
	return t.ActionBlock(content, status, "")
}

func (t *Theme) colorForStatus(status ToolStatus) lipgloss.AdaptiveColor {
	switch status {
	case StatusRunning, StatusAwaitingApproval:
		return t.PrimaryCol
	case StatusSuccess:
		return t.SuccessCol
	case StatusError:
		return t.ErrorCol
	default:
		return t.MutedCol
	}
}

func trimEmptyLines(lines []string) []string {
	out := append([]string(nil), lines...)
	for len(out) > 0 && strings.TrimSpace(out[0]) == "" {
		out = out[1:]
	}
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	return out
}

// PrimaryColor returns the theme's primary adaptive color.
func (t *Theme) PrimaryColor() lipgloss.AdaptiveColor { return t.PrimaryCol }

// SuccessColor returns the theme's success adaptive color.
func (t *Theme) SuccessColor() lipgloss.AdaptiveColor { return t.SuccessCol }

// ErrorColor returns the theme's error adaptive color.
func (t *Theme) ErrorColor() lipgloss.AdaptiveColor { return t.ErrorCol }

// MutedColor returns the theme's muted adaptive color.
func (t *Theme) MutedColor() lipgloss.AdaptiveColor { return t.MutedCol }

// TextColor returns the theme's text adaptive color.
func (t *Theme) TextColor() lipgloss.AdaptiveColor { return t.TextCol }
