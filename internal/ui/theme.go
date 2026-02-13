package ui

import (
	"strings"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/charmbracelet/lipgloss"
)

// Theme provides styling for the UI.
type Theme struct {
	// Private colors (from config)
	muted   lipgloss.AdaptiveColor
	primary lipgloss.AdaptiveColor
	success lipgloss.AdaptiveColor
	err     lipgloss.AdaptiveColor // renamed from 'error' to avoid shadowing

	// Private styles
	box     lipgloss.Style
	spinner lipgloss.Style
}

// NewTheme creates a theme from config. Exported for compose/engine deps.
func NewTheme(cfg config.UIConfig) *Theme {
	return newTheme(cfg)
}

func newTheme(cfg config.UIConfig) *Theme {
	t := &Theme{
		muted:   lipgloss.AdaptiveColor{Light: cfg.MutedColor.Light, Dark: cfg.MutedColor.Dark},
		primary: lipgloss.AdaptiveColor{Light: cfg.PrimaryColor.Light, Dark: cfg.PrimaryColor.Dark},
		success: lipgloss.AdaptiveColor{Light: cfg.SuccessColor.Light, Dark: cfg.SuccessColor.Dark},
		err:     lipgloss.AdaptiveColor{Light: cfg.ErrorColor.Light, Dark: cfg.ErrorColor.Dark},
	}

	t.box = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.muted).
		Padding(0, 0)
	t.spinner = lipgloss.NewStyle().Foreground(t.primary)

	return t
}

// Semantic API - callers say WHAT they mean, theme decides HOW it looks

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
	case StatusSuccess:
		return t.success
	case StatusError:
		return t.err
	default:
		return t.muted
	}
}

func (t *Theme) SpinnerStyle() lipgloss.Style {
	return t.spinner
}
