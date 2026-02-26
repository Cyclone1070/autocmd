// Package theme provides styling and status types for the UI.
// Theme and ToolStatus are used by tool display, status bar, and compose wiring.

package ui

import (
	"strings"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/charmbracelet/lipgloss"
)

// ToolStatus represents tool lifecycle state for display rendering.
type ToolStatus int

const (
	StatusRunning ToolStatus = iota
	StatusSuccess
	StatusError
)

// Theme provides styling for the UI.
type Theme struct {
	muted   lipgloss.AdaptiveColor
	primary lipgloss.AdaptiveColor
	success lipgloss.AdaptiveColor
	err     lipgloss.AdaptiveColor

	box lipgloss.Style
}

// NewTheme creates a theme from config.
func NewTheme(cfg config.UIConfig) *Theme {
	t := &Theme{
		muted:   lipgloss.AdaptiveColor{Light: cfg.MutedColor.Light, Dark: cfg.MutedColor.Dark},
		primary: lipgloss.AdaptiveColor{Light: cfg.PrimaryColor.Light, Dark: cfg.PrimaryColor.Dark},
		success: lipgloss.AdaptiveColor{Light: cfg.SuccessColor.Light, Dark: cfg.SuccessColor.Dark},
		err:     lipgloss.AdaptiveColor{Light: cfg.ErrorColor.Light, Dark: cfg.ErrorColor.Dark},
	}

	t.box = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.muted).
		Padding(0, 0).
		MarginTop(1).
		MarginBottom(1)

	return t
}

func (t *Theme) Success(s string) string {
	return lipgloss.NewStyle().Foreground(t.success).Render(s)
}

func (t *Theme) Error(s string) string {
	return lipgloss.NewStyle().Foreground(t.err).Render(s)
}

func (t *Theme) Muted(s string) string {
	return lipgloss.NewStyle().Foreground(t.muted).Render(s)
}

func (t *Theme) Primary(s string) string {
	return lipgloss.NewStyle().Foreground(t.primary).Render(s)
}

func (t *Theme) Separator(width int, status ToolStatus) string {
	color := t.colorForStatus(status)
	return lipgloss.NewStyle().Foreground(color).Render(strings.Repeat("─", width))
}

func (t *Theme) Box(content string, width int, status ToolStatus) string {
	borderColor := t.colorForStatus(status)
	return t.box.BorderForeground(borderColor).Width(width).Render(content)
}

func (t *Theme) colorForStatus(status ToolStatus) lipgloss.AdaptiveColor {
	switch status {
	case StatusRunning:
		return t.primary
	case StatusSuccess:
		return t.success
	case StatusError:
		return t.err
	default:
		return t.muted
	}
}

func (t *Theme) PrimaryColor() lipgloss.AdaptiveColor { return t.primary }
func (t *Theme) SuccessColor() lipgloss.AdaptiveColor { return t.success }
func (t *Theme) ErrorColor() lipgloss.AdaptiveColor   { return t.err }
func (t *Theme) MutedColor() lipgloss.AdaptiveColor   { return t.muted }
