// Package compose wires engine, tea, and markdown for UI.
// deps.go builds engine.Deps (DI for engine).

package compose

import (
	"fmt"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui/engine"
	"github.com/Cyclone1070/iav/internal/ui/layout"
	"github.com/Cyclone1070/iav/internal/ui/markdown"
	"github.com/Cyclone1070/iav/internal/ui/theme"
	"github.com/Cyclone1070/iav/internal/ui/tool"
)

// NewEngineDeps builds engine.Deps from config and markdown stream.
func NewEngineDeps(cfg *config.Config, sm *markdown.Stream, width int) engine.Deps {
	return engine.Deps{
		Markdown:     sm,
		Theme:        &themeAdapter{t: theme.NewTheme(cfg.UI)},
		Layout:       layoutAdapter{},
		ToolRenderer: newToolRenderer(cfg, width),
		Spinner:      nil, // Set at runtime
	}
}

type themeAdapter struct {
	t *theme.Theme
}

func (a *themeAdapter) Success(s string) string { return a.t.Success(s) }
func (a *themeAdapter) Error(s string) string   { return a.t.Error(s) }
func (a *themeAdapter) Muted(s string) string   { return a.t.Muted(s) }
func (a *themeAdapter) Primary(s string) string { return a.t.Primary(s) }

func (a *themeAdapter) Box(content string, width int, status theme.ToolStatus) string {
	return a.t.Box(content, width, status)
}

func (a *themeAdapter) Separator(width int, status theme.ToolStatus) string {
	return a.t.Separator(width, status)
}

type layoutAdapter struct{}

func (layoutAdapter) TruncateWithIndicator(content string, termHeight int) string {
	return layout.TruncateWithIndicator(content, termHeight)
}

// toolRenderer implements engine.ToolRenderer.
type toolRenderer struct {
	theme       *theme.Theme
	shellHeight int
	width       int
}

// newToolRenderer creates a new tool renderer with injected dependencies.
func newToolRenderer(cfg *config.Config, width int) *toolRenderer {
	return &toolRenderer{
		theme:       theme.NewTheme(cfg.UI),
		shellHeight: cfg.UI.ShellOutputHeight,
		width:       width,
	}
}

// Render implements engine.ToolRenderer.Render.
func (r *toolRenderer) Render(t *engine.ToolState, spinner engine.SpinnerViewProvider) string {
	status := t.Status

	var prefix string
	switch status {
	case theme.StatusRunning:
		if spinner != nil {
			prefix = r.theme.Primary(spinner.SpinnerView())
		}
	case theme.StatusSuccess:
		prefix = r.theme.Success("✓")
	case theme.StatusError:
		prefix = r.theme.Error("✗")
	}

	contentWidth := r.width - 2
	var content string
	switch d := t.Display.(type) {
	case domain.StringDisplay:
		content = tool.RenderString(r.theme, d, status, t.Err, prefix)
	case domain.DiffDisplay:
		content = tool.RenderDiff(contentWidth, r.theme, d, status, t.Err, prefix)
	case domain.ShellDisplay:
		content = tool.RenderShell(contentWidth, r.shellHeight, r.theme, d, t.ShellOutput, status, t.Err, prefix)
	default:
		content = tool.Pad(fmt.Sprintf("Unknown display type: %T", d), prefix)
	}
	return r.theme.Box(content, contentWidth, status)
}
