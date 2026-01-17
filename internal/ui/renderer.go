package ui

import (
	"io"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

// msg wraps domain.Event for Bubble Tea
type msg struct {
	Event domain.Event
}

// Renderer is the main entry point for the UI.
type Renderer struct {
	program *tea.Program
}

// NewRenderer creates a new Renderer writing to the given output.
func NewRenderer(output io.Writer, cfg *config.Config) (*Renderer, error) {
	m, err := newModel(cfg)
	if err != nil {
		return nil, err
	}
	p := tea.NewProgram(m, tea.WithOutput(output))
	return &Renderer{
		program: p,
	}, nil
}

func (r *Renderer) Send(ev domain.Event) {
	// Program.Send is thread-safe
	r.program.Send(msg{Event: ev})
}

func (r *Renderer) Wait() error {
	_, err := r.program.Run()
	return err
}
