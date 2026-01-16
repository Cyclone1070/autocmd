package ui

import (
	"github.com/Cyclone1070/iav/internal/config"
	"github.com/charmbracelet/lipgloss"
)

type theme struct {
	// Colors
	muted   lipgloss.AdaptiveColor
	primary lipgloss.AdaptiveColor
	success lipgloss.AdaptiveColor
	error   lipgloss.AdaptiveColor

	// Styles
	base      lipgloss.Style
	toolName  lipgloss.Style
	spinner   lipgloss.Style
	box       lipgloss.Style
	successSt lipgloss.Style
	errorSt   lipgloss.Style
	indent    lipgloss.Style
	separator lipgloss.Style
}

func newTheme(cfg config.UIConfig) *theme {
	t := &theme{
		muted:   lipgloss.AdaptiveColor{Light: cfg.MutedColor.Light, Dark: cfg.MutedColor.Dark},
		primary: lipgloss.AdaptiveColor{Light: cfg.PrimaryColor.Light, Dark: cfg.PrimaryColor.Dark},
		success: lipgloss.AdaptiveColor{Light: cfg.SuccessColor.Light, Dark: cfg.SuccessColor.Dark},
		error:   lipgloss.AdaptiveColor{Light: cfg.ErrorColor.Light, Dark: cfg.ErrorColor.Dark},
	}

	t.base = lipgloss.NewStyle().Foreground(lipgloss.Color("#FAFAFA"))
	t.toolName = lipgloss.NewStyle().Foreground(t.muted)
	t.spinner = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	t.box = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.muted).
		Padding(0, 0)
	t.successSt = lipgloss.NewStyle().Foreground(t.success)
	t.errorSt = lipgloss.NewStyle().Foreground(t.error)
	t.indent = lipgloss.NewStyle().PaddingLeft(2)
	t.separator = lipgloss.NewStyle().Foreground(t.muted)

	return t
}

const (
	boxWidth = 62
)
