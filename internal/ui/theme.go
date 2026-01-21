package ui

import (
	"strings"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/charmbracelet/lipgloss"
)

type theme struct {
	// Private colors (from config)
	muted   lipgloss.AdaptiveColor
	primary lipgloss.AdaptiveColor
	success lipgloss.AdaptiveColor
	err     lipgloss.AdaptiveColor // renamed from 'error' to avoid shadowing

	// Private styles
	box     lipgloss.Style
	spinner lipgloss.Style
}

func newTheme(cfg config.UIConfig) *theme {
	t := &theme{
		muted:   lipgloss.AdaptiveColor{Light: cfg.MutedColor.Light, Dark: cfg.MutedColor.Dark},
		primary: lipgloss.AdaptiveColor{Light: cfg.PrimaryColor.Light, Dark: cfg.PrimaryColor.Dark},
		success: lipgloss.AdaptiveColor{Light: cfg.SuccessColor.Light, Dark: cfg.SuccessColor.Dark},
		err:     lipgloss.AdaptiveColor{Light: cfg.ErrorColor.Light, Dark: cfg.ErrorColor.Dark},
	}

	t.box = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.muted).
		Padding(0, 0)
	t.spinner = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	return t
}

// Semantic API - callers say WHAT they mean, theme decides HOW it looks

func (t *theme) Success(s string) string {
	return lipgloss.NewStyle().Foreground(t.success).Render(s)
}

func (t *theme) Error(s string) string {
	return lipgloss.NewStyle().Foreground(t.err).Render(s)
}

func (t *theme) Muted(s string) string {
	return lipgloss.NewStyle().Foreground(t.muted).Render(s)
}

func (t *theme) Primary(s string) string {
	return lipgloss.NewStyle().Foreground(t.primary).Render(s)
}

func (t *theme) Separator(width int, status toolStatus) string {
	color := t.colorForStatus(status)
	return lipgloss.NewStyle().Foreground(color).Render(strings.Repeat("─", width))
}

func (t *theme) Box(content string, width int, status toolStatus) string {
	borderColor := t.colorForStatus(status)
	return t.box.BorderForeground(borderColor).Width(width).Render(content)
}

func (t *theme) colorForStatus(status toolStatus) lipgloss.AdaptiveColor {
	switch status {
	case statusSuccess:
		return t.success
	case statusError:
		return t.err
	default:
		return t.muted
	}
}

func (t *theme) SpinnerStyle() lipgloss.Style {
	return t.spinner
}
