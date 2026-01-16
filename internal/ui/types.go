package ui

import (
	"io"

	"github.com/Cyclone1070/iav/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

// Msg wraps domain.Event for Bubble Tea
type Msg struct {
	Event domain.Event
}

// Renderer is the main entry point for the UI.
type Renderer struct {
	program *tea.Program
	output  io.Writer
}
