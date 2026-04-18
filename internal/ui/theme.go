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
	StatusRunning ToolStatus = iota
	StatusSuccess
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

// ThemeConfig holds the colors and styling needed for the theme.
type ThemeConfig struct {
	PrimaryColor lipgloss.AdaptiveColor
	SuccessColor lipgloss.AdaptiveColor
	ErrorColor   lipgloss.AdaptiveColor
	MutedColor   lipgloss.AdaptiveColor
	ShortToolbox bool
}

// Theme provides styling for the UI.
type Theme struct {
	muted   lipgloss.AdaptiveColor
	primary lipgloss.AdaptiveColor
	success lipgloss.AdaptiveColor
	err     lipgloss.AdaptiveColor

	box          lipgloss.Style
	ShortToolbox bool
}

// NewTheme creates a theme from config.
func NewTheme(cfg ThemeConfig) *Theme {
	return &Theme{
		muted:        cfg.MutedColor,
		primary:      cfg.PrimaryColor,
		success:      cfg.SuccessColor,
		err:          cfg.ErrorColor,
		ShortToolbox: cfg.ShortToolbox,
		box: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(cfg.MutedColor).
			Padding(0, 1). // Add horizontal padding for breathing room
			MarginTop(0).
			MarginBottom(0),
	}
}

// StatusPrefix returns a styled icon with a trailing space.
func (t *Theme) StatusPrefix(status ToolStatus, frame string) string {
	switch status {
	case StatusRunning:
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
	return "\n\n" + t.box.BorderForeground(borderColor).Width(width).Render(content)
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
