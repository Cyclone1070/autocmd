package ui

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	// Colors
	subtle     = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#888888"}
	highlight  = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}
	special    = lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#73F59F"}
	errorColor = lipgloss.AdaptiveColor{Light: "#F05D5E", Dark: "#FF6666"}

	// Text Styles
	baseStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FAFAFA"))

	// Tool specific styles
	toolNameStyle = lipgloss.NewStyle().Foreground(subtle)

	// Spinner
	spinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	// Boxes
	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(subtle).
			Padding(0, 0)

	// Status Styles
	specialStyle = lipgloss.NewStyle().Foreground(special)
	errorStyle   = lipgloss.NewStyle().Foreground(errorColor)

	// Indentation
	indentStyle = lipgloss.NewStyle().PaddingLeft(2) // 1 tab approx

	// Separator
	separatorStyle = lipgloss.NewStyle().Foreground(subtle)
)

const (
	boxWidth = 62
)
