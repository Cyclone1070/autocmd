package ui

import (
	"log/slog"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
)

// Renderer renders markdown to ANSI strings. Failure should be handled internally.
type Renderer interface {
	Render(markdown string) string
}

// GlamourRenderer wraps glamour.TermRenderer to implement Renderer.
type GlamourRenderer struct {
	tr *glamour.TermRenderer
}

// Render implements Renderer. On error, it returns the original markdown and logs.
func (g *GlamourRenderer) Render(markdown string) string {
	rendered, err := g.tr.Render(markdown)
	if err != nil {
		slog.Warn("glamour render failed, falling back to raw text", "err", err)
		return markdown
	}
	return rendered
}

// PassthroughRenderer is a no-op renderer that returns markdown as-is.
type PassthroughRenderer struct{}

func (p *PassthroughRenderer) Render(markdown string) string {
	return markdown
}

// NewGlamourRenderer creates a Renderer using glamour with the given width.
// If glamour initialization fails, it logs the error and returns a PassthroughRenderer.
func NewGlamourRenderer(width int, isDark bool) Renderer {
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
	style.H1.Bold = new(true)
	style.H1.Underline = new(true)
	style.H1.Upper = new(true)

	// Disable document end padding
	style.Document.BlockSuffix = ""

	tr, err := glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		slog.Warn("failed to create glamour renderer, using passthrough", "err", err)
		return &PassthroughRenderer{}
	}
	return &GlamourRenderer{tr: tr}
}
