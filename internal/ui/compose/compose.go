// Package compose wires engine, tea, markdown, theme, tool, and layout for the UI.
// Entrypoints (main, cmd/*) import this package to obtain a configured Renderer.
// Compose owns engine DI (NewEngineDeps) and composition.

package compose

import (
	"fmt"
	"io"
	"os"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/ui/engine"
	"github.com/Cyclone1070/iav/internal/ui/markdown"
	teapkg "github.com/Cyclone1070/iav/internal/ui/tea"
	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
)

// Renderer is the main entry point for the UI.
type Renderer struct {
	program *tea.Program
}

// NewRenderer creates a new Renderer writing to the given output.
func NewRenderer(output io.Writer, input io.Reader, cfg *config.Config) (*Renderer, error) {
	geom, err := resolveTermSize(cfg)
	if err != nil {
		return nil, err
	}

	width := geom.Width
	mdRenderer, err := markdown.NewGlamourRenderer(width)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize markdown renderer: %w", err)
	}
	sm := markdown.NewStream(mdRenderer)

	state := engine.NewInitialState(geom)

	factory := func() engine.Deps {
		return NewEngineDeps(cfg, sm, width)
	}

	adapter := teapkg.NewTeaModelAdapter(state, factory, teapkg.ProductionSink{})
	p := tea.NewProgram(adapter, tea.WithOutput(output), tea.WithInput(input))
	return &Renderer{program: p}, nil
}

func resolveTermSize(cfg *config.Config) (engine.TermSize, error) {
	width := cfg.UI.ChatWindowWidth
	height := defaultTerminalHeight
	if w, h, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
		if w < width {
			width = w
		}
		height = h
	}
	return engine.TermSize{Width: width, Height: height}, nil
}

const defaultTerminalHeight = 24

// Send sends a message to the UI program.
func (r *Renderer) Send(m tea.Msg) {
	r.program.Send(m)
}

// Wait blocks until the program exits.
func (r *Renderer) Wait() error {
	_, err := r.program.Run()
	return err
}
