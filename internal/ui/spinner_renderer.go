package ui

import "github.com/charmbracelet/lipgloss"

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// SpinnerRenderer handles braille spinner animations.
type SpinnerRenderer struct {
	Style lipgloss.Style
}

// NewSpinnerRenderer creates a new SpinnerRenderer.
func NewSpinnerRenderer(style lipgloss.Style) *SpinnerRenderer {
	return &SpinnerRenderer{Style: style}
}

// Frame returns the spinner character for a given tick.
func (r *SpinnerRenderer) Frame(tick int) string {
	return r.Style.Render(spinnerFrames[tick%len(spinnerFrames)])
}
