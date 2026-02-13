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
func NewEngineDeps(cfg *config.Config, sm *markdown.Stream, width int, getSpinnerView func() string) engine.Deps {
	th := theme.NewTheme(cfg.UI)
	shellHeight := cfg.UI.ShellOutputHeight
	return engine.Deps{
		Markdown: sm,
		Theme:    &themeAdapter{t: th},
		Layout:   layoutAdapter{},
		ViewTool: func(t *engine.ToolState) string {
			return viewTool(th, shellHeight, width, getSpinnerView, t)
		},
	}
}

type themeAdapter struct {
	t *theme.Theme
}

func (a *themeAdapter) Success(s string) string   { return a.t.Success(s) }
func (a *themeAdapter) Error(s string) string      { return a.t.Error(s) }
func (a *themeAdapter) Muted(s string) string      { return a.t.Muted(s) }
func (a *themeAdapter) Primary(s string) string    { return a.t.Primary(s) }
func (a *themeAdapter) SpinnerStyle() string       { return "" }

func (a *themeAdapter) Box(content string, width int, status engine.ToolStatus) string {
	return a.t.Box(content, width, toToolStatus(status))
}

func (a *themeAdapter) Separator(width int, status engine.ToolStatus) string {
	return a.t.Separator(width, toToolStatus(status))
}

func toToolStatus(s engine.ToolStatus) theme.ToolStatus {
	switch s {
	case engine.StatusRunning:
		return theme.StatusRunning
	case engine.StatusSuccess:
		return theme.StatusSuccess
	case engine.StatusError:
		return theme.StatusError
	default:
		return theme.StatusRunning
	}
}

type layoutAdapter struct{}

func (layoutAdapter) TruncateWithIndicator(content string, termHeight int) string {
	return layout.TruncateWithIndicator(content, termHeight)
}

func viewTool(th *theme.Theme, shellHeight, width int, getSpinnerView func() string, t *engine.ToolState) string {
	status := toToolStatus(t.Status)
	var prefix string
	switch status {
	case theme.StatusRunning:
		prefix = getSpinnerView()
	case theme.StatusSuccess:
		prefix = th.Success("✓")
	case theme.StatusError:
		prefix = th.Error("✗")
	}
	contentWidth := width - 2
	var content string
	switch d := t.Display.(type) {
	case domain.StringDisplay:
		content = tool.RenderString(th, d, status, t.Err, prefix)
	case domain.DiffDisplay:
		content = tool.RenderDiff(contentWidth, th, d, status, t.Err, prefix)
	case domain.ShellDisplay:
		content = tool.RenderShell(contentWidth, shellHeight, th, d, t.ShellOutput, status, t.Err, prefix)
	default:
		content = tool.Pad(fmt.Sprintf("Unknown display type: %T", d), prefix)
	}
	return th.Box(content, contentWidth, status)
}
