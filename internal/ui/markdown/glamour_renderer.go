package markdown

import (
	"github.com/charmbracelet/glamour"
)

// Renderer renders markdown to ANSI strings.
type Renderer interface {
	Render(markdown string) (string, error)
}

// GlamourRenderer wraps glamour.TermRenderer to implement Renderer.
type GlamourRenderer struct {
	tr *glamour.TermRenderer
}

// Render implements Renderer.
func (g *GlamourRenderer) Render(markdown string) (string, error) {
	return g.tr.Render(markdown)
}

// NewGlamourRenderer creates a Renderer using glamour with the given width.
func NewGlamourRenderer(width int) (Renderer, error) {
	tr, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil, err
	}
	return &GlamourRenderer{tr: tr}, nil
}

// WrapGlamour adapts an existing glamour.TermRenderer to Renderer (for tests).
func WrapGlamour(tr *glamour.TermRenderer) Renderer {
	return &GlamourRenderer{tr: tr}
}
