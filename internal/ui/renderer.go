package ui

import (
	"io"

	"github.com/Cyclone1070/iav/internal/config"
	tea "github.com/charmbracelet/bubbletea"
)

// CursorDetector abstracts the ability to query the current cursor position.
type CursorDetector interface {
	GetCursorRow() (int, error)
}

// Renderer is the main entry point for the UI.
type Renderer struct {
	program *tea.Program
}

// NewRenderer creates a new Renderer writing to the given output.
func NewRenderer(output io.Writer, input io.Reader, cfg *config.Config) (*Renderer, error) {
	cd := NewTerminalCursorDetector(input, output)
	m, err := newModel(cfg, cd)
	if err != nil {
		return nil, err
	}
	p := tea.NewProgram(m, tea.WithOutput(output), tea.WithInput(input))
	return &Renderer{
		program: p,
	}, nil
}

// Send sends a message to the UI program.
// Accepts any tea.Msg, including domain.Event types (TextEvent, ToolStartEvent, etc.)
// and Bubble Tea built-in messages (tea.KeyMsg, etc.).
// Note: tea.WindowSizeMsg is intentionally ignored by the model (width is locked at startup).
// Program.Send is thread-safe.
func (r *Renderer) Send(m tea.Msg) {
	r.program.Send(m)
}

func (r *Renderer) Wait() error {
	_, err := r.program.Run()
	return err
}
