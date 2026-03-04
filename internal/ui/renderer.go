package ui

import (
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
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
func NewGlamourRenderer(width int, isDark bool) (Renderer, error) {
	var style ansi.StyleConfig
	if isDark {
		style = styles.DarkStyleConfig
	} else {
		style = styles.LightStyleConfig
	}

	// Remove background colors to ensure safe contrast on all palettes
	style.H1.BackgroundColor = nil
	style.Code.BackgroundColor = nil

	// Match H1 foreground to other headings (H2) and restore bold/underline
	style.H1.Color = style.H2.Color
	style.H1.Bold = ptr(true)
	style.H1.Underline = ptr(true)
	style.H1.Upper = ptr(true)

	// Disable document end padding
	style.Document.BlockSuffix = ""

	tr, err := glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil, err
	}
	return &GlamourRenderer{tr: tr}, nil
}

func ptr[T any](v T) *T {
	return &v
}
